// The results of a search as a file (dataCSV and dataGeoJSON in web/app.js): a row per result with its point and
// tags, fields quoted, no formulas for a spreadsheet; the GeoJSON with the query and the credit of the ODbL.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/download.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const fn = (name) => {
  const a = src.indexOf(`function ${name}(`), b = src.indexOf("\n}\n", a);
  assert.ok(a > 0 && b > a, name);
  return new Function(`${src.slice(a, b)}\n}\nreturn ${name};`)();
};
const dataCSV = fn("dataCSV"), dataGeoJSON = fn("dataGeoJSON");
const center = (f) => f.geometry.type === "Point" ? f.geometry.coordinates : [8.5, 47.25];
const feats = [
  { type: "Feature", geometry: { type: "Point", coordinates: [7.432782, 46.953921] },
    properties: { id: "node/1", name: "Café, \"Bär\"", opening_hours: "Mo-Sa 07:00-16:00", type: "amenity=cafe",
      tags: { amenity: "cafe", website: "https://example.org", "addr:street": "Mittelstrasse" } } },
  { type: "Feature", geometry: { type: "LineString", coordinates: [[8, 47], [9, 47.5]] },
    properties: { id: "way/2", tags: { amenity: "cafe", note: "=HYPERLINK(\"x\")", ele: "-5", fixme: "-x", description: "two\nlines" } } },
];
// a small CSV reader of RFC 4180
const parse = (t) => {
  const rows = [[]];
  let f = "", q = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (q) { if (c === '"' && t[i + 1] === '"') { f += '"'; i++; } else if (c === '"') q = false; else f += c; }
    else if (c === '"') q = true;
    else if (c === ",") { rows.at(-1).push(f); f = ""; }
    else if (c === "\r" && t[i + 1] === "\n") { rows.at(-1).push(f); f = ""; rows.push([]); i++; }
    else f += c;
  }
  rows.pop();
  return rows;
};

test("CSV: a BOM, a head, a row per result with the point; the most common tags first", () => {
  const t = dataCSV(feats, center);
  assert.ok(t.startsWith("﻿id,name,lat,lon,amenity,"), t.slice(0, 60));
  const [head, a, b] = parse(t.slice(1));
  assert.equal(head.length, a.length);
  assert.equal(b.length, a.length);
  const row = (r) => Object.fromEntries(head.map((k, i) => [k, r[i]]));
  assert.deepEqual([row(a).id, row(a).name, row(a).lat, row(a).lon, row(a).opening_hours], ["node/1", "Café, \"Bär\"", "46.953921", "7.432782", "Mo-Sa 07:00-16:00"]);
  assert.deepEqual([row(b).id, row(b).name, row(b).lat, row(b).lon, row(b).amenity], ["way/2", "", "47.25", "8.5", "cafe"]);
  assert.equal(row(b).description, "two\nlines");
  assert.ok(!head.includes("type"), "the kind of the answer is no tag");
});
test("CSV: no formula for a spreadsheet, numbers kept", () => {
  const [head, , b] = parse(dataCSV(feats, center).slice(1));
  const row = Object.fromEntries(head.map((k, i) => [k, b[i]]));
  assert.equal(row.note, "'=HYPERLINK(\"x\")");
  assert.equal(row.fixme, "'-x");
  assert.equal(row.ele, "-5");
});
test("CSV of no results: the head only", () => {
  assert.equal(dataCSV([], center), "﻿id,name,lat,lon\r\n");
});
test("GeoJSON: the features, the query, the credit of the ODbL", () => {
  const g = JSON.parse(dataGeoJSON(feats, "[out:json];nwr[amenity=cafe];out;"));
  assert.equal(g.type, "FeatureCollection");
  assert.equal(g.features.length, 2);
  assert.deepEqual(g.features[1].geometry, feats[1].geometry);
  assert.equal(g.features[0].properties.tags.website, "https://example.org");
  assert.match(g.attribution, /OpenStreetMap contributors/);
  assert.match(g.license, /ODbL/);
  assert.match(g.query, /amenity=cafe/);
});
