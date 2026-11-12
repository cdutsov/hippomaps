// Start and Preview (navStart in web/app.js) set navigation up in one go. A wait in the middle (it awaited the
// screen lock) let End run before the watch of the location and the timers were started: they then ran on after
// navigation, and nothing stopped them.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/nav-start.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const a = src.indexOf("\nfunction navStart(sim) {"), b = src.indexOf("\n}\n", a);

test("navStart starts the watch and the timers without a wait before them", () => {
  assert.ok(a > 0 && b > a, "navStart found, not async");
  const body = src.slice(a, b);
  assert.doesNotMatch(body, /\bawait\b/);
  // what navStop ends (stopTracking) is started here
  assert.match(body, /nav\.watch = navigator\.geolocation\.watchPosition\(onFix/);
  assert.match(body, /nav\.watchdog = setInterval\(checkFix/);
  assert.match(body, /nav\.timer = setInterval\(/);
  const stop = src.slice(src.indexOf("\nfunction stopTracking() {"), src.indexOf("\n}\n", src.indexOf("\nfunction stopTracking() {")));
  for (const k of ["clearWatch(nav.watch)", "clearInterval(nav.timer)", "clearInterval(nav.watchdog)"]) assert.ok(stop.includes(k), k);
});
// the screen lock (keepAwake): the browser answers later; navigation may be over by then
const keepAwake = (nav, wakeLock) => new Function("nav", "navigator", "document",
  `${src.slice(src.indexOf("async function keepAwake() {"), src.indexOf("\n}\n", src.indexOf("async function keepAwake() {")))}\n}\nreturn keepAwake;`)(nav, { wakeLock }, { visibilityState: "visible" });
const lockLater = () => {
  const w = { released: 0, give: null };
  const lock = { release: async () => { w.released++; }, addEventListener: (ev, fn) => { w.onRelease = fn; } };
  w.request = () => new Promise((ok) => { w.give = () => ok(lock); });
  w.lock = lock;
  return w;
};
test("the screen lock is kept while navigating, and let go by the browser", async () => {
  const nav = { on: true, lock: null }, w = lockLater();
  const done = keepAwake(nav, w)();
  w.give();
  await done;
  assert.equal(nav.lock, w.lock);
  w.onRelease(); // the page hidden
  assert.equal(nav.lock, null);
});
test("navigation ended before the browser gave the lock: the lock is released, the screen may sleep", async () => {
  const nav = { on: true, lock: null }, w = lockLater();
  const done = keepAwake(nav, w)();
  nav.on = false; // End
  w.give();
  await done;
  assert.equal(nav.lock, null);
  assert.equal(w.released, 1);
});
test("no screen lock in this browser: nothing kept", async () => {
  const nav = { on: true, lock: null };
  await keepAwake(nav, undefined)();
  assert.equal(nav.lock, null);
});
