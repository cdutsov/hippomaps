package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// the readings of overpass-search, as it writes them, in German, French and Italian
func TestUnderstoodIn(t *testing.T) {
	for _, c := range []struct{ en, de, fr, it string }{
		{"Restaurant, in the view", "Restaurant, im Kartenausschnitt", "Restaurant, dans la vue de la carte", "Ristorante, nella vista della mappa"},
		{"Fast Food, in the view", "Schnellimbiss, im Kartenausschnitt", "Restauration rapide, dans la vue de la carte", "Fast food, nella vista della mappa"},
		{"Restaurant, inside the boundary of Zürich", "Restaurant, in Zürich", "Restaurant, dans les limites de Zürich", "Ristorante, entro i confini di Zürich"},
		{"Cafe, within 1 km of your location", "Café, im Umkreis von 1 km um Ihren Standort", "Café, à moins de 1 km de votre position", "Caffè, entro 1 km dalla tua posizione"},
		{"Restaurant · open now, in the view", "Restaurant · jetzt offen, im Kartenausschnitt", "Restaurant · ouvert maintenant, dans la vue de la carte", "Ristorante · aperto ora, nella vista della mappa"},
		{"vegan: Restaurant, inside the boundary of Zürich", "vegan: Restaurant, in Zürich", "végan: Restaurant, dans les limites de Zürich", "vegano: Ristorante, entro i confini di Zürich"},
		{"Hotel, by Limmat (within 150 m) in Zürich", "Hotel, an Limmat (bis 150 m) in Zürich", "Hôtel, au bord de Limmat (à moins de 150 m) à Zürich", "Albergo, presso Limmat (entro 150 m) a Zürich"},
		{"Fuel stations, along the A3 (within 300 m)", "Tankstellen, entlang der A3 (bis 300 m)", "Stations-service, le long de l’A3 (à moins de 300 m)", "Distributori di carburante, lungo l’A3 (entro 300 m)"},
		{"Peak · elevation over 4'000 m, in the box around Wallis", "Berggipfel · Höhe über 4'000 m, im Rechteck um Wallis", "Sommet · altitude plus de 4'000 m, dans le rectangle autour de Wallis", "Picco · altitudine oltre 4'000 m, nel rettangolo intorno a Wallis"},
		{"chain “Migros”, in 50 × 50 km around the view", "Kette “Migros”, in 50 × 50 km um den Kartenausschnitt", "chaîne “Migros”, dans 50 × 50 km autour de la vue de la carte", "catena “Migros”, in 50 × 50 km intorno alla vista della mappa"},
		{"Cafés, Restaurant, in the view", "Cafés, Restaurant, im Kartenausschnitt", "Cafés, Restaurant, dans la vue de la carte", "Caffè, Ristorante, nella vista della mappa"},
		{"Bern (Schweiz · city)", "Bern (Schweiz · Stadt)", "Bern (Schweiz · ville)", "Bern (Schweiz · città)"},
		{"something we do not know", "something we do not know", "something we do not know", "something we do not know"},
	} {
		for i, want := range []string{c.de, c.fr, c.it} {
			if got := understoodIn(c.en, i+1); got != want {
				t.Errorf("%q in %s: %q, want %q", c.en, []string{"de", "fr", "it"}[i], got, want)
			}
		}
	}
}

