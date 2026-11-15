package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
)

// Other routes from start to destination (alts=1 or 2 on /api/route): each a whole answer as the route itself
// (steps, and by car the limits, traffic and marks, by bike and on foot the heights), in alts of the answer.
//
// Valhalla finds alternates only without a time (with date_time it finds none), so they come from a second call
// without one: its best route and its alternates. Those that mostly follow the route (or one another) go; each
// other one is driven again leaving now (trace_route, edge_walk, date_time): its time with live traffic, and a
// way closed now (a pass in winter) fails there, so that route goes too. A route slower than maxAltSlower times
// the route goes as well.

// maxAlts is the most other routes (max_alternates of Valhalla on overpass1).
const maxAlts = 2

// maxAltShared: a route that shares more of its length with the route or another one shown is not shown.
const maxAltShared = 0.85

// maxAltSlower: a route that takes longer than this many times the route is not shown.
const maxAltSlower = 1.5

func (s *Server) alternatives(ctx context.Context, ask map[string]any, main *routeAnswer, n int, costing string) []*routeAnswer {
	again := map[string]any{}
	for k, v := range ask {
		if k != "date_time" {
			again[k] = v
		}
	}
	again["alternates"] = n
	var v valhallaRoute
	if !s.valhalla(ctx, "/route", again, &v) || len(v.Trip.Legs) != 1 {
		return nil
	}
	trips := []valhallaTrip{v.Trip}
	for _, a := range v.Alternates {
		trips = append(trips, a.Trip)
	}
	shown := [][][2]float64{main.Line}
	var out []*routeAnswer
	for _, t := range trips {
		if len(out) == n || len(t.Legs) != 1 {
			continue
		}
		line := decodePolyline(t.Legs[0].Shape, 1e6)
		if len(line) < 2 || sharedMost(line, shown) {
			continue
		}
		// driven again now: the time with traffic, the steps in the language of the route; none when closed now
		var now valhallaRoute
		drive := map[string]any{"encoded_polyline": t.Legs[0].Shape, "costing": costing, "shape_match": "edge_walk",
			"date_time": map[string]int{"type": 0}, "units": "kilometers", "language": ask["language"], "turn_lanes": true}
		if !s.valhalla(ctx, "/trace_route", drive, &now) {
			continue
		}
		a := answerOf(now.Trip, main.lang)
		if a == nil || a.S > main.S*maxAltSlower {
			continue
		}
		shown = append(shown, line)
		out = append(out, a)
	}
	var wg sync.WaitGroup
	for _, a := range out {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.extras(ctx, a, costing)
		}()
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].S < out[j].S })
	return out
}

// valhalla posts a request to Valhalla and reads its answer into v: false on any failure.
func (s *Server) valhalla(ctx context.Context, path string, ask map[string]any, v any) bool {
	body, _ := json.Marshal(ask)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(v) == nil
}

// sharedMost: does line share more than maxAltShared of its length with one of others? The lines of Valhalla
// have the same points on the same roads, so a stretch is shared when both its ends are points of the other.
func sharedMost(line [][2]float64, others [][][2]float64) bool {
	key := func(p [2]float64) [2]int64 {
		return [2]int64{int64(math.Round(p[0] * 1e6)), int64(math.Round(p[1] * 1e6))}
	}
	for _, o := range others {
		on := make(map[[2]int64]bool, len(o))
		for _, p := range o {
			on[key(p)] = true
		}
		all, both := 0.0, 0.0
		for i := 1; i < len(line); i++ {
			d := kmBetween(line[i-1][1], line[i-1][0], line[i][1], line[i][0])
			all += d
			if on[key(line[i-1])] && on[key(line[i])] {
				both += d
			}
		}
		if all == 0 || both/all > maxAltShared {
			return true
		}
	}
	return false
}
