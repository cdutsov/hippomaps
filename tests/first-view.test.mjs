// The tiles of the first view and the fonts of a style, which the page asks for as its script starts, before the map
// does (firstTiles, styleFonts in web/app.js).
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/first-view.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const a = src.indexOf("const FIRST_TILES = "), b = src.indexOf("// what the first view of a style takes");
assert.ok(a > 0 && b > a);
const { firstTiles, styleFonts } = new Function(`${src.slice(a, b)}\nreturn { firstTiles, styleFonts };`)();
const SRC = { tiles: ["https://t.example/v/{z}/{x}/{y}.mvt"], minzoom: 0, maxzoom: 15 };
const ids = (urls) => urls.map((u) => u.replace("https://t.example/v/", "").replace(".mvt", ""));
// the box of a view: w by h pixels of the screen around a place, at a zoom (tiles of 512 px, as the map draws them)
const box = (lon, lat, zoom, w, h) => {
  const world = 512 * 2 ** zoom, rad = Math.PI / 180;
  const x = (lon + 180) / 360 * world, y = (0.5 - Math.log(Math.tan(Math.PI / 4 + lat * rad / 2)) / (2 * Math.PI)) * world;
  const toLon = (px) => px / world * 360 - 180, toLat = (py) => Math.atan(Math.sinh(Math.PI * (1 - 2 * py / world))) / rad;
  return [toLon(x - w / 2), toLat(y + h / 2), toLon(x + w / 2), toLat(y - h / 2)];
};

test("a phone at the town of the visitor, zoom 11: the three tiles under each other that the map takes", () => {
  // (Frankfurt, 390 by 844 px: MapLibre asked for these three there)
  assert.deepEqual(ids(firstTiles(box(8.6821, 50.1109, 11, 390, 844), 11, SRC)), ["11/1073/692", "11/1073/693", "11/1073/694"]);
});
test("the zoom of the tiles: that of the map rounded down, the largest of the source beyond it", () => {
  assert.ok(ids(firstTiles(box(8.54, 47.37, 15.6, 390, 844), 15.6, SRC)).every((t) => t.startsWith("15/")));
  assert.ok(ids(firstTiles(box(8.54, 47.37, 18.2, 390, 844), 18.2, SRC)).every((t) => t.startsWith("15/")));
  assert.equal(firstTiles(box(8.54, 47.37, 18.2, 390, 844), 18.2, SRC).length, 1); // one tile of zoom 15 holds the view
  assert.ok(ids(firstTiles(box(8.54, 47.37, 12.9, 390, 844), 12.9, SRC)).every((t) => t.startsWith("12/")));
});
test("every tile the view touches, each once: columns by rows", () => {
  const b = box(8.54, 47.37, 13, 1400, 900), got = ids(firstTiles(b, 13, SRC));
  const xs = new Set(got.map((t) => t.split("/")[1])), ys = new Set(got.map((t) => t.split("/")[2]));
  assert.equal(got.length, xs.size * ys.size);
  assert.equal(new Set(got).size, got.length);
  assert.ok(xs.size >= 3 && xs.size <= 4 && ys.size >= 2 && ys.size <= 3, `${xs.size} by ${ys.size}`);
  // the tile of the middle is among them
  assert.ok(got.includes("13/4290/2868"), got.join(" "));
});
test("none: below zoom 7 (the globe), a view that takes too many, a source without tiles or that starts above", () => {
  assert.deepEqual(firstTiles(box(8.54, 47.37, 5, 390, 844), 5, SRC), []);
  assert.deepEqual(firstTiles(box(8.54, 47.37, 6.9, 390, 844), 6.9, SRC), []);
  assert.equal(firstTiles(box(8.54, 47.37, 7, 390, 844), 7, SRC).length > 0, true);
  assert.deepEqual(firstTiles(box(8.54, 47.37, 15, 5000, 3000), 15, SRC), []);
  assert.equal(firstTiles(box(8.54, 47.37, 15, 2560, 1440), 15, SRC).length <= 30, true);
  assert.deepEqual(firstTiles(box(8.54, 47.37, 12, 390, 844), 12, {}), []);
  assert.deepEqual(firstTiles(box(8.54, 47.37, 9, 390, 844), 9, { ...SRC, minzoom: 10 }), []);
});
test("across the antimeridian and at the top of the world: valid tiles", () => {
  const got = ids(firstTiles([179.9, 60, 180.1, 60.1], 9, SRC));
  assert.ok(got.length >= 2 && got.every((t) => { const [z, x, y] = t.split("/").map(Number); return x >= 0 && x < 2 ** z && y >= 0 && y < 2 ** z; }), got.join(" "));
  assert.ok(got.some((t) => t.split("/")[1] === "0") && got.some((t) => t.split("/")[1] === "511"));
  assert.ok(ids(firstTiles([10, 84, 10.2, 89], 8, SRC)).every((t) => Number(t.split("/")[2]) >= 0));
});
test("the fonts of a style: lists and those in expressions, each once", () => {
  const st = { layers: [
    { layout: { "text-font": ["Noto Sans Regular"] } }, { layout: { "text-font": ["Noto Sans Italic"] } }, { layout: { "text-font": ["Noto Sans Regular"] } },
    { layout: { "text-font": ["case", ["<=", ["get", "min_zoom"], 5], ["literal", ["Noto Sans Medium"]], ["literal", ["Noto Sans Regular"]]] } },
    { layout: { "text-font": ["Open Sans Bold", "Arial Unicode MS Bold"] } }, { layout: { "text-font": ["get", "font"] } }, { layout: {} }, { type: "fill" },
  ] };
  assert.deepEqual(styleFonts(st), ["Noto Sans Regular", "Noto Sans Italic", "Noto Sans Medium", "Open Sans Bold,Arial Unicode MS Bold"]);
  assert.deepEqual(styleFonts({}), []);
});
