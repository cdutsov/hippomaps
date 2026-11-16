// Measure (web/app.js): the length of a line and the area of a shape on the earth, their texts, a line thinned, the
// heights at its points from a profile, the line written as GPX and KML and read from GPX, KML and GeoJSON files.
//   docker run --rm -v "$PWD":/w:ro -w /w node:24.21.0-alpine3.23 node --test tests/measure.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const src = readFileSync(new URL("../web/app.js", import.meta.url), "utf8");
const body = (name) => {
  const a = src.indexOf(`function ${name}(`), b = src.indexOf("\n}\n", a);
  assert.ok(a > 0 && b > a, name);
  return src.slice(a, b + 2);
};
const names = ["measureKm", "ringArea", "fmtDist", "fmtArea", "thinLine", "elesAt", "xmlText", "xmlRead", "dataGPX", "dataKML", "readTrack"];
const F = new Function(`${names.map(body).join("\n")}\nreturn { ${names.join(", ")} };`)();
const close = (got, want, tol, what) => assert.ok(Math.abs(got - want) <= tol, `${what}: ${got}, not ${want} ± ${tol}`);

test("the length of a line on the earth", () => {
  close(F.measureKm([[8.5417, 47.3769], [7.4474, 46.948]]), 95.5, 0.5, "Zurich to Bern");
  close(F.measureKm([[0, 0], [1, 0]]), 111.195, 0.01, "a degree at the equator");
  close(F.measureKm([[0, 0], [1, 0], [1, 1]]), 222.39, 0.05, "two parts");
  // closed: back to the first point
  close(F.measureKm([[0, 0], [1, 0], [1, 1]], true), 222.39 + 157.25, 0.1, "closed");
  assert.equal(F.measureKm([[8, 47]]), 0);
});
test("the area of a shape: a square of 100 m, a degree at the equator, the last point repeated or not", () => {
  const d = 100 / 111320; // 100 m in degrees at the equator
  close(F.ringArea([[0, 0], [d, 0], [d, d], [0, d]]), 10000, 30, "100 m square");
  close(F.ringArea([[0, 0], [d, 0], [d, d], [0, d], [0, 0]]), 10000, 30, "closed ring");
  close(F.ringArea([[0, 0], [0, d], [d, d], [d, 0]]), 10000, 30, "the other way round");
  close(F.ringArea([[0, 0], [1, 0], [1, 1], [0, 1]]) / 1e6, 12391, 5, "a degree at the equator, km² (the sphere of WGS84, as turf)");
});
test("the texts of a length and an area", () => {
  assert.equal(F.fmtDist(0.0374), "37 m");
  assert.equal(F.fmtDist(0.999), "999 m");
  assert.equal(F.fmtDist(2.3456), "2.35 km");
  assert.equal(F.fmtDist(42.34), "42.3 km");
  assert.equal(F.fmtDist(312.6), "313 km");
  assert.equal(F.fmtArea(850.4), "850 m²");
  assert.equal(F.fmtArea(24000), "2.40 ha");
  assert.equal(F.fmtArea(450000), "45.0 ha");
  assert.equal(F.fmtArea(3.2e6), "3.20 km²");
  assert.equal(F.fmtArea(4.1e9), "4100 km²");
});
test("a line thinned keeps its ends", () => {
  const line = Array.from({ length: 10001 }, (_, i) => [i, 0]);
  const t = F.thinLine(line, 5000);
  assert.equal(t.length, 5000);
  assert.deepEqual(t[0], [0, 0]);
  assert.deepEqual(t[4999], [10000, 0]);
  assert.equal(F.thinLine(line.slice(0, 10), 5000).length, 10);
});
test("the heights at the points of a line from its profile", () => {
  const line = [[0, 0], [0.01, 0], [0.02, 0]]; // two parts of 1.112 km
  const elev = { pts: [[0, 100], [1.112, 200], [2.224, 150]] };
  const h = F.elesAt(line, elev);
  assert.equal(h.length, 3);
  close(h[0], 100, 0.01, "start");
  close(h[1], 200, 0.5, "middle");
  close(h[2], 150, 0.01, "end");
  assert.equal(F.elesAt(line, null), null);
  assert.equal(F.elesAt([[0, 0]], elev), null);
});
test("GPX: a track with heights, waypoints, names as text, the credit of OpenStreetMap when asked", () => {
  const time = new Date("2026-11-16T10:00:00.123Z");
  const g = F.dataGPX({ name: "Zürich <& Bern>", line: [[8.5417, 47.3769], [7.4474, 46.948]], eles: [408.04, 540], time,
    wpts: [{ ll: [8.1, 47.1], name: "Café \"Bär\"" }], osm: true });
  assert.match(g, /^<\?xml version="1.0" encoding="UTF-8"\?>\n<gpx version="1.1" creator="hippomaps" xmlns="http:\/\/www.topografix.com\/GPX\/1\/1">/);
  assert.match(g, /<name>Zürich &lt;&amp; Bern&gt;<\/name>/);
  assert.match(g, /<time>2026-11-16T10:00:00Z<\/time>/);
  assert.match(g, /<wpt lat="47.1" lon="8.1"><name>Café &quot;Bär&quot;<\/name><\/wpt>/);
  assert.match(g, /<trkpt lat="47.3769" lon="8.5417"><ele>408.0<\/ele><\/trkpt>\n<trkpt lat="46.948" lon="7.4474"><ele>540.0<\/ele><\/trkpt>/);
  assert.match(g, /<copyright author="OpenStreetMap contributors">/);
  assert.ok(!/copyright/.test(F.dataGPX({ line: [[0, 0], [1, 1]], time })), "a line drawn has no credit");
  // a shape: back to its first point
  const s = F.dataGPX({ line: [[0, 0], [1, 0], [1, 1]], closed: true, time });
  assert.equal(s.match(/<trkpt /g).length, 4);
  // waypoints before the track (GPX 1.1 order)
  assert.ok(g.indexOf("<wpt") < g.indexOf("<trk>"));
});
test("KML: a line, and a shape as a polygon", () => {
  const k = F.dataKML({ name: "A & B", line: [[8.5, 47.3], [8.6, 47.4]], eles: [400, 500.25] });
  assert.match(k, /<LineString><coordinates>8.5,47.3,400.0 8.6,47.4,500.3<\/coordinates><\/LineString>/);
  assert.match(k, /<name>A &amp; B<\/name>/);
  const p = F.dataKML({ line: [[0, 0], [1, 0], [1, 1]], closed: true });
  assert.match(p, /<Polygon><outerBoundaryIs><LinearRing><coordinates>0,0 1,0 1,1 0,0<\/coordinates>/);
});
test("files read: GPX tracks, routes and waypoints; KML lines, shapes and gx:Track; GeoJSON lines, areas and points", () => {
  const gpx = `﻿<?xml version="1.0"?><gpx xmlns="http://www.topografix.com/GPX/1/1"><metadata><name>Uetliberg &amp; back</name></metadata>
    <trk><trkseg><trkpt lat="47.35" lon="8.49"><ele>870</ele></trkpt><trkpt lon='8.5' lat='47.36'/></trkseg>
    <trkseg><trkpt lat="47.37" lon="8.51"></trkpt></trkseg></trk></gpx>`;
  assert.deepEqual(F.readTrack(gpx), { pts: [[8.49, 47.35], [8.5, 47.36], [8.51, 47.37]], closed: false, name: "Uetliberg & back" });
  const rte = `<gpx><rte><name><![CDATA[Route]]></name><rtept lat="1" lon="2"/><rtept lat="3" lon="4"/></rte></gpx>`;
  assert.deepEqual(F.readTrack(rte), { pts: [[2, 1], [4, 3]], closed: false, name: "Route" });
  const wpt = `<gpx:gpx xmlns:gpx="x"><gpx:wpt lat="1" lon="2"/><gpx:wpt lat="1.5" lon="2.5"/></gpx:gpx>`;
  assert.deepEqual(F.readTrack(wpt).pts, [[2, 1], [2.5, 1.5]]);
  const kml = `<kml xmlns="http://www.opengis.net/kml/2.2"><Document><name>Walk</name><Placemark><LineString><coordinates>
    8.5,47.3,400 8.6,47.4,500
    </coordinates></LineString></Placemark></Document></kml>`;
  assert.deepEqual(F.readTrack(kml), { pts: [[8.5, 47.3], [8.6, 47.4]], closed: false, name: "Walk" });
  const ring = `<kml><Placemark><Polygon><outerBoundaryIs><LinearRing><coordinates>0,0 1,0 1,1 0,0</coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark></kml>`;
  assert.deepEqual(F.readTrack(ring), { pts: [[0, 0], [1, 0], [1, 1]], closed: true, name: "" });
  const gx = `<kml xmlns:gx="http://www.google.com/kml/ext/2.2"><gx:Track><gx:coord>8.5 47.3 400</gx:coord><gx:coord>8.6 47.4 410</gx:coord></gx:Track></kml>`;
  assert.deepEqual(F.readTrack(gx).pts, [[8.5, 47.3], [8.6, 47.4]]);
  const line = JSON.stringify({ type: "FeatureCollection", features: [{ type: "Feature", properties: { name: "L" }, geometry: { type: "LineString", coordinates: [[8, 47, 400], [9, 48]] } }] });
  assert.deepEqual(F.readTrack(line), { pts: [[8, 47], [9, 48]], closed: false, name: "L" });
  const poly = JSON.stringify({ type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 0]]] });
  assert.deepEqual(F.readTrack(poly), { pts: [[0, 0], [1, 0], [1, 1]], closed: true, name: "" });
  const points = JSON.stringify({ type: "FeatureCollection", features: [[1, 2], [3, 4]].map((c) => ({ type: "Feature", properties: {}, geometry: { type: "Point", coordinates: c } })) });
  assert.deepEqual(F.readTrack(points).pts, [[1, 2], [3, 4]]);
  // nothing to read, or points out of the world
  for (const bad of ["", "hello", "{nonsense", "<gpx></gpx>", "<html><body>x</body></html>", '{"type":"FeatureCollection","features":[]}',
    '<gpx><trkpt lat="95" lon="8"/><trkpt lat="91" lon="8"/></gpx>']) assert.equal(F.readTrack(bad), null, JSON.stringify(bad));
});
test("a GPX and a KML written are read back as the same line", () => {
  const line = [[8.5417123, 47.3769456], [8.55, 47.38], [8.56, 47.37]];
  const g = F.readTrack(F.dataGPX({ name: "x", line, eles: [1, 2, 3] }));
  assert.deepEqual(g.pts, line);
  const k = F.readTrack(F.dataKML({ name: "y", line, closed: true }));
  assert.deepEqual(k, { pts: line, closed: true, name: "y" });
});
