package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// a route east along 45° N from 9.000 to 9.010 (a point every 79 m): Italy, then Switzerland from index 7
var marksLine = func() [][2]float64 {
	var l [][2]float64
	for i := 0; i <= 10; i++ {
		l = append(l, [2]float64{9 + float64(i)*0.001, 45})
	}
	return l
}()

// fakeMarks is Valhalla (route, trace_attributes) and Overpass for the route of marksLine
func fakeMarks(t *testing.T, query *string) (valhalla, overpass *httptest.Server) {
	shape := encodePolyline(marksLine, 1e6)
	valhalla = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/trace_attributes" {
			// a tertiary road (way 100, a residential street meets it at 4), a residential street against its
			// way (101, another meets it at 7), then a street in Switzerland (102)
			io.WriteString(w, `{"shape":"`+shape+`","admins":[{"country_code":"IT"},{"country_code":"CH"}],"edges":[`+
				`{"way_id":100,"forward":true,"road_class":"tertiary","drive_on_right":true,"begin_shape_index":0,"end_shape_index":4,`+
				`"end_node":{"admin_index":0,"intersecting_edges":[{"driveability":"both","road_class":"residential"},{"road_class":"service_other"}]}},`+
				`{"way_id":101,"forward":false,"road_class":"residential","drive_on_right":true,"begin_shape_index":4,"end_shape_index":7,`+
				`"end_node":{"admin_index":0,"intersecting_edges":[{"driveability":"forward","road_class":"residential"}]}},`+
				`{"way_id":102,"forward":true,"road_class":"residential","drive_on_right":true,"begin_shape_index":7,"end_shape_index":10,`+
				`"end_node":{"admin_index":1}}]}`)
			return
		}
		io.WriteString(w, `{"trip":{"summary":{"length":0.79,"time":60},"legs":[{"shape":"`+shape+`","maneuvers":[`+
			`{"instruction":"Drive east.","length":0.79,"time":60,"type":1,"begin_shape_index":0,"end_shape_index":10},`+
			`{"instruction":"You have arrived.","length":0,"time":0,"type":4,"begin_shape_index":10,"end_shape_index":10}]}]}}`)
	}))
	t.Cleanup(valhalla.Close)
	node := func(id int, lon, lat float64, tags string) string {
		return fmt.Sprintf(`{"type":"node","id":%d,"lat":%.6f,"lon":%.6f,"tags":{%s}},`, id, lat, lon, tags)
	}
	const m = 1 / 111000.0 // a metre north
	overpass = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		*query = r.PostForm.Get("data")
		io.WriteString(w, `{"elements":[`+
			node(1, 9.004, 45, `"highway":"stop"`)+ // on the junction, the route on the more important road: not ours
			node(2, 9.007, 45, `"highway":"stop"`)+ // on the junction of two residential streets: ours (all-way)
			node(3, 9.0065, 45, `"highway":"stop"`)+ // 39 m before the junction at 7: ours
			node(4, 9.0045, 45, `"highway":"stop"`)+ // 39 m after the junction at 4: for the other way
			node(5, 9.0055, 45, `"highway":"stop","direction":"backward"`)+ // against way 101, as the route: ours
			node(6, 9.0015, 45, `"highway":"stop","direction":"forward"`)+ // along way 100, as the route: ours
			node(7, 9.0012, 45, `"highway":"stop","direction":"backward"`)+ // not ours
			node(8, 9.0025, 45, `"traffic_calming":"bump"`)+
			node(9, 9.0026, 45, `"traffic_calming":"cushion"`)+ // 8 m on: the same bump
			node(10, 9.0028, 45+110*m, `"traffic_calming":"hump"`)+ // a node of a way of the route, not on its stretch
			node(20, 9.0031, 45+5*m, `"highway":"speed_camera"`)+ // a node of way 100
			node(21, 9.0035, 45-8*m, `"highway":"speed_camera"`)+ // a node of another road only
			node(22, 9.0045, 45-20*m, `"highway":"speed_camera"`)+ // on its own, 20 m right of the route
			node(23, 9.0060, 45+20*m, `"highway":"speed_camera"`)+ // 20 m left: the other carriageway
			node(24, 9.0062, 45+10*m, `"highway":"speed_camera"`)+ // 10 m left
			node(25, 9.0063, 45+6*m, `"highway":"speed_camera"`)+ // the same gantry, another lane
			node(26, 9.00635, 45+2*m, `"highway":"speed_camera"`)+
			node(27, 9.009, 45, `"highway":"speed_camera"`)+ // in Switzerland
			`{"type":"way","id":100,"nodes":[20,1]},{"type":"way","id":200,"nodes":[21]}]}`)
	}))
	t.Cleanup(overpass.Close)
	return valhalla, overpass
}

