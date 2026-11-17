// Reachable area (web/app.js): the times of the bands, their colours, the bands without overlaps (the area of the time
// before as a hole), their areas, and the areas written as GeoJSON and KML.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/reach.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const body = (name) => {
  const a = src.indexOf(`function ${name}(`), b = src.indexOf("\n}\n", a);
  assert.ok(a > 0 && b > a, name);
  return src.slice(a, b + 2);
};
const colours = /const REACH_COLOURS = (\[[^\]]*\]);/.exec(src)[1];
const names = ["ringArea", "xmlText", "reachSteps", "reachColours", "inRing", "reachBands", "reachArea", "reachCredit", "dataReachGeoJSON", "dataReachKML"];
const F = new Function(`const REACH_COLOURS = ${colours};\n${names.map(body).join("\n")}\nreturn { REACH_COLOURS, ${names.join(", ")} };`)();
const close = (got, want, tol, what) => assert.ok(Math.abs(got - want) <= tol, `${what}: ${got}, not ${want} ± ${tol}`);

// squares around 8.5, 47.4 of half-width d degrees, closed rings
const sq = (d, x = 8.5, y = 47.4) => [[x - d, y - d], [x + d, y - d], [x + d, y + d], [x - d, y + d], [x - d, y - d]];
const answer = () => ({ type: "FeatureCollection", features: [
  { type: "Feature", properties: { min: 5, mode: "walk" }, geometry: { type: "MultiPolygon", coordinates: [[sq(0.01), sq(0.002, 8.505, 47.405)]] } },
  { type: "Feature", properties: { min: 10, mode: "walk" }, geometry: { type: "MultiPolygon", coordinates: [[sq(0.02)], [sq(0.003, 8.6, 47.4)]] } },
  { type: "Feature", properties: { min: 15, mode: "walk" }, geometry: { type: "MultiPolygon", coordinates: [[sq(0.03), sq(0.002, 8.52, 47.42)]] } },
] });

