package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
