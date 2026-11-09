// The 30 fps cap of navigation (fpsCap in web/app.js) against a fake map and fake screens.
// docker run --rm -v "$PWD":/r:ro -w /r node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const start = src.indexOf("const fpsCap = (() => {");
const end = src.indexOf("})();", start) + 5;
assert.ok(start > 0 && end > start, "fpsCap found in app.js");
const code = src.slice(start, end);

class FakeMap { triggerRepaint() { this.asked = (this.asked || 0) + 1; } }

function run(hz, secs = 10) {
  let queue = [], t = 0;
  const draws = [];
  const map = new FakeMap();
  map.redraw = () => { draws.push(t); map.triggerRepaint(); }; // a moving camera wants the next frame at once
  const raf = (f) => { queue.push(f); return queue.length; };
  const fpsCap = new Function("map", "requestAnimationFrame", "cancelAnimationFrame", `${code}; return fpsCap;`)(map, raf, () => {});
  fpsCap(true);
  assert.ok(Object.hasOwn(map, "triggerRepaint"), "the cap replaces triggerRepaint");
  map.triggerRepaint();
  for (let i = 0; i < hz * secs; i++) {
    t = (i * 1000) / hz + (Math.random() - 0.5); // vsync times with a little jitter
    const due = queue; queue = []; due.forEach((f) => f(t));
  }
  fpsCap(false);
  const gaps = draws.slice(1).map((x, i) => x - draws[i]);
  return { map, fps: draws.length / secs, minGap: Math.min(...gaps), maxGap: Math.max(...gaps) };
}

for (const hz of [60, 90, 120, 144]) {
  test(`30 draws a second at ${hz} Hz`, () => {
    const r = run(hz);
    assert.ok(Math.abs(r.fps - 30) <= 0.5, `${r.fps} draws/s`);
    assert.ok(r.minGap > 1000 / 30 - 4 - 2000 / hz, `shortest gap ${r.minGap} ms`);
    assert.ok(r.maxGap < 1000 / 30 + 1000 / hz + 1, `longest gap ${r.maxGap} ms`);
    assert.ok(!Object.hasOwn(r.map, "triggerRepaint"), "off: the prototype triggerRepaint again");
    assert.equal(r.map.triggerRepaint, FakeMap.prototype.triggerRepaint);
  });
}

test("off at once: a pending frame asks MapLibre to draw", () => {
  const map = new FakeMap();
  map.redraw = () => {};
  const fpsCap = new Function("map", "requestAnimationFrame", "cancelAnimationFrame", `${code}; return fpsCap;`)(map, () => 1, () => {});
  fpsCap(true);
  map.triggerRepaint();
  fpsCap(false);
  assert.equal(map.asked, 1);
});
