package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// How far one gets from a point in some minutes (isochrones of Valhalla, RoutingURL); the tool Reachable area of
// the page:
//
//	GET {base}/api/reach?at=47.37,8.54&mode=car|bike|walk&min=15,30,45,60
//
// min: 1 to reachMaxContours times in minutes, rising, each at most the longest of the mode (reachMaxMin). By car
// leaving now: live traffic and closures count. The answer is GeoJSON: a FeatureCollection of one MultiPolygon per
// time, the shortest first, with the properties min and mode; coordinates to 5 decimals (about 1 m). Errors as those
// of /api/route: 400 bad_request, 404 no_road (none near the point) and no_route, 429 too_many, 503 busy, 502 failed.
// A visitor gets as many as directions (RoutesPerIP), counted apart (reachPerIP); a call takes a place among the calls to
// Valhalla (routeCalls).

// reachMaxMin is the longest time of each mode, in minutes: an hour by car takes Valhalla about 2.5 s of a thread
// (90 minutes 6 s, two hours 9 s), two hours on foot or by bike under 1 s
var reachMaxMin = map[string]int{"car": 60, "bike": 120, "walk": 120}

// reachMaxContours is the most times at once (max_contours of Valhalla)
const reachMaxContours = 4

// reachGeneralize: the tolerance of Valhalla for simplifying the outlines, in metres: finer on foot, coarser for an hour
// by car (the answer for an hour around Zurich: 200 kB with 50 m)
func reachGeneralize(mode string, longest int) float64 {
	switch {
	case mode == "walk":
		return 20
	case mode == "bike" || longest <= 20:
		return 40
	}
	return 60
}

type reachFeature struct {
	Type       string `json:"type"`
	Properties struct {
		Min  int    `json:"min"`
		Mode string `json:"mode"`
	} `json:"properties"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates [][][][]float64 `json:"coordinates"`
	} `json:"geometry"`
}

// readMinutes reads "15,30,45": rising whole minutes from 1 to most, at most reachMaxContours
func readMinutes(v string, most int) ([]int, bool) {
	var out []int
	for _, s := range strings.Split(v, ",") {
		m, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || m < 1 || m > most || (len(out) > 0 && m <= out[len(out)-1]) {
			return nil, false
		}
		out = append(out, m)
	}
	return out, len(out) > 0 && len(out) <= reachMaxContours
}

func (s *Server) reach(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, lon, ok := latLon(q.Get("at"))
	name := q.Get("mode")
	mode, okm := modes[name]
	most := reachMaxMin[name]
	mins, okt := readMinutes(q.Get("min"), most)
	if !ok || !okm || most == 0 || !okt {
		routeError(w, http.StatusBadRequest, "bad_request", "at as lat,lon, mode car, bike or walk, and min as 1 to 4 rising minutes (car up to 60, bike and walk up to 120)")
		return
	}
	if !s.reachPerIP.allow(visitor(r), time.Now()) {
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many searches in a short time. Please wait a few minutes.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	select {
	case s.routeCalls <- struct{}{}:
		defer func() { <-s.routeCalls }()
	case <-time.After(5 * time.Second):
		routeError(w, http.StatusServiceUnavailable, "busy", "Directions are busy. Please try again.")
		return
	case <-ctx.Done():
		return
	}
	contours := make([]map[string]int, len(mins))
	for i, m := range mins {
		contours[i] = map[string]int{"time": m}
	}
	ask := map[string]any{
		"locations":  []map[string]float64{{"lat": lat, "lon": lon}},
		"costing":    mode.costing,
		"contours":   contours,
		"polygons":   true,
		"denoise":    0.3, // leaves out small islands of the answer under 30 % of the largest
		"generalize": reachGeneralize(name, mins[len(mins)-1]),
	}
	if name == "car" {
		ask["date_time"] = map[string]int{"type": 0} // leaving now: live traffic and closures count
	}
	body, _ := json.Marshal(ask)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/isochrone", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		if !errors.Is(r.Context().Err(), context.Canceled) {
			s.log.Warn("reach failed", "err", err)
		}
		routeError(w, http.StatusBadGateway, "failed", "The area could not be found. Please try again.")
		return
	}
	defer resp.Body.Close()
	var v struct {
		Features []struct {
			Properties struct {
				Contour float64 `json:"contour"`
			} `json:"properties"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
		ErrorCode int `json:"error_code"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v)
	switch {
	case resp.StatusCode >= 500 || (err != nil && resp.StatusCode == http.StatusOK):
		s.log.Warn("reach failed", "status", resp.StatusCode, "err", err)
		routeError(w, http.StatusBadGateway, "failed", "The area could not be found. Please try again.")
		return
	case resp.StatusCode != http.StatusOK || len(v.Features) == 0:
		if v.ErrorCode == 171 {
			routeError(w, http.StatusNotFound, "no_road", "No road near this point.")
		} else {
			routeError(w, http.StatusNotFound, "no_route", "No way from this point.")
		}
		return
	}
	out := struct {
		Type     string          `json:"type"`
		Features []*reachFeature `json:"features"`
	}{Type: "FeatureCollection"}
	for _, f := range v.Features {
		var polys [][][][]float64
		switch f.Geometry.Type {
		case "Polygon":
			var p [][][]float64
			if json.Unmarshal(f.Geometry.Coordinates, &p) == nil {
				polys = [][][][]float64{p}
			}
		case "MultiPolygon":
			json.Unmarshal(f.Geometry.Coordinates, &polys)
		}
		polys = roundPolygons(polys)
		if len(polys) == 0 {
			continue
		}
		g := &reachFeature{Type: "Feature"}
		g.Properties.Min = int(math.Round(f.Properties.Contour))
		g.Properties.Mode = name
		g.Geometry.Type = "MultiPolygon"
		g.Geometry.Coordinates = polys
		out.Features = append(out.Features, g)
	}
	if len(out.Features) == 0 {
		routeError(w, http.StatusNotFound, "no_route", "No way from this point.")
		return
	}
	slices.SortFunc(out.Features, func(a, b *reachFeature) int { return a.Properties.Min - b.Properties.Min })
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}

// roundPolygons: the coordinates to 5 decimals, repeated points dropped; rings of fewer than 4 points and polygons
// without an outer ring left out
func roundPolygons(polys [][][][]float64) [][][][]float64 {
	var out [][][][]float64
	for _, p := range polys {
		var rings [][][]float64
		for i, ring := range p {
			var rr [][]float64
			for _, c := range ring {
				if len(c) < 2 {
					continue
				}
				pt := []float64{math.Round(c[0]*1e5) / 1e5, math.Round(c[1]*1e5) / 1e5}
				if n := len(rr); n > 0 && rr[n-1][0] == pt[0] && rr[n-1][1] == pt[1] {
					continue
				}
				rr = append(rr, pt)
			}
			if len(rr) < 4 {
				if i == 0 {
					break // no outer ring: the polygon goes
				}
				continue
			}
			if a, b := rr[0], rr[len(rr)-1]; a[0] != b[0] || a[1] != b[1] {
				rr = append(rr, []float64{a[0], a[1]})
			}
			rings = append(rings, rr)
		}
		if len(rings) > 0 {
			out = append(out, rings)
		}
	}
	return out
}
