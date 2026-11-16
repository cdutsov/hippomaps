package server

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"
)

// The height profile of a line drawn on the map or opened from a file (the tool Measure of the page): POST
// /api/heights with {"line": [[lon, lat], …]}, the answer as the elev of a route (routeElev). The elevation API of
// overpass.ch with ElevationKey; the visitor counts as a search of routes (routesPerIP).
const (
	heightsMaxPoints = 5000 // the page thins a longer line to this many points
	heightsMaxKm     = 2000
)

func (s *Server) heightsOf(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Line [][2]float64 `json:"line"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || len(req.Line) < 2 || len(req.Line) > heightsMaxPoints {
		routeError(w, http.StatusBadRequest, "bad_request", "A JSON body: line as [[lon,lat],…], 2 to 5000 points.")
		return
	}
	km := 0.0
	for i, p := range req.Line {
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) || p[0] < -180 || p[0] > 180 || p[1] < -90 || p[1] > 90 {
			routeError(w, http.StatusBadRequest, "bad_request", "line as [[lon,lat],…]")
			return
		}
		if i > 0 {
			km += kmBetween(req.Line[i-1][1], req.Line[i-1][0], p[1], p[0])
		}
	}
	if km > heightsMaxKm {
		routeError(w, http.StatusBadRequest, "too_long", "The line is longer than 2000 km.")
		return
	}
	if !s.routesPerIP.allow(visitor(r), time.Now()) {
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many searches in a short time. Please wait a few minutes.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	e := s.heights(ctx, req.Line, math.Max(km, 0.01))
	if e == nil {
		if ctx.Err() == nil {
			routeError(w, http.StatusBadGateway, "upstream", "The heights could not be found. Please try again.")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(e)
}
