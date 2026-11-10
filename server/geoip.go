package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// The approximate place of a visitor from their IP address, so that the map opens near them before the
// device tells where it is (or when the visitor keeps it to themselves). The addresses are looked up in a
// file of this server, a MaxMind DB with a location for each network (DB-IP City Lite, monthly, CC BY 4.0;
// GeoLite2 City has the same fields): the address goes nowhere else and is not kept.

// Near is an approximate place: a town, or a country when the database names no town.
type Near struct {
	Lat, Lon float64
	Town     bool
}

// Zoom of the map for the place: a town with its surroundings, or the country.
func (n Near) Zoom() int {
	if n.Town {
		return 11
	}
	return 5
}

// geoIP is the database file, mapped into memory. A new file in its place (the monthly update, renamed
// over the old one) is opened within a minute; a file that does not open leaves the old one in use.
type geoIP struct {
	path string
	log  *slog.Logger

	mu sync.RWMutex // the database: lookups read it, a new file replaces it
	db *mmdb

	checking sync.Mutex   // one look at the file at a time; the others go on with the database they have
	checked  atomic.Int64 // the last look, in Unix nanoseconds
	seen     os.FileInfo  // the file last opened (or tried); nil: none yet
	missing  bool         // no file at the last look (said once in the log)
}

const geoIPCheck = time.Minute

func newGeoIP(path string, log *slog.Logger) *geoIP {
	g := &geoIP{path: path, log: log}
	g.check(time.Now())
	return g
}

// near finds the place of an address; false when the address or the database names none.
func (g *geoIP) near(ip string) (Near, bool) {
	if g == nil {
		return Near{}, false
	}
	if now := time.Now(); now.UnixNano()-g.checked.Load() > int64(geoIPCheck) {
		g.check(now)
	}
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return Near{}, false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return Near{}, false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.db == nil {
		return Near{}, false
	}
	n, err := g.db.near(a)
	if err != nil {
		g.log.Warn("geoip", "err", err)
		return Near{}, false
	}
	return n, n != Near{}
}

// check opens the file when it is new or changed.
func (g *geoIP) check(now time.Time) {
	if !g.checking.TryLock() {
		return
	}
	defer g.checking.Unlock()
	if last := g.checked.Load(); last != 0 && now.UnixNano()-last <= int64(geoIPCheck) {
		return
	}
	g.checked.Store(now.UnixNano())
	st, err := os.Stat(g.path)
	if err != nil {
		if !g.missing {
			g.log.Warn("geoip: no database, the map opens at its default place", "path", g.path, "err", err)
			g.missing = true
		}
		return
	}
	g.missing = false
	// a new file renamed over the old one is another file, even with the same time and size
	if g.seen != nil && os.SameFile(g.seen, st) && st.ModTime().Equal(g.seen.ModTime()) && st.Size() == g.seen.Size() {
		return
	}
	g.seen = st // a file that does not open is not tried again until it changes
	db, err := openMMDB(g.path)
	if err != nil {
		g.log.Error("geoip: database not opened", "path", g.path, "err", err)
		return
	}
	g.mu.Lock()
	old := g.db
	g.db = db
	g.mu.Unlock()
	if old != nil {
		old.close()
	}
	g.log.Info("geoip: database opened", "path", g.path, "type", db.kind, "built", time.Unix(int64(db.built), 0).UTC().Format(time.DateOnly))
}

// LookupNear opens a database file and finds the place of an address in it (the command hippomaps geoip).
func LookupNear(path, ip string) (Near, bool, error) {
	db, err := openMMDB(path)
	if err != nil {
		return Near{}, false, err
	}
	defer db.close()
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return Near{}, false, err
	}
	n, err := db.near(a.Unmap())
	return n, n != Near{}, err
}

// ------------------------------------------------------------------ MaxMind DB
// The format (https://maxmind.github.io/MaxMind-DB/): a binary tree of the bits of the address, 16 bytes of
// zeros, a data section of typed values, and the metadata after a marker at the end of the file.

type mmdb struct {
	file       []byte
	unmap      func()
	nodes      uint32
	recordBits int
	v6         bool
	tree       []byte
	data       []byte // the data section: pointers count from its start
	v4Start    uint32 // the node of ::/96, where IPv4 addresses begin in an IPv6 tree
	kind       string
	built      uint64
}

var mmdbMarker = []byte("\xab\xcd\xefMaxMind.com")

