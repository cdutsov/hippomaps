package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Traffic in Switzerland for the map: closures, queues, roadworks and slow stretches, from the
// traffic service next to Valhalla (TrafficURL), which reads
// the Traffic Data Platform of FEDRO.
//
//	GET {base}/api/traffic   {"updated","source","items":[{"id","kind","closed","when","text":{"de","fr","it"},"start","end","lines","at"}],"flow":[{"line","kph","level"}]}
//
// This server asks at most once a minute, whoever asks it, and serves the last answer for up to
// 15 minutes when the service fails; 502 after that.

type trafficCache struct {
	mu   sync.Mutex
	body []byte
	at   time.Time
	try  time.Time
}

func (s *Server) traffic(w http.ResponseWriter, r *http.Request) {
	c := &s.trafficCache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if (c.body == nil || now.Sub(c.at) > time.Minute) && now.Sub(c.try) > 10*time.Second {
		c.try = now
		// asked for all visitors: the one whose request starts it may leave meanwhile
		if b, err := s.fetchTraffic(context.WithoutCancel(r.Context())); err == nil {
			c.body, c.at = b, now
		} else {
			s.log.Warn("traffic", "err", err)
		}
	}
	if c.body == nil || now.Sub(c.at) > 15*time.Minute {
		routeError(w, http.StatusBadGateway, "failed", "No traffic information at the moment.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "public, max-age=60")
	w.Write(c.body)
}

func (s *Server) fetchTraffic(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.TrafficURL+"/incidents.json", nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.routeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(b[:min(len(b), 16)])), "{") {
		return nil, &httpError{resp.StatusCode}
	}
	return b, nil
}

type httpError struct{ code int }

func (e *httpError) Error() string { return "HTTP " + http.StatusText(e.code) }
