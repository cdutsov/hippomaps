package server

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMotis answers the stops in a box and a plan, and keeps what was asked; fakeWalks is Valhalla for the walks.
type fakeMotis struct {
	srv   *httptest.Server
	mu    sync.Mutex
	query url.Values     // of the last plan
	body  map[string]any // of the last plan
	plan  string         // the answer
	code  int            // of the plan, when not 200
}

var journeyStops = []motisStop{
	{"ch26_A", 47.3780, 8.5400}, {"ch27_A", 47.37801, 8.54001}, // the same platform in two timetables
	{"ch26_B", 47.3790, 8.5390},
	{"ch26_island", 47.3760, 8.5420}, // no way on foot
	{"ch26_far", 47.3879, 8.5403},    // 1.1 km: over 25 minutes on foot in the fake
	{"ch26_bern", 46.9490, 7.4395},
	{"ch26_hut", 46.5225, 8.0000}, // the only stop near a hut, 2.5 km away
	{"ch26_near", 47.3850, 8.5403},
	{"ch26_joch", 46.5476, 7.9824}, // 1.9 km from a point on a glacier
}

// a walk of the fake Valhalla: 100 s and 120 m for each 100 m in a line; the island has no way, the far stop takes
// 2000 s; from the glacier the matrix counts from the nearest path (283 s, 401 m) and the route goes round the
// mountain (20 km)
func fakeWalk(lat1, lon1, lat2, lon2 float64, matrix bool) (s, km float64, ok bool) {
	if math.Abs(lat1-46.5367) < 1e-6 {
		if matrix {
			return 283, 0.401, true
		}
		return 14651, 20.509, true
	}
	for _, p := range [][2]float64{{lat1, lon1}, {lat2, lon2}} {
		if math.Abs(p[0]-47.3760) < 1e-6 && math.Abs(p[1]-8.5420) < 1e-6 {
			return 0, 0, false
		}
		if math.Abs(p[0]-47.3879) < 1e-6 {
			return 2000, 1.4, true
		}
	}
	m := metresBetween(lat1, lon1, lat2, lon2)
	return math.Round(m), math.Round(m*1.2) / 1000, true
}

