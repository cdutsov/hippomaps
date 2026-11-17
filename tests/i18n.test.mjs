// The texts of the page: each text of English (EN in web/app.js) in German, French and Italian
// (web/i18n.js), none left in English by mistake.
// docker run --rm -v "$PWD":/r:ro -w /r node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { texts } from "../web/i18n.js";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const start = src.indexOf("const EN = {");
const end = src.indexOf("\n};\n", start) + 3;
assert.ok(start > 0 && end > start, "EN found in app.js");
const num = (n) => String(n);
const EN = new Function("num", "dur", `${src.slice(start, end)}; return EN;`)(num, (s) => `${s} s`);
const all = texts(num);
assert.ok(Object.keys(EN).length > 100, "the texts of EN read");

for (const lang of ["de", "fr", "it"]) {
  test(`every text in ${lang}`, () => {
    const missing = Object.keys(EN).filter((k) => !(k in all[lang]));
    assert.deepEqual(missing, [], `texts without ${lang}: ${missing.join(", ")}`);
  });
}
// the side panel and the menu credit only the voices and the traffic: the map credits OpenStreetMap, each picture
// its source, and the chains are not credited
test("no line of credits for the map data, the pictures and the chains", () => {
  for (const [lang, t] of [["en", EN], ...["de", "fr", "it"].map((l) => [l, all[l]])]) assert.ok(!("credit" in t), `${lang} credit`);
  assert.ok(!/name-suggestion-index|Map data ©/.test(src), "app.js");
});
test("the search in navigation in each language", () => {
  for (const lang of ["de", "fr", "it"]) for (const k of ["navFind", "navFindNone", "useAsTo", "useAsStop"]) assert.ok(all[lang][k] && all[lang][k] !== EN[k], `${lang} ${k}`);
});
// the menu below Forget recent searches: one link to the source code in each language, and the donation translated
test("free software and the donation in each language", () => {
  for (const [lang, t] of [["en", EN], ...["de", "fr", "it"].map((l) => [l, all[l]])]) {
    assert.equal(t.aboutApp.split(/[[\]]/).length, 3, `${lang} aboutApp: one [link]`);
    if (lang !== "en") for (const k of ["aboutApp", "donateAsk", "donateAmount", "donate", "donateFailed", "donated"]) assert.notEqual(t[k], EN[k], `${lang} ${k}`);
  }
});

// the rows of the menu: 3D buildings, Relief shading and Directions and voice say it all, with no line under them;
// Traffic names its countries in its label, from the page (data-traffic-countries), not from the texts (the button on
// the map stays Traffic)
test("the rows of the menu: no line under a label that says it all", () => {
  for (const [lang, t] of [["en", EN], ...["de", "fr", "it"].map((l) => [l, all[l]])]) {
    for (const k of ["buildings3DNote", "shadingNote", "navLanguageNote"]) assert.ok(!(k in t), `${lang} ${k}`);
    assert.ok(!("trafficMenu" in t), `${lang} trafficMenu: the countries come from the page`);
    assert.ok(!/Switzerland|Schweiz|Suisse|Svizzera/.test(t.trafficNote), `${lang} trafficNote`);
    assert.ok(!/colou?rs|Farben|couleurs|colori/.test(t.transitNote), `${lang} transitNote`);
    assert.ok(!/names|noms|nomi|beschriftung/i.test(t.languageNote), `${lang} languageNote: ${t.languageNote}`);
  }
  assert.match(src, /const trafficRow = toggle\(trafficLabel\(\), T\.trafficNote/, "the label of Traffic names the countries");
  assert.match(src, /new Intl\.DisplayNames\(\[UI\], \{ type: "region" \}\)/, "the countries by name in the language of the page");
  assert.equal(EN.trafficNote, "Closures, queues and roadworks");
  assert.equal(EN.transitNote, "Metro, light rail and tram lines");
  assert.equal(EN.languageNote, "Page and map");
  assert.ok(!/Note\b/.test(src.slice(src.indexOf("const buildingsRow = toggle("), src.indexOf("const buildingsRow = toggle(") + 80)), "3D buildings without a note");
  assert.match(src, /if \(note\) text\.append\(el\("span", "lm-toggle-note"/, "no empty line under a switch without a note");
});
