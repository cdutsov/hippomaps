// The heading of the compass of a phone browser (headingOf in web/app.js) from the orientation the browser gives
// (alpha, beta, gamma: turns about z, x and y of the phone) and the angle of the screen: flat, upright, tilted, over the
// face, in landscape.
// docker run --rm -v "$PWD":/r:ro -w /r node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs'
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const start = src.indexOf("const headingOf = (");
const end = src.indexOf("\n};\n", start) + 3;
assert.ok(start > 0 && end > start, "headingOf found in app.js");
const headingOf = new Function(`${src.slice(start, end)}; return headingOf;`)();

const apart = (a, b) => Math.abs(((a - b) % 360 + 540) % 360 - 180);
const near = (got, want, what) => assert.ok(got != null && apart(got, want) < 0.5, `${what}: ${got}, want ${want}`);

test("flat on a table", () => {
  near(headingOf(0, 0, 0), 0, "the top to north");
  near(headingOf(90, 0, 0), 270, "turned counterclockwise: to west");
  near(headingOf(270, 0, 0), 90, "to east");
  near(headingOf(30, 0, 0), 330, "alpha 30");
});

test("held up and tilted: the way the back faces", () => {
  near(headingOf(0, 90 - 1e-9, 0), 0, "upright, alpha 0");
  near(headingOf(90, 89, 0), 270, "upright, to west");
  for (const beta of [10, 30, 45, 60, 80]) near(headingOf(200, beta, 0), 160, `tilted ${beta}`);
  near(headingOf(0, 100, 0), 0, "past upright, the screen a little down");
});

test("tilted sideways or turned in its own plane, the heading stays", () => {
  // flat, tilted to a side (gamma)
  for (const gamma of [-60, -20, -5, 5, 20, 60]) near(headingOf(270, 0, gamma), 90, `flat, tilted sideways ${gamma}`);
  // held up facing north and turned in its own plane like a wheel (alpha -90, beta 90 - turn, gamma 90)
  for (const turn of [-30, -10, 10, 30]) near(headingOf(-90, 90 - turn, 90), 0, `held up, turned in its plane ${turn}`);
  // tipped forward and tilted to a side at once: no single answer; it moves a little (the top and the back part)
  for (const gamma of [-15, 15]) {
    const h = headingOf(200, 30, gamma);
    assert.ok(h != null && apart(h, 160) < 8, `tipped 30, tilted ${gamma}: ${h}`);
  }
});

test("over the face, the screen down", () => {
  near(headingOf(0, 180, 0), 180, "beta 180: the top to south");
  assert.equal(headingOf(0, 135, 0), null, "the top and the back cancel out");
});

test("the screen turned", () => {
  near(headingOf(0, 0, 0, 90), 90, "landscape (90): the right side of the phone, to east");
  near(headingOf(0, 0, 0, 270), 270, "landscape (270): the left side, to west");
  near(headingOf(0, 0, 0, 180), 180, "upside down");
  near(headingOf(90, 0, 0, 90), 0, "landscape, alpha 90: the right side to north");
});
