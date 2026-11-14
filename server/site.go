package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The website of a place (its website tag): its home page gives a picture (the picture it shows when shared, else its
// logo or icon), a link to its menu, and opening hours in JSON-LD. The server reads it, not the browser: the visitor's
// address goes to no business. Reading follows robots.txt of the site for the product token of USER_AGENT.

// siteDial says whether the site client may connect to an address: public addresses on ports 80 and 443 only, checked
// at the connection itself (after DNS, after each redirect), so no website can lead this server into its own networks.
type siteDial func(netip.AddrPort) bool

var errNotPublic = errors.New("site: not a public address")

// reserved: networks that are not the public internet besides those netip knows (private, loopback, link-local,
// multicast, unspecified)
var reserved = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "2001:db8::/32", "2002::/16", "100::/64"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

func publicDial(ap netip.AddrPort) bool {
	if p := ap.Port(); p != 80 && p != 443 {
		return false
	}
	a := ap.Addr().Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range reserved {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

func newSiteClient(allow siteDial) *http.Client {
	d := &net.Dialer{Timeout: 4 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil || !allow(ap) {
			return errNotPublic
		}
		return nil
	}}
	tr := &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 6 * time.Second,
		MaxIdleConns: 16, IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true}
	return &http.Client{Transport: tr, Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("site: too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("site: redirect off the web")
		}
		return nil
	}}
}

// webURL is the URL of a website tag ("example.ch", "https://example.ch/de"), nil when it is not one
func webURL(v string) *url.URL {
	v = strings.TrimSpace(strings.Split(v, ";")[0])
	if v == "" || strings.ContainsAny(v, " \t\n") {
		return nil
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil
	}
	u.Fragment = ""
	return u
}

// fetchSite reads a URL of the web, up to max bytes; the URL after redirects
func (s *Server) fetchSite(ctx context.Context, u string, max int64, accept string) ([]byte, string, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en, de;q=0.9, fr;q=0.8, it;q=0.8")
	resp, err := s.siteClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // without the URL
		}
		return nil, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("site: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", nil, err
	}
	if int64(len(b)) > max {
		return nil, "", nil, errors.New("site: answer too large")
	}
	typ, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return b, strings.TrimSpace(strings.ToLower(typ)), resp.Request.URL, nil
}

// ------------------------------------------------------------------ robots.txt

// agentToken is the product token of the user agent ("hippomaps" of "hippomaps/0.1 (+https://…)")
func (s *Server) agentToken() string {
	t := strings.ToLower(s.cfg.UserAgent)
	if i := strings.IndexAny(t, "/ "); i > 0 {
		t = t[:i]
	}
	return t
}

// robotsAllow: whether robots.txt of the site of u lets this server read u; kept a day per site. A site without one, or
// whose robots.txt cannot be read, allows; one that answers 5xx does not (RFC 9309).
func (s *Server) robotsAllow(ctx context.Context, u *url.URL) bool {
	key := "robots:" + u.Scheme + "://" + u.Host
	txt, ok := s.cache.get(key)
	if !ok {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.Scheme+"://"+u.Host+"/robots.txt", nil)
		if err != nil {
			return false
		}
		req.Header.Set("User-Agent", s.cfg.UserAgent)
		txt = []byte{}
		if resp, err := s.siteClient.Do(req); err == nil {
			switch {
			case resp.StatusCode == http.StatusOK:
				txt, _ = io.ReadAll(io.LimitReader(resp.Body, 256<<10))
			case resp.StatusCode >= 500:
				txt = []byte("User-agent: *\nDisallow: /\n")
			}
			resp.Body.Close()
		}
		s.cache.put(key, txt)
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return robotsAllows(string(txt), s.agentToken(), path)
}

// robotsAllows reads robots.txt (RFC 9309): the groups of the agent, else those of *; the longest rule that matches the
// path wins, Allow over Disallow when as long; * and $ in rules
func robotsAllows(txt, agent, path string) bool {
	type rule struct {
		allow bool
		pat   string
	}
	var mine, star []rule
	var agents []string
	inRules := false
	for _, line := range strings.Split(txt, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if inRules {
				agents, inRules = nil, false
			}
			agents = append(agents, strings.ToLower(v))
		case "allow", "disallow":
			inRules = true
			if v == "" {
				continue // "Disallow:" allows all
			}
			r := rule{allow: k == "allow", pat: v}
			for _, a := range agents {
				if a == agent || (a != "*" && strings.HasPrefix(agent, a)) {
					mine = append(mine, r)
				} else if a == "*" {
					star = append(star, r)
				}
			}
		}
	}
	rules := mine
	if len(rules) == 0 {
		rules = star
	}
	best, allow := -1, true
	for _, r := range rules {
		if robotsMatch(r.pat, path) && (len(r.pat) > best || (len(r.pat) == best && r.allow)) {
			best, allow = len(r.pat), r.allow
		}
	}
	return allow
}

