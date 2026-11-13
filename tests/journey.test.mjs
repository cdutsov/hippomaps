// Journeys by public transport (the mode Transit, web/app.js): the line of a leg, a delay, the parts of a
// connection in its row, the time chosen on the clocks of the journey, the colour of the text on a line, and the
// texts in each language.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/journey.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { texts } from "../web/i18n.js";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const fn = (name) => {
  const a = src.indexOf(`function ${name}(`), b = src.indexOf("\n}\n", a);
  assert.ok(a > 0 && b > a, name);
  return `${src.slice(a, b)}\n}\n`;
};
const { decodeShape, lateMin, tripParts, wallTime, journeyWhen, inkOn } = new Function(
  `${["decodeShape", "lateMin", "tripParts", "wallTime", "journeyWhen", "inkOn"].map(fn).join("")}
  return { decodeShape, lateMin, tripParts, wallTime, journeyWhen, inkOn };`)();

// the inverse of decodeShape, as the servers write a line
function encode(pts, p) {
  const f = 10 ** p;
  let out = "", plat = 0, plon = 0;
  const num = (v) => {
    v = v < 0 ? ~(v << 1) : v << 1;
    while (v >= 32) { out += String.fromCharCode((32 | (v & 31)) + 63); v >>>= 5; }
    out += String.fromCharCode(v + 63);
  };
  for (const [lon, lat] of pts) {
    const la = Math.round(lat * f), lo = Math.round(lon * f);
    num(la - plat); num(lo - plon);
    plat = la; plon = lo;
  }
  return out;
}

test("the line of a leg: an encoded polyline of 5, 6 or 7 decimals as [lon, lat]", () => {
  // the example of the format: 5 decimals
  assert.deepEqual(decodeShape("_p~iF~ps|U_ulLnnqC_mqNvxq`@", 5), [[-120.2, 38.5], [-120.95, 40.7], [-126.453, 43.252]]);
  const pts = [[8.540312, 47.377923], [8.536717, 47.378456], [7.439121, 46.949005], [-0.000001, -0.000002], [23.3219, 42.6977]];
  for (const p of [6, 7]) {
    const got = decodeShape(encode(pts, p), p);
    assert.equal(got.length, pts.length);
    got.forEach((g, i) => assert.ok(Math.abs(g[0] - pts[i][0]) < 1e-6 && Math.abs(g[1] - pts[i][1]) < 1e-6, `${p}: ${g} / ${pts[i]}`));
  }
  assert.deepEqual(decodeShape("", 6), []);
  assert.deepEqual(decodeShape("_p~iF~ps|U", 0), decodeShape("_p~iF~ps|U", 5), "no precision: 5 decimals");
});

test("a delay: the minutes after the time of the timetable, none without one or on time", () => {
  assert.equal(lateMin("2026-11-13T08:05:00Z", "2026-11-13T08:02:00Z"), 3);
  assert.equal(lateMin("2026-11-13T08:02:40Z", "2026-11-13T08:02:00Z"), 0);
  assert.equal(lateMin("2026-11-13T08:02:00Z", "2026-11-13T08:02:00Z"), 0);
  assert.equal(lateMin("2026-11-13T08:00:00Z", "2026-11-13T08:02:00Z"), 0, "early");
  assert.equal(lateMin("2026-11-13T08:05:00Z", undefined), 0);
  assert.equal(lateMin("2026-11-13T08:05:00Z", ""), 0);
});

test("the row of a connection: its rides, a walk at an end from three minutes, no change", () => {
  const at = { t: "2026-11-13T08:05:00Z", plan: "2026-11-13T08:02:00Z" }, on = { t: "2026-11-13T09:00:00Z" };
  const legs = [
    { k: "walk", s: 120, from: on, to: on },
    { k: "rail", name: "IC1", colour: "#eb0000", text: "#ffffff", from: at, to: on },
    { k: "walk", s: 600, from: on, to: on }, // a change of ten minutes
    { k: "tram", name: "7", from: on, to: on, cancelled: true },
    { k: "walk", s: 180, from: on, to: on },
  ];
  assert.deepEqual(tripParts(legs), [
    { k: "rail", name: "IC1", colour: "#eb0000", text: "#ffffff", late: 3, cancelled: false },
    { k: "tram", name: "7", colour: "", text: "", late: 0, cancelled: true },
    { k: "walk", min: 3 },
  ]);
  assert.deepEqual(tripParts([{ k: "walk", s: 700, from: on, to: on }, { k: "bus", from: on, to: on }]).map((p) => p.k), ["walk", "bus"]);
});

