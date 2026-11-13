package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/cdutsov/hippomaps/web"
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

// the service worker next to the page, its scope the page (also at the root of a site), asked again at each visit
func TestServiceWorker(t *testing.T) {
	for _, c := range []struct{ base, sw, scope string }{{"/labs/maps", "/labs/maps/sw.js", "/labs/maps"}, {"/", "/sw.js", "/"}} {
		s := testServer(t, nil, func(cfg *Config) { cfg.Base = c.base })
		w := get(s, c.sw)
		h := w.Header()
		if w.Code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/javascript") || h.Get("Cache-Control") != "no-cache" ||
			h.Get("Service-Worker-Allowed") != c.scope || !strings.Contains(w.Body.String(), `addEventListener("fetch"`) {
			t.Errorf("%s: %d %v", c.sw, w.Code, h)
		}
		body := get(s, c.base).Body.String()
		if !strings.Contains(body, `data-sw="`+c.sw+`"`) || !strings.Contains(body, `data-scope="`+c.scope+`"`) {
			t.Errorf("page %s: no service worker in %s", c.base, body)
		}
	}
}

// SERVICE_WORKER=off: sw.js removes itself and its caches, and the page registers none
func TestServiceWorkerOff(t *testing.T) {
	s := testServer(t, nil, func(cfg *Config) { cfg.Base = "/"; cfg.ServiceWorker = false })
	w := get(s, "/sw.js")
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(body, "unregister()") ||
		!strings.Contains(body, "caches.delete") || strings.Contains(body, `addEventListener("fetch"`) {
		t.Errorf("sw.js off: %d %s", w.Code, body)
	}
	if page := get(s, "/").Body.String(); strings.Contains(page, "data-sw=") {
		t.Errorf("page with the service worker off: %s", page)
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

// The privacy policy: English and German, linked from each other, no script; listed for search engines on a site of
// its own; the templates are not served as files.
func TestPrivacy(t *testing.T) {
	s := testServer(t, nil)
	for p, want := range map[string]string{"/labs/maps/privacy": "Privacy policy", "/labs/maps/de/privacy": "Datenschutzerklärung"} {
		w := get(s, p)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, want) || !strings.Contains(body, "info@overpass.ch") || strings.Contains(body, "<script") {
			t.Fatalf("%s: %d %s", p, w.Code, body)
		}
		if !strings.Contains(body, `/labs/maps/static/privacy.css?v=`) || !strings.Contains(body, `href="/labs/maps/de/privacy"`) && !strings.Contains(body, `href="/labs/maps/privacy"`) {
			t.Errorf("%s: links %s", p, body)
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script") {
			t.Errorf("%s: csp %s", p, csp)
		}
	}
	for _, p := range []string{"/labs/maps/static/privacy.html", "/labs/maps/static/privacy-de.html"} {
		if w := get(s, p); w.Code == 200 {
			t.Errorf("%s served as a file", p)
		}
	}
	if w := get(s, "/labs/maps/static/privacy.css"); w.Code != 200 {
		t.Errorf("privacy.css: %d", w.Code)
	}
	// on its own site (hippomaps.ch): in the sitemap, without noindex
	own := testServer(t, nil, func(c *Config) { c.Base = ""; c.PublicURL = "https://hippomaps.example" })
	if w := own.listed("/privacy") && own.listed("/de/privacy") && !own.listed("/api/route"); !w {
		t.Error("listed")
	}
	if w := get(own, "/privacy"); w.Code != 200 || w.Header().Get("X-Robots-Tag") != "" || !strings.Contains(w.Body.String(), `<link rel="canonical" href="https://hippomaps.example/privacy">`) {
		t.Errorf("own site: %d %v", w.Code, w.Header())
	}
	if w := get(own, "/sitemap.xml"); !strings.Contains(w.Body.String(), "<loc>https://hippomaps.example/de/privacy</loc>") {
		t.Errorf("sitemap: %s", w.Body.String())
	}
	// the menu links it (app.js), in German for German
	app, _ := fs.ReadFile(web.Files, "app.js")
	if !strings.Contains(string(app), `"/de/privacy"`) || !strings.Contains(string(app), "privacyLink") {
		t.Error("app.js: no link to the privacy policy")
	}
}

// The licences and sources: English and German, with the credits the page itself no longer spells out (the voices
// under CC BY, the traffic data of FEDRO in its wording, OpenStreetMap, DB-IP); no script; in the sitemap; the page
// links them and holds no credit text of its own.
func TestLicences(t *testing.T) {
	s := testServer(t, nil)
	for p, want := range map[string]string{"/labs/maps/licences": "Licences and sources", "/labs/maps/de/licences": "Lizenzen und Quellen"} {
		w := get(s, p)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, want) || strings.Contains(body, "<script") {
			t.Fatalf("%s: %d %s", p, w.Code, body)
		}
		for _, credit := range []string{"SIWIS", "Hi-Fi TTS", "serena-synthetic-it-27h (Marco Baccarani)", "LibriTTS-R", "CC BY",
			"Data source: Federal Roads Office (FEDRO) Traffic Data Platform (TDP).", "OpenStreetMap", "ODbL",
			`<a href="https://db-ip.com">IP Geolocation by DB-IP</a>`, "GNU General Public License", "MapLibre GL JS (BSD-3-Clause)",
			// the timetables of public transport: opentransportdata.swiss asks to be cited as the source
			`<a href="https://opentransportdata.swiss">opentransportdata.swiss</a>`, "https://transitous.org/sources/", "sofia.bg", "MOTIS"} {
			if !strings.Contains(body, credit) {
				t.Errorf("%s: no %q", p, credit)
			}
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("%s: csp %s", p, csp)
		}
	}
	for _, p := range []string{"/labs/maps/static/licences.html", "/labs/maps/static/licences-de.html"} {
		if w := get(s, p); w.Code == 200 {
			t.Errorf("%s served as a file", p)
		}
	}
	own := testServer(t, nil, func(c *Config) { c.Base = ""; c.PublicURL = "https://hippomaps.example" })
	if !own.listed("/licences") || !own.listed("/de/licences") {
		t.Error("listed")
	}
	if w := get(own, "/sitemap.xml"); !strings.Contains(w.Body.String(), "<loc>https://hippomaps.example/de/licences</loc>") {
		t.Errorf("sitemap: %s", w.Body.String())
	}
	app, _ := fs.ReadFile(web.Files, "app.js")
	i18n, _ := fs.ReadFile(web.Files, "i18n.js")
	if !strings.Contains(string(app), `"/de/licences"`) || !strings.Contains(string(app), "licencesLink") {
		t.Error("app.js: no link to the licences")
	}
	for _, text := range []string{"SIWIS", "Federal Roads Office"} {
		if strings.Contains(string(app), text) || strings.Contains(string(i18n), text) {
			t.Errorf("the page still spells out a credit (%s): it belongs on the licences page", text)
		}
	}
}

