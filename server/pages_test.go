package server

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"gitlab.com/cdutsov/hippomaps/web"
)

// The pages about the app (pages.go): each in English and German, linked to each other, in the sitemap and open to
// search engines on a site of its own; their pictures and links all there; no script.
func TestPages(t *testing.T) {
	own := testServer(t, nil, func(c *Config) { c.Base = ""; c.PublicURL = "https://hippomaps.example" })
	if len(own.pages) < 20 {
		t.Fatalf("only %d pages", len(own.pages))
	}
	// each page in each language
	for u, pg := range own.pages {
		for _, l := range pageLangs {
			if _, ok := own.pages[pageURL("", l, pg.Path)]; !ok {
				t.Errorf("%s: no %s version", u, l)
			}
		}
	}
	sitemap := get(own, "/sitemap.xml").Body.String()
	href := regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	// words of AI writing (the owner list and Wikipedia: Signs of AI writing), and "verb no noun" (shows no ads, braucht kein Konto)
	banned := regexp.MustCompile(`(?i)\b(seamless|robust|leverage|honestly|exactly|simply|load-bearing|delve|crucial|boasts?|vibrant|showcase|testament|pivotal|nestled|additionally|ensures?|enhances?|effortless(?:ly)?)\b|Here is how|in the heart of|stands as|’s (?:page|plan|map)\b|\b(?:needs|shows|has|finds|gives|takes|keeps|sends|uses|braucht|zeigt|hat|findet|gibt|nimmt) (?:no|kein\w*)\b|\b(?:New to|Neu bei) hippomaps\?`)
	for u, pg := range own.pages {
		w := get(own, u)
		body := w.Body.String()
		if w.Code != 200 || w.Header().Get("X-Robots-Tag") != "" {
			t.Errorf("%s: %d %v", u, w.Code, w.Header())
			continue
		}
		if strings.Contains(body, "<script") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("%s: a script, or no CSP", u)
		}
		for _, want := range []string{`<html lang="` + pg.Lang + `">`, `<link rel="canonical" href="https://hippomaps.example` + u + `">`,
			`hreflang="en" href="https://hippomaps.example` + pageURL("", "en", pg.Path) + `"`, `hreflang="de" href="https://hippomaps.example` + pageURL("", "de", pg.Path) + `"`,
			`<meta name="description" content="`, `<meta property="og:image" content="https://hippomaps.example/static/pages/img/`, "<h1>"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: no %s", u, want)
			}
		}
		if m := regexp.MustCompile(`<title>(.*?)</title>`).FindStringSubmatch(body); m == nil || len(m[1]) < 12 || len(m[1]) > 80 {
			t.Errorf("%s: title %q", u, m)
		}
		if m := regexp.MustCompile(`<meta name="description" content="([^"]*)">`).FindStringSubmatch(body); m == nil || len(m[1]) < 70 || len(m[1]) > 230 {
			t.Errorf("%s: description %q", u, m)
		}
		if !strings.Contains(sitemap, "<loc>https://hippomaps.example"+u+"</loc>") {
			t.Errorf("%s: not in the sitemap", u)
		}
		if m := banned.FindString(body); m != "" {
			t.Errorf("%s: %q", u, m)
		}
		// every link and picture of this site is there: pages, the app with a search, place or route, static files
		for _, m := range href.FindAllStringSubmatch(body, -1) {
			l, _, _ := strings.Cut(strings.ReplaceAll(m[1], "&amp;", "&"), "#") // a view of the map (#map=) is the page
			if !strings.HasPrefix(l, "/") {
				continue
			}
			if strings.HasPrefix(l, "/static/") && !regexp.MustCompile(`\?v=[0-9a-f]{6,}$`).MatchString(l) {
				t.Errorf("%s: %s without its hash (no such file)", u, l)
			}
			if r := get(own, l); r.Code != 200 {
				t.Errorf("%s: link %s: %d", u, l, r.Code)
			}
		}
	}
	// the sitemap: the other language of each page
	if !strings.Contains(sitemap, `<url><loc>https://hippomaps.example/guides/gpx</loc><xhtml:link rel="alternate" hreflang="en" href="https://hippomaps.example/guides/gpx"/><xhtml:link rel="alternate" hreflang="de" href="https://hippomaps.example/de/guides/gpx"/></url>`) {
		t.Errorf("sitemap: %s", sitemap)
	}
	// every picture of web/pages/img is used by a page
	used := ""
	for u := range own.pages {
		used += get(own, u).Body.String()
	}
	fs.WalkDir(web.Files, "pages/img", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.Contains(used, "/static/"+p+"?v=") {
			t.Errorf("%s: no page shows it", p)
		}
		return nil
	})
	// the font Overpass of pages.css: served as a font, and the CSP of the pages lets it in
	for _, f := range []string{"overpass-latin.woff2", "overpass-latin-ext.woff2"} {
		if !strings.Contains(get(own, "/static/pages/pages.css").Body.String(), "url(fonts/"+f+")") {
			t.Errorf("pages.css: no %s", f)
		}
		if w := get(own, "/static/pages/fonts/"+f); w.Code != 200 || w.Header().Get("Content-Type") != "font/woff2" {
			t.Errorf("%s: %d %s", f, w.Code, w.Header().Get("Content-Type"))
		}
	}
	if w := get(own, "/features"); !strings.Contains(w.Header().Get("Content-Security-Policy"), "font-src 'self'") {
		t.Errorf("/features: CSP without fonts: %s", w.Header().Get("Content-Security-Policy"))
	}
	// the templates are not served as files
	if w := get(own, "/static/pages/en/features.html"); w.Code == 200 {
		t.Error("a template served as a file")
	}
	// under a base (/labs/maps): the pages there, without a sitemap; the links of a page under the base too
	s := testServer(t, nil)
	w := get(s, "/labs/maps/de/guides/gpx")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/labs/maps/de/guides"`) || !strings.Contains(w.Body.String(), `href="/labs/maps/guides/gpx"`) ||
		w.Header().Get("X-Robots-Tag") != "noindex" || strings.Contains(w.Body.String(), "canonical") {
		t.Errorf("under a base: %d %v", w.Code, w.Header())
	}
}

// The page of the app tells search engines and link previews what it is, and links the pages about it without JavaScript.
func TestIndexHead(t *testing.T) {
	own := testServer(t, nil, func(c *Config) { c.Base = ""; c.PublicURL = "https://hippomaps.example" })
	body := get(own, "/").Body.String()
	for _, want := range []string{"<title>hippomaps – search the map in plain words</title>", `<meta property="og:image" content="https://hippomaps.example/static/pages/img/en/search.webp?v=`,
		`"@type":"WebApplication"`, `<a href="/features">Features</a>`, `<a href="/de/guides">Anleitungen</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("no %s", want)
		}
	}
}
