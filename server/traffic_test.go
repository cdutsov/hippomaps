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

// the traffic service fails from the start: 502 with the error as JSON (it was sent as text/plain), and it is
// asked again only after 10 s
func TestTrafficDown(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, `<html>not the service</html>`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.TrafficURL = up.URL })
	for i := 0; i < 2; i++ {
		w := get(s, "/labs/maps/api/traffic")
		if w.Code != 502 || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), `"code":"failed"`) {
			t.Errorf("traffic without the service: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("calls to the failing service: %d, want 1 in 10 s", calls.Load())
	}
	// an answer that is not JSON is not served
	s.trafficCache.try = s.trafficCache.try.Add(-11e9)
	if w := get(s, "/labs/maps/api/traffic"); w.Code != 502 || calls.Load() != 2 {
		t.Errorf("an answer that is not JSON: %d after %d calls", w.Code, calls.Load())
	}
}
