package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

// How far one gets by public transport (GET {base}/api/reach?mode=transit, as the other modes of reach.go): the stops
// around the start and the walk to each (MOTIS, Valhalla), then from each of the nearest stops of other names the stops
// MOTIS reaches within the time (one-to-all). One times the walk to the stop so as not to wait long: the search starts
// at departures of the stop (up to transitRuns of those within transitSpan after one gets there, leaving home now, with
// the delays MOTIS knows), each stop reached keeps its fastest ride, and the wait for the first vehicle is
// transitFirstWait for all. From each stop reached the rest of the time is walked in a straight line (transitPace, at
// most transitWalkMax); from the start the areas on foot of Valhalla. The areas come from a grid of cells (transitCells
// across at most) as the outlines of the cells reached within each time, MultiPolygons as Valhalla answers, the shortest
// time first.

const (
	transitPace     = 60.0 // metres a minute in a straight line on foot (4.5 km/h with the detours of streets)
	transitWalkMax  = 15   // minutes walked at most from a stop after a ride
	transitOrigins  = 8    // stops of other names asked as starts
	transitCells    = 500  // cells across the grid at most
	transitCellMinM = 80.0 // the smallest cell, metres
	// the first vehicle: a wait of transitFirstWait minutes, whatever the timetable (one times the walk to the stop);
	// the fastest ride of up to transitRuns departures of the stop in transitSpan
	transitFirstWait = 3
	transitRuns      = 8
	transitSpan      = 30 * time.Minute
)

// transitStop is a stop reached and the minutes it takes from the start
type transitStop struct {
	lat, lon float64
	min      float64
}

// transitOriginStops: the stops around the start with the minutes of the walk to each, one per name (the nearest of
// that name), the nearest first, at most transitOrigins, each within limit minutes
func (s *Server) transitOriginStops(ctx context.Context, lat, lon float64, limit float64) ([]motisNamed, error) {
	stops, err := s.stopsNamed(ctx, lat, lon, journeyNearM)
	if err != nil {
		return nil, err
	}
	at := map[spot]bool{}
	var spots []spot
	for _, st := range stops {
		p := spotOf(st.Lat, st.Lon)
		if !at[p] {
			at[p] = true
			spots = append(spots, p)
		}
	}
	walks := s.walkTimes(ctx, lat, lon, spots, false)
	best := map[string]motisNamed{}
	for _, st := range stops {
		f, ok := walks[spotOf(st.Lat, st.Lon)]
		if !ok { // no answer of Valhalla: the straight line with detours
			m := metresBetween(lat, lon, st.Lat, st.Lon) * 1.3
			f = foot{m / journeyPace, m}
		}
		st.walk = f.s / 60
		if st.walk > limit {
			continue
		}
		key := st.Name
		if key == "" {
			key = st.StopID
		}
		if b, ok := best[key]; !ok || st.walk < b.walk {
			best[key] = st
		}
	}
	out := make([]motisNamed, 0, len(best))
	for _, st := range best {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].walk != out[j].walk {
			return out[i].walk < out[j].walk
		}
		return out[i].StopID < out[j].StopID
	})
	if len(out) > transitOrigins {
		out = out[:transitOrigins]
	}
	return out, nil
}

// motisNamed is a stop of MOTIS with its name, and the walk to it (minutes)
type motisNamed struct {
	StopID   string  `json:"stopId"`
	ParentID string  `json:"parentId"`
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	walk     float64
}

// stopsNamed: the stops with departures within metres in a line of a point, with their names
func (s *Server) stopsNamed(ctx context.Context, lat, lon, metres float64) ([]motisNamed, error) {
	dLat := metres / 111320
	dLon := dLat / math.Max(0.1, math.Cos(lat*math.Pi/180))
	u := fmt.Sprintf("%s/api/v6/map/stops?min=%.6f,%.6f&max=%.6f,%.6f", s.cfg.JourneyURL, lat-dLat, lon-dLon, lat+dLat, lon+dLon)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stops: HTTP %d", resp.StatusCode)
	}
	var all, out []motisNamed
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&all); err != nil {
		return nil, err
	}
	for _, st := range all {
		if st.StopID != "" && metresBetween(lat, lon, st.Lat, st.Lon) <= metres {
			out = append(out, st)
		}
	}
	return out, nil
}

