package server

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeWeb: Overpass with the places of the tests, the website of a restaurant, and Panoramax
type fakeWeb struct {
	overpass, site, pano     *httptest.Server
	siteCalls, overpassCalls atomic.Int32
}

func newFakeWeb(t *testing.T) *fakeWeb {
	f := &fakeWeb{}
	photo, logo := encJPEG(t, testPhoto(1200, 630)), encPNG(t, testLogo(400, 400, color.NRGBA{200, 0, 0, 255}, color.NRGBA{255, 255, 255, 255}))
	f.site = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.siteCalls.Add(1)
		switch r.URL.Path {
		case "/robots.txt":
			io.WriteString(w, "User-agent: *\nDisallow: /closed\n")
		case "/", "/shop/", "/closed/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			share := "/share.jpg"
			if r.URL.Path == "/shop/" {
				share = "/logo-share.png" // a logo as the picture of a share: Panoramax first, the logo last
			}
			fmt.Fprintf(w, `<html><head><meta property="og:image" content="%s"><link rel="apple-touch-icon" href="/apple.png">
<script type="application/ld+json">{"@type":"Restaurant","address":"x","openingHoursSpecification":[{"dayOfWeek":["Monday","Tuesday"],"opens":"11:00","closes":"22:00"}]}</script>
</head><body><a href="/karte.pdf">Speisekarte</a></body></html>`, share)
		case "/share.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(photo)
		case "/apple.png", "/logo-share.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(logo)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.site.Close)
	f.pano = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/search":
			// three pictures around 47.1, 8.1: one too far, one looking away, one 15 m south looking north at the place
			fmt.Fprintf(w, `{"features":[
{"id":"far","geometry":{"coordinates":[8.1,47.0995]},"properties":{"view:azimuth":0,"pers:interior_orientation":{"field_of_view":90}},"assets":{"thumb":{"href":"%[1]s/far.jpg"},"sd":{"href":"%[1]s/far.jpg"}}},
{"id":"away","geometry":{"coordinates":[8.1,47.09987]},"properties":{"view:azimuth":180,"pers:interior_orientation":{"field_of_view":90}},"assets":{"thumb":{"href":"%[1]s/away.jpg"},"sd":{"href":"%[1]s/away.jpg"}}},
{"id":"look","geometry":{"coordinates":[8.1,47.099865]},"properties":{"view:azimuth":5,"pers:interior_orientation":{"field_of_view":90},"license":"CC-BY-SA-4.0","geovisio:producer":"anna","datetime":"2025-05-01T10:00:00Z"},"assets":{"thumb":{"href":"%[1]s/look.jpg"},"sd":{"href":"%[1]s/look.jpg"}}}]}`, f.pano.URL)
		case r.URL.Path == "/look.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(encJPEG(t, testPhoto(500, 375)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.pano.Close)
	f.overpass = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.overpassCalls.Add(1)
		r.ParseForm()
		q := r.PostForm.Get("data")
		tags := map[string]string{}
		lat, lon := 0.0, 0.0
		switch {
		case strings.Contains(q, "node(1)"): // a restaurant with a website, without opening hours
			tags = map[string]string{"amenity": "restaurant", "name": "Wolf", "website": f.site.URL + "/"}
			lat, lon = 46.0, 7.0
		case strings.Contains(q, "node(2)"): // a shop with a photo on Commons
			tags = map[string]string{"shop": "clothes", "name": "Zebra Mode"}
			lat, lon = 47.0, 8.0
		case strings.Contains(q, "node(3)"): // a site that robots.txt closes
			tags = map[string]string{"amenity": "cafe", "name": "Closed", "website": f.site.URL + "/closed/"}
		case strings.Contains(q, "way(4)"): // a shop with hours, its logo shared, Panoramax looking at it
			tags = map[string]string{"shop": "bakery", "name": "Brot", "website": f.site.URL + "/shop/", "opening_hours": "Mo-Sa 07:00-18:00"}
			lat, lon = 47.1, 8.1
		default:
			io.WriteString(w, `{"elements":[]}`)
			return
		}
		b, _ := json.Marshal(tags)
		if strings.HasPrefix(q, "[out:json]") && strings.Contains(q, "way(") {
			fmt.Fprintf(w, `{"elements":[{"type":"way","center":{"lat":%v,"lon":%v},"tags":%s}]}`, lat, lon, b)
			return
		}
		fmt.Fprintf(w, `{"elements":[{"type":"node","lat":%v,"lon":%v,"tags":%s}]}`, lat, lon, b)
	}))
	t.Cleanup(f.overpass.Close)
	return f
}

func placeServer(t *testing.T, f *fakeWeb, wm *fakeWikimedia) *Server {
	s := testServer(t, wm, func(c *Config) { c.OverpassURL = f.overpass.URL; c.PanoramaxAPI = f.pano.URL })
	s.siteClient = newSiteClient(func(netip.AddrPort) bool { return true }) // the fakes are on this machine
	return s
}

func placeGet(t *testing.T, s *Server, q string) placeAnswer {
	t.Helper()
	w := get(s, "/labs/maps/api/place?"+q)
	var a placeAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &a) != nil {
		t.Fatalf("%s: %d %s", q, w.Code, w.Body.String())
	}
	return a
}