func robotsMatch(pat, path string) bool {
	end := strings.HasSuffix(pat, "$")
	pat = strings.TrimSuffix(pat, "$")
	var rx strings.Builder
	rx.WriteString("^")
	for i, part := range strings.Split(pat, "*") {
		if i > 0 {
			rx.WriteString(".*")
		}
		rx.WriteString(regexp.QuoteMeta(part))
	}
	if end {
		rx.WriteString("$")
	}
	re, err := regexp.Compile(rx.String())
	return err == nil && re.MatchString(path)
}

// ------------------------------------------------------------------ the home page

// sitePicture is a picture a page names: of a share (photo, unless its pixels say logo), or its logo or icon (logo)
type sitePicture struct {
	URL  string
	Logo bool
}

// sitePage is what the home page of a place says
type sitePage struct {
	Pictures []sitePicture // best first
	Menu     string        // a link to the menu
	Hours    string        // opening hours of its JSON-LD, in the syntax of OpenStreetMap
}

type anchor struct{ href, text string }

// scanHTML walks the tags of a page: each start tag with its attributes (names in lower case, values unescaped), the
// text of each link, and the content of each script of JSON-LD. Comments, other scripts and styles are skipped.
func scanHTML(b string, tag func(name string, attrs map[string]string), link func(anchor), jsonld func(string)) {
	var a *anchor
	i := 0
	for i < len(b) {
		lt := strings.IndexByte(b[i:], '<')
		if lt < 0 {
			break
		}
		if a != nil {
			a.text += b[i : i+lt]
		}
		i += lt
		switch {
		case strings.HasPrefix(b[i:], "<!--"):
			end := strings.Index(b[i+4:], "-->")
			if end < 0 {
				return
			}
			i += 4 + end + 3
			continue
		case strings.HasPrefix(b[i:], "<!") || strings.HasPrefix(b[i:], "<?"):
			end := strings.IndexByte(b[i:], '>')
			if end < 0 {
				return
			}
			i += end + 1
			continue
		case strings.HasPrefix(b[i:], "</"):
			j := i + 2
			for j < len(b) && isNameByte(b[j]) {
				j++
			}
			if strings.EqualFold(b[i+2:j], "a") && a != nil {
				link(*a)
				a = nil
			}
			end := strings.IndexByte(b[j:], '>')
			if end < 0 {
				return
			}
			i = j + end + 1
			continue
		}
		j := i + 1
		for j < len(b) && isNameByte(b[j]) {
			j++
		}
		name := strings.ToLower(b[i+1 : j])
		if name == "" {
			if a != nil {
				a.text += "<"
			}
			i++
			continue
		}
		attrs, k := scanAttrs(b, j)
		i = k
		tag(name, attrs)
		switch name {
		case "a":
			if a != nil {
				link(*a)
			}
			a = &anchor{href: attrs["href"]}
		case "script", "style", "textarea", "title", "noscript", "template":
			end := indexFold(b[i:], "</"+name)
			if end < 0 {
				end = len(b) - i
			}
			if name == "script" && strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") {
				jsonld(b[i : i+end])
			}
			i += end
		}
	}
	if a != nil {
		link(*a)
	}
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':'
}

func indexFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), sub) // the tags named are ASCII: the offsets stay those of s
}

// scanAttrs reads the attributes of a tag from b[i:] up to its >; the index after it
func scanAttrs(b string, i int) (map[string]string, int) {
	attrs := map[string]string{}
	for i < len(b) {
		for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r' || b[i] == '/') {
			i++
		}
		if i >= len(b) {
			break
		}
		if b[i] == '>' {
			return attrs, i + 1
		}
		j := i
		for j < len(b) && !strings.ContainsRune(" \t\n\r/>=", rune(b[j])) {
			j++
		}
		name := strings.ToLower(b[i:j])
		i = j
		for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
			i++
		}
		val := ""
		if i < len(b) && b[i] == '=' {
			i++
			for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
				i++
			}
			if i < len(b) && (b[i] == '"' || b[i] == '\'') {
				q := b[i]
				end := strings.IndexByte(b[i+1:], q)
				if end < 0 {
					return attrs, len(b)
				}
				val = b[i+1 : i+1+end]
				i += end + 2
			} else {
				j := i
				for j < len(b) && !strings.ContainsRune(" \t\n\r>", rune(b[j])) {
					j++
				}
				val = b[i:j]
				i = j
			}
		}
		if name != "" {
			if _, seen := attrs[name]; !seen {
				attrs[name] = html.UnescapeString(val)
			}
		}
	}
	return attrs, len(b)
}

