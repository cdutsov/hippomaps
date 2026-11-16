package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

// overpass-search names what it found in English: the reading of a search ("Restaurant, in the
// view"), the other readings (preset names), its notes, and the kinds of places. It finds places
// in the language of the reader (lang=), but writes its own words in English. The relay puts these words
// in the language of the page (de, fr, it): the preset names from the iD presets (labels/presets.json,
// labels/gen.py), the phrases of the search below. What is not known here stays in English.

//go:embed labels/presets.json
var presetsJSON []byte

// langs are the languages of the page besides English, in the order of labels/presets.json
var langs = map[string]int{"de": 1, "fr": 2, "it": 3}

type presetNames struct {
	byID map[string][4]string // "amenity/cafe": English, German, French, Italian ("" where English)
	byEN map[string][4]string // the same by the English name, in lower case
}

var presets = func() presetNames {
	var raw map[string][4]string
	if err := json.Unmarshal(presetsJSON, &raw); err != nil {
		panic(err)
	}
	p := presetNames{byID: raw, byEN: map[string][4]string{}}
	for id, n := range raw {
		k := strings.ToLower(n[0])
		// the same English name for several presets ("Travel Agency"): the shortest id, the most general
		if old, ok := p.byEN[k]; !ok || len(id) < len(old[0]) {
			p.byEN[k] = [4]string{id, n[1], n[2], n[3]}
		}
	}
	return p
}()

// ours are the entries overpass-search adds to the presets (its internal/vocab/own.go), by their title
var ours = map[string][3]string{
	"Restaurants and take-aways": {"Restaurants und Take-aways", "Restaurants et vente à emporter", "Ristoranti e take-away"},
	"Bars and pubs":              {"Bars und Pubs", "Bars et pubs", "Bar e pub"},
	"Cafés":                      {"Cafés", "Cafés", "Caffè"},
	"Places to swim":             {"Badeplätze", "Lieux de baignade", "Luoghi per nuotare"},
	"Places to stay":             {"Unterkünfte", "Hébergements", "Alloggi"},
	"Camp sites":                 {"Campingplätze", "Campings", "Campeggi"},
	"Car parks":                  {"Parkplätze", "Parkings", "Parcheggi"},
	"Bike parking":               {"Veloparkplätze", "Parkings à vélos", "Parcheggi per bici"},
	"Charging stations":          {"Ladestationen", "Bornes de recharge", "Stazioni di ricarica"},
	"Food shops":                 {"Lebensmittelgeschäfte", "Magasins d’alimentation", "Negozi di alimentari"},
	"Bakeries":                   {"Bäckereien", "Boulangeries", "Panetterie"},
	"Cash machines":              {"Bancomaten", "Bancomats", "Bancomat"},
	"Fuel stations":              {"Tankstellen", "Stations-service", "Distributori di carburante"},
	"Toilets":                    {"Toiletten", "Toilettes", "Bagni"},
	"Places to work":             {"Orte zum Arbeiten", "Lieux pour travailler", "Luoghi per lavorare"},
	"Libraries and bookcases":    {"Bibliotheken und Bücherschränke", "Bibliothèques et boîtes à livres", "Biblioteche e librerie di strada"},
	"Playgrounds":                {"Spielplätze", "Places de jeux", "Parchi giochi"},
	"Gyms and sports centres":    {"Fitnesscenter und Sportzentren", "Salles de sport et centres sportifs", "Palestre e centri sportivi"},
	"Picnic places":              {"Picknickplätze", "Aires de pique-nique", "Aree picnic"},
	"Barbecue places":            {"Grillstellen", "Places de grillade", "Aree barbecue"},
	"Benches and picnic tables":  {"Bänke und Picknicktische", "Bancs et tables de pique-nique", "Panchine e tavoli da picnic"},
	"Viewpoints":                 {"Aussichtspunkte", "Points de vue", "Punti panoramici"},
	"Drinking water":             {"Trinkwasser", "Eau potable", "Acqua potabile"},
	"Places of worship":          {"Gotteshäuser", "Lieux de culte", "Luoghi di culto"},
	"Emergency departments":      {"Notfallstationen", "Urgences", "Pronto soccorso"},
	"Pharmacies":                 {"Apotheken", "Pharmacies", "Farmacie"},
	"Cycle routes":               {"Velorouten", "Itinéraires cyclables", "Percorsi ciclabili"},
	"Mountain bike routes":       {"Mountainbike-Routen", "Itinéraires VTT", "Percorsi mountain bike"},
	"Aerial lifts":               {"Seilbahnen und Lifte", "Remontées mécaniques", "Impianti a fune"},
	"Tram stops":                 {"Tramhaltestellen", "Arrêts de tram", "Fermate del tram"},
	"Public transport stops":     {"Haltestellen", "Arrêts de transports publics", "Fermate dei trasporti pubblici"},
	"Bridges":                    {"Brücken", "Ponts", "Ponti"},
	"Mountain huts":              {"Berghütten", "Cabanes", "Rifugi"},
	"Glass recycling":            {"Altglas-Sammelstellen", "Collecte du verre", "Raccolta del vetro"},
	"Dog parks":                  {"Hundeauslaufplätze", "Parcs à chiens", "Aree cani"},
	"Sights":                     {"Sehenswürdigkeiten", "Curiosités", "Attrazioni"},
	"Forests":                    {"Wälder", "Forêts", "Boschi"},
	"Coastline":                  {"Küste", "Côte", "Costa"},
	"Water":                      {"Gewässer", "Plans d’eau", "Acque"},
}

