// Package server serves the map app: the page and its files, and what the page asks of other services,
// each in a file of its own: the search of overpass-search (search.go, its words in the language of the
// page in labels.go), pictures from Wikimedia Commons (picture.go), directions from Valhalla (route.go) with
// the signs beside the route (marks.go) and the places along it (along.go), spoken sentences (say.go),
// traffic (traffic.go), transit lines (transit.go), journeys by public transport from MOTIS (journey.go),
// donations (donate.go), feedback (feedback.go), and the place of the visitor from the IP address (geoip.go).
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitlab.com/cdutsov/hippomaps/web"
)

// Config of the server.
type Config struct {
	Base      string // the path of the page: /labs/maps
	SearchAPI string // the API of overpass-search for the browser: /labs/search/api (SearchUpstream sets it)
	// SearchUpstream: overpass-search elsewhere (https://overpass.ch/labs/search/api), relayed under
	// {base}/api/search for a site of its own, as the search API answers its own site only
	SearchUpstream string
	// PublicURL: the address of the page on its own site (https://hippomaps.ch). Set, the page is for
	// search engines: no noindex, a canonical link, robots.txt and sitemap.xml at the root; empty,
	// the page stays unlisted (noindex)
	PublicURL string
	TilesURL  string // the map tiles API: https://api.overpass.ch
	MapsKey   string // a key of the tiles API for this page

	Pictures      bool          // pictures from Wikimedia Commons
	PicturesPerIP int           // pictures a visitor gets in PictureWindow
	PictureWindow time.Duration //
	CacheMB       int           // memory for answers of Wikimedia and thumbnails
	WikidataAPI   string        // https://www.wikidata.org/w/api.php
	CommonsAPI    string        // https://commons.wikimedia.org/w/api.php
	ThumbHosts    []string      // hosts of thumbnails: upload.wikimedia.org, thumb.wikimedia.org
	UserAgent     string        // for Wikimedia, with a way to reach the operator
	// PanoramaxAPI: the pictures of streets of Panoramax, for places without a picture (near.go); empty: none
	PanoramaxAPI string

	RoutingURL   string // Valhalla, for directions; empty: no directions
	ElevationURL string // the elevation API of overpass.ch, for height profiles of routes by bike and on foot
	ElevationKey string // a key for it (a maps key does not do); empty: no height profiles
	OverpassURL  string // an Overpass API of its own, asked without a key; empty: the one of overpass.ch with ElevationKey
	RoutesPerIP  int    // directions a visitor gets in RouteWindow
	// JourneyURL: MOTIS, for journeys by public transport (journey.go); empty: none. The walks to the stops come from
	// RoutingURL (without it: straight lines)
	JourneyURL string
	// JourneyRegions: where MOTIS has timetables, as country codes (CH, BG) or parts of the world by their number of
	// the UN (150: Europe); the page names them where it finds no stop
	JourneyRegions []string
	TTSURL         string        // a text-to-speech server for navigation (Piper); empty: the voices of the browser
	SaysPerIP      int           // spoken sentences a visitor gets in RouteWindow, beyond those in memory
	SearchesPerIP  int           // searches (search, query) a visitor gets in RouteWindow; taps on the map (here) three times as many
	TrafficURL     string        // the traffic service next to Valhalla (incidents.json); empty: no traffic
	RouteWindow    time.Duration //
	// GeoIPDB: a MaxMind DB file with a location per network (DB-IP City Lite): the map opens at the
	// place of the IP address of the visitor until the device tells where it is; empty: at its default place
	GeoIPDB string
	// StatsDir: a directory that lasts (a volume) for the counts of visits (stats.go: per day the page loads, the
	// visitors and the sites they came from, no address); empty: no counting. StatsIgnore: addresses and networks
	// not counted (our own).
	StatsDir    string
	StatsIgnore []string
	// ServiceWorker: the service worker of the page (web/sw.js); false: one that removes itself and its caches from the
	// browsers that have it (web/sw-off.js), and the page registers none
	ServiceWorker bool
	// StripeKey: a restricted key of Stripe that may write Checkout Sessions, for donations (donate.go); empty, or
	// without PublicURL: no donations
	StripeKey string
	StripeAPI string // https://api.stripe.com
	// FeedbackDir: a directory that lasts (a volume) for the messages of visitors (feedback.go), a file each, kept a
	// year; empty: no feedback. FeedbackTo: an address that gets each message by e-mail through the SMTP server
	// (SMTPUser is the sender); empty: none
	FeedbackDir  string
	FeedbackTo   string
	SMTPHost     string
	SMTPPort     string // 587 (STARTTLS) or 465 (TLS)
	SMTPUser     string
	SMTPPassword string
	Log          *slog.Logger
}

