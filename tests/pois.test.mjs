// The POIs of the map (morePois in web/app.js): the layer pois of a style of overpass.ch draws a list of kinds; the page
// adds a layer under the names of the streets for every other POI with a name but the things of the street, with a dot
// for a kind without an icon and a colour.
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
const { morePois, morePoisUnder, earlierMore, latePois, nearPois, POI_NOT, POI_GAP } = new Function(`${cut("const POI_NOT = [", "// the layer of the style it goes under")}
  ${cut("const morePoisUnder = ", "\n")}
  ${cut("function earlierMore(", "\n}\n")}
}
  ${cut("const POI_LATE = ", "// on the map, and in a style JSON")}
  ${cut("function nearPois(", "\n}\n")}\n}\nreturn { morePois, morePoisUnder, earlierMore, latePois, nearPois, POI_NOT, POI_GAP };`)();

const kinds = (list) => ["in", ["get", "kind"], ["literal", list]];
const layer = () => ({
  id: "pois", type: "symbol", source: "protomaps", "source-layer": "pois",
  filter: ["all", kinds(["park", "school", "townhall", "building", "restaurant", "cafe", "supermarket"]), [">=", ["zoom"], ["+", ["get", "min_zoom"], 0]]],
  layout: { "icon-image": ["match", ["get", "kind"], "station", "train_station", ["cafe", "park"], ["get", "kind"], ""], "text-field": ["get", "name"] },
  paint: { "text-color": ["case", kinds(["park"]), "#080", kinds(["school", "townhall", "building"]), "#606", kinds(["restaurant", "cafe"]), "#c60", "#eee"] },
});
// what a filter of the kind used here lets through, for a POI at the largest zoom
const passes = (f, p, zoom = 19) => {
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
    if (op === "zoom") return zoom;
    if (op === "step") { let v = a[1]; for (let i = 2; i + 1 < a.length; i += 2) if (ev(a[0]) >= a[i]) v = a[i + 1]; return v; }
    if (op === ">=") return ev(a[0]) >= ev(a[1]);
    if (op === "<") return ev(a[0]) < ev(a[1]);
    if (op === "+") return ev(a[0]) + ev(a[1]);
    if (op === "-") return ev(a[0]) - ev(a[1]);
    throw new Error(op);
  };
  return ev(f);
};

