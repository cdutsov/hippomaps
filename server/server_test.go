package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var png = []byte("\x89PNG\r\n\x1a\nfake")

// fakeWikimedia answers wbgetclaims, imageinfo and the thumbnails.
type fakeWikimedia struct {
	srv   *httptest.Server
	calls atomic.Int32
	svg   bool // thumbnails come as SVG
}

func newFake(t *testing.T) *fakeWikimedia {
	f := &fakeWikimedia{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/wd" && q.Get("action") == "query" && q.Get("list") == "search":
			if q.Get("srsearch") == "haswbstatement:P402=1682682" { // the relation of Thun
				io.WriteString(w, `{"query":{"search":[{"title":"Q1"}]}}`)
				return
			}
			io.WriteString(w, `{"query":{"search":[]}}`)
		case r.URL.Path == "/wd" && q.Get("action") == "wbgetclaims":
			item, prop := q.Get("entity"), q.Get("property")
			switch {
			case item == "Q38076" && prop == "P154": // a logo, with a deprecated older one
				io.WriteString(w, `{"claims":{"P154":[{"rank":"deprecated","mainsnak":{"datavalue":{"value":"Old logo.svg"}}},{"rank":"normal","mainsnak":{"datavalue":{"value":"Logo.svg"}}}]}}`)
			case item == "Q1" && prop == "P18":
				io.WriteString(w, `{"claims":{"P18":[{"rank":"normal","mainsnak":{"datavalue":{"value":"Photo.jpg"}}}]}}`)
			default:
				io.WriteString(w, `{"claims":{}}`)
			}
		case r.URL.Path == "/commons":
			file := strings.TrimPrefix(q.Get("titles"), "File:")
			if file == "Missing.jpg" {
				io.WriteString(w, `{"query":{"pages":[{"missing":true}]}}`)
				return
			}
			io.WriteString(w, `{"query":{"pages":[{"imageinfo":[{"thumburl":"`+f.srv.URL+`/thumb/`+url.PathEscape(file)+`","descriptionurl":"https://commons.wikimedia.org/wiki/File:`+file+`","extmetadata":{"Artist":{"value":"<a href=\"x\">Ann &amp; Bo</a><span>Ann &amp; Bo</span>"},"LicenseShortName":{"value":"CC BY-SA 4.0"}}}]}]}}`)
		case strings.HasPrefix(r.URL.Path, "/thumb/"):
			if f.svg {
				w.Header().Set("Content-Type", "image/svg+xml")
				io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func testServer(t *testing.T, f *fakeWikimedia, opts ...func(*Config)) *Server {
	cfg := DefaultConfig()
	cfg.MapsKey = "pk_test"
	cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	if f != nil {
		u, _ := url.Parse(f.srv.URL)
		cfg.WikidataAPI, cfg.CommonsAPI, cfg.ThumbHosts = f.srv.URL+"/wd", f.srv.URL+"/commons", []string{u.Host}
	}
	for _, o := range opts {
		o(&cfg)
	}
	s := New(cfg)
	if f != nil {
		s.client = f.srv.Client()
	}
	return s
}

func get(h http.Handler, u string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, u, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPage(t *testing.T) {
	s := testServer(t, nil)
	w := get(s, "/labs/maps")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `data-key="pk_test"`) || !strings.Contains(body, `data-api="/labs/search/api"`) ||
		!strings.Contains(body, `data-pictures="/labs/maps/api/picture"`) || !strings.Contains(body, `/labs/maps/static/app.js?v=`) {
		t.Fatalf("page: %d %s", w.Code, body)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https://api.overpass.ch") || !strings.Contains(csp, "img-src 'self' data: blob: https://api.overpass.ch") || strings.Contains(csp, "unsafe") {
		t.Errorf("csp: %s", csp)
	}
	if w.Header().Get("X-Robots-Tag") != "noindex" || !strings.Contains(w.Header().Get("Permissions-Policy"), "geolocation=(self)") {
		t.Errorf("headers: %v", w.Header())
	}
	if w := get(s, "/labs/maps/"); w.Code != http.StatusMovedPermanently {
		t.Errorf("slash: %d", w.Code)
	}
}

func TestStatic(t *testing.T) {
	s := testServer(t, nil)
	v := s.hashes["app.js"]
	w := get(s, "/labs/maps/static/app.js?v="+v)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("app.js: %d %v", w.Code, w.Header())
	}
	if w := get(s, "/labs/maps/static/app.js?v=old"); w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("old hash: %v", w.Header())
	}
	if w := get(s, "/labs/maps/static/vendor/maplibre-gl-6.13.0/maplibre-gl.mjs"); w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("vendor: %d", w.Code)
	}
	for _, p := range []string{"/labs/maps/static/index.html", "/labs/maps/static/../server.go", "/labs/maps/static/nothing.js"} {
		if w := get(s, p); w.Code == 200 { // the router sends ".." to the clean path first
			t.Errorf("%s: %d", p, w.Code)
		}
	}
}

