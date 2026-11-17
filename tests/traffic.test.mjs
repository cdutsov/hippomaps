// Traffic of several countries: the switch of the menu names them (from data-traffic-countries, in the language of the
// page), and a message names its source and the time of its data (the credit the providers ask for).
// node --test tests/traffic.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const body = src.slice(src.indexOf("function trafficLabel() {"), src.indexOf("\n}\n", src.indexOf("function trafficLabel() {")) + 2);

function label(countries, UI) {
  const root = { dataset: countries == null ? {} : { trafficCountries: countries } };
  const T = { traffic: UI === "de" ? "Verkehr" : "Traffic" };
  return new Function("root", "T", "UI", `${body}; return trafficLabel();`)(root, T, UI);
}

test("the label of Traffic names the countries by name, in order, in the language of the page", () => {
  assert.equal(label("CH,FR,NL", "en"), "Traffic (France, Netherlands, Switzerland)");
  assert.equal(label("CH,FR,NL", "de"), "Verkehr (Frankreich, Niederlande, Schweiz)");
  assert.equal(label("CH", "en"), "Traffic (Switzerland)");
  assert.equal(label("", "en"), "Traffic"); // not known yet: the page goes without them
  assert.equal(label(null, "en"), "Traffic");
});

test("a message: text in the page language or any, its road, length and delay, and its source with the time", () => {
  assert.match(src, /it\.text\?\.\[UI\] \|\| it\.text\?\.en \|\| Object\.values\(it\.text \|\| \{\}\)\[0\]/);
  assert.match(src, /\[it\.road, it\.length \? fmtDist\(it\.length \/ 1000\) : "", it\.delay >= 60 \? T\.delay\(/);
  assert.match(src, /const src = trafficSources\[it\?\.src \|\| f\.properties\.src\];/);
  assert.match(src, /`\$\{src\.credit\}, \$\{when\}`/);
  assert.match(src, /trafficSources = d\.sources \|\| \{\};/);
});

test("a new language writes the label again (it is not one of the texts)", () => {
  assert.match(src, /if \(TRAFFIC\) trafficRow\.querySelector\("\.lm-toggle-text > span"\)\.textContent = trafficLabel\(\);/);
});
