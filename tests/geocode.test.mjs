// The start and destination of directions: a place of the geocoder before the area of the same name
// around it (placesOverAreas in web/app.js). The answer for Gadmen of 2026-11-10: the area of the old
// municipality first, with its point on the slope of the Susten pass, past its winter closure.
// docker run --rm -v "$PWD":/r:ro -w /r node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const start = src.indexOf("function placesOverAreas(");
const end = src.indexOf("\n}\n", start) + 2;
assert.ok(start > 0 && end > start, "placesOverAreas found in app.js");
const placesOverAreas = new Function(`${src.slice(start, end)}; return placesOverAreas;`)();

const f = (name, key, ll, extent) => ({ geometry: { coordinates: ll }, properties: { name, osm_key: key, ...(extent ? { extent } : {}) } });
const area = f("Gadmen", "boundary", [8.3801308, 46.7035822], [8.2730142, 46.7743193, 8.4551633, 46.6327683]);
const elsewhere = f("Gadmen", "place", [9.1434229, 47.1426349]);
const village = f("Gadmen", "place", [8.3527408, 46.7381684]);
const quarter = f("Gadmen", "place", [8.1811501, 46.7582531]);

test("the village in the area takes its rank, the area goes", () => {
  assert.deepEqual(placesOverAreas([area, elsewhere, village, quarter]), [village, elsewhere, quarter]);
});
test("an area without a place of its name in it stays", () => {
  assert.deepEqual(placesOverAreas([area, elsewhere, quarter]), [area, elsewhere, quarter]);
});
test("places and addresses alone stay as they are", () => {
  const street = f("Bahnhofstrasse", "highway", [8.54, 47.37]);
  assert.deepEqual(placesOverAreas([street, village]), [street, village]);
});