// menu words: a link whose text is one of these leads to the menu (German, French, Italian, English, Bulgarian); "Karte"
// and "carte" alone may be a map: only as a whole text
var (
	menuText = regexp.MustCompile(`(?i)^(?:(?:unsere|die|our|the|la|le|notre|nos|il|la nostra|il nostro|наше(?:то)?|food|lunch|dinner|drinks?)\s+)?` +
		`(?:speise-?karten?|getränke-?karten?|wein-?karten?|menü-?karten?|menu-?karten?|tages-?karten?|mittags-?karten?|mittags-?menüs?|tages-?menüs?|` +
		`menus?|menüs?|menù|menue|carte|cartes|carta|karte|меню(?:то)?)(?:\s*(?:&|und|and|et|e)\s*(?:getränke|drinks|boissons|bevande|vins?|wein|preise))?\s*(?:\(pdf\))?$`)
	menuHref  = regexp.MustCompile(`(?i)(?:speise-?karte|getraenke-?karte|getränke-?karte|wein-?karte|menu-?karte|menue|menü|menu|la-carte|lacarte|carta|%d0%bc%d0%b5%d0%bd%d1%8e)`)
	drinkText = regexp.MustCompile(`(?i)(getränke|wein|drinks?|wine|vins?|boissons|bevande|cocktail)`) // a list of drinks: after a menu of food
	social    = regexp.MustCompile(`(?i)(^|\.)(facebook|instagram|tiktok|twitter|x|youtube|linkedin|pinterest|tripadvisor|google)\.[a-z.]+$`)
)

// readPage reads a home page: the pictures it names, the link to its menu, its opening hours. base: the URL of the page.
func readPage(body string, base *url.URL) sitePage {
	page := base
	var shares, jsonImages, logos []string
	type icon struct {
		href string
		size int
	}
	var icons []icon
	var anchors []anchor
	var lds []string
	meta := map[string]string{}
	scanHTML(body, func(name string, a map[string]string) {
		switch name {
		case "base":
			if h := a["href"]; h != "" {
				if u, err := base.Parse(h); err == nil {
					base = u
				}
			}
		case "meta":
			k := strings.ToLower(a["property"])
			if k == "" {
				k = strings.ToLower(a["name"])
			}
			if k != "" && a["content"] != "" {
				if _, seen := meta[k]; !seen {
					meta[k] = a["content"]
				}
			}
		case "link":
			rel := " " + strings.ToLower(a["rel"]) + " "
			if strings.Contains(rel, " apple-touch-icon") {
				icons = append(icons, icon{a["href"], 1000 + iconSize(a["sizes"])}) // opaque, made for a home screen
			} else if strings.Contains(rel, " icon ") {
				if n := iconSize(a["sizes"]); n >= 96 {
					icons = append(icons, icon{a["href"], n})
				}
			}
		}
	}, func(x anchor) { anchors = append(anchors, x) }, func(s string) { lds = append(lds, s) })

	for _, k := range []string{"og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src"} {
		if v := meta[k]; v != "" {
			shares = append(shares, v)
		}
	}
	ld := readJSONLD(lds)
	jsonImages, logos = ld.images, ld.logos
	sort.SliceStable(icons, func(i, j int) bool { return icons[i].size > icons[j].size })

	var p sitePage
	seen := map[string]bool{}
	add := func(h string, logo bool) {
		u, err := base.Parse(strings.TrimSpace(h))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.HasSuffix(strings.ToLower(u.Path), ".svg") || seen[u.String()] {
			return
		}
		seen[u.String()] = true
		p.Pictures = append(p.Pictures, sitePicture{URL: u.String(), Logo: logo})
	}
	for _, h := range shares {
		add(h, false)
	}
	for _, h := range jsonImages {
		add(h, false)
	}
	for _, h := range logos {
		add(h, true)
	}
	for _, ic := range icons {
		add(ic.href, true)
	}
	p.Hours = ld.hours
	p.Menu = menuLink(ld.menu, anchors, base, page)
	return p
}

func iconSize(sizes string) int {
	n := 0
	for _, s := range strings.Fields(strings.ToLower(sizes)) {
		w, _, _ := strings.Cut(s, "x")
		if v, err := strconv.Atoi(w); err == nil && v > n {
			n = v
		}
	}
	return n
}

