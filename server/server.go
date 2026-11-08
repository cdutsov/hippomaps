// Package server serves the map app: the page, its files, pictures from Wikimedia
// Commons, and directions from Valhalla. The search itself is the API of overpass-search, which the browser calls.
package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"gitlab.com/cdutsov/hippomaps/web"
)

// Config of the server.
type Config struct {
	Base      string // the path of the page: /labs/maps
	SearchAPI string // the API of overpass-search for the browser: /labs/search/api
	TilesURL  string // the map tiles API: https://api.overpass.ch
	MapsKey   string // a key of the tiles API for this page
	Version   string

	Pictures      bool          // pictures from Wikimedia Commons
	PicturesPerIP int           // pictures a visitor gets in PictureWindow
	PictureWindow time.Duration //
	CacheMB       int           // memory for answers of Wikimedia and thumbnails
	WikidataAPI   string        // https://www.wikidata.org/w/api.php
	CommonsAPI    string        // https://commons.wikimedia.org/w/api.php
	ThumbHosts    []string      // hosts of thumbnails: upload.wikimedia.org, thumb.wikimedia.org
	UserAgent     string        // for Wikimedia, with a way to reach the operator

	RoutingURL   string        // Valhalla, for directions; empty: no directions
	ElevationURL string        // the elevation API of overpass.ch, for height profiles of routes by bike and on foot
	ElevationKey string        // a key for it (a maps key does not do); empty: no height profiles
	RoutesPerIP  int           // directions a visitor gets in RouteWindow
	TTSURL       string        // a text-to-speech server for navigation (Piper); empty: the voices of the browser
	SaysPerIP    int           // spoken sentences a visitor gets in RouteWindow, beyond those in memory
	RouteWindow  time.Duration //
	Log          *slog.Logger
}

// DefaultConfig is the configuration without the environment.
func DefaultConfig() Config {
	return Config{
		Base: "/labs/maps", SearchAPI: "/labs/search/api", TilesURL: "https://api.overpass.ch",
		Pictures: true, PicturesPerIP: 600, PictureWindow: 10 * time.Minute, CacheMB: 64,
		WikidataAPI: "https://www.wikidata.org/w/api.php", CommonsAPI: "https://commons.wikimedia.org/w/api.php",
		ThumbHosts: []string{"upload.wikimedia.org", "thumb.wikimedia.org"}, UserAgent: "hippomaps (+https://gitlab.com/cdutsov/hippomaps)",
		RoutesPerIP: 120, RouteWindow: 10 * time.Minute, ElevationURL: "https://api.overpass.ch", SaysPerIP: 600,
		Log: slog.Default(),
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
	cfg    Config
	log    *slog.Logger
	mux    *http.ServeMux
	page   *template.Template
	hashes map[string]string // file -> short hash, for addresses that change with the file
	client *http.Client
	cache  *memo
	perIP  *window
	calls  chan struct{} // calls to Wikimedia at once

	routeClient *http.Client
	routesPerIP *window
	routeCalls  chan struct{} // calls to Valhalla at once
	saysPerIP   *window
	sayCalls    chan struct{} // calls to the text-to-speech server at once
}

// New makes the server.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, log: cfg.Log, mux: http.NewServeMux(), hashes: map[string]string{},
		client: &http.Client{Timeout: 10 * time.Second}, cache: newMemo(cfg.CacheMB<<20, 24*time.Hour),
		perIP: newWindow(cfg.PicturesPerIP, cfg.PictureWindow), calls: make(chan struct{}, 4),
		routeClient: &http.Client{Timeout: 30 * time.Second}, routesPerIP: newWindow(cfg.RoutesPerIP, cfg.RouteWindow),
		routeCalls: make(chan struct{}, 4), saysPerIP: newWindow(cfg.SaysPerIP, cfg.RouteWindow), sayCalls: make(chan struct{}, 4)}
	s.cfg.TTSURL = strings.TrimRight(cfg.TTSURL, "/")
	s.cfg.RoutingURL = strings.TrimRight(cfg.RoutingURL, "/")
	if s.log == nil {
		s.log = slog.Default()
	}
	fs.WalkDir(web.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := fs.ReadFile(web.Files, p)
			sum := sha256.Sum256(b)
			s.hashes[p] = hex.EncodeToString(sum[:])[:10]
		}
		return nil
	})
	s.page = template.Must(template.New("index.html").Funcs(template.FuncMap{"asset": s.asset}).ParseFS(web.Files, "index.html"))
	base := strings.TrimRight(cfg.Base, "/")
	s.mux.HandleFunc("GET "+base, s.index)
	s.mux.HandleFunc("GET "+base+"/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base, http.StatusMovedPermanently)
	})
	s.mux.Handle("GET "+base+"/static/", http.StripPrefix(base+"/static/", http.HandlerFunc(s.static)))
	if cfg.Pictures {
		s.mux.HandleFunc("GET "+base+"/api/picture", s.picture)
	}
	if s.cfg.RoutingURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/route", s.route)
	}
	if s.cfg.TTSURL != "" {
		s.mux.HandleFunc("GET "+base+"/api/say", s.say)
	}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex")
	s.mux.ServeHTTP(w, r)
}

// asset is the address of a file of the app, with its hash: browsers keep it for a year.
func (s *Server) asset(p string) string {
	return strings.TrimRight(s.cfg.Base, "/") + "/static/" + p + "?v=" + s.hashes[p]
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	data := map[string]any{"Base": strings.TrimRight(s.cfg.Base, "/"), "SearchAPI": s.cfg.SearchAPI, "Tiles": s.cfg.TilesURL,
		"Key": s.cfg.MapsKey, "Pictures": s.cfg.Pictures, "Routes": s.cfg.RoutingURL != "", "Say": s.cfg.TTSURL != "", "Version": s.cfg.Version}
	if err := s.page.Execute(&buf, data); err != nil {
		s.log.Error("page", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Security-Policy", s.csp())
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(self), payment=()")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin") // the tiles key holds for the site only
	w.Write(buf.Bytes())
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
	".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".txt": "text/plain; charset=utf-8", ".png": "image/png"}

// static serves the files of the app. With the hash of the file in v they are kept for a
// year; the libraries in vendor/ have their version in the path.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	p := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	b, err := fs.ReadFile(web.Files, p)
	if err != nil || p == "index.html" || strings.HasPrefix(p, ".") {
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
