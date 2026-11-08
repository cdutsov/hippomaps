package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// encodePolyline is the inverse of decodePolyline, for the fake.
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
		lat, lon := int(p[1]*precision+0.5*sign(p[1])), int(p[0]*precision+0.5*sign(p[0]))
		enc(lat - plat)
		enc(lon - plon)
		plat, plon = lat, lon
	}
	return b.String()
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

var routeLine = [][2]float64{{8.5403, 47.3779}, {8.2, 47.2}, {7.4391, 46.949}}

func fakeValhalla(t *testing.T, calls *atomic.Int32, last *map[string]any) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == http.MethodPost && r.URL.Path == "/v1/elevation/profile" {
			var req struct {
				Path    [][2]float64 `json:"path"`
				Samples int          `json:"samples"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if r.Header.Get("X-API-Key") != "ek_test" || len(req.Path) != 3 || req.Path[0] != [2]float64{routeLine[0][1], routeLine[0][0]} || req.Samples < 2 || req.Samples > 300 {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"status":"OK","results":[{"elevation":408.2,"distance":0},{"elevation":null,"distance":500},{"elevation":540,"distance":1000.4}],`+
				`"profile":{"distance_m":1000.4,"ascent_m":140.4,"descent_m":8.6,"min_m":405,"max_m":541}}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/trace_attributes" {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if req["encoded_polyline"] != encodePolyline(routeLine, 1e6) || req["shape_match"] != "edge_walk" {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"edges":[{"speed_limit":50,"begin_shape_index":0,"end_shape_index":1},{"speed_limit":"unlimited","begin_shape_index":1,"end_shape_index":1},`+
				`{"speed_limit":120,"begin_shape_index":1,"end_shape_index":2}]}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/route" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		*last = req
		locs := req["locations"].([]any)
		if locs[0].(map[string]any)["lat"].(float64) == 0 { // the sea
			w.WriteHeader(400)
			io.WriteString(w, `{"error_code":171,"error":"No suitable edges near location"}`)
			return
		}
		io.WriteString(w, `{"trip":{"summary":{"length":124.321,"time":5400.4},"legs":[{"shape":"`+encodePolyline(routeLine, 1e6)+
			`","maneuvers":[{"instruction":"Drive west.","length":60.1,"time":2500,"type":1,"begin_shape_index":0,"end_shape_index":1,`+
			`"verbal_pre_transition_instruction":"Drive west on A1.","street_names":["A1"],"highway":true,`+
			`"sign":{"exit_number_elements":[{"text":"52"}],"exit_branch_elements":[{"text":"A1"},{"text":"E 25"}],"exit_toward_elements":[{"text":"Bern"},{"text":"Basel"}]}},`+
			`{"instruction":"You have arrived.","length":0,"time":0,"type":4,"begin_shape_index":2}]}]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRoute(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL + "/"; c.ElevationURL = v.URL; c.ElevationKey = "ek_test" })
	if w := get(s, "/labs/maps"); !strings.Contains(w.Body.String(), `data-route="/labs/maps/api/route"`) {
		t.Fatalf("page has no data-route")
	}
	w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=car&lang=de")
	if w.Code != 200 {
		t.Fatalf("route: %d %s", w.Code, w.Body)
	}
	var a routeAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.Km != 124.321 || a.S != 5400 || len(a.Line) != 3 || len(a.Steps) != 2 {
		t.Fatalf("answer: %+v", a)
	}
	for i, p := range routeLine {
		if d := a.Line[i][0] - p[0] + a.Line[i][1] - p[1]; d > 1e-6 || d < -1e-6 {
			t.Errorf("line %d: %v, want %v", i, a.Line[i], p)
		}
	}
	if a.Steps[1].At != a.Line[2] || a.Steps[0].Text != "Drive west." {
		t.Errorf("steps: %+v", a.Steps)
	}
	st := a.Steps[0]
	if st.Pre != "Drive west on A1." || st.Street != "A1" || !st.Highway || st.Exit != "52" || st.End != 1 ||
		strings.Join(st.Routes, ",") != "A1,E 25" || strings.Join(st.Toward, ",") != "Bern,Basel" {
		t.Errorf("step for navigation: %+v", st)
	}
	if len(a.Limits) != 2 || a.Limits[0] != [3]int{0, 1, 50} || a.Limits[1] != [3]int{1, 2, 120} {
		t.Errorf("limits: %v", a.Limits)
	}
	if a.Elev != nil {
		t.Errorf("heights by car: %+v", a.Elev)
	}
	if dt, _ := last["date_time"].(map[string]any); dt["type"] != 0.0 {
		t.Errorf("no date_time now: %v", last["date_time"])
	}
	if last["costing"] != "auto" || last["language"] != "de-DE" || last["units"] != "kilometers" {
		t.Errorf("request: %v", last)
	}
	get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=walk&lang=xx")
	if last["costing"] != "pedestrian" || last["language"] != "en-US" {
		t.Errorf("walk: %v", last)
	}
	w = get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=walk")
	if strings.Contains(w.Body.String(), `"limits"`) {
		t.Errorf("limits on foot: %s", w.Body)
	}
	var b routeAnswer
	json.Unmarshal(w.Body.Bytes(), &b)
	if e := b.Elev; e == nil || e.Up != 140 || e.Down != 9 || e.Min != 405 || e.Max != 541 || len(e.Pts) != 2 || e.Pts[1] != [2]float64{1, 540} {
		t.Errorf("heights on foot: %+v", b.Elev)
	}

	n := calls.Load()
	for _, u := range []string{
		"/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=plane",
		"/labs/maps/api/route?from=97,8&to=46.949,7.4391&mode=car",
		"/labs/maps/api/route?from=47.3&to=46.949,7.4391&mode=car",
		"/labs/maps/api/route?from=NaN,8&to=46.949,7.4391&mode=car",
	} {
		if w := get(s, u); w.Code != 400 || !strings.Contains(w.Body.String(), `"bad_request"`) {
			t.Errorf("%s: %d %s", u, w.Code, w.Body)
		}
	}
	// Zurich to Lisbon is 1700 km in a line: too far by car, so Valhalla is not asked
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=38.72,-9.14&mode=car"); w.Code != 400 || !strings.Contains(w.Body.String(), `"too_far"`) {
		t.Errorf("too far: %d %s", w.Code, w.Body)
	}
	if calls.Load() != n {
		t.Errorf("Valhalla asked for a bad request")
	}
	if w := get(s, "/labs/maps/api/route?from=0,0&to=0.1,0.1&mode=car"); w.Code != 404 || !strings.Contains(w.Body.String(), "No road near") {
		t.Errorf("no road: %d %s", w.Code, w.Body)
	}
}

func TestRouteLimit(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) {
		c.RoutingURL = v.URL
		c.ElevationURL = v.URL
		c.ElevationKey = "ek_test"
		c.RoutesPerIP = 2
	})
	u := "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=bike"
	for i, want := range []int{200, 200, 429} {
		if w := get(s, u, "X-Forwarded-For", "203.0.113.9"); w.Code != want {
			t.Errorf("call %d: %d", i, w.Code)
		}
	}
	if w := get(s, u, "X-Forwarded-For", "203.0.113.10"); w.Code != 200 {
		t.Errorf("other visitor: %d", w.Code)
	}
}

func TestRouteOff(t *testing.T) {
	s := testServer(t, nil)
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=car"); w.Code != 404 {
		t.Errorf("route without RoutingURL: %d", w.Code)
	}
	if strings.Contains(get(s, "/labs/maps").Body.String(), "data-route") {
		t.Errorf("page has data-route without RoutingURL")
	}
}

func TestRouteUpstreamDown(t *testing.T) {
	v := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	t.Cleanup(v.Close)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL; c.ElevationURL = v.URL; c.ElevationKey = "ek_test" })
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=car"); w.Code != 502 || !strings.Contains(w.Body.String(), `"failed"`) {
		t.Errorf("down: %d %s", w.Code, w.Body)
	}
}

// a route on foot without heights, when the elevation API fails
func TestRouteHeightsDown(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL; c.ElevationURL = v.URL; c.ElevationKey = "wrong" })
	w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=walk")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"elev"`) {
		t.Errorf("heights down: %d %s", w.Code, w.Body)
	}
}
