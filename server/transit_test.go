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

func TestTransit(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		r.ParseForm()
		if r.URL.Path != "/v1/interpreter" || r.Header.Get("X-API-Key") != "ek_test" || !strings.Contains(r.PostForm.Get("data"), `out geom(`) {
			w.WriteHeader(400)
			return
		}
		io.WriteString(w, `{"elements":[
			{"type":"relation","id":1,"tags":{"route":"tram","ref":"4","name":"Tram 4","colour":"#3B2F8F"},"members":[
				{"type":"node","role":"stop"},
				{"type":"way","role":"","geometry":[{"lat":47.37,"lon":8.54},{"lat":47.38,"lon":8.54},null,{"lat":47.39,"lon":8.55},{"lat":47.395,"lon":8.555}]},
				{"type":"way","role":"platform","geometry":[{"lat":47.37,"lon":8.541},{"lat":47.371,"lon":8.541}]}]},
			{"type":"relation","id":2,"tags":{"route":"bus","ref":"31"},"members":[{"type":"way","role":"","geometry":[{"lat":47.37,"lon":8.54},{"lat":47.38,"lon":8.54}]}]}]}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.ElevationURL = up.URL; c.ElevationKey = "ek_test" })
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-transit="/labs/maps/api/transit"`) {
		t.Fatalf("page has no data-transit")
	}
	w := get(s, "/labs/maps/api/transit?tile=10/536/358")
	var d struct {
		Features []struct {
			Geometry struct {
				Type        string         `json:"type"`
				Coordinates [][][2]float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]string `json:"properties"`
		} `json:"features"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil || len(d.Features) != 1 {
		t.Fatalf("transit: %d %s", w.Code, w.Body)
	}
	f := d.Features[0]
	if f.Properties["ref"] != "4" || f.Properties["colour"] != "#3B2F8F" || f.Properties["kind"] != "tram" || f.Geometry.Type != "MultiLineString" ||
		len(f.Geometry.Coordinates) != 2 || f.Geometry.Coordinates[0][0] != [2]float64{8.54, 47.37} {
		t.Errorf("tram 4: %+v", f)
	}
	get(s, "/labs/maps/api/transit?tile=10/536/358")
	if calls.Load() != 1 {
		t.Errorf("calls to Overpass for one tile twice: %d", calls.Load())
	}
	for _, q := range []string{"tile=9/1/1", "tile=10/2000/1", "tile=x"} {
		if w := get(s, "/labs/maps/api/transit?"+q); w.Code != 400 {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
	if w := get(testServer(t, nil), "/labs/maps/api/transit?tile=10/536/358"); w.Code != 404 {
		t.Errorf("transit without a key: %d", w.Code)
	}
}

// OverpassURL: an Overpass API of its own, asked without a key, also with no ElevationKey
func TestTransitOwnOverpass(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		r.ParseForm()
		if r.URL.Path != "/api/interpreter" || r.Header.Get("X-API-Key") != "" || !strings.Contains(r.PostForm.Get("data"), `out geom(`) {
			w.WriteHeader(400)
			return
		}
		io.WriteString(w, `{"elements":[{"type":"relation","id":1,"tags":{"route":"tram","ref":"4"},"members":[
			{"type":"way","role":"","geometry":[{"lat":47.37,"lon":8.54},{"lat":47.38,"lon":8.54}]}]}]}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.OverpassURL = up.URL + "/api/interpreter"; c.ElevationKey = "" })
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-transit="/labs/maps/api/transit"`) {
		t.Fatalf("page has no data-transit")
	}
	if w := get(s, "/labs/maps/api/transit?tile=10/536/358"); w.Code != 200 || !strings.Contains(w.Body.String(), `"ref":"4"`) || calls.Load() != 1 {
		t.Fatalf("transit: %d %s (%d calls)", w.Code, w.Body, calls.Load())
	}
}

// what is no tile of zoom 10: 400 without a query. Overpass fails or answers nonsense: 502 and nothing kept,
// the next call asks again. The limit of a visitor counts the tiles asked of Overpass, not those in memory.
func TestTransitFailsAndLimit(t *testing.T) {
	var calls, mode atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case 0:
			w.WriteHeader(504)
		case 1:
			io.WriteString(w, `<html>busy</html>`)
		default:
			io.WriteString(w, `{"elements":[{"type":"relation","id":3,"tags":{"route":"tram"},"members":[{"type":"way","role":"","geometry":[{"lat":47.37,"lon":8.54}]}]}]}`)
		}
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.OverpassURL = up.URL })
	s.transitPerIP = newWindow(3, time.Hour)
	for _, tile := range []string{"", "10/536", "9/268/179", "10/1024/0", "10/-1/0", "10/a/b", "10/536/358/1", "99/1/1"} {
		if w := get(s, "/labs/maps/api/transit?tile="+tile); w.Code != 400 {
			t.Errorf("tile %q: %d", tile, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("Overpass asked for a bad tile")
	}
	for m, want := range []int{502, 502, 200} {
		mode.Store(int32(m))
		w := get(s, "/labs/maps/api/transit?tile=10/536/358")
		if w.Code != want {
			t.Errorf("Overpass in mode %d: %d %s", m, w.Code, w.Body)
		}
		if m == 2 && !strings.Contains(w.Body.String(), `"features":[]`) { // a line of one point is none
			t.Errorf("a tile without lines: %s", w.Body)
		}
	}
	if calls.Load() != 3 {
		t.Errorf("calls to Overpass: %d, want 3 (a failure is not kept)", calls.Load())
	}
	if w := get(s, "/labs/maps/api/transit?tile=10/536/359"); w.Code != 429 {
		t.Errorf("a fourth tile from Overpass: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/transit?tile=10/536/358"); w.Code != 200 || calls.Load() != 3 {
		t.Errorf("the tile in memory, over the limit: %d, %d calls", w.Code, calls.Load())
	}
}