// DefaultConfig is the configuration without the environment.
func DefaultConfig() Config {
	return Config{
		Base: "/labs/maps", SearchAPI: "/labs/search/api", TilesURL: "https://api.overpass.ch",
		Pictures: true, PicturesPerIP: 600, PictureWindow: 10 * time.Minute, CacheMB: 64,
		WikidataAPI: "https://www.wikidata.org/w/api.php", CommonsAPI: "https://commons.wikimedia.org/w/api.php",
		ThumbHosts: []string{"upload.wikimedia.org", "thumb.wikimedia.org"}, UserAgent: "hippomaps (+https://gitlab.com/cdutsov/hippomaps)",
		RoutesPerIP: 120, RouteWindow: 10 * time.Minute, ElevationURL: "https://api.overpass.ch", SaysPerIP: 600, SearchesPerIP: 100,
		StripeAPI: "https://api.stripe.com", ServiceWorker: true, PanoramaxAPI: "https://api.panoramax.xyz", Log: slog.Default(),
	}
}

func (c Config) allowedThumb(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "https" {
		return false
	}
	for _, h := range c.ThumbHosts {
		if p.Host == h {
			return true
		}
	}
	return false
}

// Server is the http.Handler of the app.
type Server struct {
	cfg          Config
	log          *slog.Logger
	mux          *http.ServeMux
	page         *template.Template
	privacyPages *template.Template // privacy.html, privacy-de.html
	pages        map[string]*page   // the pages about the app, by their path (pages.go)
	hashes       map[string]string  // file -> short hash, for addresses that change with the file
	files        map[string][]byte  // the scripts served with the hashes of their imports in the addresses (served)
	worker       string             // the address of the worker of MapLibre (mapWorker)
	client       *http.Client
	cache        *memo
	perIP        *window
	calls        chan struct{} // calls to Wikimedia at once

	routeClient *http.Client
	routesPerIP *window
	reachPerIP  *window // reachable areas (reach.go): apart from directions, so a pin dragged about leaves them
	routeCalls  chan struct{} // calls to Valhalla at once
	saysPerIP   *window
	sayCalls    chan struct{} // calls to the text-to-speech server at once

	transitPerIP *window
	transitCalls chan struct{} // queries to Overpass for transit at once

	journeyEnds endsKept // the stops and walks around the ends of journeys asked for in the last minutes

	trafficCache  trafficCache
	searchClient  *http.Client
	searchesPerIP *window // search: it runs on Overpass
	queriesPerIP  *window // query: how a search is read, asked by the page before each search
	herePerIP     *window // taps on the map

	geo   *geoIP // nil without GeoIPDB
	stats *stats // nil without StatsDir

	donatesPerIP *window // checkouts of donations

	feedbackPerIP *window       // messages of visitors
	mails         chan Feedback // messages to mail (mailer)
	sendMail      func(to string, msg []byte) error
	mailRetry     time.Duration // the wait before the second try, twice that before the third

	siteClient  *http.Client // the websites of places and pictures of the web (site.go): public addresses only
	places      *placeState  // more about places: answers and pictures kept (place.go)
	placesPerIP *window
	placeCalls  chan struct{} // places looked up at once
}

