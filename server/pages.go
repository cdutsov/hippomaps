package server

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"gitlab.com/cdutsov/hippomaps/web"
)

// The pages about the app: what it does (features) and how to do things with it (guides), in English and German.
// Each is a template in web/pages/<lang>/ that defines "title", "desc" and "body"; web/pages/layout.html puts them
// in the frame of the site. The pictures are pages/img/<lang>/<name>.webp. The path follows the file: pages/en/guides/gpx.html is /guides/gpx, pages/de/guides/gpx.html
// is /de/guides/gpx, pages/en/features.html is /features. The pictures are in web/pages/img (static, with a hash).

// pageLangs: the languages of the pages, English first (at the root of the site)
var pageLangs = []string{"en", "de"}

type page struct {
	Path string // /guides/gpx, without the base and the language
	Lang string
	file string // pages/en/guides/gpx.html
	tmpl *template.Template
}

// url is the path of a page under base: /de/guides/gpx
func pageURL(base, lang, p string) string {
	if lang != "en" {
		p = "/" + lang + p
	}
	return base + p
}

func (s *Server) loadPages() {
	s.pages = map[string]*page{}
	base := strings.TrimRight(s.cfg.Base, "/")
	for _, lang := range pageLangs {
		fs.WalkDir(web.Files, "pages/"+lang, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || path.Ext(p) != ".html" {
				return err
			}
			pg := &page{Path: strings.TrimSuffix(strings.TrimPrefix(p, "pages/"+lang), ".html"), Lang: lang, file: p}
			funcs := template.FuncMap{
				"asset": s.asset,
				// link: the address of another page in the language of this one
				"link": func(to string) string { return pageURL(base, lang, to) },
				// app: the app with a search, a place or a route of the query ("q", "cafés in Zurich")
				"app": func(kv ...string) string {
					v := url.Values{}
					for i := 0; i+1 < len(kv); i += 2 {
						v.Add(kv[i], kv[i+1])
					}
					u := s.home()
					if len(v) > 0 {
						u += "?" + v.Encode()
					}
					return u
				},
				// img: a picture of the app (pages/img/<lang>/<name>.webp), the English one when the language has none
				"img": func(name string) string {
					p := "pages/img/" + lang + "/" + name + ".webp"
					if _, err := fs.Stat(web.Files, p); err != nil {
						p = "pages/img/en/" + name + ".webp"
					}
					return s.asset(p)
				},
				// view: the app at a place of the map (#map=zoom/lat/lon)
				"view": func(v string) string { return s.home() + "#map=" + v },
			}
			pg.tmpl = template.Must(template.New("layout.html").Funcs(funcs).ParseFS(web.Files, "pages/layout.html", p))
			s.pages[pageURL(base, lang, pg.Path)] = pg
			return nil
		})
	}
	for u, pg := range s.pages {
		s.mux.HandleFunc("GET "+u, s.servePage(pg))
	}
}

// alternates: the address of a page in each language it has
func (s *Server) alternates(p string) map[string]string {
	base := strings.TrimRight(s.cfg.Base, "/")
	out := map[string]string{}
	for _, l := range pageLangs {
		if _, ok := s.pages[pageURL(base, l, p)]; ok {
			out[l] = pageURL(base, l, p)
		}
	}
	return out
}

func (s *Server) servePage(pg *page) http.HandlerFunc {
	base := strings.TrimRight(s.cfg.Base, "/")
	return func(w http.ResponseWriter, r *http.Request) {
		type alt struct{ Lang, URL, Name string }
		var alts []alt
		names := map[string]string{"en": "English", "de": "Deutsch"}
		for _, l := range pageLangs {
			if u, ok := s.alternates(pg.Path)[l]; ok {
				alts = append(alts, alt{l, u, names[l]})
			}
		}
		section := strings.Split(strings.TrimPrefix(pg.Path, "/"), "/")[0]
		data := map[string]any{"Base": base, "Public": strings.TrimRight(s.cfg.PublicURL, "/"), "Lang": pg.Lang, "Path": pg.Path,
			"URL": pageURL(base, pg.Lang, pg.Path), "Alts": alts, "Section": section, "Home": s.home(),
			"Features": pageURL(base, pg.Lang, "/features"), "Guides": pageURL(base, pg.Lang, "/guides"),
			"Privacy": pageURL(base, pg.Lang, "/privacy"), "Licences": pageURL(base, pg.Lang, "/licences")}
		// the picture of a link to the page (og:image): the template "image" names one of pages/img, else that of a search
		data["Image"] = "search"
		var buf bytes.Buffer
		if pg.tmpl.Lookup("image") != nil && pg.tmpl.ExecuteTemplate(&buf, "image", nil) == nil {
			data["Image"] = strings.TrimSpace(buf.String())
			buf.Reset()
		}
		if err := pg.tmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
			s.log.Error("page", "page", pg.file, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Write(buf.Bytes())
	}
}

// sitemapXML: the page, the pages about it (with their other language), the privacy policy and the licences
func (s *Server) sitemapXML() string {
	pub := strings.TrimRight(s.cfg.PublicURL, "/")
	base := strings.TrimRight(s.cfg.Base, "/")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	loc := func(u string, alts map[string]string) {
		fmt.Fprintf(&b, "<url><loc>%s</loc>", html.EscapeString(pub+strings.TrimPrefix(u, base)))
		if len(alts) > 1 {
			for _, l := range pageLangs {
				if a, ok := alts[l]; ok {
					fmt.Fprintf(&b, `<xhtml:link rel="alternate" hreflang="%s" href="%s"/>`, l, html.EscapeString(pub+strings.TrimPrefix(a, base)))
				}
			}
		}
		b.WriteString("</url>\n")
	}
	loc(base+"/", nil)
	var urls []string
	for u := range s.pages {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	for _, u := range urls {
		loc(u, s.alternates(s.pages[u].Path))
	}
	for _, p := range []string{"/privacy", "/de/privacy", "/licences", "/de/licences"} {
		loc(base+p, nil)
	}
	b.WriteString("</urlset>\n")
	return b.String()
}
