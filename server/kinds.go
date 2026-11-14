package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// The names of the kinds of POIs of the map tiles, for the popup of a POI the page shows on a tap or under the mouse.
// A POI of the tiles carries its name and the value of one tag (kind "supermarket", "place_of_worship"), with a detail
// for some (kind_detail "christian"), but not the key of the tag. Its name is that of the preset of the first key in
// kindKeys that has the value (the iD presets of labels/presets.json), in the language of the page:
//
//	GET {base}/api/kinds?lang=de   {"supermarket": "Supermarkt", "place_of_worship/christian": "Christliche Kirche", ...}
//
// A language other than de, fr and it gets the English names. The answer changes only with a new build.

// kindKeys: the keys a value of the tiles belongs to, the first that has a preset with it wins ("station" is a
// station of the railway, "hotel" one of tourism, not a building)
var kindKeys = []string{"amenity", "shop", "tourism", "leisure", "healthcare", "historic", "railway", "public_transport",
	"aeroway", "office", "craft", "emergency", "man_made", "natural", "landuse", "highway", "sport", "waterway", "boundary",
	"place", "club", "attraction", "building", "barrier", "power"}

// kindNot: values whose preset names something else than the POI of the tiles ("administrative" are the names of
// quarters, not an office), or nothing
var kindNot = map[string]bool{"administrative": true, "yes": true, "other": true}

// kindOwn: our names where that of the preset names the tagging rather than the place ("Tram Stopping Position",
// "Bushaltestelle (Wartebereich)", "Managed Forest" for every forest of the tiles, "School Grounds"): English, German,
// French, Italian
var kindOwn = map[string][4]string{
	"bus_stop":     {"Bus Stop", "Bushaltestelle", "Arrêt de bus", "Fermata dell’autobus"},
	"tram_stop":    {"Tram Stop", "Tramhaltestelle", "Arrêt de tram", "Fermata del tram"},
	"forest":       {"Forest", "Wald", "Forêt", "Bosco"},
	"pharmacy":     {"Pharmacy", "Apotheke", "Pharmacie", "Farmacia"},
	"school":       {"School", "Schule", "École", "Scuola"},
	"university":   {"University", "Universität", "Université", "Università"},
	"college":      {"College", "Hochschule", "Haute école", "Istituto superiore"},
	"hospital":     {"Hospital", "Spital", "Hôpital", "Ospedale"},
	"kindergarten": {"Kindergarten", "Kindergarten", "École enfantine", "Scuola dell’infanzia"},
}

// kindJSON is the answer for each language (0 English, then langs)
var kindJSON = func() [4][]byte {
	rank := map[string]int{}
	for i, k := range kindKeys {
		rank[k] = i + 1
	}
	type pick struct {
		rank  int
		names [4]string
	}
	best := map[string]pick{}
	for id, n := range presets.byID {
		parts := strings.Split(id, "/")
		r := rank[parts[0]]
		if r == 0 || len(parts) < 2 || len(parts) > 3 || kindNot[parts[1]] || n[0] == "" {
			continue
		}
		v := strings.Join(parts[1:], "/")
		if old, ok := best[v]; !ok || r < old.rank {
			best[v] = pick{r, n}
		}
	}
	for v, n := range kindOwn {
		best[v] = pick{0, n}
	}
	var out [4][]byte
	for i := range out {
		m := make(map[string]string, len(best))
		for v, p := range best {
			m[v] = p.names[0]
			if i > 0 && p.names[i] != "" {
				m[v] = p.names[i]
			}
		}
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		out[i] = b
	}
	return out
}()

func (s *Server) kinds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(kindJSON[langs[r.URL.Query().Get("lang")]])
}
