// The spoken turns of navigation (web/app.js): how far ahead a turn is said ("In 150 meters, …"), the
// rounding of that distance, and when the turn itself is said. The functions are taken from the source
// of app.js (from "const NAV = {" to the line of "const nav = "), with route, nav and STEP_LANG
// given here.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/nav-announce.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const from = src.indexOf("const NAV = {"), to = src.indexOf("const nav = {", from);
assert.ok(from > 0 && to > from, "the navigation settings in app.js");
const make = new Function(`let route = { mode: "bike" }, nav = { on: false }, STEP_LANG = "en";
${src.slice(from, to)}
return { NAV, sayRound, aheadOf, preAt, sayV, set: (mode, lang = "en", n = { on: false }) => { route.mode = mode; STEP_LANG = lang; nav = n; } };`);
const A = make();
// a turn after a way of m metres (the step before), by Valhalla
const steps = (m, alert = "Turn right onto Seilergraben.", s = m / 5) => [{ km: m / 1000, s, type: 8 }, { km: 0.3, s: 60, type: 10, alert }];

test("distances are said rounded down", () => {
  assert.deepEqual([47, 99, 149, 251, 299, 480, 999, 1400, 2600].map(A.sayRound), [40, 90, 100, 250, 250, 400, 900, 1000, 2500]);
});

test("by bike: 150 m ahead, the turn 20–30 m before it", () => {
  A.set("bike");
  assert.deepEqual(A.aheadOf(steps(600), 1, 5), { at: 150, text: "In 150 meters, turn right onto Seilergraben." });
  assert.equal(A.preAt(5), 25);
  assert.equal(A.preAt(2), 20);
  assert.equal(A.preAt(9), 30);
  // fast (downhill): 12 s ahead
  assert.equal(A.aheadOf(steps(600), 1, 15).at, 150);
  assert.equal(A.aheadOf(steps(600), 1, 20).at, 200);
});

test("by bike: a turn soon after the one before is said ahead at the distance there is, not below 50 m", () => {
  A.set("bike");
  assert.equal(A.aheadOf(steps(120), 1, 5).at, 100);
  assert.equal(A.aheadOf(steps(94), 1, 5).at, 80);
  assert.equal(A.aheadOf(steps(55), 1, 5), null); // the turn before says "Then …"
});

test("nothing ahead for a way straight on, the first step, or no sentence", () => {
  A.set("bike");
  assert.equal(A.aheadOf([{ km: 0.5, s: 100 }, { km: 0.3, s: 60, type: 8, alert: "Continue." }], 1, 5), null);
  assert.equal(A.aheadOf(steps(600), 0, 5), null);
  assert.equal(A.aheadOf(steps(600, ""), 1, 5), null);
});

test("on foot: 60 m ahead, the turn 10–15 m before it", () => {
  A.set("walk");
  assert.equal(A.aheadOf(steps(300), 1, 1.4).at, 60);
  assert.equal(A.aheadOf(steps(25), 1, 1.4), null);
  assert.equal(A.preAt(1.4), 10);
});

test("by car: by the speed of the road (town 200 m, country road 500 m, motorway 1 km)", () => {
  A.set("car");
  assert.equal(A.aheadOf(steps(2000, "Turn left.", 2000 / 12), 1, 30).at, 200); // the road, not the speed now
  assert.equal(A.aheadOf(steps(2000, "Turn left.", 2000 / 20), 1, 0).at, 500);
  assert.deepEqual(A.aheadOf(steps(5000, "Take the A1 exit.", 5000 / 30), 1, 0), { at: 1000, text: "In 1 kilometer, take the A1 exit." });
  assert.equal(A.aheadOf(steps(150, "Turn left.", 150 / 12), 1, 0), null);
  assert.equal(A.preAt(14), 84);
  assert.equal(A.preAt(33), 198);
});

test("the distance in the language of the steps", () => {
  A.set("bike", "de");
  assert.equal(A.aheadOf(steps(600, "Rechts auf Seilergraben abbiegen."), 1, 5).text, "In 150 Metern: Rechts auf Seilergraben abbiegen.");
  A.set("bike", "fr");
  assert.equal(A.aheadOf(steps(600, "Tournez à droite dans Seilergraben."), 1, 5).text, "Dans 150 mètres, tournez à droite dans Seilergraben.");
  A.set("bike", "it");
  assert.equal(A.aheadOf(steps(600, "Svolta a destra su Seilergraben."), 1, 5).text, "Tra 150 metri, svolta a destra su Seilergraben.");
  A.set("car", "de");
  assert.equal(A.aheadOf(steps(5000, "Die Ausfahrt nehmen.", 5000 / 30), 1, 0).text, "In einem Kilometer: Die Ausfahrt nehmen.");
  A.set("car", "fr");
  assert.equal(A.aheadOf(steps(5000, "Prenez la sortie.", 5000 / 30), 1, 0).text, "Dans 1 kilomètre, prenez la sortie.");
});

test("the speed: of the device, a third of it in Preview, else of the mode", () => {
  A.set("bike", "en", { on: true, sim: false, speed: 6 });
  assert.equal(A.sayV({ km: 1, s: 200 }), 6);
  A.set("bike", "en", { on: true, sim: true, speed: 15 });
  assert.equal(A.sayV({ km: 1, s: 200 }), 5);
  A.set("bike", "en", { on: true, sim: false, speed: 0 });
  assert.equal(A.sayV({ km: 1, s: 200 }), 5);
  A.set("car", "en", { on: true, sim: false, speed: 5 });
  assert.equal(A.sayV({ km: 1, s: 50 }), 20);
});
