package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Directions from Valhalla (RoutingURL). The browser asks this server only:
//
//	GET {base}/api/route?from=47.37,8.54&via=47.05,8.31&to=46.95,7.44&mode=car|bike|walk&lang=en
//
// via: stops on the way, in order, up to maxStops (each a leg of Valhalla; the legs are joined into
// one line, and stops holds the index of the line where each stop is).
//
// answers {"km","s","line":[[lon,lat],…],"steps":[…],"stops":[i,…],"limits":[[from,to,kmh],…]}, or
// {"error":{"code","message"}}: 400 bad_request and too_far, 404 no_route, no_road (none near a point) and
// closed_now (a route at another time: a pass closed in winter, a road closed by traffic), 429 too_many,
// 503 busy, 502 failed. A step has the text, its length and time, where it starts on the line
// (at, i, end), the spoken texts for navigation (alert before, pre at the turn, post after),
// the street, and what a sign shows (exit number and name, route numbers, toward). limits are
// the speed limits along the line by car (indexes of line), from a second call to Valhalla
// (trace_attributes, leaving now, in pieces of at most traceMaxKm), with the stretches slowed by traffic and the
// delay; without them where that call fails. marks: speed cameras, stop signs and speed bumps on the route by
// car (marks.go), from Overpass; none when it fails. ferries: the boats and car trains on the way (Valhalla: a
// rail ferry, a ferry without the flag ferry), from the steps that take them. By bike and on foot, elev is the
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

// maxStops is the most stops between start and destination.
const maxStops = 8

// the languages of the steps (and of the voice, sayLangs): the language of navigation chosen in the page, else
// that of the page; Norwegian also as no (browsers) and nn
var languages = map[string]string{"en": "en-US", "de": "de-DE", "fr": "fr-FR", "it": "it-IT", "es": "es-ES", "nl": "nl-NL", "pt": "pt-PT",
	"bg": "bg-BG", "el": "el-GR", "et": "et-EE", "fi": "fi-FI", "hu": "hu-HU", "nb": "nb-NO", "no": "nb-NO", "nn": "nb-NO", "pl": "pl-PL",
	"ro": "ro-RO", "ru": "ru-RU", "sv": "sv-SE", "uk": "uk-UA"}

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
	Lanes      []lane     `json:"lanes,omitempty"` // at the turn, from left to right (car)
	Stop       int        `json:"stop,omitempty"`  // the arrival at a stop on the way: its number, from 1
}

// lane is one lane before a turn, from Valhalla (turn_lanes): its arrows as bits (1 none,
// 2 through, 4 sharp left, 8 left, 16 slight left, 32 slight right, 64 right, 128 sharp right,
// 256 reverse, 512 merge left, 1024 merge right), and the arrow to take in it: active, the best
// lanes; valid, lanes that also lead there.
type lane struct {
	Dirs   int `json:"d"`
	Active int `json:"a,omitempty"`
	Valid  int `json:"v,omitempty"`
}

type routeAnswer struct {
	Km      float64      `json:"km"`
	S       float64      `json:"s"`
	Line    [][2]float64 `json:"line"`
	Steps   []routeStep  `json:"steps"`
	Stops   []int        `json:"stops,omitempty"` // the index of line at each stop on the way
	Limits  [][3]int     `json:"limits,omitempty"`
	Traffic [][3]int     `json:"traffic,omitempty"` // by car: slow (1) and queued (2) stretches, in indexes of line
	Delay   float64      `json:"delay,omitempty"`   // by car: seconds that traffic adds
	Tunnels [][3]int     `json:"tunnels,omitempty"` // by car: [from, to, metres] in indexes of line
	Marks   []routeMark  `json:"marks,omitempty"`   // by car: speed cameras (where allowed), stop signs, speed bumps (marks.go)
	Ferries []routeFerry `json:"ferries,omitempty"` // boats and car trains on the way, in their order
	Elev    *routeElev   `json:"elev,omitempty"`
	lang    string       // the language of the steps (bg-BG): fixes of its phrases in addSteps
}