// words of a reading: the properties searched for (the modifiers of overpass-search), "open now"
var words = map[string][3]string{
	"vegan": {"vegan", "végan", "vegano"}, "vegetarian": {"vegetarisch", "végétarien", "vegetariano"},
	"gluten-free": {"glutenfrei", "sans gluten", "senza glutine"}, "lactose-free": {"laktosefrei", "sans lactose", "senza lattosio"},
	"organic": {"bio", "bio", "biologico"}, "wheelchair accessible": {"rollstuhlgängig", "accessible en fauteuil roulant", "accessibile in sedia a rotelle"},
	"with outdoor seating": {"mit Terrasse", "avec terrasse", "con posti all’aperto"}, "with wifi": {"mit WLAN", "avec wifi", "con wifi"},
	"open 24/7": {"rund um die Uhr offen", "ouvert 24 h/24", "aperto 24 ore su 24"}, "dogs allowed": {"Hunde erlaubt", "chiens admis", "cani ammessi"},
	"free of charge": {"gratis", "gratuit", "gratuito"}, "covered": {"überdacht", "couvert", "coperto"}, "lit": {"beleuchtet", "éclairé", "illuminato"},
	"with backrest": {"mit Lehne", "avec dossier", "con schienale"}, "self-service": {"Selbstbedienung", "libre-service", "self-service"},
	"takeaway": {"zum Mitnehmen", "à emporter", "da asporto"}, "delivery": {"Lieferdienst", "livraison", "consegna a domicilio"},
	"non-smoking": {"rauchfrei", "non-fumeur", "non fumatori"}, "smoking allowed": {"Rauchen erlaubt", "fumeurs", "fumatori"},
	"air-conditioned": {"klimatisiert", "climatisé", "climatizzato"}, "accepts credit cards": {"Kreditkarten", "cartes de crédit", "carte di credito"},
	"contactless payment": {"kontaktlos zahlen", "paiement sans contact", "pagamento contactless"}, "with toilets": {"mit Toiletten", "avec toilettes", "con bagni"},
	"with showers": {"mit Duschen", "avec douches", "con docce"}, "with a pool": {"mit Pool", "avec piscine", "con piscina"},
	"with drinking water": {"mit Trinkwasser", "avec eau potable", "con acqua potabile"}, "with changing table": {"mit Wickeltisch", "avec table à langer", "con fasciatoio"},
	"public": {"öffentlich", "public", "pubblico"}, "indoor": {"drinnen", "couvert", "al coperto"}, "outdoor": {"draussen", "en plein air", "all’aperto"},
	"unpaved": {"unbefestigt", "non goudronné", "sterrato"}, "glass": {"Glas", "verre", "vetro"}, "paper": {"Papier", "papier", "carta"},
	"cardboard": {"Karton", "carton", "cartone"}, "clothes": {"Kleider", "vêtements", "vestiti"}, "batteries": {"Batterien", "piles", "batterie"},
	"plastic": {"Plastik", "plastique", "plastica"}, "cans": {"Dosen", "canettes", "lattine"}, "electronics": {"Elektrogeräte", "électronique", "elettronica"},
	"green waste": {"Grüngut", "déchets verts", "rifiuti verdi"},
	"open now":    {"jetzt offen", "ouvert maintenant", "aperto ora"}, "anything": {"alles", "tout", "tutto"},
	// the keys of a condition: "elevation over 4'000 m"
	"elevation": {"Höhe", "altitude", "altitudine"}, "height": {"Höhe", "hauteur", "altezza"}, "floors": {"Stockwerke", "étages", "piani"},
	"population": {"Einwohner", "habitants", "abitanti"}, "capacity": {"Plätze", "capacité", "capienza"}, "beds": {"Betten", "lits", "letti"},
	"rooms": {"Zimmer", "chambres", "camere"}, "speed limit": {"Höchstgeschwindigkeit", "vitesse maximale", "limite di velocità"},
	"built": {"gebaut", "construit", "costruito"}, "area": {"Fläche", "surface", "superficie"},
	"over": {"über", "plus de", "oltre"}, "under": {"unter", "moins de", "sotto"}, "at least": {"mindestens", "au moins", "almeno"},
	"at most": {"höchstens", "au plus", "al massimo"}, "after": {"nach", "après", "dopo"}, "before": {"vor", "avant", "prima del"},
	"in or after": {"ab", "à partir de", "dal"}, "in or before": {"bis", "jusqu’en", "fino al"},
}

