// The back button of a place card: "Here" only when the card was opened from a lookup page the visitor asked for.
// A name tapped on the map opens the lookup and then the card at once; its card must not offer "Here".
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/back-label.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");

test("the card of a tapped name is selected with its own origin, not inferred from the lookup page", () => {
  assert.match(src, /select\(hit, \{ push: false, fly: false, from: "tap"(, [a-zA-Z]+: [a-z]+)* \}\)/);
  assert.match(src, /state\.from = from \|\| \(hereView\.hidden \? "list" : "here"\)/);
});

test("the back button of a tapped card closes it when there are no results to go back to", () => {
  assert.match(src, /state\.from === "tap" && !state\.answer \? T\.close : T\.back/);
  assert.match(src, /if \(state\.from === "tap" && !state\.answer\) return closeCard\(\)/);
});
