package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Live traffic for the map: closures, queues, roadworks and slow stretches, from the traffic
// service next to Valhalla (TrafficURL), which reads the open traffic data of some countries
// (Switzerland: FEDRO, the Netherlands: NDW, France: Bison Futé; /opt/valhalla/traffic/sources).
//
//	GET {base}/api/traffic   {"updated","countries":["CH",...],"sources":{"ch":{"country","credit","updated"}},
//	                          "items":[{"id","src","kind","closed","when","text":{lang},"start","end","lines","at","road","length","delay"}],
//	                          "flow":[{"line","kph","level","src"}]}
//
// This server asks at most once a minute, whoever asks it, and serves the last answer for up to
// 15 minutes when the service fails; 502 after that. The countries of the last answer go into
// the page (data-traffic-countries), for the switch of the menu.

type trafficCache struct {
	mu        sync.Mutex
	body      []byte
	at        time.Time
	try       time.Time
	countries string // of the last answer: "CH,FR,NL"
}

// trafficBody: the last answer, asked again when it is a minute old (at most every 10 s when the service fails)
func (s *Server) trafficBody(ctx context.Context) ([]byte, time.Time) {
	c := &s.trafficCache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if (c.body == nil || now.Sub(c.at) > time.Minute) && now.Sub(c.try) > 10*time.Second {
		c.try = now
		// asked for all visitors: the one whose request starts it may leave meanwhile
		if b, err := s.fetchTraffic(context.WithoutCancel(ctx)); err == nil {
			c.body, c.at = b, now
			var head struct {
				Countries []string `json:"countries"`
			}
			if json.Unmarshal(b, &head) == nil {
				c.countries = strings.Join(head.Countries, ",")
			}
		} else {
			s.log.Warn("traffic", "err", err)
		}
	}
	return c.body, c.at
}

func (s *Server) traffic(w http.ResponseWriter, r *http.Request) {
	body, at := s.trafficBody(r.Context())
	if body == nil || time.Since(at) > 15*time.Minute {
		routeError(w, http.StatusBadGateway, "failed", "No traffic information at the moment.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "public, max-age=60")
	w.Write(body)
}

// trafficCountries: the countries with traffic, for the page; not known yet, it asks the service in the background
// and the page goes without them
func (s *Server) trafficCountries() string {
	if s.cfg.TrafficURL == "" {
		return ""
	}
	c := &s.trafficCache
	c.mu.Lock()
	known := c.countries
	c.mu.Unlock()
	if known == "" {
		go s.trafficBody(context.Background())
	}
	return known
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
