package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(h http.Handler, u, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, u, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// a route with a stop: two legs of Valhalla joined into one line, the stop at the joint
func TestRouteStops(t *testing.T) {
	leg1 := [][2]float64{{8.5403, 47.3779}, {8.4, 47.3}, {8.31, 47.05}}
	leg2 := [][2]float64{{8.31, 47.05}, {7.9, 47.0}, {7.4391, 46.949}}
	var last map[string]any
	v := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/trace_attributes" {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if req["encoded_polyline"] != encodePolyline(append(leg1, leg2[1:]...), 1e6) {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"shape":"`+encodePolyline(append(leg1, leg2[1:]...), 1e6)+`","edges":[{"speed_limit":80,"begin_shape_index":3,"end_shape_index":4}]}`)
			return
		}
		json.NewDecoder(r.Body).Decode(&last)
		step := func(text string, typ, begin, end int) string {
			return `{"instruction":"` + text + `","length":1,"time":60,"type":` + itoa(typ) + `,"begin_shape_index":` + itoa(begin) + `,"end_shape_index":` + itoa(end) + `}`
		}
		io.WriteString(w, `{"trip":{"summary":{"length":160,"time":7000},"legs":[`+
			`{"shape":"`+encodePolyline(leg1, 1e6)+`","maneuvers":[`+step("Drive south.", 1, 0, 2)+`,`+step("Your stop is on the right.", 5, 2, 2)+`]},`+
			`{"shape":"`+encodePolyline(leg2, 1e6)+`","maneuvers":[`+step("Drive west.", 2, 0, 2)+`,`+step("You have arrived.", 4, 2, 2)+`]}]}}`)
	}))
	t.Cleanup(v.Close)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL })
	w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&via=47.05,8.31&to=46.949,7.4391&mode=car")
	if w.Code != 200 {
		t.Fatalf("route: %d %s", w.Code, w.Body)
	}
	var a routeAnswer
	json.Unmarshal(w.Body.Bytes(), &a)
	if len(a.Line) != 5 || len(a.Stops) != 1 || a.Stops[0] != 2 || len(a.Steps) != 4 {
		t.Fatalf("answer: %+v", a)
	}
	if a.Steps[1].Stop != 1 || a.Steps[3].Stop != 0 || a.Steps[2].I != 2 || a.Steps[3].I != 4 || a.Steps[3].At != a.Line[4] {
		t.Errorf("steps: %+v", a.Steps)
	}
	if len(a.Limits) != 1 || a.Limits[0] != [3]int{3, 4, 80} {
		t.Errorf("limits on the joined line: %v", a.Limits)
	}
	locs := last["locations"].([]any)
	if len(locs) != 3 || locs[1].(map[string]any)["type"] != "break" || locs[1].(map[string]any)["lat"] != 47.05 {
		t.Errorf("locations: %v", locs)
	}
	many := "/labs/maps/api/route?from=47.3779,8.5403&to=46.949,7.4391&mode=car" + strings.Repeat("&via=47.05,8.31", maxStops+1)
	if w := get(s, many); w.Code != 400 {
		t.Errorf("too many stops: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&via=x&to=46.949,7.4391&mode=car"); w.Code != 400 {
		t.Errorf("bad stop: %d", w.Code)
	}
	// 40 km in a line, but 300 km by the stop: too far on foot
	if w := get(s, "/labs/maps/api/route?from=47.3779,8.5403&via=46.2,6.14&to=47.05,8.31&mode=walk"); w.Code != 400 || !strings.Contains(w.Body.String(), "too_far") {
		t.Errorf("too far by the stop: %d %s", w.Code, w.Body)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestAlong(t *testing.T) {
	var query string
	o := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		query = r.PostForm.Get("data")
		if r.URL.Path != "/v1/interpreter" || r.Header.Get("X-API-Key") != "ek_test" {
			w.WriteHeader(403)
			return
		}
		io.WriteString(w, `{"elements":[`+
			`{"type":"node","id":2,"lat":47.0,"lon":8.0021,"tags":{"amenity":"fuel","name":"Far on","brand":"Shell","opening_hours":"24/7","note":"x"}},`+
			`{"type":"way","id":1,"center":{"lat":47.0,"lon":8.0005},"tags":{"amenity":"fuel"}},`+
			`{"type":"node","id":3,"lat":47.05,"lon":8.1,"tags":{"amenity":"fuel","name":"Too far off"}}]}`)
	}))
	t.Cleanup(o.Close)
	s := testServer(t, nil, func(c *Config) {
		c.RoutingURL = "http://127.0.0.1:1"
		c.ElevationURL = o.URL
		c.ElevationKey = "ek_test"
	})
	if !strings.Contains(get(s, "/labs/maps").Body.String(), `data-along="/labs/maps/api/along"`) {
		t.Fatalf("page has no data-along")
	}
	// a line east along 47° N, 7.6 km
	w := post(s, "/labs/maps/api/along", `{"line":[[8,47],[8.05,47],[8.1,47]],"cat":"fuel","mode":"car"}`)
	if w.Code != 200 {
		t.Fatalf("along: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(query, `nwr["amenity"="fuel"](around:500,47.00000,8.00000,47.00000,8.10000);`) {
		t.Errorf("query: %s", query)
	}
	var d struct {
		Features []struct {
			Geometry   struct{ Coordinates [2]float64 }
			Properties struct {
				ID, Name, Type, OpeningHours string
				At, Off                      float64
				Tags                         map[string]string
			}
		}
	}
	json.Unmarshal(w.Body.Bytes(), &d)
	if len(d.Features) != 2 {
		t.Fatalf("features: %s", w.Body)
	}
	a, b := d.Features[0].Properties, d.Features[1].Properties
	if a.ID != "way/1" || b.ID != "node/2" || a.Type != "amenity=fuel" || b.Name != "Far on" || b.Tags["brand"] != "Shell" || b.Tags["note"] != "" {
		t.Errorf("places: %+v", d.Features)
	}
	if a.At < 0.03 || a.At > 0.05 || b.At < 0.15 || b.At > 0.17 || a.Off != 0 {
		t.Errorf("at, off: %+v %+v", a, b)
	}
	if !strings.Contains(w.Body.String(), `"opening_hours":"24/7"`) {
		t.Errorf("no opening_hours: %s", w.Body)
	}
	for _, body := range []string{`{"line":[[8,47]],"cat":"fuel","mode":"car"}`, `{"line":[[8,47],[8.1,47]],"cat":"bombs","mode":"car"}`,
		`{"line":[[8,47],[8.1,47]],"cat":"fuel","mode":"plane"}`, `{"line":[[8,97],[8.1,47]],"cat":"fuel","mode":"car"}`, `nope`} {
		if w := post(s, "/labs/maps/api/along", body); w.Code != 400 {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	// without a key: no search along, and the page says nothing of it
	s2 := testServer(t, nil, func(c *Config) { c.RoutingURL = "http://127.0.0.1:1" })
	if w := post(s2, "/labs/maps/api/along", `{}`); w.Code != 404 && w.Code != 405 {
		t.Errorf("along without key: %d", w.Code)
	}
}

func TestSpread(t *testing.T) {
	var ps []alongPlace
	for i := 0; i < 300; i++ { // 300 in the first km of 100, then one every 10 km
		ps = append(ps, alongPlace{Properties: map[string]any{"at": float64(i) / 300, "off": float64(300 - i)}})
	}
	for k := 1; k < 10; k++ {
		ps = append(ps, alongPlace{Properties: map[string]any{"at": float64(k * 10), "off": 400.0}})
	}
	got := spread(ps, 100)
	far := 0
	for _, p := range got {
		if p.Properties["at"].(float64) >= 1 {
			far++
		}
	}
	if len(got) != alongMax || far != 9 {
		t.Errorf("spread: %d kept, %d beyond the first km", len(got), far)
	}
}

func TestSimplify(t *testing.T) {
	line := [][2]float64{{8, 47}, {8.001, 47.00001}, {8.002, 47}, {8.003, 47.01}, {8.004, 47}}
	got := simplify(line, 30)
	if len(got) != 4 || got[1] != line[2] {
		t.Errorf("simplify: %v", got)
	}
}
