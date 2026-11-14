package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// More about a place for its page (GET {base}/api/place?osm=node/123&pic=1), from its website and from free pictures near
// it, when OpenStreetMap does not have it:
//
//	menu     a link to the menu (restaurants, cafés, bars), from the home page
//	hours    the opening hours of the home page (JSON-LD), when the place has no opening_hours
//	picture  with pic=1 (the page found no picture of Wikimedia for the place): a photo of Commons near the place that
//	         names it, else the picture the website shows when shared, else a picture of Panoramax looking at the place,
//	         else the logo or icon of the website; served at {base}/api/place/picture
//
// The server looks up the tags of the place in Overpass: it reads only websites that OpenStreetMap gives for a place,
// never an address a browser sends. Answers are kept a week in memory, pictures too.

var osmIDRe = regexp.MustCompile(`^(node|way|relation)/([1-9][0-9]{0,11})$`)

// foodKinds: the places whose menu is looked for
var foodKinds = map[string]bool{"restaurant": true, "cafe": true, "bar": true, "pub": true, "fast_food": true, "biergarten": true,
	"food_court": true, "ice_cream": true}

type placeAnswer struct {
	Web     string        `json:"web,omitempty"`   // the website of the place in OpenStreetMap
	Site    string        `json:"site,omitempty"`  // the website as read, as its host
	Menu    string        `json:"menu,omitempty"`  // a link to the menu
	Hours   string        `json:"hours,omitempty"` // opening hours, in the syntax of OpenStreetMap
	Picture *placePicture `json:"picture,omitempty"`
}

type placePicture struct {
	URL    string `json:"url"`
	Kind   string `json:"kind"` // photo | logo
	Credit string `json:"credit,omitempty"`
	Source string `json:"source"` // Wikimedia Commons, Panoramax, or the host of the website
	Page   string `json:"page"`   // the page of the picture, or the website
}

// placeState: answers and pictures kept, and the lookups running (one per place at a time)
type placeState struct {
	answers  *memo
	short    *memo // answers while a source of pictures failed: asked again within the hour
	pictures *memo
	mu       sync.Mutex
	running  map[string]*placeRun
}

type placeRun struct {
	done chan struct{}
	a    *placeAnswer
	err  error
}

func newPlaceState() *placeState {
	return &placeState{answers: newMemo(16<<20, 7*24*time.Hour), short: newMemo(4<<20, time.Hour), pictures: newMemo(64<<20, 7*24*time.Hour),
		running: map[string]*placeRun{}}
}

