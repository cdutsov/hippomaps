// Coordinates written in the search box or a field of directions (coordsOf in web/app.js): the point itself, where the
// geocoder would find a street with those numbers on another continent.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/coords.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const a = src.indexOf("function coordsOf("), b = src.indexOf("\n}\n", a);
assert.ok(a > 0 && b > a);
const coordsOf = new Function(`${src.slice(a, b)}\n}\nreturn coordsOf;`)();
const near = (got, lon, lat) => assert.ok(got && Math.abs(got[0] - lon) < 1e-4 && Math.abs(got[1] - lat) < 1e-4, JSON.stringify(got));

test("decimal degrees, the latitude first: with a comma, a space, a semicolon, negative", () => {
  near(coordsOf("47.3769, 8.5417"), 8.5417, 47.3769);
  near(coordsOf("46.9480 7.4474"), 7.4474, 46.948);
  near(coordsOf(" -33.869;151.209 "), 151.209, -33.869);
  near(coordsOf("40.7580,-73.9855"), -73.9855, 40.758);
});
test("degrees and minutes, degrees minutes and seconds, with letters before or after", () => {
  near(coordsOf("N 46° 56.9' E 7° 26.8'"), 7.44667, 46.94833);
  near(coordsOf("46°56'54\"N 7°26'48\"E"), 7.44667, 46.94833);
  near(coordsOf("46° 56′ 54″ N, 7° 26′ 48″ E"), 7.44667, 46.94833);
  near(coordsOf("33.869 S 151.209 E"), 151.209, -33.869);
  near(coordsOf("47° 8°"), 8, 47);
});
test("letters say which is which: the longitude may come first; O is east (Ost, ouest is W)", () => {
  near(coordsOf("E 7.4474 N 46.9480"), 7.4474, 46.948);
  near(coordsOf("73.9855 W, 40.7580 N"), -73.9855, 40.758);
  near(coordsOf("47.3769 N 8.5417 O"), 8.5417, 47.3769);
});
test("other texts are no coordinates", () => {
  for (const t of ["", "8 10", "12, 5", "Bahnhofstrasse 10", "cafes in Zurich", "47.3769", "47.3769, 8.5417, 3", "91.5, 8.5", "47.5, 181.2",
    "N 47.3 N 8.5", "N 47.3 S 8.5", "1.5 km", "route 66", "2.5 3.5 4.5", "Zürich 8001"]) assert.equal(coordsOf(t), null, t);
});
