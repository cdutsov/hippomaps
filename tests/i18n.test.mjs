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
test("the search in navigation in each language", () => {
  for (const lang of ["de", "fr", "it"]) for (const k of ["navFind", "navFindNone", "useAsTo", "useAsStop"]) assert.ok(all[lang][k] && all[lang][k] !== EN[k], `${lang} ${k}`);
});