func TestRouteMarks(t *testing.T) {
	var query string
	v, o := fakeMarks(t, &query)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL; c.OverpassURL = o.URL })
	w := get(s, "/labs/maps/api/route?from=45,9&to=45,9.01&mode=car")
	if w.Code != 200 {
		t.Fatalf("route: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(query, "way(id:100,101,102)->.r;") || !strings.Contains(query, `[traffic_calming~"^(`+bumpKinds+`)$"]`) ||
		!strings.Contains(query, "node(around:40,45.00000,9.00000,45.00000,9.01000)[highway=speed_camera]") {
		t.Errorf("query: %s", query)
	}
	var a routeAnswer
	json.Unmarshal(w.Body.Bytes(), &a)
	var got []string
	for _, m := range a.Marks {
		got = append(got, fmt.Sprintf("%s %d %.4f", m.Kind, m.I, m.At[0]))
	}
	want := []string{"stop 1 9.0015", "bump 2 9.0025", "camera 3 9.0031", "camera 4 9.0045", "stop 5 9.0055", "camera 6 9.0062", "stop 6 9.0065", "stop 6 9.0070"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("marks:\n got %v\nwant %v", got, want)
	}
	// by bike: none (and no call)
	query = ""
	if w := get(s, "/labs/maps/api/route?from=45,9&to=45,9.01&mode=bike"); strings.Contains(w.Body.String(), `"marks"`) || query != "" {
		t.Errorf("marks by bike: %s", w.Body)
	}
	// Overpass down: the route without marks
	o.Close()
	if w := get(s, "/labs/maps/api/route?from=45,9&to=45,9.01&mode=car"); w.Code != 200 || strings.Contains(w.Body.String(), `"marks"`) {
		t.Errorf("Overpass down: %d %s", w.Code, w.Body)
	}
}

// a line longer than Valhalla matches is asked in pieces; the edges come back in indexes of the whole line
func TestTracePieces(t *testing.T) {
	var line [][2]float64
	for i := 0; i < 8; i++ { // 50 km apart along the equator: 350 km
		line = append(line, [2]float64{float64(i) * 0.4497, 0})
	}
	var calls atomic.Int32
	v := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Poly string `json:"encoded_polyline"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		pts := decodePolyline(req.Poly, 1e6)
		cum := cumKm(pts)
		if cum[len(cum)-1] > traceMaxKm {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"Path distance exceeds the max distance limit"}`)
			return
		}
		n := calls.Add(1)
		io.WriteString(w, fmt.Sprintf(`{"shape":%q,"admins":[{"country_code":"X%d"}],"edges":[{"speed_limit":%d,"way_id":%d,"begin_shape_index":0,"end_shape_index":%d,"end_node":{"admin_index":0}}]}`,
			req.Poly, n, 40+10*n, n, len(pts)-1))
	}))
	t.Cleanup(v.Close)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = v.URL })
	limits, _, _, _, edges := s.alongRoute(t.Context(), line)
	if calls.Load() != 3 || fmt.Sprint(limits) != "[[0 3 50] [3 6 60] [6 7 70]]" {
		t.Errorf("calls %d, limits %v", calls.Load(), limits)
	}
	if len(edges) != 3 || edges[1].a != 3 || edges[1].b != 6 || edges[2].country != "X3" {
		t.Errorf("edges: %+v", edges)
	}
}

func TestStopFor(t *testing.T) {
	js := []junction{{i: 4, km: 1, in: 4, cross: 6}, {i: 9, km: 2, in: 6, cross: 6}, {i: 12, km: 3, in: 6, cross: 2}}
	fw, bw := &tracedEdge{Forward: true}, &tracedEdge{Forward: false}
	for _, c := range []struct {
		tags map[string]string
		km   float64
		e    *tracedEdge
		want bool
	}{
		{map[string]string{"stop": "all"}, 1, fw, true},
		{map[string]string{"direction": "forward"}, 1.5, fw, true},
		{map[string]string{"direction": "forward"}, 1.5, bw, false},
		{map[string]string{"direction": "backward"}, 1.5, bw, true},
		{nil, 1, fw, false},    // the route on the more important road
		{nil, 2, fw, true},     // two residential streets
		{nil, 3.002, fw, true}, // just past the junction where the route comes from a residential street onto a primary road
		{nil, 1.98, fw, true},  // 20 m before a junction
		{nil, 2.02, fw, false}, // 20 m after
		{nil, 2.9, fw, false},  // 100 m before: too far from it
		{nil, 0.97, fw, true},  // before the first
		{nil, 3.03, fw, false}, // after the last
		{nil, 1.5, fw, false},  // half way
		{nil, 0.5, fw, false},  // nowhere near
		{nil, 1.96, nil, true}, // 40 m before (the edge is not needed)
	} {
		if got := stopFor(c.tags, c.km, c.e, js); got != c.want {
			t.Errorf("%v at %v km: %v", c.tags, c.km, got)
		}
	}
	if stopFor(nil, 1, fw, nil) {
		t.Errorf("no junctions: a stop")
	}
}

func TestJunctions(t *testing.T) {
	var e1, e2, e3, e4 tracedEdge
	e1.b, e1.Class = 2, "secondary"
	e1.EndNode.Cross = []struct {
		Drive string `json:"driveability"`
		Class string `json:"road_class"`
	}{{"", "residential"}, {"both", "service_other"}} // a footway and a driveway: no junction on a secondary road
	e2.b, e2.Class = 4, "service_other"
	e2.EndNode.Cross = e1.EndNode.Cross // on a service road: the driveway counts
	e3.b, e3.Class = 6, "residential"
	e3.EndNode.Cross = append(e1.EndNode.Cross[:1:1], struct {
		Drive string `json:"driveability"`
		Class string `json:"road_class"`
	}{"backward", "primary"})
	e4.b, e4.Class = 6, "residential" // an edge of no length: the same junction
	e4.EndNode.Cross = e3.EndNode.Cross[1:]
	cum := []float64{0, 1, 2, 3, 4, 5, 6}
	js := junctions([]tracedEdge{e1, e2, e3, e4}, cum)
	if fmt.Sprint(js) != "[{4 4 7 7} {6 6 6 2}]" {
		t.Errorf("junctions: %v", js)
	}
}
