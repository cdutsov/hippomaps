package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Journeys by public transport from MOTIS (JourneyURL): the timetables, with delays where an operator
// publishes them. MOTIS runs here without street data, so the walks come from Valhalla (RoutingURL): the time on
// foot from the start to each stop around it and from each stop around the destination to it (one matrix each)
// goes to MOTIS with the question, and the way of each walk of the answer is a route on foot. Without Valhalla, or
// when it fails, a walk is the straight line at walking pace.
//
//	GET {base}/api/journey?from=47.37,8.54&to=46.95,7.44[&time=2026-11-13T08:00:00Z][&arrive=1][&page=CURSOR][&lang=de]
//
// time: the departure, or with arrive=1 the arrival; now without it. page: earlier or later of an answer, for
// the journeys before or after it. lang: the names of the stops, where the timetable has them in it.
//
// answers {"trips":[{"start","end","s","changes","legs":[…]}],"earlier","later","walk":{"s","km"},"tz"}, or
// {"error":{"code","message"}}: 400 bad_request, 404 no_stops_from and no_stops_to (no stop of the timetables
// near that end) and no_journey, 429 too_many, 503 busy, 502 failed. A leg has k (walk, rail, tram, metro, bus,
// boat, cable, air, other), its seconds, from and to ({name, at: [lon, lat], t, plan: the time of the timetable
// when t is another, track, tz when not that of the answer}), and for a ride the line (name), where it is headed
// (head), the operator, the colours of the line, the stops between, live with times of the operator, alerts; for
// a walk its metres. shape is its way as an encoded polyline of p decimals; a walk without one is a straight
// line (a change of under journeyStraight metres, or between the platforms of one station). walk: on foot all the way, when the ends are within journeyWalkKm; a journey that takes as long is left out.

const (
	journeyNearM    = 1200.0 // stops around an end, and with none there
	journeyFarM     = 3500.0 // those further away: a hut an hour from the bus
	journeyNearS    = 25 * 60
	journeyFarS     = 75 * 60
	journeyMaxStops = 250 // places of stops asked of Valhalla for one end
	journeyWalkKm   = 3.0 // on foot all the way is asked up to this distance in a line
	journeyStraight = 150 // a change between stops this near is drawn as a straight line
	journeyPace     = 1.1 // metres a second in a straight line, when Valhalla has no way
	journeyFastest  = 1.4 // metres a second: no walk is faster than this in a straight line
)

type journeyAnswer struct {
	Trips   []journeyTrip `json:"trips"`
	Earlier string        `json:"earlier,omitempty"`
	Later   string        `json:"later,omitempty"`
	Walk    *journeyWalk  `json:"walk,omitempty"`
	TZ      string        `json:"tz,omitempty"`
}

type journeyWalk struct {
	S  float64 `json:"s"`
	Km float64 `json:"km"`
}

type journeyTrip struct {
	Start   string       `json:"start"`
	End     string       `json:"end"`
	S       int          `json:"s"`
	Changes int          `json:"changes"`
	Legs    []journeyLeg `json:"legs"`
}

type journeyPlace struct {
	Name  string     `json:"name,omitempty"`
	At    [2]float64 `json:"at"` // lon, lat
	T     string     `json:"t"`
	Plan  string     `json:"plan,omitempty"`
	Track string     `json:"track,omitempty"`
	TZ    string     `json:"tz,omitempty"`
}

type journeyStop struct {
	Name string `json:"name"`
	T    string `json:"t,omitempty"`
}

type journeyLeg struct {
	K         string        `json:"k"`
	S         int           `json:"s"`
	M         int           `json:"m,omitempty"`
	Name      string        `json:"name,omitempty"`
	Head      string        `json:"head,omitempty"`
	Agency    string        `json:"agency,omitempty"`
	Colour    string        `json:"colour,omitempty"`
	Text      string        `json:"text,omitempty"`
	From      journeyPlace  `json:"from"`
	To        journeyPlace  `json:"to"`
	Stops     []journeyStop `json:"stops,omitempty"`
	Live      bool          `json:"live,omitempty"`
	Cancelled bool          `json:"cancelled,omitempty"`
	Alerts    []string      `json:"alerts,omitempty"`
	Shape     string        `json:"shape,omitempty"`
	P         int           `json:"p,omitempty"`
}