test("a POI of any other kind with a name is in the added layer: a dentist, a pharmacy, a bank, a hotel, a kind never seen", () => {
  const m = morePois(layer());
  assert.equal(m.id, "lm-pois-more");
  for (const kind of ["dentist", "pharmacy", "bank", "hotel", "bakery", "a_kind_of_tomorrow"]) {
    assert.equal(passes(layer().filter, { kind, name: "X", min_zoom: 17 }), false, kind);
    assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 17 }), true, kind);
  }
});
test("the kinds of the style stay in the layer of the style alone; other kinds without a name are in neither", () => {
  const m = morePois(layer());
  assert.equal(passes(m.filter, { kind: "park", min_zoom: 15 }), false);
  assert.equal(passes(m.filter, { kind: "cafe", name: "X", min_zoom: 16 }), false);
  assert.equal(passes(layer().filter, { kind: "cafe", name: "X", min_zoom: 16 }), true);
  assert.equal(passes(m.filter, { kind: "dentist", min_zoom: 17 }), false);
});
test("from zoom 15; at the largest zooms two zooms before the tiles say", () => {
  const m = morePois(layer());
  assert.equal(m.minzoom, 15);
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 21 }), true); // 19 >= 21 - 2
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 22 }), false);
});
test("things of the street, of the railway and areas of land use stay out, with a name too", () => {
  const m = morePois(layer());
  for (const kind of ["crossing", "traffic_signals", "tree", "stop", "platform", "vacant", "residential", "street_lamp", "zone"]) { // (zone: "Subzone 12" of the parking of Sofia)
    assert.ok(POI_NOT.includes(kind), kind);
    assert.equal(passes(m.filter, { kind, name: "X", min_zoom: 16 }), false, kind);
  }
});
test("a list of kinds the style leaves out (Outdoor: its huts and peaks have a layer of their own) is left out here too", () => {
  const l = layer();
  l.filter = ["all", l.filter[1], ["!", kinds(["peak", "alpine_hut"])], l.filter[2]];
  const m = morePois(l);
  assert.equal(passes(m.filter, { kind: "alpine_hut", name: "X", min_zoom: 15 }), false);
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 17 }), true);
  assert.deepEqual(l.filter[2], ["!", kinds(["peak", "alpine_hut"])]); // the layer of the style as it was
});
test("a kind without an icon in the sprite gets the dot", () => {
  const icon = morePois(layer()).layout["icon-image"];
  assert.equal(icon.at(-1), "lm-poi-dot");
  assert.deepEqual(icon.slice(0, -1), layer().layout["icon-image"].slice(0, -1));
});
test("the colour of the text: other kinds as the public buildings, food as the restaurants, never the colour of the land", () => {
  const c = morePois(layer()).paint["text-color"];
  assert.equal(c.at(-1), "#606");
  assert.ok(c[5][2][1].includes("bakery") && c[5][2][1].includes("restaurant"));
});
test("where POIs are dense, those the tiles hold from a lower zoom come first", () => {
  assert.deepEqual(morePois(layer()).layout["symbol-sort-key"], ["coalesce", ["get", "min_zoom"], 17]);
});
test("the added layer lies under the names of the streets, which come before it; without such a layer under pois", () => {
  const ids = (...a) => a.map((id) => ({ id }));
  assert.equal(morePoisUnder(ids("earth", "roads_oneway", "roads_labels_minor", "roads_shields", "roads_labels_major", "pois")), "roads_labels_minor");
  assert.equal(morePoisUnder(ids("earth", "roads_labels_major", "pois")), "roads_labels_major");
  assert.equal(morePoisUnder(ids("earth", "pois")), "pois");
});
test("the layer of the style is not changed; a layer without a list of kinds gets no added layer", () => {
  const l = layer(), before = JSON.stringify(l);
  morePois(l);
  assert.equal(JSON.stringify(l), before);
  assert.equal(morePois({ ...l, filter: [">=", ["zoom"], 15] }), null);
  assert.equal(morePois(undefined), null);
});
test("made from a layer pois already shown two zooms earlier (nearPois), the added layer is the same", () => {
  const l = layer(), near = nearPois(l);
  const m = morePois({ ...l, filter: near.filter });
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 21 }), true);
  assert.equal(passes(m.filter, { kind: "cafe", name: "X", min_zoom: 16 }), false);
});
test("the name of an added POI goes where there is room around its dot, and the dot stays without it", () => {
  const l = layer();
  l.layout["text-offset"] = [1.1, 0];
  const m = morePois(l);
  assert.deepEqual(m.layout["text-variable-anchor"], ["left", "right", "top", "bottom"]);
  assert.deepEqual(m.layout["text-optional"], ["step", ["zoom"], false, 17, true]); // below zoom 17 no icon without its name
  assert.deepEqual(m.layout["icon-allow-overlap"], ["step", ["zoom"], false, 18, true]); // the dot in any case from zoom 18
  assert.equal(m.layout["text-offset"], undefined);
  assert.deepEqual(l.layout["text-offset"], [1.1, 0]); // the layer of the style as it was
});
test("a style with places of more kinds under the names of the streets (pois_more of overpass.ch): those kinds are not ours, ours go under it", () => {
  const theirs = { id: "pois_more", filter: ["all", kinds(["dentist", "hotel"]), [">=", ["zoom"], ["+", ["get", "min_zoom"], 0]]] };
  const m = morePois(layer(), theirs);
  assert.equal(passes(m.filter, { kind: "dentist", name: "X", min_zoom: 17 }), false);
  assert.equal(passes(m.filter, { kind: "money_lender", name: "X", min_zoom: 17 }), true);
  assert.equal(morePoisUnder([{ id: "earth" }, { id: "pois_more" }, { id: "roads_labels_minor" }, { id: "pois" }]), "pois_more");
});
test("pois_more of a style is shown as early as our layer, once", () => {
  const theirs = { id: "pois_more", filter: ["all", kinds(["dentist", "hotel"]), [">=", ["zoom"], ["+", ["get", "min_zoom"], 0]]] };
  const f = earlierMore(theirs);
  assert.equal(passes(f, { kind: "dentist", name: "X", min_zoom: 21 }), true);
  assert.equal(passes(f, { kind: "bank", name: "X", min_zoom: 17 }), false);
  assert.equal(earlierMore({ ...theirs, filter: f }), null);
  assert.equal(earlierMore(undefined), null);
});
test("benches: out of the layer of the style, in a layer of their own from zoom 18; the other places as before", () => {
  const l = layer();
  l.filter[1][2][1].push("bench");
  const late = latePois(l);
  assert.equal(late.layer.id, "lm-pois-late");
  assert.equal(late.layer.minzoom, 18);
  assert.equal(passes(late.layer.filter, { kind: "bench" }), true);
  assert.equal(passes(late.layer.filter, { kind: "cafe", name: "X" }), false);
  assert.equal(passes(late.filter, { kind: "bench", min_zoom: 17 }), false);
  assert.equal(passes(late.filter, { kind: "cafe", name: "X", min_zoom: 16 }), true);
  assert.equal(latePois({ ...l, filter: late.filter }), null); // once
  assert.equal(latePois(layer()), null); // a style without benches
  // our layer of the other places and the places two zooms earlier have no benches either
  const m = morePois({ ...l, filter: late.filter });
  assert.equal(passes(m.filter, { kind: "bench", name: "Bank am See", min_zoom: 17 }), false);
  assert.equal(passes(nearPois({ ...l, filter: late.filter }).layer.filter, { kind: "bench", min_zoom: 17 }), false);
});
test("the middle zooms are not full of places: one zoom before the tiles say below zoom 17, two from there, in every layer", () => {
  const l = layer(), near = nearPois(l);
  const theirs = { id: "pois_more", filter: ["all", kinds(["dentist", "hotel"]), [">=", ["zoom"], ["+", ["get", "min_zoom"], 0]]] };
  const filters = { near: near.layer.filter, pois: near.filter, more: morePois(l).filter, theirs: earlierMore(theirs) };
  const kind = { near: "restaurant", pois: "restaurant", more: "bank", theirs: "dentist" };
  for (const [name, f] of Object.entries(filters)) {
    const at = (zoom, min_zoom) => passes(f, { kind: kind[name], name: "X", min_zoom }, zoom);
    if (name !== "pois" && name !== "more" && name !== "theirs") { // zoom 14: the layer of the tiles of zoom 15
      assert.equal(at(14, 15), true, name); // a supermarket, a hotel
      assert.equal(at(14, 16), false, name); // no restaurants yet
    }
    if (name !== "near") {
      assert.equal(at(15, 16), true, name);
      assert.equal(at(15, 17), false, name); // no shops yet
      assert.equal(at(16, 17), true, name);
      assert.equal(at(16, 18), false, name);
      assert.equal(at(17, 19), true, name);
    }
  }
  // the icons keep their distance below zoom 17: more at the smaller zooms, the 2 px of MapLibre from 17
  assert.deepEqual(POI_GAP.slice(0, 3), ["interpolate", ["linear"], ["zoom"]]);
  const stops = POI_GAP.slice(3);
  assert.ok(stops[1] > stops[3] && stops[3] > 2 && stops.at(-2) === 17 && stops.at(-1) === 2, JSON.stringify(stops));
  assert.deepEqual(near.layer.layout["icon-padding"], POI_GAP);
  assert.deepEqual(morePois(l).layout["icon-padding"], POI_GAP);
});
