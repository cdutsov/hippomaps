package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeIsochrone answers /isochrone as Valhalla: the largest time first, a Polygon and a MultiPolygon, coordinates of 6
// decimals; at 0,0 (the sea) error 171
func fakeIsochrone(t *testing.T, calls *atomic.Int32, last *map[string]any) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/isochrone" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		*last = req
		if loc := req["locations"].([]any)[0].(map[string]any); loc["lat"] == 0.0 {
			w.WriteHeader(400)
			io.WriteString(w, `{"error_code":171,"error":"No suitable edges near location"}`)
			return
		}
		io.WriteString(w, `{"features":[`+
			`{"properties":{"contour":30.0,"metric":"time","fill":"#bf4040"},"geometry":{"type":"MultiPolygon","coordinates":[`+
			`[[[8.4,47.3],[8.7,47.3],[8.7,47.5],[8.4,47.5],[8.4,47.3]],[[8.5,47.35],[8.51,47.35],[8.51,47.36],[8.5,47.35]]],`+
			`[[[9.0,47.0],[9.000001,47.0],[9.0,47.0000001],[9.0,47.0]]]]},"type":"Feature"},`+
			`{"properties":{"contour":15.0,"metric":"time"},"geometry":{"type":"Polygon","coordinates":`+
			`[[[8.5123456,47.3612345],[8.6,47.37],[8.6,47.37],[8.55,47.42],[8.5123456,47.3612345]]]},"type":"Feature"}`+
			`],"type":"FeatureCollection"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestReach(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeIsochrone(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL; c.RoutesPerIP = 4 })
	w := get(s, "/labs/maps/api/reach?at=47.3779,8.5403&mode=car&min=15,30")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/geo+json") {
		t.Fatalf("reach: %d %s", w.Code, w.Body)
	}
	var fc struct {
		Type     string          `json:"type"`
		Features []*reachFeature `json:"features"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &fc); err != nil {
		t.Fatal(err)
	}
	if fc.Type != "FeatureCollection" || len(fc.Features) != 2 {
		t.Fatalf("answer: %s", w.Body)
	}
	// the shortest time first; a Polygon as a MultiPolygon; 5 decimals, the repeated point gone
	a, b := fc.Features[0], fc.Features[1]
	if a.Properties.Min != 15 || b.Properties.Min != 30 || a.Properties.Mode != "car" || a.Geometry.Type != "MultiPolygon" {
		t.Errorf("features: %+v %+v", a, b)
	}
	if r := a.Geometry.Coordinates[0][0]; len(r) != 4 || r[0][0] != 8.51235 || r[0][1] != 47.36123 || r[3][0] != r[0][0] {
		t.Errorf("ring of 15 min: %v", r)
	}
	// 30 min: the outer ring and its hole kept, the speck of three equal points (after rounding) left out
	if len(b.Geometry.Coordinates) != 1 || len(b.Geometry.Coordinates[0]) != 2 {
		t.Errorf("polygons of 30 min: %v", b.Geometry.Coordinates)
	}
	if last["costing"] != "auto" || last["polygons"] != true || last["date_time"] == nil {
		t.Errorf("ask: %v", last)
	}
	if c := last["contours"].([]any); len(c) != 2 || c[1].(map[string]any)["time"] != 30.0 {
		t.Errorf("contours: %v", last["contours"])
	}
	// on foot: no time of leaving, finer outlines
	if w := get(s, "/labs/maps/api/reach?at=47.3779,8.5403&mode=walk&min=120"); w.Code != 200 {
		t.Fatalf("walk: %d %s", w.Code, w.Body)
	}
	if last["costing"] != "pedestrian" || last["date_time"] != nil || last["generalize"] != 20.0 {
		t.Errorf("walk ask: %v", last)
	}
	// no road near the sea
	if w := get(s, "/labs/maps/api/reach?at=0,0&mode=bike&min=10"); w.Code != 404 || !strings.Contains(w.Body.String(), "no_road") {
		t.Errorf("sea: %d %s", w.Code, w.Body)
	}
	// wrong requests, not sent on
	n := calls.Load()
	for _, q := range []string{"at=47.3,8.5&mode=car&min=90", "at=47.3,8.5&mode=transit&min=10", "at=47.3,8.5&mode=walk&min=30,15",
		"at=47.3,8.5&mode=walk&min=5,10,15,20,25", "at=47.3,8.5&mode=walk&min=0", "at=x&mode=walk&min=10", "at=47.3,8.5&mode=walk"} {
		if w := get(s, "/labs/maps/api/reach?"+q); w.Code != 400 {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
	if calls.Load() != n {
		t.Errorf("wrong requests reached Valhalla")
	}
	// RoutesPerIP (4) areas: three so far, then the fourth, then none; directions still allowed (a window of their own)
	if w := get(s, "/labs/maps/api/reach?at=47.3,8.5&mode=walk&min=10"); w.Code != 200 {
		t.Errorf("fourth: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/reach?at=47.3,8.5&mode=walk&min=10"); w.Code != 429 {
		t.Errorf("fifth: %d", w.Code)
	}
	if !s.routesPerIP.allow("192.0.2.1", time.Now()) {
		t.Errorf("areas counted as directions")
	}
}

func TestReachOnlyWithRouting(t *testing.T) {
	s := testServer(t, nil)
	if w := get(s, "/labs/maps/api/reach?at=47.3,8.5&mode=walk&min=10"); w.Code != 404 {
		t.Errorf("without RoutingURL: %d", w.Code)
	}
}
