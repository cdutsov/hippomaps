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

// Search along a route: the places of a kind near the line of a route (fuel, charging, food…),
// from Overpass (OverpassURL, or the Overpass API of overpass.ch with ElevationKey).
//
//	POST {base}/api/along   {"line":[[lon,lat],…],"cat":"fuel","mode":"car"}
//
// answers GeoJSON points in the order of the route, at most alongMax: properties id, name, type (and kind: the name of the type in lang, de, fr or it)
// (key=value), tags, opening_hours, at (km from the start of the line) and off (metres from the
// line); total, the places found. With more than alongMax, those kept are spread over the route
// (alongBins stretches, the nearest to the line of each first), not the first towns only. 400 bad_request, 429 too_many, 503 busy, 502 failed, as {"error":{"code","message"}}.

// alongCats are the kinds of places, as Overpass filters.
var alongCats = map[string]string{
	"fuel":      `["amenity"="fuel"]`,
	"charging":  `["amenity"="charging_station"]["motorcar"!="no"]`, // not those of bikes alone
	"rest":      `["highway"~"^(services|rest_area)$"]`,
	"food":      `["amenity"~"^(restaurant|fast_food)$"]`,
	"cafe":      `["amenity"="cafe"]`,
	"groceries": `["shop"~"^(supermarket|convenience)$"]`,
	"toilets":   `["amenity"="toilets"]`,
	"water":     `["amenity"="drinking_water"]`,
	"hotel":     `["tourism"~"^(hotel|motel|guest_house|hostel|alpine_hut)$"]`,
	"bike":      `["shop"="bicycle"]`,
	"pharmacy":  `["amenity"="pharmacy"]`,
}

// alongRadius is how far from the line a place may be, by mode (metres).
var alongRadius = map[string]float64{"car": 500, "bike": 250, "walk": 150}

const (
	alongMax       = 200   // places in an answer
	alongBins      = 40    // stretches of the route that share alongMax
	alongMaxPoints = 60000 // points of a line asked (1500 km, a point every 25 m)
	alongQueryPts  = 600   // points of the line in the query to Overpass, at most
)

// alongTags are the tags a place keeps in the answer.
var alongTags = []string{"name", "brand", "operator", "cuisine", "opening_hours", "addr:street", "addr:housenumber", "addr:place",
	"addr:postcode", "addr:city", "addr:country", "website", "contact:website", "phone", "contact:phone", "wikidata", "brand:wikidata",
	"image", "wikimedia_commons", "fee", "capacity", "socket:type2", "socket:type2_combo", "socket:chademo", "fuel:diesel", "fuel:lpg",
	"fuel:octane_95", "fuel:octane_98", "stars", "wheelchair", "access", "self_service", "description"}