func openMMDB(path string) (*mmdb, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < int64(len(mmdbMarker))+16 || st.Size() > math.MaxInt32 {
		return nil, fmt.Errorf("%d bytes: not a database", st.Size())
	}
	b, unmap, err := mapFile(f, int(st.Size()))
	if err != nil {
		return nil, err
	}
	db, err := parseMMDB(b)
	if err != nil {
		unmap()
		return nil, err
	}
	db.unmap = unmap
	return db, nil
}

func (db *mmdb) close() {
	if db.unmap != nil {
		db.unmap()
	}
}

func parseMMDB(b []byte) (*mmdb, error) {
	tail := b[max(0, len(b)-128*1024):] // the metadata is within the last 128 KiB
	i := bytes.LastIndex(tail, mmdbMarker)
	if i < 0 {
		return nil, errors.New("no metadata: not a MaxMind DB")
	}
	metaAt := len(b) - len(tail) + i + len(mmdbMarker)
	v, _, err := (&decoder{b: b[metaAt:], left: maxValues}).decode(0, 0)
	if err != nil {
		return nil, fmt.Errorf("metadata: %w", err)
	}
	meta, _ := v.(map[string]any)
	db := &mmdb{file: b}
	nodes, ok1 := asUint(meta["node_count"])
	bits, ok2 := asUint(meta["record_size"])
	ipv, ok3 := asUint(meta["ip_version"])
	if !ok1 || !ok2 || !ok3 || nodes == 0 || nodes > math.MaxUint32 || bits != 24 && bits != 28 && bits != 32 || ipv != 4 && ipv != 6 {
		return nil, fmt.Errorf("metadata: node_count %v, record_size %v, ip_version %v", meta["node_count"], meta["record_size"], meta["ip_version"])
	}
	db.nodes, db.recordBits, db.v6 = uint32(nodes), int(bits), ipv == 6
	db.kind, _ = meta["database_type"].(string)
	db.built, _ = asUint(meta["build_epoch"])
	treeSize := int(nodes) * int(bits) / 4
	end := metaAt - len(mmdbMarker)
	if treeSize+16 > end {
		return nil, fmt.Errorf("a tree of %d bytes in a file of %d", treeSize, len(b))
	}
	db.tree, db.data = b[:treeSize], b[treeSize+16:end]
	if db.v6 {
		n := uint32(0)
		for i := 0; i < 96 && n < db.nodes; i++ {
			n = db.record(n, 0)
		}
		db.v4Start = n
	}
	return db, nil
}

// record is the left (0) or right (1) record of a node.
func (db *mmdb) record(node uint32, bit int) uint32 {
	switch db.recordBits {
	case 24:
		p := db.tree[node*6+uint32(bit)*3:]
		return uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
	case 28:
		p := db.tree[node*7:]
		if bit == 0 {
			return uint32(p[3]&0xf0)<<20 | uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
		}
		return uint32(p[3]&0x0f)<<24 | uint32(p[4])<<16 | uint32(p[5])<<8 | uint32(p[6])
	default:
		return binary.BigEndian.Uint32(db.tree[node*8+uint32(bit)*4:])
	}
}

// lookup walks the tree to the record of an address: nil when no network holds it.
func (db *mmdb) lookup(a netip.Addr) (any, error) {
	node, bits := uint32(0), 128
	if a.Is4() {
		bits = 32
		if db.v6 {
			node = db.v4Start
		}
	} else if !db.v6 {
		return nil, nil
	}
	ip := a.As16()
	if a.Is4() {
		copy(ip[:4], ip[12:])
	}
	for i := 0; i < bits && node < db.nodes; i++ {
		node = db.record(node, int(ip[i/8]>>(7-i%8)&1))
	}
	switch {
	case node == db.nodes:
		return nil, nil
	case node < db.nodes:
		return nil, errors.New("tree deeper than the address")
	}
	off := int(node-db.nodes) - 16
	if off < 0 || off >= len(db.data) {
		return nil, fmt.Errorf("record %d outside the data", off)
	}
	v, _, err := (&decoder{b: db.data, left: maxValues}).decode(off, 0)
	return v, err
}

// near reads location.latitude and location.longitude of the record of an address, and whether it names a town.
func (db *mmdb) near(a netip.Addr) (Near, error) {
	v, err := db.lookup(a)
	if err != nil || v == nil {
		return Near{}, err
	}
	rec, _ := v.(map[string]any)
	loc, _ := rec["location"].(map[string]any)
	lat, ok1 := asFloat(loc["latitude"])
	lon, ok2 := asFloat(loc["longitude"])
	if !ok1 || !ok2 || math.Abs(lat) > 90 || math.Abs(lon) > 180 || lat == 0 && lon == 0 {
		return Near{}, nil
	}
	_, town := rec["city"]
	return Near{Lat: lat, Lon: lon, Town: town}, nil
}

