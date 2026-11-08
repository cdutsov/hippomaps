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
// answers {"km","s","line":[[lon,lat],…],"steps":[…],"limits":[[from,to,kmh],…]}, or
// {"error":{"code","message"}}: 400 bad_request and too_far, 404 no_route, 429 too_many,
// 503 busy, 502 failed. A step has the text, its length and time, where it starts on the line
// (at, i, end), the spoken texts for navigation (alert before, pre at the turn, post after),
// the street, and what a sign shows (exit number and name, route numbers, toward). limits are
// the speed limits along the line by car (indexes of line), from a second call to Valhalla
// (trace_attributes); without them when that call fails. By bike and on foot, elev is the
// height profile from the elevation API of overpass.ch (ElevationURL, ElevationKey); without it
// when that call fails or there is no key.

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
	Text       string     `json:"text"`
	Km         float64    `json:"km"`
	S          float64    `json:"s"`
	At         [2]float64 `json:"at"` // lon, lat
	Type       int        `json:"type"`
	I          int        `json:"i"`   // index of line where the step starts
	End        int        `json:"end"` // and ends
	Alert      string     `json:"alert,omitempty"`
	Pre        string     `json:"pre,omitempty"`
	Post       string     `json:"post,omitempty"`
	Street     string     `json:"street,omitempty"`
	Highway    bool       `json:"highway,omitempty"`
	Roundabout int        `json:"roundabout,omitempty"` // the exit to take
	Exit       string     `json:"exit,omitempty"`       // the number of the exit
	ExitName   string     `json:"exit_name,omitempty"`
	Routes     []string   `json:"routes,omitempty"` // route numbers on the sign
	Toward     []string   `json:"toward,omitempty"`
}

type routeAnswer struct {
	Km     float64      `json:"km"`
	S      float64      `json:"s"`
	Line   [][2]float64 `json:"line"`
	Steps  []routeStep  `json:"steps"`
	Limits [][3]int     `json:"limits,omitempty"`
	Elev   *routeElev   `json:"elev,omitempty"`
}

// routeElev is the height profile of a route by bike or on foot, from the elevation API of
// overpass.ch: up and down in metres, the lowest and highest point, and the heights along the
// line as [km from the start, metres].
type routeElev struct {
	Up   float64      `json:"up"`
	Down float64      `json:"down"`
	Min  float64      `json:"min"`
	Max  float64      `json:"max"`
	Pts  [][2]float64 `json:"pts"`
}

type signElement struct {
	Text string `json:"text"`
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
				Instruction string   `json:"instruction"`
				Length      float64  `json:"length"`
				Time        float64  `json:"time"`
				Type        int      `json:"type"`
				Begin       int      `json:"begin_shape_index"`
				End         int      `json:"end_shape_index"`
				Alert       string   `json:"verbal_transition_alert_instruction"`
				Pre         string   `json:"verbal_pre_transition_instruction"`
				Post        string   `json:"verbal_post_transition_instruction"`
				Streets     []string `json:"street_names"`
				Highway     bool     `json:"highway"`
				Roundabout  int      `json:"roundabout_exit_count"`
				Sign        struct {
					Number []signElement `json:"exit_number_elements"`
					Branch []signElement `json:"exit_branch_elements"`
					Toward []signElement `json:"exit_toward_elements"`
					Name   []signElement `json:"exit_name_elements"`
				} `json:"sign"`
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
		st := routeStep{Text: m.Instruction, Km: round(m.Length, 3), S: math.Round(m.Time), Type: m.Type, I: m.Begin, End: m.End,
			Alert: m.Alert, Pre: m.Pre, Post: m.Post, Street: strings.Join(m.Streets, " / "), Highway: m.Highway, Roundabout: m.Roundabout,
			Exit: strings.Join(texts(m.Sign.Number), " "), ExitName: strings.Join(texts(m.Sign.Name), " / "),
			Routes: texts(m.Sign.Branch), Toward: texts(m.Sign.Toward)}
		if m.Begin >= 0 && m.Begin < len(a.Line) {
			st.At = a.Line[m.Begin]
		}
		a.Steps = append(a.Steps, st)
	}
	if mode.costing == "auto" {
		a.Limits = s.speedLimits(ctx, leg.Shape)
	} else if s.cfg.ElevationKey != "" {
		a.Elev = s.heights(ctx, a.Line, a.Km)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a)
}