// the relay: the reading, the other readings by their preset, the notes, the kinds of the places found and
// the details of the suggestions in the language of the page; English and other languages as they came
func TestSearchInLanguage(t *testing.T) {
	answers := map[string]string{
		"/labs/query": `{"text":"restaurants","understood":"Restaurant · open now, in the view","exact":true,
			"alternatives":[{"label":"Fast Food","what":"amenity/fast_food"},{"label":"Outdoor Seating Area","what":"leisure/outdoor_seating"},{"label":"Cafés","what":"ours/coffee"}],
			"notes":["The query finds all places with opening hours. It does not check the hours.","Something new."]}`,
		"/labs/search": `{"understood":"Restaurant, in the view","total":2,"results":{"type":"FeatureCollection","features":[
			{"type":"Feature","geometry":{"type":"Point","coordinates":[8.5,47.3]},"properties":{"id":"node/1","name":"A","type":"amenity=fast_food","n":1.50}},
			{"type":"Feature","geometry":{"type":"Point","coordinates":[8.5,47.3]},"properties":{"id":"node/2","name":"B & C","type":"shop=nothing_known"}}]},
			"try":[{"label":"cafés in Thun","q":"cafés in Thun"}],"places":[{"label":"Bern","detail":"Schweiz · city","place":"relation/1"}]}`,
		"/labs/complete": `{"suggestions":[{"kind":"what","text":"Restaurant chinois","label":"Restaurant chinois","detail":"Chinese Restaurant · amenity=restaurant + cuisine=chinese"},
			{"kind":"place","text":"Schnell","label":"Schnell","detail":"Illinois, Vereinigte Staaten von Amerika · hamlet","place":"node/1"}]}`,
		"/labs/here": `{"place":{"label":"Waaggasse 3, Zürich","detail":"Altstadt, Zürich, Schweiz · address"},"results":{"features":[{"properties":{"type":"amenity=pharmacy"}}]}}`,
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, answers[r.URL.Path])
	}))
	t.Cleanup(up.Close)
	s := testServer(t, nil, func(c *Config) { c.Base = "/"; c.SearchUpstream = up.URL + "/labs" })
	ask := func(path string) map[string]any {
		t.Helper()
		w := get(s, path)
		var d map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return d
	}
	at := func(v any, path ...any) any {
		for _, p := range path {
			switch k := p.(type) {
			case string:
				v = v.(map[string]any)[k]
			case int:
				v = v.([]any)[k]
			}
		}
		return v
	}

	d := ask("/api/search/query?q=restaurants&lang=de")
	if got := at(d, "understood"); got != "Restaurant · jetzt offen, im Kartenausschnitt" {
		t.Errorf("reading: %v", got)
	}
	var alts []string
	for _, a := range d["alternatives"].([]any) {
		alts = append(alts, at(a, "label").(string))
	}
	if strings.Join(alts, "|") != "Schnellimbiss|Aussengastronomie|Cafés" || at(d, "alternatives", 0, "what") != "amenity/fast_food" {
		t.Errorf("other readings: %v", d["alternatives"])
	}
	if at(d, "notes", 0) != "Die Abfrage findet alle Orte mit Öffnungszeiten. Sie prüft die Zeiten nicht." || at(d, "notes", 1) != "Something new." {
		t.Errorf("notes: %v", d["notes"])
	}
	if at(ask("/api/search/query?q=restaurants&lang=fr"), "alternatives", 0, "label") != "Restauration rapide" {
		t.Errorf("French")
	}

	d = ask("/api/search/search?q=restaurants&lang=it")
	if at(d, "understood") != "Ristorante, nella vista della mappa" || at(d, "results", "features", 0, "properties", "kind") != "Fast food" {
		t.Errorf("search in Italian: %v %v", d["understood"], at(d, "results", "features", 0, "properties"))
	}
	if _, ok := at(d, "results", "features", 1, "properties").(map[string]any)["kind"]; ok {
		t.Errorf("a kind for a tag without a preset")
	}
	if w := get(s, "/api/search/search?q=x&lang=it"); !strings.Contains(w.Body.String(), `"n":1.50`) || !strings.Contains(w.Body.String(), `"B & C"`) {
		t.Errorf("numbers and text kept as they came: %s", w.Body)
	}
	if at(d, "try", 0, "label") != "caffè a Thun" || at(d, "try", 0, "q") != "caffè a Thun" || at(d, "places", 0, "detail") != "Schweiz · città" {
		t.Errorf("tries and places: %v %v", d["try"], d["places"])
	}

	d = ask("/api/search/complete?q=chin&lang=de")
	if at(d, "suggestions", 0, "detail") != "Chinesisches Restaurant · amenity=restaurant + cuisine=chinese" || at(d, "suggestions", 1, "detail") != "Illinois, Vereinigte Staaten von Amerika · Weiler" {
		t.Errorf("suggestions: %v", d["suggestions"])
	}
	d = ask("/api/search/here?lat=47.37&lon=8.54&zoom=17&lang=de")
	if at(d, "place", "detail") != "Altstadt, Zürich, Schweiz · Adresse" || at(d, "results", "features", 0, "properties", "kind") != "Apotheke" {
		t.Errorf("a tap on the map: %v", d)
	}

	// English, and a language of no texts of the page, as they came
	for _, q := range []string{"lang=en", "lang=es", ""} {
		if w := get(s, "/api/search/query?q=restaurants&"+q); w.Body.String() != answers["/labs/query"] {
			t.Errorf("%s: the answer changed: %s", q, w.Body)
		}
	}
}