type alongPlace struct {
	Type       string         `json:"type"`
	Geometry   alongPoint     `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

type alongPoint struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

func (s *Server) along(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Line [][2]float64 `json:"line"`
		Cat  string       `json:"cat"`
		Mode string       `json:"mode"`
		Lang string       `json:"lang"` // the kind of each place in this language (labels.go)
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 3<<20)).Decode(&req); err != nil {
		routeError(w, http.StatusBadRequest, "bad_request", "A JSON body: line, cat and mode.")
		return
	}
	filter, okc := alongCats[req.Cat]
	radius, okm := alongRadius[req.Mode]
	if !okc || !okm || len(req.Line) < 2 || len(req.Line) > alongMaxPoints {
		routeError(w, http.StatusBadRequest, "bad_request", "line as [[lon,lat],…], a known cat, and mode car, bike or walk")
		return
	}
	for _, p := range req.Line {
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) || p[0] < -180 || p[0] > 180 || p[1] < -90 || p[1] > 90 {
			routeError(w, http.StatusBadRequest, "bad_request", "line as [[lon,lat],…]")
			return
		}
	}
	if !s.routesPerIP.allow(clientIP(r), time.Now()) {
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many searches in a short time. Please wait a few minutes.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	select {
	case s.transitCalls <- struct{}{}:
		defer func() { <-s.transitCalls }()
	case <-time.After(5 * time.Second):
		routeError(w, http.StatusServiceUnavailable, "busy", "The search is busy. Please try again.")
		return
	case <-ctx.Done():
		return
	}
	places, total, err := s.alongQuery(ctx, req.Line, filter, radius)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("along", "err", err)
		}
		routeError(w, http.StatusBadGateway, "failed", "The search failed. Please try again.")
		return
	}
	if i := langs[req.Lang]; i != 0 {
		for _, p := range places {
			if t, _ := p.Properties["type"].(string); t != "" {
				if n := presetName(strings.Replace(t, "=", "/", 1), i); n != "" {
					p.Properties["kind"] = n
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": places, "total": total})
}

func (s *Server) alongQuery(ctx context.Context, line [][2]float64, filter string, radius float64) ([]alongPlace, int, error) {
	pts, _ := simplifyTo(line, 30, alongQueryPts)
	var coords strings.Builder
	for i, p := range pts {
		if i > 0 {
			coords.WriteByte(',')
		}
		fmt.Fprintf(&coords, "%.5f,%.5f", p[1], p[0])
	}
	q := fmt.Sprintf(`[out:json][timeout:25];nwr%s(around:%d,%s);out tags center;`, filter, int(radius), coords.String())
	req := s.overpassRequest(ctx, q)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("overpass: HTTP %d", resp.StatusCode)
	}
	var d struct {
		Elements []struct {
			Type   string                      `json:"type"`
			ID     int64                       `json:"id"`
			Lat    *float64                    `json:"lat"`
			Lon    *float64                    `json:"lon"`
			Center *struct{ Lat, Lon float64 } `json:"center"`
			Tags   map[string]string           `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&d); err != nil {
		return nil, 0, err
	}
	cum := cumKm(line)
	out := []alongPlace{}
	for _, e := range d.Elements {
		var ll [2]float64
		switch {
		case e.Lat != nil && e.Lon != nil:
			ll = [2]float64{*e.Lon, *e.Lat}
		case e.Center != nil:
			ll = [2]float64{e.Center.Lon, e.Center.Lat}
		default:
			continue
		}
		at, off := project(line, cum, ll)
		if off > radius*1.5 { // a large area whose edge is near, its centre far
			continue
		}
		props := map[string]any{"id": e.Type + "/" + strconv.FormatInt(e.ID, 10), "at": round(at, 2), "off": math.Round(off)}
		tags := map[string]string{}
		for _, k := range alongTags {
			if v := e.Tags[k]; v != "" {
				tags[k] = v
			}
		}
		if v := e.Tags["name"]; v != "" {
			props["name"] = v
		}
		if v := e.Tags["opening_hours"]; v != "" {
			props["opening_hours"] = v
			delete(tags, "opening_hours")
		}
		for _, k := range []string{"amenity", "shop", "tourism", "highway"} {
			if v := e.Tags[k]; v != "" {
				props["type"] = k + "=" + v
				break
			}
		}
		props["tags"] = tags
		out = append(out, alongPlace{Type: "Feature", Geometry: alongPoint{Type: "Point", Coordinates: [2]float64{round(ll[0], 6), round(ll[1], 6)}}, Properties: props})
	}
	total := len(out)
	out = spread(out, cum[len(cum)-1])
	sort.SliceStable(out, func(i, j int) bool { return out[i].Properties["at"].(float64) < out[j].Properties["at"].(float64) })
	return out, total, nil
}

// spread keeps alongMax places of a route of km: in turn one of each stretch of the route, the
// nearest to the line first.
func spread(ps []alongPlace, km float64) []alongPlace {
	if len(ps) <= alongMax {
		return ps
	}
	bins := make([][]alongPlace, alongBins)
	for _, p := range ps {
		b := int(p.Properties["at"].(float64) / km * alongBins)
		b = max(0, min(alongBins-1, b))
		bins[b] = append(bins[b], p)
	}
	for _, b := range bins {
		sort.SliceStable(b, func(i, j int) bool { return b[i].Properties["off"].(float64) < b[j].Properties["off"].(float64) })
	}
	out := make([]alongPlace, 0, alongMax)
	for k := 0; len(out) < alongMax; k++ {
		for _, b := range bins {
			if k < len(b) && len(out) < alongMax {
				out = append(out, b[k])
			}
		}
	}
	return out
}

