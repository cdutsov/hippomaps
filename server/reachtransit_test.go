package server

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Public transport in Reachable area (reachtransit.go)

const evLat, evLon = 47.39, 8.53

func lonOff(m float64) float64 { return m / (111320 * math.Cos(evLat*math.Pi/180)) }

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: %v, not %v ± %v", what, got, want, tol)
	}
}

// shoelace: twice the signed area of a closed ring of corners
func shoelace(r [][2]int) int {
	a := 0
	for i := 0; i+1 < len(r); i++ {
		a += r[i][0]*r[i+1][1] - r[i+1][0]*r[i][1]
	}
	return a
}

func TestCellPolygons(t *testing.T) {
	grid := func(rows ...string) (int, int, func(x, y int) bool) {
		ny, nx := len(rows), len(rows[0])
		return nx, ny, func(x, y int) bool { // the first row is the top (y = ny-1)
			return x >= 0 && y >= 0 && x < nx && y < ny && rows[ny-1-y][x] == '#'
		}
	}
	// a ring of cells with a hole: one polygon, its outer ring counter-clockwise (area 9 cells), a hole clockwise (1 cell)
	nx, ny, in := grid("###", "#.#", "###")
	ps := cellPolygons(nx, ny, in)
	if len(ps) != 1 || len(ps[0]) != 2 {
		t.Fatalf("a ring: %v", ps)
	}
	if a := shoelace(ps[0][0]); a != 18 || len(ps[0][0]) != 5 {
		t.Errorf("outer ring: %v (twice the area %d)", ps[0][0], a)
	}
	if a := shoelace(ps[0][1]); a != -2 {
		t.Errorf("hole: %v (twice the area %d)", ps[0][1], a)
	}
	for _, r := range ps[0] {
		if r[0] != r[len(r)-1] {
			t.Errorf("ring not closed: %v", r)
		}
	}
	// cells touching at a corner: two polygons
	nx, ny, in = grid("#.", ".#")
	if ps := cellPolygons(nx, ny, in); len(ps) != 2 || len(ps[0]) != 1 || len(ps[1]) != 1 {
		t.Errorf("two cells at a corner: %v", ps)
	}
	// an L and an island in its hole-free bend
	nx, ny, in = grid("#...", "#..#", "###.")
	if ps := cellPolygons(nx, ny, in); len(ps) != 2 {
		t.Errorf("an L and a cell: %v", ps)
	}
	// two holes in one piece, and a piece inside a hole of another
	nx, ny, in = grid("#######", "#.#...#", "###.#.#", "#.....#", "#######")
	ps = cellPolygons(nx, ny, in)
	holes, outers := 0, 0
	for _, p := range ps {
		outers++
		holes += len(p) - 1
	}
	if outers != 2 || holes != 2 {
		t.Errorf("pieces and holes: %d outer rings, %d holes: %v", outers, holes, ps)
	}
	if ps := cellPolygons(3, 3, func(x, y int) bool { return false }); len(ps) != 0 {
		t.Errorf("nothing in: %v", ps)
	}
}

