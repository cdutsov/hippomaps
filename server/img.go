package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	pngenc "image/png"
)

// Pictures of the web (of the websites of places, of Panoramax) are decoded and written again by this server: shrunk to
// the width the page shows, turned as their EXIF says, without their metadata (the place where a photo was taken, the
// camera), and never as the bytes a site sent (WebP excepted, which Go does not decode: its size read from its header,
// passed on with a type the browser holds to).

var errNotPicture = errors.New("not a picture to show")

// maxPixels: larger pictures are not decoded (a small file can claim a huge image)
const maxPixels = 40_000_000

// webPicture is a picture to serve: its bytes and type, its size, and whether it looks like a logo
type webPicture struct {
	Type   string
	Bytes  []byte
	W, H   int
	Logo   bool
	Width0 int // the width of the picture as it came
}

// readPicture decodes b (JPEG, PNG, GIF; WebP by its header) and writes it at most width pixels wide. minW: the width
// under which it is not shown. Blank pictures (one colour) are not shown.
func readPicture(b []byte, width, minW int) (*webPicture, error) {
	if w, h, ok := webpSize(b); ok {
		if w < minW || w*h > maxPixels || len(b) > 1<<20 {
			return nil, errNotPicture
		}
		// a logo: small for its size (flat colours), or a small square (an icon)
		logo := float64(len(b))/float64(w*h) < 0.035 || (w <= 512 && h <= 512 && 5*w >= 4*h && 5*h >= 4*w)
		return &webPicture{Type: "image/webp", Bytes: b, W: w, H: h, Logo: logo, Width0: w}, nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < minW || cfg.Height < minW/3 || cfg.Width*cfg.Height > maxPixels {
		return nil, errNotPicture
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errNotPicture
	}
	if format == "jpeg" {
		img = orient(img, exifOrientation(b))
	}
	return encodePicture(img, width)
}

// encodePicture writes an image at most width wide: a logo as PNG (it may be transparent), a photo as JPEG
func encodePicture(img image.Image, width int) (*webPicture, error) {
	logo, blank := looks(img)
	if blank {
		return nil, errNotPicture
	}
	w0 := img.Bounds().Dx()
	img = shrink(img, width)
	var buf bytes.Buffer
	p := &webPicture{W: img.Bounds().Dx(), H: img.Bounds().Dy(), Logo: logo, Width0: w0}
	if logo {
		p.Type = "image/png"
		if lightOnClear(img) { // a white logo for dark pages: on dark grey, else it disappears on the page
			out := image.NewNRGBA(img.Bounds())
			draw.Draw(out, out.Bounds(), &image.Uniform{color.NRGBA{0x2b, 0x2f, 0x33, 0xff}}, image.Point{}, draw.Src)
			draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Over)
			img = out
		}
		if err := pngenc.Encode(&buf, img); err != nil {
			return nil, err
		}
	} else {
		p.Type = "image/jpeg"
		if err := jpeg.Encode(&buf, flat(img), &jpeg.Options{Quality: 82}); err != nil {
			return nil, err
		}
	}
	p.Bytes = buf.Bytes()
	return p, nil
}

// looks: a logo has few colours or much of one (its background); a blank picture is nearly all of one colour
func looks(img image.Image) (logo, blank bool) {
	b := img.Bounds()
	const n = 48
	counts := map[uint32]int{}
	total := 0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			x := b.Min.X + (b.Dx()*(2*i+1))/(2*n)
			y := b.Min.Y + (b.Dy()*(2*j+1))/(2*n)
			r, g, bl, a := img.At(x, y).RGBA()
			key := uint32(1 << 31) // transparent
			if a >= 0x8000 {
				key = (r>>12)<<8 | (g>>12)<<4 | bl>>12
			}
			counts[key]++
			total++
		}
	}
	top := 0
	for _, c := range counts {
		if c > top {
			top = c
		}
	}
	f := float64(top) / float64(total)
	return f >= 0.4 || len(counts) <= 40, f >= 0.97
}

// lightOnClear: a picture with much transparency whose visible part is light (a white logo)
func lightOnClear(img image.Image) bool {
	b := img.Bounds()
	const n = 48
	clear, seen := 0, 0
	var luma float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			r, g, bl, a := img.At(b.Min.X+(b.Dx()*(2*i+1))/(2*n), b.Min.Y+(b.Dy()*(2*j+1))/(2*n)).RGBA()
			if a < 0x8000 {
				clear++
				continue
			}
			seen++
			luma += (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bl)) / float64(a)
		}
	}
	return clear >= n*n/5 && seen > 0 && luma/float64(seen) > 0.82
}

// shrink scales an image down to width (each pixel of the result the mean of the pixels it covers); smaller ones stay
func shrink(img image.Image, width int) image.Image {
	b := img.Bounds()
	if b.Dx() <= width {
		return img
	}
	h := b.Dy() * width / b.Dx()
	if h < 1 {
		h = 1
	}
	src := image.NewNRGBA(b)
	draw.Draw(src, b, img, b.Min, draw.Src)
	dst := image.NewNRGBA(image.Rect(0, 0, width, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*b.Dy()/h, (y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < width; x++ {
			x0, x1 := x*b.Dx()/width, (x+1)*b.Dx()/width
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				o := src.PixOffset(b.Min.X+x0, b.Min.Y+yy)
				for xx := x0; xx < x1; xx++ {
					p := src.Pix[o : o+4]
					w := uint64(p[3])
					r += uint64(p[0]) * w
					g += uint64(p[1]) * w
					bl += uint64(p[2]) * w
					a += w
					n++
					o += 4
				}
			}
			d := dst.PixOffset(x, y)
			if a > 0 {
				dst.Pix[d], dst.Pix[d+1], dst.Pix[d+2] = uint8(r/a), uint8(g/a), uint8(bl/a)
			}
			dst.Pix[d+3] = uint8(a / n)
		}
	}
	return dst
}

// flat puts a picture on white (JPEG has no transparency)
func flat(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}

// crop cuts r out of an image
func crop(img image.Image, r image.Rectangle) image.Image {
	r = r.Intersect(img.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

// exifOrientation is the orientation (1-8) the EXIF of a JPEG gives, 1 without
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) && b[i] == 0xFF {
		marker := b[i+1]
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		if marker == 0xDA || size < 2 || i+2+size > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			t := seg[6:]
			var bo binary.ByteOrder
			switch string(t[:2]) {
			case "II":
				bo = binary.LittleEndian
			case "MM":
				bo = binary.BigEndian
			default:
				return 1
			}
			off := int(bo.Uint32(t[4:]))
			if off+2 > len(t) {
				return 1
			}
			n := int(bo.Uint16(t[off:]))
			for k := 0; k < n; k++ {
				e := off + 2 + 12*k
				if e+12 > len(t) {
					return 1
				}
				if bo.Uint16(t[e:]) == 0x0112 {
					if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
						return v
					}
					return 1
				}
			}
			return 1
		}
		i += 2 + size
	}
	return 1
}

// orient turns an image as its EXIF orientation says (2-8: mirrored, turned 180, 90 one way or the other)
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// webpSize reads the size of a WebP picture from its header
func webpSize(b []byte) (w, h int, ok bool) {
	if len(b) < 30 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(b[12:16]) {
	case "VP8 ":
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(b[26:]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:]) & 0x3fff), true
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, false
		}
		v := binary.LittleEndian.Uint32(b[21:])
		return int(v&0x3fff) + 1, int(v>>14&0x3fff) + 1, true
	case "VP8X":
		w = int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h = int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, true
	}
	return 0, 0, false
}
