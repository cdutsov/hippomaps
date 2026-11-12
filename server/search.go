package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"
)

// search relays GET {base}/api/search/{complete,query,search,...}?… to SearchUpstream with the
// address of the visitor in X-Forwarded-For (its limits count per visitor, and the server in front
// of it trusts this one). The answer goes back as it came, with its status.
//
// Each visitor gets SearchesPerIP searches (search) in RouteWindow and three times as many taps on the map (here);
// completions while typing are not counted. The page asks how a search is read (query) before each search: that
// counts apart, as many again, so a search of the page is one search; with the searches used up it is refused too.
// Other paths are not relayed.
// With lang=de, fr or it the English words of the answer are put in that language (labels.go).
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	var lim *window
	now, who := time.Now(), visitor(r)
	switch r.PathValue("rest") {
	case "search":
		lim = s.searchesPerIP
	case "query":
		lim = s.queriesPerIP
		if s.searchesPerIP.full(who, now) {
			lim = s.searchesPerIP // refused as a search is
		}
	case "here":
		lim = s.herePerIP
	case "complete":
	default:
		http.NotFound(w, r)
		return
	}
	if lim != nil && !lim.allow(who, now) {
		w.Header().Set("Retry-After", "600")
		routeError(w, http.StatusTooManyRequests, "too_many", "Too many searches. Please wait a few minutes.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 80*time.Second) // a search may take a minute
	defer cancel()
	// the time to write an answer (WriteTimeout of the server in cmd/hippomaps, 40 s) counts from the request:
	// without more of it the answer of a search that took longer is cut off
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(90 * time.Second))
	u := s.cfg.SearchUpstream + "/" + r.PathValue("rest")
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	if v := r.Header.Get("Accept-Language"); v != "" {
		req.Header.Set("Accept-Language", v)
	}
	ip := clientIP(r)
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := s.searchClient.Do(req)
	if err != nil {
		s.log.Warn("search", "err", err)
		routeError(w, http.StatusBadGateway, "failed", "The search failed. Please try again.")
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Cache-Control", "Retry-After"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// the words of overpass-search in the language of the page (labels.go)
	if lang := r.URL.Query().Get("lang"); resp.StatusCode == http.StatusOK && langs[lang] != 0 {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			routeError(w, http.StatusBadGateway, "failed", "The search failed. Please try again.")
			return
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(translate(body, r.PathValue("rest"), lang))
		return
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, 32<<20))
}