// kinds of places in the details of the geocoder ("Bern (Schweiz · city)") that are not preset names
var kinds = map[string][3]string{
	"street": {"Strasse", "rue", "strada"}, "address": {"Adresse", "adresse", "indirizzo"}, "area": {"Gebiet", "région", "area"},
	"city": {"Stadt", "ville", "città"}, "town": {"Stadt", "ville", "città"}, "village": {"Dorf", "village", "paese"},
	"hamlet": {"Weiler", "hameau", "frazione"}, "suburb": {"Stadtteil", "quartier", "quartiere"}, "quarter": {"Quartier", "quartier", "quartiere"},
	"neighbourhood": {"Quartier", "quartier", "quartiere"}, "locality": {"Flurname", "lieu-dit", "località"}, "farm": {"Hof", "ferme", "fattoria"},
	"isolated dwelling": {"Einzelhof", "habitation isolée", "casa isolata"}, "island": {"Insel", "île", "isola"}, "islet": {"Inselchen", "îlot", "isolotto"},
	"state": {"Kanton", "canton", "cantone"}, "country": {"Land", "pays", "paese"}, "county": {"Bezirk", "district", "distretto"},
	"region": {"Region", "région", "regione"}, "municipality": {"Gemeinde", "commune", "comune"}, "postcode": {"Postleitzahl", "code postal", "CAP"},
}

// tr gives the word in language i (1 de, 2 fr, 3 it) from a table, or the word itself
func tr(m map[string][3]string, s string, i int) string {
	if v, ok := m[s]; ok && v[i-1] != "" {
		return v[i-1]
	}
	return s
}

// presetName is the name of a preset in language i, by its id ("amenity/cafe"), or "" when not known
func presetName(id string, i int) string {
	n, ok := presets.byID[id]
	if !ok {
		return ""
	}
	if n[i] != "" {
		return n[i]
	}
	return n[0]
}

// nameOf is a name of a reading (a preset by its English name, or an entry of ours) in language i
func nameOf(s string, i int) string {
	if v, ok := ours[s]; ok {
		return v[i-1]
	}
	if n, ok := presets.byEN[strings.ToLower(s)]; ok && n[i] != "" {
		return n[i]
	}
	return s
}

// kindOf is the kind of a place ("city", "bus stop") in language i
func kindOf(s string, i int) string {
	if v, ok := kinds[s]; ok {
		return v[i-1]
	}
	v := strings.ReplaceAll(s, " ", "_")
	for _, k := range []string{"place", "amenity", "shop", "tourism", "leisure", "natural", "highway", "railway", "historic", "building", "landuse", "man_made", "waterway", "aeroway", "office", "craft"} {
		if n := presetName(k+"/"+v, i); n != "" {
			return n
		}
	}
	return s
}

