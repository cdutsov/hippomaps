// A change of language (setLang in web/app.js): the texts of the page are found by their old wording and given the
// new one (textSwaps). A word that is two labels in one language and two words in another keeps the first there;
// the labels this can hit are written again by their key.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/lang.test.mjs
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
const EN = new Function("num", "riverName", `${cut("const EN = {", "\n};\n")}\n}; return EN;`)(String, riverName);
const textSwaps = new Function(`${cut("function textSwaps(", "\n}\n")}\n}\nreturn textSwaps;`)();
const all = texts(String);
// the texts of a language as the page has them (loadTexts), and as setLang compares them: strings only, no ideas
const T = (lang) => {
  const tr = all[lang] || {};
  const t = JSON.parse(JSON.stringify({ ...EN, ...tr, errors: { ...EN.errors, ...tr.errors }, routeErrors: { ...EN.routeErrors, ...tr.routeErrors } }));
  delete t.idea;
  return t;
};
const langs = ["en", "de", "fr", "it"];

test("a text gets its text in the new language; one that is the same stays", () => {
  const swap = textSwaps({ a: "Save", b: { c: "Menu" }, list: [["car", "Car"]] }, { a: "Merken", b: { c: "Menu" }, list: [["car", "Auto"]] });
  assert.deepEqual([...swap], [["Save", "Merken"], ["Car", "Auto"]]);
});
test("a text with two translations keeps the first and tells the path of the other", () => {
  const told = [];
  const swap = textSwaps({ alongCats: { fuel: "Fuel" }, bubbles: [["petrol stations", "Fuel"]] },
    { alongCats: { fuel: "Carburant" }, bubbles: [["petrol stations", "Stations-service"]] }, (path, text) => told.push([path, text]));
  assert.equal(swap.get("Fuel"), "Carburant");
  assert.deepEqual(told, [["bubbles.0.1", "Fuel"]]);
});
// The bubbles and the kinds along a route are written again by their key (setLang), so a word they share does no harm.
// The message of a closed road (a popup of the map, gone with the next tap) shares Closed with the opening hours.
// Any other text with two translations would show the wrong one after a change of language.
test("between any two languages only the labels written again by their key share a word with two translations", () => {
  for (const from of langs) for (const to of langs) {
    if (from === to) continue;
    const told = [];
    const swap = textSwaps(T(from), T(to), (path, text) => told.push(`${path} "${text}"`));
    assert.deepEqual(told.filter((x) => !/^(bubbles\.\d+\.1|alongCats\.\w+|trafficKinds\.closure|foodMenu) /.test(x)), [], `${from} to ${to}`);
    // the other label of each such word is one written again too: the swap gives it the first translation
    for (const x of told) {
      const text = x.slice(x.indexOf('"') + 1, -1), first = swap.get(text);
      // (the menu of the page keeps "Menu"; the menu of a restaurant, a label with data-t, is written again by its key)
      const owners = [...Object.values(T(to).alongCats), ...T(to).bubbles.map((b) => b[1]), T(to).closed, T(to).menu];
      assert.ok(owners.includes(first), `${from} to ${to}: "${text}" first means ${first}`);
    }
  }
});
test("the page writes the bubbles and the kinds along a route again by their key, and the hint of a field of directions in the language of now", () => {
  const body = cut("function setLang(", "\n}\n");
  assert.match(body, /for \(const b of bubbles\.children\) b\.textContent = T\.bubbles\[b\.dataset\.n\]\[1\];/);
  assert.match(body, /querySelectorAll\("\[data-t\]"\)\) e\.textContent = T\[e\.dataset\.t\]/);
  assert.match(body, /\.lm-along-cats \[data-cat\]"\)\) c\.textContent = T\.alongCats\[c\.dataset\.cat\];/);
  assert.match(body, /end\?\.label === old\.mapPoint\) end\.label = T\.mapPoint/);
  const field = cut("function placeField(", "\n}\n");
  assert.match(field, /const hint = \(\) => /);
  assert.match(field, /field\.placeholder = hint\(\);/);
  assert.doesNotMatch(field, /const hint = field\.placeholder/);
});
