package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Directions from Valhalla (RoutingURL). The browser asks this server only:
//
//	GET {base}/api/route?from=47.37,8.54&to=46.95,7.44&mode=car|bike|walk&lang=en
//
// answers {"km","s","line":[[lon,lat],…],"steps":[{"text","km","s","at":[lon,lat],"type"}]},
// or {"error":{"code","message"}}: 400 bad_request and too_far, 404 no_route, 429 too_many,
// 503 busy, 502 failed.

// modes are the modes of the page, as Valhalla costings, with the longest trip in a line (km).
var modes = map[string]struct {
	costing string
	maxKm   float64
}{
	"car":  {"auto", 1500},
	"bike": {"bicycle", 300},
	"walk": {"pedestrian", 100},
}

// languages of the steps, by the language of the browser
var languages = map[string]string{"en": "en-US", "de": "de-DE", "fr": "fr-FR", "it": "it-IT", "es": "es-ES", "nl": "nl-NL", "pt": "pt-PT"}

type routeStep struct {
	Text string     `json:"text"`
	Km   float64    `json:"km"`
	S    float64    `json:"s"`
	At   [2]float64 `json:"at"` // lon, lat
	Type int        `json:"type"`
}

type routeAnswer struct {
	Km    float64      `json:"km"`
	S     float64      `json:"s"`
	Line  [][2]float64 `json:"line"`
	Steps []routeStep  `json:"steps"`
}

// valhallaRoute is the part of an answer of Valhalla /route the page needs.
type valhallaRoute struct {
	Trip struct {
		Summary struct {
			Length float64 `json:"length"`
			Time   float64 `json:"time"`
		} `json:"summary"`
		Legs []struct {
			Shape     string `json:"shape"`
			Maneuvers []struct {
				Instruction string  `json:"instruction"`
				Length      float64 `json:"length"`
				Time        float64 `json:"time"`
				Type        int     `json:"type"`
				Begin       int     `json:"begin_shape_index"`
			} `json:"maneuvers"`
		} `json:"legs"`
	} `json:"trip"`
	ErrorCode int    `json:"error_code"`
	Error     string `json:"error"`
}

func routeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// latLon reads "47.37,8.54".
func latLon(v string) (lat, lon float64, ok bool) {
	a, b, found := strings.Cut(v, ",")
	if !found {
		return 0, 0, false
	}
	lat, e1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	lon, e2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if e1 != nil || e2 != nil || math.IsNaN(lat) || math.IsNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

// kmBetween is the distance on the earth in a line.
func kmBetween(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 2 * 6371 * math.Asin(math.Sqrt(a))
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat1, lon1, ok1 := latLon(q.Get("from"))
	lat2, lon2, ok2 := latLon(q.Get("to"))
	mode, okm := modes[q.Get("mode")]
	if !ok1 || !ok2 || !okm {
		routeError(w, http.StatusBadRequest, "bad_request", "from and to as lat,lon, and mode car, bike or walk")
		return
	}
	if d := kmBetween(lat1, lon1, lat2, lon2); d > mode.maxKm {
		routeError(w, http.StatusBadRequest, "too_far", "The points are too far apart for this mode.")
		return
	}
	if !s.routesPerIP.allow(clientIP(r), time.Now()) {
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many directions in a short time. Please wait a few minutes.")
		return
	}
	lang := languages[strings.ToLower(q.Get("lang"))]
	if lang == "" {
		lang = "en-US"
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
	body, _ := json.Marshal(map[string]any{
		"locations": []map[string]float64{{"lat": lat1, "lon": lon1}, {"lat": lat2, "lon": lon2}},
		"costing":   mode.costing,
		"units":     "kilometers",
		"language":  lang,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		if !errors.Is(r.Context().Err(), context.Canceled) {
			s.log.Warn("route failed", "err", err)
		}
		routeError(w, http.StatusBadGateway, "failed", "Directions failed. Please try again.")
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	var v valhallaRoute
	if err == nil {
		err = json.Unmarshal(raw, &v)
	}
	switch {
	case err != nil || resp.StatusCode >= 500:
		s.log.Warn("route failed", "status", resp.StatusCode, "err", err)
		routeError(w, http.StatusBadGateway, "failed", "Directions failed. Please try again.")
		return
	case resp.StatusCode != http.StatusOK || len(v.Trip.Legs) == 0:
		// 442 no path, 171 no road near a point, 154/150 over the limits of Valhalla
		code, msg := "no_route", "No route between these points."
		if v.ErrorCode == 171 {
			msg = "No road near one of the points."
		} else if v.ErrorCode >= 150 && v.ErrorCode <= 159 {
			code, msg = "too_far", "The points are too far apart for this mode."
		}
		status := http.StatusNotFound
		if code == "too_far" {
			status = http.StatusBadRequest
		}
		routeError(w, status, code, msg)
		return
	}
	a := routeAnswer{Km: round(v.Trip.Summary.Length, 3), S: math.Round(v.Trip.Summary.Time), Steps: []routeStep{}}
	leg := v.Trip.Legs[0]
	a.Line = decodePolyline(leg.Shape, 1e6)
	for _, m := range leg.Maneuvers {
		st := routeStep{Text: m.Instruction, Km: round(m.Length, 3), S: math.Round(m.Time), Type: m.Type}
		if m.Begin >= 0 && m.Begin < len(a.Line) {
			st.At = a.Line[m.Begin]
		}
		a.Steps = append(a.Steps, st)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a)
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// decodePolyline reads an encoded polyline (Valhalla: precision 1e6) as [lon, lat] pairs.
func decodePolyline(s string, precision float64) [][2]float64 {
	out := [][2]float64{}
	var lat, lon int
	for i := 0; i < len(s); {
		var d [2]int
		for k := range d {
			res, shift := 0, 0
			for {
				if i >= len(s) {
					return out
				}
				b := int(s[i]) - 63
				i++
				res |= (b & 0x1f) << shift
				shift += 5
				if b < 0x20 {
					break
				}
			}
			if res&1 != 0 {
				d[k] = ^(res >> 1)
			} else {
				d[k] = res >> 1
			}
		}
		lat += d[0]
		lon += d[1]
		out = append(out, [2]float64{float64(lon) / precision, float64(lat) / precision})
	}
	return out
}
