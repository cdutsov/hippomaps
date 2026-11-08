package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTraffic(t *testing.T) {
	var calls atomic.Int32
	var down atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if down.Load() || r.URL.Path != "/incidents.json" {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, `{"items":[{"id":"situation.1","kind":"closure","closed":true}],"flow":[]}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.TrafficURL = up.URL + "/" })
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-traffic="/labs/maps/api/traffic"`) {
		t.Fatalf("page has no data-traffic")
	}
	for i := 0; i < 3; i++ {
		w := get(s, "/labs/maps/api/traffic")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"closure"`) {
			t.Fatalf("traffic: %d %s", w.Code, w.Body)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("calls to the service: %d, want 1 (kept a minute)", calls.Load())
	}
	// the service fails: the last answer stays
	down.Store(true)
	s.trafficCache.at = s.trafficCache.at.Add(-2 * 60e9)
	s.trafficCache.try = s.trafficCache.try.Add(-60e9)
	if w := get(s, "/labs/maps/api/traffic"); w.Code != 200 {
		t.Errorf("stale answer: %d", w.Code)
	}
}

func TestTrafficOff(t *testing.T) {
	s := testServer(t, nil)
	if w := get(s, "/labs/maps/api/traffic"); w.Code != 404 {
		t.Errorf("traffic without TrafficURL: %d", w.Code)
	}
}