// Wikimedia fails: 502, and the failure is not kept (the next call asks again); a thumbnail larger than a
// picture may be: 502; a picture found by another way wins over the failure of the first
func TestPictureWikimediaFails(t *testing.T) {
	f := &fakeWikimedia{}
	var down atomic.Bool
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		q := r.URL.Query()
		switch {
		case down.Load():
			w.WriteHeader(503)
		case r.URL.Path == "/wd" && q.Get("entity") == "Q5": // an item whose claims fail
			w.WriteHeader(500)
		case r.URL.Path == "/wd" && q.Get("list") == "search":
			io.WriteString(w, `not JSON`)
		case r.URL.Path == "/wd":
			io.WriteString(w, `{"claims":{"`+q.Get("property")+`":[{"rank":"normal","mainsnak":{"datavalue":{"value":"Photo.jpg"}}}]}}`)
		case r.URL.Path == "/commons":
			file := strings.TrimPrefix(q.Get("titles"), "File:")
			io.WriteString(w, `{"query":{"pages":[{"imageinfo":[{"thumburl":"`+f.srv.URL+`/thumb/`+url.PathEscape(file)+`","extmetadata":{"Artist":{"value":"Unknown author"},"LicenseShortName":{"value":"`+strings.Repeat("long ", 30)+`"}}}]}]}}`)
		case r.URL.Path == "/thumb/Huge.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(make([]byte, 2<<20+1))
		default:
			w.Header().Set("Content-Type", "image/png; charset=binary")
			w.Write(png)
		}
	}))
	t.Cleanup(f.srv.Close)
	s := testServer(t, f)
	if w := get(s, "/labs/maps/api/picture?wikidata=Q5"); w.Code != 502 {
		t.Errorf("claims fail: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/picture?osm=relation/5"); w.Code != 502 {
		t.Errorf("the search of Wikidata answers nonsense: %d", w.Code)
	}
	if w := get(s, "/labs/maps/api/picture?commons=File:Huge.jpg"); w.Code != 502 {
		t.Errorf("a thumbnail of more than 2 MB: %d", w.Code)
	}
	// the photo of the place fails, the logo of the chain is there
	w := get(s, "/labs/maps/api/picture?wikidata=Q5&brand=Q6")
	if w.Code != 200 || w.Header().Get("X-Picture-Kind") != "logo" || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("a logo after a failure: %d %v", w.Code, w.Header())
	}
	// no author named, a long licence cut
	if c, _ := url.QueryUnescape(w.Header().Get("X-Picture-Credit")); !strings.HasPrefix(c, "long long") || !strings.HasSuffix(c, "…") || len([]rune(c)) != 101 {
		t.Errorf("credit: %q", c)
	}
	down.Store(true)
	n := f.calls.Load()
	for i := 0; i < 2; i++ {
		if w := get(s, "/labs/maps/api/picture?wikidata=Q7"); w.Code != 502 {
			t.Errorf("Wikimedia down: %d", w.Code)
		}
	}
	if c := f.calls.Load() - n; c < 2 {
		t.Errorf("a failure was kept: %d calls for two requests", c)
	}
	down.Store(false)
	if w := get(s, "/labs/maps/api/picture?wikidata=Q7"); w.Code != 200 {
		t.Errorf("Wikimedia back: %d", w.Code)
	}
}

