package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		{"Bus line 31, in the view", "Buslinie 31, im Kartenausschnitt", "Ligne de bus 31, dans la vue de la carte", "Linea bus 31, nella vista della mappa"},
		{"Minibus line 31, in the view", "Minibuslinie 31, im Kartenausschnitt", "Ligne de minibus 31, dans la vue de la carte", "Linea minibus 31, nella vista della mappa"},
		{"Train line S3, inside the boundary of Zürich", "Linie S3, in Zürich", "Ligne S3, dans les limites de Zürich", "Linea S3, entro i confini di Zürich"},
		{"Cafe named “board game”, in the view", "Café namens “board game”, im Kartenausschnitt", "Café nommé “board game”, dans la vue de la carte", "Caffè chiamato “board game”, nella vista della mappa"},
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

// the notes of a line found by its number with letters beside it, of a name left out, and of a search
// of more places than Overpass gives
func TestLineNotesIn(t *testing.T) {
	for _, c := range []struct{ en, de string }{
		{"No line is named 28 exactly: the search shows those with letters beside the number (28E, M3).",
			"Keine Linie heisst genau 28: die Suche zeigt jene mit Buchstaben neben der Nummer (28E, M3)."},
		{"Nothing named “jazz” was found: the search leaves out the name.", "Nichts mit dem Namen “jazz” gefunden: die Suche lässt den Namen weg."},
		{"Only the first 25'000 of 31'200 were tested.", "Nur die ersten 25'000 von 31'200 wurden geprüft."},
	} {
		if got := noteIn(c.en, 1); got != c.de {
			t.Errorf("%q: %q, want %q", c.en, got, c.de)
		}
		if got := noteIn(c.en, 3); got == c.en {
			t.Errorf("%q: not in Italian", c.en)
		}
	}
}

// a reading with a long text of the visitor in it: each part is read once (the ways to read it doubled with
// each ", " and each " in the view"), and one longer than maxUnderstood stays as it came
func TestUnderstoodInLongText(t *testing.T) {
	for _, n := range []int{14, 40} {
		s := "Cafe named “" + strings.Repeat("a, ", n) + "a" + strings.Repeat(" in the view", n) + "”, in the view"
		if n == 14 && len(s) > maxUnderstood {
			t.Fatalf("the short text is %d bytes, over maxUnderstood", len(s))
		}
		done := make(chan string, 1)
		go func() { done <- understoodIn(s, 1) }()
		select {
		case got := <-done:
			if n == 40 && got != s {
				t.Errorf("a reading of %d bytes was changed: %q", len(s), got)
			}
			if n == 14 && !strings.HasSuffix(got, "im Kartenausschnitt") {
				t.Errorf("a reading of %d bytes: %q", len(s), got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("a reading of %d bytes takes more than 5 s", len(s))
		}
	}
	// the longest text read, with as many parts as fit
	s := strings.Repeat("a, ", 60) + strings.Repeat(" in the view", 30)
	s = s[:min(len(s), maxUnderstood-13)] + ", in the view"
	t0 := time.Now()
	understoodIn(s, 2)
	if d := time.Since(t0); d > 2*time.Second {
		t.Errorf("a reading of %d bytes took %s", len(s), d)
	}
}

// the kind of a place found, in the detail of the place and of a suggestion: the words of the geocoder, else
// the name of the preset of the kind; a kind not known stays
func TestDetailIn(t *testing.T) {
	stop := presetName("highway/bus_stop", 1)
	if stop == "" || stop == "bus stop" {
		t.Fatalf("no German name of highway/bus_stop: %q", stop)
	}
	for _, c := range []struct{ en, de string }{
		{"Schweiz · city", "Schweiz · Stadt"},
		{"Bern, Schweiz · isolated dwelling", "Bern, Schweiz · Einzelhof"},
		{"Bern, Schweiz · bus stop", "Bern, Schweiz · " + stop},
		{"bus stop", stop},
		{"village", "Dorf"},
		{"Bern, Schweiz", "Bern, Schweiz"},
		{"Bern · flux capacitor", "Bern · flux capacitor"},
	} {
		if got := detailIn(c.en, 1); got != c.de {
			t.Errorf("%q: %q, want %q", c.en, got, c.de)
		}
	}
	if got := understoodIn("Bahnhof (bus stop)", 1); got != "Bahnhof ("+stop+")" {
		t.Errorf("a place found: %q", got)
	}
	for s, want := range map[string]string{"cafés in Bern": "Cafés in Bern", "benches near Thun": "Sitzbänke bei Thun", "dragons in Bern": "dragons in Bern", "cafés": "cafés"} {
		if got := tryIn(s, 1); got != want {
			t.Errorf("try %q: %q, want %q", s, got, want)
		}
	}
	// an answer that is not JSON, a path and a language not known: as they came
	for _, c := range [][3]string{{"<html>", "search", "de"}, {`{"understood":"Restaurant, in the view"}`, "status", "de"}, {`{"understood":"Restaurant, in the view"}`, "search", "es"}} {
		if got := translate([]byte(c[0]), c[1], c[2]); string(got) != c[0] {
			t.Errorf("translate %v: %s", c, got)
		}
	}
}

// The times a search asks places to be open at (overpass-search: the reading and open.label) in German, French and
// Italian: the count above the list said "80 open on Sunday, von 118 mit Öffnungszeiten".
func TestOpenIn(t *testing.T) {
	for _, c := range []struct{ en, de, fr, it string }{
		{"open now", "jetzt offen", "ouvert maintenant", "aperto ora"},
		{"open today", "heute offen", "ouvert aujourd’hui", "aperto oggi"},
		{"open at 23:00 today", "heute um 23:00 offen", "ouvert aujourd’hui à 23:00", "aperto oggi alle 23:00"},
		{"open on Saturday and Sunday", "am Samstag und Sonntag offen", "ouvert samedi et dimanche", "aperto sabato e domenica"},
		{"open on Sunday", "am Sonntag offen", "ouvert le dimanche", "aperto la domenica"},
		{"open on Monday at 07:30", "am Montag um 07:30 offen", "ouvert le lundi à 07:30", "aperto lunedì alle 07:30"},
		{"something else", "something else", "something else", "something else"},
	} {
		for i, want := range []string{c.de, c.fr, c.it} {
			if got := openIn(c.en, i+1); got != want {
				t.Errorf("%q in %d: %q, want %q", c.en, i+1, got, want)
			}
		}
	}
	if got := understoodIn("Bakery · open on Sunday, inside the boundary of Zürich", 1); got != "Bäckerei · am Sonntag offen, in Zürich" {
		t.Errorf("understood: %q", got)
	}
	body := translate([]byte(`{"understood":"Bakery · open on Sunday, inside the boundary of Zürich","open":{"day":0,"minute":-1,"label":"open on Sunday","country":"ch"}}`), "search", "de")
	if !strings.Contains(string(body), `"label":"am Sonntag offen"`) || !strings.Contains(string(body), `"country":"ch"`) {
		t.Errorf("translate: %s", body)
	}
}
