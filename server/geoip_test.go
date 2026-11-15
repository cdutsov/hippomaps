package server

import (
	"encoding/binary"
	"io/fs"
	"math"
	"math/rand/v2"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/cdutsov/hippomaps/web"
)

// A MaxMind DB written here, as DB-IP City Lite has it: records with city, country and location, a
// record reached through a pointer, sizes of one, two and three bytes, in each of the three record sizes.

type mmdbEnc struct{ b []byte }

func (e *mmdbEnc) ctrl(typ, size int) {
	first, ext := byte(0), -1
	if typ <= 7 {
		first = byte(typ << 5)
	} else {
		ext = typ - 7
	}
	var sz []byte
	switch {
	case size < 29:
		first |= byte(size)
	case size < 285:
		first |= 29
		sz = []byte{byte(size - 29)}
	case size < 65821:
		first |= 30
		sz = []byte{byte((size - 285) >> 8), byte(size - 285)}
	default:
		first |= 31
		s := size - 65821
		sz = []byte{byte(s >> 16), byte(s >> 8), byte(s)}
	}
	e.b = append(e.b, first)
	if ext >= 0 {
		e.b = append(e.b, byte(ext))
	}
	e.b = append(e.b, sz...)
}
func (e *mmdbEnc) str(s string) { e.ctrl(2, len(s)); e.b = append(e.b, s...) }
func (e *mmdbEnc) double(f float64) {
	e.ctrl(3, 8)
	e.b = binary.BigEndian.AppendUint64(e.b, math.Float64bits(f))
}
func (e *mmdbEnc) float(f float32) {
	e.ctrl(15, 4)
	e.b = binary.BigEndian.AppendUint32(e.b, math.Float32bits(f))
}
func (e *mmdbEnc) uint(typ int, x uint64) {
	var p []byte
	for ; x > 0; x >>= 8 {
		p = append([]byte{byte(x)}, p...)
	}
	e.ctrl(typ, len(p))
	e.b = append(e.b, p...)
}
func (e *mmdbEnc) ptr(p int) {
	if p < 2048 {
		e.b = append(e.b, byte(1<<5|p>>8&7), byte(p))
		return
	}
	q := p - 2048
	e.b = append(e.b, byte(1<<5|1<<3|q>>16&7), byte(q>>8), byte(q))
}

type testNet struct {
	prefix string
	rec    int // offset of its record in the data section
}

// writeMMDB writes an IPv6 tree (IPv4 networks under ::/96) of the networks and their records.
func writeMMDB(recordBits int, nets []testNet, data []byte) []byte {
	const empty, child, rec = 0, 1, 2
	type slot struct{ kind, v int }
	nodes := [][2]slot{{}}
	for _, n := range nets {
		p := netip.MustParsePrefix(n.prefix)
		ip, bits := p.Addr().As16(), p.Bits()
		if p.Addr().Is4() {
			ip, bits = netip.AddrFrom4(p.Addr().As4()).As16(), bits+96
			ip[10], ip[11] = 0, 0 // ::a.b.c.d, not ::ffff:a.b.c.d
		}
		node := 0
		for i := range bits {
			bit := int(ip[i/8] >> (7 - i%8) & 1)
			if i == bits-1 {
				nodes[node][bit] = slot{rec, n.rec}
				break
			}
			if nodes[node][bit].kind != child {
				nodes = append(nodes, [2]slot{})
				nodes[node][bit] = slot{child, len(nodes) - 1}
			}
			node = nodes[node][bit].v
		}
	}
	count := len(nodes)
	val := func(s slot) uint32 {
		switch s.kind {
		case child:
			return uint32(s.v)
		case rec:
			return uint32(count + 16 + s.v)
		}
		return uint32(count)
	}
	var out []byte
	for _, n := range nodes {
		l, r := val(n[0]), val(n[1])
		switch recordBits {
		case 24:
			out = append(out, byte(l>>16), byte(l>>8), byte(l), byte(r>>16), byte(r>>8), byte(r))
		case 28:
			out = append(out, byte(l>>16), byte(l>>8), byte(l), byte(l>>24<<4|r>>24&0x0f), byte(r>>16), byte(r>>8), byte(r))
		default:
			out = binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(out, l), r)
		}
	}
	out = append(out, make([]byte, 16)...)
	out = append(out, data...)
	out = append(out, mmdbMarker...)
	m := &mmdbEnc{}
	m.ctrl(7, 8)
	m.str("node_count")
	m.uint(6, uint64(count))
	m.str("record_size")
	m.uint(5, uint64(recordBits))
	m.str("ip_version")
	m.uint(5, 6)
	m.str("database_type")
	m.str("DBIP-City-Lite")
	m.str("build_epoch")
	m.uint(9, 1793404800)
	m.str("languages")
	m.ctrl(11, 1)
	m.str("en")
	m.str("binary_format_major_version")
	m.uint(5, 2)
	m.str("description")
	m.ctrl(7, 1)
	m.str("en")
	m.str("test")
	return append(out, m.b...)
}