// flatXY is a point in metres on a plane; kx is the cosine of the latitude of the plane.
func flatXY(p [2]float64, kx float64) (float64, float64) {
	return p[0] * 111320 * kx, p[1] * 110540
}

// cumKm is the distance along line at each of its points.
func cumKm(line [][2]float64) []float64 {
	cum := make([]float64, len(line))
	for i := 1; i < len(line); i++ {
		cum[i] = cum[i-1] + kmBetween(line[i-1][1], line[i-1][0], line[i][1], line[i][0])
	}
	return cum
}

// project finds the point of line nearest to p: how far along the line it is (km), and how far p
// is from it (metres).
func project(line [][2]float64, cum []float64, p [2]float64) (at, off float64) {
	off = math.Inf(1)
	kx := math.Cos(p[1] * math.Pi / 180)
	px, py := flatXY(p, kx)
	for i := 0; i+1 < len(line); i++ {
		ax, ay := flatXY(line[i], kx)
		bx, by := flatXY(line[i+1], kx)
		dx, dy := bx-ax, by-ay
		t := 0.0
		if l2 := dx*dx + dy*dy; l2 > 0 {
			t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
		}
		if d := math.Hypot(ax+t*dx-px, ay+t*dy-py); d < off {
			off, at = d, cum[i]+(cum[i+1]-cum[i])*t
		}
	}
	return at, off
}

// simplifyTo is line with at most n points: simplified with the tolerance tol (metres), doubled until they
// are as few; the tolerance used comes back. A line that no tolerance brings to n points (simplifyRun keeps
// points of a very long one) gives n of its points at even steps.
func simplifyTo(line [][2]float64, tol float64, n int) ([][2]float64, float64) {
	pts := simplify(line, tol)
	for len(pts) > n && tol < 1e8 { // beyond the size of the earth every point is within the tolerance
		tol *= 2
		pts = simplify(line, tol)
	}
	if len(pts) > n {
		out := make([][2]float64, n)
		for i := range out {
			out[i] = pts[i*(len(pts)-1)/(n-1)]
		}
		pts = out
	}
	return pts, tol
}

// simplifyRun is the longest run of points simplify looks at as one: a longer run is halved and its middle
// point kept. Douglas-Peucker looks at every point of a run for each point it keeps, so a line that keeps
// most of its points (a zigzag, which a visitor may send to along) takes the square of its length in time;
// in runs it takes at most simplifyRun times its length. A line of a route keeps a few points more.
const simplifyRun = 512

// simplify drops the points of line that are less than tol metres off the line through the others
// (Douglas-Peucker, in runs of at most simplifyRun points).
func simplify(line [][2]float64, tol float64) [][2]float64 {
	if len(line) < 3 {
		return line
	}
	lat0 := math.Cos(line[len(line)/2][1] * math.Pi / 180)
	keep := make([]bool, len(line))
	keep[0], keep[len(line)-1] = true, true
	stack := [][2]int{{0, len(line) - 1}}
	for len(stack) > 0 {
		ij := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		i, j := ij[0], ij[1]
		if j-i > simplifyRun {
			mid := (i + j) / 2
			keep[mid] = true
			stack = append(stack, [2]int{i, mid}, [2]int{mid, j})
			continue
		}
		ax, ay := flatXY(line[i], lat0)
		bx, by := flatXY(line[j], lat0)
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		best, bestD := -1, 0.0
		for k := i + 1; k < j; k++ {
			px, py := flatXY(line[k], lat0)
			t := 0.0
			if l2 > 0 {
				t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
			}
			if d := math.Hypot(ax+t*dx-px, ay+t*dy-py); d > bestD {
				best, bestD = k, d
			}
		}
		if bestD > tol {
			keep[best] = true
			stack = append(stack, [2]int{i, best}, [2]int{best, j})
		}
	}
	out := make([][2]float64, 0, 64)
	for i, k := range keep {
		if k {
			out = append(out, line[i])
		}
	}
	return out
}