// New makes the server.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, log: cfg.Log, mux: http.NewServeMux(), hashes: map[string]string{}, files: map[string][]byte{},
		client: &http.Client{Timeout: 10 * time.Second}, cache: newMemo(cfg.CacheMB<<20, 24*time.Hour),
		perIP: newWindow(cfg.PicturesPerIP, cfg.PictureWindow), calls: make(chan struct{}, 4),
		routeClient: &http.Client{Timeout: 30 * time.Second}, routesPerIP: newWindow(cfg.RoutesPerIP, cfg.RouteWindow),
		reachPerIP: newWindow(cfg.RoutesPerIP, cfg.RouteWindow),
		routeCalls: make(chan struct{}, 4), saysPerIP: newWindow(cfg.SaysPerIP, cfg.RouteWindow), sayCalls: make(chan struct{}, 4),
		transitPerIP: newWindow(300, 10*time.Minute), transitCalls: make(chan struct{}, 2),
		searchesPerIP: newWindow(cfg.SearchesPerIP, cfg.RouteWindow), queriesPerIP: newWindow(cfg.SearchesPerIP, cfg.RouteWindow), herePerIP: newWindow(3*cfg.SearchesPerIP, cfg.RouteWindow),
		donatesPerIP: newWindow(10, 10*time.Minute), siteClient: newSiteClient(publicDial),
		places: newPlaceState(), placesPerIP: placeWindow(cfg), placeCalls: make(chan struct{}, 4),
		feedbackPerIP: newWindow(feedbackPerHour, time.Hour), mailRetry: time.Minute}
	s.sendMail = s.smtpSend
	if s.mailOn() {
		s.mails = make(chan Feedback, 100)
		go s.mailer()
	}
	s.cfg.TTSURL = strings.TrimRight(cfg.TTSURL, "/")
	s.cfg.TrafficURL = strings.TrimRight(cfg.TrafficURL, "/")
	s.cfg.SearchUpstream = strings.TrimRight(cfg.SearchUpstream, "/")
	s.searchClient = &http.Client{Timeout: 90 * time.Second}
	if s.cfg.SearchUpstream != "" {
		s.cfg.SearchAPI = strings.TrimRight(cfg.Base, "/") + "/api/search"
	}
	s.cfg.RoutingURL = strings.TrimRight(cfg.RoutingURL, "/")
	s.cfg.JourneyURL = strings.TrimRight(cfg.JourneyURL, "/")
	if s.log == nil {
		s.log = slog.Default()
	}
	if cfg.GeoIPDB != "" {
		s.geo = newGeoIP(cfg.GeoIPDB, s.log)
	}
	if cfg.StatsDir != "" {
		s.stats = newStats(cfg.StatsDir, cfg.StatsIgnore, s.log)
	}
	fs.WalkDir(web.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			s.served(p, nil)
		}
		return nil
	})
	s.worker = s.mapWorker()
	s.page = template.Must(template.New("index.html").Funcs(template.FuncMap{"asset": s.asset, "vendor": s.vendor}).ParseFS(web.Files, "index.html"))
	s.privacyPages = template.Must(template.New("privacy").Funcs(template.FuncMap{"asset": s.asset}).ParseFS(web.Files, "privacy.html", "privacy-de.html", "licences.html", "licences-de.html"))
	base := strings.TrimRight(cfg.Base, "/")
	if base == "" { // the page at the root of its own site (hippomaps.ch)
		s.mux.HandleFunc("GET /{$}", s.index)
	} else {
		s.mux.HandleFunc("GET "+base, s.index)
		s.mux.HandleFunc("GET "+base+"/{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base, http.StatusMovedPermanently)
		})
	}
	// the privacy policy (the app links it from its menu; Google Play asks for its address)
	s.mux.HandleFunc("GET "+base+"/privacy", s.privacy("privacy.html"))
	s.mux.HandleFunc("GET "+base+"/de/privacy", s.privacy("privacy-de.html"))
	// the licences and sources (voices, traffic, map data, libraries): the page links it small
	s.mux.HandleFunc("GET "+base+"/licences", s.privacy("licences.html"))
	s.mux.HandleFunc("GET "+base+"/de/licences", s.privacy("licences-de.html"))
	// the features and guides (pages.go)
	s.loadPages()
	s.mux.Handle("GET "+base+"/static/", http.StripPrefix(base+"/static/", http.HandlerFunc(s.static)))
	// the service worker (web/sw.js): the page and the map of a route without a connection; next to the page, with
	// the page in its scope
	s.mux.HandleFunc("GET "+base+"/sw.js", s.serviceWorker)
	if cfg.Pictures {
		s.mux.HandleFunc("GET "+base+"/api/picture", s.picture)
	}
	if s.placeExtras() { // more about a place: its website, free pictures near it (place.go)
		s.mux.HandleFunc("GET "+base+"/api/place", s.place)
		s.mux.HandleFunc("GET "+base+"/api/place/picture", s.placePicture)
	}
	// the names of the kinds of POIs of the tiles, for their popups (kinds.go)
	s.mux.HandleFunc("GET "+base+"/api/kinds", s.kinds)
	if s.cfg.RoutingURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/route", s.route)
		s.mux.HandleFunc("GET "+base+"/api/reach", s.reach) // how far in some minutes (reach.go)
	}
	if s.cfg.JourneyURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/journey", s.journey)
	}
	if s.cfg.TTSURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/say", s.say)
	}
	if s.cfg.TrafficURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/traffic", s.traffic)
	}
	if s.cfg.SearchUpstream != "" {
		s.mux.HandleFunc("GET "+base+"/api/search/{rest...}", s.search)
	}
	if s.overpass() { // an Overpass API of its own, or the key of the APIs of overpass.ch: Overpass too
		s.mux.HandleFunc("GET "+base+"/api/transit", s.transit)
		if s.cfg.RoutingURL != "" {
			s.mux.HandleFunc("POST "+base+"/api/along", s.along)
		}
	}
	if s.cfg.ElevationKey != "" {
		s.mux.HandleFunc("POST "+base+"/api/heights", s.heightsOf)
	}
	if s.donations() {
		s.mux.HandleFunc("POST "+base+"/api/donate", s.donate)
	}
	if s.feedbackOn() {
		s.mux.HandleFunc("POST "+base+"/api/feedback", s.feedback)
	}
	if s.cfg.PublicURL != "" {
		s.mux.HandleFunc("GET /robots.txt", s.robots)
		s.mux.HandleFunc("GET /sitemap.xml", s.sitemap)
		// the app for Android opens the links of the site (android/README.md)
		s.mux.HandleFunc("GET /.well-known/assetlinks.json", func(w http.ResponseWriter, r *http.Request) {
			b, err := fs.ReadFile(web.Files, "assetlinks.json")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		})
	}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	if s.cfg.TrafficURL != "" {
		go s.trafficBody(context.Background()) // the countries of traffic for the first page
	}
	return s
}

