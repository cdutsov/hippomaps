package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Marks along a route by car, for small signs beside the route on the map: speed cameras
// (highway=speed_camera, in the countries where a navigation app may show them), stop signs
// (highway=stop) and speed bumps (traffic_calming), from OpenStreetMap through Overpass.
//
// Valhalla gives the ways of the route (tracedEdge). Stop signs and bumps are nodes of those ways
// on the stretch the route takes. A stop sign is the traveller's when it says so (stop=all,
// direction=forward/backward along the way); on a junction when the route comes on a road no more
// important than the others there (two residential streets: an all-way stop as mapped in North
// America); else when the junction it stands before is ahead. Marks of a kind close together are one
// (a camera on each lane of a gantry, a cushion on each side of the road). Speed cameras: nodes of a way
// of the route, or beside it on their own (to the left only near the line: further left is the
// other carriageway); a camera on another road is not on the route. The direction of a camera is
// not used: mappers write the way it faces, towards the traffic in some countries and with it in
// others.

// routeMark is a sign on the way of a route: k camera, stop or bump; at where it is (lon, lat); i the
// index of the line where the route passes it.
type routeMark struct {
	Kind string     `json:"k"`
	At   [2]float64 `json:"at"`
	I    int        `json:"i"`
}

// noCameras are the countries (ISO 3166-1) where the cameras are not shown: a navigation app may not
// warn of them (CH, LI, CY, MK, KW: TomTom offers no camera warnings there), only of danger zones
// (FR), or not while driving (DE, §23 StVO). A road whose country is not known shows none either.
var noCameras = map[string]bool{"CH": true, "LI": true, "DE": true, "FR": true, "CY": true, "MK": true, "KW": true}

const (
	bumpKinds     = "bump|hump|table|cushion|mini_bumps|dip" // traffic_calming that a car goes over
	marksQueryPts = 1500                                     // points of the line in the query for cameras, at most
	marksMax      = 3000                                     // marks in an answer
	camOnWay      = 15.0                                     // metres off the line: a camera on a way of the route
	camBeside     = 30.0                                     // a camera on its own on the side of the traffic
	camFarSide    = 12.0                                     // on the other side (left where traffic keeps right)
	onLine        = 4.0                                      // a stop sign or a bump: a node on the route
	stopAhead     = 0.06                                     // km: a stop sign is this near its junction at most
)

// markGap: marks of a kind closer than this along the route (km) are one.
var markGap = map[string]float64{"camera": 0.1, "stop": 0.015, "bump": 0.015}

// roadRank orders the road classes of Valhalla, the most important first.
var roadRank = map[string]int{"motorway": 0, "trunk": 1, "primary": 2, "secondary": 3, "tertiary": 4, "unclassified": 5, "residential": 6, "service_other": 7}

func rank(class string) int {
	if r, ok := roadRank[class]; ok {
		return r
	}
	return 7
}

// junction is where roads a car may use meet the route, at index i of the line (km along it): in is
// the rank of the road the route comes on, cross the most important of the others.
type junction struct {
	i         int
	km        float64
	in, cross int
}

// junctions are the ends of edges where another road meets the route. Service roads (driveways,
// parking aisles) count only where the route itself is on one.
func junctions(edges []tracedEdge, cum []float64) []junction {
	var out []junction
	for _, e := range edges {
		cross := 99
		for _, x := range e.EndNode.Cross {
			if x.Drive != "" && x.Drive != "none" {
				cross = min(cross, rank(x.Class))
			}
		}
		in := rank(e.Class)
		if cross == 99 || (cross == 7 && in < 7) || e.b >= len(cum) {
			continue
		}
		if k := len(out) - 1; k >= 0 && out[k].i == e.b { // an edge of no length: the same junction
			out[k].cross = min(out[k].cross, cross)
			continue
		}
		out = append(out, junction{i: e.b, km: cum[e.b], in: in, cross: cross})
	}
	return out
}

