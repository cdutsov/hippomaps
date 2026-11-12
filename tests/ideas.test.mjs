// The searches at the start (ideasFor in web/app.js, T.idea in each language): what the map view shows turned into
// sentences the search reads, with the article of a river in German, French and Italian.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/ideas.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { texts, riverName } from "../web/i18n.js";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const cut = (from, to) => {
  const a = src.indexOf(from), b = src.indexOf(to, a);
  assert.ok(a > 0 && b > a, from);
  return src.slice(a, b);
};
const EN = new Function("num", "dur", "riverName", `${cut("const EN = {", "\n};\n")}\n}; return EN;`)(String, (s) => `${s} s`, riverName);
const ideasFor = new Function(`${cut("function ideasFor(", "\n}\n")}\n}\nreturn ideasFor;`)();
const all = { en: EN, ...texts(String) };
const first = (options) => options[0]; // the first kind of place each time

test("a river and a lake: the longer search along the river near the lake, the river, its bridges, the lake, the town", () => {
  const d = ideasFor({ river: "Limmat", riverLake: true, lake: true, town: "Zurich", city: true, sight: "Zürich Hauptbahnhof" }, EN.idea, 12, first);
  assert.equal(d.short, "open restaurants");
  assert.equal(d.long, "cafés along the Limmat near the lake");
  assert.deepEqual(d.list, ["cafés along the Limmat", "bridges over the Limmat", "picnic spots near the lake", "cafés near Zürich Hauptbahnhof"]);
});
test("open now after the hour: bakeries in the morning, bars at night", () => {
  assert.equal(ideasFor({}, EN.idea, 7, first).short, "open bakeries");
  assert.equal(ideasFor({}, EN.idea, 23, first).short, "open bars");
});
test("a town alone: a kind of place in it; a view without names: kinds of places of the view; nothing known: the example", () => {
  assert.deepEqual(ideasFor({ town: "Chur" }, EN.idea, 12, first).list, ["playgrounds in Chur", "viewpoints", "picnic spots"]);
  assert.equal(ideasFor({ town: "Bern", city: true }, EN.idea, 12, first).long, "vegan restaurants in Bern");
  const none = ideasFor({}, EN.idea, 12, first);
  assert.equal(none.long, EN.idea.fallback);
  assert.deepEqual(none.list, ["viewpoints", "picnic spots", "drinking water"]);
});
test("the map out too far for a search of its view: only searches with a named place", () => {
  const d = ideasFor({ far: true, river: "Aare", lake: true, sea: true, huts: 2000, town: "Bern", city: true }, EN.idea, 12, first);
  assert.equal(d.short, "");
  assert.ok(d.list.every((s) => /Aare|Bern/.test(s)), d.list.join(" | "));
});
test("the sea, a beach, high mountains", () => {
  assert.ok(ideasFor({ sea: true }, EN.idea, 12, first).list.includes("restaurants by the sea"));
  assert.ok(ideasFor({ beach: true }, EN.idea, 12, first).list.includes("bars near the beach"));
  assert.ok(ideasFor({ huts: 2000 }, EN.idea, 12, first).list.includes("mountain huts over 2000 m"));
});
test("no sentence twice, at most four in the list", () => {
  for (const lang of ["en", "de", "fr", "it"]) {
    const d = ideasFor({ river: "Aare", riverLake: true, lake: true, sea: true, huts: 2000, town: "Thun", city: false, sight: "Schloss Thun" }, all[lang].idea, 12, first);
    assert.ok(d.list.length <= 4, lang);
    assert.equal(new Set([d.short, d.long, ...d.list]).size, d.list.length + 2, `${lang}: ${[d.short, d.long, ...d.list].join(" | ")}`);
  }
});
test("each language: every kind of place its own word, the sentences without English", () => {
  for (const lang of ["de", "fr", "it"]) {
    const I = all[lang].idea;
    assert.ok(I && I.fallback !== EN.idea.fallback, lang);
    for (const [k, w] of Object.entries(I.cats)) assert.ok(w && k in EN.idea.cats, `${lang} ${k}`);
    const d = ideasFor({ river: "Limmat", riverLake: true, lake: true, town: "Zürich", city: true }, I, 12, first);
    for (const s of [d.short, d.long, ...d.list]) assert.ok(!/\b(along|near|over|open|the)\b/.test(s), `${lang}: ${s}`);
  }
});
test("the name of a river without its article, one of several names, nothing in brackets", () => {
  assert.deepEqual(riverName("La Seine"), { bare: "Seine", art: "la" });
  assert.deepEqual(riverName("L'Arve"), { bare: "Arve", art: "l'" });
  assert.deepEqual(riverName("el Llobregat"), { bare: "Llobregat", art: "el" });
  assert.deepEqual(riverName("La Thièle / Zihl"), { bare: "Thièle", art: "la" });
  assert.deepEqual(riverName("La Drize (conduite provisoire avant renaturation)"), { bare: "Drize", art: "la" });
  assert.deepEqual(riverName("River Thames"), { bare: "River Thames", art: "" });
  assert.equal(EN.idea.along("pubs", "River Thames"), "pubs along the River Thames");
  assert.equal(EN.idea.bridges("La Seine"), "bridges over the Seine");
});
test("the article of a river: German an der / am, French de la / du / de l', Italian la / il / l'", () => {
  const de = all.de.idea, fr = all.fr.idea, it = all.it.idea;
  assert.equal(de.along("Cafés", "Limmat"), "Cafés an der Limmat");
  assert.equal(de.along("Cafés", "Rhein"), "Cafés am Rhein");
  assert.equal(de.along("Cafés", "Aabach"), "Cafés am Aabach");
  assert.equal(de.bridges("Main"), "Brücken über den Main");
  assert.equal(de.bridges("Aare"), "Brücken über die Aare");
  assert.equal(fr.along("cafés", "La Seine"), "cafés le long de la Seine");
  assert.equal(fr.along("cafés", "Le Rhône"), "cafés le long du Rhône");
  assert.equal(fr.along("cafés", "L'Arve"), "cafés le long de l'Arve");
  assert.equal(fr.along("cafés", "Limmat"), "cafés le long de la Limmat");
  assert.equal(fr.along("cafés", "Rhin"), "cafés le long du Rhin");
  assert.equal(fr.along("cafés", "Aare"), "cafés le long de l'Aare");
  assert.equal(fr.bridges("Le Rhône"), "ponts sur le Rhône");
  assert.equal(fr.open("boulangeries", "bakery"), "boulangeries ouvertes");
  assert.equal(fr.open("bars", "bar"), "bars ouverts");
  assert.equal(it.along("caffè", "Limmat"), "caffè lungo la Limmat");
  assert.equal(it.along("caffè", "Cassarate"), "caffè lungo il Cassarate");
  assert.equal(it.along("caffè", "Aare"), "caffè lungo l'Aare");
  assert.equal(it.bridges("Cassarate"), "ponti sul Cassarate");
  assert.equal(it.bridges("Limmat"), "ponti sulla Limmat");
  assert.equal(it.open("farmacie", "pharmacy"), "farmacie aperte");
});
