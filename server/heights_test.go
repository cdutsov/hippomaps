package server

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

// The height profile of a line drawn on the map (Measure): the elevation API asked with the line as lat, lon; the
// answer as the elev of a route; bad lines, a line too long and the limit of a visitor answered without asking it.
func TestHeights(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.ElevationURL = v.URL; c.ElevationKey = "ek_test"; c.RoutesPerIP = 3 })
	line, _ := json.Marshal(map[string]any{"line": routeLine})
	w := post(s, "/labs/maps/api/heights", string(line))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("heights: %d %s", w.Code, w.Body)
	}
	var e routeElev
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.Up != 140 || e.Down != 9 || e.Min != 405 || e.Max != 541 || len(e.Pts) != 2 || e.Pts[1] != [2]float64{1.0, 540} {
		t.Fatalf("elev: %+v %v", e, err)
	}
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-heights="/labs/maps/api/heights"`) {
		t.Error("the page has no data-heights")
	}
	n := calls.Load()
	for _, bad := range []string{`{}`, `{"line":[[8,47]]}`, `{"line":[[8,47],[200,47]]}`, `{"line":[[8,47],[8,"x"]]}`, `nonsense`,
		`{"line":[[8,47],[8,70]]}`} { // 2500 km
		if w := post(s, "/labs/maps/api/heights", bad); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if calls.Load() != n {
		t.Error("a bad line asked the elevation API")
	}
	post(s, "/labs/maps/api/heights", string(line))
	post(s, "/labs/maps/api/heights", string(line))
	if w := post(s, "/labs/maps/api/heights", string(line)); w.Code != 429 {
		t.Errorf("fourth line of a visitor allowed three: %d", w.Code)
	}
	// without a key of the elevation API there are no heights, and the page does not offer them
	s = testServer(t, nil)
	if w := post(s, "/labs/maps/api/heights", string(line)); w.Code == 200 {
		t.Errorf("no key: %d", w.Code)
	}
	if strings.Contains(get(s, "/labs/maps").Body.String(), "data-heights") {
		t.Error("data-heights without a key")
	}
}

// A failing elevation API: 502, not an empty profile.
func TestHeightsUpstream(t *testing.T) {
	var calls atomic.Int32
	var last map[string]any
	v := fakeValhalla(t, &calls, &last)
	s := testServer(t, nil, func(c *Config) { c.ElevationURL = v.URL; c.ElevationKey = "wrong" })
	line, _ := json.Marshal(map[string]any{"line": routeLine})
	if w := post(s, "/labs/maps/api/heights", string(line)); w.Code != 502 {
		t.Fatalf("upstream failing: %d %s", w.Code, w.Body)
	}
}