// stopFor says whether a stop sign at km along the route, on edge e, is for the traveller.
func stopFor(tags map[string]string, km float64, e *tracedEdge, js []junction) bool {
	if tags["stop"] == "all" {
		return true
	}
	switch tags["direction"] {
	case "forward":
		return e.Forward
	case "backward":
		return !e.Forward
	}
	k := sort.Search(len(js), func(n int) bool { return js[n].km >= km }) // the first junction ahead
	ahead, behind := math.Inf(1), math.Inf(1)
	if k < len(js) {
		ahead = js[k].km - km
	}
	if k > 0 {
		behind = km - js[k-1].km
	}
	switch {
	case ahead <= 0.005: // on the junction: not for the more important road
		return js[k].in >= js[k].cross
	case behind <= 0.005:
		return js[k-1].in >= js[k-1].cross
	}
	return ahead <= behind && ahead <= stopAhead
}

// lineIndex finds the segments of a line near a point: segments by cells of about a kilometre.
type lineIndex struct {
	line  [][2]float64
	cum   []float64
	cells map[[2]int32][]int
}

const cellDeg = 0.01

func newLineIndex(line [][2]float64, cum []float64) *lineIndex {
	x := &lineIndex{line: line, cum: cum, cells: map[[2]int32][]int{}}
	for i := 0; i+1 < len(line); i++ {
		a, b := line[i], line[i+1]
		x0, x1 := int32(math.Floor(min(a[0], b[0])/cellDeg)), int32(math.Floor(max(a[0], b[0])/cellDeg))
		y0, y1 := int32(math.Floor(min(a[1], b[1])/cellDeg)), int32(math.Floor(max(a[1], b[1])/cellDeg))
		if int(x1-x0+1)*int(y1-y0+1) > 10000 { // across the antimeridian or a jump: not indexed
			continue
		}
		for cx := x0; cx <= x1; cx++ {
			for cy := y0; cy <= y1; cy++ {
				x.cells[[2]int32{cx, cy}] = append(x.cells[[2]int32{cx, cy}], i)
			}
		}
	}
	return x
}

// locate finds the segment k of the line nearest to p (the first of equals): km along the line, metres
// off it, and whether p is on its left. ok is false when no segment is within about a kilometre.
func (x *lineIndex) locate(p [2]float64) (k int, km, off float64, left, ok bool) {
	cx, cy := int32(math.Floor(p[0]/cellDeg)), int32(math.Floor(p[1]/cellDeg))
	kx := math.Cos(p[1] * math.Pi / 180)
	px, py := flatXY(p, kx)
	off = math.Inf(1)
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			for _, i := range x.cells[[2]int32{cx + dx, cy + dy}] {
				ax, ay := flatXY(x.line[i], kx)
				bx, by := flatXY(x.line[i+1], kx)
				vx, vy := bx-ax, by-ay
				t := 0.0
				if l2 := vx*vx + vy*vy; l2 > 0 {
					t = math.Max(0, math.Min(1, ((px-ax)*vx+(py-ay)*vy)/l2))
				}
				d := math.Hypot(ax+t*vx-px, ay+t*vy-py)
				if d < off || (d == off && i < k) {
					k, off, ok = i, d, true
					km = x.cum[i] + (x.cum[i+1]-x.cum[i])*t
					left = vx*(py-ay)-vy*(px-ax) > 0
				}
			}
		}
	}
	return k, km, off, left, ok
}