// phrase is a pattern of overpass-search in English and its forms in de, fr, it ($1, $2 … as in regexp)
type phrase struct {
	re *regexp.Regexp
	to [3]string
}

func phrases(p ...[4]string) []phrase {
	out := make([]phrase, len(p))
	for i, x := range p {
		out[i] = phrase{regexp.MustCompile("^" + x[0] + "$"), [3]string{x[1], x[2], x[3]}}
	}
	return out
}

const dist = `(\d[\d'.]* k?m)` // "150 m", "1.5 km"

// where: where a search looked (WhereFor of overpass-search). The places are names in the language of
// the reader already; "the A3" is a road.
var where = phrases(
	[4]string{`in the view`, "im Kartenausschnitt", "dans la vue de la carte", "nella vista della mappa"},
	[4]string{`everywhere`, "überall", "partout", "ovunque"},
	[4]string{`in (\d+) × (\d+) km around the view`, "in $1 × $2 km um den Kartenausschnitt", "dans $1 × $2 km autour de la vue de la carte", "in $1 × $2 km intorno alla vista della mappa"},
	[4]string{`within ` + dist + ` of your location`, "im Umkreis von $1 um Ihren Standort", "à moins de $1 de votre position", "entro $1 dalla tua posizione"},
	[4]string{`within ` + dist + ` of (.+)`, "im Umkreis von $1 um $2", "à moins de $1 de $2", "entro $1 da $2"},
	[4]string{`more than ` + dist + ` from (.+)`, "mehr als $1 von $2 entfernt", "à plus de $1 de $2", "a più di $1 da $2"},
	[4]string{`inside the boundary of (.+)`, "in $1", "dans les limites de $1", "entro i confini di $1"},
	[4]string{`in the box around (.+)`, "im Rechteck um $1", "dans le rectangle autour de $1", "nel rettangolo intorno a $1"},
	[4]string{`between (.+) and (.+)`, "zwischen $1 und $2", "entre $1 et $2", "tra $1 e $2"},
	[4]string{`along (.+)`, "entlang $1", "le long de $1", "lungo $1"},
	[4]string{`by (.+)`, "an $1", "au bord de $1", "presso $1"},
	[4]string{`over (.+)`, "über $1", "sur $1", "sopra $1"},
	[4]string{`next to (.+)`, "neben $1", "à côté de $1", "accanto a $1"},
	[4]string{`near (.+)`, "nahe $1", "près de $1", "vicino a $1"},
	[4]string{`inside (.+)`, "in $1", "dans $1", "dentro $1"},
	[4]string{`in (.+)`, "in $1", "dans $1", "in $1"},
)

// the tail of a place near a line or a place: " (within 150 m)", " in Zürich", " in the view"
var (
	withinRe = regexp.MustCompile(` \(within ` + dist + `\)`)
	withinTo = [3]string{" (bis $1)", " (à moins de $1)", " (entro $1)"}
	inRe     = regexp.MustCompile(`\) in (.+)$`) // the region after it: "by Limmat (within 150 m) in Zürich"
	inTo     = [3]string{") in $1", ") à $1", ") a $1"}
	theRe    = regexp.MustCompile(`(^| )the ([A-Z]+ ?\d+\b)`) // "the A3"
	theTo    = [3]string{"${1}der $2", "${1}l’$2", "${1}l’$2"}
	chainRe  = regexp.MustCompile(`a place of the chain “([^”]+)”`)
	chainTo  = [3]string{"einem Ort der Kette “$1”", "un lieu de la chaîne “$1”", "un luogo della catena “$1”"}
)

// whereIn puts where a search looked in language i; "" when it is not a phrase known here. A text is read in
// parts, each in several ways (cut at a ", ", without " in the view" at its end); seen keeps the answer of each
// part, so that each is read once: a search names what the visitor wrote ("named “a, a in the view in the
// view”"), and without it the ways to read such a text double with each part.
func whereIn(s string, i int, seen map[string]string) string {
	if w, ok := seen[s]; ok {
		return w
	}
	w := wherePart(s, i, seen)
	seen[s] = w
	return w
}

