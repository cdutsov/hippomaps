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
			if req["encoded_polyline"] != encodePolyline(routeLine, 1e6) || req["shape_match"] != "walk_or_snap" || req["date_time"] == nil {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"shape":"`+encodePolyline(routeLine, 1e6)+`","edges":[{"speed_limit":50,"length":10,"speeds_non_faded":{"current_flow":20,"no_flow":50},"speed":20,"begin_shape_index":0,"end_shape_index":1},`+
				`{"speed_limit":"unlimited","begin_shape_index":1,"end_shape_index":1},`+
				`{"speed_limit":120,"length":60,"speeds_non_faded":{"current_flow":110,"no_flow":120},"speed":110,"tunnel":true,"begin_shape_index":1,"end_shape_index":2}]}`)
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
		if lat := locs[0].(map[string]any)["lat"].(float64); lat == 1 || lat == 2 { // 1: a pass closed now, a route without a time; 2: no route at all
			if lat == 2 || req["date_time"] != nil {
				w.WriteHeader(400)
				io.WriteString(w, `{"error_code":442,"error":"No path could be found for input"}`)
				return
			}
		}
		if locs[0].(map[string]any)["lat"].(float64) == 0 { // the sea
			w.WriteHeader(400)
			io.WriteString(w, `{"error_code":171,"error":"No suitable edges near location"}`)
			return
		}
		io.WriteString(w, `{"trip":{"summary":{"length":124.321,"time":5400.4},"legs":[{"shape":"`+encodePolyline(routeLine, 1e6)+
			`","maneuvers":[{"instruction":"Drive west.","length":60.1,"time":2500,"type":1,"begin_shape_index":0,"end_shape_index":1,`+
			`"verbal_pre_transition_instruction":"Drive west on A1.","street_names":["A1"],"highway":true,`+
			`"lanes":[{"directions":2},{"directions":34,"active":32},{"directions":32,"valid":32}],`+
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
	if len(st.Lanes) != 3 || st.Lanes[1] != (lane{34, 32, 0}) || st.Lanes[2] != (lane{32, 0, 32}) || a.Steps[1].Lanes != nil {
		t.Errorf("lanes: %+v / %+v", st.Lanes, a.Steps[1].Lanes)
	}
	if last["turn_lanes"] != true {
		t.Errorf("no turn_lanes asked")
	}
	if len(a.Limits) != 2 || a.Limits[0] != [3]int{0, 1, 50} || a.Limits[1] != [3]int{1, 2, 120} {
		t.Errorf("limits: %v", a.Limits)
	}
	// 10 km at 20 instead of 50 km/h: a queue, 1080 s; 60 km at 110 instead of 120: 164 s more, no colour
	if len(a.Traffic) != 1 || a.Traffic[0] != [3]int{0, 1, 2} || a.Delay != 1080+164 {
		t.Errorf("traffic: %v, delay %v", a.Traffic, a.Delay)
	}
	if len(a.Tunnels) != 1 || a.Tunnels[0] != [3]int{1, 2, 60000} {
		t.Errorf("tunnels: %v", a.Tunnels)
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
	if loc := last["locations"].([]any)[0].(map[string]any); loc["heading"] != nil {
		t.Errorf("heading without one asked: %v", loc)
	}
	get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=car&heading=271.6")
	if loc := last["locations"].([]any)[0].(map[string]any); loc["heading"] != 272.0 || loc["heading_tolerance"] != 45.0 {
		t.Errorf("heading: %v", loc)
	}
	get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=car&heading=400")
	if loc := last["locations"].([]any)[0].(map[string]any); loc["heading"] != nil {
		t.Errorf("heading out of range: %v", loc)
	}
	get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=walk&lang=xx")
	if last["costing"] != "pedestrian" || last["language"] != "en-US" {
		t.Errorf("walk: %v", last)
	}
	// the steps in the language of navigation: Bulgarian; Norwegian as no and as nb
	for lang, want := range map[string]string{"bg": "bg-BG", "no": "nb-NO", "nb": "nb-NO", "uk": "uk-UA"} {
		get(s, "/labs/maps/api/route?from=47.3779,8.5403&to=47.38,8.55&mode=car&lang="+lang)
		if last["language"] != want {
			t.Errorf("lang %s: %v", lang, last["language"])
		}
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
	if w := get(s, "/labs/maps/api/route?from=1,8&to=1.1,8.1&mode=car"); w.Code != 404 || !strings.Contains(w.Body.String(), `"closed_now"`) {
		t.Errorf("a road closed now (a pass in winter): %d %s", w.Code, w.Body)
	}
	if w := get(s, "/labs/maps/api/route?from=2,8&to=2.1,8.1&mode=car"); w.Code != 404 || !strings.Contains(w.Body.String(), `"no_route"`) {
		t.Errorf("no route at any time: %d %s", w.Code, w.Body)
	}
	if w := get(s, "/labs/maps/api/route?from=0,0&to=0.1,0.1&mode=car"); w.Code != 404 || !strings.Contains(w.Body.String(), `"no_road"`) || !strings.Contains(w.Body.String(), "No road near") {
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

// "Continue." says how far: the sentence after the turn is said at it; one with more ("Then…") stays
func TestContinueDistance(t *testing.T) {
	var a routeAnswer
	a.Line = routeLine
	a.addSteps([]maneuver{
		{Instruction: "Weiter.", Type: 8, Alert: "Weiter.", Pre: "Weiter.", Post: "600 Meter weiter der Route folgen.", Begin: 0},
		{Instruction: "Continue.", Type: 8, Pre: "Continue. Then, in 30 meters, Turn left.", Post: "Continue for 30 meters.", Begin: 1},
		{Instruction: "Turn left.", Type: 15, Pre: "Turn left.", Post: "Continue for 1 kilometer.", Begin: 2},
	}, 0, false, 0)
	if s := a.Steps[0]; s.Pre != "600 Meter weiter der Route folgen." || s.Post != "" || s.Alert != "" || s.Text != "Weiter." {
		t.Errorf("continue: %+v", s)
	}
	if s := a.Steps[1]; s.Pre != "Continue. Then, in 30 meters, Turn left." || s.Post != "Continue for 30 meters." {
		t.Errorf("continue then: %+v", s)
	}
	if s := a.Steps[2]; s.Pre != "Turn left." || s.Post != "Continue for 1 kilometer." {
		t.Errorf("turn: %+v", s)
	}
}

// The phrases of Valhalla mended: a name glued to the word before it gets its space (Bulgarian, Hungarian,
// Swedish), not inside a word or after a capital; the Bulgarian lengths without a number get the length of the step.
func TestPhrasesMended(t *testing.T) {
	names := []string{"Aarestrasse", "6", "Zürich", "Bern"}
	for in, want := range map[string]string{
		"Влезте по Guisanplatz и вземете 3-тия изход поAarestrasse/6.": "Влезте по Guisanplatz и вземете 3-тия изход по Aarestrasse/6.",
		"Forduljon jobbra a(z)Aarestrasse utcára.":                     "Forduljon jobbra a(z) Aarestrasse utcára.",
		"Håll höger motZürich.":                                        "Håll höger mot Zürich.",
		"Rechts auf Aarestrasse/6 abbiegen.":                           "Rechts auf Aarestrasse/6 abbiegen.",
		"Oberbern, A6, Bern.":                                          "Oberbern, A6, Bern.",
	} {
		if got := unglue(in, names); got != want {
			t.Errorf("unglue %q: %q", in, got)
		}
	}
	for _, c := range []struct {
		in   string
		km   float64
		want string
	}{
		{"Продължете метра.", 0.08, "Продължете 80 метра."},
		{"Продължете метра.", 0.22, "Продължете 200 метра."},
		{"Продължете по Aarestrasse за километри.", 2.46, "Продължете по Aarestrasse за 2,5 километра."},
		{"Продължете метра.", 0.97, "Продължете 1 километър."},
		{"Продължете метра.", 0.005, "Продължете по-малко от 10 метра."},
		{"Продължете 80 метра.", 0.08, "Продължете 80 метра."},
	} {
		if got := bgLengths(c.in, c.km); got != c.want {
			t.Errorf("bgLengths %q %v: %q", c.in, c.km, got)
		}
	}
	a := routeAnswer{Line: routeLine, lang: "bg-BG"}
	a.addSteps([]maneuver{
		{Instruction: "Тръгнете на север.", Type: 1, Post: "Продължете метра.", Length: 0.31, Begin: 0},
		{Instruction: "Влезте и вземете 3-тия изход поAarestrasse.", Type: 26, Pre: "Влезте и вземете 3-тия изход поAarestrasse.", Begin: 1},
		{Instruction: "Излезте по Aarestrasse.", Type: 27, Streets: []string{"Aarestrasse"}, Begin: 2},
	}, 0, false, 0)
	if a.Steps[0].Post != "Продължете 300 метра." || a.Steps[1].Text != "Влезте и вземете 3-тия изход по Aarestrasse." || a.Steps[1].Pre != a.Steps[1].Text {
		t.Errorf("bg steps: %+v", a.Steps[:2])
	}
}