func (s *Server) place(w http.ResponseWriter, r *http.Request) {
	m := osmIDRe.FindStringSubmatch(r.URL.Query().Get("osm"))
	if m == nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.placesPerIP.allow(visitor(r), time.Now()) {
		http.Error(w, "too many places", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	a, err := s.placeInfo(ctx, m[0], r.URL.Query().Get("pic") == "1", false)
	if err != nil {
		s.log.Warn("place failed", "osm", m[0], "err", err)
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(a)
}

// placePicture serves the picture an answer names; when it is no longer in memory, the place is looked up again
func (s *Server) placePicture(w http.ResponseWriter, r *http.Request) {
	m := osmIDRe.FindStringSubmatch(r.URL.Query().Get("osm"))
	if m == nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.perIP.allow(visitor(r), time.Now()) {
		http.Error(w, "too many pictures", http.StatusTooManyRequests)
		return
	}
	v, ok := s.places.pictures.get("pic:" + m[0])
	if !ok {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		if _, err := s.placeInfo(ctx, m[0], true, true); err != nil {
			http.Error(w, "upstream", http.StatusBadGateway)
			return
		}
		v, ok = s.places.pictures.get("pic:" + m[0])
	}
	typ, b, cut := strings.Cut(string(v), "\n")
	if !ok || !cut || !pictureTypes[typ] {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("Cache-Control", "public, max-age=604800")
	io.WriteString(w, b)
}

// placeInfo is the answer for a place, from memory or looked up (once at a time per place); again: look up even when
// in memory (its picture was dropped)
func (s *Server) placeInfo(ctx context.Context, osm string, pic, again bool) (*placeAnswer, error) {
	key := fmt.Sprintf("%s:%t", osm, pic)
	for _, m := range []*memo{s.places.answers, s.places.short} {
		if v, ok := m.get(key); ok && !again {
			var a placeAnswer
			if json.Unmarshal(v, &a) == nil {
				return &a, nil
			}
		}
	}
	ps := s.places
	ps.mu.Lock()
	if run, ok := ps.running[key]; ok {
		ps.mu.Unlock()
		select {
		case <-run.done:
			return run.a, run.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	run := &placeRun{done: make(chan struct{})}
	ps.running[key] = run
	ps.mu.Unlock()
	defer func() {
		ps.mu.Lock()
		delete(ps.running, key)
		ps.mu.Unlock()
		close(run.done)
	}()
	select {
	case s.placeCalls <- struct{}{}:
		defer func() { <-s.placeCalls }()
	case <-ctx.Done():
		run.err = ctx.Err()
		return nil, run.err
	}
	var failed bool
	run.a, failed, run.err = s.lookUpPlace(ctx, osm, pic)
	if run.err == nil {
		if v, err := json.Marshal(run.a); err == nil {
			if failed {
				ps.short.put(key, v)
			} else {
				ps.answers.put(key, v)
			}
		}
	}
	return run.a, run.err
}

// osmPlace is what Overpass gives of a place
type osmPlace struct {
	Tags     map[string]string
	Lat, Lon float64
}

// placeTags looks up the tags and the middle of a place in Overpass; nil when it does not exist (any more)
func (s *Server) placeTags(ctx context.Context, osm string) (*osmPlace, error) {
	if v, ok := s.cache.get("tags:" + osm); ok {
		if len(v) == 0 {
			return nil, nil
		}
		var p osmPlace
		if json.Unmarshal(v, &p) == nil {
			return &p, nil
		}
	}
	typ, id, _ := strings.Cut(osm, "/")
	resp, err := s.routeClient.Do(s.overpassRequest(ctx, fmt.Sprintf("[out:json][timeout:15];%s(%s);out tags center;", typ, id)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass: HTTP %d", resp.StatusCode)
	}
	var d struct {
		Elements []struct {
			Lat    float64                     `json:"lat"`
			Lon    float64                     `json:"lon"`
			Center *struct{ Lat, Lon float64 } `json:"center"`
			Tags   map[string]string           `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&d); err != nil {
		return nil, err
	}
	if len(d.Elements) == 0 {
		s.cache.put("tags:"+osm, nil)
		return nil, nil
	}
	e := d.Elements[0]
	p := &osmPlace{Tags: e.Tags, Lat: e.Lat, Lon: e.Lon}
	if e.Center != nil {
		p.Lat, p.Lon = e.Center.Lat, e.Center.Lon
	}
	if v, err := json.Marshal(p); err == nil {
		s.cache.put("tags:"+osm, v)
	}
	return p, nil
}

// nearTime: Commons and Panoramax get at most this long (the menu and the hours of a website wait for them)
const nearTime = 5 * time.Second

// lookUpPlace reads the website of a place and, for a picture, Commons and Panoramax around it, at the same time; failed:
// Commons or Panoramax did not answer (the answer is kept an hour only)
func (s *Server) lookUpPlace(ctx context.Context, osm string, pic bool) (*placeAnswer, bool, error) {
	p, err := s.placeTags(ctx, osm)
	if err != nil || p == nil {
		return &placeAnswer{}, false, err
	}
	t := p.Tags
	web := webURL(t["website"])
	if web == nil {
		web = webURL(t["contact:website"])
	}
	var (
		wg    sync.WaitGroup
		page  *sitePage
		final *url.URL
		file  string
		pano  *panoPick
		fails atomic.Int32
	)
	if web != nil {
		wg.Add(1)
		go func() { defer wg.Done(); page, final = s.readSite(ctx, web) }()
	}
	if pic && (p.Lat != 0 || p.Lon != 0) {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, nearTime)
			defer cancel()
			f, err := s.commonsNear(c, t["name"], p.Lat, p.Lon)
			if err != nil {
				fails.Add(1)
			}
			file = f
		}()
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, nearTime)
			defer cancel()
			pk, err := s.panoramaxNear(c, p.Lat, p.Lon)
			if err != nil {
				fails.Add(1)
			}
			pano = pk
		}()
	}
	wg.Wait()

	a := &placeAnswer{}
	if web != nil {
		a.Web = web.String()
	}
	if page != nil {
		a.Site = strings.TrimPrefix(final.Hostname(), "www.")
		if foodKinds[t["amenity"]] && page.Menu != "" {
			a.Menu = page.Menu
		}
		if t["opening_hours"] == "" && page.Hours != "" {
			a.Hours = page.Hours
		}
	}
	failed := fails.Load() > 0
	if !pic {
		return a, failed, nil
	}
	const width = 500
	keep := func(w *webPicture, pp placePicture) {
		sum := sha256.Sum256(w.Bytes)
		pp.URL = fmt.Sprintf("%s/api/place/picture?osm=%s&v=%s", strings.TrimRight(s.cfg.Base, "/"), osm, hex.EncodeToString(sum[:4]))
		s.places.pictures.put("pic:"+osm, append([]byte(w.Type+"\n"), w.Bytes...))
		a.Picture = &pp
	}
	// a photo of Commons that names the place
	if file != "" {
		if th, err := s.thumbnail(ctx, file, width); err == nil && th != nil {
			keep(&webPicture{Type: th.Type, Bytes: th.Bytes}, placePicture{Kind: "photo", Credit: th.Credit, Source: "Wikimedia Commons", Page: th.Page})
			return a, failed, nil
		}
	}
	// the picture of the website, when a photo; its logo kept for last
	var logo *webPicture
	if page != nil {
		tries := 0
		for _, sp := range page.Pictures {
			if tries >= 4 || (sp.Logo && logo != nil) {
				continue
			}
			tries++
			w := s.webPicture(ctx, sp.URL, width, map[bool]int{false: 200, true: 96}[sp.Logo])
			if w == nil {
				continue
			}
			if !w.Logo && !sp.Logo {
				keep(w, placePicture{Kind: "photo", Source: a.Site, Page: final.String()})
				return a, failed, nil
			}
			if logo == nil {
				logo = w
			}
		}
	}
	// Panoramax
	if pano != nil {
		if w, err := s.panoPicture(ctx, pano, width); err == nil && w != nil {
			keep(w, placePicture{Kind: "photo", Credit: pano.Credit, Source: "Panoramax", Page: "https://api.panoramax.xyz/#focus=pic&pic=" + url.QueryEscape(pano.ID)})
			return a, failed, nil
		}
	}
	if logo != nil {
		logo.Logo = true
		keep(logo, placePicture{Kind: "logo", Source: a.Site, Page: final.String()})
	}
	return a, failed, nil
}

// readSite reads the home page of a place, as robots.txt of its site allows; nil when it cannot. What it read is kept a
// day: the page of a place asks without a picture, then with one when Wikimedia has none.
func (s *Server) readSite(ctx context.Context, u *url.URL) (*sitePage, *url.URL) {
	type kept struct {
		Page  *sitePage
		Final string
	}
	key := "site:" + u.String()
	if v, ok := s.cache.get(key); ok {
		var k kept
		if json.Unmarshal(v, &k) == nil && k.Page != nil {
			if f, err := url.Parse(k.Final); err == nil {
				return k.Page, f
			}
		}
		return nil, nil
	}
	p, final := s.fetchPage(ctx, u)
	k := kept{Page: p}
	if final != nil {
		k.Final = final.String()
	}
	if v, err := json.Marshal(k); err == nil {
		s.cache.put(key, v)
	}
	return p, final
}

func (s *Server) fetchPage(ctx context.Context, u *url.URL) (*sitePage, *url.URL) {
	if !s.robotsAllow(ctx, u) {
		return nil, nil
	}
	body, typ, final, err := s.fetchSite(ctx, u.String(), 1_500_000, "text/html, application/xhtml+xml")
	if err != nil || (typ != "text/html" && typ != "application/xhtml+xml") {
		return nil, nil
	}
	if final.Host != u.Host && !s.robotsAllow(ctx, final) {
		return nil, nil
	}
	p := readPage(string(body), final)
	return &p, final
}

// webPicture fetches a picture of a website (as its robots.txt allows) and writes it again; nil when it is not one to
// show
func (s *Server) webPicture(ctx context.Context, u string, width, minW int) *webPicture {
	pu, err := url.Parse(u)
	if err != nil || !s.robotsAllow(ctx, pu) {
		return nil
	}
	b, _, _, err := s.fetchSite(ctx, u, 5<<20, "image/jpeg, image/png, image/webp, image/gif")
	if err != nil {
		return nil
	}
	w, err := readPicture(b, width, minW)
	if err != nil {
		return nil
	}
	return w
}

// placeExtras: more about places, with the pictures and an Overpass to look up their tags
func (s *Server) placeExtras() bool { return s.cfg.Pictures && s.overpass() }

// placeWindow: places a visitor may look up in a window (each page of a place opened is one)
func placeWindow(cfg Config) *window {
	n := cfg.SearchesPerIP * 3
	if n <= 0 {
		n = 300
	}
	return newWindow(n, cfg.RouteWindow)
}