func wherePart(s string, i int, seen map[string]string) string {
	s = withinRe.ReplaceAllString(s, withinTo[i-1])
	s = inRe.ReplaceAllString(s, inTo[i-1])
	s = theRe.ReplaceAllString(s, theTo[i-1])
	s = chainRe.ReplaceAllString(s, chainTo[i-1])
	// a kind of place before the place: "within 500 m of the old town, inside the boundary of Bern"
	if a, b, ok := strings.Cut(s, ", "); ok {
		if wa, wb := whereIn(a, i, seen), whereIn(b, i, seen); wa != "" && wb != "" {
			return wa + ", " + wb
		}
	}
	if t, ok := strings.CutSuffix(s, " in the view"); ok {
		if w := whereIn(t, i, seen); w != "" {
			return w + " " + where[0].to[i-1]
		}
	}
	for _, p := range where {
		if p.re.MatchString(s) {
			out := p.re.ReplaceAllString(s, p.to[i-1])
			if i == 1 {
				out = strings.ReplaceAll(out, " and ", " und ") // several places: "in Bern and Thun"
			} else {
				out = strings.ReplaceAll(out, " and ", map[int]string{2: " et ", 3: " e "}[i])
			}
			return out
		}
	}
	return ""
}

var (
	condRe   = regexp.MustCompile(`^(elevation|height|floors|population|capacity|beds|rooms|speed limit|built|area) (over|under|at least|at most|after|before|in or after|in or before) (.+)$`)
	starsRe  = regexp.MustCompile(`^(\d+) stars$`)
	quoteRe  = regexp.MustCompile(`^(chain|brand or name) “(.+)”$`)
	quoteTo  = map[string][3]string{"chain": {"Kette", "chaîne", "catena"}, "brand or name": {"Marke oder Name", "marque ou nom", "marchio o nome"}}
	ofChain  = [3]string{" der Kette “", " de la chaîne “", " della catena “"}
	named    = [3]string{" namens “", " nommé “", " chiamato “"}
	withWord = [3]string{" mit ", " avec ", " con "}
	starsTo  = [3]string{"$1 Sterne", "$1 étoiles", "$1 stelle"}
	// a line by its number: "Bus line 31"
	lineRe = regexp.MustCompile(`^(Bus|Trolleybus|Tram|Metro|Ferry|Train|Minibus|Transit) line (\S+)$`)
	lineTo = map[string][3]string{"Bus": {"Buslinie", "Ligne de bus", "Linea bus"}, "Trolleybus": {"Trolleybuslinie", "Ligne de trolleybus", "Linea filobus"},
		"Tram": {"Tramlinie", "Ligne de tram", "Linea tram"}, "Metro": {"Metrolinie", "Ligne de métro", "Linea metropolitana"},
		"Ferry": {"Schiffslinie", "Ligne de bateau", "Linea battello"}, "Train": {"Linie", "Ligne", "Linea"}, "Transit": {"Linie", "Ligne", "Linea"},
		"Minibus": {"Minibuslinie", "Ligne de minibus", "Linea minibus"}}
)

// the days of opening hours of overpass-search ("open on Sunday"), in German, French and Italian; the Italian with its article
var days = map[string][3]string{
	"Monday": {"Montag", "lundi", "il lunedì"}, "Tuesday": {"Dienstag", "mardi", "il martedì"}, "Wednesday": {"Mittwoch", "mercredi", "il mercoledì"},
	"Thursday": {"Donnerstag", "jeudi", "il giovedì"}, "Friday": {"Freitag", "vendredi", "il venerdì"}, "Saturday": {"Samstag", "samedi", "il sabato"},
	"Sunday": {"Sonntag", "dimanche", "la domenica"},
}

var (
	openTodayAtRe = regexp.MustCompile(`^open at (\d\d:\d\d) today$`)
	openDayAtRe   = regexp.MustCompile(`^open on (Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)(?: at (\d\d:\d\d))?$`)
)