// decoder reads the values of a section; pointers count from its start. A record has some dozens of
// values: the limit stops a damaged file (maps that point back to themselves) early.
type decoder struct {
	b    []byte
	left int
}

const maxValues = 10000

// decode reads the value at off: the value and the offset after it.
func (d *decoder) decode(off, depth int) (any, int, error) {
	b := d.b
	if d.left--; d.left < 0 {
		return nil, 0, errors.New("too many values")
	}
	if depth > 32 {
		return nil, 0, errors.New("values nested too deep")
	}
	if off >= len(b) {
		return nil, 0, errors.New("value outside the data")
	}
	ctrl := b[off]
	off++
	typ := int(ctrl >> 5)
	if typ == 1 { // pointer: the value at another place; its size bits are part of the address
		ss, n := int(ctrl>>3&3), int(ctrl&7)
		if off+ss+1 > len(b) {
			return nil, 0, errors.New("pointer outside the data")
		}
		p := 0
		switch ss {
		case 0:
			p = n<<8 | int(b[off])
		case 1:
			p = (n<<16 | int(b[off])<<8 | int(b[off+1])) + 2048
		case 2:
			p = (n<<24 | int(b[off])<<16 | int(b[off+1])<<8 | int(b[off+2])) + 526336
		case 3:
			p = int(binary.BigEndian.Uint32(b[off:]))
		}
		if p < 0 || p >= len(b) || b[p]>>5 == 1 {
			return nil, 0, errors.New("pointer to nothing or to a pointer")
		}
		v, _, err := d.decode(p, depth+1)
		return v, off + ss + 1, err
	}
	if typ == 0 { // extended type
		if off >= len(b) {
			return nil, 0, errors.New("type outside the data")
		}
		typ = 7 + int(b[off])
		off++
	}
	size := int(ctrl & 31)
	if size >= 29 {
		n := size - 28
		if off+n > len(b) {
			return nil, 0, errors.New("size outside the data")
		}
		x := 0
		for _, c := range b[off : off+n] {
			x = x<<8 | int(c)
		}
		size = [...]int{0, 29, 285, 65821}[n] + x
		off += n
	}
	if typ == 7 || typ == 11 || typ == 14 { // map, array, boolean: the size counts entries or is the value
		switch typ {
		case 14:
			return size != 0, off, nil
		case 7:
			m := make(map[string]any, min(size, 64))
			for range size {
				k, next, err := d.decode(off, depth+1)
				if err != nil {
					return nil, 0, err
				}
				key, ok := k.(string)
				if !ok {
					return nil, 0, errors.New("a key that is not text")
				}
				v, next2, err := d.decode(next, depth+1)
				if err != nil {
					return nil, 0, err
				}
				m[key], off = v, next2
			}
			return m, off, nil
		default:
			a := make([]any, 0, min(size, 64))
			for range size {
				v, next, err := d.decode(off, depth+1)
				if err != nil {
					return nil, 0, err
				}
				a, off = append(a, v), next
			}
			return a, off, nil
		}
	}
	if off+size > len(b) {
		return nil, 0, errors.New("value outside the data")
	}
	p := b[off : off+size]
	off += size
	switch typ {
	case 2: // text
		return string(p), off, nil
	case 3: // double
		if size != 8 {
			return nil, 0, errors.New("a double not of 8 bytes")
		}
		return math.Float64frombits(binary.BigEndian.Uint64(p)), off, nil
	case 15: // float
		if size != 4 {
			return nil, 0, errors.New("a float not of 4 bytes")
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(p))), off, nil
	case 5, 6, 9: // uint16, uint32, uint64
		if size > 8 {
			return nil, 0, errors.New("an integer of more than 8 bytes")
		}
		var x uint64
		for _, c := range p {
			x = x<<8 | uint64(c)
		}
		return x, off, nil
	case 8: // int32
		if size > 4 {
			return nil, 0, errors.New("an int32 of more than 4 bytes")
		}
		var x uint32
		for _, c := range p {
			x = x<<8 | uint32(c)
		}
		if size == 4 {
			return int64(int32(x)), off, nil
		}
		return int64(x), off, nil
	case 4, 10: // bytes, uint128: not needed here
		return nil, off, nil
	}
	return nil, 0, fmt.Errorf("unknown type %d", typ)
}

func asUint(v any) (uint64, bool) {
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		return uint64(x), x >= 0
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case uint64:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}
