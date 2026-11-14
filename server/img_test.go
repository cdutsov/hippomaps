package server

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	pngenc "image/png"
	"math/rand"
	"testing"
)

// a photo: many colours; a logo: two; a blank picture: one
func testPhoto(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x*255/w + r.Intn(40)), uint8(y*255/h + r.Intn(40)), uint8(r.Intn(255)), 255})
		}
	}
	return img
}

func testLogo(w, h int, ink, bg color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			if x > w/4 && x < 3*w/4 && y > h/3 && y < 2*h/3 {
				c = ink
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func encJPEG(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encPNG(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := pngenc.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadPicture(t *testing.T) {
	// a photo: shrunk to the width, a JPEG, not a logo
	p, err := readPicture(encJPEG(t, testPhoto(1200, 630)), 500, 200)
	if err != nil || p.Logo || p.Type != "image/jpeg" || p.W != 500 || p.H != 262 || p.Width0 != 1200 {
		t.Fatalf("photo: %+v %v", p, err)
	}
	if c, err := jpeg.DecodeConfig(bytes.NewReader(p.Bytes)); err != nil || c.Width != 500 {
		t.Fatalf("the bytes: %v %v", c, err)
	}
	// a logo: a PNG; a white logo on a clear background gets dark grey behind it
	p, err = readPicture(encPNG(t, testLogo(800, 300, color.NRGBA{200, 0, 0, 255}, color.NRGBA{255, 255, 255, 255})), 500, 200)
	if err != nil || !p.Logo || p.Type != "image/png" {
		t.Fatalf("logo: %+v %v", p, err)
	}
	p, err = readPicture(encPNG(t, testLogo(800, 300, color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 0})), 500, 200)
	if err != nil || !p.Logo {
		t.Fatalf("white logo: %+v %v", p, err)
	}
	if img, _ := pngenc.Decode(bytes.NewReader(p.Bytes)); img == nil || colorOf(img.At(2, 2)) != (color.NRGBA{0x2b, 0x2f, 0x33, 0xff}) {
		t.Errorf("a white logo on a clear background: %v", img.At(2, 2))
	}
	// blank, too small, not a picture, too large to decode
	if _, err := readPicture(encPNG(t, testLogo(800, 300, color.NRGBA{250, 250, 250, 255}, color.NRGBA{250, 250, 250, 255})), 500, 200); err == nil {
		t.Error("a blank picture")
	}
	if _, err := readPicture(encJPEG(t, testPhoto(150, 100)), 500, 200); err == nil {
		t.Error("a picture under the width")
	}
	if _, err := readPicture([]byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>"), 500, 200); err == nil {
		t.Error("SVG")
	}
	huge := encPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(huge[16:], 30000) // the header claims 30000 x 30000 pixels
	binary.BigEndian.PutUint32(huge[20:], 30000)
	binary.BigEndian.PutUint32(huge[29:], crc32.ChecksumIEEE(huge[12:29]))
	if _, err := readPicture(huge, 500, 1); err == nil {
		t.Error("a picture claiming 900 million pixels")
	}
}

func colorOf(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }

// a JPEG with EXIF orientation 6 (turned 90° clockwise): written upright, its height its width
func TestOrientation(t *testing.T) {
	b := encJPEG(t, testPhoto(600, 300))
	exif := []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00\x00\x00\x00\x00")
	seg := append([]byte{0xFF, 0xE1, 0, 0}, exif...)
	binary.BigEndian.PutUint16(seg[2:], uint16(len(exif)+2))
	b = append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
	if o := exifOrientation(b); o != 6 {
		t.Fatalf("orientation %d", o)
	}
	p, err := readPicture(b, 1000, 200)
	if err != nil || p.W != 300 || p.H != 600 {
		t.Fatalf("turned: %+v %v", p, err)
	}
}

// a photo of a street: half its width around the place, at the side the place is
func TestPhotoView(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 800, 600))
	for x := 0; x < 800; x++ {
		for y := 0; y < 600; y++ {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / 799), 0, 0, 255})
		}
	}
	v := photoView(img, 30, 90) // 30° right in a view 90° wide: at 5/6 of the width
	if v.Bounds().Dx() != 400 || v.Bounds().Dy() != 300 {
		t.Fatalf("size %v", v.Bounds())
	}
	if r := colorOf(v.At(399, 0)).R; r < 250 {
		t.Errorf("the right half (red %d at its right edge)", r)
	}
	if l := colorOf(photoView(img, -40, 90).At(0, 0)).R; l > 5 {
		t.Errorf("far left: the left edge (red %d)", l)
	}
}

func TestWebpSize(t *testing.T) {
	b := make([]byte, 40)
	copy(b, "RIFF")
	copy(b[8:], "WEBPVP8X")
	b[24], b[25] = 0x1f, 0x03 // 800 - 1
	b[27], b[28] = 0x57, 0x02 // 600 - 1
	if w, h, ok := webpSize(b); !ok || w != 800 || h != 600 {
		t.Fatalf("VP8X %d %d %v", w, h, ok)
	}
	p, err := readPicture(b, 500, 200)
	if err != nil || p.Type != "image/webp" || !bytes.Equal(p.Bytes, b) {
		t.Fatalf("webp passed on: %+v %v", p, err)
	}
}

// the quarter of a sphere that looks at a place: straight ahead the middle, 180° the seam
func TestSphereView(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for x := 0; x < 400; x++ {
		for y := 0; y < 200; y++ {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / 399), 0, 0, 255})
		}
	}
	v := sphereView(img, 0, 90)
	if v.Bounds().Dx() != 100 || v.Bounds().Dy() != 80 {
		t.Fatalf("size %v", v.Bounds())
	}
	mid := colorOf(v.At(50, 40)).R
	if mid < 120 || mid > 135 {
		t.Errorf("straight ahead: the middle of the sphere (red %d)", mid)
	}
	back := sphereView(img, 180, 90)
	if l, r := colorOf(back.At(30, 10)).R, colorOf(back.At(70, 10)).R; l < 235 || r > 30 {
		t.Errorf("behind: over the seam (%d, %d)", l, r)
	}
}