// openIn puts a time a search asks places to be open at (the label of open in an answer of overpass-search: "open now",
// "open today", "open at 23:00 today", "open on Saturday and Sunday", "open on Sunday", "open on Sunday at 10:00") in
// language i; another text stays as it is
func openIn(s string, i int) string {
	switch s {
	case "open now":
		return tr(words, s, i)
	case "open today":
		return [3]string{"heute offen", "ouvert aujourd’hui", "aperto oggi"}[i-1]
	case "open on Saturday and Sunday":
		return [3]string{"am Samstag und Sonntag offen", "ouvert samedi et dimanche", "aperto sabato e domenica"}[i-1]
	}
	if m := openTodayAtRe.FindStringSubmatch(s); m != nil {
		return [3]string{"heute um " + m[1] + " offen", "ouvert aujourd’hui à " + m[1], "aperto oggi alle " + m[1]}[i-1]
	}
	if m := openDayAtRe.FindStringSubmatch(s); m != nil {
		d := days[m[1]][i-1]
		if m[2] == "" {
			return [3]string{"am " + d + " offen", "ouvert le " + d, "aperto " + d}[i-1]
		}
		it, _ := strings.CutPrefix(strings.TrimPrefix(d, "il "), "la ") // "aperto domenica alle 10:00"
		return [3]string{"am " + d + " um " + m[2] + " offen", "ouvert le " + d + " à " + m[2], "aperto " + it + " alle " + m[2]}[i-1]
	}
	return s
}

// readingIn puts the reading of a search ("vegan: Restaurant · open now") in language i
func readingIn(s string, i int) string {
	parts := strings.Split(s, " · ")
	for k, c := range parts[1:] { // the conditions
		if m := condRe.FindStringSubmatch(c); m != nil {
			parts[k+1] = tr(words, m[1], i) + " " + tr(words, m[2], i) + " " + m[3]
		} else if starsRe.MatchString(c) {
			parts[k+1] = starsRe.ReplaceAllString(c, starsTo[i-1])
		} else if o := openIn(c, i); o != c {
			parts[k+1] = o
		} else {
			parts[k+1] = tr(words, c, i)
		}
	}
	front, what, ok := strings.Cut(parts[0], ": ")
	if !ok {
		front, what = "", parts[0]
	}
	if front != "" {
		fs := strings.Split(front, ", ")
		for k, f := range fs {
			fs[k] = tr(words, f, i)
		}
		front = strings.Join(fs, ", ") + ": "
	}
	if m := quoteRe.FindStringSubmatch(what); m != nil {
		what = quoteTo[m[1]][i-1] + " “" + m[2] + "”"
	} else if m := lineRe.FindStringSubmatch(what); m != nil {
		what = lineTo[m[1]][i-1] + " " + m[2]
	} else {
		tail := ""
		for _, t := range []struct {
			en string
			to [3]string
		}{{" of the chain “", ofChain}, {" named “", named}} {
			if a, b, ok := strings.Cut(what, t.en); ok {
				what, tail = a, t.to[i-1]+b
				break
			}
		}
		if a, b, ok := strings.Cut(what, " with "); ok {
			what, tail = a, withWord[i-1]+b+tail
		}
		ns := strings.Split(what, ", ")
		for k, n := range ns {
			ns[k] = tr(words, nameOf(n, i), i)
		}
		what = strings.Join(ns, ", ") + tail
	}
	parts[0] = front + what
	return strings.Join(parts, " · ")
}

var placeRe = regexp.MustCompile(`^(.+) \((.+)\)$`) // "Bern (Schweiz · city)"

// maxUnderstood is the longest reading put in another language (bytes): those of overpass-search have some
// dozens of characters, and a longer one carries a long text of the visitor.
const maxUnderstood = 600

// understoodIn puts the search as read in language i: a reading and where it looked, or a place found
func understoodIn(s string, i int) string {
	if len(s) > maxUnderstood {
		return s
	}
	seen := map[string]string{}
	// the reading ends at the first ", " that begins a known where ("Cafe, Restaurant, in the view")
	for at := 0; ; {
		k := strings.Index(s[at:], ", ")
		if k < 0 {
			break
		}
		at += k + 2
		if w := whereIn(s[at:], i, seen); w != "" {
			return readingIn(s[:at-2], i) + ", " + w
		}
	}
	if m := placeRe.FindStringSubmatch(s); m != nil {
		return m[1] + " (" + detailIn(m[2], i) + ")"
	}
	return s
}

// detailIn puts the kind at the end of the detail of a place in language i: "Bern, Schweiz · city"
func detailIn(s string, i int) string {
	k := strings.LastIndex(s, " · ")
	if k < 0 {
		if !strings.Contains(s, ",") {
			return kindOf(s, i) // the kind alone
		}
		return s
	}
	return s[:k+len(" · ")] + kindOf(s[k+len(" · "):], i)
}