test("a date and time chosen is on the clocks of the journey, whatever the clock of the device", () => {
  assert.equal(wallTime("2026-11-14T08:30", "Europe/Zurich").toISOString(), "2026-11-14T07:30:00.000Z");
  assert.equal(wallTime("2026-07-01T08:30", "Europe/Zurich").toISOString(), "2026-07-01T06:30:00.000Z", "summer time");
  assert.equal(wallTime("2026-11-14T08:30", "Europe/Sofia").toISOString(), "2026-11-14T06:30:00.000Z");
  assert.equal(wallTime("2026-11-14T08:30", "America/New_York").toISOString(), "2026-11-14T13:30:00.000Z");
  assert.equal(wallTime("2026-11-14T00:10", "Europe/Zurich").toISOString(), "2026-11-13T23:10:00.000Z", "the day before in UTC");
  // without a zone, or one unknown: the clock of the device
  assert.equal(wallTime("2026-11-14T08:30", "").getTime(), new Date("2026-11-14T08:30").getTime());
  assert.equal(wallTime("2026-11-14T08:30", "Mars/Olympus").getTime(), new Date("2026-11-14T08:30").getTime());
  assert.ok(Number.isNaN(wallTime("soon", "Europe/Zurich").getTime()));
});

test("the time of the question: none to leave now, the departure, the arrival", () => {
  assert.deepEqual(journeyWhen("now", "2026-11-14T08:30", "Europe/Zurich"), {});
  assert.deepEqual(journeyWhen("depart", "", "Europe/Zurich"), {});
  assert.deepEqual(journeyWhen("depart", "nonsense", "Europe/Zurich"), {});
  assert.deepEqual(journeyWhen("depart", "2026-11-14T08:30", "Europe/Zurich"), { time: "2026-11-14T07:30:00Z" });
  assert.deepEqual(journeyWhen("arrive", "2026-11-14T08:30", "Europe/Sofia"), { time: "2026-11-14T06:30:00Z", arrive: "1" });
});

test("the text on the colour of a line: dark on a light colour, white on the others", () => {
  for (const c of ["#ffdd00", "#ffffff", "#9be36d", "#f8c300"]) assert.equal(inkOn(c), "#111827", c);
  for (const c of ["#1c75bc", "#eb0000", "#000000", "#6a1b9a", "#00796b", "#c62828", "#ef6c00"]) assert.equal(inkOn(c), "#fff", c);
  for (const c of ["", "red", "#fff", undefined]) assert.equal(inkOn(c), "#fff", String(c));
});

test("the mode Transit and the texts of a journey in each language", () => {
  const start = src.indexOf("const EN = {"), end = src.indexOf("\n};\n", start) + 3;
  const EN = new Function("num", `${src.slice(start, end)}; return EN;`)(String);
  const all = { en: EN, ...texts(String) };
  for (const [lang, t] of Object.entries(all)) {
    assert.deepEqual(t.modes.map(([m]) => m), ["car", "bike", "walk", "transit"], `${lang} modes`);
    assert.equal(t.modes[3][1], t.transit, `${lang}: the mode named as the layer of the lines`);
    assert.match(t.noStops(true, "Schweiz, Bulgarien"), /Schweiz, Bulgarien/, `${lang} noStops`);
    assert.notEqual(t.noStops(true, ""), t.noStops(false, ""), `${lang}: start and destination apart`);
    assert.ok(!/undefined|: \.$/.test(t.noStops(false, "")), `${lang} noStops without regions`);
    assert.match(t.noJourney("Suisse"), /Suisse/, `${lang} noJourney`);
    assert.notEqual(t.platform("7", true), t.platform("7", false), `${lang}: a track and a stand apart`);
    assert.notEqual(t.changes(0), t.changes(1));
    assert.notEqual(t.changes(1), t.changes(2));
    assert.match(t.changeLeg("6 min", "170 m"), /6 min.*170 m/);
    assert.ok(!/·\s*$/.test(t.walkLeg("2 min", "")), `${lang} walkLeg without a distance`);
    if (lang === "en") continue;
    for (const k of ["leaveNow", "departAt", "arriveBy", "whenTime", "journeySearching", "later", "cancelled", "noFaster"]) assert.notEqual(t[k], EN[k], `${lang} ${k}`);
    for (const k of ["changes", "walkLeg", "changeLeg", "stopsBetween", "onFoot"]) assert.notEqual(t[k](2, "x"), EN[k](2, "x"), `${lang} ${k}`);
  }
  assert.equal(all.de.platform("7", true), "Gleis 7");
  assert.equal(all.de.platform("A", false), "Kante A");
});
