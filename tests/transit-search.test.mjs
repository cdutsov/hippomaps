// The lines of a city ("metro in Lausanne", transitSearch in web/app.js): a search of its own, with the place from
// the geocoder and the lines from the tiles. A newer search (or the X) aborts it: what comes back after that writes
// nothing into the panel of the newer search.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/transit-search.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const cut = (from, to) => {
  const a = src.indexOf(from), b = src.indexOf(to, a);
  assert.ok(a > 0 && b > a, from);
  return src.slice(a, b);
};
// the page around transitSearch: what it writes is kept in page
function page() {
  const p = { title: { textContent: "" }, notes: { textContent: "" }, count: { textContent: "" }, results: [], said: [], fitted: 0, sheet: "", answer: null, fetched: null };
  const line = (kind, ref) => ({ properties: { kind, ref, name: `Line ${ref}` }, geometry: { coordinates: [[[6.6, 46.5], [6.7, 46.6]]] } });
  const env = {
    clear: () => { p.results.length = 0; }, showView: () => {}, listView: {}, title: p.title, notes: p.notes, count: p.count,
    results: { append: (li) => p.results.push(li) }, say: (m) => p.said.push(m), setTransit: () => {}, setSheet: (s) => { p.sheet = s; },
    failText: (t) => t, UI: "en", root: { dataset: { tiles: "https://tiles.example", key: "k" } },
    T: { searching: "Searching…", failed: "failed", transitIn: (n) => `Public transport in ${n}`, transitNone: "none", results: (n) => `${n} results`, transitKinds: { subway: "Metro" } },
    fetch: (url, o) => { p.fetched = { url, signal: o?.signal }; return new Promise((ok) => { p.answer = ok; }); },
    map: { once: (ev, fn) => setTimeout(fn, 20), getBounds: () => null }, // the map takes a while to get there
    fitBox: () => { p.fitted++; }, fit: () => { p.fitted++; }, glide: () => {}, pad: () => ({}),
    loadTransit: async () => [line("subway", "M1"), line("subway", "M2")],
    el: (tag, cls, attrs, text) => ({ tag, text, style: {}, append() {} }),
  };
  const names = Object.keys(env);
  p.transitSearch = new Function(...names, `${cut("async function transitSearch(", "\n}\n")}\n}\nreturn transitSearch;`)(...names.map((k) => env[k]));
  p.place = { json: async () => ({ features: [{ properties: { name: "Lausanne", extent: [6.5, 46.7, 6.8, 46.4] }, geometry: { coordinates: [6.63, 46.52] } }] }) };
  return p;
}
const q = { kinds: ["subway"], place: "Lausanne" };

test("the lines of the city: the title, the map on the city, the lines listed", async () => {
  const p = page(), ctl = new AbortController();
  const done = p.transitSearch(q, "metro in Lausanne", ctl.signal);
  assert.equal(p.fetched.signal, ctl.signal, "the request for the place ends with the search");
  p.answer(p.place);
  await done;
  assert.equal(p.title.textContent, "Public transport in Lausanne");
  assert.equal(p.fitted, 1);
  assert.equal(p.results.length, 2);
  assert.equal(p.count.textContent, "2 results");
  assert.equal(p.sheet, "half");
});
test("a newer search while the place is looked up: no title, no move of the map, no lines in its list", async () => {
  const p = page(), ctl = new AbortController();
  const done = p.transitSearch(q, "metro in Lausanne", ctl.signal);
  ctl.abort(); // run() of the newer search, or the X
  p.title.textContent = "Cafés";
  p.count.textContent = "12 results";
  p.said.length = 0;
  p.answer(p.place);
  await done;
  assert.equal(p.title.textContent, "Cafés");
  assert.equal(p.count.textContent, "12 results");
  assert.equal(p.fitted, 0);
  assert.equal(p.results.length, 0);
  assert.deepEqual(p.said, [], "the status of the newer search stays");
  assert.equal(p.sheet, "");
});
test("a newer search while the map moves to the city: no lines in its list", async () => {
  const p = page(), ctl = new AbortController();
  const done = p.transitSearch(q, "metro in Lausanne", ctl.signal);
  p.answer(p.place);
  await new Promise((ok) => setImmediate(ok)); // the place is there, the map on its way
  ctl.abort();
  p.count.textContent = "12 results";
  await done;
  assert.equal(p.results.length, 0);
  assert.equal(p.count.textContent, "12 results");
  assert.equal(p.sheet, "");
});
test("run hands transitSearch the signal of the search", () => {
  assert.match(cut("async function run(", "\n}\n"), /return transitSearch\(tq, text, ctl\.signal\);/);
});
