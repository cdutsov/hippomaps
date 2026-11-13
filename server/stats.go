package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Counts of visits, kept over restarts: for each day (UTC) the page loads, the visitors and the sites they came
// from. No cookie and no address is written: a visitor is counted once a day by a hash of address and browser with a
// salt of that day, in memory only, and dropped at midnight. What reaches the file are numbers and host names.

const (
	statsFile      = "stats.json"
	maxReferrers   = 200 // host names a day; the rest are counted as "(other)"
	refDirect      = "(direct)"
	refOther       = "(other)"
	statsFlushWait = time.Minute
)

// DayStats are the counts of one day.
type DayStats struct {
	Visits    int            `json:"visits"`              // page loads
	Visitors  int            `json:"visitors"`            // distinct visitors; one who restarts the server in between may count twice
	Referrers map[string]int `json:"referrers,omitempty"` // visits by the site they came from
}

type stats struct {
	dir    string
	log    *slog.Logger
	ignore []netip.Prefix
	now    func() time.Time

	mu    sync.Mutex
	days  map[string]*DayStats
	today string                // the day of seen
	salt  []byte                // of today, in memory only
	seen  map[[16]byte]struct{} // today's visitors
	dirty bool

	stop chan struct{}
	done chan struct{}
}

// botAgent: crawlers, tools and the headless browsers of our tests are no visitors
var botAgent = regexp.MustCompile(`(?i)bot|spider|crawl|slurp|curl|wget|python|go-http|headless|puppeteer|playwright|monitor|uptime|check|preview|facebookexternalhit|scan`)