// notes of overpass-search
var notes = phrases(
	[4]string{`The query finds all places with opening hours\. It does not check the hours\.`,
		"Die Abfrage findet alle Orte mit Öffnungszeiten. Sie prüft die Zeiten nicht.",
		"La requête trouve tous les lieux avec des horaires. Elle ne vérifie pas les horaires.",
		"La query trova tutti i luoghi con orari di apertura. Non controlla gli orari."},
	[4]string{`The opening hours were not checked: these are all places with opening hours\.`,
		"Die Öffnungszeiten wurden nicht geprüft: das sind alle Orte mit Öffnungszeiten.",
		"Les horaires n’ont pas été vérifiés : ce sont tous les lieux avec des horaires.",
		"Gli orari non sono stati controllati: sono tutti i luoghi con orari di apertura."},
	[4]string{`The query does not sort by distance\.`, "Die Abfrage sortiert nicht nach Entfernung.", "La requête ne trie pas par distance.", "La query non ordina per distanza."},
	[4]string{`Your location was not shared: the search uses the view\.`,
		"Ihr Standort wurde nicht geteilt: die Suche nimmt den Kartenausschnitt.",
		"Votre position n’a pas été partagée : la recherche utilise la vue de la carte.",
		"La tua posizione non è stata condivisa: la ricerca usa la vista della mappa."},
	[4]string{`Nothing found\. OpenStreetMap may use other tags for this here; the other readings may find it\.`,
		"Nichts gefunden. OpenStreetMap hat dafür hier vielleicht andere Tags; die anderen Lesarten finden es vielleicht.",
		"Rien trouvé. OpenStreetMap utilise peut-être d’autres tags ici ; les autres lectures le trouvent peut-être.",
		"Niente trovato. Qui OpenStreetMap usa forse altri tag; le altre letture potrebbero trovarlo."},
	[4]string{`([\d']+) results; this answer holds ([\d']+) of them\. The query does not have a limit\.`,
		"$1 Ergebnisse; diese Antwort enthält $2 davon. Die Abfrage hat keine Grenze.",
		"$1 résultats ; cette réponse en contient $2. La requête n’a pas de limite.",
		"$1 risultati; questa risposta ne contiene $2. La query non ha limiti."},
	[4]string{`Only the first ([\d']+) of ([\d']+) were tested\.`,
		"Nur die ersten $1 von $2 wurden geprüft.", "Seuls les $1 premiers sur $2 ont été vérifiés.", "Solo i primi $1 su $2 sono stati controllati."},
	[4]string{`OpenStreetMap has nothing for “(.+)”: the search leaves it out\.`,
		"OpenStreetMap hat nichts für “$1”: die Suche lässt es weg.",
		"OpenStreetMap n’a rien pour “$1” : la recherche l’ignore.",
		"OpenStreetMap non ha nulla per “$1”: la ricerca lo ignora."},
	[4]string{`Not used: “(.+)”\.`, "Nicht verwendet: “$1”.", "Pas utilisé : “$1”.", "Non usato: “$1”."},
	[4]string{`Nothing named “(.+)” was found: the search leaves out the name\.`,
		"Nichts mit dem Namen “$1” gefunden: die Suche lässt den Namen weg.",
		"Rien de nommé “$1” : la recherche ignore le nom.",
		"Niente con il nome “$1”: la ricerca ignora il nome."},
	[4]string{`No line is named (\S+) exactly: the search shows those with letters beside the number \(28E, M3\)\.`,
		"Keine Linie heisst genau $1: die Suche zeigt jene mit Buchstaben neben der Nummer (28E, M3).",
		"Aucune ligne ne s’appelle exactement $1 : la recherche montre celles avec des lettres à côté du numéro (28E, M3).",
		"Nessuna linea si chiama esattamente $1: la ricerca mostra quelle con lettere accanto al numero (28E, M3)."},
)

func noteIn(s string, i int) string {
	for _, p := range notes {
		if p.re.MatchString(s) {
			return p.re.ReplaceAllString(s, p.to[i-1])
		}
	}
	return s
}

