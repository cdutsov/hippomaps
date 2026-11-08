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
			`","maneuvers":[{"instruction":"Drive west.","length":60.1,"time":2500,"type":1,"begin_shape_index":0},`+
			`{"instruction":"You have arrived.","length":0,"time":0,"type":4,"begin_shape_index":2}]}]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRoute(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL + "/" })
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
	if last["costing"] != "auto" || last["language"] != "de-DE" || last["units"] != "kilometers" {
		t.Errorf("request: %v", last)
	}
	get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=walk&lang=xx")
	if last["costing"] != "pedestrian" || last["language"] != "en-US" {
		t.Errorf("walk: %v", last)
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
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL; c.RoutesPerIP = 2 })
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
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL })
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=car"); w.Code != 502 || !strings.Contains(w.Body.String(), `"failed"`) {
		t.Errorf("down: %d %s", w.Code, w.Body)
	}
}
