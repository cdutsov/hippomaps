package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Public transport on the map: the lines of metro, light rail, monorail and tram with their
// colours, from the Overpass API of overpass.ch (ElevationURL, ElevationKey: a key of all the APIs).
//
//	GET {base}/api/transit?tile=10/536/358   GeoJSON, a feature per route: {kind, ref, name, colour, network}
//
// The map asks by tiles of zoom 10 (about 25 km in Switzerland); each is kept in memory for a day.
// 400 for a bad tile, 429 too many, 503 busy, 502 failed.

const transitZoom = 10

var transitKinds = map[string]bool{"subway": true, "light_rail": true, "monorail": true, "tram": true}

func (s *Server) transit(w http.ResponseWriter, r *http.Request) {
	z, x, y, ok := parseTile(r.URL.Query().Get("tile"))
	if !ok || z != transitZoom {
		http.Error(w, "tile=10/x/y", http.StatusBadRequest)
		return
	}
	key := fmt.Sprintf("transit:%d/%d/%d", z, x, y)
	if b, ok := s.cache.get(key); ok {
		sendJSON(w, b)
		return
	}
	if !s.transitPerIP.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many", http.StatusTooManyRequests)
		return
	}
	select {
	case s.transitCalls <- struct{}{}:
		defer func() { <-s.transitCalls }()
	case <-time.After(5 * time.Second):
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	if b, ok := s.cache.get(key); ok { // asked meanwhile
		sendJSON(w, b)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()
	b, err := s.transitTile(ctx, z, x, y)
	if err != nil {
		s.log.Warn("transit", "tile", key, "err", err)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	s.cache.put(key, b)
	sendJSON(w, b)
}

func sendJSON(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(b)
}

func parseTile(v string) (z, x, y int, ok bool) {
	p := strings.Split(v, "/")
	if len(p) != 3 {
		return 0, 0, 0, false
	}
	var err [3]error
	z, err[0] = strconv.Atoi(p[0])
	x, err[1] = strconv.Atoi(p[1])
	y, err[2] = strconv.Atoi(p[2])
	n := 1 << z
	return z, x, y, err[0] == nil && err[1] == nil && err[2] == nil && z >= 0 && z < 20 && x >= 0 && x < n && y >= 0 && y < n
}

// tileBox is the box of a tile: south, west, north, east
func tileBox(z, x, y int) (s, w, n, e float64) {
	k := float64(int(1) << z)
	lat := func(t float64) float64 { return math.Atan(math.Sinh(math.Pi*(1-2*t/k))) * 180 / math.Pi }
	return lat(float64(y + 1)), float64(x)/k*360 - 180, lat(float64(y)), float64(x+1)/k*360 - 180
}

type geoFeature struct {
	Type       string            `json:"type"`
	Geometry   geoGeometry       `json:"geometry"`
	Properties map[string]string `json:"properties"`
}

type geoGeometry struct {
	Type        string         `json:"type"`
	Coordinates [][][2]float64 `json:"coordinates"`
}

func (s *Server) transitTile(ctx context.Context, z, x, y int) ([]byte, error) {
	so, we, no, ea := tileBox(z, x, y)
	box := fmt.Sprintf("%.5f,%.5f,%.5f,%.5f", so, we, no, ea)
	q := `[out:json][timeout:60];rel["route"~"^(subway|light_rail|monorail|tram)$"](` + box + `);out geom(` + box + `);`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.ElevationURL, "/")+"/v1/interpreter",
		bytes.NewReader([]byte("data="+url.QueryEscape(q))))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-API-Key", s.cfg.ElevationKey)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass: HTTP %d", resp.StatusCode)
	}
	var d struct {
		Elements []struct {
			Type    string            `json:"type"`
			ID      int64             `json:"id"`
			Tags    map[string]string `json:"tags"`
			Members []struct {
				Type     string `json:"type"`
				Role     string `json:"role"`
				Geometry []*struct {
					Lat float64 `json:"lat"`
					Lon float64 `json:"lon"`
				} `json:"geometry"`
			} `json:"members"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<20)).Decode(&d); err != nil {
		return nil, err
	}
	out := struct {
		Type     string       `json:"type"`
		Features []geoFeature `json:"features"`
	}{Type: "FeatureCollection", Features: []geoFeature{}}
	for _, e := range d.Elements {
		kind := e.Tags["route"]
		if e.Type != "relation" || !transitKinds[kind] {
			continue
		}
		var lines [][][2]float64
		for _, m := range e.Members {
			if m.Type != "way" || (m.Role != "" && m.Role != "forward" && m.Role != "backward") {
				continue // stops and platforms
			}
			var line [][2]float64
			flush := func() {
				if len(line) > 1 {
					lines = append(lines, line)
				}
				line = nil
			}
			for _, p := range m.Geometry {
				if p == nil { // the way leaves the tile here
					flush()
					continue
				}
				pt := [2]float64{round(p.Lon, 5), round(p.Lat, 5)}
				if len(line) == 0 || line[len(line)-1] != pt {
					line = append(line, pt)
				}
			}
			flush()
		}
		if len(lines) == 0 {
			continue
		}
		colour := e.Tags["colour"]
		if colour == "" {
			colour = e.Tags["color"]
		}
		props := map[string]string{"id": strconv.FormatInt(e.ID, 10), "kind": kind}
		for k, v := range map[string]string{"ref": e.Tags["ref"], "name": e.Tags["name"], "colour": colour, "network": e.Tags["network"]} {
			if v != "" {
				props[k] = v
			}
		}
		out.Features = append(out.Features, geoFeature{Type: "Feature", Geometry: geoGeometry{Type: "MultiLineString", Coordinates: lines}, Properties: props})
	}
	return json.Marshal(out)
}