// heights asks the elevation API for the profile of a line of km, in about 300 samples (one
// every 10 m at least); nil when the call fails.
func (s *Server) heights(ctx context.Context, line [][2]float64, km float64) *routeElev {
	if len(line) < 2 {
		return nil
	}
	path := make([][2]float64, len(line))
	for i, p := range line {
		path[i] = [2]float64{p[1], p[0]}
	}
	samples := int(math.Min(300, math.Max(2, km*100)))
	body, _ := json.Marshal(map[string]any{"path": path, "samples": samples})
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.ElevationURL, "/")+"/v1/elevation/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.cfg.ElevationKey)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		s.cfg.Log.Warn("elevation", "err", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.cfg.Log.Warn("elevation", "status", resp.StatusCode)
		return nil
	}
	var v struct {
		Results []struct {
			Elevation *float64 `json:"elevation"`
			Distance  *float64 `json:"distance"`
		} `json:"results"`
		Profile *struct {
			Ascent  float64  `json:"ascent_m"`
			Descent float64  `json:"descent_m"`
			Min     *float64 `json:"min_m"`
			Max     *float64 `json:"max_m"`
		} `json:"profile"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&v); err != nil || v.Profile == nil || v.Profile.Min == nil || v.Profile.Max == nil {
		s.cfg.Log.Warn("elevation", "err", "unreadable answer")
		return nil
	}
	e := &routeElev{Up: math.Round(v.Profile.Ascent), Down: math.Round(v.Profile.Descent), Min: math.Round(*v.Profile.Min), Max: math.Round(*v.Profile.Max)}
	for _, r := range v.Results {
		if r.Elevation != nil && r.Distance != nil {
			e.Pts = append(e.Pts, [2]float64{round(*r.Distance/1000, 3), round(*r.Elevation, 1)})
		}
	}
	if len(e.Pts) < 2 {
		return nil
	}
	return e
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

func texts(es []signElement) []string {
	var out []string
	for _, e := range es {
		if e.Text != "" {
			out = append(out, e.Text)
		}
	}
	return out
}

// speedLimits are the posted limits along a route line by car, as [from, to, km/h] in indexes
// of the line, from Valhalla trace_attributes; nil when Valhalla does not answer.
func (s *Server) speedLimits(ctx context.Context, shape string) [][3]int {
	body, _ := json.Marshal(map[string]any{
		"encoded_polyline": shape, "costing": "auto", "shape_match": "edge_walk",
		"filters": map[string]any{"attributes": []string{"edge.speed_limit", "edge.begin_shape_index", "edge.end_shape_index"}, "action": "include"},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/trace_attributes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var v struct {
		Edges []struct {
			Limit json.RawMessage `json:"speed_limit"` // a number, or "unlimited"
			Begin int             `json:"begin_shape_index"`
			End   int             `json:"end_shape_index"`
		} `json:"edges"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v) != nil {
		return nil
	}
	var out [][3]int
	for _, e := range v.Edges {
		kmh := 0
		if n, err := strconv.Atoi(string(e.Limit)); err == nil && n > 0 && n < 200 {
			kmh = n
		}
		if kmh == 0 {
			continue
		}
		if k := len(out) - 1; k >= 0 && out[k][2] == kmh && out[k][1] >= e.Begin {
			out[k][1] = e.End
			continue
		}
		out = append(out, [3]int{e.Begin, e.End, kmh})
	}
	return out
}