// all the calls to Wikimedia taken: 503 after 3 s
func TestPictureBusy(t *testing.T) {
	f := newFake(t)
	s := testServer(t, f)
	for i := 0; i < cap(s.calls); i++ {
		s.calls <- struct{}{}
	}
	t0 := time.Now()
	if w := get(s, "/labs/maps/api/picture?brand=Q38076"); w.Code != 503 || time.Since(t0) < 2*time.Second || f.calls.Load() != 0 {
		t.Errorf("busy: %d after %s, %d calls", w.Code, time.Since(t0), f.calls.Load())
	}
}

// files named in the tags of a place: the forms of wikimedia_commons and of an image URL on Commons
func TestCommonsFile(t *testing.T) {
	for v, want := range map[string]string{
		"File:Bundeshaus_Bern.jpg": "Bundeshaus Bern.jpg",
		" File:A b.JPEG ":          "A b.JPEG",
		"https://commons.wikimedia.org/wiki/File:Z%C3%BCrich_HB.jpg": "Zürich HB.jpg",
		"http://commons.wikimedia.org/wiki/File:X.png":               "X.png",
		"https://commons.wikimedia.org/wiki/File:%zz.jpg":            "",
		"https://commons.wikimedia.org/wiki/Category:Bern":           "",
		"https://example.org/File:X.png":                             "",
		"Category:Bern":                                              "",
		"File:no extension":                                          "",
		"File:a|b.jpg":                                               "",
		"File:a/b.jpg":                                               "",
		"File:" + strings.Repeat("a", 250) + ".jpg":                  "",
		"X.jpg": "",
	} {
		if got := commonsFile(v); got != want {
			t.Errorf("%q: %q, want %q", v, got, want)
		}
	}
}

// the visitor: the first address the proxy in front names, else the address of the connection
func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "198.51.100.4:5123"
	if ip := clientIP(r); ip != "198.51.100.4" {
		t.Errorf("from the connection: %q", ip)
	}
	r.RemoteAddr = "[2001:db8::7]:5123"
	if ip, v := clientIP(r), visitor(r); ip != "2001:db8::7" || v != "2001:db8::/64" {
		t.Errorf("from an IPv6 connection: %q, visitor %q", ip, v)
	}
	r.RemoteAddr = "no port"
	if ip := clientIP(r); ip != "no port" {
		t.Errorf("an address without a port: %q", ip)
	}
	r.Header.Set("X-Forwarded-For", " 203.0.113.7 , 10.0.0.1")
	if ip := clientIP(r); ip != "203.0.113.7" {
		t.Errorf("from the proxy: %q", ip)
	}
}

// a site of its own: the file that lets the Android app open its links; none without PublicURL
func TestAssetLinks(t *testing.T) {
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.PublicURL = "https://hippomaps.example" })
	w := get(s, "/.well-known/assetlinks.json")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), "android_app") {
		t.Errorf("assetlinks.json: %d %.100s", w.Code, w.Body)
	}
	if w := get(testServer(t, nil), "/.well-known/assetlinks.json"); w.Code != 404 {
		t.Errorf("assetlinks.json without a site: %d", w.Code)
	}
	if w := get(s, "/healthz"); w.Code != 200 || w.Body.String() != "ok\n" {
		t.Errorf("healthz: %d %q", w.Code, w.Body)
	}
}
