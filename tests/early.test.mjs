// What the page asks for as it starts, before its large scripts are there (web/early.js): at a first visit the light
// and the dark style in the language of the browser with their TileJSON, later what the map asked for the last time
// (hm-early, kept by app.js), and the worker of MapLibre unless the service worker has the files of the page.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/early.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/early.js", import.meta.url), "utf8");
const TILES = "https://tiles.example", KEY = "pk_1";
const WORKER = "/static/vendor/maplibre-gl-6.13.0/maplibre-gl-worker.mjs?v=6.13.0";
const STYLE = (name, lang = "en") => `${TILES}/v1/${KEY}/maps/styles/${name}.json?lang=${lang}`;
const TILEJSON = `${TILES}/v1/${KEY}/maps/vector.json`;

// the script in a page: what it asked for, in order, once all its answers came. answers: address -> JSON
async function run({ dataset = { tiles: TILES, key: KEY, worker: WORKER }, early, language = "en-GB", controller = null, answers = {}, storage = true } = {}) {
  const asked = [], read = [];
  const pending = [];
  const fetch = (u) => {
    asked.push(u);
    const body = answers[u];
    const p = Promise.resolve({
      ok: body !== undefined,
      json: async () => { read.push(u); return body; },
      arrayBuffer: async () => { read.push(u); return new ArrayBuffer(0); },
    });
    pending.push(p);
    return p;
  };
  const localStorage = { getItem: (k) => { if (!storage) throw new Error("no storage"); return k === "hm-early" ? early ?? null : null; } };
  const window = { fetch };
  new Function("document", "window", "fetch", "navigator", "localStorage", "devicePixelRatio", src)(
    { currentScript: dataset && { dataset } }, window, fetch, { language, serviceWorker: { controller } }, localStorage, 2);
  for (let i = 0; i < 5; i++) { await Promise.all(pending.slice()); await new Promise((done) => setTimeout(done, 0)); }
  return { asked, read, early: window.hmEarly };
}
const styleOf = (extra = {}) => ({ sources: { protomaps: { type: "vector", url: TILEJSON }, ...extra }, layers: [] });

test("a first visit: the worker, the light and the dark style in the language of the browser, their TileJSON once", async () => {
  const first = await run({ answers: { [STYLE("light")]: styleOf(), [STYLE("dark")]: styleOf(), [TILEJSON]: { tiles: ["t"] } } });
  assert.deepEqual(first.asked, [WORKER, STYLE("light"), STYLE("dark"), TILEJSON]);
  assert.ok(first.read.includes(WORKER) && first.read.includes(TILEJSON), "read to the end, so the browser keeps them");
  // what it read is there for app.js, by address: the styles and the TileJSON
  assert.deepEqual(Object.keys(first.early.styles), [STYLE("light"), STYLE("dark")]);
  assert.deepEqual(first.early.sources, { [TILEJSON]: { tiles: ["t"] } });
  const de = await run({ language: "de-CH", answers: { [STYLE("light", "de")]: styleOf(), [STYLE("dark", "de")]: styleOf() } });
  assert.deepEqual(de.asked, [WORKER, STYLE("light", "de"), STYLE("dark", "de"), TILEJSON]);
  const bg = await run({ language: "bg" }); // a language without names of its own on the map: English, as app.js asks
  assert.deepEqual(bg.asked, [WORKER, STYLE("light"), STYLE("dark")]);
});

test("a later visit: what the map asked for the last time, nothing else", async () => {
  const last = [`${TILES}/v1/${KEY}/maps/styles/outdoor.json?lang=fr&hillshade=1`, TILEJSON, `${TILES}/maps/assets/sprites/v4/light@2x.json`, `${TILES}/maps/assets/sprites/v4/light@2x.png`];
  const { asked, read, early } = await run({ early: JSON.stringify(last) });
  assert.deepEqual(asked, [WORKER, ...last]);
  assert.deepEqual(read, asked);
  assert.equal(early, undefined); // (app.js has its own note of the last visit: hm-first)
});

test("the service worker has the files of the page: the worker is not asked for", async () => {
  const last = [STYLE("dark"), TILEJSON];
  const { asked } = await run({ early: JSON.stringify(last), controller: {} });
  assert.deepEqual(asked, last);
});

test("a list of another key or site, or not a list: as a first visit; addresses of other sites are left out", async () => {
  const first = [WORKER, STYLE("light"), STYLE("dark")];
  for (const early of [JSON.stringify([`${TILES}/v1/pk_old/maps/styles/dark.json?lang=en`, TILEJSON]), JSON.stringify(["https://evil.example/x"]), "{", "null", '"x"', JSON.stringify([7])]) {
    assert.deepEqual((await run({ early })).asked, first, early);
  }
  assert.deepEqual((await run({ storage: false })).asked, first);
  const mixed = await run({ early: JSON.stringify([STYLE("dark"), "https://evil.example/x", 7, null, TILEJSON]) });
  assert.deepEqual(mixed.asked, [WORKER, STYLE("dark"), TILEJSON]);
  // a style of a first visit that names a source elsewhere: not asked for
  const other = await run({ answers: { [STYLE("light")]: styleOf({ x: { type: "vector", url: "https://evil.example/t.json" }, y: { type: "geojson", data: {} } }) } });
  assert.deepEqual(other.asked, [WORKER, STYLE("light"), STYLE("dark"), TILEJSON]);
});

test("a server without a key for the tiles: the worker alone; a style that fails: no error", async () => {
  assert.deepEqual((await run({ dataset: { tiles: TILES, key: "", worker: WORKER } })).asked, [WORKER]);
  assert.deepEqual((await run({ dataset: null })).asked, []);
  assert.deepEqual((await run({ answers: {} })).asked, [WORKER, STYLE("light"), STYLE("dark")]);
});

test("app.js keeps what early.js reads: hm-early, the style first", () => {
  const app = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
  assert.match(app, /store\.set\("hm-early", JSON\.stringify\(\[styleURL\(\), /);
  assert.match(src, /localStorage\.getItem\("hm-early"\)/);
});