// testDB: 192.0.2.0/24 a town (Zurich), 2001:db8:1::/48 the same record, 198.51.100.0/25 a country only
// (its location through a pointer beyond 2048 bytes), 203.0.113.0/24 a record without a location.
func testDB(recordBits int, lat, lon float64) []byte {
	e := &mmdbEnc{}
	e.str("en")                      // at 0: a key reached through a pointer of two bytes
	e.str(strings.Repeat("x", 2100)) // a size of two bytes, and pointers of three bytes beyond it
	town := len(e.b)
	e.ctrl(7, 5)
	e.str("city")
	e.ctrl(7, 2)
	e.str("names")
	e.ctrl(7, 1)
	e.ptr(0)
	e.str("Zürich")
	e.str("geoname_id")
	e.uint(6, 2657896)
	e.str("country")
	e.ctrl(7, 2)
	e.str("iso_code")
	e.str("CH")
	e.str("is_in_european_union")
	e.ctrl(14, 0)
	e.str("location")
	e.ctrl(7, 3)
	e.str("latitude")
	e.double(lat)
	e.str("longitude")
	e.double(lon)
	e.str("accuracy")
	e.float(20)
	e.str("subdivisions")
	e.ctrl(11, 1)
	e.ctrl(7, 1)
	e.str("iso_code")
	e.str("ZH")
	e.str("note")
	e.str(strings.Repeat("y", 40)) // a size of one byte
	country := len(e.b)
	e.ctrl(7, 2)
	e.str("country")
	e.ctrl(7, 1)
	e.str("iso_code")
	e.str("DE")
	e.str("location")
	e.ptr(len(e.b) + 3) // a pointer of three bytes to the map after it
	e.ctrl(7, 2)
	e.str("latitude")
	e.double(51)
	e.str("longitude")
	e.uint(8, 9) // an int32
	noLoc := len(e.b)
	e.ctrl(7, 1)
	e.ptr(town + 1) // the key "city" of Zurich
	e.ctrl(7, 0)
	return writeMMDB(recordBits, []testNet{{"192.0.2.0/24", town}, {"2001:db8:1::/48", town}, {"198.51.100.0/25", country}, {"203.0.113.0/24", noLoc}}, e.b)
}

func writeTestDB(t *testing.T, path string, lat, lon float64) {
	t.Helper()
	tmp := path + ".next"
	if err := os.WriteFile(tmp, testDB(28, lat, lon), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil { // as the monthly update replaces it
		t.Fatal(err)
	}
}

func TestGeoIPLookup(t *testing.T) {
	for _, bits := range []int{24, 28, 32} {
		db, err := parseMMDB(testDB(bits, 47.3769, 8.5417))
		if err != nil {
			t.Fatalf("%d: %v", bits, err)
		}
		if db.kind != "DBIP-City-Lite" || db.built != 1793404800 {
			t.Errorf("%d: metadata %q %d", bits, db.kind, db.built)
		}
		for ip, want := range map[string]Near{
			"192.0.2.77":         {47.3769, 8.5417, true},
			"::ffff:192.0.2.1":   {47.3769, 8.5417, true},
			"2001:db8:1:ff::5":   {47.3769, 8.5417, true},
			"198.51.100.5":       {51, 9, false},
			"198.51.100.200":     {}, // outside the /25
			"203.0.113.9":        {}, // no location
			"2001:db8:2::1":      {},
			"192.0.3.1":          {},
			"0.0.0.0":            {},
			"ffff:ffff::1":       {},
			"2001:db8:1:ffff::1": {47.3769, 8.5417, true},
		} {
			got, err := db.near(netip.MustParseAddr(ip).Unmap())
			if err != nil || got != want {
				t.Errorf("%d %s: %+v %v, want %+v", bits, ip, got, err, want)
			}
		}
	}
	if (Near{Town: true}).Zoom() != 11 || (Near{}).Zoom() != 5 {
		t.Error("zoom")
	}
}

// A damaged file fails or finds nothing, never brings the server down.
func TestGeoIPDamaged(t *testing.T) {
	good := testDB(24, 47.37, 8.54)
	for _, b := range [][]byte{nil, []byte("not a database"), good[:len(good)/2], good[:len(good)-20]} {
		if _, err := parseMMDB(b); err == nil {
			t.Errorf("%d bytes: no error", len(b))
		}
	}
	// a map whose two values point back to it: 2^32 values without a limit
	e := &mmdbEnc{}
	e.ctrl(7, 2)
	e.str("a")
	e.ptr(0)
	e.str("b")
	e.ptr(0)
	loop, err := parseMMDB(writeMMDB(24, []testNet{{"192.0.2.0/24", 0}}, e.b))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.near(netip.MustParseAddr("192.0.2.1")); err == nil {
		t.Error("a map that points to itself: no error")
	}
	rnd := rand.New(rand.NewPCG(1, 2))
	ips := []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("198.51.100.1"), netip.MustParseAddr("2001:db8:1::1"), netip.MustParseAddr("203.0.113.1")}
	for range 3000 {
		b := append([]byte(nil), good...)
		for range 1 + rnd.IntN(8) {
			b[rnd.IntN(len(b))] = byte(rnd.IntN(256))
		}
		db, err := parseMMDB(b)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			db.near(ip) // must not panic
		}
	}
}

func TestGeoIPPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	writeTestDB(t, path, 47.3769, 8.5417)
	s := testServer(t, nil, func(c *Config) { c.GeoIPDB = path })
	w := get(s, "/labs/maps", "X-Forwarded-For", "192.0.2.10, 10.0.0.1")
	if !strings.Contains(w.Body.String(), ` data-near="47.377,8.542,11">`) {
		t.Errorf("a visitor in Zurich: %s", w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "private") || !strings.Contains(cc, "no-cache") {
		t.Errorf("cache-control: %q", cc)
	}
	if w := get(s, "/labs/maps", "X-Forwarded-For", "198.51.100.3"); !strings.Contains(w.Body.String(), ` data-near="51.000,9.000,5"`) {
		t.Errorf("a country only: %s", w.Body.String())
	}
	for _, ip := range []string{"203.0.113.9", "10.1.2.3", "127.0.0.1", "not an address", "192.0.3.1"} {
		if w := get(s, "/labs/maps", "X-Forwarded-For", ip); strings.Contains(w.Body.String(), "data-near") {
			t.Errorf("%s: a place", ip)
		}
	}
	if w := get(testServer(t, nil), "/labs/maps", "X-Forwarded-For", "192.0.2.10"); strings.Contains(w.Body.String(), "data-near") {
		t.Error("a place without a database")
	}
	// the page opens the map there and takes the sun from there, unless it has kept the last location of the device;
	// the map does not credit DB-IP
	app, _ := fs.ReadFile(web.Files, "app.js")
	for _, want := range []string{"root.dataset.near", "let meForSun = LAST_ME ? [LAST_ME[1], LAST_ME[0]] : NEAR ?",
		"center: LAST_ME ? [LAST_ME[1], LAST_ME[0]] : NEAR ? [NEAR[1], NEAR[0]]"} {
		if !strings.Contains(string(app), want) {
			t.Errorf("app.js: no %s", want)
		}
	}
	if strings.Contains(string(app), "db-ip.com") {
		t.Error("app.js: the credit of DB-IP on the map")
	}
	// the privacy policy credits DB-IP with a link (CC BY 4.0), in English and German
	for _, f := range []string{"privacy.html", "privacy-de.html"} {
		b, _ := fs.ReadFile(web.Files, f)
		if !strings.Contains(string(b), `<a href="https://db-ip.com">IP Geolocation by DB-IP</a>`) {
			t.Errorf("%s: no credit of DB-IP", f)
		}
	}
}

// The monthly file, renamed over the old one, is opened within a minute; a missing file at the start
// is opened once there; a damaged file leaves the old one in use.
func TestGeoIPReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	g := newGeoIP(path, testServer(t, nil).log)
	if _, ok := g.near("192.0.2.1"); ok {
		t.Fatal("a place without a file")
	}
	writeTestDB(t, path, 47.3769, 8.5417)
	if _, ok := g.near("192.0.2.1"); ok {
		t.Error("the file looked at again before a minute")
	}
	g.checked.Store(1) // a minute later
	if n, ok := g.near("192.0.2.1"); !ok || n.Lat != 47.3769 {
		t.Fatalf("the new file: %+v %v", n, ok)
	}
	writeTestDB(t, path, 46.948, 7.4474)
	g.checked.Store(1)
	if n, _ := g.near("192.0.2.1"); n.Lat != 46.948 || n.Lon != 7.4474 {
		t.Errorf("the update: %+v", n)
	}
	if err := os.WriteFile(path+".next", []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Rename(path+".next", path)
	g.checked.Store(1)
	if n, _ := g.near("192.0.2.1"); n.Lat != 46.948 {
		t.Errorf("a damaged file: %+v", n)
	}
	if n, ok, err := LookupNear(path, "192.0.2.1"); err == nil || ok {
		t.Errorf("hippomaps geoip of a damaged file: %+v %v %v", n, ok, err)
	}
	writeTestDB(t, path, 47.3769, 8.5417)
	if n, ok, err := LookupNear(path, "192.0.2.1"); err != nil || !ok || n.Lat != 47.3769 {
		t.Errorf("hippomaps geoip: %+v %v %v", n, ok, err)
	}
}