// routeMarks finds the marks along line, a route by car with its edges, in the order of the route.
// Without edges or when Overpass fails: none.
func (s *Server) routeMarks(ctx context.Context, line [][2]float64, edges []tracedEdge) []routeMark {
	if len(edges) == 0 || len(line) < 2 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ways := map[int64]bool{}
	var ids strings.Builder
	for _, e := range edges {
		if e.Way > 0 && !ways[e.Way] {
			ways[e.Way] = true
			if ids.Len() > 0 {
				ids.WriteByte(',')
			}
			ids.WriteString(strconv.FormatInt(e.Way, 10))
		}
	}
	if len(ways) == 0 {
		return nil
	}
	// the cameras near the line: the line with fewer points, the radius larger by as much
	pts, tol := simplifyTo(line, 10, marksQueryPts)
	var coords strings.Builder
	for i, p := range pts {
		if i > 0 {
			coords.WriteByte(',')
		}
		fmt.Fprintf(&coords, "%.5f,%.5f", p[1], p[0])
	}
	q := fmt.Sprintf(`[out:json][timeout:20];way(id:%s)->.r;node(w.r)->.n;(node.n[highway=stop];node.n[traffic_calming~"^(%s)$"];);out;`+
		`node(around:%d,%s)[highway=speed_camera]->.c;.c out;way(bn.c)[highway];out skel;`, ids.String(), bumpKinds, int(math.Ceil(camBeside+tol)), coords.String())
	resp, err := s.routeClient.Do(s.overpassRequest(ctx, q))
	if err != nil {
		if ctx.Err() == nil || ctx.Err() == context.DeadlineExceeded {
			s.log.Warn("marks", "err", err)
		}
		return nil
	}
	defer resp.Body.Close()
	var d struct {
		Elements []struct {
			Type  string            `json:"type"`
			ID    int64             `json:"id"`
			Lat   float64           `json:"lat"`
			Lon   float64           `json:"lon"`
			Tags  map[string]string `json:"tags"`
			Nodes []int64           `json:"nodes"`
		} `json:"elements"`
	}
	if resp.StatusCode != http.StatusOK {
		s.log.Warn("marks", "status", resp.StatusCode)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&d); err != nil {
		s.log.Warn("marks", "err", err)
		return nil
	}
	// the cameras that are nodes of a road: of the route, or of another road only
	onRoute, onOther := map[int64]bool{}, map[int64]bool{}
	for _, e := range d.Elements {
		if e.Type != "way" {
			continue
		}
		for _, n := range e.Nodes {
			if ways[e.ID] {
				onRoute[n] = true
			} else {
				onOther[n] = true
			}
		}
	}
	cum := cumKm(line)
	edgeOf := make([]int, len(line)) // the edge of each segment of the line
	for i := range edgeOf {
		edgeOf[i] = -1
	}
	for n, e := range edges {
		for i := max(e.a, 0); i < e.b && i < len(edgeOf); i++ {
			edgeOf[i] = n
		}
	}
	js := junctions(edges, cum)
	x := newLineIndex(line, cum)
	seen := map[int64]bool{}
	type found struct {
		routeMark
		km float64
	}
	var fs []found
	for _, e := range d.Elements {
		if e.Type != "node" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		p := [2]float64{e.Lon, e.Lat}
		k, km, off, left, ok := x.locate(p)
		if !ok || edgeOf[k] < 0 {
			continue
		}
		edge := &edges[edgeOf[k]]
		kind := ""
		switch {
		case e.Tags["highway"] == "speed_camera":
			if edge.country == "" || noCameras[edge.country] || onOther[e.ID] && !onRoute[e.ID] {
				continue
			}
			farSide := left == edge.Right // left where traffic keeps right, right where it keeps left
			if (onRoute[e.ID] && off > camOnWay) || (!onRoute[e.ID] && (off > camBeside || farSide && off > camFarSide)) {
				continue
			}
			kind = "camera"
		case e.Tags["highway"] == "stop":
			if off > onLine || !stopFor(e.Tags, km, edge, js) {
				continue
			}
			kind = "stop"
		case e.Tags["traffic_calming"] != "":
			if off > onLine {
				continue
			}
			kind = "bump"
		default:
			continue
		}
		fs = append(fs, found{routeMark{Kind: kind, At: [2]float64{round(p[0], 6), round(p[1], 6)}, I: k}, km})
	}
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].km < fs[j].km })
	out := []routeMark{}
	last := map[string]float64{}
	for _, f := range fs {
		if at, ok := last[f.Kind]; ok && f.km-at < markGap[f.Kind] {
			continue
		}
		last[f.Kind] = f.km
		if out = append(out, f.routeMark); len(out) == marksMax {
			break
		}
	}
	return out
}