// hostName: what may stand for a referring site
var hostName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,98}[a-z0-9])?$`)

func newStats(dir string, ignore []string, log *slog.Logger) *stats {
	st := &stats{dir: dir, log: log, now: time.Now, days: map[string]*DayStats{}, seen: map[[16]byte]struct{}{}, stop: make(chan struct{}), done: make(chan struct{})}
	for _, s := range ignore {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			st.ignore = append(st.ignore, p.Masked())
		} else if a, err := netip.ParseAddr(s); err == nil {
			st.ignore = append(st.ignore, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
		} else {
			log.Warn("stats: not an address or network, ignored", "value", s)
		}
	}
	st.load()
	go st.loop()
	return st
}

func (st *stats) load() {
	b, err := os.ReadFile(filepath.Join(st.dir, statsFile))
	if os.IsNotExist(err) {
		return
	}
	if err == nil {
		err = json.Unmarshal(b, &st.days)
	}
	if err != nil { // keep what is there for a look, start again
		st.log.Error("stats: file unreadable, kept as .bad", "err", err)
		os.Rename(filepath.Join(st.dir, statsFile), filepath.Join(st.dir, statsFile+".bad"))
		st.days = map[string]*DayStats{}
	}
}

func (st *stats) loop() {
	defer close(st.done)
	t := time.NewTicker(statsFlushWait)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			st.flush()
		case <-st.stop:
			st.flush()
			return
		}
	}
}

// Close writes the counts and stops the writer.
func (st *stats) Close() {
	if st == nil {
		return
	}
	select {
	case <-st.stop:
	default:
		close(st.stop)
	}
	<-st.done
}

// flush writes the file when something changed: to a new file, then renamed over the old.
func (st *stats) flush() {
	st.mu.Lock()
	if !st.dirty {
		st.mu.Unlock()
		return
	}
	b, err := json.MarshalIndent(st.days, "", " ")
	st.dirty = false
	st.mu.Unlock()
	if err == nil {
		tmp := filepath.Join(st.dir, statsFile+".tmp")
		if err = os.WriteFile(tmp, append(b, '\n'), 0o644); err == nil {
			err = os.Rename(tmp, filepath.Join(st.dir, statsFile))
		}
	}
	if err != nil {
		st.log.Error("stats: write failed", "err", err)
		st.mu.Lock()
		st.dirty = true
		st.mu.Unlock()
	}
}

// record counts a page load.
func (st *stats) record(r *http.Request, own string) {
	if st == nil {
		return
	}
	ua := r.UserAgent()
	if ua == "" || botAgent.MatchString(ua) {
		return
	}
	if p := r.Header.Get("Purpose") + r.Header.Get("Sec-Purpose") + r.Header.Get("X-Moz"); strings.Contains(p, "prefetch") || strings.Contains(p, "prerender") {
		return
	}
	v := visitor(r)
	if a, err := netip.ParseAddr(clientIP(r)); err == nil {
		a = a.Unmap()
		for _, p := range st.ignore {
			if p.Contains(a) {
				return
			}
		}
	}
	ref := referrer(r, own)
	t := st.now().UTC()
	day := t.Format("2006-01-02")

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.today != day { // a new day: a new salt, the visitors of yesterday forgotten
		st.today, st.salt, st.seen = day, make([]byte, 16), map[[16]byte]struct{}{}
		rand.Read(st.salt)
	}
	d := st.days[day]
	if d == nil {
		d = &DayStats{}
		st.days[day] = d
	}
	d.Visits++
	sum := sha256.Sum256(append(append(append([]byte{}, st.salt...), v...), '\n'))
	h := sha256.Sum256(append(sum[:], ua...))
	var key [16]byte
	copy(key[:], h[:])
	if _, ok := st.seen[key]; !ok {
		st.seen[key] = struct{}{}
		d.Visitors++
	}
	if d.Referrers == nil {
		d.Referrers = map[string]int{}
	}
	if _, ok := d.Referrers[ref]; !ok && len(d.Referrers) >= maxReferrers {
		ref = refOther
	}
	d.Referrers[ref]++
	st.dirty = true
}

// referrer is the host of the site that sent the visitor: the address ?ref=name (a link we hand out, a campaign),
// else the Referer header, which browsers cut to the site. Our own site and a missing header are "(direct)".
func referrer(r *http.Request, own string) string {
	q := r.URL.Query()
	for _, k := range []string{"ref", "utm_source"} {
		if v := strings.ToLower(strings.TrimSpace(q.Get(k))); hostName.MatchString(v) {
			return v
		}
	}
	u, err := url.Parse(r.Referer())
	if err != nil || u.Hostname() == "" {
		return refDirect
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if h == own || h == strings.ToLower(r.Host) || !hostName.MatchString(h) {
		return refDirect
	}
	return h
}

// Snapshot is a copy of the counts, by day.
func (st *stats) Snapshot() map[string]DayStats {
	out := map[string]DayStats{}
	st.mu.Lock()
	defer st.mu.Unlock()
	for k, d := range st.days {
		c := *d
		c.Referrers = map[string]int{}
		for h, n := range d.Referrers {
			c.Referrers[h] = n
		}
		out[k] = c
	}
	return out
}

// ReadStats reads the file of a directory.
func ReadStats(dir string) (map[string]DayStats, error) {
	b, err := os.ReadFile(filepath.Join(dir, statsFile))
	if err != nil {
		return nil, err
	}
	days := map[string]DayStats{}
	return days, json.Unmarshal(b, &days)
}

// FormatStats is the table of the last n days, newest first, then the totals and the sites over those days.
func FormatStats(days map[string]DayStats, n int) string {
	keys := make([]string, 0, len(days))
	for k := range days {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	if n > 0 && len(keys) > n {
		keys = keys[:n]
	}
	var b strings.Builder
	var visits, visitors int
	refs := map[string]int{}
	fmt.Fprintf(&b, "%-10s %7s %9s  %s\n", "day (UTC)", "visits", "visitors", "from")
	for _, k := range keys {
		d := days[k]
		visits += d.Visits
		visitors += d.Visitors
		for h, c := range d.Referrers {
			refs[h] += c
		}
		fmt.Fprintf(&b, "%-10s %7d %9d  %s\n", k, d.Visits, d.Visitors, topRefs(d.Referrers, 3))
	}
	fmt.Fprintf(&b, "%-10s %7d %9d  (visitors: the sum of the days, a returning visitor counts each day)\n", "total", visits, visitors)
	fmt.Fprintf(&b, "\nsites over these days: %s\n", topRefs(refs, 15))
	return b.String()
}

func topRefs(m map[string]int, n int) string {
	type kv struct {
		k string
		v int
	}
	l := make([]kv, 0, len(m))
	for k, v := range m {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v || l[i].v == l[j].v && l[i].k < l[j].k })
	if len(l) > n {
		l = l[:n]
	}
	s := make([]string, len(l))
	for i, e := range l {
		s[i] = fmt.Sprintf("%s %d", e.k, e.v)
	}
	return strings.Join(s, ", ")
}