// routeFerry is a boat or a car train on the way (both a ferry for Valhalla): k ferry or train, its name, where
// it starts and ends on the line, its length and time (with no waiting for the departure).
type routeFerry struct {
	K    string  `json:"k"`
	Name string  `json:"name,omitempty"`
	I    int     `json:"i"`
	End  int     `json:"end"`
	Km   float64 `json:"km"`
	S    float64 `json:"s"`
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
			Shape     string     `json:"shape"`
			Maneuvers []maneuver `json:"maneuvers"`
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

// routeAtAnyTime: is there a route for the request without a time (closures by date and traffic not counted)?
func (s *Server) routeAtAnyTime(ctx context.Context, ask map[string]any) bool {
	again := map[string]any{}
	for k, v := range ask {
		if k != "date_time" {
			again[k] = v
		}
	}
	body, _ := json.Marshal(again)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var v valhallaRoute
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v) == nil && len(v.Trip.Legs) > 0
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
	var stops [][2]float64 // lat, lon
	for _, v := range q["via"] {
		lat, lon, ok := latLon(v)
		if !ok || len(stops) == maxStops {
			routeError(w, http.StatusBadRequest, "bad_request", "via as lat,lon, at most "+strconv.Itoa(maxStops)+" stops")
			return
		}
		stops = append(stops, [2]float64{lat, lon})
	}
	// the way in a line through the stops
	d, prev := 0.0, [2]float64{lat1, lon1}
	for _, p := range append(stops, [2]float64{lat2, lon2}) {
		d += kmBetween(prev[0], prev[1], p[0], p[1])
		prev = p
	}
	if d > mode.maxKm {
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
	start := map[string]any{"lat": lat1, "lon": lon1}
	// heading: the way the traveller goes (degrees from north), when navigation asks for a new route on
	// the way; Valhalla then starts on a road in that direction, not with turning round
	if h, err := strconv.ParseFloat(q.Get("heading"), 64); err == nil && h >= 0 && h < 360 {
		start["heading"] = math.Round(h)
		start["heading_tolerance"] = 45
	}
	locations := []map[string]any{start}
	for _, p := range stops {
		locations = append(locations, map[string]any{"lat": p[0], "lon": p[1], "type": "break"})
	}
	locations = append(locations, map[string]any{"lat": lat2, "lon": lon2})
	ask := map[string]any{
		"locations":  locations,
		"costing":    mode.costing,
		"units":      "kilometers",
		"language":   lang,
		"date_time":  map[string]int{"type": 0}, // leaving now: live traffic and closures count
		"turn_lanes": true,
	}
	body, _ := json.Marshal(ask)
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
			code, msg = "no_road", "No road near one of the points."
		} else if v.ErrorCode == 442 && s.routeAtAnyTime(ctx, ask) {
			// a way at another time: a road closed now (a pass in winter, access:conditional), or closed by traffic
			code, msg = "closed_now", "The way is closed at this time, for example a pass in winter."
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
	a := routeAnswer{Km: round(v.Trip.Summary.Length, 3), S: math.Round(v.Trip.Summary.Time), Steps: []routeStep{}, Line: [][2]float64{}, lang: lang}
	for n, leg := range v.Trip.Legs {
		// the legs joined: each starts at the point where the one before ends
		off := 0
		pts := decodePolyline(leg.Shape, 1e6)
		if n > 0 && len(a.Line) > 0 && len(pts) > 0 {
			off = len(a.Line) - 1
			pts = pts[1:]
		}
		a.Line = append(a.Line, pts...)
		a.addSteps(leg.Maneuvers, off, n < len(v.Trip.Legs)-1, n+1)
		if n < len(v.Trip.Legs)-1 {
			a.Stops = append(a.Stops, len(a.Line)-1)
		}
	}
	if len(a.Line) < 2 {
		routeError(w, http.StatusNotFound, "no_route", "No route between these points.")
		return
	}
	if mode.costing == "auto" {
		var edges []tracedEdge
		a.Limits, a.Traffic, a.Tunnels, a.Delay, edges = s.alongRoute(ctx, a.Line)
		if s.overpass() {
			a.Marks = s.routeMarks(ctx, a.Line, edges)
		}
	} else if s.cfg.ElevationKey != "" {
		a.Elev = s.heights(ctx, a.Line, a.Km)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a)
}

type maneuver = struct {
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
	Ferry       bool     `json:"ferry"` // on a boat; a car train (a rail ferry) has no flag
	Roundabout  int      `json:"roundabout_exit_count"`
	Lanes       []struct {
		Directions int `json:"directions"`
		Active     int `json:"active"`
		Valid      int `json:"valid"`
	} `json:"lanes"`
	Sign struct {
		Number []signElement `json:"exit_number_elements"`
		Branch []signElement `json:"exit_branch_elements"`
		Toward []signElement `json:"exit_toward_elements"`
		Name   []signElement `json:"exit_name_elements"`
	} `json:"sign"`
}

// addSteps adds the maneuvers of a leg whose line starts at index off of a.Line; toStop: the leg
// ends at stop number stop, not at the destination.
func (a *routeAnswer) addSteps(ms []maneuver, off int, toStop bool, stop int) {
	names := legNames(ms)
	for k, m := range ms {
		m.Instruction, m.Alert, m.Pre, m.Post = unglue(m.Instruction, names), unglue(m.Alert, names), unglue(m.Pre, names), unglue(m.Post, names)
		if a.lang == "bg-BG" {
			m.Post = bgLengths(m.Post, m.Length)
		}
		st := routeStep{Text: m.Instruction, Km: round(m.Length, 3), S: math.Round(m.Time), Type: m.Type, I: m.Begin + off, End: m.End + off,
			Alert: m.Alert, Pre: m.Pre, Post: m.Post, Street: strings.Join(m.Streets, " / "), Highway: m.Highway, Roundabout: m.Roundabout,
			Exit: strings.Join(texts(m.Sign.Number), " "), ExitName: strings.Join(texts(m.Sign.Name), " / "),
			Routes: texts(m.Sign.Branch), Toward: texts(m.Sign.Toward)}
		// a way straight on ("Continue.", "Weiter."; also "X becomes Y", "Keep straight") says how far
		// it goes when it is said: Valhalla has the distance only in the sentence after the turn
		if (m.Type == 7 || m.Type == 8 || m.Type == 22) && m.Post != "" && strings.TrimSpace(m.Pre) == strings.TrimSpace(m.Instruction) {
			st.Pre, st.Post, st.Alert = m.Post, "", ""
		}
		if st.I >= 0 && st.I < len(a.Line) {
			st.At = a.Line[st.I]
		}
		if toStop && k == len(ms)-1 {
			st.Stop = stop
		}
		if m.Type == 28 { // onto a ferry: a boat, or a car train (Lötschberg, Sylt-Shuttle)
			f := routeFerry{K: "train", Name: longest(m.Streets), I: st.I, End: st.End, Km: st.Km, S: st.S}
			if m.Ferry {
				f.K = "ferry"
			}
			a.Ferries = append(a.Ferries, f)
		}
		for _, l := range m.Lanes {
			if l.Active != 0 || l.Valid != 0 { // lanes without a choice say nothing
				st.Lanes = make([]lane, len(m.Lanes))
				for i, l := range m.Lanes {
					st.Lanes[i] = lane{l.Directions, l.Active, l.Valid}
				}
				break
			}
		}
		a.Steps = append(a.Steps, st)
	}
}

// longest of the names of a ferry: that of its line ("Dover - Calais"), not a port ("Dover (UK)") or a road number
// ("E 47") beside it.
func longest(names []string) string {
	out := ""
	for _, n := range names {
		if utf8.RuneCountInString(n) > utf8.RuneCountInString(out) {
			out = n
		}
	}
	return out
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

// encodePolyline is the inverse of decodePolyline.
func encodePolyline(pts [][2]float64, precision float64) string {
	var b strings.Builder
	var plat, plon int
	enc := func(v int) {
		u := v << 1
		if v < 0 {
			u = ^u
		}
		for u >= 0x20 {
			b.WriteByte(byte((0x20 | (u & 0x1f)) + 63))
			u >>= 5
		}
		b.WriteByte(byte(u + 63))
	}
	for _, p := range pts {
		lat, lon := int(math.Round(p[1]*precision)), int(math.Round(p[0]*precision))
		enc(lat - plat)
		enc(lon - plon)
		plat, plon = lat, lon
	}
	return b.String()
}

// legNames: the names of the streets and signs of the maneuvers of a leg (a name of a roundabout exit is the
// street of the maneuver after it)
func legNames(ms []maneuver) []string {
	var names []string
	for _, m := range ms {
		names = append(names, m.Streets...)
		for _, es := range [][]signElement{m.Sign.Number, m.Sign.Branch, m.Sign.Toward, m.Sign.Name} {
			names = append(names, texts(es)...)
		}
	}
	return names
}

// unglue puts back the space a phrase of Valhalla leaves out before a name in some languages ("изход
// поAarestrasse" in Bulgarian, "a(z)Aarestrasse" in Hungarian, "motZürich" in Swedish): before a name of the leg
// that starts with a capital letter or a digit, right after a small letter or ")".
func unglue(s string, names []string) string {
	for _, n := range names {
		r, _ := utf8.DecodeRuneInString(n)
		if n == "" || !(unicode.IsUpper(r) || unicode.IsDigit(r)) || (len(n) < 2 && !unicode.IsDigit(r)) {
			continue
		}
		for from := 0; ; {
			i := strings.Index(s[from:], n)
			if i < 0 {
				break
			}
			i += from
			if p, _ := utf8.DecodeLastRuneInString(s[:i]); i > 0 && (unicode.IsLower(p) || p == ')') {
				s = s[:i] + " " + s[i:]
				i++
			}
			from = i + len(n)
		}
	}
	return s
}

// bgRe: a length without its number in the Bulgarian of Valhalla 3.9 (its words for lengths have no
// <METERS> and <KILOMETERS>: "Продължете метра.")
var bgRe = regexp.MustCompile(`(\p{L}) (метра|километри)(\P{L}|$)`)

// bgLengths puts the length of the step (km) into its Bulgarian sentence, rounded as Valhalla rounds it in
// the other languages: tens of metres up to 94 m, hundreds up to 949 m, tenths of kilometres beyond.
func bgLengths(s string, km float64) string {
	if !bgRe.MatchString(s) {
		return s
	}
	m, say := math.Round(km*1000), ""
	switch {
	case m > 949:
		if k := math.Round(km*10) / 10; k == 1 {
			say = "1 километър"
		} else {
			say = strings.Replace(strconv.FormatFloat(k, 'f', -1, 64), ".", ",", 1) + " километра"
		}
	case m > 94:
		say = strconv.Itoa(int(math.Round(m/100)*100)) + " метра"
	case m > 9:
		say = strconv.Itoa(int(math.Round(m/10)*10)) + " метра"
	default:
		say = "по-малко от 10 метра"
	}
	return bgRe.ReplaceAllString(s, "$1 "+say+"$3")
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

// alongRoute reads the edges along a route line by car from Valhalla (trace_attributes, leaving
// now): the posted limits as [from, to, km/h], the traffic as [from, to, level] (1 slow: under 75 %
// of the usual speed; 2 queue: under 50 %, as it is now), in indexes of line, and the delay in
// seconds that live traffic adds to the time of the route (which counts live speeds less the
// further ahead they are), and the tunnels as [from, to, metres]; and the edges themselves (traceEdges).
func (s *Server) alongRoute(ctx context.Context, line [][2]float64) (limits, traffic, tunnels [][3]int, delay float64, edges []tracedEdge) {
	edges = s.traceEdges(ctx, line)
	merge := func(out [][3]int, a, b, val int) [][3]int {
		if k := len(out) - 1; k >= 0 && out[k][2] == val && out[k][1] >= a {
			out[k][1] = b
			return out
		}
		return append(out, [3]int{a, b, val})
	}
	for _, e := range edges {
		a, b := e.a, e.b
		if n, err := strconv.Atoi(string(e.Limit)); err == nil && n > 0 && n < 200 {
			limits = merge(limits, a, b, n)
		}
		if e.Tunnel { // one tunnel may be several edges
			if k := len(tunnels) - 1; k >= 0 && tunnels[k][1] >= a {
				tunnels[k][1] = b
				tunnels[k][2] += int(math.Round(e.Km * 1000))
			} else {
				tunnels = append(tunnels, [3]int{a, b, int(math.Round(e.Km * 1000))})
			}
		}
		cur, usual := e.Speeds.Current, e.Speeds.Usual
		if cur <= 0 || usual <= 0 {
			continue
		}
		if e.Used > 0 && e.Used < usual {
			delay += e.Km * 3600 * (1/e.Used - 1/usual)
		}
		level := 0
		switch r := cur / usual; {
		case r < 0.5:
			level = 2
		case r < 0.75:
			level = 1
		}
		if level > 0 {
			traffic = merge(traffic, a, b, level)
		}
	}
	return limits, traffic, tunnels, math.Round(delay), edges
}

// traceMaxKm is the longest piece of a line asked of trace_attributes: Valhalla matches at most 200 km
// (service_limits.trace.max_distance), so a longer route is asked in pieces.
const traceMaxKm = 180

// tracedEdge is an edge of a route by car from trace_attributes. a and b: where it begins and ends,
// in indexes of the whole line; country: the ISO code of the country at its end ("" unknown).
type tracedEdge struct {
	Limit  json.RawMessage `json:"speed_limit"` // a number, or "unlimited"
	Speeds struct {
		Current float64 `json:"current_flow"`
		Usual   float64 `json:"no_flow"`
	} `json:"speeds_non_faded"`
	Used    float64 `json:"speed"` // what the time of the route counts: live speeds fade out within an hour
	Tunnel  bool    `json:"tunnel"`
	Km      float64 `json:"length"`
	Begin   int     `json:"begin_shape_index"`
	End     int     `json:"end_shape_index"`
	Way     int64   `json:"way_id"`
	Forward bool    `json:"forward"` // along the way in the order of its nodes
	Class   string  `json:"road_class"`
	Right   bool    `json:"drive_on_right"`
	EndNode struct {
		Admin *int       `json:"admin_index"`
		Cross []struct { // the other edges at its end
			Drive string `json:"driveability"` // forward, backward, both; none or missing: not by car
			Class string `json:"road_class"`
		} `json:"intersecting_edges"`
	} `json:"end_node"`
	a, b    int
	country string
}

// traceEdges asks Valhalla for the edges along line (trace_attributes, leaving now), in pieces of at most
// traceMaxKm. A piece that fails leaves its stretch without edges. The matched shape may drop a point of
// the line; its indexes are mapped back.
func (s *Server) traceEdges(ctx context.Context, line [][2]float64) []tracedEdge {
	cum := cumKm(line)
	var out []tracedEdge
	for from := 0; from < len(line)-1; {
		to := from + 1
		for to < len(line)-1 && cum[to+1]-cum[from] <= traceMaxKm {
			to++
		}
		out = append(out, s.tracePiece(ctx, line[from:to+1], from)...)
		from = to
	}
	return out
}

// tracePiece is traceEdges for a piece of the line that starts at index off.
func (s *Server) tracePiece(ctx context.Context, piece [][2]float64, off int) []tracedEdge {
	body, _ := json.Marshal(map[string]any{
		"encoded_polyline": encodePolyline(piece, 1e6), "costing": "auto", "shape_match": "walk_or_snap", "date_time": map[string]int{"type": 0},
		"filters": map[string]any{"attributes": []string{"edge.speed_limit", "edge.speeds_non_faded", "edge.speed", "edge.length", "edge.tunnel",
			"edge.begin_shape_index", "edge.end_shape_index", "edge.way_id", "edge.forward", "edge.road_class", "edge.drive_on_right",
			"node.admin_index", "node.intersecting_edge.driveability", "node.intersecting_edge.road_class", "admin.country_code", "shape"}, "action": "include"},
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
		Shape  string       `json:"shape"`
		Edges  []tracedEdge `json:"edges"`
		Admins []struct {
			Country string `json:"country_code"`
		} `json:"admins"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v) != nil {
		return nil
	}
	at := shapeIndex(decodePolyline(v.Shape, 1e6), piece)
	out := v.Edges[:0]
	for _, e := range v.Edges {
		if e.Begin < 0 || e.End >= len(at) || e.Begin > e.End {
			continue
		}
		e.a, e.b = at[e.Begin]+off, at[e.End]+off
		if i := e.EndNode.Admin; i != nil && *i >= 0 && *i < len(v.Admins) {
			e.country = v.Admins[*i].Country
		}
		out = append(out, e)
	}
	return out
}

// shapeIndex maps each point of a matched shape to the index of the same point of line (both
// in order; the nearest of the next few)
func shapeIndex(matched, line [][2]float64) []int {
	at := make([]int, len(matched))
	j := 0
	for i, p := range matched {
		best, bestD := j, math.Inf(1)
		for k := j; k < len(line) && k < j+6; k++ {
			if d := math.Abs(line[k][0]-p[0]) + math.Abs(line[k][1]-p[1]); d < bestD {
				best, bestD = k, d
			}
		}
		at[i], j = best, best
	}
	return at
}
