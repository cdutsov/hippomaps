// The POIs of the map (morePois in web/app.js): the layer pois of a style of overpass.ch draws a list of kinds; the page
// widens it to every POI with a name but the things of the street, gives a kind without an icon a dot and a colour.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/pois.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const cut = (from, to) => {
  const a = src.indexOf(from), b = src.indexOf(to, a);
  assert.ok(a > 0 && b > a, from);
  return src.slice(a, b);
};
const { morePois, nearPois, POI_NOT } = new Function(`${cut("const POI_NOT = [", "// on the map, and in a style JSON")}
  ${cut("function nearPois(", "\n}\n")}\n}\nreturn { morePois, nearPois, POI_NOT };`)();

const kinds = (list) => ["in", ["get", "kind"], ["literal", list]];
const layer = () => ({
  id: "pois", type: "symbol", source: "protomaps", "source-layer": "pois",
  filter: ["all", kinds(["park", "school", "townhall", "building", "restaurant", "cafe", "supermarket"]), [">=", ["zoom"], ["+", ["get", "min_zoom"], 0]]],
  layout: { "icon-image": ["match", ["get", "kind"], "station", "train_station", ["cafe", "park"], ["get", "kind"], ""], "text-field": ["get", "name"] },
  paint: { "text-color": ["case", kinds(["park"]), "#080", kinds(["school", "townhall", "building"]), "#606", kinds(["restaurant", "cafe"]), "#c60", "#eee"] },
});
// what a filter of the kind used here lets through, for a POI at the largest zoom
const passes = (f, p) => {
  const ev = (x) => {
    if (!Array.isArray(x)) return x;
    const [op, ...a] = x;
    if (op === "all") return a.every(ev);
    if (op === "any") return a.some(ev);
    if (op === "!") return !ev(a[0]);
    if (op === "has") return a[0] in p;
    if (op === "get") return p[a[0]];
    if (op === "literal") return a[0];
    if (op === "in") return ev(a[1]).includes(ev(a[0]));
    if (op === "zoom") return 19;
    if (op === ">=") return ev(a[0]) >= ev(a[1]);
    if (op === "<") return ev(a[0]) < ev(a[1]);
    if (op === "+") return ev(a[0]) + ev(a[1]);
    if (op === "-") return ev(a[0]) - ev(a[1]);
    throw new Error(op);
  };
  return ev(f);
};

test("a POI of any kind with a name is drawn: a dentist, a pharmacy, a bank, a hotel, a kind never seen", () => {
  const m = morePois(layer());
  for (const kind of ["dentist", "pharmacy", "bank", "hotel", "bakery", "a_kind_of_tomorrow"]) {
    assert.equal(passes(layer().filter, { kind, name: "X", min_zoom: 17 }), false, kind);
    assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 17 }), true, kind);
  }
});
test("the kinds of the style as before, with a name or without; other kinds without a name not", () => {
  const m = morePois(layer());
  assert.equal(passes(m.filter, { kind: "park", min_zoom: 15 }), true);
  assert.equal(passes(m.filter, { kind: "cafe", name: "X", min_zoom: 16 }), true);
  assert.equal(passes(m.filter, { kind: "dentist", min_zoom: 17 }), false);
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 30 }), false); // the zoom of the tiles still counts
});
test("things of the street, of the railway and areas of land use stay out, with a name too", () => {
  const m = morePois(layer());
  for (const kind of ["crossing", "traffic_signals", "tree", "stop", "platform", "vacant", "residential", "street_lamp"]) {
    assert.ok(POI_NOT.includes(kind), kind);
    assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 16 }), false, kind);
  }
});
test("a kind without an icon in the sprite gets the dot; the icons of the style stay", () => {
  const icon = morePois(layer()).layout["icon-image"];
  assert.equal(icon.at(-1), "lm-poi-dot");
  assert.deepEqual(icon.slice(0, -1), layer().layout["icon-image"].slice(0, -1));
});
test("the colour of the text: other kinds as the public buildings, food as the restaurants, never the colour of the land", () => {
  const c = morePois(layer()).paint["text-color"];
  assert.equal(c.at(-1), "#606");
  assert.ok(c[5][2][1].includes("bakery") && c[5][2][1].includes("restaurant"));
  assert.deepEqual(c.slice(0, 5), layer().paint["text-color"].slice(0, 5));
});
test("where POIs are dense, those the tiles hold from a lower zoom come first", () => {
  assert.deepEqual(morePois(layer()).layout["symbol-sort-key"], ["coalesce", ["get", "min_zoom"], 17]);
});
test("a layer widened once is left as it is (a style comes through twice); a layer without a list of kinds too", () => {
  const l = layer(), m = morePois(l);
  assert.equal(morePois({ ...l, filter: m.filter }), null);
  assert.equal(morePois({ ...l, filter: [">=", ["zoom"], 15] }), null);
  assert.equal(morePois(undefined), null);
});
test("the POIs two zooms earlier (nearPois) keep the wider filter", () => {
  const l = layer(), m = morePois(l);
  const near = nearPois({ ...l, filter: m.filter, layout: { ...l.layout, ...m.layout } });
  assert.equal(passes(near.layer.filter, { kind: "dentist", name: "X", min_zoom: 21 }), true); // 19 >= 21 - 2
  assert.equal(passes(near.filter, { kind: "dentist", name: "X", min_zoom: 17 }), true);
  assert.equal(morePois({ ...l, filter: near.filter }), null);
});
// the Outdoor style: its layer pois leaves the kinds of its own layers (peaks, huts, viewpoints) out with a second
// list of kinds; that list stays as it is, or every POI with a name goes with it
test("a style that leaves kinds to another layer (Outdoor: peaks, huts): its POIs with a name are drawn, those kinds are not", () => {
  const l = layer();
  l.filter = ["all", l.filter, ["!", kinds(["peak", "alpine_hut", "viewpoint"])]];
  const m = morePois(l);
  for (const kind of ["restaurant", "cafe", "dentist", "hotel"]) assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 17 }), true, kind);
  assert.equal(passes(m.filter, { kind: "park", min_zoom: 15 }), true);
  for (const kind of ["peak", "alpine_hut", "viewpoint"]) assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 15 }), false, kind);
});
