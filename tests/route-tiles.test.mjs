// The tiles of the map kept for a route (routeTiles in web/app.js): under the line at the 6 highest zooms of a
// source, at the 3 highest also those around them, the box of the route at the 5 below when it is a few tiles, in
// the order of the way, at most max.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/route-tiles.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const a = src.indexOf("function routeTiles("), b = src.indexOf("\n}\n", a) + 3;
assert.ok(a > 0 && b > a, "routeTiles in app.js");
const routeTiles = new Function("ROUTE_TILES", `${src.slice(a, b)}; return routeTiles;`)(5000);

const vector = { tiles: ["https://t/v1/k/maps/vector/{z}/{x}/{y}.mvt"], minzoom: 0, maxzoom: 15 };
const terrain = { tiles: ["https://t/v1/k/maps/terrain/{z}/{x}/{y}.webp"], minzoom: 0, maxzoom: 10 };
const zxy = (u) => u.match(/\/(\d+)\/(\d+)\/(\d+)\.\w+$/).slice(1).map(Number);
const tile = (lon, lat, z) => {
  const n = 2 ** z, r = (lat * Math.PI) / 180;
  return [z, Math.floor(((lon + 180) / 360) * n), Math.floor(((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * n)];
};
// Zurich main station to Bellevue, about 1.3 km
const line = [[8.5403, 47.3779], [8.5431, 47.3735], [8.5450, 47.3669]];

test("along the line: the tile under each point at zooms 10 to 15, at 13 to 15 also the 8 around it", () => {
  const got = new Set(routeTiles(line, vector).map((u) => zxy(u).join("/")));
  for (const [lon, lat] of line) {
    for (let z = 10; z <= 15; z++) {
      const [, x, y] = tile(lon, lat, z);
      assert.ok(got.has(`${z}/${x}/${y}`), `${z}/${x}/${y}`);
      if (z >= 13) for (const [dx, dy] of [[-1, -1], [1, 1], [-1, 1], [1, -1], [0, 1]]) assert.ok(got.has(`${z}/${x + dx}/${y + dy}`), `${z}/${x + dx}/${y + dy}`);
    }
  }
  const z10 = [...got].filter((k) => k.startsWith("10/"));
  assert.ok(z10.length <= 2, `zoom 10: the tiles under the line only (${z10})`);
  assert.ok(![...got].some((k) => +k.split("/")[0] > 15), "nothing above the zoom of the source");
});

test("the overview: the box of the route at zooms 5 to 9, first", () => {
  const list = routeTiles(line, vector).map(zxy);
  for (let z = 5; z <= 9; z++) assert.ok(list.some(([tz]) => tz === z), `zoom ${z}`);
  const firstNear = list.findIndex(([z]) => z >= 10);
  assert.ok(list.slice(0, firstNear).every(([z]) => z < 10) && list.slice(firstNear).every(([z]) => z >= 10), "the overview before the line");
});

test("in the order of the way: the start before the end", () => {
  const list = routeTiles(line, vector).map((u) => zxy(u).join("/"));
  const start = list.indexOf(tile(...line[0], 15).join("/")), end = list.indexOf(tile(...line[2], 15).join("/"));
  assert.ok(start >= 0 && end > start, `${start} < ${end}`);
});

test("terrain to zoom 10: zooms 5 to 10 near the line", () => {
  const zs = new Set(routeTiles(line, terrain).map((u) => zxy(u)[0]));
  for (let z = 5; z <= 10; z++) assert.ok(zs.has(z), `zoom ${z}`);
  assert.ok(![...zs].some((z) => z > 10));
});

test("each tile once, at most max, a long route", () => {
  const long = [[8.5403, 47.3779], [7.4391, 46.949], [6.6323, 46.5197]]; // Zurich, Bern, Lausanne
  const all = routeTiles(long, vector);
  assert.equal(new Set(all).size, all.length);
  assert.ok(all.length > 600 && all.length < 2500, `${all.length} tiles`);
  assert.equal(routeTiles(long, vector, 100).length, 100);
});

test("across the antimeridian and near the poles: valid tiles only", () => {
  for (const u of routeTiles([[179.99, 89], [-179.99, 89]], vector)) {
    const [z, x, y] = zxy(u);
    assert.ok(x >= 0 && x < 2 ** z && y >= 0 && y < 2 ** z, u);
  }
});
