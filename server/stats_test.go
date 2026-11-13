package server

import (
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const browser = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/156.0 Safari/537.36"

func visit(st *stats, ip, ua, ref, query string) {
	r := httptest.NewRequest("GET", "/"+query, nil)
	r.Host = "hippomaps.ch"
	r.RemoteAddr = ip + ":1234"
	r.Header.Set("User-Agent", ua)
	if ref != "" {
		r.Header.Set("Referer", ref)
	}
	st.record(r, "hippomaps.ch")
}

func newTestStats(t *testing.T, dir string, ignore ...string) *stats {
	t.Helper()
	st := newStats(dir, ignore, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	st.now = func() time.Time { return time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(st.Close)
	return st
}

func TestStatsCounts(t *testing.T) {
	st := newTestStats(t, t.TempDir(), "203.0.113.0/24", "198.51.100.7")
	visit(st, "192.0.2.1", browser, "https://news.example/story?id=1", "")
	visit(st, "192.0.2.1", browser, "https://hippomaps.ch/", "") // the same visitor again, from our own page
	visit(st, "192.0.2.2", browser, "", "")
	visit(st, "192.0.2.3", browser, "https://www.example.org/x", "?ref=Newsletter")
	visit(st, "192.0.2.4", browser, "not a url", "?ref=bad%20host!")
	// not counted: bots, tests, our own addresses
	visit(st, "192.0.2.9", "Googlebot/2.1", "", "")
	visit(st, "192.0.2.9", "Mozilla/5.0 HeadlessChrome/156.0", "", "")
	visit(st, "192.0.2.9", "curl/8.0", "", "")
	visit(st, "192.0.2.9", "", "", "")
	visit(st, "203.0.113.5", browser, "", "")
	visit(st, "198.51.100.7", browser, "", "")
	d := st.Snapshot()["2026-11-13"]
	if d.Visits != 5 || d.Visitors != 4 {
		t.Fatalf("visits %d visitors %d, want 5 and 4", d.Visits, d.Visitors)
	}
	want := map[string]int{"news.example": 1, refDirect: 3, "newsletter": 1}
	for h, n := range want {
		if d.Referrers[h] != n {
			t.Errorf("referrer %q: %d, want %d (%v)", h, d.Referrers[h], n, d.Referrers)
		}
	}
}

// An IPv6 visitor has every address of the network: one visitor for the /64.
func TestStatsIPv6Network(t *testing.T) {
	st := newTestStats(t, t.TempDir())
	visit(st, "[2001:db8:1:2::1]", browser, "", "")
	visit(st, "[2001:db8:1:2::ffff]", browser, "", "")
	if d := st.Snapshot()["2026-11-13"]; d.Visits != 2 || d.Visitors != 1 {
		t.Fatalf("visits %d visitors %d, want 2 and 1", d.Visits, d.Visitors)
	}
}

// The counts last over a restart, and a visitor is counted once a day, not forever.
func TestStatsPersist(t *testing.T) {
	dir := t.TempDir()
	st := newTestStats(t, dir)
	visit(st, "192.0.2.1", browser, "https://a.example/", "")
	st.Close()
	if b, _ := os.ReadFile(filepath.Join(dir, statsFile)); strings.Contains(string(b), "192.0.2") {
		t.Fatalf("an address in the file: %s", b)
	}
	st2 := newTestStats(t, dir)
	visit(st2, "192.0.2.2", browser, "https://a.example/", "")
	d := st2.Snapshot()["2026-11-13"]
	if d.Visits != 2 || d.Referrers["a.example"] != 2 {
		t.Fatalf("after restart: %+v", d)
	}
	st2.now = func() time.Time { return time.Date(2026, 11, 14, 0, 0, 1, 0, time.UTC) }
	visit(st2, "192.0.2.2", browser, "", "")
	s := st2.Snapshot()
	if s["2026-11-13"].Visits != 2 || s["2026-11-14"].Visits != 1 || s["2026-11-14"].Visitors != 1 {
		t.Fatalf("next day: %+v", s)
	}
	if len(st2.seen) != 1 {
		t.Fatalf("visitors of yesterday kept: %d", len(st2.seen))
	}
}

func TestStatsBadFileKept(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, statsFile), []byte("{broken"), 0o644)
	st := newTestStats(t, dir)
	visit(st, "192.0.2.1", browser, "", "")
	if _, err := os.Stat(filepath.Join(dir, statsFile+".bad")); err != nil {
		t.Fatal("the unreadable file was not kept:", err)
	}
	if st.Snapshot()["2026-11-13"].Visits != 1 {
		t.Fatal("no counting after an unreadable file")
	}
}

func TestStatsReferrerCap(t *testing.T) {
	st := newTestStats(t, t.TempDir())
	for i := 0; i < maxReferrers+20; i++ {
		visit(st, "192.0.2.1", browser, "", "?ref=site"+strings.Repeat("a", i%50)+string(rune('a'+i%26))+strings.Repeat("b", i/26))
	}
	d := st.Snapshot()["2026-11-13"]
	if len(d.Referrers) > maxReferrers+1 || d.Referrers[refOther] == 0 {
		t.Fatalf("%d referrers, other %d", len(d.Referrers), d.Referrers[refOther])
	}
}

// The page counts a load; the tiles, the API and the other pages do not.
func TestStatsOnlyThePage(t *testing.T) {
	dir := t.TempDir()
	s := testServer(t, nil, func(c *Config) { c.StatsDir = dir; c.Base = "/" })
	get := func(path string) {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("User-Agent", browser)
		r.Header.Set("Referer", "https://example.net/a")
		s.ServeHTTP(httptest.NewRecorder(), r)
	}
	get("/")
	get("/privacy")
	get("/healthz")
	get("/static/app.js")
	var visits int
	for _, d := range s.stats.Snapshot() {
		visits += d.Visits
	}
	if visits != 1 {
		t.Fatalf("%d visits, want 1", visits)
	}
	s.Close()
	days, err := ReadStats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out := FormatStats(days, 7); !strings.Contains(out, "example.net 1") || !strings.Contains(out, "total") {
		t.Fatalf("table:\n%s", out)
	}
}

func TestStatsOff(t *testing.T) {
	s := testServer(t, nil, func(c *Config) { c.Base = "/" })
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("User-Agent", browser)
	if w := httptest.NewRecorder(); func() int { s.ServeHTTP(w, r); return w.Code }() != 200 {
		t.Fatal("page without counting")
	}
	s.Close()
}
