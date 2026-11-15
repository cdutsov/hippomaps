// The other routes of directions (web/app.js): how much longer each takes (altDiff), the name that tells it from the
// others (altVia), where its time stands on the map (altLabelAt).
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/alternates.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const fn = (name) => {
  const a = src.indexOf(`function ${name}(`), b = src.indexOf("\n}\n", a) + 3;
  assert.ok(a > 0 && b > a, `${name} in app.js`);
  return src.slice(a, b);
};
const T = {
  sameTime: "Same time",
  duration: (s) => {
    const m = Math.max(1, Math.round(s / 60));
    return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ""}`;
  },
};
const route = { alts: [] };
const { altDiff, altVia, altLabelAt } = new Function("T", "route", `${fn("altDiff")}${fn("altVia")}${fn("altLabelAt")}; return { altDiff, altVia, altLabelAt };`)(T, route);

test("how much longer: + or − its minutes, hours beyond an hour, Same time within half a minute", () => {
  assert.equal(altDiff({ s: 4500 }, { s: 3600 }), "+15 min");
  assert.equal(altDiff({ s: 3300 }, { s: 3600 }), "−5 min");
  assert.equal(altDiff({ s: 3620 }, { s: 3600 }), "Same time");
  assert.equal(altDiff({ s: 3600 + 4500 }, { s: 3600 }), "+1 h 15 min");
  assert.equal(altDiff({ s: 3640 }, { s: 3600 }), "+1 min");
});

test("via: the road a route follows for the most kilometres more than the others, a sixth of it at least", () => {
  const st = (street, km) => ({ street, km });
  const a = { km: 120, steps: [st("Zürichstrasse", 2), st("A1", 110), st("Laupenstrasse", 1)] };
  const b = { km: 150, steps: [st("Zürichstrasse", 2), st("A1", 20), st("A2 / E35", 70), st("A6", 50), st("Laupenstrasse", 1)] };
  const c = { km: 125, steps: [st("Zürichstrasse", 2), st("A1", 118), st("Bernstrasse", 4)] }; // all its own roads short
  route.alts = [a, b];
  assert.equal(altVia(a), "A1");
  assert.equal(altVia(b), "A2");
  route.alts = [a, c];
  assert.equal(altVia(a), "");
  assert.equal(altVia(c), "");
});

test("the label: the middle of the longest stretch off the route; the middle of the line without one", () => {
  const route = [[0, 0], [1, 0], [2, 0], [3, 0], [9, 0]];
  const on = new Set(route.map(String));
  const alt = [[0, 0], [0.5, 1], [1, 0], [1, 1], [1.5, 1], [2, 1], [2.5, 1], [3, 0], [9, 0]];
  assert.deepEqual(altLabelAt(alt, on), [1.5, 1]);
  assert.deepEqual(altLabelAt(route, on), [2, 0]);
});