const journeyPlan = `{"itineraries":[
 {"duration":4200,"startTime":"2026-11-13T08:00:00Z","endTime":"2026-11-13T09:10:00Z","transfers":1,"legs":[
  {"mode":"WALK","from":{"name":"START","lat":47.3779,"lon":8.5403,"tz":"Europe/Zurich","departure":"2026-11-13T08:00:00Z"},
   "to":{"name":"Zürich HB","stopId":"ch26_A","lat":47.3780,"lon":8.5400,"tz":"Europe/Zurich","arrival":"2026-11-13T08:02:00Z","track":"7"},
   "duration":120,"startTime":"2026-11-13T08:00:00Z","endTime":"2026-11-13T08:02:00Z","legGeometry":{"points":"","precision":6,"length":0}},
  {"mode":"LONG_DISTANCE","from":{"name":"Zürich HB","stopId":"ch26_A","lat":47.3780,"lon":8.5400,"tz":"Europe/Zurich","departure":"2026-11-13T08:05:00Z","scheduledDeparture":"2026-11-13T08:02:00Z","track":"7"},
   "to":{"name":"Bern","stopId":"ch26_bern","lat":46.9490,"lon":7.4395,"tz":"Europe/Zurich","arrival":"2026-11-13T09:00:00Z","scheduledArrival":"2026-11-13T09:00:00Z","track":"3"},
   "duration":3300,"startTime":"2026-11-13T08:05:00Z","endTime":"2026-11-13T09:00:00Z","scheduledStartTime":"2026-11-13T08:02:00Z","scheduledEndTime":"2026-11-13T09:00:00Z",
   "realTime":true,"headsign":"Genève-Aéroport","displayName":"IC1","routeShortName":"IC","agencyName":"SBB","routeColor":"EB0000","routeTextColor":"FFFFFF",
   "intermediateStops":[{"name":"Olten","lat":47.35,"lon":7.9,"arrival":"2026-11-13T08:33:00Z","departure":"2026-11-13T08:35:00Z"}],
   "legGeometry":{"points":"railshape","precision":6,"length":900},
   "alerts":[{"headerText":"","descriptionText":"Lift out of order in Bern"},{"headerText":"A","descriptionText":"a"},{"headerText":"B","descriptionText":"b"},{"headerText":"C","descriptionText":"c"}]},
  {"mode":"WALK","from":{"name":"Bern","stopId":"ch26_bern","lat":46.9490,"lon":7.4395,"tz":"Europe/Zurich","departure":"2026-11-13T09:00:00Z","track":"3"},
   "to":{"name":"Bern, Bahnhof","stopId":"ch26_tram","lat":46.9485,"lon":7.4400,"tz":"Europe/Zurich","arrival":"2026-11-13T09:04:00Z"},
   "duration":240,"startTime":"2026-11-13T09:00:00Z","endTime":"2026-11-13T09:04:00Z","legGeometry":{"points":"","precision":6,"length":0}},
  {"mode":"TRAM","from":{"name":"Bern, Bahnhof","stopId":"ch26_tram","lat":46.9485,"lon":7.4400,"tz":"Europe/Zurich","departure":"2026-11-13T09:05:00Z","scheduledDeparture":"2026-11-13T09:05:00Z"},
   "to":{"name":"Bern, Zytglogge","stopId":"ch26_zyt","lat":46.9480,"lon":7.4480,"tz":"Europe/Zurich","arrival":"2026-11-13T09:08:00Z","scheduledArrival":"2026-11-13T09:08:00Z"},
   "duration":180,"startTime":"2026-11-13T09:05:00Z","endTime":"2026-11-13T09:08:00Z","scheduledStartTime":"2026-11-13T09:05:00Z","scheduledEndTime":"2026-11-13T09:08:00Z",
   "realTime":false,"headsign":"Ostring","displayName":"","routeShortName":"7","agencyName":"Bernmobil","routeColor":"","legGeometry":{"points":"tramshape","precision":6,"length":40}},
  {"mode":"WALK","from":{"name":"Bern, Zytglogge","stopId":"ch26_zyt","lat":46.9480,"lon":7.4480,"tz":"Europe/Zurich","departure":"2026-11-13T09:08:00Z"},
   "to":{"name":"END","lat":46.9490,"lon":7.4391,"tz":"Europe/Zurich","arrival":"2026-11-13T09:10:00Z"},
   "duration":120,"startTime":"2026-11-13T09:08:00Z","endTime":"2026-11-13T09:10:00Z","legGeometry":{"points":"","precision":6,"length":0}}]},
 {"duration":600,"startTime":"2026-11-13T08:10:00Z","endTime":"2026-11-13T08:20:00Z","transfers":0,"legs":[
  {"mode":"WALK","from":{"name":"START","lat":47.3779,"lon":8.5403,"tz":"Europe/Zurich","departure":"2026-11-13T08:10:00Z"},
   "to":{"name":"END","lat":46.9490,"lon":7.4391,"tz":"Europe/Zurich","arrival":"2026-11-13T08:20:00Z"},"duration":600,"startTime":"2026-11-13T08:10:00Z","endTime":"2026-11-13T08:20:00Z"}]},
 {"duration":900,"startTime":"2026-11-13T08:30:00Z","endTime":"2026-11-13T08:45:00Z","transfers":0,"legs":[
  {"mode":"WALK","from":{"name":"START","lat":47.3779,"lon":8.5403,"tz":"Europe/Zurich","departure":"2026-11-13T08:30:00Z"},
   "to":{"name":"Zürich HB","stopId":"ch26_A","lat":47.3780,"lon":8.5400,"tz":"Europe/Zurich","arrival":"2026-11-13T08:32:00Z"},"duration":120,"startTime":"2026-11-13T08:30:00Z","endTime":"2026-11-13T08:32:00Z"},
  {"mode":"SUBURBAN","from":{"name":"Zürich HB","lat":47.3780,"lon":8.5400,"tz":"Europe/Zurich","departure":"2026-11-13T08:32:00Z","scheduledDeparture":"2026-11-13T08:32:00Z"},
   "to":{"name":"Zürich HB","parentId":"p1","lat":47.3781,"lon":8.5370,"tz":"Europe/Zurich","arrival":"2026-11-13T08:32:30Z","scheduledArrival":"2026-11-13T08:32:30Z"},
   "duration":30,"startTime":"2026-11-13T08:32:00Z","endTime":"2026-11-13T08:32:30Z","displayName":"S3"},
  {"mode":"WALK","from":{"name":"Zürich HB","parentId":"p1","lat":47.3781,"lon":8.5370,"tz":"Europe/Zurich","departure":"2026-11-13T08:32:30Z"},
   "to":{"name":"Zürich HB (Tief)","parentId":"p1","lat":47.3781,"lon":8.5400,"tz":"Europe/Zurich","arrival":"2026-11-13T08:32:40Z"},
   "duration":10,"startTime":"2026-11-13T08:32:30Z","endTime":"2026-11-13T08:32:40Z"},
  {"mode":"FUNICULAR","from":{"name":"Zürich HB","lat":47.3780,"lon":8.5400,"tz":"Europe/Zurich","departure":"2026-11-13T08:32:00Z","scheduledDeparture":"2026-11-13T08:32:00Z"},
   "to":{"name":"Up","lat":47.3800,"lon":8.5450,"tz":"Europe/Berlin","arrival":"2026-11-13T08:45:00Z","scheduledArrival":"2026-11-13T08:45:00Z"},
   "duration":780,"startTime":"2026-11-13T08:32:00Z","endTime":"2026-11-13T08:45:00Z","routeLongName":"Polybahn","cancelled":true,"legGeometry":{"points":"cable","precision":7,"length":3}}]}
],"previousPageCursor":"EARLIER|1","nextPageCursor":"LATER|2"}`