// tries are the searches offered for a place found ("cafés in Bern"): the label and the search in
// language i, words overpass-search reads in each of them (each tried: "Bänke" is a bank there)
var tries = map[string][3]string{
	"national parks": {"Nationalparks", "parcs nationaux", "parchi nazionali"}, "castles": {"Burgen", "châteaux", "castelli"},
	"lighthouses": {"Leuchttürme", "phares", "fari"}, "cafés": {"Cafés", "cafés", "caffè"}, "playgrounds": {"Spielplätze", "aires de jeux", "parchi giochi"},
	"museums": {"Museen", "musées", "musei"}, "drinking water": {"Trinkwasser", "eau potable", "acqua potabile"},
	"bus stops": {"Bushaltestellen", "arrêts de bus", "fermate dell’autobus"}, "benches": {"Sitzbänke", "bancs", "panchine"},
}
var tryRe = regexp.MustCompile(`^(.+?) (in|near) (.+)$`)

func tryIn(s string, i int) string {
	m := tryRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	what, ok := tries[m[1]]
	if !ok {
		return s
	}
	prep := map[string][3]string{"in": {"in", "à", "a"}, "near": {"bei", "près de", "vicino a"}}[m[2]][i-1]
	return what[i-1] + " " + prep + " " + m[3]
}

// kindsIn gives each place found the name of its kind from its main tag ("amenity=fast_food" is
// "Schnellimbiss"); the page shows it in place of the tag
func kindsIn(d map[string]any, i int) {
	r, _ := d["results"].(map[string]any)
	fs, _ := r["features"].([]any)
	for _, f := range fs {
		fm, _ := f.(map[string]any)
		p, _ := fm["properties"].(map[string]any)
		if t, _ := p["type"].(string); t != "" {
			if n := presetName(strings.Replace(t, "=", "/", 1), i); n != "" {
				p["kind"] = n
			}
		}
	}
}

// translate puts the English words of an answer of overpass-search (the path after /api/search/) in
// the language lang; other answers and languages stay as they came
func translate(body []byte, path, lang string) []byte {
	i := langs[lang]
	if i == 0 {
		return body
	}
	var d map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if dec.Decode(&d) != nil {
		return body
	}
	str := func(m map[string]any, k string, f func(string, int) string) {
		if s, ok := m[k].(string); ok && s != "" {
			m[k] = f(s, i)
		}
	}
	each := func(k string, f func(map[string]any)) {
		list, _ := d[k].([]any)
		for _, x := range list {
			if m, ok := x.(map[string]any); ok {
				f(m)
			}
		}
	}
	switch path {
	case "query", "search":
		str(d, "understood", understoodIn)
		each("alternatives", func(a map[string]any) {
			if id, _ := a["what"].(string); presetName(id, i) != "" {
				a["label"] = presetName(id, i)
			} else {
				str(a, "label", nameOf)
			}
		})
		if ns, ok := d["notes"].([]any); ok {
			for k, n := range ns {
				if s, ok := n.(string); ok {
					ns[k] = noteIn(s, i)
				}
			}
		}
		each("try", func(t map[string]any) { str(t, "label", tryIn); str(t, "q", tryIn) })
		each("places", func(p map[string]any) { str(p, "detail", detailIn) })
		if o, ok := d["open"].(map[string]any); ok { // the count of the places open then: "80 am Sonntag offen, von 118 …"
			str(o, "label", openIn)
		}
		kindsIn(d, i)
	case "here": // a tap on the map: the address, the area, the places around
		for _, k := range []string{"place", "area"} {
			if p, ok := d[k].(map[string]any); ok {
				str(p, "detail", detailIn)
			}
		}
		kindsIn(d, i)
	case "complete":
		each("suggestions", func(s map[string]any) {
			d, _ := s["detail"].(string)
			if s["kind"] == "what" { // "Chinese Restaurant · amenity=restaurant + cuisine=chinese", or the tags alone
				if name, tags, ok := strings.Cut(d, " · "); ok {
					s["detail"] = nameOf(name, i) + " · " + tags
				}
			} else if d != "" {
				s["detail"] = detailIn(d, i)
			}
		})
	default:
		return body
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if enc.Encode(d) != nil {
		return body
	}
	return out.Bytes()
}
