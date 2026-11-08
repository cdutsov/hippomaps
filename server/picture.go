package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pictures of places and logos of chains, from Wikimedia Commons. The browser asks
// this server only: GET {base}/api/picture with the tags of a place gives the bytes of a
// thumbnail, and the credit in headers. In order, the first that has one:
//
//	commons   the file of wikimedia_commons (or of an image tag on Commons): a photo
//	wikidata  the image (P18) of the item of the place: a photo
//	brand     the logo (P154), else the small logo (P8972), of the item of the chain
//	wikidata  the logo of the item of the place
//
// osm=relation/<id> (a town or a region found by the search, which carries no tags) stands for
// the item of Wikidata that names that relation (P402), found by the search of Wikidata.
//
// Answers of Wikidata and Commons are kept in memory, the bytes too, up to a limit.

var (
	qidRe  = regexp.MustCompile(`^Q[1-9][0-9]{0,11}$`)
	osmRe  = regexp.MustCompile(`^relation/([1-9][0-9]{0,11})$`)
	fileRe = regexp.MustCompile(`^[^#<>\[\]|{}/\x00-\x1f\x7f]{1,240}\.[A-Za-z]{3,4}$`)
	tagRe  = regexp.MustCompile(`<[^>]*>`)
)

// widths are the sizes of a thumbnail: the list and the page of a place. Wikimedia keeps
// thumbnails of standard widths, 120 and 500 among them.
var widths = map[string]int{"": 120, "120": 120, "500": 500}

// picture is a thumbnail found for a place.
type picture struct {
	Kind   string // photo | logo
	Credit string // "Author, CC BY-SA 4.0"
	Page   string // the page of the file on Commons
	Type   string // of the bytes
	Bytes  []byte
}

type candidate struct {
	kind, item, prop, file string // a file, or the property of an item that names one
}

// candidates are the files to try for a request, best first; nil when it does not name one.
func candidates(q url.Values) []candidate {
	var out []candidate
	if f := commonsFile(q.Get("commons")); f != "" {
		out = append(out, candidate{kind: "photo", file: f})
	}
	wd, brand := q.Get("wikidata"), q.Get("brand")
	if qidRe.MatchString(wd) {
		out = append(out, candidate{kind: "photo", item: wd, prop: "P18"})
	}
	if qidRe.MatchString(brand) {
		out = append(out, candidate{kind: "logo", item: brand, prop: "P154"}, candidate{kind: "logo", item: brand, prop: "P8972"})
	}
	if qidRe.MatchString(wd) {
		out = append(out, candidate{kind: "logo", item: wd, prop: "P154"})
	}
	return out
}

// commonsFile is the file name of a wikimedia_commons value ("File:X.jpg") or of an image URL
// on Commons; "" for anything else, a category among them.
func commonsFile(v string) string {
	v = strings.TrimSpace(v)
	for _, p := range []string{"https://commons.wikimedia.org/wiki/", "http://commons.wikimedia.org/wiki/"} {
		if strings.HasPrefix(v, p) {
			u, err := url.PathUnescape(strings.TrimPrefix(v, p))
			if err != nil {
				return ""
			}
			v = u
		}
	}
	if !strings.HasPrefix(v, "File:") {
		return ""
	}
	f := strings.ReplaceAll(strings.TrimPrefix(v, "File:"), "_", " ")
	if !fileRe.MatchString(f) {
		return ""
	}
	return f
}

// picture serves a thumbnail: 204 when the place does not have one, 429 over the limit of the
// visitor, 503 when the calls to Wikimedia stay taken.
func (s *Server) picture(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	width, ok := widths[q.Get("w")]
	osm := osmRe.FindStringSubmatch(q.Get("osm"))
	cands := candidates(q)
	if !ok || (len(cands) == 0 && osm == nil) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.perIP.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many pictures", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	select {
	case s.calls <- struct{}{}:
		defer func() { <-s.calls }()
	case <-time.After(3 * time.Second):
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	case <-ctx.Done():
		return
	}
	if len(cands) == 0 { // a relation: its item first
		item, err := s.itemOf(ctx, osm[1])
		if err != nil {
			s.log.Warn("picture failed", "err", err)
			http.Error(w, "upstream", http.StatusBadGateway)
			return
		}
		if item != "" {
			cands = candidates(url.Values{"wikidata": {item}})
		}
	}
	var p *picture
	var err error
	if len(cands) > 0 {
		p, err = s.find(ctx, cands, width)
	}
	h := w.Header()
	if err != nil {
		s.log.Warn("picture failed", "err", err)
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	h.Set("Cache-Control", "public, max-age=86400")
	if p == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.Set("Content-Type", p.Type)
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("X-Picture-Kind", p.Kind)
	h.Set("X-Picture-Credit", url.QueryEscape(p.Credit))
	h.Set("X-Picture-Page", p.Page)
	h.Set("Access-Control-Expose-Headers", "X-Picture-Kind, X-Picture-Credit, X-Picture-Page")
	w.Write(p.Bytes)
}

// find is the first picture of the candidates. A failure of Wikimedia is the answer only
// when no candidate gives a picture.
func (s *Server) find(ctx context.Context, cands []candidate, width int) (*picture, error) {
	var first error
	for _, c := range cands {
		file := c.file
		if file == "" {
			f, err := s.claim(ctx, c.item, c.prop)
			if err != nil {
				first = cmpErr(first, err)
				continue
			}
			file = f
		}
		if file == "" {
			continue
		}
		p, err := s.thumbnail(ctx, file, width)
		if err != nil {
			first = cmpErr(first, err)
			continue
		}
		if p != nil {
			out := *p
			out.Kind = c.kind
			return &out, nil
		}
	}
	return nil, first
}

func cmpErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// claim is the file a property of an item names: the preferred statement, else the
// first normal one; "" when it does not have one.
func (s *Server) claim(ctx context.Context, item, prop string) (string, error) {
	key := "claim:" + item + ":" + prop
	if v, ok := s.cache.get(key); ok {
		return string(v), nil
	}
	q := url.Values{"action": {"wbgetclaims"}, "entity": {item}, "property": {prop}, "format": {"json"}}
	var d struct {
		Claims map[string][]struct {
			Rank     string `json:"rank"`
			Mainsnak struct {
				Datavalue struct {
					Value json.RawMessage `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	}
	if err := s.getJSON(ctx, s.cfg.WikidataAPI+"?"+q.Encode(), &d); err != nil {
		return "", err
	}
	file := ""
	for _, rank := range []string{"preferred", "normal"} {
		for _, c := range d.Claims[prop] {
			var v string
			if c.Rank == rank && json.Unmarshal(c.Mainsnak.Datavalue.Value, &v) == nil && fileRe.MatchString(v) && file == "" {
				file = v
			}
		}
	}
	s.cache.put(key, []byte(file))
	return file, nil
}

// itemOf is the item of Wikidata of an OSM relation (P402), "" when none has it; kept in memory
func (s *Server) itemOf(ctx context.Context, relation string) (string, error) {
	key := "osm:relation/" + relation
	if v, ok := s.cache.get(key); ok {
		return string(v), nil
	}
	q := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {"haswbstatement:P402=" + relation}, "srlimit": {"1"}, "format": {"json"}}
	var d struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := s.getJSON(ctx, s.cfg.WikidataAPI+"?"+q.Encode(), &d); err != nil {
		return "", err
	}
	item := ""
	if len(d.Query.Search) > 0 && qidRe.MatchString(d.Query.Search[0].Title) {
		item = d.Query.Search[0].Title
	}
	s.cache.put(key, []byte(item))
	return item, nil
}

// thumbnail is the thumbnail of a file on Commons at a width, with its credit; nil when
// Commons does not have the file or the bytes are not a picture to show.
func (s *Server) thumbnail(ctx context.Context, file string, width int) (*picture, error) {
	key := fmt.Sprintf("thumb:%d:%s", width, file)
	if v, ok := s.cache.get(key); ok {
		if len(v) == 0 {
			return nil, nil
		}
		var p picture
		if err := json.Unmarshal(v, &p); err == nil {
			return &p, nil
		}
	}
	q := url.Values{"action": {"query"}, "titles": {"File:" + file}, "prop": {"imageinfo"}, "iiprop": {"url|extmetadata|mime"},
		"iiurlwidth": {strconv.Itoa(width)}, "iiextmetadatafilter": {"Artist|LicenseShortName"}, "format": {"json"}, "formatversion": {"2"}}
	var d struct {
		Query struct {
			Pages []struct {
				Missing   bool `json:"missing"`
				ImageInfo []struct {
					ThumbURL    string `json:"thumburl"`
					Description string `json:"descriptionurl"`
					Mime        string `json:"mime"`
					Meta        map[string]struct {
						Value any `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := s.getJSON(ctx, s.cfg.CommonsAPI+"?"+q.Encode(), &d); err != nil {
		return nil, err
	}
	none := func() (*picture, error) {
		s.cache.put(key, nil)
		return nil, nil
	}
	if len(d.Query.Pages) == 0 || d.Query.Pages[0].Missing || len(d.Query.Pages[0].ImageInfo) == 0 {
		return none()
	}
	ii := d.Query.Pages[0].ImageInfo[0]
	if ii.ThumbURL == "" || !s.cfg.allowedThumb(ii.ThumbURL) {
		return none()
	}
	b, typ, err := s.getBytes(ctx, ii.ThumbURL)
	if err != nil {
		return nil, err
	}
	if !pictureTypes[typ] {
		return none()
	}
	text := func(k string) string {
		v, _ := ii.Meta[k].Value.(string)
		v = html.UnescapeString(tagRe.ReplaceAllString(v, ""))
		v = strings.Join(strings.Fields(v), " ")
		if r := []rune(v); len(r) > 100 {
			v = string(r[:100]) + "…"
		}
		return v
	}
	artist := text("Artist")
	if n := len(artist); n%2 == 0 && artist[:n/2] == artist[n/2:] { // the same name twice, from two tags of the page
		artist = artist[:n/2]
	}
	if strings.EqualFold(artist, "unknown author") || strings.EqualFold(artist, "unknown") {
		artist = ""
	}
	credit := strings.Join(nonEmpty(artist, text("LicenseShortName")), ", ")
	p := &picture{Credit: credit, Page: ii.Description, Type: typ, Bytes: b}
	if v, err := json.Marshal(p); err == nil {
		s.cache.put(key, v)
	}
	return p, nil
}

// pictureTypes are the types served. SVG is not among them: from this origin its scripts would run.
var pictureTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func (s *Server) getJSON(ctx context.Context, u string, v any) error {
	b, _, err := s.get(ctx, u, 2<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Server) getBytes(ctx context.Context, u string) ([]byte, string, error) {
	return s.get(ctx, u, 2<<20)
}

// get reads a URL of Wikimedia, up to max bytes.
func (s *Server) get(ctx context.Context, u string, max int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // without the URL
		}
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("wikimedia: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > max {
		return nil, "", errors.New("wikimedia: answer too large")
	}
	typ, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return b, strings.TrimSpace(strings.ToLower(typ)), nil
}
