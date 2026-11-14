package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Free pictures near a place that has none of its own: a photo on Wikimedia Commons taken there whose name names the
// place, else a picture of Panoramax (pictures of the streets, under free licences) taken close by and looking at it.

// nameWords: the words of a name that a file of Commons must have, in lower case; none when the name has no word of
// its own (only "Restaurant", "Café", "Bar": too many files have them)
func nameWords(name string) []string {
	common := map[string]bool{"restaurant": true, "ristorante": true, "café": true, "cafe": true, "caffè": true, "bar": true, "hotel": true,
		"the": true, "zum": true, "zur": true, "der": true, "die": true, "das": true, "und": true, "and": true, "les": true, "des": true,
		"del": true, "della": true, "shop": true, "store": true, "bäckerei": true, "pizzeria": true, "pub": true, "kiosk": true}
	var out []string
	own := false
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) < 3 {
			continue
		}
		out = append(out, w)
		if !common[w] && len([]rune(w)) >= 4 {
			own = true
		}
	}
	if !own {
		return nil
	}
	return out
}

// commonsNear is the file of Commons closest to a point (within 60 m) whose name has all the words of the name of the
// place; "" when none has
func (s *Server) commonsNear(ctx context.Context, name string, lat, lon float64) (string, error) {
	words := nameWords(name)
	if len(words) == 0 {
		return "", nil
	}
	q := url.Values{"action": {"query"}, "list": {"geosearch"}, "gscoord": {fmt.Sprintf("%.6f|%.6f", lat, lon)}, "gsradius": {"60"},
		"gsnamespace": {"6"}, "gslimit": {"50"}, "format": {"json"}}
	var d struct {
		Query struct {
			Geosearch []struct {
				Title string  `json:"title"`
				Dist  float64 `json:"dist"`
			} `json:"geosearch"`
		} `json:"query"`
	}
	if err := s.getJSON(ctx, s.cfg.CommonsAPI+"?"+q.Encode(), &d); err != nil {
		return "", err
	}
	hits := d.Query.Geosearch
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Dist < hits[j].Dist })
	for _, h := range hits {
		title := strings.ToLower(strings.ReplaceAll(h.Title, "_", " "))
		all := true
		for _, w := range words {
			if !strings.Contains(title, w) {
				all = false
				break
			}
		}
		if f := commonsFile(h.Title); all && f != "" {
			return f, nil
		}
	}
	return "", nil
}

// panoPick is a picture of Panoramax that looks at a place
type panoPick struct {
	ID         string
	Thumb, SD  string  // its pictures 500 and 2048 pixels wide
	Pano       bool    // a sphere (360°)
	FOV        float64 // the width of the view of a photo, in degrees
	Toward     float64 // degrees from the middle of the picture to the place, right positive
	Credit     string
	Dist, Diff float64
}