// menuLink: the menu of JSON-LD, else the link whose text says menu, else one whose address does; "" when none
func menuLink(ld string, anchors []anchor, base, page *url.URL) string {
	path := func(u *url.URL) string {
		if u.Path == "" {
			return "/"
		}
		return u.Path
	}
	ok := func(h string) string {
		u, err := base.Parse(strings.TrimSpace(h))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || social.MatchString(u.Hostname()) {
			return ""
		}
		if u.Host == page.Host && path(u) == path(page) && u.RawQuery == page.RawQuery {
			return "" // the page itself: a link that opens the navigation of the site
		}
		u.Fragment = ""
		return u.String()
	}
	if v := ok(ld); ld != "" && v != "" {
		return v
	}
	byHref, drinks := "", ""
	for _, a := range anchors {
		if a.href == "" || strings.HasPrefix(a.href, "#") {
			continue
		}
		text := strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(a.text, " "))), " ")
		if text != "" && len([]rune(text)) <= 40 && menuText.MatchString(text) {
			if v := ok(a.href); v != "" && !drinkText.MatchString(text) {
				return v
			} else if drinks == "" {
				drinks = v
			}
		}
		if byHref == "" {
			if u, err := base.Parse(a.href); err == nil && u.Host == page.Host && menuHref.MatchString(u.EscapedPath()) {
				byHref = ok(a.href)
			}
		}
	}
	if drinks != "" {
		return drinks
	}
	return byHref
}

// ------------------------------------------------------------------ JSON-LD

type jsonLD struct {
	images, logos []string
	menu, hours   string
}

// readJSONLD reads the scripts of JSON-LD of a page: the pictures and logo of the business (a thing with an address or
// opening hours), its menu, its opening hours
func readJSONLD(scripts []string) jsonLD {
	var out jsonLD
	var walk func(x any)
	str := func(x any) []string { // a URL, an ImageObject, or a list of them
		var r []string
		switch v := x.(type) {
		case string:
			r = append(r, v)
		case map[string]any:
			for _, k := range []string{"url", "contentUrl", "@id"} {
				if s, ok := v[k].(string); ok && s != "" {
					return append(r, s)
				}
			}
		case []any:
			for _, e := range v {
				r = append(r, strFirst(e)...)
			}
		}
		return r
	}
	walk = func(x any) {
		switch v := x.(type) {
		case []any:
			for _, e := range v {
				walk(e)
			}
		case map[string]any:
			_, addr := v["address"]
			oh, hasOH := v["openingHours"]
			spec, hasSpec := v["openingHoursSpecification"]
			if addr || hasOH || hasSpec {
				out.images = append(out.images, str(v["image"])...)
				out.logos = append(out.logos, str(v["logo"])...)
				if out.hours == "" && hasOH {
					out.hours = hoursOf(oh)
				}
				if out.hours == "" && hasSpec {
					out.hours = hoursOfSpec(spec)
				}
			} else if t := fmt.Sprint(v["@type"]); strings.Contains(t, "Organization") {
				out.logos = append(out.logos, str(v["logo"])...)
			}
			for _, k := range []string{"hasMenu", "menu"} {
				if m := str(v[k]); out.menu == "" && len(m) > 0 {
					out.menu = m[0]
				}
			}
			for k, e := range v {
				if k != "image" && k != "logo" {
					walk(e)
				}
			}
		}
	}
	for _, s := range scripts {
		var x any
		if json.Unmarshal([]byte(strings.TrimSpace(s)), &x) == nil {
			walk(x)
		}
	}
	return out
}

func strFirst(x any) []string {
	switch v := x.(type) {
	case string:
		return []string{v}
	case map[string]any:
		for _, k := range []string{"url", "contentUrl", "@id"} {
			if s, ok := v[k].(string); ok && s != "" {
				return []string{s}
			}
		}
	}
	return nil
}

var (
	ohDays  = []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	ohRule  = regexp.MustCompile(`^((?:Mo|Tu|We|Th|Fr|Sa|Su)(?:\s*[-,]\s*(?:Mo|Tu|We|Th|Fr|Sa|Su))*)((?:\s+\d{1,2}:\d\d\s*-\s*\d{1,2}:\d\d(?:\s*,\s*\d{1,2}:\d\d\s*-\s*\d{1,2}:\d\d)*)?)$`)
	ohSplit = regexp.MustCompile(`(\d:\d\d)\s*,\s*(Mo|Tu|We|Th|Fr|Sa|Su)\b`) // "…18:00, Sa 09:00-14:00": two rules
)

