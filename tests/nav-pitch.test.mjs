// The camera of navigation zoomed out (navPitchFor in web/app.js): tilted near, straight down far.
// docker run --rm -v "$PWD":/r:ro -w /r node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const line = src.match(/^const navPitchFor = .*$/m);
assert.ok(line, "navPitchFor found in app.js");
const pitch = (np) => new Function("NAV_PITCH", `${line[0]}; return navPitchFor;`)(np);

test("tilted at zoom 15 and closer, flat at 12 and farther, in between by the zoom", () => {
  const f = pitch(50);
  assert.equal(f(18), 50);
  assert.equal(f(15), 50);
  assert.equal(f(13.5), 25);
  assert.equal(f(12), 0);
  assert.equal(f(5), 0);
  for (let z = 10; z < 17; z += 0.25) assert.ok(f(z) <= f(z + 0.25), `no more tilt farther out (${z})`);
});

test("the handler leaves the camera that follows and the 2D map alone", () => {
  const i = src.indexOf("const navPitchFor");
  const h = src.slice(i, src.indexOf("});", i));
  assert.match(h, /!e\.originalEvent \|\| !nav\.on \|\| nav\.follow \|\| nav\.flat/);
});