// what MOTIS answers (api/v6), the part the page needs
type motisPlace struct {
	Name               string  `json:"name"`
	StopID             string  `json:"stopId"`
	ParentID           string  `json:"parentId"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	TZ                 string  `json:"tz"`
	Arrival            string  `json:"arrival"`
	Departure          string  `json:"departure"`
	ScheduledArrival   string  `json:"scheduledArrival"`
	ScheduledDeparture string  `json:"scheduledDeparture"`
	Track              string  `json:"track"`
}

type motisLeg struct {
	Mode               string       `json:"mode"`
	From               motisPlace   `json:"from"`
	To                 motisPlace   `json:"to"`
	Duration           int          `json:"duration"`
	StartTime          string       `json:"startTime"`
	EndTime            string       `json:"endTime"`
	ScheduledStartTime string       `json:"scheduledStartTime"`
	ScheduledEndTime   string       `json:"scheduledEndTime"`
	RealTime           bool         `json:"realTime"`
	Cancelled          bool         `json:"cancelled"`
	Headsign           string       `json:"headsign"`
	DisplayName        string       `json:"displayName"`
	RouteShortName     string       `json:"routeShortName"`
	RouteLongName      string       `json:"routeLongName"`
	TripShortName      string       `json:"tripShortName"`
	AgencyName         string       `json:"agencyName"`
	RouteColor         string       `json:"routeColor"`
	RouteTextColor     string       `json:"routeTextColor"`
	IntermediateStops  []motisPlace `json:"intermediateStops"`
	LegGeometry        struct {
		Points    string `json:"points"`
		Precision int    `json:"precision"`
	} `json:"legGeometry"`
	Alerts []struct {
		HeaderText      string `json:"headerText"`
		DescriptionText string `json:"descriptionText"`
	} `json:"alerts"`
}

type motisPlan struct {
	Itineraries []struct {
		Duration  int        `json:"duration"`
		StartTime string     `json:"startTime"`
		EndTime   string     `json:"endTime"`
		Transfers int        `json:"transfers"`
		Legs      []motisLeg `json:"legs"`
	} `json:"itineraries"`
	PreviousPageCursor string `json:"previousPageCursor"`
	NextPageCursor     string `json:"nextPageCursor"`
}

// journeyKinds: the modes of MOTIS as the kinds of the page; what is not here is "other"
var journeyKinds = map[string]string{
	"WALK": "walk", "TRAM": "tram", "SUBWAY": "metro", "METRO": "metro", "FERRY": "boat", "AIRPLANE": "air", "BUS": "bus", "COACH": "bus",
	"RAIL": "rail", "HIGHSPEED_RAIL": "rail", "LONG_DISTANCE": "rail", "NIGHT_RAIL": "rail", "REGIONAL_FAST_RAIL": "rail",
	"REGIONAL_RAIL": "rail", "SUBURBAN": "rail", "FUNICULAR": "cable", "AERIAL_LIFT": "cable", "AREAL_LIFT": "cable", "CABLE_CAR": "cable",
}

var (
	hexColour   = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
	journeyLang = regexp.MustCompile(`^[a-z]{2,3}$`)
	cursorOK    = regexp.MustCompile(`^[\x21-\x7e]+$`)
)

// foot is a walk between a point and a stop: seconds and metres
type foot struct {
	s, m float64
}

// spot is a place of stops: the platforms of two timetables at the same point are one (about 10 m)
type spot struct{ lat, lon float64 }

func spotOf(lat, lon float64) spot { return spot{round(lat, 4), round(lon, 4)} }

func metresBetween(lat1, lon1, lat2, lon2 float64) float64 {
	return 1000 * kmBetween(lat1, lon1, lat2, lon2)
}

func (s *Server) journey(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat1, lon1, ok1 := latLon(q.Get("from"))
	lat2, lon2, ok2 := latLon(q.Get("to"))
	if !ok1 || !ok2 {
		routeError(w, http.StatusBadRequest, "bad_request", "from and to as lat,lon")
		return
	}
	now := time.Now()
	when := now
	if v := q.Get("time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil || t.Before(now.Add(-24*time.Hour)) || t.After(now.Add(366*24*time.Hour)) {
			routeError(w, http.StatusBadRequest, "bad_request", "time as 2026-11-13T08:00:00Z, from yesterday to a year ahead")
			return
		}
		when = t
	}
	page := q.Get("page")
	if page != "" && (len(page) > 4096 || !cursorOK.MatchString(page)) {
		routeError(w, http.StatusBadRequest, "bad_request", "page as an answer gave it")
		return
	}
	lang := strings.ToLower(q.Get("lang"))
	if !journeyLang.MatchString(lang) {
		lang = "en"
	}
	if !s.routesPerIP.allow(visitor(r), now) {
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many directions in a short time. Please wait a few minutes.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	select {
	case s.routeCalls <- struct{}{}:
		defer func() { <-s.routeCalls }()
	case <-time.After(5 * time.Second):
		routeError(w, http.StatusServiceUnavailable, "busy", "Directions are busy. Please try again.")
		return
	case <-ctx.Done():
		return
	}
	failed := func(what string, err error) {
		if !errors.Is(r.Context().Err(), context.Canceled) {
			s.log.Warn("journey failed", "what", what, "err", err)
		}
		routeError(w, http.StatusBadGateway, "failed", "Directions failed. Please try again.")
	}

	// the stops around both ends, and on foot all the way when the ends are near
	var (
		wg           sync.WaitGroup
		ends         [2]journeyEnd
		errs         [2]error
		direct       *journeyWalk
		lats, lons   = [2]float64{lat1, lat2}, [2]float64{lon1, lon2}
		straightLine = kmBetween(lat1, lon1, lat2, lon2)
	)
	for i := range ends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ends[i], errs[i] = s.journeyEnd(ctx, lats[i], lons[i], i == 1)
		}()
	}
	if straightLine <= journeyWalkKm && s.cfg.RoutingURL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f, _, ok := s.walkWay(ctx, lat1, lon1, lat2, lon2); ok {
				direct = &journeyWalk{S: math.Round(f.s), Km: round(f.m/1000, 3)}
			}
		}()
	}
	wg.Wait()
	for i, code := range []string{"no_stops_from", "no_stops_to"} {
		if errs[i] != nil {
			failed("stops", errs[i])
			return
		}
		if len(ends[i].offsets) == 0 {
			if direct != nil { // no stop near, and near enough to walk
				break
			}
			msg := "No public transport stop of the timetables near the start."
			if i == 1 {
				msg = "No public transport stop of the timetables near the destination."
			}
			routeError(w, http.StatusNotFound, code, msg)
			return
		}
	}

	a := journeyAnswer{Trips: []journeyTrip{}, Walk: direct}
	if len(ends[0].offsets) > 0 && len(ends[1].offsets) > 0 {
		ask := url.Values{
			"fromPlace": {fmt.Sprintf("%.6f,%.6f", lat1, lon1)}, "toPlace": {fmt.Sprintf("%.6f,%.6f", lat2, lon2)},
			"time": {when.UTC().Format(time.RFC3339)}, "arriveBy": {strconv.FormatBool(q.Get("arrive") == "1")},
			"numItineraries": {"5"}, "detailedTransfers": {"false"}, "timeout": {"15"}, "language": {lang},
		}
		if page != "" {
			ask.Set("pageCursor", page)
		}
		body, _ := json.Marshal(map[string]any{"fromOffsets": ends[0].offsets, "toOffsets": ends[1].offsets})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.JourneyURL+"/api/v6/plan?"+ask.Encode(), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", s.cfg.UserAgent)
		resp, err := s.routeClient.Do(req)
		if err != nil {
			failed("plan", err)
			return
		}
		defer resp.Body.Close()
		var plan motisPlan
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
			if resp.StatusCode == http.StatusBadRequest && page != "" { // a page of another answer, or an old one
				routeError(w, http.StatusBadRequest, "bad_request", "page as an answer gave it")
				return
			}
			failed("plan", fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw))
			return
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&plan); err != nil {
			failed("plan", err)
			return
		}
		a.Earlier, a.Later = plan.PreviousPageCursor, plan.NextPageCursor
		a.Trips, a.TZ = s.journeyTrips(ctx, plan, [2]float64{lat1, lon1}, [2]float64{lat2, lon2}, ends, direct)
	}
	if len(a.Trips) == 0 && a.Walk == nil {
		routeError(w, http.StatusNotFound, "no_journey", "No journey by public transport between these points at this time.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a)
}

// journeyEnd is one end of a journey: what MOTIS is told of the stops around it, and the walk to each place of
// stops (by its spot)
type journeyEnd struct {
	offsets []map[string]any
	walks   map[spot]foot
}

type motisStop struct {
	StopID string  `json:"stopId"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

// journeyEnd finds the stops around a point and the walk to each (toPoint: from the stop to the point, the
// destination): those within journeyNearM that take at most journeyNearS on foot, and with none of them those
// within journeyFarM and journeyFarS.
func (s *Server) journeyEnd(ctx context.Context, lat, lon float64, toPoint bool) (journeyEnd, error) {
	for _, reach := range [][2]float64{{journeyNearM, journeyNearS}, {journeyFarM, journeyFarS}} {
		stops, err := s.stopsAround(ctx, lat, lon, reach[0])
		if err != nil {
			return journeyEnd{}, err
		}
		if len(stops) == 0 {
			continue
		}
		// the places of the stops, the nearest first
		at := map[spot]float64{}
		for _, st := range stops {
			at[spotOf(st.Lat, st.Lon)] = metresBetween(lat, lon, st.Lat, st.Lon)
		}
		spots := make([]spot, 0, len(at))
		for p := range at {
			spots = append(spots, p)
		}
		sort.Slice(spots, func(i, j int) bool {
			if at[spots[i]] != at[spots[j]] {
				return at[spots[i]] < at[spots[j]]
			}
			return spots[i].lat < spots[j].lat || spots[i].lat == spots[j].lat && spots[i].lon < spots[j].lon
		})
		if len(spots) > journeyMaxStops {
			spots = spots[:journeyMaxStops]
		}
		walks := s.walkTimes(ctx, lat, lon, spots, toPoint)
		if len(walks) == 0 { // no Valhalla, or no way on foot from here: the straight line
			walks = map[spot]foot{}
			for _, p := range spots {
				walks[p] = foot{at[p] * 1.3 / journeyPace, at[p] * 1.3}
			}
		}
		e := journeyEnd{walks: walks}
		for _, st := range stops {
			f, ok := walks[spotOf(st.Lat, st.Lon)]
			if !ok || f.s > reach[1] {
				continue
			}
			// MOTIS counts in whole minutes and cuts the seconds off: the minute begun counts, so no walk is shorter
			// than it is, and the walk of a stop right here is still a leg of the answer
			e.offsets = append(e.offsets, map[string]any{"stopId": st.StopID, "duration": 60 * int(math.Max(1, math.Ceil(f.s/60))), "mode": "WALK"})
		}
		if len(e.offsets) > 0 {
			return e, nil
		}
	}
	return journeyEnd{}, nil
}

// stopsAround asks MOTIS for the stops with departures in a box of metres around a point, those within the
// metres in a line
func (s *Server) stopsAround(ctx context.Context, lat, lon, metres float64) ([]motisStop, error) {
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
	var all, out []motisStop
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

// walkTimes asks Valhalla for the walk between a point and each spot, in one matrix (toPoint: from the spots to
// the point). No answer, or a spot without a way on foot: not in the result.
func (s *Server) walkTimes(ctx context.Context, lat, lon float64, spots []spot, toPoint bool) map[spot]foot {
	if s.cfg.RoutingURL == "" || len(spots) == 0 {
		return nil
	}
	point := []map[string]float64{{"lat": lat, "lon": lon}}
	many := make([]map[string]float64, len(spots))
	for i, p := range spots {
		many[i] = map[string]float64{"lat": p.lat, "lon": p.lon}
	}
	ask := map[string]any{"sources": point, "targets": many, "costing": "pedestrian", "units": "kilometers", "verbose": false}
	if toPoint {
		ask["sources"], ask["targets"] = many, point
	}
	body, _ := json.Marshal(ask)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/sources_to_targets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var v struct {
		M struct {
			Durations [][]*float64 `json:"durations"`
			Distances [][]*float64 `json:"distances"`
		} `json:"sources_to_targets"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&v) != nil {
		s.log.Warn("journey: walk times", "status", resp.StatusCode)
		return nil
	}
	cell := func(m [][]*float64, i int) *float64 {
		r, c := 0, i
		if toPoint {
			r, c = i, 0
		}
		if r >= len(m) || c >= len(m[r]) {
			return nil
		}
		return m[r][c]
	}
	out := map[spot]foot{}
	for i, p := range spots {
		d, km := cell(v.M.Durations, i), cell(v.M.Distances, i)
		if d == nil || km == nil {
			continue
		}
		// a point far from any way (a glacier, a lake) is put on the nearest one, and the walk counted from there: it
		// takes at least the straight line at walking pace
		straight := metresBetween(lat, lon, p.lat, p.lon)
		out[p] = foot{math.Max(*d, straight/journeyFastest), math.Max(*km*1000, straight)}
	}
	return out
}

// walkWay is the route on foot between two points from Valhalla: its time and length, and its line as an
// encoded polyline of 6 decimals
func (s *Server) walkWay(ctx context.Context, lat1, lon1, lat2, lon2 float64) (foot, string, bool) {
	if s.cfg.RoutingURL == "" {
		return foot{}, "", false
	}
	body, _ := json.Marshal(map[string]any{
		"locations": []map[string]float64{{"lat": lat1, "lon": lon1}, {"lat": lat2, "lon": lon2}},
		"costing":   "pedestrian", "units": "kilometers", "directions_type": "none",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.RoutingURL+"/route", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return foot{}, "", false
	}
	defer resp.Body.Close()
	var v valhallaRoute
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&v) != nil || len(v.Trip.Legs) != 1 || v.Trip.Legs[0].Shape == "" {
		return foot{}, "", false
	}
	return foot{v.Trip.Summary.Time, v.Trip.Summary.Length * 1000}, v.Trip.Legs[0].Shape, true
}

// journeyTrips turns the itineraries of MOTIS into the trips of the answer, with the way of each walk. A trip
// that takes as long as the way on foot (direct) is left out. tz: the time zone of the first stop.
func (s *Server) journeyTrips(ctx context.Context, plan motisPlan, from, to [2]float64, ends [2]journeyEnd, direct *journeyWalk) ([]journeyTrip, string) {
	trips, tz := []journeyTrip{}, ""
	type way struct{ a, b [2]float64 } // lat, lon of both ends
	ways := map[way][]*journeyLeg{}
	knownM := map[way]float64{} // the metres of a walk as the matrix had them
	for _, it := range plan.Itineraries {
		rides := 0
		for _, l := range it.Legs {
			if l.Mode != "WALK" {
				rides++
			}
		}
		if rides == 0 || direct != nil && float64(it.Duration) >= direct.S {
			continue
		}
		trip := journeyTrip{Start: it.StartTime, End: it.EndTime, S: it.Duration, Changes: it.Transfers, Legs: make([]journeyLeg, 0, len(it.Legs))}
		for n, l := range it.Legs {
			if tz == "" {
				tz = firstOf(l.From.TZ, l.To.TZ)
			}
			leg := journeyLeg{K: journeyKinds[l.Mode], S: l.Duration,
				From: journeyPlace{Name: l.From.Name, At: [2]float64{round(l.From.Lon, 6), round(l.From.Lat, 6)}, T: firstOf(l.From.Departure, l.StartTime), Track: l.From.Track},
				To:   journeyPlace{Name: l.To.Name, At: [2]float64{round(l.To.Lon, 6), round(l.To.Lat, 6)}, T: firstOf(l.To.Arrival, l.EndTime), Track: l.To.Track}}
			if leg.K == "" {
				leg.K = "other"
			}
			if l.From.TZ != tz {
				leg.From.TZ = l.From.TZ
			}
			if l.To.TZ != tz {
				leg.To.TZ = l.To.TZ
			}
			if leg.K != "walk" {
				leg.Name = firstOf(l.DisplayName, l.RouteShortName, l.TripShortName, l.RouteLongName)
				leg.Head, leg.Agency, leg.Live, leg.Cancelled = l.Headsign, l.AgencyName, l.RealTime, l.Cancelled
				if hexColour.MatchString(l.RouteColor) {
					leg.Colour = "#" + strings.ToLower(l.RouteColor)
					if hexColour.MatchString(l.RouteTextColor) {
						leg.Text = "#" + strings.ToLower(l.RouteTextColor)
					}
				}
				if p := firstOf(l.From.ScheduledDeparture, l.ScheduledStartTime); p != leg.From.T {
					leg.From.Plan = p
				}
				if p := firstOf(l.To.ScheduledArrival, l.ScheduledEndTime); p != leg.To.T {
					leg.To.Plan = p
				}
				for _, st := range l.IntermediateStops {
					leg.Stops = append(leg.Stops, journeyStop{Name: st.Name, T: firstOf(st.Departure, st.Arrival)})
				}
				for _, al := range l.Alerts {
					if t := strings.TrimSpace(firstOf(al.HeaderText, al.DescriptionText)); t != "" && len(leg.Alerts) < 3 {
						leg.Alerts = append(leg.Alerts, clip(t, 300))
					}
				}
				leg.Shape, leg.P = l.LegGeometry.Points, l.LegGeometry.Precision
				trip.Legs = append(trip.Legs, leg)
				continue
			}
			// a walk: from the start to a stop, between two stops, from a stop to the destination
			leg.From.Track, leg.To.Track = "", ""
			a, b := [2]float64{l.From.Lat, l.From.Lon}, [2]float64{l.To.Lat, l.To.Lon}
			known := false
			if n == 0 {
				leg.From.Name, a = "", from
				if f, ok := ends[0].walks[spotOf(l.To.Lat, l.To.Lon)]; ok {
					leg.M, known = int(math.Round(f.m)), true
				}
			}
			if n == len(it.Legs)-1 {
				leg.To.Name, b = "", to
				if f, ok := ends[1].walks[spotOf(l.From.Lat, l.From.Lon)]; ok {
					leg.M, known = int(math.Round(f.m)), true
				}
			}
			leg.From.At, leg.To.At = [2]float64{round(a[1], 6), round(a[0], 6)}, [2]float64{round(b[1], 6), round(b[0], 6)}
			straight := metresBetween(a[0], a[1], b[0], b[1])
			if !known {
				leg.M = int(math.Round(straight))
			}
			trip.Legs = append(trip.Legs, leg)
			// a change inside a station (two platforms of one station): its ways are not on the map of the streets
			inside := !known && (l.From.Name == l.To.Name || l.From.ParentID != "" && l.From.ParentID == l.To.ParentID)
			if known || straight > journeyStraight && !inside {
				k := way{[2]float64{round(a[0], 5), round(a[1], 5)}, [2]float64{round(b[0], 5), round(b[1], 5)}}
				ways[k] = append(ways[k], &trip.Legs[len(trip.Legs)-1])
				knownM[k] = math.Max(float64(leg.M), straight)
			}
		}
		trips = append(trips, trip)
	}
	// the way of each walk, the same walk of several trips once; at most four at a time
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for k, legs := range ways {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			f, shape, ok := s.walkWay(ctx, k.a[0], k.a[1], k.b[0], k.b[1])
			// a way over twice as long as the walk MOTIS counted with is another walk (an end far from any way): the
			// straight line then
			if !ok || f.m > 2*knownM[k]+200 {
				return
			}
			for _, leg := range legs {
				leg.Shape, leg.P, leg.M = shape, 6, int(math.Round(f.m))
			}
		}()
	}
	wg.Wait()
	return trips, tz
}

func firstOf(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// clip cuts a text to n runes
func clip(t string, n int) string {
	r := []rune(t)
	if len(r) <= n {
		return t
	}
	return string(r[:n-1]) + "…"
}
