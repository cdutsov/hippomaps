package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// the page at the root of its own site, the search relayed to overpass-search elsewhere
func TestOwnSite(t *testing.T) {
	var got *http.Request
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"code":"too_many"}}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) {
		c.Base = "/"
		c.SearchUpstream = up.URL + "/labs/search/api/"
		c.PublicURL = "https://hippomaps.example"
	})
	w := get(s, "/")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `data-api="/api/search"`) || !strings.Contains(w.Body.String(), `/static/app.js?v=`) {
		t.Fatalf("page at the root: %d %.300s", w.Code, w.Body)
	}
	if w.Header().Get("X-Robots-Tag") != "" || !strings.Contains(w.Body.String(), `<link rel="canonical" href="https://hippomaps.example/">`) || strings.Contains(w.Body.String(), "noindex") {
		t.Errorf("the page of its own site is for search engines: %v", w.Header().Get("X-Robots-Tag"))
	}
	if w := get(s, "/robots.txt"); w.Code != 200 || !strings.Contains(w.Body.String(), "Sitemap: https://hippomaps.example/sitemap.xml") || !strings.Contains(w.Body.String(), "Disallow: /api/") {
		t.Errorf("robots.txt: %d %s", w.Code, w.Body)
	}
	if w := get(s, "/sitemap.xml"); w.Code != 200 || !strings.Contains(w.Body.String(), "<loc>https://hippomaps.example/</loc>") {
		t.Errorf("sitemap.xml: %d %s", w.Code, w.Body)
	}
	if w := get(s, "/api/traffic"); w.Header().Get("X-Robots-Tag") != "noindex" {
		t.Errorf("the API is not for search engines")
	}
	if w := get(s, "/static/app.js"); w.Code != 200 {
		t.Errorf("script at the root: %d", w.Code)
	}
	if w := get(s, "/nothing"); w.Code != 404 {
		t.Errorf("another path: %d", w.Code)
	}
	w = get(s, "/api/search/complete?q=Bern&lang=de", "X-Forwarded-For", "203.0.113.7")
	if w.Code != 429 || !strings.Contains(w.Body.String(), "too_many") || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("relayed answer: %d %s", w.Code, w.Body)
	}
	if got == nil || got.URL.Path != "/labs/search/api/complete" || got.URL.RawQuery != "q=Bern&lang=de" || got.Header.Get("X-Forwarded-For") != "203.0.113.7" {
		t.Errorf("relayed request: %+v", got)
	}
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(get(s, "/").Header().Get("Content-Security-Policy"), up.URL) {
		t.Errorf("the search host in the CSP: %s", csp)
	}
}

// SearchesPerIP: searches of a visitor in RouteWindow, three times as many taps; completions free;
// other paths not relayed
func TestSearchLimit(t *testing.T) {
	var calls int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL + "/labs"; c.SearchesPerIP = 2 })
	ip := func(w *httptest.ResponseRecorder) int { return w.Code }
	for i, want := range []int{200, 200, 429} {
		if c := ip(get(s, "/api/search/search?q=x", "X-Forwarded-For", "203.0.113.7")); c != want {
			t.Errorf("search %d: %d, want %d", i, c, want)
		}
	}
	if c := ip(get(s, "/api/search/query?q=x", "X-Forwarded-For", "203.0.113.7")); c != 429 {
		t.Errorf("query after the searches: %d", c)
	}
	if w := get(s, "/api/search/search?q=x", "X-Forwarded-For", "203.0.113.7"); !strings.Contains(w.Body.String(), `"too_many"`) {
		t.Errorf("answer over the limit: %s", w.Body)
	}
	// a search of the page asks query, then search: one search of the visitor, not two
	for i, want := range []int{200, 200, 200, 200, 429} {
		p := []string{"query", "search"}[i%2]
		if c := ip(get(s, "/api/search/"+p+"?q=x", "X-Forwarded-For", "203.0.113.9")); c != want {
			t.Errorf("a search of the page, call %d (%s): %d, want %d", i, p, c, want)
		}
	}
	// query alone has its own count, as many
	for i, want := range []int{200, 200, 429} {
		if c := ip(get(s, "/api/search/query?q=x", "X-Forwarded-For", "203.0.113.10")); c != want {
			t.Errorf("query %d: %d, want %d", i, c, want)
		}
	}
	if c := ip(get(s, "/api/search/search?q=x", "X-Forwarded-For", "203.0.113.8")); c != 200 {
		t.Errorf("another visitor: %d", c)
	}
	for i := 0; i < 10; i++ {
		if c := ip(get(s, "/api/search/complete?q=x", "X-Forwarded-For", "203.0.113.7")); c != 200 {
			t.Fatalf("completion %d: %d", i, c)
		}
	}
	for i := 0; i < 7; i++ {
		c := ip(get(s, "/api/search/here?lat=1&lon=1", "X-Forwarded-For", "203.0.113.7"))
		if want := map[bool]int{true: 200, false: 429}[i < 6]; c != want {
			t.Errorf("tap %d: %d, want %d", i, c, want)
		}
	}
	n := calls
	if c := ip(get(s, "/api/search/status", "X-Forwarded-For", "203.0.113.9")); c != 404 || calls != n {
		t.Errorf("another path: %d, relayed %v", c, calls != n)
	}
}