// panoramaxNear is the picture of Panoramax best looking at a point: taken 4 to 40 m from it, the point well inside
// what it shows, the closer, the more straight on and the newer the better; nil when none is
func (s *Server) panoramaxNear(ctx context.Context, lat, lon float64) (*panoPick, error) {
	if s.cfg.PanoramaxAPI == "" {
		return nil, nil
	}
	dLat := 45 / 111320.0
	dLon := dLat / math.Max(0.1, math.Cos(lat*math.Pi/180))
	q := url.Values{"bbox": {fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", lon-dLon, lat-dLat, lon+dLon, lat+dLat)}, "limit": {"100"}}
	b, _, _, err := s.fetchSite(ctx, s.cfg.PanoramaxAPI+"/api/search?"+q.Encode(), 8<<20, "application/geo+json, application/json")
	if err != nil {
		return nil, err
	}
	var d struct {
		Features []struct {
			ID       string `json:"id"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties struct {
				Azimuth *float64 `json:"view:azimuth"`
				Orient  struct {
					FOV float64 `json:"field_of_view"`
				} `json:"pers:interior_orientation"`
				License  string `json:"license"`
				Producer string `json:"geovisio:producer"`
				Datetime string `json:"datetime"`
			} `json:"properties"`
			Assets map[string]struct {
				Href string `json:"href"`
			} `json:"assets"`
		} `json:"features"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	var best *panoPick
	bestScore := math.Inf(1)
	for _, f := range d.Features {
		c, pr := f.Geometry.Coordinates, f.Properties
		if len(c) < 2 || pr.Azimuth == nil || f.Assets["sd"].Href == "" || f.Assets["thumb"].Href == "" {
			continue
		}
		dist, bear := distBearing(c[1], c[0], lat, lon)
		if dist < 4 || dist > 40 {
			continue
		}
		pano := pr.Orient.FOV >= 300
		fov := pr.Orient.FOV
		if fov <= 0 {
			fov = 70
		}
		diff := angle(bear - *pr.Azimuth)
		if !pano && math.Abs(diff) > fov/2-8 {
			continue
		}
		age := 0.0
		if t, err := time.Parse(time.RFC3339, pr.Datetime); err == nil {
			age = time.Since(t).Hours() / 24 / 365
		}
		score := math.Abs(dist-12)/30 + age/10
		if !pano {
			score += math.Abs(diff) / fov
		} else {
			score += 0.15 // a part of a sphere is less sharp than a photo
		}
		if score < bestScore {
			bestScore = score
			best = &panoPick{ID: f.ID, Thumb: f.Assets["thumb"].Href, SD: f.Assets["sd"].Href, Pano: pano, FOV: fov, Toward: diff,
				Credit: strings.Join(nonEmpty(pr.Producer, licenceName(pr.License)), ", "), Dist: dist, Diff: diff}
		}
	}
	return best, nil
}

// panoPicture is the part of a picture of Panoramax around the place, at most width wide: of a sphere 70° around it, of
// a photo half its width around it (pictures of streets look along the street, the place is at a side)
func (s *Server) panoPicture(ctx context.Context, p *panoPick, width int) (*webPicture, error) {
	b, _, _, err := s.fetchSite(ctx, p.SD, 6<<20, "image/jpeg, image/*")
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width*cfg.Height > maxPixels || cfg.Width < 400 {
		return nil, errNotPicture
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errNotPicture
	}
	var part image.Image
	if p.Pano {
		part = sphereView(img, p.Toward, 70)
	} else {
		part = photoView(orient(img, exifOrientation(b)), p.Toward, p.FOV)
	}
	pic, err := encodePicture(part, width)
	if pic != nil {
		pic.Logo = false
	}
	return pic, err
}

// photoView cuts half the width of a photo (4:3) around the place, toward degrees right of its middle in a view fov wide
func photoView(img image.Image, toward, fov float64) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cw := w / 2
	ch := cw * 3 / 4
	if ch > h {
		ch = h
	}
	cx := int((0.5+toward/fov)*float64(w)) - cw/2
	cx = max(0, min(cx, w-cw))
	cy := max(0, min(h/2-ch/2, h-ch))
	return crop(img, image.Rect(b.Min.X+cx, b.Min.Y+cy, b.Min.X+cx+cw, b.Min.Y+cy+ch))
}

// sphereView cuts deg degrees of the width out of the picture of a sphere (equirectangular, its middle the direction
// the camera faced) around toward degrees right of the middle, from 30 to 70 % of its height
func sphereView(img image.Image, toward, deg float64) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cw := int(float64(w) * deg / 360)
	cx := int(float64(w)/2+toward/360*float64(w)) - cw/2
	cx = ((cx % w) + w) % w
	top, bot := h*30/100, h*70/100
	if cx+cw <= w {
		return crop(img, image.Rect(b.Min.X+cx, b.Min.Y+top, b.Min.X+cx+cw, b.Min.Y+bot))
	}
	// over the seam of the sphere: the two parts side by side
	left := crop(img, image.Rect(b.Min.X+cx, b.Min.Y+top, b.Max.X, b.Min.Y+bot))
	right := crop(img, image.Rect(b.Min.X, b.Min.Y+top, b.Min.X+cw-(w-cx), b.Min.Y+bot))
	out := image.NewNRGBA(image.Rect(0, 0, cw, bot-top))
	drawAt(out, left, 0)
	drawAt(out, right, left.Bounds().Dx())
	return out
}

func drawAt(dst *image.NRGBA, src image.Image, x int) {
	b := src.Bounds()
	for yy := 0; yy < b.Dy(); yy++ {
		for xx := 0; xx < b.Dx(); xx++ {
			dst.Set(x+xx, yy, src.At(b.Min.X+xx, b.Min.Y+yy))
		}
	}
}

// distBearing: metres and degrees (from north, clockwise) from one point to another
func distBearing(lat1, lon1, lat2, lon2 float64) (float64, float64) {
	r := math.Pi / 180
	y := (lat2 - lat1) * 111320
	x := (lon2 - lon1) * 111320 * math.Cos((lat1+lat2)/2*r)
	return math.Hypot(x, y), math.Mod(math.Atan2(x, y)/r+360, 360)
}

// angle puts degrees between -180 and 180
func angle(a float64) float64 {
	a = math.Mod(a+540, 360) - 180
	return a
}

// licenceName: "CC-BY-SA-4.0" as "CC BY-SA 4.0"
func licenceName(l string) string {
	if strings.HasPrefix(strings.ToUpper(l), "CC-") {
		parts := strings.Split(l, "-")
		if n := len(parts); n >= 3 {
			return "CC " + strings.ToUpper(strings.Join(parts[1:n-1], "-")) + " " + parts[n-1]
		}
	}
	if strings.EqualFold(l, "etalab-2.0") {
		return "Licence Ouverte 2.0"
	}
	return l
}
