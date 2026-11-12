// A search around the view (a chain or a name alone, which the search looks for in 50 by 50 km): the map goes to the
// nearest result and a quarter beyond, not out to all of them (aroundView, nearBox in web/app.js).
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/near.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const a = src.indexOf("const aroundView = "), b = src.indexOf("function nearBox("), c = src.indexOf("\n}\n", b);
assert.ok(a > 0 && b > a && c > b);
const { aroundView, nearBox } = new Function(`${src.slice(a, src.indexOf("\n", a))}\n${src.slice(b, c)}\n}\nreturn { aroundView, nearBox };`)();

test("a search around the view is known by the comment of its query, in any language of the page", () => {
  assert.equal(aroundView({ query: "[out:json][timeout:60];\n// 50 × 50 km around the centre of the view\n// chain “Migros”\n(nwr…);" }), true);
  assert.equal(aroundView({ query: "[out:json];\n// Cafe\n// inside the boundary of Zurich\nnwr…;" }), false);
  assert.equal(aroundView({ query: "[out:json];\n// Restaurant, in the view\nnwr…;" }), false);
  assert.equal(aroundView({}), false);
});
test("the box holds the nearest place and a quarter more each way, around where the visitor is", () => {
  const from = [8.54, 47.37], near = [8.56, 47.38], far = [8.9, 47.6];
  const box = nearBox([far, near, null], from);
  const k = Math.cos(47.37 * Math.PI / 180), d = Math.hypot(0.02 * k, 0.01);
  assert.ok(Math.abs((box[3] - box[1]) / 2 - 1.25 * d) < 1e-9);
  assert.ok(Math.abs((box[0] + box[2]) / 2 - from[0]) < 1e-9 && Math.abs((box[1] + box[3]) / 2 - from[1]) < 1e-9);
  assert.ok(near[0] > box[0] && near[0] < box[2] && near[1] > box[1] && near[1] < box[3]);
  assert.ok(!(far[0] < box[2] && far[1] < box[3]));
});
test("a place at the door: 200 m around at least; no place: no box", () => {
  const box = nearBox([[8.54001, 47.37001]], [8.54, 47.37]);
  assert.ok(Math.abs((box[3] - box[1]) / 2 - 0.002) < 1e-9);
  assert.equal(nearBox([], [8.54, 47.37]), null);
  assert.equal(nearBox([null], [8.54, 47.37]), null);
});