// a search that takes longer than the server gives an answer (WriteTimeout) still gets its answer
func TestSearchSlow(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"understood":"Restaurant, in the view"}`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL })
	ts := httptest.NewUnstartedServer(s)
	ts.Config.WriteTimeout = 200 * time.Millisecond
	ts.Start()
	t.Cleanup(ts.Close)
	for _, u := range []string{"/api/search/search?q=restaurants", "/api/search/search?q=restaurants&lang=de"} {
		resp, err := http.Get(ts.URL + u)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(b), "Restaurant, ") {
			t.Errorf("%s: %d %q", u, resp.StatusCode, b)
		}
	}
}

// overpass-search does not answer: 502 with the error as JSON (it was sent as text/plain)
func TestSearchUpstreamDown(t *testing.T) {
	up := httptest.NewServer(http.NotFoundHandler())
	up.Close()
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL; c.SearchesPerIP = 1 })
	w := get(s, "/api/search/search?q=x&lang=de")
	if w.Code != 502 || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), `"code":"failed"`) {
		t.Errorf("search without overpass-search: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	w = get(s, "/api/search/search?q=x")
	if w.Code != 429 || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Retry-After") != "600" {
		t.Errorf("over the limit: %d %v", w.Code, w.Header())
	}
}

// an answer of overpass-search that is not 200, or not JSON, goes back as it came, also with lang
func TestSearchAnswerAsItCame(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "bad" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"code":"bad_request","message":"Restaurant, in the view"}}`)
			return
		}
		w.Header().Set("Cache-Control", "max-age=60")
		io.WriteString(w, `<html>Restaurant, in the view`)
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL })
	if w := get(s, "/api/search/search?q=bad&lang=de"); w.Code != 400 || !strings.Contains(w.Body.String(), "Restaurant, in the view") {
		t.Errorf("an error of overpass-search: %d %s", w.Code, w.Body)
	}
	if w := get(s, "/api/search/search?q=x&lang=de"); w.Code != 200 || w.Body.String() != `<html>Restaurant, in the view` || w.Header().Get("Cache-Control") != "max-age=60" {
		t.Errorf("an answer that is not JSON: %d %s", w.Code, w.Body)
	}
}

// the limits count an IPv6 visitor by the /64 of the address: another address of the same network is the
// same visitor, another network is not; an IPv4 address counts as it is
func TestLimitPerNetwork(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL; c.SearchesPerIP = 2 })
	for i, c := range []struct {
		ip   string
		want int
	}{
		{"2001:db8:1:2::1", 200}, {"2001:db8:1:2:aaaa:bbbb:cccc:dddd", 200}, {"2001:db8:1:2::3", 429},
		{"2001:db8:1:3::1", 200},
		{"203.0.113.7", 200}, {"203.0.113.8", 200}, {"203.0.113.7", 200}, {"203.0.113.7", 429}, {"::ffff:203.0.113.8", 200}, {"::ffff:203.0.113.8", 200},
	} {
		if w := get(s, "/api/search/search?q=x", "X-Forwarded-For", c.ip); w.Code != c.want {
			t.Errorf("search %d from %s: %d, want %d", i, c.ip, w.Code, c.want)
		}
	}
	for ip, want := range map[string]string{"2001:db8:1:2::1": "2001:db8:1:2::/64", "203.0.113.7": "203.0.113.7", "not an address": "not an address"} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-For", ip+", 198.51.100.1")
		if got := visitor(r); got != want {
			t.Errorf("visitor %s: %s, want %s", ip, got, want)
		}
	}
}