// hoursOf: openingHours of schema.org ("Mo-Fr 09:00-18:00", or a list of such) in the syntax of OpenStreetMap; "" when a
// rule is not of that form. The rules of schema.org add up ("Mo-Sa 11:00-14:30", "Mo-Th 17:00-21:30"): they are read
// per day, then written as rules that do not overlap (in OpenStreetMap a later rule replaces an earlier one).
func hoursOf(x any) string {
	var rules []string
	switch v := x.(type) {
	case string:
		rules = strings.Split(ohSplit.ReplaceAllString(v, "$1;$2"), ";")
	case []any:
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return ""
			}
			rules = append(rules, strings.Split(ohSplit.ReplaceAllString(s, "$1;$2"), ";")...)
		}
	default:
		return ""
	}
	times := make([][]string, 7)
	for _, r := range rules {
		r = strings.Join(strings.Fields(r), " ")
		if r == "" {
			continue
		}
		m := ohRule.FindStringSubmatch(r)
		if m == nil {
			return ""
		}
		days := dayList(m[1])
		ranges := []string{"00:00-24:00"}
		if t := strings.TrimSpace(m[2]); t != "" {
			ranges = nil
			for _, rg := range strings.Split(t, ",") {
				o, c, _ := strings.Cut(rg, "-")
				o, c = clock(o), clock(c)
				if o == "" || c == "" {
					return ""
				}
				ranges = append(ranges, o+"-"+c)
			}
		}
		for _, d := range days {
			times[d] = append(times[d], ranges...)
		}
	}
	return groupDays(times)
}

// dayList: the days of "Mo-Fr", "Mo,We,Fr", "Fr-Mo" (over the weekend)
func dayList(s string) []int {
	var out []int
	for _, part := range strings.Split(strings.ReplaceAll(s, " ", ""), ",") {
		a, b, rng := strings.Cut(part, "-")
		i, j := dayIndex(a), dayIndex(a)
		if rng {
			j = dayIndex(b)
		}
		for k := i; ; k = (k + 1) % 7 {
			out = append(out, k)
			if k == j {
				break
			}
		}
	}
	return out
}

// groupDays writes the times of each day (Monday first) as rules, days with the same times together
func groupDays(times [][]string) string {
	key := make([]string, 7)
	for i, t := range times {
		seen := map[string]bool{}
		var u []string
		for _, r := range t {
			if !seen[r] {
				seen[r] = true
				u = append(u, r)
			}
		}
		sort.Strings(u)
		key[i] = strings.Join(u, ",")
	}
	var rules []string
	for i := 0; i < 7; {
		if key[i] == "" {
			i++
			continue
		}
		j := i
		for j+1 < 7 && key[j+1] == key[i] {
			j++
		}
		days := ohDays[i]
		if j == i+1 {
			days += "," + ohDays[j]
		} else if j > i {
			days += "-" + ohDays[j]
		}
		rules = append(rules, days+" "+key[i])
		i = j + 1
	}
	return strings.Join(rules, "; ")
}

// hoursOfSpec: openingHoursSpecification of schema.org in the syntax of OpenStreetMap; entries for some dates only
// (validFrom, validThrough) and public holidays are left out
func hoursOfSpec(x any) string {
	var list []any
	switch v := x.(type) {
	case []any:
		list = v
	case map[string]any:
		list = []any{v}
	default:
		return ""
	}
	times := make([][]string, 7)
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return ""
		}
		if m["validFrom"] != nil || m["validThrough"] != nil {
			continue
		}
		opens, ok1 := m["opens"].(string)
		closes, ok2 := m["closes"].(string)
		o, c := clock(opens), clock(closes)
		if !ok1 || !ok2 || o == "" || c == "" {
			return ""
		}
		if o == c && o == "00:00" {
			continue // closed
		}
		if c == "23:59" || (c == "00:00" && o != "00:00") {
			c = "24:00"
		}
		var days []any
		switch d := m["dayOfWeek"].(type) {
		case string:
			days = []any{d}
		case []any:
			days = d
		default:
			return ""
		}
		for _, d := range days {
			name, _ := d.(string)
			i := dayIndex(name)
			if i < 0 {
				if strings.Contains(strings.ToLower(name), "holiday") {
					continue
				}
				return ""
			}
			times[i] = append(times[i], o+"-"+c)
		}
	}
	return groupDays(times)
}

func dayIndex(s string) int {
	s = strings.ToLower(s[strings.LastIndexAny(s, "/#")+1:])
	for i, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		if s == d || s == d[:2] {
			return i
		}
	}
	return -1
}

// clock: "9:00", "09:00:00", "09:00+01:00" as "09:00"; "" when not a time of day
func clock(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "+Z"); i > 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return ""
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}