// Close writes the counts of visits.
func (s *Server) Close() { s.stats.Close() }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	// only the pages are for search engines, not the API; robots.txt and the sitemap are read by them as they are
	if s.cfg.PublicURL == "" || !s.listed(r.URL.Path) && r.URL.Path != "/robots.txt" && r.URL.Path != "/sitemap.xml" {
		h.Set("X-Robots-Tag", "noindex")
	}
	s.mux.ServeHTTP(w, r)
}

// listed: the page, the pages about it, the privacy policy and the licences, the pages of the sitemap
func (s *Server) listed(p string) bool {
	b := strings.TrimRight(s.cfg.Base, "/")
	if _, ok := s.pages[p]; ok {
		return true
	}
	return p == s.home() || p == b+"/privacy" || p == b+"/de/privacy" || p == b+"/licences" || p == b+"/de/licences"
}

// home is the path of the page, and the scope of its service worker
func (s *Server) home() string {
	if b := strings.TrimRight(s.cfg.Base, "/"); b != "" {
		return b
	}
	return "/"
}

// robots and sitemap of a site of its own (PublicURL)
func (s *Server) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /api/\nSitemap: %s/sitemap.xml\n", strings.TrimRight(s.cfg.PublicURL, "/"))
}

func (s *Server) sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	io.WriteString(w, s.sitemapXML())
}

// asset is the address of a file of the app, with its hash: browsers keep it for a year.
func (s *Server) asset(p string) string {
	return strings.TrimRight(s.cfg.Base, "/") + "/static/" + p + "?v=" + s.hashes[p]
}

// vendor is the address of a file of a library (web/vendor): its version is in the path, and it has no hash, as
// the scripts import it (./vendor/…): a preload under another address would load the file a second time.
func (s *Server) vendor(p string) string {
	return strings.TrimRight(s.cfg.Base, "/") + "/static/vendor/" + p
}

// mapWorker is the address MapLibre asks its worker at: next to its module, with its version as v. The page asks
// for it ahead (web/early.js). The version is that of the MapLibre app.js imports.
func (s *Server) mapWorker() string {
	m := mapLibreRe.FindSubmatch(s.served("app.js", nil))
	if m == nil {
		return ""
	}
	return s.vendor("maplibre-gl-" + string(m[1]) + "/maplibre-gl-worker.mjs?v=" + string(m[1]))
}