test("the times of the bands", () => {
  assert.deepEqual(F.reachSteps(10), [5, 10]);
  assert.deepEqual(F.reachSteps(15), [5, 10, 15]);
  assert.deepEqual(F.reachSteps(30), [10, 20, 30]);
  assert.deepEqual(F.reachSteps(45), [15, 30, 45]);
  assert.deepEqual(F.reachSteps(60), [15, 30, 45, 60]);
  assert.deepEqual(F.reachSteps(120), [30, 60, 90, 120]);
  for (const m of [10, 15, 30, 45, 60, 120]) assert.ok(F.reachSteps(m).length <= 4, `${m}: at most 4 (the server)`);
});
test("the colours: green near, red far, spread over the four", () => {
  const C = F.REACH_COLOURS;
  assert.equal(C.length, 4);
  assert.deepEqual(F.reachColours(1), [C[0]]);
  assert.deepEqual(F.reachColours(2), [C[0], C[3]]);
  assert.deepEqual(F.reachColours(3), [C[0], C[2], C[3]]);
  assert.deepEqual(F.reachColours(4), C);
});
test("a point in a ring", () => {
  assert.ok(F.inRing([8.5, 47.4], sq(0.01)));
  assert.ok(!F.inRing([8.52, 47.4], sq(0.01)));
});
test("the bands: each time without the time before, its holes a band of this time, and an outline each", () => {
  const b = F.reachBands(answer()).features, fills = b.filter((f) => !f.properties.edge), edges = b.filter((f) => f.properties.edge);
  assert.equal(fills.length, 3);
  assert.equal(edges.length, 3);
  assert.deepEqual(fills.map((f) => f.properties.min), [5, 10, 15]);
  assert.deepEqual(fills.map((f) => f.properties.c), F.reachColours(3));
  // 5 min: as it came, its hole kept
  assert.equal(fills[0].geometry.coordinates.length, 1);
  assert.equal(fills[0].geometry.coordinates[0].length, 2);
  // 10 min: the square of 5 min a hole of its large square, the island beside without; the hole of 5 min a band of 10
  const ten = fills[1].geometry.coordinates;
  assert.equal(ten.length, 3);
  assert.equal(ten[0].length, 2);
  assert.deepEqual(ten[0][1], sq(0.01));
  assert.equal(ten[1].length, 1);
  assert.deepEqual(ten[2], [sq(0.002, 8.505, 47.405)]);
  // 15 min: its own hole and the square of 10 min; the island of 10 min (outside 15 min's square) no hole
  const fifteen = fills[2].geometry.coordinates;
  assert.equal(fifteen.length, 1);
  assert.equal(fifteen[0].length, 3);
  // the answer is not changed
  assert.equal(answer().features[1].geometry.coordinates[0].length, 1);
  // the band areas add up to the largest area (without the hole of 15 min and the island of 10 outside it)
  const sum = fills.reduce((s, f) => s + F.reachArea(f.geometry), 0);
  const a = answer().features;
  close(sum, F.reachArea(a[2].geometry) + F.reachArea({ coordinates: [a[1].geometry.coordinates[1]] }), 1, "bands");
});
test("a hole of both times (a lake) stays without colour", () => {
  const lake = sq(0.001, 8.505, 47.405), fc = { features: [
    { properties: { min: 5 }, geometry: { type: "MultiPolygon", coordinates: [[sq(0.01), lake]] } },
    { properties: { min: 10 }, geometry: { type: "MultiPolygon", coordinates: [[sq(0.02), lake]] } }] };
  const ten = F.reachBands(fc).features.filter((f) => !f.properties.edge)[1].geometry.coordinates;
  assert.equal(ten.length, 1, "no band in the lake");
  assert.deepEqual(ten[0], [sq(0.02), lake, sq(0.01)]);
});
test("an area with holes", () => {
  const g = answer().features[0].geometry;
  close(F.reachArea(g), F.ringArea(sq(0.01)) - F.ringArea(sq(0.002, 8.505, 47.405)), 0.01, "5 min");
  close(F.reachArea(g) / 1e6, 3.22, 0.03, "1.5 by 2.2 km less the hole");
});
test("GeoJSON: each time with minutes, mode, area and colours, the largest first, the start last", () => {
  const g = JSON.parse(F.dataReachGeoJSON({ fc: answer(), at: [8.5, 47.4], name: "Zürich", mode: "walk" }));
  assert.equal(g.type, "FeatureCollection");
  assert.match(g.attribution, /OpenStreetMap contributors/);
  assert.deepEqual(g.features.map((f) => f.properties.minutes ?? "start"), [15, 10, 5, "start"]);
  const f = g.features[2];
  assert.equal(f.geometry.type, "MultiPolygon");
  assert.equal(f.properties.mode, "walk");
  assert.equal(f.properties.fill, F.REACH_COLOURS[0]);
  assert.equal(f.properties.stroke, F.REACH_COLOURS[0]);
  close(f.properties.area_m2, F.reachArea(answer().features[0].geometry), 1, "area_m2");
  assert.deepEqual(g.features[3], { type: "Feature", geometry: { type: "Point", coordinates: [8.5, 47.4] }, properties: { name: "Zürich", start: true, mode: "walk" } });
});
test("KML: a style per time in the colours of KML, polygons with their holes, the start", () => {
  const k = F.dataReachKML({ fc: answer(), at: [8.5, 47.4], name: "A & B", mode: "bike" });
  assert.match(k, /^<\?xml/);
  assert.match(k, /<name>A &amp; B<\/name>/);
  // #0f9f7a: alpha, blue, green, red
  assert.ok(k.includes(`<PolyStyle><color>4d7a9f0f</color>`), "colour of KML");
  assert.equal((k.match(/<Style id=/g) || []).length, 3);
  assert.equal((k.match(/<Placemark>/g) || []).length, 4);
  assert.equal((k.match(/<innerBoundaryIs>/g) || []).length, 2);
  assert.ok(k.indexOf("15 min (bike)") < k.indexOf("5 min (bike)"), "the largest first");
  assert.match(k, /<Point><coordinates>8.5,47.4<\/coordinates><\/Point>/);
  assert.match(k, /OpenStreetMap contributors/);
});

test("by public transport: the files credit the timetables, the other modes not", () => {
  const fc = answer(), at = [8.5, 47.4];
  for (const f of fc.features) f.properties.mode = "transit";
  const g = JSON.parse(F.dataReachGeoJSON({ fc, at, mode: "transit" }));
  assert.match(g.attribution, /Valhalla.*timetables of transitous\.org by MOTIS/);
  assert.ok(g.features.every((f) => f.properties.mode === "transit"));
  assert.match(F.dataReachKML({ fc, at, mode: "transit" }), /<description>[^<]*transitous\.org[^<]*<\/description>/);
  assert.doesNotMatch(JSON.parse(F.dataReachGeoJSON({ fc: answer(), at, mode: "walk" })).attribution, /MOTIS/);
});
test("by public transport: up to an hour, the times the server takes", () => {
  const times = /const REACH_TIMES = \{[^\n]*transit: (\[[^\]]*\])/.exec(src);
  assert.ok(times, "transit in REACH_TIMES");
  for (const m of JSON.parse(times[1])) {
    assert.ok(m <= 60, `${m} min: the server takes up to 60 by public transport`);
    assert.ok(F.reachSteps(m).length <= 4);
  }
});