// oneToAll: the stops MOTIS reaches from a stop leaving at t within max minutes, with the minutes from t
func (s *Server) oneToAll(ctx context.Context, stopID string, t time.Time, max int) ([]transitStop, error) {
	q := url.Values{"one": {stopID}, "maxTravelTime": {fmt.Sprint(max)}, "time": {t.UTC().Format(time.RFC3339)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.JourneyURL+"/api/v1/one-to-all?"+q.Encode(), nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("one-to-all: HTTP %d", resp.StatusCode)
	}
	var v struct {
		All []struct {
			Place struct {
				Lat     float64 `json:"lat"`
				Lon     float64 `json:"lon"`
				Arrival string  `json:"arrival"`
			} `json:"place"`
			Duration float64 `json:"duration"`
		} `json:"all"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&v); err != nil {
		return nil, err
	}
	out := make([]transitStop, 0, len(v.All))
	for _, a := range v.All {
		m := a.Duration
		if at, err := time.Parse(time.RFC3339, a.Place.Arrival); err == nil {
			m = at.Sub(t).Minutes() // the time of arrival, waits for the first ride included
		}
		if m >= 0 && m <= float64(max) {
			out = append(out, transitStop{a.Place.Lat, a.Place.Lon, m})
		}
	}
	return out, nil
}

// departureTimes: the times vehicles leave a stop from from until to, at most n spread over them (the first and the last
// among them)
func (s *Server) departureTimes(ctx context.Context, stopID string, from, to time.Time, n int) ([]time.Time, error) {
	q := url.Values{"stopId": {stopID}, "time": {from.UTC().Format(time.RFC3339)}, "n": {"200"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.JourneyURL+"/api/v6/stoptimes?"+q.Encode(), nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stoptimes: HTTP %d", resp.StatusCode)
	}
	var v struct {
		StopTimes []struct {
			Place struct {
				Departure string `json:"departure"`
			} `json:"place"`
			Cancelled bool `json:"cancelled"`
		} `json:"stopTimes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v); err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var all []time.Time
	for _, st := range v.StopTimes {
		t, err := time.Parse(time.RFC3339, st.Place.Departure)
		if err != nil || st.Cancelled || t.Before(from) || !t.Before(to) || seen[t.Unix()] {
			continue
		}
		seen[t.Unix()] = true
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Before(all[j]) })
	return spreadTimes(all, n), nil
}

// spreadTimes: at most n of the times (sorted), the first and the last among them, the others evenly between
func spreadTimes(all []time.Time, n int) []time.Time {
	if len(all) <= n {
		return all
	}
	out := make([]time.Time, n)
	for i := range out {
		out[i] = all[i*(len(all)-1)/(n-1)]
	}
	return out
}

// reachTransit answers /api/reach?mode=transit for the minutes mins (rising), leaving at leave
func (s *Server) reachTransit(ctx context.Context, lat, lon float64, mins []int, leave time.Time) ([]byte, int, string) {
	fc, status, code := s.transitFC(ctx, lat, lon, mins, transitRuns, leave)
	if status != http.StatusOK {
		return nil, status, code
	}
	b, _ := json.Marshal(fc)
	return b, http.StatusOK, ""
}

// transitFC: the areas reached by public transport within mins leaving at leave, a FeatureCollection as transitAreas
// makes it
func (s *Server) transitFC(ctx context.Context, lat, lon float64, mins []int, runs int, leave time.Time) (map[string]any, int, string) {
	longest := mins[len(mins)-1]
	origins, err := s.transitOriginStops(ctx, lat, lon, math.Min(float64(longest), journeyNearS/60))
	if err != nil {
		s.log.Warn("reach transit: stops", "err", err)
		return nil, http.StatusBadGateway, "failed"
	}
	reached := []transitStop{{lat, lon, 0}}
	if len(origins) > 0 {
		var mu sync.Mutex
		var wg sync.WaitGroup
		gate := make(chan struct{}, 4)
		best := map[spot]transitStop{} // the fastest to each stop reached
		keep := func(st transitStop) {
			k := spot{st.lat, st.lon}
			if b, ok := best[k]; !ok || st.min < b.min {
				best[k] = st
			}
		}
		for _, o := range origins {
			walk := math.Ceil(o.walk)
			rest := longest - int(walk) - transitFirstWait
			if rest < 1 {
				continue
			}
			mu.Lock()
			keep(transitStop{o.Lat, o.Lon, walk})
			mu.Unlock()
			id := o.StopID
			if o.ParentID != "" {
				id = o.ParentID // all the platforms of the stop
			}
			at := leave.Add(time.Duration(walk) * time.Minute)
			deps, err := s.departureTimes(ctx, id, at, at.Add(transitSpan), runs)
			if err != nil {
				s.log.Warn("reach transit: departures", "stop", id, "err", err)
				continue
			}
			for _, run := range deps {
				wg.Add(1)
				go func(run time.Time) {
					defer wg.Done()
					gate <- struct{}{}
					defer func() { <-gate }()
					got, err := s.oneToAll(ctx, o.StopID, run, rest)
					if err != nil {
						s.log.Warn("reach transit: one-to-all", "stop", o.StopID, "err", err)
						return
					}
					mu.Lock()
					defer mu.Unlock()
					for _, g := range got { // the ride from the departure at run, the walk and the set wait before it
						g.min += walk + transitFirstWait
						keep(g)
					}
				}(run)
			}
		}
		wg.Wait()
		if ctx.Err() != nil {
			return nil, http.StatusBadGateway, "failed"
		}
		for _, st := range best {
			reached = append(reached, st)
		}
	}
	if len(origins) == 0 {
		return nil, http.StatusNotFound, "no_stops"
	}
	// on foot from the start: the area Valhalla gives for each time (paths, slopes, rivers), else the straight line
	walk := s.walkArea(ctx, lat, lon, mins)
	return transitAreas(lat, lon, reached, mins, walk), http.StatusOK, ""
}

// walkArea: the areas reached on foot from a point within each of mins (isochrones of Valhalla, MultiPolygons as [lon,
// lat]), in the order of mins; nil when Valhalla gives none
func (s *Server) walkArea(ctx context.Context, lat, lon float64, mins []int) [][][][][]float64 {
	return s.isochrone(ctx, lat, lon, "pedestrian", mins, nil)
}

// isochrone: the areas of Valhalla for a costing within each of mins, in their order (dateTime: date_time of Valhalla, nil
// none); nil when it gives none
func (s *Server) isochrone(ctx context.Context, lat, lon float64, costing string, mins []int, dateTime map[string]any) [][][][][]float64 {
	contours := make([]map[string]int, len(mins))
	for i, m := range mins {
		contours[i] = map[string]int{"time": m}
	}
	ask := map[string]any{"locations": []map[string]float64{{"lat": lat, "lon": lon}}, "costing": costing,
		"contours": contours, "polygons": true, "denoise": 0.3, "generalize": 20}
	if dateTime != nil {
		ask["date_time"] = dateTime
	}
	body, _ := json.Marshal(ask)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/isochrone", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		s.log.Warn("isochrone", "costing", costing, "err", err)
		return nil
	}
	defer resp.Body.Close()
	var v struct {
		Features []struct {
			Properties struct {
				Contour float64 `json:"contour"`
			} `json:"properties"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&v) != nil {
		s.log.Warn("isochrone", "costing", costing, "status", resp.StatusCode)
		return nil
	}
	out := make([][][][][]float64, len(mins))
	found := false
	for _, f := range v.Features {
		var polys [][][][]float64
		switch f.Geometry.Type {
		case "Polygon":
			var p [][][]float64
			if json.Unmarshal(f.Geometry.Coordinates, &p) == nil {
				polys = [][][][]float64{p}
			}
		case "MultiPolygon":
			json.Unmarshal(f.Geometry.Coordinates, &polys)
		}
		for i, m := range mins {
			if int(math.Round(f.Properties.Contour)) == m && len(polys) > 0 {
				out[i], found = polys, true
			}
		}
	}
	if !found {
		return nil
	}
	return out
}

// transitAreas: the areas reached within each time: from the start on foot for the whole time (walk: the areas of
// walkArea, one per time; nil: a straight line), from a stop for the rest of it, up to transitWalkMax in a straight line
func transitAreas(lat0, lon0 float64, reached []transitStop, mins []int, walk [][][][][]float64) map[string]any {
	longest := float64(mins[len(mins)-1])
	kx := math.Cos(lat0 * math.Pi / 180)
	toXY := func(lat, lon float64) (float64, float64) { return (lon - lon0) * 111320 * kx, (lat - lat0) * 110540 }
	// the box of all circles
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	radius := func(i int, st transitStop) float64 {
		if i == 0 && walk != nil { // the start: the areas on foot instead
			return 0
		}
		r := (longest - st.min) * transitPace
		if i > 0 {
			r = math.Min(r, transitWalkMax*transitPace)
		}
		return r
	}
	for i, st := range reached {
		x, y := toXY(st.lat, st.lon)
		r := radius(i, st)
		minX, minY, maxX, maxY = math.Min(minX, x-r), math.Min(minY, y-r), math.Max(maxX, x+r), math.Max(maxY, y+r)
	}
	for _, mp := range walk {
		for _, p := range mp {
			for _, c := range p[0] {
				if len(c) < 2 {
					continue
				}
				x, y := toXY(c[1], c[0])
				minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
			}
		}
	}
	cell := math.Max(transitCellMinM, math.Max(maxX-minX, maxY-minY)/transitCells)
	nx, ny := int(math.Ceil((maxX-minX)/cell))+1, int(math.Ceil((maxY-minY)/cell))+1
	grid := make([]float32, nx*ny) // minutes to the middle of each cell
	for i := range grid {
		grid[i] = math.MaxFloat32
	}
	for i, st := range reached {
		x, y := toXY(st.lat, st.lon)
		r := radius(i, st)
		if r <= 0 {
			continue
		}
		i0, i1 := int((x-r-minX)/cell), int((x+r-minX)/cell)
		j0, j1 := int((y-r-minY)/cell), int((y+r-minY)/cell)
		for j := max(j0, 0); j <= min(j1, ny-1); j++ {
			cy := minY + (float64(j)+0.5)*cell
			for k := max(i0, 0); k <= min(i1, nx-1); k++ {
				cx := minX + (float64(k)+0.5)*cell
				d := math.Hypot(cx-x, cy-y)
				if d > r {
					continue
				}
				if m := float32(st.min + d/transitPace); m < grid[j*nx+k] {
					grid[j*nx+k] = m
				}
			}
		}
	}
	// the areas on foot: each cell whose middle is in the area of a time takes that time (the shortest such)
	for i := len(walk) - 1; i >= 0; i-- {
		for _, p := range walk[i] {
			rings := make([][][2]float64, 0, len(p)) // in metres from the start
			bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			for k, r := range p {
				ring := make([][2]float64, 0, len(r))
				for _, c := range r {
					if len(c) < 2 {
						continue
					}
					x, y := toXY(c[1], c[0])
					ring = append(ring, [2]float64{x, y})
					if k == 0 {
						bx0, by0, bx1, by1 = math.Min(bx0, x), math.Min(by0, y), math.Max(bx1, x), math.Max(by1, y)
					}
				}
				rings = append(rings, ring)
			}
			for j := max(int((by0-minY)/cell), 0); j <= min(int((by1-minY)/cell), ny-1); j++ {
				cy := minY + (float64(j)+0.5)*cell
				for k := max(int((bx0-minX)/cell), 0); k <= min(int((bx1-minX)/cell), nx-1); k++ {
					cx := minX + (float64(k)+0.5)*cell
					in := false
					for _, r := range rings { // even-odd over the outer ring and its holes
						if inRingXY(cx, cy, r) {
							in = !in
						}
					}
					if in && float32(mins[i]) < grid[j*nx+k] {
						grid[j*nx+k] = float32(mins[i])
					}
				}
			}
		}
	}
	toLL := func(gx, gy int) []float64 {
		x, y := minX+float64(gx)*cell, minY+float64(gy)*cell
		return []float64{round(lon0+x/(111320*kx), 5), round(lat0+y/110540, 5)}
	}
	features := make([]map[string]any, 0, len(mins))
	for _, m := range mins {
		in := func(gx, gy int) bool {
			return gx >= 0 && gy >= 0 && gx < nx && gy < ny && grid[gy*nx+gx] <= float32(m)
		}
		polys := cellPolygons(nx, ny, in)
		coords := make([][][][]float64, 0, len(polys))
		for _, p := range polys {
			poly := make([][][]float64, 0, len(p))
			for k, ring := range p {
				line := make([][2]float64, 0, len(ring))
				for _, v := range ring {
					ll := toLL(v[0], v[1])
					line = append(line, [2]float64{ll[0], ll[1]})
				}
				// the steps of the cells cut into slopes (Douglas-Peucker); a ring too small for that stays as it is
				if sm := simplify(line, cell*0.7); len(sm) >= 4 {
					line = sm
				}
				if len(line) < 4 {
					if k == 0 {
						break
					}
					continue
				}
				r := make([][]float64, len(line))
				for i, c := range line {
					r[i] = []float64{c[0], c[1]}
				}
				poly = append(poly, r)
			}
			if len(poly) > 0 {
				coords = append(coords, poly)
			}
		}
		features = append(features, map[string]any{"type": "Feature", "properties": map[string]any{"min": m, "mode": "transit"},
			"geometry": map[string]any{"type": "MultiPolygon", "coordinates": coords}})
	}
	return map[string]any{"type": "FeatureCollection", "features": features}
}

// cellPolygons: the outlines of the cells of an nx by ny grid for which in is true, as polygons (an outer ring counter-
// clockwise, then its holes clockwise), each ring closed, in corners of cells (x, y: the corner below left of cell x, y).
// Cells that touch at a corner only are apart.
func cellPolygons(nx, ny int, in func(x, y int) bool) [][][][2]int {
	type edge struct {
		from, to [2]int
		cx, cy   int // the cell on the left
	}
	// the edges between a cell in and one out, the cell in on the left
	out := map[[2]int][]int{}
	var edges []edge
	add := func(e edge) {
		out[e.from] = append(out[e.from], len(edges))
		edges = append(edges, e)
	}
	for y := 0; y < ny; y++ {
		for x := 0; x < nx; x++ {
			if !in(x, y) {
				continue
			}
			if !in(x, y-1) {
				add(edge{[2]int{x, y}, [2]int{x + 1, y}, x, y})
			}
			if !in(x+1, y) {
				add(edge{[2]int{x + 1, y}, [2]int{x + 1, y + 1}, x, y})
			}
			if !in(x, y+1) {
				add(edge{[2]int{x + 1, y + 1}, [2]int{x, y + 1}, x, y})
			}
			if !in(x-1, y) {
				add(edge{[2]int{x, y + 1}, [2]int{x, y}, x, y})
			}
		}
	}
	// the cells joined by sides: one label per piece
	label := map[[2]int]int{}
	n := 0
	for _, e := range edges {
		c := [2]int{e.cx, e.cy}
		if _, ok := label[c]; ok {
			continue
		}
		n++
		stack := [][2]int{c}
		label[c] = n
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				q := [2]int{p[0] + d[0], p[1] + d[1]}
				if _, ok := label[q]; !ok && in(q[0], q[1]) {
					label[q] = n
					stack = append(stack, q)
				}
			}
		}
	}
	used := make([]bool, len(edges))
	type ring struct {
		pts   [][2]int
		piece int
		area  int // twice the signed area: > 0 counter-clockwise
		probe [2]float64
	}
	var rings []ring
	for i := range edges {
		if used[i] {
			continue
		}
		piece := label[[2]int{edges[i].cx, edges[i].cy}]
		var pts [][2]int
		e := i
		for !used[e] {
			used[e] = true
			pts = append(pts, edges[e].from)
			next := -1
			for _, c := range out[edges[e].to] {
				if !used[c] && label[[2]int{edges[c].cx, edges[c].cy}] == piece {
					next = c
					break
				}
			}
			if next < 0 {
				break
			}
			e = next
		}
		pts = straighten(pts)
		if len(pts) < 3 {
			continue
		}
		a := 0
		for k := range pts {
			p, q := pts[k], pts[(k+1)%len(pts)]
			a += p[0]*q[1] - q[0]*p[1]
		}
		// a point inside the cell left of the first edge: inside the outer ring this ring belongs to
		f := edges[i]
		pts = append(pts, pts[0])
		rings = append(rings, ring{pts, piece, a, [2]float64{float64(f.cx) + 0.5, float64(f.cy) + 0.5}})
	}
	// the outer rings of each piece, and the holes in the outer ring that holds them
	var polys [][][][2]int
	outer := map[int][]int{} // piece -> indexes in polys
	for _, r := range rings {
		if r.area > 0 {
			outer[r.piece] = append(outer[r.piece], len(polys))
			polys = append(polys, [][][2]int{r.pts})
		}
	}
	for _, r := range rings {
		if r.area >= 0 {
			continue
		}
		cands := outer[r.piece]
		if len(cands) == 0 {
			continue
		}
		host := cands[0]
		if len(cands) > 1 {
			for _, c := range cands {
				if inIntRing(r.probe, polys[c][0]) {
					host = c
					break
				}
			}
		}
		polys[host] = append(polys[host], r.pts)
	}
	return polys
}

// straighten drops the corners of a ring that lie on a straight line between their neighbours
func straighten(pts [][2]int) [][2]int {
	if len(pts) < 4 {
		return pts
	}
	out := make([][2]int, 0, len(pts))
	n := len(pts)
	for i := range pts {
		a, b, c := pts[(i+n-1)%n], pts[i], pts[(i+1)%n]
		if (b[0]-a[0])*(c[1]-b[1])-(b[1]-a[1])*(c[0]-b[0]) != 0 {
			out = append(out, b)
		}
	}
	return out
}

// inIntRing: is p inside the closed ring (even-odd)?
func inIntRing(p [2]float64, ring [][2]int) bool {
	inside := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[i], ring[j]
		ay, by := float64(a[1]), float64(b[1])
		if (ay > p[1]) != (by > p[1]) && p[0] < (float64(b[0])-float64(a[0]))*(p[1]-ay)/(by-ay)+float64(a[0]) {
			inside = !inside
		}
	}
	return inside
}

// inRingXY: is the point inside the ring (even-odd)?
func inRingXY(x, y float64, ring [][2]float64) bool {
	inside := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if (a[1] > y) != (b[1] > y) && x < (b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0] {
			inside = !inside
		}
	}
	return inside
}