func TestTransitAreas(t *testing.T) {
	lat, lon := 47.39, 8.53
	far := transitStop{lat, lon + 5000/(111320*math.Cos(lat*math.Pi/180)), 10} // 5 km east, reached in 10 minutes
	fc := transitAreas(lat, lon, []transitStop{{lat, lon, 0}, far}, []int{15, 30}, nil)
	b, _ := json.Marshal(fc)
	var v struct {
		Features []struct {
			Properties struct {
				Min  int    `json:"min"`
				Mode string `json:"mode"`
			} `json:"properties"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates [][][][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(b, &v); err != nil || len(v.Features) != 2 {
		t.Fatalf("answer: %s", b)
	}
	f15, f30 := v.Features[0], v.Features[1]
	if f15.Properties.Min != 15 || f15.Properties.Mode != "transit" || f15.Geometry.Type != "MultiPolygon" {
		t.Errorf("15: %+v", f15.Properties)
	}
	// 15 minutes: the walk around the start (900 m) and 5 minutes around the stop (300 m), apart
	if len(f15.Geometry.Coordinates) != 2 {
		t.Errorf("15 minutes: %d polygons", len(f15.Geometry.Coordinates))
	}
	// the area of the start in 15 minutes: a circle of 900 m, 2.5 km² (on a grid of cells)
	area := 0.0
	for _, p := range f15.Geometry.Coordinates {
		r := p[0]
		a := 0.0
		for i := 0; i+1 < len(r); i++ {
			a += (r[i][0]*r[i+1][1] - r[i+1][0]*r[i][1])
		}
		area += math.Abs(a) / 2 * 111320 * math.Cos(lat*math.Pi/180) * 110540
	}
	near(t, "area of 15 minutes, km²", area/1e6, math.Pi*(0.9*0.9+0.3*0.3), 0.35)
	// 30 minutes: more, and at most 15 minutes walked from the stop
	if len(f30.Geometry.Coordinates) == 0 {
		t.Error("30 minutes: nothing")
	}
	for _, p := range f30.Geometry.Coordinates {
		for _, c := range p[0] {
			if d := metresBetween(lat, lon, c[1], c[0]); d > 1800+200 && metresBetween(far.lat, far.lon, c[1], c[0]) > 900+200 {
				t.Fatalf("a point of 30 minutes too far: %v", c)
			}
		}
	}
}

// square: a closed ring of half-width m metres around lat, lon, as [lon, lat]
func square(lat, lon, m float64) [][]float64 {
	dy, dx := m/110540, m/(111320*math.Cos(lat*math.Pi/180))
	return [][]float64{{lon - dx, lat - dy}, {lon + dx, lat - dy}, {lon + dx, lat + dy}, {lon - dx, lat + dy}, {lon - dx, lat - dy}}
}

func TestSpreadTimes(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	var all []time.Time
	for i := 0; i < 20; i++ {
		all = append(all, t0.Add(time.Duration(i)*time.Minute))
	}
	got := spreadTimes(all, 8)
	if len(got) != 8 || !got[0].Equal(all[0]) || !got[7].Equal(all[19]) {
		t.Errorf("spread: %v", got)
	}
	if got := spreadTimes(all[:3], 8); len(got) != 3 {
		t.Errorf("fewer than n: %v", got)
	}
}

func TestTransitWalkArea(t *testing.T) {
	// on foot from the start: the areas of Valhalla (squares of 300 and 600 m, the larger with a hole: a lake), not circles
	lat, lon := 47.39, 8.53
	walk := [][][][][]float64{{{square(lat, lon, 300)}}, {{square(lat, lon, 600), square(lat+0.004, lon, 60)}}}
	fc := transitAreas(lat, lon, []transitStop{{lat, lon, 0}}, []int{15, 30}, walk)
	b, _ := json.Marshal(fc)
	var v struct {
		Features []struct {
			Geometry struct {
				Coordinates [][][][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	json.Unmarshal(b, &v)
	if len(v.Features) != 2 {
		t.Fatalf("answer: %s", b)
	}
	far := func(i int) (m float64) {
		for _, p := range v.Features[i].Geometry.Coordinates {
			for _, c := range p[0] {
				m = math.Max(m, math.Max(math.Abs(c[1]-lat)*110540, math.Abs(c[0]-lon)*111320*math.Cos(lat*math.Pi/180)))
			}
		}
		return m
	}
	// the square of 15 minutes, not the circle of 900 m; the cells of 80 m round it
	if f := far(0); f < 250 || f > 400 {
		t.Errorf("15 minutes reach %v m from the start, not about 300", f)
	}
	if f := far(1); f < 550 || f > 700 {
		t.Errorf("30 minutes reach %v m from the start, not about 600", f)
	}
	holes := 0
	for _, p := range v.Features[1].Geometry.Coordinates {
		holes += len(p) - 1
	}
	if holes == 0 {
		t.Error("30 minutes: the lake is no hole")
	}
}

// fakeTransit answers as Valhalla (isochrones: squares of 20 m a minute; walks: the line x 1.2 at 1.3 m/s) and as MOTIS:
// the stop Bahnhof A (two platforms of P1) 200 m north, Far 1.2 km north; P1 has vehicles 7, 9, 20, 50 and 70 minutes
// after the time asked; each ride reaches a stop 6 km east in 12 minutes
func fakeTransit(t *testing.T, oneToAll *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/isochrone":
			var req struct {
				Costing  string `json:"costing"`
				Contours []struct {
					Time int `json:"time"`
				} `json:"contours"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			var fs []string
			for _, c := range req.Contours {
				ring, _ := json.Marshal(square(evLat, evLon, float64(c.Time)*20))
				fs = append(fs, fmt.Sprintf(`{"type":"Feature","properties":{"contour":%d},"geometry":{"type":"Polygon","coordinates":[%s]}}`, c.Time, ring))
			}
			io.WriteString(w, `{"type":"FeatureCollection","features":[`+strings.Join(fs, ",")+`]}`)
		case "/sources_to_targets":
			var req struct {
				Sources, Targets []map[string]float64
			}
			json.NewDecoder(r.Body).Decode(&req)
			var d, k []string
			for _, p := range req.Targets {
				m := metresBetween(req.Sources[0]["lat"], req.Sources[0]["lon"], p["lat"], p["lon"]) * 1.2
				d, k = append(d, fmt.Sprintf("%f", m/1.3)), append(k, fmt.Sprintf("%f", m/1000))
			}
			io.WriteString(w, `{"sources_to_targets":{"durations":[[`+strings.Join(d, ",")+`]],"distances":[[`+strings.Join(k, ",")+`]]}}`)
		case "/api/v6/map/stops":
			io.WriteString(w, fmt.Sprintf(`[{"stopId":"s1","parentId":"P1","name":"Bahnhof A","lat":%f,"lon":%f},
				{"stopId":"s2","parentId":"P1","name":"Bahnhof A","lat":%f,"lon":%f},
				{"stopId":"s3","name":"Far","lat":%f,"lon":%f}]`, evLat+0.0018, evLon, evLat+0.0019, evLon, evLat+0.0105, evLon))
		case "/api/v6/stoptimes":
			from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("time"))
			if r.URL.Query().Get("stopId") != "P1" {
				io.WriteString(w, `{"stopTimes":[]}`)
				return
			}
			var st []string
			for _, m := range []int{7, 9, 9, 20, 50, 70} {
				st = append(st, fmt.Sprintf(`{"place":{"departure":"%s"}}`, from.Add(time.Duration(m)*time.Minute).Format(time.RFC3339)))
			}
			io.WriteString(w, `{"stopTimes":[`+strings.Join(st, ",")+`]}`)
		case "/api/v1/one-to-all":
			*oneToAll = append(*oneToAll, r.URL.Query().Get("time"))
			t0, _ := time.Parse(time.RFC3339, r.URL.Query().Get("time"))
			io.WriteString(w, fmt.Sprintf(`{"all":[{"place":{"lat":%f,"lon":%f,"arrival":"%s"},"duration":12}]}`, evLat, evLon+lonOff(6000), t0.Add(12*time.Minute).Format(time.RFC3339)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestReachTransit(t *testing.T) {
	var runs []string
	f := fakeTransit(t, &runs)
	s := testServer(t, nil, func(c *Config) { c.RoutingURL = f.URL; c.JourneyURL = f.URL })
	start := time.Now()
	w := get(s, "/labs/maps/api/reach?at=47.39,8.53&mode=transit&min=15,20,30")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/geo+json") {
		t.Fatalf("reach transit: %d %s", w.Code, w.Body)
	}
	var fc struct {
		Features []*reachFeature `json:"features"`
	}
	json.Unmarshal(w.Body.Bytes(), &fc)
	if len(fc.Features) != 3 || fc.Features[0].Properties.Min != 15 || fc.Features[2].Properties.Mode != "transit" {
		t.Fatalf("features: %s", w.Body)
	}
	// leaving now: the first ride asked is the vehicle 7 minutes after one gets to Bahnhof A, a few minutes from now
	if len(runs) == 0 {
		t.Fatal("no one-to-all")
	}
	for _, r := range runs {
		at, _ := time.Parse(time.RFC3339, r)
		if d := at.Sub(start); d < 7*time.Minute || d > 45*time.Minute {
			t.Errorf("a ride %v after now: not leaving now", d)
		}
	}
	east := func(ft *reachFeature) bool {
		for _, p := range ft.Geometry.Coordinates {
			for _, c := range p[0] {
				if c[0] > evLon+lonOff(5500) {
					return true
				}
			}
		}
		return false
	}
	// on foot from the start the area of Valhalla (20 m a minute: 600 m in 30 minutes), not 30 minutes in a line (1.8 km):
	// to the south, where no stop is
	for _, p := range fc.Features[1].Geometry.Coordinates {
		for _, c := range p[0] {
			if d := (evLat - c[1]) * 110540; d > 1200 {
				t.Fatalf("a point of 30 minutes %v m south of the start", d)
			}
		}
	}
	// the walk to Bahnhof A (4 min), a set wait of 3 and the ride of 12: 19 minutes, within 20, not 15
	if !east(fc.Features[1]) || east(fc.Features[0]) || !east(fc.Features[2]) {
		t.Errorf("around the stop reached in 19 minutes: within 20 %v, within 15 %v", east(fc.Features[1]), east(fc.Features[0]))
	}
	if w := get(s, "/labs/maps/api/reach?at=47.39,8.53&mode=transit&min=90"); w.Code != 400 {
		t.Errorf("90 minutes: %d", w.Code)
	}
	// without MOTIS: no mode transit
	s = testServer(t, nil, func(c *Config) { c.RoutingURL = f.URL; c.JourneyURL = "" })
	if w := get(s, "/labs/maps/api/reach?at=47.39,8.53&mode=transit&min=15"); w.Code != 400 {
		t.Errorf("without MOTIS: %d", w.Code)
	}
	// no stop near: 404 no_stops
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `[]`) }))
	defer empty.Close()
	s = testServer(t, nil, func(c *Config) { c.RoutingURL = f.URL; c.JourneyURL = empty.URL })
	if w := get(s, "/labs/maps/api/reach?at=47.39,8.53&mode=transit&min=15"); w.Code != 404 || !strings.Contains(w.Body.String(), "no_stops") {
		t.Errorf("no stops: %d %s", w.Code, w.Body)
	}
}
