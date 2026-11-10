// The arrows of the turns ahead in navigation (turnArrows in web/app.js): where they start and end on the line,
// which steps have one, the way their head points, the part behind the traveller left out.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/turn-arrows.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const part = (from, to) => {
  const a = src.indexOf(from), b = src.indexOf(to, a);
  assert.ok(a > 0 && b > a, `${from} in app.js`);
  return src.slice(a, b);
};
const { cumOf, turnArrows } = new Function(`
${part("function cumOf(", "let elevMarker")}
${part("const M = 111320;", "// prepare a route answer")}
${part("const TURN_TYPES", "// the arrows of the turns ahead")}
return { cumOf, turnArrows };`)();

// a line from a point in steps of 10 m: n steps north (N), east (E), south (S) or west (W)
const M = 111320, LAT = 47;
function walk(moves) {
  const line = [[8.5, LAT]], corners = [];
  for (const [dir, n] of moves) {
    for (let k = 0; k < n; k++) {
      const [x, y] = line[line.length - 1];
      const dx = { E: 10, W: -10 }[dir] || 0, dy = { N: 10, S: -10 }[dir] || 0;
      line.push([x + dx / (M * Math.cos((LAT * Math.PI) / 180)), y + dy / M]);
    }
    corners.push(line.length - 1);
  }
  return { line, cum: cumOf(line), corners };
}
const len = (f) => { const c = cumOf(f.geometry.coordinates); return c[c.length - 1]; };
const arrows = (fs) => fs.filter((f) => f.geometry.type === "LineString");
const heads = (fs) => fs.filter((f) => f.geometry.type === "Point");
const near = (a, b, d = 1.5) => Math.abs(a - b) <= d;

// north 300 m, right (east) 300 m, left (north) 300 m
const R = walk([["N", 30], ["E", 30], ["N", 30]]);
const steps = [{ type: 1, i: 0 }, { type: 10, i: R.corners[0] }, { type: 15, i: R.corners[1] }, { type: 4, i: R.corners[2] }];

test("an arrow for each turn: 60 px before it and 45 px after, the head the way on", () => {
  const fs = turnArrows(steps, R.line, R.cum, 0, 1);
  assert.equal(arrows(fs).length, 2);
  const [a, b] = arrows(fs), [ha, hb] = heads(fs);
  assert.ok(near(len(a), 105) && near(len(b), 105), `${len(a)} ${len(b)}`);
  // the first from 240 m along the line, the head 45 m east of the corner, pointing east
  assert.ok(near((a.geometry.coordinates[0][1] - LAT) * M, 240), "starts 240 m north");
  assert.deepEqual(a.geometry.coordinates.at(-1), ha.geometry.coordinates);
  assert.ok(near(ha.properties.rotate, 90, 1), `rotate ${ha.properties.rotate}`);
  assert.ok(near(hb.properties.rotate, 0, 1), `rotate ${hb.properties.rotate}`);
});

test("the size follows the zoom: twice the metres of a pixel, twice as long", () => {
  const [a] = arrows(turnArrows(steps, R.line, R.cum, 0, 2));
  assert.ok(near(len(a), 210), `${len(a)}`);
});

test("the part behind the traveller is left out; a turn passed has none", () => {
  const turnAt = R.cum[R.corners[0]];
  const [a] = arrows(turnArrows(steps, R.line, R.cum, turnAt - 20, 1));
  assert.ok(near(len(a), 65), `${len(a)}`);
  const fs = turnArrows(steps, R.line, R.cum, turnAt + 50, 1);
  assert.equal(arrows(fs).length, 1);
  assert.equal(heads(fs)[0].properties.n, 2);
});

test("two turns close together: each arrow to half the way, no overlap", () => {
  const C = walk([["N", 30], ["E", 3], ["N", 30]]);
  const st = [{ type: 1, i: 0 }, { type: 10, i: C.corners[0] }, { type: 15, i: C.corners[1] }, { type: 4, i: C.corners[2] }];
  const [a, b] = arrows(turnArrows(st, C.line, C.cum, 0, 1));
  assert.ok(near(len(a), 60 + 15) && near(len(b), 15 + 45), `${len(a)} ${len(b)}`);
});

test("straight on, a merge and the arrival have no arrow", () => {
  const st = [{ type: 1, i: 0 }, { type: 8, i: R.corners[0] }, { type: 25, i: R.corners[1] }, { type: 4, i: R.corners[2] }];
  assert.deepEqual(turnArrows(st, R.line, R.cum, 0, 1), []);
});

test("a roundabout: one arrow from before it to after its exit", () => {
  // into the roundabout at the first corner, out at the second (its arc the 30 m east), on north
  const C = walk([["N", 30], ["E", 3], ["N", 30]]);
  const st = [{ type: 1, i: 0 }, { type: 26, i: C.corners[0], end: C.corners[1] }, { type: 27, i: C.corners[1] }, { type: 4, i: C.corners[2] }];
  const fs = turnArrows(st, C.line, C.cum, 0, 1);
  assert.equal(arrows(fs).length, 1);
  assert.ok(near(len(arrows(fs)[0]), 60 + 30 + 45), `${len(arrows(fs)[0])}`);
  assert.ok(near(heads(fs)[0].properties.rotate, 0, 1));
});

test("a turn soon after a roundabout: the two arrows meet half way, no overlap", () => {
  // the roundabout (30 m east), out north, 40 m on a turn right
  const C = walk([["N", 30], ["E", 3], ["N", 4], ["E", 30]]);
  const st = [{ type: 1, i: 0 }, { type: 26, i: C.corners[0], end: C.corners[1] }, { type: 27, i: C.corners[1] },
    { type: 10, i: C.corners[2] }, { type: 4, i: C.corners[3] }];
  const [a, b] = arrows(turnArrows(st, C.line, C.cum, 0, 1));
  assert.ok(near(len(a), 60 + 30 + 20) && near(len(b), 20 + 45), `${len(a)} ${len(b)}`);
});

test("at most 4 arrows, within 3 km", () => {
  const Z = walk(Array.from({ length: 12 }, (_, k) => [k % 2 ? "E" : "N", 30]));
  const st = [{ type: 1, i: 0 }, ...Z.corners.slice(0, -1).map((i, k) => ({ type: k % 2 ? 15 : 10, i })), { type: 4, i: Z.corners.at(-1) }];
  assert.equal(arrows(turnArrows(st, Z.line, Z.cum, 0, 1)).length, 4);
  const far = walk([["N", 400], ["E", 30]]);
  assert.deepEqual(turnArrows([{ type: 1, i: 0 }, { type: 10, i: far.corners[0] }, { type: 4, i: far.corners[1] }], far.line, far.cum, 0, 1), []);
});
