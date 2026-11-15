package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// altValhalla: the route leaving now (main), and without a time the same route with three alternates: one that
// mostly follows it, one over a pass closed now (trace_route fails), one far slower, and one other way.
func altValhalla(t *testing.T, asked *[]string, mu *sync.Mutex) *httptest.Server {
	main := [][2]float64{{8.54, 47.37}, {8.3, 47.2}, {8.0, 47.1}, {7.5, 46.97}, {7.44, 46.95}}
	near := [][2]float64{{8.54, 47.37}, {8.3, 47.2}, {8.0, 47.1}, {7.5, 46.97}, {7.47, 46.955}, {7.44, 46.95}} // the last 5 km only differ
	pass := [][2]float64{{8.54, 47.37}, {8.4, 46.6}, {7.44, 46.95}}
	slow := [][2]float64{{8.54, 47.37}, {8.9, 47.6}, {7.44, 46.95}}
	other := [][2]float64{{8.54, 47.37}, {8.1, 47.4}, {7.44, 46.95}}
	trip := func(line [][2]float64, s float64) string {
		return `{"summary":{"length":130,"time":` + jsonNum(s) + `},"legs":[{"shape":` + jsonStr(encodePolyline(line, 1e6)) +
			`,"maneuvers":[{"instruction":"Drive.","length":130,"time":` + jsonNum(s) + `,"type":1,"begin_shape_index":0,"street_names":["A1"]},` +
			`{"instruction":"You have arrived.","type":4,"begin_shape_index":` + jsonNum(float64(len(line)-1)) + `}]}]}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*asked = append(*asked, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/route":
			if req["date_time"] != nil {
				if req["alternates"] != nil {
					w.WriteHeader(400) // Valhalla: no alternates with a time
					return
				}
				io.WriteString(w, `{"trip":`+trip(main, 3600)+`}`)
				return
			}
			io.WriteString(w, `{"trip":`+trip(main, 3590)+`,"alternates":[{"trip":`+trip(near, 3700)+`},{"trip":`+trip(pass, 3000)+`},`+
				`{"trip":`+trip(slow, 4000)+`},{"trip":`+trip(other, 4100)+`}]}`)
		case "/trace_route":
			if req["date_time"] == nil || req["shape_match"] != "edge_walk" || req["language"] != "de-DE" {
				w.WriteHeader(400)
				return
			}
			switch req["encoded_polyline"] {
			case encodePolyline(pass, 1e6):
				w.WriteHeader(400)
				io.WriteString(w, `{"error_code":443,"error":"Exact route match algorithm failed to find path"}`)
			case encodePolyline(slow, 1e6):
				io.WriteString(w, `{"trip":`+trip(slow, 5500)+`}`) // with traffic: over 1.5 times the route
			case encodePolyline(other, 1e6):
				io.WriteString(w, `{"trip":`+trip(other, 4200)+`}`)
			default:
				io.WriteString(w, `{"trip":`+trip(near, 3720)+`}`)
			}
		case "/trace_attributes":
			io.WriteString(w, `{"shape":`+jsonStr(req["encoded_polyline"].(string))+`,"edges":[{"speed_limit":80,"length":5,"speeds_non_faded":{"current_flow":20,"no_flow":80},"speed":20,"begin_shape_index":0,"end_shape_index":1}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func jsonNum(v float64) string { b, _ := json.Marshal(v); return string(b) }

func jsonStr(v string) string { b, _ := json.Marshal(v); return string(b) }

func TestRouteAlternates(t *testing.T) {
	var asked []string
	var mu sync.Mutex
	v := altValhalla(t, &asked, &mu)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL })
	count := func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, p := range asked {
			if p == path {
				n++
			}
		}
		asked = nil
		return n
	}
	w := get(s, "/labs/maps/api/route?from=47.37,8.54&to=46.95,7.44&mode=car&lang=de&alts=2")
	if w.Code != 200 {
		t.Fatalf("route: %d %s", w.Code, w.Body)
	}
	var a routeAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	// the route without a time is the route: gone; near shares most of it: gone; the pass is closed now, slow is
	// too slow with traffic: gone
	if a.S != 3600 || len(a.Alts) != 1 {
		t.Fatalf("alternates: %d, %s", len(a.Alts), w.Body)
	}
	o := a.Alts[0]
	if o.S != 4200 || len(o.Line) != 3 || len(o.Steps) != 2 || o.Steps[0].Street != "A1" || o.Alts != nil {
		t.Errorf("other way: %+v", o)
	}
	// its traffic and limits as the route has them
	if len(o.Limits) != 1 || o.Limits[0] != [3]int{0, 1, 80} || len(o.Traffic) != 1 || o.Delay <= 0 || len(a.Limits) != 1 {
		t.Errorf("extras of the other way: %v %v %v; route %v", o.Limits, o.Traffic, o.Delay, a.Limits)
	}
	if n := count("/trace_route"); n != 3 {
		t.Errorf("trace_route asked %d times, want 3 (pass, slow, other)", n)
	}
	// none without alts, with stops, in navigation (heading)
	for _, u := range []string{
		"/labs/maps/api/route?from=47.37,8.54&to=46.95,7.44&mode=car&lang=de",
		"/labs/maps/api/route?from=47.37,8.54&to=46.95,7.44&via=47.1,8.0&mode=car&lang=de&alts=2",
		"/labs/maps/api/route?from=47.37,8.54&to=46.95,7.44&mode=car&lang=de&alts=2&heading=90",
	} {
		w := get(s, u)
		if w.Code != 200 || strings.Contains(w.Body.String(), `"alts"`) {
			t.Errorf("%s: %d alternates %s", u, w.Code, w.Body)
		}
		if n := count("/route"); n != 1 {
			t.Errorf("%s: /route asked %d times", u, n)
		}
	}
	// alts=1: the one best other route
	w = get(s, "/labs/maps/api/route?from=47.37,8.54&to=46.95,7.44&mode=car&lang=de&alts=1")
	json.Unmarshal(w.Body.Bytes(), &a)
	if len(a.Alts) != 1 {
		t.Errorf("alts=1: %d", len(a.Alts))
	}
}

func TestSharedMost(t *testing.T) {
	a := [][2]float64{{8, 47}, {8.1, 47}, {8.2, 47}, {8.3, 47}}
	if !sharedMost(a, [][][2]float64{a}) {
		t.Error("the same line")
	}
	b := [][2]float64{{8, 47}, {8.1, 47}, {8.15, 47.1}, {8.3, 47}}
	if sharedMost(b, [][][2]float64{a}) {
		t.Error("a line that leaves after a third")
	}
	if sharedMost(b, [][][2]float64{{{9, 46}, {9.1, 46}}}) {
		t.Error("another line")
	}
}