func TestPlace(t *testing.T) {
	f, wm := newFakeWeb(t), newFake(t)
	s := placeServer(t, f, wm)
	if w := get(s, "/labs/maps"); !strings.Contains(w.Body.String(), `data-place="/labs/maps/api/place"`) {
		t.Fatal("the page has no data-place")
	}
	host := strings.TrimPrefix(f.site.URL, "http://")
	host, _, _ = strings.Cut(host, ":")

	// the restaurant: its menu, the hours of its website, its shared picture as the photo
	a := placeGet(t, s, "osm=node/1&pic=1")
	if a.Menu != f.site.URL+"/karte.pdf" || a.Hours != "Mo,Tu 11:00-22:00" || a.Site != host || a.Web != f.site.URL+"/" {
		t.Errorf("restaurant: %+v", a)
	}
	if a.Picture == nil || a.Picture.Kind != "photo" || a.Picture.Source != host || a.Picture.Page != f.site.URL+"/" ||
		!strings.HasPrefix(a.Picture.URL, "/labs/maps/api/place/picture?osm=node/1&v=") {
		t.Fatalf("restaurant picture: %+v", a.Picture)
	}
	w := get(s, a.Picture.URL)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") || w.Body.Len() < 1000 {
		t.Fatalf("the picture: %d %v %d bytes", w.Code, w.Header(), w.Body.Len())
	}
	// kept: asked again, no new reads of the site; without pic, the same menu and hours
	n := f.siteCalls.Load()
	placeGet(t, s, "osm=node/1&pic=1")
	if f.siteCalls.Load() != n {
		t.Errorf("read the site again: %d calls", f.siteCalls.Load()-n)
	}
	if a := placeGet(t, s, "osm=node/1&pic=0"); a.Picture != nil || a.Menu == "" {
		t.Errorf("without a picture: %+v", a)
	}
	if f.siteCalls.Load() != n {
		t.Errorf("the home page read again for the answer without a picture: %d calls", f.siteCalls.Load()-n)
	}

	// a shop without a website: the photo of Commons that names it (not the closer file of another building)
	a = placeGet(t, s, "osm=node/2&pic=1")
	if a.Picture == nil || a.Picture.Source != "Wikimedia Commons" || a.Picture.Credit != "Ann & Bo, CC BY-SA 4.0" ||
		!strings.Contains(a.Picture.Page, "Zebra Mode, front.jpg") || a.Menu != "" || a.Site != "" {
		t.Errorf("shop: %+v %+v", a, a.Picture)
	}

	// robots.txt closes the site: nothing of it
	if a := placeGet(t, s, "osm=node/3&pic=1"); a.Site != "" || a.Menu != "" || a.Hours != "" || a.Picture != nil || a.Web != f.site.URL+"/closed/" {
		t.Errorf("closed site: %+v", a)
	}

	// a bakery with hours of its own, its logo shared: no hours of the site, no menu (not a place to eat), the picture of
	// Panoramax looking at it
	a = placeGet(t, s, "osm=way/4&pic=1")
	if a.Hours != "" || a.Menu != "" || a.Picture == nil || a.Picture.Source != "Panoramax" || a.Picture.Credit != "anna, CC BY-SA 4.0" ||
		a.Picture.Page != "https://api.panoramax.xyz/#focus=pic&pic=look" {
		t.Errorf("bakery: %+v %+v", a, a.Picture)
	}
	// without Panoramax the logo of the site
	s2 := placeServer(t, f, wm)
	s2.cfg.PanoramaxAPI = ""
	if a := placeGet(t, s2, "osm=way/4&pic=1"); a.Picture == nil || a.Picture.Kind != "logo" || a.Picture.Source != host {
		t.Errorf("bakery without Panoramax: %+v", a.Picture)
	}
	if w := get(s2, "/labs/maps/api/place/picture?osm=way/4"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("the logo: %d %s", w.Code, w.Header().Get("Content-Type"))
	}

	// a place that does not exist; a bad id; a picture no longer in memory is looked up again
	if a := placeGet(t, s, "osm=node/99&pic=1"); a != (placeAnswer{}) {
		t.Errorf("no place: %+v", a)
	}
	for _, q := range []string{"osm=node/0", "osm=foo/1", "osm=node/1/2", "osm=" + url.QueryEscape("node/1 "), ""} {
		if w := get(s, "/labs/maps/api/place?"+q); w.Code != 400 {
			t.Errorf("%q: %d", q, w.Code)
		}
	}
	s.places.pictures = newMemo(1<<20, time.Hour)
	if w := get(s, "/labs/maps/api/place/picture?osm=node/1"); w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("the picture looked up again: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/place/picture?osm=node/3"); w.Code != 404 {
		t.Errorf("no picture: %d", w.Code)
	}
}

// without Overpass no lookups: no route, no data-place
func TestPlaceOff(t *testing.T) {
	s := testServer(t, nil, func(c *Config) { c.OverpassURL = ""; c.ElevationKey = "" })
	if w := get(s, "/labs/maps/api/place?osm=node/1"); w.Code != 404 {
		t.Errorf("place without Overpass: %d", w.Code)
	}
	if w := get(s, "/labs/maps"); strings.Contains(w.Body.String(), "data-place") {
		t.Error("data-place without Overpass")
	}
}
