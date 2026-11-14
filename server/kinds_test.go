package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// The names of the kinds of POIs of the tiles: by value (and value/detail), the key that wins where several have
// the value, the language of the page, English for the others, and the page that says where they are.
func TestKinds(t *testing.T) {
	s := testServer(t, nil)
	names := func(lang string) map[string]string {
		w := get(s, "/labs/maps/api/kinds?lang="+lang)
		var m map[string]string
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &m) != nil {
			t.Fatalf("kinds?lang=%s: %d %s", lang, w.Code, w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
			t.Errorf("Cache-Control %q", cc)
		}
		return m
	}
	en, de, fr, it, other := names("en"), names("de"), names("fr"), names("it"), names("xx")
	for _, c := range []struct {
		m          map[string]string
		kind, want string
	}{
		{en, "supermarket", "Supermarket"},
		{de, "supermarket", "Supermarkt"},
		{fr, "supermarket", "Supermarché"},
		{it, "supermarket", "Supermercato"},
		{de, "place_of_worship", "Andachtsstätte"},
		{de, "place_of_worship/christian", "Christliche Kirche"},
		{en, "station", "Train Station"}, // railway before public_transport
		{en, "hotel", "Hotel"},           // tourism before building
		{de, "restaurant", "Restaurant"}, // no German name in the presets: the English one
		{other, "supermarket", "Supermarket"},
		{de, "tram_stop", "Tramhaltestelle"}, // ours, not "Strassenbahn-Halteplatz"
		{en, "forest", "Forest"},             // not "Managed Forest"
		{fr, "school", "École"},
	} {
		if got := c.m[c.kind]; got != c.want {
			t.Errorf("%s: %q, want %q", c.kind, got, c.want)
		}
	}
	for _, k := range []string{"administrative", "yes", "other", "@templates/contact", "contact"} {
		if v, ok := en[k]; ok {
			t.Errorf("%s has a name: %q", k, v)
		}
	}
	if w := get(s, "/labs/maps"); !strings.Contains(w.Body.String(), `data-kinds="/labs/maps/api/kinds"`) {
		t.Errorf("the page has no data-kinds")
	}
}
