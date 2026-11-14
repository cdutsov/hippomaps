package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"testing"
)

// The site client connects to public addresses on ports 80 and 443 only, this server's own networks never.
func TestPublicDial(t *testing.T) {
	for _, c := range []struct {
		addr string
		ok   bool
	}{
		{"203.0.114.7:443", true}, {"8.8.8.8:80", true}, {"[2a01:4f8::1]:443", true},
		{"8.8.8.8:8080", false}, {"127.0.0.1:443", false}, {"10.0.0.5:80", false}, {"172.18.0.3:443", false},
		{"192.168.1.1:80", false}, {"169.254.169.254:80", false}, {"100.64.1.1:443", false}, {"0.0.0.0:80", false},
		{"[::1]:443", false}, {"[fd7a:8c4e:2b19:1::5]:443", false}, {"[fe80::1]:80", false}, {"[::ffff:127.0.0.1]:80", false},
		{"[64:ff9b::a00:1]:80", false}, {"224.0.0.1:80", false}, {"203.0.113.9:443", false},
	} {
		if got := publicDial(netip.MustParseAddrPort(c.addr)); got != c.ok {
			t.Errorf("%s: %v", c.addr, got)
		}
	}
	// the real client refuses a server on this machine, also after a redirect from elsewhere
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("secret")) }))
	defer srv.Close()
	if _, err := newSiteClient(publicDial).Get(srv.URL); err == nil {
		t.Fatal("the site client reached 127.0.0.1")
	}
}

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"example.ch": "https://example.ch", "https://example.ch/de#x": "https://example.ch/de", "http://a.b/c?d=1": "http://a.b/c?d=1",
		"https://a.ch;https://b.ch": "https://a.ch", "ftp://a.ch": "", "javascript:alert(1)": "", "": "", "https://user:pw@a.ch": "",
		"not a site": "",
	} {
		got := ""
		if u := webURL(in); u != nil {
			got = u.String()
		}
		if got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestRobots(t *testing.T) {
	txt := `# robots
User-agent: *
Disallow: /private
Allow: /private/menu$

User-agent: BadBot
User-agent: hippomaps
Disallow: /img/*.jpg$
Allow: /img/public/

User-agent: other
Disallow: /
`
	for _, c := range []struct {
		agent, path string
		ok          bool
	}{
		{"hippomaps", "/", true},
		{"hippomaps", "/private", true},          // its own group, not that of *
		{"hippomaps", "/img/a.jpg", false},       // * and $
		{"hippomaps", "/img/a.jpg?x=1", true},    // $: the end
		{"hippomaps", "/img/public/a.jpg", true}, // the longer Allow wins
		{"someone", "/private/x", false},
		{"someone", "/private/menu", true},
		{"someone", "/", true},
	} {
		if got := robotsAllows(txt, c.agent, c.path); got != c.ok {
			t.Errorf("%s %s: %v", c.agent, c.path, got)
		}
	}
	if !robotsAllows("", "hippomaps", "/x") || robotsAllows("User-agent: *\nDisallow: /\n", "hippomaps", "/") {
		t.Error("empty robots.txt, or one for all")
	}
	if !robotsAllows("User-agent: *\nDisallow:\n", "hippomaps", "/a") {
		t.Error("Disallow: with nothing allows all")
	}
}

func TestReadPage(t *testing.T) {
	base, _ := url.Parse("https://www.example.ch/de/")
	page := `<!doctype html><html><head>
<base href="https://www.example.ch/">
<meta property="og:image" content="/media/share.jpg?w=1200&amp;h=630">
<meta name="twitter:image" content="https://cdn.example.ch/tw.png">
<link rel="icon" href="/favicon.ico">
<link rel="icon" sizes="192x192" href="/icon-192.png">
<link rel="apple-touch-icon" href="/apple.png">
<link rel="icon" type="image/svg+xml" sizes="512x512" href="/icon.svg">
<!-- <meta property="og:image" content="/commented.jpg"> -->
<script>var s = "<a href='/speisekarte'>Speisekarte</a>";</script>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","image":"/site.jpg"},
 {"@type":"Restaurant","address":{"@type":"PostalAddress"},"image":{"@type":"ImageObject","url":"/ld.jpg"},"logo":"/logo.png",
  "openingHours":["Mo-Sa 11:00-14:30","Mo-Th 17:00-21:30","Fr-Sa 17:00-22:00"]}]}</script>
</head><body>
<a href="#" class="menu-toggle">Menu</a>
<a href="/de/">Menü</a>
<a href="https://www.facebook.com/x">Speisekarte</a>
<a href="/anfahrt">Karte &amp; Anfahrt</a>
<a href="/files/Speisekarte-2026.pdf"><span>Unsere</span> <b>Speisekarte</b></a>
</body></html>`
	p := readPage(page, base)
	var urls []string
	for _, x := range p.Pictures {
		urls = append(urls, x.URL)
	}
	want := []string{"https://www.example.ch/media/share.jpg?w=1200&h=630", "https://cdn.example.ch/tw.png", "https://www.example.ch/ld.jpg",
		"https://www.example.ch/logo.png", "https://www.example.ch/apple.png", "https://www.example.ch/icon-192.png"}
	if !reflect.DeepEqual(urls, want) {
		t.Errorf("pictures %q\nwant %q", urls, want)
	}
	if p.Pictures[3].Logo != true || p.Pictures[0].Logo != false {
		t.Errorf("logo flags %+v", p.Pictures)
	}
	if p.Menu != "https://www.example.ch/files/Speisekarte-2026.pdf" {
		t.Errorf("menu %q", p.Menu)
	}
	if p.Hours != "Mo-Th 11:00-14:30,17:00-21:30; Fr,Sa 11:00-14:30,17:00-22:00" {
		t.Errorf("hours %q", p.Hours)
	}
	// the menu of JSON-LD first; a link by its address when no text says menu; none at all
	if p := readPage(`<script type="application/ld+json">{"@type":"Restaurant","hasMenu":"https://menu.example.ch/"}</script><a href="/speisekarte">Essen</a>`, base); p.Menu != "https://menu.example.ch/" {
		t.Errorf("menu of JSON-LD: %q", p.Menu)
	}
	if p := readPage(`<a href="/de/speisekarte/">Essen &amp; Trinken</a>`, base); p.Menu != "https://www.example.ch/de/speisekarte/" {
		t.Errorf("menu by address: %q", p.Menu)
	}
	if p := readPage(`<a href="/kontakt">Kontakt</a><a href="/map">Karte</a>`, base); p.Menu != "https://www.example.ch/map" {
		t.Errorf("Karte alone is the menu of a restaurant: %q", p.Menu)
	}
	if p := readPage(`<a href="/wein.pdf">Weinkarte</a><a href="/essen.pdf">Speisekarte</a>`, base); p.Menu != "https://www.example.ch/essen.pdf" {
		t.Errorf("the food before the wine: %q", p.Menu)
	}
	if p := readPage(`<a href="/wein.pdf">Weinkarte</a>`, base); p.Menu != "https://www.example.ch/wein.pdf" {
		t.Errorf("only wine: %q", p.Menu)
	}
	if p := readPage(`<a href="/kontakt">Kontakt</a><a href="/anfahrt">Anfahrt und Karte</a>`, base); p.Menu != "" {
		t.Errorf("no menu: %q", p.Menu)
	}
	for _, text := range []string{"Speisekarte", "Menu", "MENÜ", "La carte", "Notre carte", "Menù", "Меню", "Food Menu", "Weinkarte", "Mittagsmenü", "Speisekarte (PDF)"} {
		if !menuText.MatchString(text) {
			t.Errorf("%q is not read as a menu", text)
		}
	}
	for _, text := range []string{"Kontakt", "Menu of the day was great", "Speisekarte und Öffnungszeiten der Filialen in der Region"} {
		if menuText.MatchString(text) {
			t.Errorf("%q is read as a menu", text)
		}
	}
}

func TestHours(t *testing.T) {
	for _, c := range []struct {
		in   any
		want string
	}{
		{"Mo-Fr 09:00-18:00", "Mo-Fr 09:00-18:00"},
		{"Mo-Fr 9:00-18:00; Sa 10:00-14:00", "Mo-Fr 09:00-18:00; Sa 10:00-14:00"},
		{"Mo-Fr 09:00-18:00, Sa 09:00-14:00", "Mo-Fr 09:00-18:00; Sa 09:00-14:00"},
		{"Tu,Th 16:00-20:00", "Tu 16:00-20:00; Th 16:00-20:00"},
		{"Fr-Mo 10:00-12:00", "Mo 10:00-12:00; Fr-Su 10:00-12:00"},
		{[]any{"Mo-Su"}, "Mo-Su 00:00-24:00"},
		{"Montag bis Freitag 9-18 Uhr", ""},
		{[]any{"Mo-Fr 09:00-18:00", 5}, ""},
	} {
		if got := hoursOf(c.in); got != c.want {
			t.Errorf("%v: %q, want %q", c.in, got, c.want)
		}
	}
	spec := []any{
		map[string]any{"dayOfWeek": []any{"https://schema.org/Monday", "Tuesday", "Wednesday", "Thursday", "Friday"}, "opens": "08:00:00", "closes": "12:00:00"},
		map[string]any{"dayOfWeek": []any{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday"}, "opens": "13:30", "closes": "18:30"},
		map[string]any{"dayOfWeek": "Saturday", "opens": "9:00", "closes": "00:00"},
		map[string]any{"dayOfWeek": "Sunday", "opens": "00:00", "closes": "00:00"},
		map[string]any{"dayOfWeek": "PublicHolidays", "opens": "10:00", "closes": "12:00"},
		map[string]any{"dayOfWeek": "Monday", "opens": "10:00", "closes": "12:00", "validFrom": "2026-12-24", "validThrough": "2026-12-24"},
	}
	if got := hoursOfSpec(spec); got != "Mo-Fr 08:00-12:00,13:30-18:30; Sa 09:00-24:00" {
		t.Errorf("spec: %q", got)
	}
	if got := hoursOfSpec(map[string]any{"dayOfWeek": "Funday", "opens": "1", "closes": "2"}); got != "" {
		t.Errorf("a bad spec: %q", got)
	}
}

// robots.txt is read once a day per site, through the site client
func TestRobotsAllow(t *testing.T) {
	n := 0
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			n++
			w.Write([]byte("User-agent: hippomaps\nDisallow: /no\n"))
		}
	}))
	defer site.Close()
	s := testServer(t, nil)
	s.siteClient = newSiteClient(func(netip.AddrPort) bool { return true })
	u1, _ := url.Parse(site.URL + "/yes")
	u2, _ := url.Parse(site.URL + "/no/x")
	if !s.robotsAllow(context.Background(), u1) || s.robotsAllow(context.Background(), u2) || n != 1 {
		t.Fatalf("robots: %d reads", n)
	}
}