func TestPictures(t *testing.T) {
	f := newFake(t)
	s := testServer(t, f)
	// a chain: its logo
	w := get(s, "/labs/maps/api/picture?brand=Q38076")
	if w.Code != 200 || w.Body.String() != string(png) || w.Header().Get("X-Picture-Kind") != "logo" || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("logo: %d %v", w.Code, w.Header())
	}
	if c, _ := url.QueryUnescape(w.Header().Get("X-Picture-Credit")); c != "Ann & Bo, CC BY-SA 4.0" {
		t.Errorf("credit: %q", c)
	}
	if p := w.Header().Get("X-Picture-Page"); p != "https://commons.wikimedia.org/wiki/File:Logo.svg" {
		t.Errorf("page: %q", p)
	}
	// the photo of the place comes before the logo of the chain
	if w := get(s, "/labs/maps/api/picture?wikidata=Q1&brand=Q38076"); w.Header().Get("X-Picture-Kind") != "photo" {
		t.Errorf("photo first: %v", w.Header())
	}
	// a file of the place; from an image URL on Commons too
	if w := get(s, "/labs/maps/api/picture?commons=File:Bundeshaus_Bern.jpg"); w.Code != 200 || !strings.Contains(w.Header().Get("X-Picture-Page"), "File:Bundeshaus Bern.jpg") {
		t.Errorf("commons: %d %v", w.Code, w.Header())
	}
	// kept: the same picture again does not ask Wikimedia
	n := f.calls.Load()
	get(s, "/labs/maps/api/picture?brand=Q38076")
	if f.calls.Load() != n {
		t.Errorf("not kept: %d calls more", f.calls.Load()-n)
	}
	// nothing to show
	// a town found by the search: its relation names the item, whose photo it gets; kept in memory
	before := f.calls.Load()
	if w := get(s, "/labs/maps/api/picture?osm=relation/1682682&w=500"); w.Code != 200 || w.Header().Get("X-Picture-Kind") != "photo" {
		t.Errorf("a relation: %d %v", w.Code, w.Header())
	}
	get(s, "/labs/maps/api/picture?osm=relation/1682682&w=500")
	if w := get(s, "/labs/maps/api/picture?osm=relation/7"); w.Code != 204 {
		t.Errorf("a relation without an item: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/picture?osm=node/7"); w.Code != 400 {
		t.Errorf("a node: %d", w.Code)
	}
	if c := f.calls.Load() - before; c > 5 {
		t.Errorf("calls to Wikimedia for a relation, twice, and another: %d", c)
	}
	if w := get(s, "/labs/maps/api/picture?wikidata=Q99"); w.Code != 204 {
		t.Errorf("none: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/picture?commons=File:Missing.jpg"); w.Code != 204 {
		t.Errorf("missing: %d", w.Code)
	}
	// what does not name a picture
	for _, q := range []string{"", "wikidata=42", "brand=Q1;DROP", "commons=Category:Bern", "commons=File:../x.jpg", "wikidata=Q1&w=77"} {
		if w := get(s, "/labs/maps/api/picture?"+q); w.Code != 400 {
			t.Errorf("%q: %d", q, w.Code)
		}
	}
}

func TestPictureNotSVG(t *testing.T) {
	f := newFake(t)
	f.svg = true
	s := testServer(t, f)
	if w := get(s, "/labs/maps/api/picture?brand=Q38076"); w.Code != 204 {
		t.Errorf("svg: %d %s", w.Code, w.Body.String())
	}
}

func TestPictureThumbHost(t *testing.T) {
	f := newFake(t)
	s := testServer(t, f, func(c *Config) {})
	s.cfg.ThumbHosts = []string{"upload.wikimedia.org"} // the fake is not it
	if w := get(s, "/labs/maps/api/picture?brand=Q38076"); w.Code != 204 {
		t.Errorf("other host: %d", w.Code)
	}
}

func TestPictureLimit(t *testing.T) {
	f := newFake(t)
	s := testServer(t, f, func(c *Config) { c.PicturesPerIP, c.PictureWindow = 2, time.Hour })
	for i, want := range []int{200, 200, 429} {
		if w := get(s, "/labs/maps/api/picture?brand=Q38076", "X-Forwarded-For", "203.0.113.5"); w.Code != want {
			t.Errorf("call %d: %d", i, w.Code)
		}
	}
	if w := get(s, "/labs/maps/api/picture?brand=Q38076", "X-Forwarded-For", "203.0.113.6"); w.Code != 200 {
		t.Errorf("other visitor: %d", w.Code)
	}
}

func TestPicturesOff(t *testing.T) {
	s := testServer(t, nil, func(c *Config) { c.Pictures = false })
	if w := get(s, "/labs/maps/api/picture?brand=Q38076"); w.Code != 404 {
		t.Errorf("off: %d", w.Code)
	}
	if w := get(s, "/labs/maps"); strings.Contains(w.Body.String(), "data-pictures") {
		t.Errorf("off: the page names the pictures")
	}
}

func TestMemo(t *testing.T) {
	m := newMemo(100, time.Hour)
	m.put("a", make([]byte, 10))
	m.put("b", make([]byte, 10))
	m.put("c", make([]byte, 90)) // larger than an eighth: not kept
	if _, ok := m.get("c"); ok {
		t.Error("c kept")
	}
	for i := 0; i < 10; i++ {
		m.put(string(rune('d'+i)), make([]byte, 11))
	}
	if _, ok := m.get("a"); ok || m.size > 100 {
		t.Errorf("a kept, size %d", m.size)
	}
	at := time.Now()
	m.now = func() time.Time { return at.Add(2 * time.Hour) }
	if _, ok := m.get("m"); ok {
		t.Error("old value kept")
	}
}