func newFakeMotis(t *testing.T) *fakeMotis {
	f := &fakeMotis{plan: journeyPlan}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v6/map/stops":
			lat1, lon1, ok1 := latLon(r.URL.Query().Get("min"))
			lat2, lon2, ok2 := latLon(r.URL.Query().Get("max"))
			if !ok1 || !ok2 || lat1 >= lat2 || lon1 >= lon2 {
				w.WriteHeader(400)
				return
			}
			if lat1 < 1 { // the sea off Africa: MOTIS fails
				w.WriteHeader(500)
				return
			}
			out := []motisStop{}
			for _, st := range journeyStops {
				if st.Lat >= lat1 && st.Lat <= lat2 && st.Lon >= lon1 && st.Lon <= lon2 {
					out = append(out, st)
				}
			}
			json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v6/plan":
			f.mu.Lock()
			defer f.mu.Unlock()
			f.query, f.body = r.URL.Query(), nil
			json.NewDecoder(r.Body).Decode(&f.body)
			if f.code != 0 {
				w.WriteHeader(f.code)
				io.WriteString(w, `{"error":"no"}`)
				return
			}
			io.WriteString(w, f.plan)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type fakeWalks struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes int
	down   bool
}

func newFakeWalks(t *testing.T) *fakeWalks {
	f := &fakeWalks{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.down {
			w.WriteHeader(503)
			return
		}
		var req struct {
			Sources   []struct{ Lat, Lon float64 }
			Targets   []struct{ Lat, Lon float64 }
			Locations []struct{ Lat, Lon float64 }
			Costing   string
			Type      string `json:"directions_type"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if r.Method != http.MethodPost || req.Costing != "pedestrian" {
			w.WriteHeader(400)
			return
		}
		switch r.URL.Path {
		case "/sources_to_targets":
			if len(req.Sources) != 1 && len(req.Targets) != 1 {
				w.WriteHeader(400)
				return
			}
			var ds, kms [][]any
			for _, a := range req.Sources {
				var d, k []any
				for _, b := range req.Targets {
					if s, km, ok := fakeWalk(a.Lat, a.Lon, b.Lat, b.Lon, true); ok {
						d, k = append(d, s), append(k, km)
					} else {
						d, k = append(d, nil), append(k, nil)
					}
				}
				ds, kms = append(ds, d), append(kms, k)
			}
			json.NewEncoder(w).Encode(map[string]any{"sources_to_targets": map[string]any{"durations": ds, "distances": kms}, "units": "kilometers"})
		case "/route":
			if len(req.Locations) != 2 || req.Type != "none" {
				w.WriteHeader(400)
				return
			}
			f.routes++
			a, b := req.Locations[0], req.Locations[1]
			s, km, ok := fakeWalk(a.Lat, a.Lon, b.Lat, b.Lon, false)
			if !ok {
				w.WriteHeader(400)
				io.WriteString(w, `{"error_code":442}`)
				return
			}
			fmt.Fprintf(w, `{"trip":{"summary":{"length":%g,"time":%g},"legs":[{"shape":"walk:%.4f,%.4f>%.4f,%.4f"}]}}`, km, s, a.Lat, a.Lon, b.Lat, b.Lon)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func journeyServer(t *testing.T, m *fakeMotis, v *fakeWalks) *Server {
	return testServer(t, nil, func(c *Config) {
		c.JourneyURL = m.srv.URL + "/"
		c.JourneyRegions = []string{"CH", "BG"}
		if v != nil {
			c.RoutingURL = v.srv.URL
		}
	})
}

func offsetsOf(t *testing.T, body map[string]any, side string) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	list, _ := body[side].([]any)
	for _, x := range list {
		o := x.(map[string]any)
		if o["mode"] != "WALK" {
			t.Errorf("%s: mode %v", side, o["mode"])
		}
		out[o["stopId"].(string)] = o["duration"].(float64)
	}
	return out
}

func journeyOf(t *testing.T, s *Server, path string) journeyAnswer {
	t.Helper()
	w := get(s, path)
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body)
	}
	var a journeyAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: %q", w.Header().Get("Cache-Control"))
	}
	return a
}

// a journey: MOTIS is asked with the walk from the start to each stop around it and from each stop around the
// destination, as Valhalla has them; the answer has the rides, the walks with their way, the times
func TestJourney(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	s := journeyServer(t, m, v)
	page := get(s, "/labs/maps").Body.String()
	if !strings.Contains(page, `data-journey="/labs/maps/api/journey"`) || !strings.Contains(page, `data-journey-regions="CH,BG"`) {
		t.Fatalf("the page has no data-journey with its regions")
	}
	a := journeyOf(t, s, "/labs/maps/api/journey?from=47.3779,8.5403&to=46.949,7.4391&time=2026-11-13T09:00:00%2B01:00&arrive=1&lang=de")

	q := m.query
	if q.Get("fromPlace") != "47.377900,8.540300" || q.Get("toPlace") != "46.949000,7.439100" || q.Get("time") != "2026-11-13T08:00:00Z" ||
		q.Get("arriveBy") != "true" || q.Get("language") != "de" || q.Get("numItineraries") != "5" || q.Get("pageCursor") != "" {
		t.Errorf("the question to MOTIS: %v", q)
	}
	from, to := offsetsOf(t, m.body, "fromOffsets"), offsetsOf(t, m.body, "toOffsets")
	// both timetables of the platform with the walk of their place, in whole minutes, the minute begun counted (25 s:
	// one minute; 144 s: three); no stop without a way on foot, none 25 minutes away
	wantB, _, _ := fakeWalk(47.3779, 8.5403, 47.3790, 8.5390, true)
	if len(from) != 4 || from["ch26_A"] != 60 || from["ch27_A"] != 60 || wantB < 121 || wantB > 179 || from["ch26_B"] != 180 || from["ch26_near"] == 0 {
		t.Errorf("fromOffsets: %v", from)
	}
	if len(to) != 1 || to["ch26_bern"] != 60 {
		t.Errorf("toOffsets: %v", to)
	}

	if len(a.Trips) != 2 || a.Earlier != "EARLIER|1" || a.Later != "LATER|2" || a.TZ != "Europe/Zurich" || a.Walk != nil {
		t.Fatalf("answer: %+v", a)
	}
	tr := a.Trips[0]
	if tr.Start != "2026-11-13T08:00:00Z" || tr.End != "2026-11-13T09:10:00Z" || tr.S != 4200 || tr.Changes != 1 || len(tr.Legs) != 5 {
		t.Fatalf("trip: %+v", tr)
	}
	// the walk to the first stop: from the start (no name), its metres and way from Valhalla
	l := tr.Legs[0]
	_, km, _ := fakeWalk(47.3779, 8.5403, 47.3780, 8.5400, true)
	if l.K != "walk" || l.S != 120 || l.From.Name != "" || l.From.At != [2]float64{8.5403, 47.3779} || l.To.Name != "Zürich HB" || l.To.Track != "" ||
		l.M != int(math.Round(km*1000)) || l.Shape != "walk:47.3779,8.5403>47.3780,8.5400" || l.P != 6 {
		t.Errorf("first walk: %+v", l)
	}
	// the train: the line, where it is headed, the colours, the delay (plan), the tracks, the stops between, the alerts
	l = tr.Legs[1]
	if l.K != "rail" || l.Name != "IC1" || l.Head != "Genève-Aéroport" || l.Agency != "SBB" || l.Colour != "#eb0000" || l.Text != "#ffffff" || !l.Live ||
		l.From.T != "2026-11-13T08:05:00Z" || l.From.Plan != "2026-11-13T08:02:00Z" || l.From.Track != "7" || l.To.Plan != "" || l.To.Track != "3" ||
		len(l.Stops) != 1 || l.Stops[0] != (journeyStop{"Olten", "2026-11-13T08:35:00Z"}) || l.Shape != "railshape" || l.P != 6 || l.M != 0 ||
		len(l.Alerts) != 3 || l.Alerts[0] != "Lift out of order in Bern" {
		t.Errorf("train: %+v", l)
	}
	// a change of 67 m: a straight line, no way asked
	l = tr.Legs[2]
	if l.K != "walk" || l.S != 240 || l.From.Name != "Bern" || l.To.Name != "Bern, Bahnhof" || l.Shape != "" || l.M < 55 || l.M > 80 {
		t.Errorf("change: %+v", l)
	}
	// a tram without a display name or colour
	l = tr.Legs[3]
	if l.K != "tram" || l.Name != "7" || l.Colour != "" || l.Text != "" || l.Live || l.From.Plan != "" || l.Shape != "tramshape" {
		t.Errorf("tram: %+v", l)
	}
	// the walk from the last stop to the destination
	l = tr.Legs[4]
	if l.K != "walk" || l.To.Name != "" || l.To.At != [2]float64{7.4391, 46.949} || l.From.Name != "Bern, Zytglogge" || !strings.HasPrefix(l.Shape, "walk:46.9480,7.4480>46.9490,7.4391") {
		t.Errorf("last walk: %+v", l)
	}
	// an itinerary on foot only is none; a cable car, cancelled, in another time zone at its end
	// a change of 225 m between two platforms of one station: a straight line, its ways are not those of the streets
	if l = a.Trips[1].Legs[2]; l.K != "walk" || l.Shape != "" || l.M < 200 || l.M > 250 {
		t.Errorf("change inside a station: %+v", l)
	}
	l = a.Trips[1].Legs[3]
	if len(a.Trips[1].Legs) != 4 || a.Trips[1].Legs[1].Name != "S3" || l.K != "cable" || l.Name != "Polybahn" || !l.Cancelled || l.P != 7 || l.To.TZ != "Europe/Berlin" || l.From.TZ != "" {
		t.Errorf("cable car: %+v", a.Trips[1])
	}
	// the walk to the first stop of both trips is asked once, the change is a straight line: two ways in all
	if v.routes != 2 {
		t.Errorf("%d routes on foot asked, want 2", v.routes)
	}

	// earlier or later: the page of the answer goes to MOTIS
	journeyOf(t, s, "/labs/maps/api/journey?from=47.3779,8.5403&to=46.949,7.4391&page=LATER%7C2")
	if m.query.Get("pageCursor") != "LATER|2" || m.query.Get("arriveBy") != "false" || m.query.Get("language") != "en" {
		t.Errorf("a page: %v", m.query)
	}
	if ts, err := time.Parse(time.RFC3339, m.query.Get("time")); err != nil || time.Since(ts) > time.Minute || time.Since(ts) < -time.Minute {
		t.Errorf("without a time: now, got %q", m.query.Get("time"))
	}
}

// ends near each other: the way on foot is in the answer, and a journey that takes as long is not
func TestJourneyNearEnds(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	s := journeyServer(t, m, v)
	// 780 m in a line: 780 s on foot in the fake; the first itinerary takes 4200 s, the cable car 900 s
	a := journeyOf(t, s, "/labs/maps/api/journey?from=47.3779,8.5403&to=47.3849,8.5403")
	if a.Walk == nil || math.Abs(a.Walk.S-779) > 3 || math.Abs(a.Walk.Km-0.935) > 0.01 || len(a.Trips) != 0 {
		t.Errorf("near ends: walk %+v, %d trips", a.Walk, len(a.Trips))
	}
	// near, and no stop around the start: the way on foot alone, MOTIS is not asked
	m.query = nil
	a = journeyOf(t, s, "/labs/maps/api/journey?from=46.0,9.0&to=46.005,9.0")
	if a.Walk == nil || len(a.Trips) != 0 || m.query != nil {
		t.Errorf("near without stops: %+v, asked %v", a, m.query)
	}
}

// the only stop is far: it is taken with its walk of up to 75 minutes
func TestJourneyFarStop(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	s := journeyServer(t, m, v)
	journeyOf(t, s, "/labs/maps/api/journey?from=46.5,8.0&to=46.949,7.4391")
	from := offsetsOf(t, m.body, "fromOffsets")
	if len(from) != 1 || from["ch26_hut"] < 2400 || from["ch26_hut"] > 2600 {
		t.Errorf("fromOffsets: %v", from)
	}
}

// a start far from any way (a glacier): Valhalla counts the walk from the nearest path, and its route goes another
// way round. The walk takes at least the straight line at walking pace, and is drawn as a straight line
func TestJourneyOffTheWays(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	m.plan = `{"itineraries":[{"duration":3000,"startTime":"2026-11-13T08:00:00Z","endTime":"2026-11-13T08:50:00Z","transfers":0,"legs":[
	 {"mode":"WALK","from":{"name":"START","lat":46.5367,"lon":7.9626,"departure":"2026-11-13T08:00:00Z"},"to":{"name":"Jungfraujoch","lat":46.5476,"lon":7.9824,"arrival":"2026-11-13T08:23:00Z"},"duration":1380,"startTime":"2026-11-13T08:00:00Z","endTime":"2026-11-13T08:23:00Z"},
	 {"mode":"FUNICULAR","from":{"name":"Jungfraujoch","lat":46.5476,"lon":7.9824,"departure":"2026-11-13T08:23:00Z"},"to":{"name":"Bern","lat":46.9490,"lon":7.4395,"arrival":"2026-11-13T08:50:00Z"},"duration":1620,"startTime":"2026-11-13T08:23:00Z","endTime":"2026-11-13T08:50:00Z","displayName":"65"},
	 {"mode":"WALK","from":{"name":"Bern","lat":46.9490,"lon":7.4395,"departure":"2026-11-13T08:50:00Z"},"to":{"name":"END","lat":46.9490,"lon":7.4391,"arrival":"2026-11-13T08:50:00Z"},"duration":30,"startTime":"2026-11-13T08:50:00Z","endTime":"2026-11-13T08:50:00Z"}]}]}`
	s := journeyServer(t, m, v)
	a := journeyOf(t, s, "/labs/maps/api/journey?from=46.5367,7.9626&to=46.949,7.4391")
	from := offsetsOf(t, m.body, "fromOffsets")
	straight := metresBetween(46.5367, 7.9626, 46.5476, 7.9824)
	if from["ch26_joch"] != 60*math.Ceil(straight/1.4/60) {
		t.Errorf("fromOffsets: %v, want the straight line of %.0f m at 1.4 m/s", from, straight)
	}
	if l := a.Trips[0].Legs[0]; l.Shape != "" || math.Abs(float64(l.M)-straight) > 2 {
		t.Errorf("the walk from the glacier: %+v", l)
	}
	if l := a.Trips[0].Legs[2]; !strings.HasPrefix(l.Shape, "walk:") {
		t.Errorf("the walk to the destination: %+v", l)
	}
}

// without Valhalla, and when it fails: the walks in a straight line at walking pace, no way
func TestJourneyStraightWalks(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	v.down = true
	for name, s := range map[string]*Server{"no Valhalla": journeyServer(t, m, nil), "Valhalla down": journeyServer(t, m, v)} {
		a := journeyOf(t, s, "/labs/maps/api/journey?from=47.3779,8.5403&to=46.949,7.4391")
		from := offsetsOf(t, m.body, "fromOffsets")
		// 25 m in a line, 1.3 times as far on foot at 1.1 m/s: 30 s, a minute; the island counts as any stop; the stop
		// 1.1 km away takes 22 minutes
		if len(from) != 6 || from["ch26_A"] != 60 || from["ch26_island"] == 0 || from["ch26_far"] < 1200 || from["ch26_far"] > 1400 {
			t.Errorf("%s: fromOffsets %v", name, from)
		}
		if l := a.Trips[0].Legs[0]; l.Shape != "" || l.M < 25 || l.M > 40 {
			t.Errorf("%s: first walk %+v", name, l)
		}
		if a.Walk != nil {
			t.Errorf("%s: a way on foot: %+v", name, a.Walk)
		}
	}
}

func TestJourneyErrors(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	s := journeyServer(t, m, v)
	errorOf := func(path string) (int, string) {
		t.Helper()
		w := get(s, path)
		var e struct {
			Error struct{ Code, Message string }
		}
		json.Unmarshal(w.Body.Bytes(), &e)
		if e.Error.Message == "" {
			t.Errorf("%s: no message: %s", path, w.Body)
		}
		return w.Code, e.Error.Code
	}
	ok := "/labs/maps/api/journey?from=47.3779,8.5403&to=46.949,7.4391"
	for path, want := range map[string]string{
		"/labs/maps/api/journey?from=47.3779&to=46.949,7.4391": "400 bad_request",
		"/labs/maps/api/journey?from=47.3779,8.5403&to=91,7":   "400 bad_request",
		ok + "&time=tomorrow":             "400 bad_request",
		ok + "&time=2020-01-01T00:00:00Z": "400 bad_request",
		ok + "&time=" + time.Now().Add(400*24*time.Hour).Format("2006-01-02T15:04:05Z"): "400 bad_request",
		ok + "&page=a%20b":                                       "400 bad_request",
		ok + "&page=" + strings.Repeat("x", 5000):                "400 bad_request",
		"/labs/maps/api/journey?from=40.0,9.0&to=46.949,7.4391":  "404 no_stops_from",
		"/labs/maps/api/journey?from=47.3779,8.5403&to=40.0,9.0": "404 no_stops_to",
		"/labs/maps/api/journey?from=0.5,9.0&to=46.949,7.4391":   "502 failed",
	} {
		if code, e := errorOf(path); fmt.Sprintf("%d %s", code, e) != want {
			t.Errorf("%s: %d %s, want %s", path, code, e, want)
		}
	}
	m.plan = `{"itineraries":[]}`
	if code, e := errorOf(ok); code != 404 || e != "no_journey" {
		t.Errorf("no itinerary: %d %s", code, e)
	}
	m.code = 500
	if code, e := errorOf(ok); code != 502 || e != "failed" {
		t.Errorf("MOTIS fails: %d %s", code, e)
	}
	m.code = 400 // a page MOTIS does not know: the fault of the question
	if code, e := errorOf(ok + "&page=old"); code != 400 || e != "bad_request" {
		t.Errorf("an old page: %d %s", code, e)
	}
	if code, e := errorOf(ok); code != 502 || e != "failed" {
		t.Errorf("MOTIS answers 400 without a page: %d %s", code, e)
	}
	m.srv.Close()
	if code, e := errorOf(ok); code != 502 || e != "failed" {
		t.Errorf("no MOTIS: %d %s", code, e)
	}
}

// journeys count against the directions of a visitor, and there are none without JOURNEY_URL
func TestJourneyLimitAndOff(t *testing.T) {
	m, v := newFakeMotis(t), newFakeWalks(t)
	s := testServer(t, nil, func(c *Config) { c.JourneyURL = m.srv.URL; c.RoutingURL = v.srv.URL; c.RoutesPerIP = 2 })
	path := "/labs/maps/api/journey?from=47.3779,8.5403&to=46.949,7.4391"
	for i, want := range []int{200, 200, 429} {
		if w := get(s, path); w.Code != want {
			t.Errorf("request %d: %d, want %d", i+1, w.Code, want)
		}
	}
	off := testServer(t, nil)
	if w := get(off, path); w.Code != 404 {
		t.Errorf("without JOURNEY_URL: %d", w.Code)
	}
	if strings.Contains(get(off, "/labs/maps").Body.String(), "data-journey") {
		t.Errorf("the page has data-journey without JOURNEY_URL")
	}
}
