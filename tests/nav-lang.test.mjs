// The languages of navigation (web/app.js): each has a name in the menu, a lead for a turn ahead ("In 150 meters, "),
// the sentences of the app (rerouted, GPS lost, arrived, a stop reached, no connection for a new route), a language of the steps on the server
// (server/route.go) and a voice (server/say.go).
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/nav-lang.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { texts } from "../web/i18n.js";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const part = (from, to) => {
  const a = src.indexOf(from), b = src.indexOf(to, a);
  assert.ok(a > 0 && b > a, `${from} in app.js`);
  return src.slice(a, b);
};
const top = new Function(`let LANG = "de"; ${part("const LOCALES = {", "let chosenNavLang")} return { LOCALES, NAV_NAMES };`)();
const { LOCALES, NAV_NAMES } = top;
const make = new Function(`let route = { mode: "car" }, nav = { on: false }, STEP_LANG = "en";
${part("const NAV = {", "const nav = {")}
return { SAY_IN, SPOKEN, aheadOf, set: (lang) => { STEP_LANG = lang; } };`);
const A = make();
const all = texts(String);
const go = readFileSync(new URL("../server/route.go", import.meta.url), "utf8");
const say = readFileSync(new URL("../server/say.go", import.meta.url), "utf8");
const KEYS = ["rerouted", "gpsLost", "arrived", "stopReached", "offlineRoute"];

test("each language of navigation: a name, a lead, the sentences of the app", () => {
  assert.ok(Object.keys(LOCALES).length >= 18);
  for (const l of Object.keys(LOCALES)) {
    assert.ok(NAV_NAMES[l], `${l}: name`);
    assert.ok(A.SAY_IN[l], `${l}: lead`);
    const t = A.SPOKEN[l] || all[l] || (l === "en" && {});
    assert.ok(t, `${l}: sentences`);
    if (l !== "en") for (const k of KEYS) assert.ok(t[k], `${l}: ${k}`);
  }
});

test("the languages of navigation are those of the steps and of the voices of the server", () => {
  const steps = new Set([...go.matchAll(/"([a-z]{2})": "[a-z]{2}-[A-Z]{2}"/g)].map((m) => m[1]));
  const voices = new Set([...say.slice(say.indexOf("var sayLangs")).split("\n\n")[0].matchAll(/"([a-z]{2})": true/g)].map((m) => m[1]));
  for (const l of Object.keys(LOCALES)) {
    assert.ok(steps.has(l), `${l}: steps on the server`);
    assert.ok(voices.has(l), `${l}: voice on the server`);
  }
  assert.deepEqual([...voices].sort(), Object.keys(LOCALES).sort());
});

test("leads with the plural of round numbers", () => {
  const L = A.SAY_IN;
  assert.equal(L.bg("150", false), "След 150 метра, ");
  assert.equal(L.bg("1", true), "След 1 километър, ");
  assert.equal(L.ru("300", false), "Через 300 метров ");
  assert.equal(L.ru("1", true), "Через 1 километр ");
  assert.equal(L.uk("50", false), "Через 50 метрів ");
  assert.equal(L.pl("100", false), "Za 100 metrów ");
  assert.equal(L.ro("10", false), "Peste 10 metri, ");
  assert.equal(L.ro("150", false), "Peste 150 de metri, ");
  assert.equal(L.ro("1", true), "Peste un kilometru, ");
  assert.equal(L.fi("200", false), "200 metrin päästä ");
  assert.equal(L.hu("200", false), "200 méter múlva ");
  assert.equal(L.el("1", true), "Σε 1 χιλιόμετρο, ");
});

test("a turn ahead in Bulgarian: the lead, then the instruction of Valhalla in small letters", () => {
  const B = make();
  B.set("bg");
  const steps = [{ km: 0.6, s: 60, type: 8 }, { km: 0.3, s: 60, type: 10, alert: "Завийте надясно по Витоша." }];
  assert.deepEqual(B.aheadOf(steps, 1, 10), { at: 200, text: "След 200 метра, завийте надясно по Витоша." });
});