var (
	// a script of the app imported by another (import … from "./i18n.js", import("./wmm.js")): not the libraries
	importRe   = regexp.MustCompile(`(\bfrom\s*|\bimport\s*\(\s*)"\./([A-Za-z0-9_-]+\.js)"`)
	mapLibreRe = regexp.MustCompile(`from\s*"\./vendor/maplibre-gl-([0-9.]+)/maplibre-gl\.mjs"`)
)

// served is a file of the app as the server sends it, and notes its hash. A script imports the other scripts of
// the app with their hashes in the addresses (./i18n.js?v=…): they are kept for a year too, and none is asked for
// again at each visit. The hash of a script is that of what is sent, so it changes with a script it imports.
func (s *Server) served(p string, above []string) []byte {
	b, ok := s.files[p]
	if !ok {
		b, _ = fs.ReadFile(web.Files, p)
	}
	if _, done := s.hashes[p]; done {
		return b
	}
	if path.Ext(p) == ".js" && !strings.Contains(p, "/") {
		above = append(above, p)
		out := importRe.ReplaceAllFunc(b, func(m []byte) []byte {
			sub := importRe.FindSubmatch(m)
			name := string(sub[2])
			if _, err := fs.Stat(web.Files, name); err != nil {
				return m
			}
			for _, a := range above { // scripts that import each other: no hash can hold both
				if a == name {
					return m
				}
			}
			s.served(name, above)
			return []byte(string(sub[1]) + `"./` + name + "?v=" + s.hashes[name] + `"`)
		})
		if !bytes.Equal(out, b) {
			b = out
			s.files[p] = b
		}
	}
	sum := sha256.Sum256(b)
	s.hashes[p] = hex.EncodeToString(sum[:])[:10]
	return b
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	own := ""
	if u, err := url.Parse(s.cfg.PublicURL); err == nil {
		own = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	s.stats.record(r, own)
	var buf bytes.Buffer
	data := map[string]any{"Base": strings.TrimRight(s.cfg.Base, "/"), "SearchAPI": s.cfg.SearchAPI, "Tiles": s.cfg.TilesURL,
		"Key": s.cfg.MapsKey, "Pictures": s.cfg.Pictures, "Place": s.placeExtras(), "Routes": s.cfg.RoutingURL != "", "Journey": s.cfg.JourneyURL != "", "JourneyRegions": strings.Join(s.cfg.JourneyRegions, ","), "Say": s.cfg.TTSURL != "", "Voices": voiceList(), "Traffic": s.cfg.TrafficURL != "", "TrafficCountries": s.trafficCountries(), "Transit": s.overpass(), "Along": s.overpass() && s.cfg.RoutingURL != "", "Heights": s.cfg.ElevationKey != "", "Public": strings.TrimRight(s.cfg.PublicURL, "/"), "Donate": s.donations(), "Feedback": s.feedbackOn(), "Scope": s.home(), "SW": s.cfg.ServiceWorker, "Worker": s.worker}
	// a connection ahead to the tiles API on another site; on the site of the page there is one already, and one
	// without cookies (crossorigin) would be a second that nothing uses
	if u, err := url.Parse(s.cfg.TilesURL); err == nil && u.Host != "" && !strings.EqualFold(u.Host, r.Host) {
		data["Preconnect"] = u.Scheme + "://" + u.Host
	}
	if n, ok := s.geo.near(clientIP(r)); ok { // where the map opens: latitude, longitude, zoom
		data["Near"] = fmt.Sprintf("%.3f,%.3f,%d", n.Lat, n.Lon, n.Zoom())
	}
	if err := s.page.Execute(&buf, data); err != nil {
		s.log.Error("page", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "private, no-cache") // the place of the visitor in it
	h.Set("Content-Security-Policy", s.csp())
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(self), payment=()")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin") // the tiles key holds for the site only
	w.Write(buf.Bytes())
}

// serviceWorker serves web/sw.js, or with ServiceWorker off web/sw-off.js; the browser asks for it again at each
// visit (no-cache)
func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) {
	name := "sw.js"
	if !s.cfg.ServiceWorker {
		name = "sw-off.js"
	}
	b, err := fs.ReadFile(web.Files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Service-Worker-Allowed", s.home()) // its scope: the path of the page
	w.Write(b)
}

// privacy serves a page of text, no script: the privacy policy or the licences, in one language
func (s *Server) privacy(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		data := map[string]any{"Base": strings.TrimRight(s.cfg.Base, "/"), "Public": strings.TrimRight(s.cfg.PublicURL, "/")}
		if err := s.privacyPages.ExecuteTemplate(&buf, name, data); err != nil {
			s.log.Error("privacy", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Write(buf.Bytes())
	}
}

// csp lets the page reach this site and the tiles API; pictures and spoken sentences come as blobs.
func (s *Server) csp() string {
	tiles := ""
	if u, err := url.Parse(s.cfg.TilesURL); err == nil && u.Host != "" {
		tiles = " " + u.Scheme + "://" + u.Host
	}
	api := ""
	if u, err := url.Parse(s.cfg.SearchAPI); err == nil && u.Host != "" {
		api = " " + u.Scheme + "://" + u.Host
	}
	return strings.Join([]string{
		"default-src 'self'",
		"img-src 'self' data: blob:" + tiles,
		"style-src 'self'",
		"script-src 'self'",
		"worker-src 'self' blob:",
		"child-src 'self' blob:",
		"connect-src 'self'" + tiles + api,
		"font-src 'self'",
		"media-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
	}, "; ")
}

var types = map[string]string{".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".txt": "text/plain; charset=utf-8", ".png": "image/png", ".webp": "image/webp", ".woff2": "font/woff2"}

// static serves the files of the app. With the hash of the file in v they are kept for a
// year; the libraries in vendor/ have their version in the path.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	p := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	b, ok := s.files[p] // a script with the hashes of its imports
	var err error
	if !ok {
		b, err = fs.ReadFile(web.Files, p)
	}
	if err != nil || strings.HasSuffix(p, ".html") || strings.HasPrefix(p, ".") { // the pages are templates
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	if t, ok := types[path.Ext(p)]; ok {
		h.Set("Content-Type", t)
	} else {
		h.Set("Content-Type", "application/octet-stream")
	}
	if v := r.URL.Query().Get("v"); v != "" && v == s.hashes[p] || strings.HasPrefix(p, "vendor/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	w.Write(b)
}

// clientIP is the visitor: the first address of X-Forwarded-For, which the proxy in front sets.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// visitor is the key of the limits per visitor: the address, or of an IPv6 address the /64 it is in (a
// visitor has every address of their network, and may take another for each request).
func visitor(r *http.Request) string {
	ip := clientIP(r)
	if a, err := netip.ParseAddr(ip); err == nil && a.Is6() && !a.Is4In6() {
		if p, err := a.WithZone("").Prefix(64); err == nil {
			return p.String()
		}
	}
	return ip
}

// window counts calls per key in a fixed window.
type window struct {
	mu    sync.Mutex
	max   int
	every time.Duration
	start time.Time
	n     map[string]int
}

func newWindow(max int, every time.Duration) *window {
	return &window{max: max, every: every, n: map[string]int{}}
}

func (w *window) allow(key string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if now.Sub(w.start) >= w.every {
		w.start, w.n = now, map[string]int{}
	}
	w.n[key]++
	return w.n[key] <= w.max
}

// full says whether key has used up the window, without counting
func (w *window) full(key string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return now.Sub(w.start) < w.every && w.n[key] >= w.max
}

// memo keeps values for a time, up to a size; when full, the oldest go first.
type memo struct {
	mu    sync.Mutex
	max   int
	ttl   time.Duration
	size  int
	items map[string]memoItem
	order []string // keys by age
	now   func() time.Time
}

type memoItem struct {
	v  []byte
	at time.Time
}

func newMemo(max int, ttl time.Duration) *memo {
	return &memo{max: max, ttl: ttl, items: map[string]memoItem{}, now: time.Now}
}

func (m *memo) get(k string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[k]
	if !ok || m.now().Sub(it.at) > m.ttl {
		return nil, false
	}
	return it.v, true
}

func (m *memo) put(k string, v []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(v)+len(k) > m.max/8 {
		return // one value does not take the memory of others
	}
	if old, ok := m.items[k]; ok {
		m.size -= len(old.v) + len(k)
	} else {
		m.order = append(m.order, k)
	}
	m.items[k] = memoItem{v: v, at: m.now()}
	m.size += len(v) + len(k)
	for m.size > m.max && len(m.order) > 0 {
		old := m.order[0]
		m.order = m.order[1:]
		if it, ok := m.items[old]; ok {
			m.size -= len(it.v) + len(old)
			delete(m.items, old)
		}
	}
}
