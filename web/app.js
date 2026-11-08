// hippomaps: a search box with suggestions, the results and the page of a place in a
// side bar, and the map beside it. The search is the API of overpass-search (data-api):
// /complete while typing, /query for the reading, /search for the results. Pictures and
// logos come from data-pictures, which reads Wikimedia Commons. Directions come from
// data-route (Valhalla, through this server) when the server has them. Text from OpenStreetMap
// only ever goes in as text, never as HTML.
import * as maplibregl from "./vendor/maplibre-gl-6.13.0/maplibre-gl.mjs";

maplibregl.setWorkerCount(Math.max(maplibregl.getWorkerCount(), 2));

const root = document.getElementById("app");
const side = root.querySelector(".lm-side");
const mapEl = root.querySelector(".lm-map");
const API = root.dataset.api.replace(/\/$/, "");
const PICTURES = root.dataset.pictures || "";
const ROUTE = root.dataset.route || "";
const LANG = (navigator.language || "en").slice(0, 2).toLowerCase();
const placeKeys = ["place", "place2", "region"];
const extraKeys = [...placeKeys, "what", "view"]; // view=1: the same search in the map view

const T = {
  placeholder: "Search places, or what and where",
  examples: ["cafés", "Migros", "museums in Bern", "playgrounds near me", "restaurants open now", "hotels along the Limmat in Zurich"],
  intro: "Search in plain words: a place, a chain, or what and where. A word alone searches the map view.",
  searching: "Searching…",
  locating: "Finding your location…",
  results: (n) => (n === 1 ? "1 result" : `${n.toLocaleString("en")} results`),
  shown: (n, all) => `${n.toLocaleString("en")} of ${all.toLocaleString("en")} shown`,
  more: "Show more",
  noName: "Unnamed",
  back: "Results",
  searchArea: "Search this area",
  myLocation: "Show my location",
  north: "Turn the map to north",
  inArea: (name, kind) => `In ${name}${kind ? ` (${kind})` : ""}`,
  here: "Here",
  around: "Around here",
  hereLooking: "Looking at this point…",
  mapLost: "The map stopped drawing in this browser: it took the graphics back. Reload the page; with 3D off the map needs less memory.",
  hereNothing: "Nothing with a name right here. Zoom in closer, or tap a symbol of the map.",
  locBlocked: "Location is blocked for this page. Allow it in the settings of the browser, then try again.",
  locFailed: "Your location could not be found.",
  locMissing: "This browser does not share a location.",
  otherReadings: "Or search for",
  otherPlaces: "Other places",
  tryHere: "Search here",
  query: "Overpass query",
  copy: "Copy",
  copied: "Copied",
  open: "Open",
  closed: "Closed",
  opens: (t) => `opens ${t}`,
  closes: (t) => `closes ${t}`,
  failed: "The search failed. Please try again.",
  tooLong: "The search took too long. Try a smaller place, or a less common feature.",
  errors: {
    zoom_in: "Zoom in to search the map view, or add a place to the search.",
    need_location: "Share your location, or zoom the map to where you are, or add a place to the search.",
  },
  directions: "Directions",
  from: "From",
  to: "To",
  swap: "Swap start and destination",
  myPlace: "Your location",
  mapPoint: "Point on the map",
  tapStart: "Tap the map to set the start, or use your location.",
  useLocation: "Use my location",
  routing: "Finding the route…",
  modes: [["car", "Car"], ["bike", "Bike"], ["walk", "Walk"]],
  routeFailed: "The route could not be found. Please try again.",
  start: "Start",
  preview: "Preview",
  previewNote: "A drive along the route, three times as fast",
  endNav: "End navigation",
  mute: "Sound off",
  unmute: "Sound on",
  recenter: "Re-centre",
  overview: "Whole route",
  arrival: "Arrival",
  left: "left",
  rerouting: "New route…",
  arrived: "You have arrived.",
  noGPS: "Navigation needs your location. Allow it in the settings of the browser, then try again.",
  waitGPS: "Waiting for your location…",
  then: "Then",
  exit: "Exit",
  speedLimit: (v) => `Speed limit ${v}`,
  heights: "Height profile",
  up: (m) => `${m.toLocaleString("en")} m up`,
  down: (m) => `${m.toLocaleString("en")} m down`,
  climbLeft: (m) => `${m.toLocaleString("en")} m still to climb`,
  duration: (s) => {
    const m = Math.max(1, Math.round(s / 60));
    return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ""}`;
  },
  website: "Website",
  call: "Call",
  osm: "OpenStreetMap",
  allTags: "All tags",
  photo: "Photo",
  logo: "Logo",
  menu: "Menu",
  closeMenu: "Close the menu",
  mapStyle: "Map style",
  show: "Show",
  threeD: "3D",
  threeDNote: "Terrain and buildings; tilt with two fingers or the right mouse button",
  shading: "Relief shading",
  shadingNote: "Light and shadow of the mountains",
  styles: [["light", "Light"], ["dark", "Dark"], ["outdoor", "Outdoor"], ["relief", "Relief"], ["driving", "Driving"],
    ["marine", "Marine"], ["white", "White"], ["grayscale", "Grayscale"], ["black", "Black"]],
  credit: "Map data © OpenStreetMap contributors (ODbL). Pictures from Wikimedia Commons. Chains from the name-suggestion-index.",
  voiceCredit: "Voices by Piper; the French voice from the SIWIS database (CC BY 4.0).",
};

// ------------------------------------------------------------------ helpers
const el = (tag, cls, attrs = {}, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (text != null) e.textContent = text;
  return e;
};
const svg = (d, size = 18) =>
  `<svg viewBox="0 0 24 24" width="${size}" height="${size}" aria-hidden="true"><path d="${d}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
const ICON = {
  search: "M10.5 17a6.5 6.5 0 1 1 0-13 6.5 6.5 0 0 1 0 13zM15.5 15.5 21 21",
  back: "M15 18l-6-6 6-6",
  close: "M18 6 6 18M6 6l12 12",
  pin: "M12 21s-7-6.2-7-11.2A7 7 0 0 1 19 9.8C19 14.8 12 21 12 21zM12 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 2",
  phone: "M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18",
  mail: "M4 6h16v12H4zM4 7l8 6 8-6",
  info: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 11v6M12 7.5v.5",
  route: "M6 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM18 9a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM6 15V9a3 3 0 0 1 3-3h6",
  map: "M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2zM9 4v14M15 6v14",
  wheel: "M12 6a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM11 8v6h5l2 5M8 11a5 5 0 1 0 6 7",
  food: "M7 3v8M5 3v5a2 2 0 0 0 4 0V3M7 11v10M17 3c-2 0-3 2-3 5s1 4 3 4v9",
  book: "M5 4h10a4 4 0 0 1 4 4v12H9a4 4 0 0 1-4-4zM5 16a4 4 0 0 1 4-4h10",
  burger: "M4 7h16M4 12h16M4 17h16",
  locate: "M12 19a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM12 2v3M12 19v3M2 12h3M19 12h3",
  refresh: "M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7",
  navigate: "M12 2 20 21l-8-4-8 4z",
  sound: "M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12",
  muted: "M4 9v6h4l5 4V5L8 9zM17 9l5 6M22 9l-5 6",
  overview: "M4 6h16M4 12h10M4 18h6",
  car: "M5 17h14v-5l-2-5H7l-2 5zM5 12h14M7.5 17v2M16.5 17v2M8 14.5h.01M16 14.5h.01",
  bike: "M5.5 18a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM18.5 18a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM5.5 14.5 9 8h6l3.5 6.5M9 8l3 6.5 3-6.5M8 5h3",
  walk: "M13 5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM10 21l2-6 3 3v3M12 15l-1-6 4 2 2 3M11 9l-3 2-1 3",
  swap: "M7 4v16M7 4 4 7M7 4l3 3M17 20V4M17 20l-3-3M17 20l3-3",
};
const human = (v) => (v ? v.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase()) : "");
const tagsOf = (p) => (typeof p.tags === "string" ? JSON.parse(p.tags) : p.tags) || {};
const kindOf = (p) => {
  const [k, v] = (p.type || "").split("=");
  return { key: k || "", label: v && v !== "yes" ? human(v) : human(k) };
};
const KEY_COLOURS = { amenity: "#e8590c", shop: "#7048e8", tourism: "#1c7ed6", leisure: "#2f9e44", historic: "#a05a2c", natural: "#2b8a3e",
  highway: "#495057", railway: "#495057", public_transport: "#1971c2", office: "#4c6ef5", craft: "#b7791f", healthcare: "#e03131", building: "#868e96" };
const addressOf = (t) => {
  const street = [t["addr:street"] || t["addr:place"], t["addr:housenumber"]].filter(Boolean).join(" ");
  const town = [t["addr:postcode"], t["addr:city"]].filter(Boolean).join(" ");
  return [street, town].filter(Boolean).join(", ");
};
const safeURL = (v) => {
  if (!v) return "";
  const s = v.split(";")[0].trim();
  try {
    const u = new URL(/^https?:\/\//i.test(s) ? s : `https://${s}`);
    return u.protocol === "https:" || u.protocol === "http:" ? u.href : "";
  } catch { return ""; }
};
const shortURL = (u) => u.replace(/^https?:\/\/(www\.)?/, "").replace(/\/$/, "");
const fold = (s) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
const center = (f) => {
  if (f.geometry.type === "Point") return f.geometry.coordinates;
  const b = extent(f.geometry);
  return b ? [(b[0] + b[2]) / 2, (b[1] + b[3]) / 2] : null;
};
function extent(g) {
  let w = 180, s = 90, e = -180, n = -90;
  const walk = (c) => {
    if (typeof c[0] === "number") { w = Math.min(w, c[0]); e = Math.max(e, c[0]); s = Math.min(s, c[1]); n = Math.max(n, c[1]); }
    else c.forEach(walk);
  };
  walk(g.coordinates);
  return w <= e ? [w, s, e, n] : null;
}
const km = (a, b) => {
  const r = Math.PI / 180, x = (b[0] - a[0]) * r * Math.cos(((a[1] + b[1]) / 2) * r), y = (b[1] - a[1]) * r;
  return Math.sqrt(x * x + y * y) * 6371;
};
const fmtKm = (d) => (d < 1 ? `${Math.round(d * 100) * 10} m` : `${d.toFixed(d < 10 ? 1 : 0)} km`);

// ------------------------------------------------------------------ the map style
// The style of the map, 3D (terrain and buildings) and relief shading, kept in the browser. The
// page is dark with the dark and black styles; the first style follows the theme of the site.
const store = { get: (k) => { try { return localStorage.getItem(k); } catch { return null; } }, set: (k, v) => { try { localStorage.setItem(k, v); } catch {} } };
const look = {
  style: T.styles.some(([k]) => k === store.get("hm-style")) ? store.get("hm-style") : store.get("theme") === "dark" ? "dark" : "light",
  threeD: store.get("hm-3d") === "1",
  shading: store.get("hm-shading") === "1",
};
const theme = () => (look.style === "dark" || look.style === "black" ? "dark" : "light");
document.documentElement.dataset.theme = theme();
const styleURL = () => {
  const p = new URLSearchParams({ lang: ["de", "fr", "it"].includes(LANG) ? LANG : "en" });
  if (look.shading) p.set("hillshade", "1");
  if (look.threeD) p.set("terrain", "1");
  if (look.threeD && look.style !== "relief") p.set("buildings", "1");
  return `${root.dataset.tiles}/v1/${root.dataset.key}/maps/styles/${look.style}.json?${p}`;
};

// ------------------------------------------------------------------ the side bar
const top = el("div", "lm-top");
const logo = () => {
  const a = el("a", "lm-brand", { href: location.pathname });
  // the name as overpass.ch writes its own: "maps" in the blue of ".ch"
  const word = el("span", "lm-word", { "aria-label": "hippomaps" });
  word.append(el("span", "", {}, "hippo"), el("span", "lm-word-maps", {}, "maps"));
  a.append(el("img", "lm-logo-img", { src: root.dataset.logo, alt: "", width: "28", height: "28" }), word);
  return a;
};
const tag = el("span", "lm-tag", {}, "Labs");
const burger = () => {
  const b = el("button", "lm-icon-btn lm-burger", { type: "button", title: T.menu, "aria-label": T.menu, "aria-controls": "lm-menu", "aria-expanded": "false" });
  b.innerHTML = svg(ICON.burger, 22);
  return b;
};
const menuBtn = burger();
const sheetBtn = el("button", "lm-sheet-btn", { type: "button", "aria-label": "Show or hide the list" });
top.append(menuBtn, logo(), tag);

const form = el("form", "lm-form", { role: "search" });
const combo = el("div", "lm-combo");
const input = el("input", "lm-input", { type: "search", role: "combobox", "aria-autocomplete": "list", "aria-expanded": "false",
  "aria-controls": "lm-list", "aria-label": T.placeholder, placeholder: T.placeholder, autocomplete: "off", spellcheck: "false", maxlength: "160", enterkeyhint: "search" });
const clearBtn = el("button", "lm-clear", { type: "button", "aria-label": "Clear" });
clearBtn.innerHTML = svg(ICON.close, 16);
clearBtn.hidden = true;
const goBtn = el("button", "lm-go", { type: "submit", "aria-label": "Search" });
goBtn.innerHTML = svg(ICON.search, 20);
const list = el("ul", "lm-suggest", { id: "lm-list", role: "listbox" });
list.hidden = true;
combo.append(input, clearBtn, goBtn, list);
form.append(combo);

const status = el("div", "lm-status", { role: "status", "aria-live": "polite" });
status.hidden = true;
const body = el("div", "lm-body");
const head = el("div", "lm-head"); // the title of the list, where the sheet is dragged on a phone
const listView = el("section", "lm-listview");
const detailView = el("section", "lm-detail");
detailView.hidden = true;
const hereView = el("section", "lm-here");
hereView.hidden = true;
const routeView = el("section", "lm-route");
routeView.hidden = true;
body.append(listView, hereView, detailView, routeView);
// show one of the views of the side bar: the list, what is here, the page of a place, directions
const showView = (v) => {
  if (v !== routeView && !routeView.hidden) routeDone();
  listView.hidden = v !== listView;
  hereView.hidden = v !== hereView;
  detailView.hidden = v !== detailView;
  routeView.hidden = v !== routeView;
  body.scrollTop = 0;
};
const foot = el("div", "lm-foot", {}, root.dataset.say ? `${T.credit} ${T.voiceCredit}` : T.credit);
side.append(sheetBtn, top, form, status, body, foot);

// ------------------------------------------------------------------ the map
// a link with a place on the map (#map=) opens there; otherwise the map starts where the visitor is
const linkedView = /(^|&)map=/.test(location.hash.slice(1));
const map = new maplibregl.Map({
  container: mapEl,
  style: styleURL(),
  center: [8.54, 47.37],
  zoom: 13,
  hash: "map",
  maxPitch: 85,
  attributionControl: { compact: true },
});
map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "bottom-right");
map.addControl(new maplibregl.ScaleControl({ unit: "metric" }), "bottom-left");
// the toast over the map: the search again, in what the map shows now
const areaBtn = el("button", "lm-area", { type: "button" });
areaBtn.innerHTML = svg(ICON.refresh, 16);
areaBtn.append(el("span", "", {}, T.searchArea));
areaBtn.hidden = true;
// the compass: its needle turns with the map; a tap turns the map to north (and flat, unless 3D)
const compassBtn = el("button", "lm-compass", { type: "button", title: T.north, "aria-label": T.north });
compassBtn.innerHTML = '<svg viewBox="0 0 24 24" width="26" height="26" aria-hidden="true"><g class="lm-needle">' +
  '<path d="M12 3 15.5 12h-7z" fill="#d0312d"/><path d="M12 21 8.5 12h7z" fill="currentColor" opacity=".55"/></g></svg>';
const locBtn = el("button", "lm-locate", { type: "button", title: T.myLocation, "aria-label": T.myLocation });
locBtn.innerHTML = svg(ICON.locate, 22);
mapEl.append(areaBtn, compassBtn, locBtn);
const needle = compassBtn.querySelector(".lm-needle");
const turnNeedle = () => {
  needle.style.transform = `rotate(${-map.getBearing()}deg)`;
  compassBtn.classList.toggle("lm-turned", Math.abs(map.getBearing()) > 0.5 || map.getPitch() > 0.5);
};
map.on("rotate", turnNeedle);
map.on("pitch", turnNeedle);
map.on("load", turnNeedle);
compassBtn.addEventListener("click", () => map.easeTo({ bearing: 0, pitch: look.threeD ? map.getPitch() : 0, duration: 500 }));

const empty = { type: "FeatureCollection", features: [] };
const sources = { me: empty, results: empty, lines: empty, selected: empty, hover: empty, pin: empty, area: empty, route: empty };
const ACCENT = "#d0312d";
map.on("style.load", () => {
  for (const [name, d] of Object.entries(sources)) map.addSource(`lm-${name}`, { type: "geojson", data: d });
  const below = map.getStyle().layers.find((l) => l.type === "symbol")?.id;
  const dashed = ["in", ["get", "kind"], ["literal", ["border", "circle"]]];
  const pts = ["in", ["geometry-type"], ["literal", ["Point", "MultiPoint"]]];
  const polys = ["in", ["geometry-type"], ["literal", ["Polygon", "MultiPolygon"]]];
  map.addLayer({ id: "lm-drawn-dashed", type: "line", source: "lm-lines", filter: dashed,
    paint: { "line-color": "#1f5fbf", "line-width": ["match", ["get", "kind"], "circle", 1.2, 2], "line-dasharray": [3, 2], "line-opacity": 0.8 } }, below);
  map.addLayer({ id: "lm-drawn", type: "line", source: "lm-lines", filter: ["!", dashed], layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ["match", ["get", "kind"], "road", "#c2410c", "river", "#0284c7", "#1f5fbf"], "line-width": ["match", ["get", "kind"], "shape", 1.5, 3], "line-opacity": 0.85 } }, below);
  map.addLayer({ id: "lm-fill", type: "fill", source: "lm-results", filter: polys, paint: { "fill-color": ACCENT, "fill-opacity": 0.15 } }, below);
  map.addLayer({ id: "lm-line", type: "line", source: "lm-results", filter: ["!", pts], layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ACCENT, "line-width": ["interpolate", ["linear"], ["zoom"], 8, 1.5, 16, 4] } }, below);
  map.addLayer({ id: "lm-point", type: "circle", source: "lm-results", filter: pts, paint: {
    "circle-radius": ["interpolate", ["linear"], ["zoom"], 6, ["case", ["boolean", ["get", "place"], false], 8, 3], 15, ["case", ["boolean", ["get", "place"], false], 10, 7]],
    "circle-color": ["case", ["boolean", ["get", "place"], false], "#1f5fbf", ACCENT],
    "circle-stroke-color": "#fff", "circle-stroke-width": 1.5 } });
  map.addLayer({ id: "lm-hover", type: "circle", source: "lm-hover", filter: pts,
    paint: { "circle-radius": 11, "circle-color": ACCENT, "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 } });
  map.addLayer({ id: "lm-hover-line", type: "line", source: "lm-hover", filter: ["!", pts], paint: { "line-color": "#111827", "line-width": 4, "line-opacity": 0.5 } });
  map.addLayer({ id: "lm-sel-line", type: "line", source: "lm-selected", filter: ["!", pts], paint: { "line-color": "#111827", "line-width": 5, "line-opacity": 0.65 } });
  map.addLayer({ id: "lm-sel", type: "circle", source: "lm-selected", filter: pts,
    paint: { "circle-radius": 13, "circle-color": "#111827", "circle-stroke-color": "#fff", "circle-stroke-width": 3 } });
  map.addLayer({ id: "lm-me-area", type: "fill", source: "lm-me", filter: polys, paint: { "fill-color": "#1a73e8", "fill-opacity": 0.12 } });
  map.addLayer({ id: "lm-me", type: "circle", source: "lm-me", filter: pts,
    paint: { "circle-radius": 7, "circle-color": "#1a73e8", "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 } });
  // the area of a tap: its outline in the blue of overpass.ch, with a glow
  map.addLayer({ id: "lm-area-glow", type: "line", source: "lm-area", layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": "#0284c7", "line-width": 9, "line-opacity": 0.22, "line-blur": 2 } }, below);
  map.addLayer({ id: "lm-area-line", type: "line", source: "lm-area", layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": "#0284c7", "line-width": 2.5, "line-opacity": 0.95 } }, below);
  // directions: the line with a casing, the start (white) and the destination (red)
  const line = ["==", ["geometry-type"], "LineString"];
  map.addLayer({ id: "lm-route-case", type: "line", source: "lm-route", filter: line, layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": "#0b3d91", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 5, 16, 11], "line-opacity": 0.9 } }, below);
  map.addLayer({ id: "lm-route-line", type: "line", source: "lm-route", filter: line, layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": "#1a73e8", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 3, 16, 8] } }, below);
  map.addLayer({ id: "lm-route-ends", type: "circle", source: "lm-route", filter: ["!", line], paint: {
    "circle-radius": 7, "circle-color": ["match", ["get", "end"], "from", "#fff", ACCENT],
    "circle-stroke-color": ["match", ["get", "end"], "from", "#111827", "#fff"], "circle-stroke-width": 3 } });
  map.addLayer({ id: "lm-pin", type: "circle", source: "lm-pin",
    paint: { "circle-radius": 8, "circle-color": "#111827", "circle-stroke-color": "#fff", "circle-stroke-width": 3 } });
  map.addLayer({ id: "lm-label", type: "symbol", source: "lm-results", minzoom: 14, filter: ["all", ["has", "name"], pts],
    layout: { "text-field": ["get", "name"], "text-font": ["Noto Sans Medium"], "text-size": 12, "text-offset": [0, 1], "text-anchor": "top",
      "text-max-width": 10, "text-optional": true },
    paint: { "text-color": theme() === "dark" ? "#ffb4a9" : "#7f1d1d", "text-halo-color": theme() === "dark" ? "#111" : "#fff", "text-halo-width": 1.5 } });
});
const setSource = (name, d) => {
  sources[name] = d || empty;
  map.getSource(`lm-${name}`)?.setData(sources[name]);
};
const one = (f) => ({ type: "FeatureCollection", features: f ? [f] : [] });
for (const layer of ["lm-point", "lm-line", "lm-fill"]) {
  map.on("mouseenter", layer, (e) => {
    map.getCanvas().style.cursor = "pointer";
    const f = state.byId.get(e.features[0].properties.id);
    if (f) hover(f);
  });
  map.on("mouseleave", layer, () => { map.getCanvas().style.cursor = ""; hover(null); });
}
const viewBox = () => {
  const b = map.getBounds();
  return [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()].map((v) => v.toFixed(5)).join(",");
};

// ------------------------------------------------------------------ the menu
// Closed at first; the burger in the top bar (on a phone, over the map) opens it.
const mapMenuBtn = burger();
mapMenuBtn.classList.add("lm-map-burger");
mapEl.append(mapMenuBtn);
const backdrop = el("div", "lm-backdrop");
backdrop.hidden = true;
const menu = el("nav", "lm-menu", { id: "lm-menu", "aria-label": T.menu });
menu.hidden = true;
const menuHead = el("div", "lm-menu-head");
const closeMenuBtn = el("button", "lm-icon-btn", { type: "button", title: T.closeMenu, "aria-label": T.closeMenu });
closeMenuBtn.innerHTML = svg(ICON.close, 20);
menuHead.append(logo(), closeMenuBtn);
const styleGrid = el("div", "lm-styles", { role: "radiogroup", "aria-label": T.mapStyle });
for (const [key, label] of T.styles) {
  const b = el("button", "lm-style", { type: "button", role: "radio", "data-style": key, "aria-checked": String(key === look.style) });
  b.append(el("span", `lm-swatch lm-swatch-${key}`), el("span", "", {}, label));
  b.addEventListener("click", () => {
    look.style = key;
    store.set("hm-style", key);
    document.documentElement.dataset.theme = theme();
    styleGrid.querySelectorAll("[data-style]").forEach((x) => x.setAttribute("aria-checked", String(x.dataset.style === key)));
    restyle();
  });
  styleGrid.append(b);
}
const toggle = (label, note, on, set) => {
  const row = el("label", "lm-toggle");
  const box = el("input", "", { type: "checkbox", role: "switch" });
  box.checked = on;
  box.addEventListener("change", () => set(box.checked));
  const text = el("span", "lm-toggle-text");
  text.append(el("span", "", {}, label), el("span", "lm-toggle-note", {}, note));
  row.append(text, box);
  return row;
};
menu.append(menuHead, el("h2", "lm-menu-title", {}, T.mapStyle), styleGrid, el("h2", "lm-menu-title", {}, T.show),
  toggle(T.threeD, T.threeDNote, look.threeD, (on) => {
    look.threeD = on;
    store.set("hm-3d", on ? "1" : "0");
    restyle();
    if (on && map.getPitch() < 30) map.easeTo({ pitch: 60, duration: 800 });
    if (!on) map.easeTo({ pitch: 0, bearing: 0, duration: 800 });
  }),
  toggle(T.shading, T.shadingNote, look.shading, (on) => {
    look.shading = on;
    store.set("hm-shading", on ? "1" : "0");
    restyle();
  }),
  poweredBy(), el("p", "lm-menu-foot", {}, T.credit));
root.append(backdrop, menu);
// poweredBy is the line "Powered by overpass.ch" of the menu, with the logo of overpass.ch
function poweredBy() {
  const a = el("a", "lm-powered", { href: "/", title: "overpass.ch" });
  const light = el("img", "lm-powered-light", { src: root.dataset.overpass, alt: "overpass.ch", height: "22" });
  const dark = el("img", "lm-powered-dark", { src: root.dataset.overpassDark, alt: "overpass.ch", height: "22" });
  a.append(el("span", "", {}, "Powered by"), light, dark);
  return a;
}
function restyle() {
  map.setStyle(styleURL(), { diff: false }); // our layers come back on style.load
}
const openMenu = (open) => {
  menu.hidden = backdrop.hidden = !open;
  for (const b of [menuBtn, mapMenuBtn]) b.setAttribute("aria-expanded", String(open));
  if (open) closeMenuBtn.focus();
};
for (const b of [menuBtn, mapMenuBtn]) b.addEventListener("click", () => openMenu(true));
closeMenuBtn.addEventListener("click", () => openMenu(false));
backdrop.addEventListener("click", () => openMenu(false));
addEventListener("keydown", (e) => { if (e.key === "Escape" && !menu.hidden) openMenu(false); });
// only the map zooms: Safari ignores user-scalable=no and zooms the page on a pinch
for (const ev of ["gesturestart", "gesturechange"]) {
  document.addEventListener(ev, (e) => { if (!mapEl.contains(e.target)) e.preventDefault(); }, { passive: false });
}
document.addEventListener("dblclick", (e) => { if (!mapEl.contains(e.target)) e.preventDefault(); });

// ------------------------------------------------------------------ the sheet on a phone
// The side bar is a sheet over the lower part of the map, at one of three heights: peek
// (the search box and the title), half, full. It is dragged by its handle and its title.
const phone = () => matchMedia("(max-width: 760px)").matches;
const sheet = { state: "half" };
const sheetHeights = () => ({
  peek: Math.min(form.offsetTop + form.offsetHeight + 64, innerHeight * 0.4),
  half: Math.round(innerHeight * 0.48),
  full: innerHeight - 56,
});
function setSheet(state) {
  if (!phone()) return;
  sheet.state = state;
  root.dataset.sheet = state;
  side.style.height = `${sheetHeights()[state]}px`;
}
const toggleSheet = () => setSheet(sheet.state === "peek" ? "half" : "peek");
let pointerAt = 0; // the last release on a drag zone: the click after it is that release again
sheetBtn.addEventListener("click", () => { if (Date.now() - pointerAt > 500) toggleSheet(); }); // the keyboard
// snapFrom sets the sheet to the height a drag that began at startH ends nearest to
function snapFrom(startH) {
  const h = sheetHeights(), now = side.offsetHeight, dy = now - startH;
  const order = ["peek", "half", "full"];
  let i = order.indexOf(sheet.state);
  if (dy > 50) i = Math.min(2, i + (now > h.half + 80 ? 2 : 1));
  else if (dy < -50) i = Math.max(0, i - (now < h.half - 80 ? 2 : 1));
  setSheet(order[Math.min(2, Math.max(0, i))]);
}
// a mouse drags the sheet by its handle and title (a narrow window)
for (const zone of [sheetBtn, head]) {
  let startY = 0, startH = 0, moved = false, id = null;
  zone.addEventListener("pointerdown", (e) => {
    if (!phone() || e.pointerType !== "mouse") return;
    id = e.pointerId;
    startY = e.clientY;
    startH = side.offsetHeight;
    moved = false;
    zone.setPointerCapture(id);
    side.classList.add("lm-dragging");
  });
  zone.addEventListener("pointermove", (e) => {
    if (e.pointerId !== id) return;
    const dy = startY - e.clientY;
    if (Math.abs(dy) > 4) moved = true;
    const h = sheetHeights();
    side.style.height = `${Math.max(h.peek * 0.8, Math.min(h.full, startH + dy))}px`;
  });
  const end = (e) => {
    if (e.pointerId !== id) return;
    id = null;
    pointerAt = Date.now();
    side.classList.remove("lm-dragging");
    if (!moved) return zone === sheetBtn ? toggleSheet() : setSheet(sheet.state);
    snapFrom(startH);
  };
  zone.addEventListener("pointerup", end);
  zone.addEventListener("pointercancel", end);
}
// a finger moves the sheet from anywhere on it: above the list always; on the list when the
// list is at its top and the finger goes down, or when the sheet is not full and it goes up.
// Otherwise the list scrolls.
let swipe = null;
side.addEventListener("touchstart", (e) => {
  if (!phone() || e.touches.length !== 1) return;
  const t = e.touches[0];
  swipe = { x: t.clientX, y: t.clientY, h: side.offsetHeight, mode: "", inList: body.contains(e.target) && !head.contains(e.target) };
}, { passive: true });
side.addEventListener("touchmove", (e) => {
  if (!swipe || e.touches.length !== 1) return;
  const t = e.touches[0], dx = t.clientX - swipe.x, dy = t.clientY - swipe.y;
  if (!swipe.mode) {
    if (Math.abs(dy) < 6 || Math.abs(dy) < Math.abs(dx)) return;
    const down = dy > 0;
    swipe.mode = !swipe.inList || (down && body.scrollTop <= 0) || (!down && sheet.state !== "full") ? "sheet" : "scroll";
    if (swipe.mode === "sheet") {
      side.classList.add("lm-dragging");
      if (document.activeElement === input) input.blur();
      closeList();
    }
  }
  if (swipe.mode !== "sheet") return;
  e.preventDefault();
  const h = sheetHeights();
  side.style.height = `${Math.max(h.peek * 0.8, Math.min(h.full, swipe.h - dy))}px`;
}, { passive: false });
const swipeEnd = () => {
  const sw = swipe;
  swipe = null;
  if (!sw || sw.mode !== "sheet") return;
  side.classList.remove("lm-dragging");
  pointerAt = Date.now();
  snapFrom(sw.h);
};
side.addEventListener("touchend", swipeEnd);
side.addEventListener("touchcancel", swipeEnd);
addEventListener("resize", () => setSheet(sheet.state));
// the sheet covers the lower part of the map: its height lifts the controls
new ResizeObserver(() => root.style.setProperty("--sheet", `${side.offsetHeight}px`)).observe(side);

// ------------------------------------------------------------------ state
const state = {
  text: "", extra: {}, answer: null, features: [], byId: new Map(), shown: 0, selected: null, searchable: false, me: null,
  country: "", // of the place searched, when the answer says it: for public holidays
  pushedSel: false, // the page of a place has its own entry in the history: Back goes to the list
};

// ------------------------------------------------------------------ suggestions
let options = [], active = -1, suggestTimer = 0, suggestCtl = null, suggestFor = "", chosen = null;
const closeList = () => {
  list.hidden = true;
  input.setAttribute("aria-expanded", "false");
  input.removeAttribute("aria-activedescendant");
  active = -1;
};
const markActive = (i) => {
  active = i;
  list.querySelectorAll("[role=option]").forEach((o, j) => o.setAttribute("aria-selected", String(j === i)));
  if (i >= 0) {
    input.setAttribute("aria-activedescendant", `lm-opt-${i}`);
    list.querySelector(`#lm-opt-${i}`)?.scrollIntoView({ block: "nearest" });
  }
};
const placeOf = (s) => Object.fromEntries(placeKeys.filter((k) => s[k]).map((k) => [k, s[k]]));
const renderList = (items) => {
  options = items;
  list.innerHTML = "";
  if (!items.length) return closeList();
  items.forEach((s, i) => {
    const li = el("li", `lm-opt lm-opt-${s.kind}`, { role: "option", id: `lm-opt-${i}`, "aria-selected": "false" });
    const ic = el("span", "lm-opt-icon");
    ic.innerHTML = svg(s.kind === "place" ? ICON.pin : ICON.search, 16);
    const txt = el("span", "lm-opt-text");
    txt.append(el("span", "lm-opt-label", {}, s.label));
    if (s.detail) txt.append(el("span", "lm-opt-detail", {}, s.detail));
    li.append(ic, txt);
    li.addEventListener("mousedown", (e) => { e.preventDefault(); pick(i); });
    list.append(li);
  });
  list.hidden = false;
  input.setAttribute("aria-expanded", "true");
  markActive(-1);
};
const suggest = () => {
  const q = input.value;
  if (q.trim().length < 2) return Promise.resolve(closeList());
  suggestCtl?.abort();
  suggestCtl = new AbortController();
  return fetch(`${API}/complete?${new URLSearchParams({ q, lang: LANG, bbox: viewBox() })}`, { signal: suggestCtl.signal })
    .then((r) => (r.ok ? r.json() : { suggestions: [] }))
    .then((d) => {
      if (input.value !== q || document.activeElement !== input) return;
      suggestFor = q;
      renderList(d.suggestions || []);
    })
    .catch(() => {});
};
const pick = (i) => {
  const s = options[i];
  if (!s) return;
  input.value = s.text;
  chosen = null;
  closeList();
  if (phone()) input.blur();
  run(s.text, placeOf(s));
};
const accept = (s) => {
  input.value = s.kind === "what" ? `${s.text} ` : s.text;
  chosen = { text: s.text, extra: placeOf(s) };
  options = [];
  closeList();
  suggest();
};
input.addEventListener("input", () => {
  clearBtn.hidden = !input.value;
  clearTimeout(suggestTimer);
  suggestTimer = setTimeout(suggest, 150);
});
input.addEventListener("keydown", (e) => {
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    if (list.hidden) { suggest(); return; }
    e.preventDefault();
    const n = options.length;
    markActive(e.key === "ArrowDown" ? (active + 1) % n : (active - 1 + n) % n);
  } else if (e.key === "Enter" && !list.hidden && active >= 0) {
    e.preventDefault();
    pick(active);
  } else if (e.key === "Tab" && !e.shiftKey && !list.hidden && options.length && suggestFor === input.value) {
    e.preventDefault();
    accept(active >= 0 ? options[active] : options[0]);
  } else if (e.key === "Escape") {
    if (!list.hidden) { e.preventDefault(); closeList(); }
  }
});
input.addEventListener("focus", () => setSheet("full"));
input.addEventListener("blur", () => setTimeout(closeList, 120));
clearBtn.addEventListener("click", () => {
  input.value = "";
  clearBtn.hidden = true;
  input.focus();
  closeList();
});
form.addEventListener("submit", (e) => {
  e.preventDefault();
  clearTimeout(suggestTimer);
  suggestCtl?.abort();
  closeList();
  const text = input.value.trim();
  if (phone()) { // the keyboard goes, and the map is in view again (Safari may leave the page scrolled)
    input.blur();
    scrollTo(0, 0);
  }
  if (text) run(text, chosen && chosen.text.trim() === text ? chosen.extra : {});
});

// ------------------------------------------------------------------ searching
let runCtl = null;
const say = (msg, bad) => {
  status.textContent = msg || "";
  status.hidden = !msg;
  status.classList.toggle("lm-bad", !!bad);
};
const problem = (d) => T.errors[d.error?.code] || d.error?.message || T.failed;
const locate = () => {
  if (state.me && Date.now() - state.me.at < 5 * 60e3) return Promise.resolve(state.me.v);
  if (!navigator.geolocation) return Promise.resolve("");
  return new Promise((done) =>
    navigator.geolocation.getCurrentPosition(
      (pos) => done(meAt(pos)),
      () => done(""),
      { timeout: 8000, maximumAge: 300e3 },
    ));
};
// meAt keeps a position of the visitor and draws it: a dot, and the circle of its accuracy
function meAt(pos) {
  const c = [pos.coords.longitude, pos.coords.latitude];
  state.me = { v: `${c[1].toFixed(5)},${c[0].toFixed(5)}`, at: Date.now(), at2: c };
  const r = Math.min(pos.coords.accuracy || 0, 5000), ring = [];
  for (let i = 0; i <= 48; i++) {
    const a = (i / 48) * 2 * Math.PI;
    ring.push([c[0] + (r / (111320 * Math.cos((c[1] * Math.PI) / 180))) * Math.cos(a), c[1] + (r / 110574) * Math.sin(a)]);
  }
  setSource("me", { type: "FeatureCollection", features: [
    { type: "Feature", geometry: { type: "Polygon", coordinates: [ring] }, properties: {} },
    { type: "Feature", geometry: { type: "Point", coordinates: c }, properties: {} },
  ] });
  return state.me.v;
}
// the button asks the browser, which asks the visitor the first time; quiet: at the start of the
// page, without a message when the visitor says no, and not after they moved the map or searched
function showMe(quiet) {
  if (!navigator.geolocation) return quiet || say(T.locMissing, true);
  locBtn.classList.add("lm-busy");
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      locBtn.classList.remove("lm-busy");
      locBtn.classList.add("lm-on");
      meAt(pos);
      if (quiet && (state.touched || state.text || !routeView.hidden || nav.on)) return;
      if (status.classList.contains("lm-bad")) say("");
      fit(() => map.flyTo({ center: state.me.at2, zoom: Math.max(map.getZoom(), 15), duration: 900, padding: pad() }));
    },
    (err) => {
      locBtn.classList.remove("lm-busy");
      if (!quiet) say(err.code === 1 ? T.locBlocked : T.locFailed, true);
    },
    { enableHighAccuracy: true, timeout: 10000, maximumAge: 60000 },
  );
}
locBtn.addEventListener("click", () => showMe(false));
map.once("dragstart", () => { state.touched = true; });

// run searches a text; extra holds place, place2, region and what, as the API takes them
async function run(text, extra = {}, { push = true, bbox = null, sel = null } = {}) {
  runCtl?.abort();
  const ctl = runCtl = new AbortController();
  input.value = text;
  clearBtn.hidden = !text;
  const params = new URLSearchParams({ q: text, bbox: bbox || viewBox() });
  for (const k of extraKeys) if (extra[k]) params.set(k, extra[k]);
  state.text = text;
  state.extra = extra;
  if (push) writeURL(sel);
  areaBtn.hidden = true;
  say(T.searching);
  hereDone();
  const limit = setTimeout(() => ctl.abort("timeout"), 75e3);
  try {
    const qr = await fetch(`${API}/query?${params}`, { signal: ctl.signal });
    const p = await qr.json().catch(() => ({}));
    if (ctl.signal.aborted) return;
    const wants = p.wants_location || p.error?.code === "need_location";
    if (!qr.ok && !wants) return say(problem(p), true);
    if (qr.ok) showReading(p);
    if (wants) {
      say(T.locating);
      const me = await locate();
      if (ctl.signal.aborted) return;
      if (me) params.set("me", me);
      say(T.searching);
    }
    const r = await fetch(`${API}/search?${params}`, { signal: ctl.signal });
    const d = await r.json().catch(() => ({}));
    if (ctl.signal.aborted) return;
    if (!r.ok) return say(problem(d), true);
    say("");
    show(d, sel);
  } catch (err) {
    if (ctl.signal.reason === "timeout") say(T.tooLong, true);
    else if (err.name !== "AbortError") say(T.failed, true);
  } finally {
    clearTimeout(limit);
  }
}

// the search in the address bar: ?q=…&place=…&sel=node/1, and the map in #map=
function writeURL(sel, replace = false) {
  const u = new URL(location.href);
  u.search = "";
  if (state.text) u.searchParams.set("q", state.text);
  for (const k of extraKeys) if (state.extra[k]) u.searchParams.set(k, state.extra[k]);
  if (sel) u.searchParams.set("sel", sel);
  if (u.href === location.href) return;
  if (replace) history.replaceState(null, "", u);
  else history.pushState(null, "", u);
}
window.addEventListener("popstate", () => {
  state.pushedSel = false;
  const u = new URLSearchParams(location.search);
  const q = u.get("q") || "";
  const sel = u.get("sel");
  const extra = Object.fromEntries(extraKeys.map((k) => [k, u.get(k) || ""]));
  if (q && q === state.text && JSON.stringify(extra) === JSON.stringify(Object.fromEntries(extraKeys.map((k) => [k, state.extra[k] || ""])))) {
    const f = sel && state.byId.get(sel);
    if (f) select(f, { push: false }); else showList();
  } else if (q) run(q, extra, { push: false, sel });
  else { state.text = ""; input.value = ""; clear(); intro(); }
});

// ------------------------------------------------------------------ the list
const title = el("h1", "lm-title");
const notes = el("p", "lm-notes");
const chips = el("div", "lm-chips");
const count = el("p", "lm-count");
const results = el("ol", "lm-results");
const moreBtn = el("button", "lm-more", { type: "button" }, T.more);
moreBtn.hidden = true;
const query = el("details", "lm-query");
const qSum = el("summary", "", {}, T.query);
const qPre = el("pre");
const qCopy = el("button", "lm-copy", { type: "button" }, T.copy);
query.append(qSum, qPre, qCopy);
query.hidden = true;
head.append(title, notes);
listView.append(head, chips, count, results, moreBtn, query);
qCopy.addEventListener("click", () => navigator.clipboard?.writeText(qPre.textContent).then(() => {
  qCopy.textContent = T.copied;
  setTimeout(() => { qCopy.textContent = T.copy; }, 1500);
}));

function intro() {
  clear();
  title.textContent = "";
  notes.textContent = T.intro;
  for (const x of T.examples) chips.append(chip(x, () => run(x, {})));
}
function clear() {
  state.searchable = false;
  areaBtn.hidden = true;
  setSource("results", empty);
  setSource("lines", empty);
  setSource("selected", empty);
  setSource("hover", empty);
  state.features = [];
  state.byId = new Map();
  state.selected = null;
  chips.innerHTML = "";
  results.innerHTML = "";
  count.textContent = "";
  moreBtn.hidden = true;
  query.hidden = true;
  showView(listView);
}
const chip = (label, onClick, cls = "") => {
  const b = el("button", `lm-chip ${cls}`.trim(), { type: "button" }, label);
  b.addEventListener("click", onClick);
  return b;
};
const chipRow = (name, items) => {
  if (!items.length) return;
  const row = el("div", "lm-chiprow");
  row.append(el("span", "lm-chipname", {}, name), ...items);
  chips.append(row);
};
function showReading(p) {
  clear();
  title.textContent = p.understood || "";
  notes.textContent = (p.notes || []).join(" ");
  qPre.textContent = p.query || "";
  query.hidden = !p.query;
}

let quietUntil = 0; // until then the map moves for a search, not for the visitor
const fit = (fn, ms = 1300) => {
  quietUntil = Date.now() + ms;
  fn();
};
// the visitor moved the map (a drag, the wheel, a pinch, the keys, the buttons): the
// moves of the map itself, to show a search, do not count
let touched = false, touchedAt = 0;
for (const ev of ["pointerdown", "wheel", "touchstart", "keydown"]) {
  mapEl.addEventListener(ev, () => { touched = true; touchedAt = Date.now(); }, { passive: true, capture: true });
}
// fitBox shows the box of an answer. On a phone the keyboard closes while a fast answer comes, and
// the view changes size after the map moved: when the view settles within 2.5 s, the box is
// fitted again, unless the visitor touched the map since.
let lastFit = null, refitTimer = 0;
function fitBox(box, maxZoom) {
  lastFit = { box, maxZoom, at: Date.now() };
  fit(() => map.fitBounds([[box[0], box[1]], [box[2], box[3]]], { padding: pad(), maxZoom, duration: 800 }));
}
(window.visualViewport || window).addEventListener("resize", () => {
  clearTimeout(refitTimer);
  refitTimer = setTimeout(() => {
    if (!lastFit || Date.now() - lastFit.at > 2500 || touchedAt > lastFit.at) return;
    map.resize();
    const b = lastFit.box;
    fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: lastFit.maxZoom, duration: 300 }));
    map.once("moveend", () => map.triggerRepaint());
  }, 150);
});
// a phone may take the graphics of the map back when its memory runs short: the map then stops
// drawing, and the visitor needs to know why
map.on("webglcontextlost", () => say(T.mapLost, true));
map.on("webglcontextrestored", () => { say(""); map.triggerRepaint(); });
map.on("moveend", () => {
  if (touched && Date.now() > quietUntil && state.searchable) areaBtn.hidden = false;
  touched = false;
});
areaBtn.addEventListener("click", () => {
  const extra = { ...state.extra, view: "1" };
  for (const k of [...placeKeys, "what"]) delete extra[k]; // the places of the sentence go; the what of another reading too
  run(state.text, extra);
});

function show(d, sel) {
  clear();
  state.answer = d;
  const found = d.results?.features || [];
  state.features = d.order === "distance" ? found : found.filter((f) => f.properties.name).concat(found.filter((f) => !f.properties.name));
  for (const f of found) state.byId.set(f.properties.id, f);
  state.searchable = false;
  state.country = d.open?.country || "";
  ohCache.clear();
  setSource("results", d.results);
  setSource("lines", d.lines);
  const place = found.length === 1 && found[0].properties.place ? found[0] : null;
  title.textContent = place ? place.properties.name || d.text : d.understood;
  notes.textContent = [place?.properties.detail, ...(d.notes || [])].filter(Boolean).join(" ");
  chipRow(T.tryHere, (d.try || []).map((x) => chip(x.label, () => run(x.q, {}))));
  chipRow(T.otherReadings, (d.alternatives || []).map((a) => chip(a.label, () => run(state.text, { ...state.extra, what: a.what }))));
  const where = (p) => [p.label, (p.detail || "").split(" · ")[0]].filter(Boolean).join(", ");
  chipRow(T.otherPlaces, (d.places || []).slice(0, 4).map((p) => {
    const c = chip(where(p), () => run(state.text, { ...state.extra, place: p.place }), "lm-chip-place");
    c.title = [p.label, p.detail].filter(Boolean).join(" · ");
    return c;
  }));
  qPre.textContent = d.query || "";
  query.hidden = !d.query;
  setSheet(place ? "peek" : "half");
  const box = d.results?.bbox;
  const pick = sel && state.byId.get(sel);
  if (box?.length === 4 && !pick) fitBox(box, place ? 16 : 16.5);
  if (place) {
    count.textContent = "";
    return;
  }
  state.searchable = true; // moving the map offers the search there
  count.textContent = T.results(d.total ?? found.length);
  if (d.truncated) count.textContent += ` · ${T.shown(found.length, d.total)}`;
  state.shown = 0;
  renderMore();
  if (d.open) openNow(d.open);
  else if (found.some((f) => f.properties.opening_hours)) loadHours().then(() => markHours());
  if (pick) select(pick, { push: false });
}
// pad keeps what the map fits to out from under the sheet on a phone
const pad = () => (phone()
  ? { top: 40, left: 30, right: 30, bottom: Math.min(innerHeight - 140, sheetHeights()[sheet.state] + 20) }
  : { top: 50, left: 50, right: 50, bottom: 50 });

const PAGE = 40;
function renderMore() {
  const next = state.features.slice(state.shown, state.shown + PAGE);
  for (const f of next) results.append(card(f));
  state.shown += next.length;
  moreBtn.hidden = state.shown >= state.features.length;
}
moreBtn.addEventListener("click", renderMore);

function card(f, { fromHere = false } = {}) {
  const p = f.properties, t = tagsOf(p), k = kindOf(p);
  const li = el("li");
  const b = el("button", "lm-card", { type: "button", "data-id": p.id });
  const thumb = el("span", "lm-thumb");
  thumb.style.setProperty("--c", KEY_COLOURS[k.key] || "#868e96");
  thumb.textContent = (k.label || p.name || "?").slice(0, 1).toUpperCase();
  const text = el("span", "lm-card-text");
  text.append(el("span", "lm-card-name", {}, p.name || T.noName));
  const sub = [k.label, t.brand && t.brand !== p.name ? t.brand : ""].filter(Boolean).join(" · ");
  if (sub) text.append(el("span", "lm-card-sub", {}, sub));
  const addr = addressOf(t);
  const c = center(f);
  const dist = state.me?.at2 && c ? fmtKm(km(state.me.at2, c)) : "";
  if (addr || dist) text.append(el("span", "lm-card-addr", {}, [dist, addr].filter(Boolean).join(" · ")));
  if (p.opening_hours) text.append(el("span", "lm-card-hours", { "data-hours": "" }));
  b.append(text, thumb);
  b.addEventListener("click", () => select(f, { push: !fromHere }));
  b.addEventListener("mouseenter", () => hover(f));
  b.addEventListener("mouseleave", () => hover(null));
  li.append(b);
  if (hasPicture(t)) lazyPicture(thumb, t);
  if (OH) markHours(li);
  return li;
}
function hover(f) {
  setSource("hover", one(f));
  results.querySelectorAll(".lm-card.lm-hot").forEach((c) => c.classList.remove("lm-hot"));
  if (f) results.querySelector(`[data-id="${CSS.escape(f.properties.id)}"]`)?.classList.add("lm-hot");
}

// ------------------------------------------------------------------ pictures
const hasPicture = (t) => PICTURES && (t.wikidata || t["brand:wikidata"] || t.wikimedia_commons || /wikimedia\.org/.test(t.image || ""));
const commonsOf = (t) => {
  if (t.wikimedia_commons?.startsWith("File:")) return t.wikimedia_commons;
  const m = /^https?:\/\/(?:commons\.wikimedia\.org\/wiki\/(File:[^?#]+)|upload\.wikimedia\.org\/wikipedia\/commons\/(?:thumb\/)?[0-9a-f]\/[0-9a-f]{2}\/([^/?#]+))/.exec(t.image || "");
  if (!m) return "";
  try { return m[1] ? decodeURIComponent(m[1]) : `File:${decodeURIComponent(m[2])}`; } catch { return ""; }
};
const pictures = new Map(); // params -> promise of {url, kind, credit, page} or null
let picQueue = [], picRunning = 0;
const pump = () => {
  while (picRunning < 4 && picQueue.length) {
    const job = picQueue.shift();
    picRunning++;
    job().finally(() => { picRunning--; pump(); });
  }
};
function picture(t, w) {
  const q = new URLSearchParams();
  if (t.wikidata) q.set("wikidata", t.wikidata);
  if (t["brand:wikidata"]) q.set("brand", t["brand:wikidata"]);
  const c = commonsOf(t);
  if (c) q.set("commons", c);
  q.set("w", String(w));
  const key = q.toString();
  if (!pictures.has(key)) {
    pictures.set(key, new Promise((done) => {
      picQueue.push(() => fetch(`${PICTURES}?${key}`)
        .then(async (r) => {
          if (r.status !== 200) return done(null); // 204: the place does not have one
          const blob = await r.blob();
          let credit = "";
          try { credit = decodeURIComponent((r.headers.get("X-Picture-Credit") || "").replace(/\+/g, " ")); } catch {}
          done({ url: URL.createObjectURL(blob), kind: r.headers.get("X-Picture-Kind") || "photo", credit, page: r.headers.get("X-Picture-Page") || "" });
        })
        .catch(() => done(null)));
      pump();
    }));
  }
  return pictures.get(key);
}
const seen = new IntersectionObserver((entries) => {
  for (const e of entries) {
    if (!e.isIntersecting) continue;
    seen.unobserve(e.target);
    const t = JSON.parse(e.target.dataset.tags);
    picture(t, 120).then((pic) => {
      if (!pic) return;
      const img = el("img", pic.kind === "logo" ? "lm-logo" : "", { src: pic.url, alt: "", loading: "lazy", title: pic.credit });
      e.target.textContent = "";
      e.target.classList.add("lm-has-img");
      e.target.append(img);
    });
  }
}, { root: body, rootMargin: "200px" });
const lazyPicture = (thumb, t) => {
  thumb.dataset.tags = JSON.stringify({ wikidata: t.wikidata, "brand:wikidata": t["brand:wikidata"], wikimedia_commons: t.wikimedia_commons, image: t.image });
  seen.observe(thumb);
};

// ------------------------------------------------------------------ opening hours
let OH = null, ohLoading = null;
const loadHours = () => (ohLoading ||= import("./vendor/opening_hours-3.15.0/opening_hours.mjs").then((m) => { OH = m.default; }).catch(() => {}));
const ohCache = new Map();
// noHolidays drops the rules of public holidays ("PH off", "Su,PH 10:00-18:00"): opening_hours.js
// needs the country, and the region in some, to know the holidays. On other days the hours are the same.
const noHolidays = (v) => v.replace(/\s*,\s*PH\b/g, "").replace(/\bPH\s*,\s*/g, "")
  .split(";").filter((r) => !/^\s*PH\b/.test(r)).join(";").trim();
const hoursOf = (f) => {
  const p = f.properties;
  if (!OH || !p.opening_hours) return null;
  if (ohCache.has(p.id)) return ohCache.get(p.id);
  const c = center(f) || [0, 0];
  const cc = (tagsOf(p)["addr:country"] || state.country || "").toLowerCase();
  const read = (v, country) => {
    const oh = new OH(v, { lat: c[1], lon: c[0], address: { country_code: country, state: "" } }, { mode: 0, locale: "en" });
    oh.getState(new Date()); // a holiday the country does not have fails here, not in the constructor
    return oh;
  };
  let oh = null;
  const v = p.opening_hours;
  try { oh = read(cc || !/\bPH\b/.test(v) ? v : noHolidays(v), cc); } catch {
    try { oh = read(noHolidays(v), ""); } catch { oh = null; }
  }
  ohCache.set(p.id, oh);
  return oh;
};
const hm = (d) => d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
const dayHm = (d) => {
  const today = new Date();
  return d.toDateString() === today.toDateString() ? hm(d) : `${d.toLocaleDateString("en-GB", { weekday: "short" })} ${hm(d)}`;
};
// stateText is "Open · closes 18:00", for the list and the page of a place
function stateText(oh) {
  const now = new Date();
  const open = oh.getState(now);
  const next = oh.getNextChange(now);
  const when = next ? (open ? T.closes(dayHm(next)) : T.opens(dayHm(next))) : "";
  return { open, text: [open ? T.open : T.closed, when].filter(Boolean).join(" · ") };
}
function markHours(scope = results) {
  for (const s of scope.querySelectorAll("[data-hours]")) {
    const f = state.byId.get(s.closest(".lm-card")?.dataset.id);
    const oh = f && hoursOf(f);
    if (!oh) continue;
    const st = stateText(oh);
    s.textContent = st.text;
    s.classList.toggle("lm-open", st.open);
    s.classList.toggle("lm-closed", !st.open);
  }
}
// openNow keeps the places open at the time a search asks ("open now", "open on Sunday")
async function openNow(o) {
  await loadHours();
  if (!OH) return;
  const now = new Date();
  const at = (day, minute) => {
    const x = new Date(now);
    if (day >= 0) x.setDate(x.getDate() + ((day - x.getDay() + 7) % 7));
    if (minute >= 0) x.setHours(Math.floor(minute / 60), minute % 60, 0, 0); else x.setHours(0, 0, 0, 0);
    return x;
  };
  const isOpen = (oh, day) => {
    if (o.now) return oh.getState(now);
    const from = at(day, o.minute);
    if (o.minute >= 0) return oh.getState(from);
    const to = new Date(from); to.setDate(to.getDate() + 1);
    return oh.getOpenIntervals(from, to).length > 0;
  };
  const all = state.features.length;
  state.features = state.features.filter((f) => {
    const oh = hoursOf(f);
    try { return oh && (o.weekend ? isOpen(oh, 6) && isOpen(oh, 0) : isOpen(oh, o.day)); } catch { return false; }
  });
  const keep = new Set(state.features.map((f) => f.properties.id));
  setSource("results", { ...state.answer.results, features: state.answer.results.features.filter((f) => keep.has(f.properties.id)) });
  count.textContent = `${state.features.length.toLocaleString("en")} ${o.label}, of ${all.toLocaleString("en")} with opening hours`;
  notes.textContent = notes.textContent.replace(/The opening hours were not checked[^.]*\./, "").trim();
  results.innerHTML = "";
  state.shown = 0;
  renderMore();
}

// ------------------------------------------------------------------ the page of a place
function showList() {
  state.selected = null;
  setSource("selected", empty);
  showView(state.from === "here" ? hereView : listView);
}
function select(f, { push = true, fly = true } = {}) {
  state.selected = f;
  setSource("selected", one(f));
  if (push) {
    writeURL(f.properties.id);
    state.pushedSel = true;
  }
  const c = center(f);
  if (fly && c) {
    const b = extent(f.geometry);
    if (f.geometry.type === "Point" || !b) fit(() => map.easeTo({ center: c, zoom: Math.max(map.getZoom(), 16), duration: 600, padding: pad() }));
    else fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: 17, duration: 600 }));
  }
  state.from = hereView.hidden ? "list" : "here";
  renderDetail(f);
  showView(detailView);
  setSheet(sheet.state === "full" ? "full" : "half");
}
function renderDetail(f) {
  const p = f.properties, t = tagsOf(p), k = kindOf(p);
  detailView.innerHTML = "";
  const back = el("button", "lm-back", { type: "button" });
  back.innerHTML = svg(ICON.back, 18);
  back.append(state.from === "here" ? T.here : T.back);
  back.addEventListener("click", () => {
    if (state.pushedSel && state.from !== "here") return history.back();
    showList();
    if (state.from !== "here") writeURL(null, true);
  });
  detailView.append(back);
  const fig = el("figure", "lm-pic");
  fig.hidden = true;
  detailView.append(fig);
  if (hasPicture(t)) {
    picture(t, 500).then((pic) => {
      if (!pic || state.selected !== f) return;
      fig.hidden = false;
      fig.classList.toggle("lm-pic-logo", pic.kind === "logo");
      fig.append(el("img", "", { src: pic.url, alt: p.name || "" }));
      const cap = el("figcaption");
      cap.append(`${pic.kind === "logo" ? T.logo : T.photo}${pic.credit ? `: ${pic.credit}` : ""} · `);
      const a = el("a", "", { href: pic.page, target: "_blank", rel: "noopener" }, "Wikimedia Commons");
      cap.append(a);
      fig.append(cap);
    });
  }
  detailView.append(el("h2", "lm-dname", {}, p.name || T.noName));
  const sub = [k.label, t.brand && t.brand !== p.name ? t.brand : "", t.cuisine ? human(t.cuisine.split(";").join(", ")) : ""].filter(Boolean).join(" · ");
  if (sub) detailView.append(el("p", "lm-dsub", {}, sub));
  const oh = hoursOf(f);
  if (oh) {
    const st = stateText(oh);
    detailView.append(el("p", `lm-dstate ${st.open ? "lm-open" : "lm-closed"}`, {}, st.text));
  } else if (p.opening_hours && !OH) loadHours().then(() => { if (OH && state.selected === f) renderDetail(f); }); // once: hours it cannot read stay text

  const c = center(f);
  const actions = el("div", "lm-actions");
  const action = (label, href, icon) => {
    const a = el("a", "lm-action", { href, target: "_blank", rel: "noopener nofollow" });
    a.innerHTML = svg(icon, 18);
    a.append(el("span", "", {}, label));
    actions.append(a);
  };
  if (c && ROUTE) actions.append(routeButton(() => directions({ ll: c, label: p.name || kindOf(p).label || T.noName })));
  else if (c) {
    const from = state.me ? `from=${state.me.at2[1].toFixed(6)}%2C${state.me.at2[0].toFixed(6)}&` : "";
    action(T.directions, `https://www.openstreetmap.org/directions?${from}to=${c[1].toFixed(6)}%2C${c[0].toFixed(6)}`, ICON.route);
  }
  const web = safeURL(t.website || t["contact:website"]);
  if (web) action(T.website, web, ICON.globe);
  const phone = (t.phone || t["contact:phone"] || "").split(";")[0].trim();
  if (phone) action(T.call, `tel:${phone.replace(/[^+\d]/g, "")}`, ICON.phone);
  if (/^(node|way|relation)\/\d+$/.test(p.id || "")) action(T.osm, `https://www.openstreetmap.org/${p.id}`, ICON.map);
  detailView.append(actions);

  const rows = el("ul", "lm-rows");
  const row = (icon, content) => {
    const li = el("li");
    const i = el("span", "lm-row-icon");
    i.innerHTML = svg(icon, 18);
    li.append(i, content);
    rows.append(li);
  };
  const addr = addressOf(t);
  if (addr) row(ICON.pin, el("span", "", {}, addr));
  if (p.opening_hours) {
    const box = el("span", "lm-hours");
    let text = p.opening_hours;
    try { if (oh) text = oh.prettifyValue({ conf: { locale: "en" } }); } catch {}
    for (const part of text.split(/\s*;\s*/)) box.append(el("span", "", {}, part));
    row(ICON.clock, box);
  }
  if (web) row(ICON.globe, el("a", "", { href: web, target: "_blank", rel: "noopener nofollow" }, shortURL(web)));
  if (phone) row(ICON.phone, el("a", "", { href: `tel:${phone.replace(/[^+\d]/g, "")}` }, phone));
  const mail = (t.email || t["contact:email"] || "").split(";")[0].trim();
  if (/^[^@\s]+@[^@\s]+$/.test(mail)) row(ICON.mail, el("a", "", { href: `mailto:${mail}` }, mail));
  if (t.wheelchair) row(ICON.wheel, el("span", "", {}, `Wheelchair: ${human(t.wheelchair)}`));
  if (t.description) row(ICON.info, el("span", "", {}, t.description));
  const wp = /^([a-z-]{2,12}):(.+)$/.exec(t.wikipedia || "");
  if (wp) row(ICON.book, el("a", "", { href: `https://${wp[1]}.wikipedia.org/wiki/${encodeURIComponent(wp[2].replace(/ /g, "_"))}`, target: "_blank", rel: "noopener" }, `Wikipedia: ${wp[2]}`));
  if (rows.children.length) detailView.append(rows);

  const keys = Object.keys(t);
  if (keys.length) {
    const all = el("details", "lm-tags");
    all.append(el("summary", "", {}, T.allTags));
    const dl = el("dl");
    for (const key of keys) dl.append(el("dt", "", {}, key), el("dd", "", {}, t[key]));
    if (p.opening_hours) dl.append(el("dt", "", {}, "opening_hours"), el("dd", "", {}, p.opening_hours));
    all.append(dl);
    detailView.append(all);
  }
}

// ------------------------------------------------------------------ what is here
// A tap on the map, not on a result: a pin, the address and the named things around it.
// The first tap only closes the search box when it is open. A tapped symbol of the map
// opens that place when the answer has it.
let hereCtl = null;
map.on("click", (e) => {
  if (e.defaultPrevented || nav.on) return;
  if (document.activeElement === input || !list.hidden) {
    closeList();
    input.blur();
    setSheet(state.answer || !hereView.hidden ? "half" : "peek");
    return;
  }
  const hit = resultAt(e.point);
  if (!routeView.hidden) {
    const c = hit && center(hit);
    return setFrom(c ? { ll: c, label: hit.properties.name || T.mapPoint }
      : { ll: [+e.lngLat.lng.toFixed(5), +e.lngLat.lat.toFixed(5)], label: symbolAt(e.point) || T.mapPoint });
  }
  if (hit) return select(hit, { fly: false });
  here(e.lngLat, symbolAt(e.point));
});
// resultAt is the result nearest to a point of the screen, within the size of a finger
// on a phone (24 px) or of a pointer (12 px); null when none is that near
function resultAt(pt) {
  const r = phone() ? 24 : 12;
  const layers = ["lm-point", "lm-line", "lm-fill"].filter((l) => map.getLayer(l));
  let best = null, bestD = Infinity;
  for (const f of map.queryRenderedFeatures([[pt.x - r, pt.y - r], [pt.x + r, pt.y + r]], { layers })) {
    const full = state.byId.get(f.properties.id);
    if (!full) continue;
    let d = 0;
    if (f.geometry.type === "Point") {
      const q = map.project(f.geometry.coordinates);
      d = Math.hypot(q.x - pt.x, q.y - pt.y);
    }
    if (d < bestD) { best = full; bestD = d; }
  }
  return bestD <= r * 1.42 ? best : null;
}
// symbolAt is the name of a symbol of the map (a shop, a stop) under a point
function symbolAt(pt) {
  const layers = (map.getStyle()?.layers || []).filter((l) => l.type === "symbol" && /poi|pois/i.test(l.id)).map((l) => l.id);
  const f = layers.length ? map.queryRenderedFeatures([[pt.x - 6, pt.y - 6], [pt.x + 6, pt.y + 6]], { layers })[0] : null;
  return f?.properties?.name || "";
}
function hereDone() {
  hereCtl?.abort();
  setSource("pin", empty);
  setSource("area", empty);
}
async function here(ll, symbol) {
  hereCtl?.abort();
  const ctl = hereCtl = new AbortController();
  const lat = +ll.lat.toFixed(5), lon = +ll.lng.toFixed(5);
  setSource("pin", one({ type: "Feature", geometry: { type: "Point", coordinates: [lon, lat] }, properties: {} }));
  setSource("area", empty);
  setSource("selected", empty);
  state.selected = null;
  hereView.innerHTML = "";
  if (state.answer) {
    const back = el("button", "lm-back", { type: "button" });
    back.innerHTML = svg(ICON.back, 18);
    back.append(T.back);
    back.addEventListener("click", () => { hereDone(); showView(listView); });
    hereView.append(back);
  }
  const h = el("h2", "lm-dname", {}, symbol || T.here);
  const sub = el("p", "lm-dsub", {}, `${lat.toFixed(5)}, ${lon.toFixed(5)}`);
  const note = el("p", "lm-notes lm-here-note", {}, T.hereLooking);
  hereView.append(h, sub);
  if (ROUTE) {
    const acts = el("div", "lm-actions lm-here-actions");
    acts.append(routeButton(() => directions({ ll: [lon, lat], label: h.textContent })));
    hereView.append(acts);
  }
  hereView.append(note);
  showView(hereView);
  setSheet("half");
  let d;
  try {
    const r = await fetch(`${API}/here?${new URLSearchParams({ lat, lon, zoom: String(Math.round(map.getZoom())) })}`, { signal: ctl.signal });
    d = await r.json().catch(() => ({}));
    if (!r.ok) {
      note.textContent = problem(d);
      return;
    }
  } catch (err) {
    if (err.name !== "AbortError") note.textContent = T.failed;
    return;
  }
  if (ctl.signal.aborted) return;
  const fs = d.results?.features || [];
  for (const f of fs) state.byId.set(f.properties.id, f);
  // the area tapped (a country far out, a lake or a park close in) leads when the map is out
  // far; close in the address leads, and the area follows
  const area = d.area, zoom = map.getZoom();
  setSource("area", d.lines || empty);
  const coords = `${lat.toFixed(5)}, ${lon.toFixed(5)}`;
  if (area && (zoom < 14 || !d.place)) {
    h.textContent = area.label;
    sub.textContent = [area.detail, coords].filter(Boolean).join(" · ");
    if (d.place && d.place.label !== area.label) sub.after(el("p", "lm-dsub", {}, d.place.label));
  } else {
    h.textContent = d.place?.label || symbol || T.here;
    sub.textContent = [d.place?.detail, coords].filter(Boolean).join(" · ");
    if (area) sub.after(el("p", "lm-dsub lm-in-area", {}, T.inArea(area.label, area.detail)));
  }
  note.textContent = fs.length ? "" : T.hereNothing;
  note.hidden = fs.length > 0;
  if (fs.length) {
    hereView.append(el("p", "lm-count", {}, T.around));
    const ol = el("ol", "lm-results");
    for (const f of fs) ol.append(card(f, { fromHere: true }));
    hereView.append(ol);
    if (fs.some((f) => f.properties.opening_hours)) loadHours().then(() => markHours(ol));
  }
  // the symbol tapped: its page at once
  const hit = symbol && fs.find((f) => fold(f.properties.name || "") === fold(symbol));
  if (hit) select(hit, { push: false, fly: false });
}

// ------------------------------------------------------------------ directions
// From the page of a place or from a point of the map: to there, from the location of the
// visitor, or from a tap on the map. The line and the steps come from data-route.
const MODE_ICON = { car: ICON.car, bike: ICON.bike, walk: ICON.walk };
const route = { from: null, to: null, mode: ["car", "bike", "walk"].includes(store.get("hm-mode")) ? store.get("hm-mode") : "car", back: null, ctl: null };
function routeButton(go) {
  const b = el("button", "lm-action", { type: "button" });
  b.innerHTML = svg(ICON.route, 18);
  b.append(el("span", "", {}, T.directions));
  b.addEventListener("click", go);
  return b;
}
function routeDone() {
  route.ctl?.abort();
  setSource("route", empty);
}
function drawEnds(line) {
  const fs = [];
  if (line) fs.push({ type: "Feature", geometry: { type: "LineString", coordinates: line }, properties: {} });
  if (route.from) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: route.from.ll }, properties: { end: "from" } });
  if (route.to) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: route.to.ll }, properties: { end: "to" } });
  setSource("route", { type: "FeatureCollection", features: fs });
}
async function directions(to) {
  route.back = [listView, hereView, detailView].find((v) => !v.hidden) || listView;
  route.to = to;
  route.from = state.me ? { ll: state.me.at2, label: T.myPlace, me: true } : null;
  hereDone();
  showView(routeView);
  setSheet("half");
  renderRoute();
  if (!route.from) { // ask the browser once; the visitor can tap the map instead
    const me = await locate();
    if (me && !route.from && !routeView.hidden) setFrom({ ll: state.me.at2, label: T.myPlace, me: true });
  } else findRoute();
}
function setFrom(from) {
  route.from = from;
  renderRoute();
  findRoute();
}
const routeHead = el("div", "lm-route-head");
const routeMsg = el("p", "lm-notes lm-route-msg", { role: "status", "aria-live": "polite" });
const routeSum = el("p", "lm-route-sum");
const routeSteps = el("ol", "lm-steps");
function renderRoute() {
  routeView.innerHTML = "";
  routeHead.innerHTML = "";
  const back = el("button", "lm-back", { type: "button" });
  back.innerHTML = svg(ICON.back, 18);
  back.append(route.back === detailView ? (state.selected?.properties.name || T.back) : route.back === hereView ? T.here : T.back);
  back.addEventListener("click", () => showView(route.back || listView));
  const modes = el("div", "lm-modes", { role: "radiogroup", "aria-label": T.directions });
  for (const [m, label] of T.modes) {
    const b = el("button", "lm-mode", { type: "button", role: "radio", "aria-checked": String(m === route.mode), title: label });
    b.innerHTML = svg(MODE_ICON[m], 18);
    b.append(el("span", "", {}, label));
    b.addEventListener("click", () => {
      if (route.mode === m) return;
      route.mode = m;
      store.set("hm-mode", m);
      renderRoute();
      findRoute();
    });
    modes.append(b);
  }
  const ends = el("div", "lm-ends");
  const end = (name, v, cls) => {
    const row = el("div", `lm-end ${cls}`);
    row.append(el("span", "lm-end-dot"), el("span", "lm-end-name", {}, name));
    row.append(el("span", `lm-end-label${v ? "" : " lm-end-none"}`, {}, v ? v.label : "–"));
    return row;
  };
  ends.append(end(T.from, route.from, "lm-end-from"), end(T.to, route.to, "lm-end-to"));
  const swap = el("button", "lm-icon-btn lm-swap", { type: "button", title: T.swap, "aria-label": T.swap });
  swap.innerHTML = svg(ICON.swap, 18);
  swap.disabled = !route.from || !route.to;
  swap.addEventListener("click", () => {
    [route.from, route.to] = [route.to, route.from];
    renderRoute();
    findRoute();
  });
  ends.append(swap);
  routeHead.append(modes, ends);
  if (!route.from?.me && navigator.geolocation) {
    const use = el("button", "lm-chip lm-use-me", { type: "button" }, T.useLocation);
    use.addEventListener("click", async () => {
      const me = await locate();
      if (me) setFrom({ ll: state.me.at2, label: T.myPlace, me: true });
      else routeMsg.textContent = T.locBlocked;
    });
    routeHead.append(use);
  }
  routeMsg.textContent = route.from ? "" : T.tapStart;
  routeSum.textContent = "";
  route.answer = null;
  routeSteps.innerHTML = "";
  routeView.append(back, el("h2", "lm-dname", {}, T.directions), routeHead, routeMsg, routeSum, routeSteps);
  drawEnds(null);
}
async function findRoute() {
  if (!route.from || !route.to) return;
  route.ctl?.abort();
  const ctl = route.ctl = new AbortController();
  const ll = (v) => `${v.ll[1].toFixed(5)},${v.ll[0].toFixed(5)}`;
  routeMsg.textContent = T.routing;
  routeSum.textContent = "";
  routeSteps.innerHTML = "";
  routeView.querySelector(".lm-nav-go")?.remove();
  routeView.querySelector(".lm-elev")?.remove();
  showOnRoute(null);
  route.answer = null;
  drawEnds(null);
  let d;
  try {
    const r = await fetch(`${ROUTE}?${new URLSearchParams({ from: ll(route.from), to: ll(route.to), mode: route.mode, lang: LANG })}`, { signal: ctl.signal });
    d = await r.json().catch(() => ({}));
    if (ctl.signal.aborted) return;
    if (!r.ok) {
      routeMsg.textContent = d.error?.message || T.routeFailed;
      return;
    }
  } catch (err) {
    if (err.name !== "AbortError") routeMsg.textContent = T.routeFailed;
    return;
  }
  routeMsg.textContent = "";
  route.answer = d;
  routeSum.textContent = `${T.duration(d.s)} · ${fmtKm(d.km)}`;
  const go = el("div", "lm-nav-go");
  const startBtn = el("button", "lm-nav-start", { type: "button" });
  startBtn.innerHTML = svg(ICON.navigate, 18);
  startBtn.append(T.start);
  startBtn.addEventListener("click", () => navStart(false));
  const simBtn = el("button", "lm-chip", { type: "button", title: T.previewNote }, T.preview);
  simBtn.addEventListener("click", () => navStart(true));
  go.append(startBtn, simBtn);
  routeSum.after(go);
  if (d.elev?.pts?.length > 1) go.after(heightChart(d.elev, { onPoint: showOnRoute }).node);
  loadAvatar();
  if (SAY) fetchSay(d.steps[0]?.pre || d.steps[0]?.text); // the first sentence, ready for Start
  for (const st of d.steps) {
    const li = el("li", "lm-step");
    const b = el("button", "", { type: "button" });
    b.append(el("span", "lm-step-text", {}, st.text));
    if (st.km > 0) b.append(el("span", "lm-step-km", {}, fmtKm(st.km)));
    b.addEventListener("click", () => fit(() => map.easeTo({ center: st.at, zoom: Math.max(map.getZoom(), 16), duration: 600, padding: pad() })));
    li.append(b);
    routeSteps.append(li);
  }
  drawEnds(d.line);
  const b = extent({ type: "LineString", coordinates: d.line });
  if (b) fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: 16, duration: 700 }));
}

// ------------------------------------------------------------------ height profile
// The heights along a route by bike or on foot (elev of the route answer: up, down, min, max, and
// pts as [km, m]), coloured by how steep each part is. Under a finger or the mouse it shows the
// height and the distance there, and the point on the map.
const GRADES = [[3, "#37b24d"], [6, "#f2b705"], [10, "#f08c00"], [Infinity, "#e03131"]];
let chartN = 0;
function heightChart(e, { compact = false, onPoint } = {}) {
  const pts = e.pts, km = pts[pts.length - 1][0] || 1;
  const lo = e.min - Math.max(5, (e.max - e.min) * 0.12), hi = e.max + Math.max(5, (e.max - e.min) * 0.08);
  const W = 1000, H = 100, X = (k) => (k / km) * W, Y = (h) => H - ((h - lo) / (hi - lo)) * H;
  // the steepness over at least 100 m, for the colour of each part
  const stops = [];
  let j = 0;
  for (let i = 0; i < pts.length - 1; i++) {
    while (j < pts.length - 1 && pts[j][0] - pts[i][0] < 0.1) j++;
    const dk = (pts[j][0] - pts[i][0]) * 1000;
    const g = dk > 0 ? Math.abs(pts[j][1] - pts[i][1]) / dk * 100 : 0;
    const c = GRADES.find(([max]) => g < max)[1];
    const at = ((pts[i][0] / km) * 100).toFixed(2), next = ((pts[i + 1][0] / km) * 100).toFixed(2);
    stops.push(`<stop offset="${at}%" stop-color="${c}"/><stop offset="${next}%" stop-color="${c}"/>`);
  }
  const id = `lm-elev-${++chartN}`;
  const line = pts.map((p, i) => `${i ? "L" : "M"}${X(p[0]).toFixed(1)} ${Y(p[1]).toFixed(1)}`).join("");
  const box = el("div", `lm-elev${compact ? " lm-elev-compact" : ""}`);
  const plot = el("div", "lm-elev-plot", { role: "img", "aria-label": `${T.heights}: ${T.up(e.up)}, ${T.down(e.down)}, ${e.min}–${e.max} m` });
  plot.innerHTML = `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" aria-hidden="true"><defs><linearGradient id="${id}" x1="0" x2="1" y1="0" y2="0">${stops.join("")}</linearGradient></defs>` +
    `<path d="${line}L${W} ${H}L0 ${H}Z" fill="url(#${id})" fill-opacity=".38"/>` +
    `<path d="${line}" fill="none" stroke="url(#${id})" stroke-width="2.5" vector-effect="non-scaling-stroke" stroke-linejoin="round"/></svg>`;
  const cursor = el("div", "lm-elev-cursor"), dot = el("div", "lm-elev-dot");
  cursor.hidden = dot.hidden = true;
  plot.append(cursor, dot);
  const heightAt = (k) => {
    let i = 0;
    while (i < pts.length - 2 && pts[i + 1][0] < k) i++;
    const a = pts[i], b = pts[i + 1], t = b[0] > a[0] ? Math.max(0, Math.min(1, (k - a[0]) / (b[0] - a[0]))) : 0;
    return a[1] + (b[1] - a[1]) * t;
  };
  const mark = (k) => { // a point of the profile, km from the start; null hides it
    cursor.hidden = dot.hidden = k == null;
    if (k == null) return;
    k = Math.max(0, Math.min(km, k));
    const x = `${(k / km) * 100}%`;
    cursor.style.left = dot.style.left = x;
    dot.style.top = `${(Y(heightAt(k)) / H) * 100}%`;
    return heightAt(k);
  };
  if (compact) {
    const left = el("div", "lm-elev-left");
    box.append(plot, left);
    // the climb from a point to the end
    const rest = [0];
    for (let i = pts.length - 2; i >= 0; i--) rest.unshift(rest[0] + Math.max(0, pts[i + 1][1] - pts[i][1]));
    return { node: box, km, mark(k) {
      mark(k);
      let i = 0;
      while (i < pts.length - 1 && pts[i][0] < k) i++;
      const m = Math.round(rest[i] * (e.up / (rest[0] || 1)) / 10) * 10; // scaled to the climb of the API
      left.textContent = m > 0 ? T.climbLeft(m) : "";
    } };
  }
  const head = el("div", "lm-elev-head");
  const read = el("span", "lm-elev-read");
  head.append(el("span", "lm-elev-up", {}, `↗ ${T.up(e.up)}`), el("span", "lm-elev-down", {}, `↘ ${T.down(e.down)}`), read);
  const axis = el("div", "lm-elev-axis");
  axis.append(el("span", "", {}, "0"), el("span", "", {}, fmtKm(km)));
  const scale = el("div", "lm-elev-scale");
  scale.append(el("span", "", {}, `${e.max} m`), el("span", "", {}, `${e.min} m`));
  const frame = el("div", "lm-elev-frame");
  frame.append(scale, plot);
  box.append(head, frame, axis);
  const at = (ev) => {
    const r = plot.getBoundingClientRect();
    const k = Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width)) * km;
    const h = mark(k);
    read.textContent = `${Math.round(h)} m · ${fmtKm(k)}`;
    onPoint?.(k / km);
  };
  plot.addEventListener("pointermove", at);
  plot.addEventListener("pointerdown", at);
  plot.addEventListener("pointerleave", () => {
    mark(null);
    read.textContent = "";
    onPoint?.(null);
  });
  return { node: box, km, mark };
}
// the distance along a line at each of its points, in metres
function cumOf(line) {
  const cum = [0];
  for (let i = 1; i < line.length; i++) {
    const [ax, ay] = flat(line[i - 1], line[i][1]), [bx, by] = flat(line[i], line[i][1]);
    cum.push(cum[i - 1] + Math.hypot(bx - ax, by - ay));
  }
  return cum;
}
// the point at a distance along a line
function pointOn(line, cum, along, from = 0) {
  let i = Math.max(0, Math.min(from, cum.length - 2));
  while (i < cum.length - 2 && cum[i + 1] < along) i++;
  const seg = cum[i + 1] - cum[i] || 1, t = Math.max(0, Math.min(1, (along - cum[i]) / seg));
  const a = line[i], b = line[i + 1];
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t];
}
let elevMarker = null;
function showOnRoute(frac) {
  const d = route.answer;
  if (frac == null || !d) { elevMarker?.remove(); elevMarker = null; return; }
  d.cum ||= cumOf(d.line);
  const ll = pointOn(d.line, d.cum, frac * d.cum[d.cum.length - 1]);
  if (!elevMarker) elevMarker = new maplibregl.Marker({ element: el("div", "lm-elev-pin") });
  elevMarker.setLngLat(ll).addTo(map);
}

// ------------------------------------------------------------------ navigation
// Turn by turn along the route of the directions: the map tilted and turned with the way, a
// sprite for the car, the bike or the walker, the next turn with its distance (and the sign of a
// motorway by car), the speed limit, the spoken instructions of Valhalla, a new route when the
// visitor leaves this one. Start follows the location of the browser; Preview drives the route
// at three times its speed.
const SPRITE = {
  car: '<svg viewBox="0 0 40 40" width="40" height="40"><circle cx="20" cy="20" r="19" fill="#1a73e8" fill-opacity=".18"/>' +
    '<rect x="12.5" y="6" width="15" height="28" rx="6" fill="#1a73e8" stroke="#fff" stroke-width="2"/>' +
    '<path d="M15 14.5q5-3 10 0l-1.2 4h-7.6z" fill="#cfe3ff"/><path d="M16 28.5h8l1 2.5h-10z" fill="#cfe3ff"/>' +
    '<rect x="11" y="11" width="2" height="3.5" rx="1" fill="#0b3d91"/><rect x="27" y="11" width="2" height="3.5" rx="1" fill="#0b3d91"/></svg>',
  bike: '<svg viewBox="0 0 40 40" width="40" height="40"><circle cx="20" cy="20" r="19" fill="#1a73e8" fill-opacity=".18"/>' +
    '<rect x="18.5" y="5" width="3" height="10" rx="1.5" fill="#111827"/><rect x="18.5" y="25" width="3" height="10" rx="1.5" fill="#111827"/>' +
    '<path d="M12 13h16" stroke="#111827" stroke-width="2.4" stroke-linecap="round"/>' +
    '<path d="M20 13v12" stroke="#1a73e8" stroke-width="3"/><ellipse cx="20" cy="20" rx="6" ry="4.5" fill="#1a73e8" stroke="#fff" stroke-width="1.6"/>' +
    '<circle cx="20" cy="18.5" r="3" fill="#f2c9a0" stroke="#fff" stroke-width="1"/></svg>',
  walk: '<svg viewBox="0 0 40 40" width="40" height="40"><circle cx="20" cy="20" r="19" fill="#1a73e8" fill-opacity=".18"/>' +
    '<path d="M20 4 25 11h-10z" fill="#1a73e8"/><ellipse cx="20" cy="21" rx="9" ry="5" fill="#1a73e8" stroke="#fff" stroke-width="2"/>' +
    '<circle cx="20" cy="21" r="4" fill="#f2c9a0" stroke="#fff" stroke-width="1.5"/></svg>',
};
// the traveller on the map: in 3D (avatar.js, three.js) where WebGL lets it, else the sprite
let AVATAR = null, avatarLoading = null;
const loadAvatar = () => (avatarLoading ||= import("./avatar.js").then((m) => { AVATAR = m; }).catch(() => {}));
function makeAvatar(mode) {
  if (AVATAR) {
    try {
      const a = AVATAR.avatar(map, maplibregl, mode);
      return { is3d: true, set(ll, h, ms, sp) { this.at = ll; this.heading = h; a.set(ll, h, ms, sp); }, remove: () => a.remove() };
    } catch {}
  }
  const sprite = el("div", "lm-sprite");
  sprite.innerHTML = SPRITE[mode];
  const m = new maplibregl.Marker({ element: sprite, rotationAlignment: "map", pitchAlignment: "map" });
  return { is3d: false, set(ll, h) { this.at = ll; this.heading = h; m.setLngLat(ll).setRotation(h).addTo(map); }, remove: () => m.remove() };
}
// arrows for the maneuver types of Valhalla
const ARROW = {
  straight: "M12 21V4M6 10l6-6 6 6",
  right: "M7 21v-8a4 4 0 0 1 4-4h9M15 4l5 5-5 5",
  left: "M17 21v-8a4 4 0 0 0-4-4H4M9 4 4 9l5 5",
  slightRight: "M9 21v-7l8-8M11 5h7v7",
  slightLeft: "M15 21v-7L7 6M13 5H6v7",
  sharpRight: "M7 3v9l10 8M18 13v8h-8",
  sharpLeft: "M17 3v9L7 20M6 13v8h8",
  uturnLeft: "M16 21V9a4 4 0 0 0-8 0v7M4 13l4 4 4-4",
  uturnRight: "M8 21V9a4 4 0 0 1 8 0v7M12 13l4 4 4-4",
  roundabout: "M12 21v-6M12 15a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7M14.5 9.5 19 5M14 5h5v5",
  arrive: "M12 21s-6-5.4-6-10a6 6 0 0 1 12 0c0 4.6-6 10-6 10zM12 13a2 2 0 1 0 0-4 2 2 0 0 0 0 4z",
  merge: "M12 21v-8L7 4M12 13l5-9M17 9V4h-5",
};
const arrowOf = (type) => ({
  2: "right", 3: "left", 4: "arrive", 5: "arrive", 6: "arrive", 9: "slightRight", 10: "right", 11: "sharpRight",
  12: "uturnRight", 13: "uturnLeft", 14: "sharpLeft", 15: "left", 16: "slightLeft", 18: "slightRight", 19: "slightLeft",
  20: "slightRight", 21: "slightLeft", 23: "slightRight", 24: "slightLeft", 25: "merge", 26: "roundabout", 27: "roundabout",
  37: "merge", 38: "merge",
}[type] || "straight");
const arrowSVG = (type, size) =>
  `<svg viewBox="0 0 24 24" width="${size}" height="${size}" aria-hidden="true"><path d="${ARROW[arrowOf(type)]}" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
// the colours of route numbers on a sign: motorways (A) red, European routes green, others blue
const shieldClass = (r) => (/^A\s?\d/.test(r) ? "lm-shield-a" : /^E\s?\d/.test(r) ? "lm-shield-e" : "lm-shield");
// how near a turn is said first (alert) and again (pre), and how far off the line is off the route, in metres
const NAV = {
  car: { alert: 700, pre: 160, off: 45, zoom: 16.6, arrive: 40 },
  bike: { alert: 250, pre: 50, off: 30, zoom: 17.4, arrive: 20 },
  walk: { alert: 120, pre: 25, off: 25, zoom: 18, arrive: 15 },
};
const nav = { on: false, sim: false, follow: true, muted: store.get("hm-mute") === "1", spoken: new Set(), off: 0, rerouting: false };

const navBox = el("div", "lm-nav", { "aria-live": "polite" });
navBox.hidden = true;
const navBanner = el("div", "lm-nav-banner");
const navSign = el("div", "lm-nav-sign");
const navThen = el("div", "lm-nav-then");
const navLimit = el("div", "lm-nav-limit");
const navBottom = el("div", "lm-nav-bottom");
const navEta = el("div", "lm-nav-eta");
const navElev = el("div", "lm-nav-elev");
navElev.hidden = true;
const navBtn = (icon, label, cls = "") => {
  const b = el("button", `lm-nav-btn ${cls}`.trim(), { type: "button", title: label, "aria-label": label });
  b.innerHTML = svg(icon, 22);
  return b;
};
const navEnd = navBtn(ICON.close, T.endNav, "lm-nav-x");
const navMute = navBtn(ICON.sound, T.mute);
const navOverview = navBtn(ICON.overview, T.overview);
const navCenter = el("button", "lm-nav-center", { type: "button" });
navCenter.innerHTML = svg(ICON.navigate, 16);
navCenter.append(T.recenter);
navCenter.hidden = true;
navBottom.append(navMute, navEta, navOverview, navEnd);
navBox.append(navBanner, navSign, navThen, navLimit, navElev, navCenter, navBottom);
mapEl.append(navBox);

// distances on the earth, in metres, near a point: x east, y north
const M = 111320;
const flat = (a, lat0) => [a[0] * M * Math.cos((lat0 * Math.PI) / 180), a[1] * M];
const bearingOf = (a, b) => {
  const [ax, ay] = flat(a, a[1]), [bx, by] = flat(b, a[1]);
  return (Math.atan2(bx - ax, by - ay) * 180) / Math.PI;
};
// prepare a route answer for following: the distance along the line at each point
function navLoad(d) {
  nav.d = d;
  nav.cum = cumOf(d.line);
  navElev.innerHTML = "";
  nav.elev = d.elev?.pts?.length > 1 ? heightChart(d.elev, { compact: true }) : null;
  navElev.hidden = !nav.elev;
  if (nav.elev) navElev.append(nav.elev.node);
  nav.idx = 0;
  nav.spoken = new Set();
  nav.off = 0;
}
// snap a point to the line: the nearest point from a little behind the last one to well ahead
function snap(p, wide) {
  const line = nav.d.line, cum = nav.cum;
  const from = wide ? 0 : Math.max(0, nav.idx - 3), to = wide ? line.length - 1 : Math.min(line.length - 1, nav.idx + 400);
  let best = { dist: Infinity, i: nav.idx, t: 0 };
  const P = flat(p, p[1]);
  for (let i = from; i < to; i++) {
    const A = flat(line[i], p[1]), B = flat(line[i + 1], p[1]);
    const dx = B[0] - A[0], dy = B[1] - A[1], len2 = dx * dx + dy * dy;
    const t = len2 ? Math.max(0, Math.min(1, ((P[0] - A[0]) * dx + (P[1] - A[1]) * dy) / len2)) : 0;
    const dist = Math.hypot(A[0] + t * dx - P[0], A[1] + t * dy - P[1]);
    if (dist < best.dist) best = { dist, i, t };
  }
  const a = line[best.i], b = line[Math.min(best.i + 1, line.length - 1)];
  return { ...best, ll: [a[0] + (b[0] - a[0]) * best.t, a[1] + (b[1] - a[1]) * best.t], along: cum[best.i] + (cum[best.i + 1] - cum[best.i] || 0) * best.t,
    heading: bearingOf(a, b) };
}
// the point at a distance along the line (Preview)
const pointAt = (along) => pointOn(nav.d.line, nav.cum, along, nav.idx);
// ------------------------------------------------------------------ the voice
// The instructions are spoken by the voices of this server (data-say: Piper, in German, English,
// French and Italian), fetched up to three turns ahead; else, or when one is late, by the best voice
// the browser has for the language.
const SAY = root.dataset.say || "";
const LOCALES = { en: "en-US", de: "de-DE", fr: "fr-FR", it: "it-IT", es: "es-ES", nl: "nl-NL", pt: "pt-PT" };
const STEP_LANG = LOCALES[LANG] ? LANG : "en"; // the language of the texts of Valhalla
const OUR_VOICES = ["de", "en", "fr", "it"];
const hasSpeech = "speechSynthesis" in window;
let browserVoices = [];
if (hasSpeech) {
  const list = () => { browserVoices = speechSynthesis.getVoices(); };
  list();
  speechSynthesis.addEventListener?.("voiceschanged", list);
}
// the best voice of the browser: natural voices of Edge, enhanced voices of Apple, Google voices
// of Chrome; not the novelty voices of Apple or eSpeak; the region of the browser when it can
function bestVoice(lang) {
  const want = (navigator.language || "").toLowerCase(), locale = (LOCALES[lang] || "en-US").toLowerCase();
  let best = null, top = -Infinity;
  for (const v of browserVoices) {
    const vl = v.lang.toLowerCase().replace("_", "-");
    if (vl.slice(0, 2) !== lang) continue;
    let sc = 0;
    if (/natural|neural|online/i.test(v.name)) sc += 6;
    if (/premium|enhanced|siri/i.test(v.name)) sc += 5;
    if (/google/i.test(v.name)) sc += 4;
    if (/compact|espeak|eloquence/i.test(v.name)) sc -= 4;
    if (/^(albert|bad news|bahh|bells|boing|bubbles|cellos|good news|jester|organ|superstar|trinoids|whisper|wobble|zarvox|fred|junior|ralph|kathy|grandma|grandpa|rocko|shelley|flo|eddy|reed|sandy)\b/i.test(v.name)) sc -= 8;
    if (vl === want) sc += 2;
    else if (vl === locale) sc += 1;
    if (v.default) sc += 0.5;
    if (sc > top) { top = sc; best = v; }
  }
  return best;
}
function browserSay(text, lang) {
  if (!hasSpeech) return;
  const u = new SpeechSynthesisUtterance(text);
  u.lang = LOCALES[lang] || "en-US";
  const v = bestVoice(lang);
  if (v) u.voice = v;
  speechSynthesis.cancel();
  speechSynthesis.speak(u);
}
const player = new Audio();
const sayCache = new Map(); // lang and text → a promise of a blob URL, or null
function fetchSay(text, lang = STEP_LANG) {
  if (!SAY || !text || !OUR_VOICES.includes(lang)) return null;
  const key = `${lang}\n${text}`;
  if (!sayCache.has(key)) {
    sayCache.set(key, fetch(`${SAY}?${new URLSearchParams({ lang, text })}`)
      .then((r) => (r.ok ? r.blob() : null)).then((b) => (b ? URL.createObjectURL(b) : null)).catch(() => null));
  }
  return sayCache.get(key);
}
// the sentences of the next turns, before they are needed
function preSay(steps) {
  for (const st of steps) if (st) for (const t of [st.alert, st.pre || st.text]) fetchSay(t);
}
let sayN = 0;
async function say1(text, lang = STEP_LANG) {
  if (nav.muted || !text) return;
  const n = ++sayN;
  player.pause();
  if (hasSpeech) speechSynthesis.cancel();
  const p = fetchSay(text, lang);
  if (p) {
    const url = await Promise.race([p, new Promise((done) => setTimeout(() => done(null), 2500))]);
    if (n !== sayN || nav.muted) return;
    if (url) {
      player.src = url;
      try {
        await player.play();
        return;
      } catch {}
    }
  }
  browserSay(text, lang);
}
// inside the tap on Start: a silent sound and an empty word, so that the browser lets the page speak later
function unlockSound() {
  const n = 800, b = new DataView(new ArrayBuffer(44 + n * 2));
  const str = (o, t) => [...t].forEach((c, i) => b.setUint8(o + i, c.charCodeAt(0)));
  str(0, "RIFF"); b.setUint32(4, 36 + n * 2, true); str(8, "WAVEfmt "); b.setUint32(16, 16, true); b.setUint16(20, 1, true);
  b.setUint16(22, 1, true); b.setUint32(24, 8000, true); b.setUint32(28, 16000, true); b.setUint16(32, 2, true); b.setUint16(34, 16, true);
  str(36, "data"); b.setUint32(40, n * 2, true);
  player.src = URL.createObjectURL(new Blob([b], { type: "audio/wav" }));
  player.play().catch(() => {});
  if (hasSpeech) {
    const u = new SpeechSynthesisUtterance(" ");
    u.volume = 0;
    speechSynthesis.speak(u);
  }
}
function hush() {
  sayN++;
  player.pause();
  if (hasSpeech) speechSynthesis.cancel();
}
const clock = (s) => new Date(Date.now() + s * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
// the sign of the next junction, by car, in its last 2 km
function renderSign(st, toNext) {
  navSign.innerHTML = "";
  const show = route.mode === "car" && toNext < 2000 && (st.exit || st.toward?.length || st.routes?.length || st.exit_name);
  navSign.hidden = !show;
  if (!show) return;
  navSign.className = `lm-nav-sign ${st.highway || st.exit ? "lm-sign-motorway" : "lm-sign-road"}`;
  const top = el("div", "lm-sign-top");
  if (st.exit) top.append(el("span", "lm-sign-exit", {}, `${T.exit} ${st.exit}`));
  for (const r of (st.routes || []).slice(0, 3)) top.append(el("span", shieldClass(r), {}, r));
  if (top.children.length) navSign.append(top);
  if (st.exit_name) navSign.append(el("div", "lm-sign-name", {}, st.exit_name));
  for (const t of (st.toward || []).slice(0, 3)) navSign.append(el("div", "lm-sign-to", {}, t));
}
// the state of the trip at a point on the line
function navUpdate(p, rawPos, off = false) {
  const d = nav.d, steps = d.steps, cfg = NAV[route.mode];
  nav.idx = p.i;
  // the step being driven: the last that starts at or before this point
  let k = 0;
  while (k + 1 < steps.length && nav.cum[steps[k + 1].i] <= p.along + 1) k++;
  const next = steps[k + 1] || steps[k];
  const toNext = Math.max(0, nav.cum[Math.min(next.i, nav.cum.length - 1)] - p.along);
  const total = nav.cum[nav.cum.length - 1];
  const left = Math.max(0, total - p.along);
  // time left: the rest of this step by its share, and the steps after it
  const cur = steps[k], span = (nav.cum[next.i] - nav.cum[cur.i]) || 1;
  let s = (cur.s || 0) * Math.min(1, toNext / span);
  for (let j = k + 1; j < steps.length; j++) s += steps[j].s || 0;
  // the banner: the turn, how far, onto what
  navBanner.innerHTML = "";
  const arrow = el("div", "lm-nav-arrow");
  arrow.innerHTML = arrowSVG(next.type, 44);
  const txt = el("div", "lm-nav-text");
  txt.append(el("div", "lm-nav-dist", {}, fmtKm(toNext / 1000)));
  // the street to turn into; a bare route number ("1") says less than the instruction
  txt.append(el("div", "lm-nav-instr", {}, next.street && !/^[\d\s/]+$/.test(next.street) && next.type !== 4 ? next.street : next.text));
  if (next.roundabout) txt.lastChild.textContent = next.text;
  navBanner.append(arrow, txt);
  renderSign(next, toNext);
  // the turn after, when it comes soon after this one
  const after = steps[k + 2];
  navThen.innerHTML = "";
  navThen.hidden = !(after && toNext < 1000 && nav.cum[after.i] - nav.cum[next.i] < 250);
  if (!navThen.hidden) {
    navThen.append(T.then);
    const a = el("span", "lm-nav-then-arrow");
    a.innerHTML = arrowSVG(after.type, 20);
    navThen.append(a);
  }
  // the speed limit here, by car
  const lim = route.mode === "car" && (d.limits || []).find((l) => l[0] <= p.i && p.i < l[1]);
  navLimit.hidden = !lim;
  if (lim) {
    navLimit.textContent = lim[2];
    navLimit.title = navLimit.ariaLabel = T.speedLimit(lim[2]);
  }
  navEta.innerHTML = "";
  navEta.append(el("strong", "", {}, clock(s)), el("span", "", {}, `${T.duration(s)} · ${fmtKm(left / 1000)}`));
  // the spoken instructions: early, then at the turn; after the turn what comes
  const n = steps.indexOf(next);
  preSay([next, steps[k + 2], steps[k + 3]]);
  if (k === 0 && !nav.spoken.has("0")) {
    nav.spoken.add("0");
    say1(steps[0].pre || steps[0].text);
  } else if (toNext <= cfg.pre && !nav.spoken.has(`${n}p`)) {
    nav.spoken.add(`${n}p`).add(`${n}a`);
    say1(next.pre || next.text);
  } else if (toNext <= cfg.alert && toNext > cfg.pre * 1.8 && !nav.spoken.has(`${n}a`) && next.alert) {
    nav.spoken.add(`${n}a`);
    say1(next.alert);
  }
  // the camera and the sprite
  // off the route: the real position, and the heading of the device when it has one
  const at = off ? rawPos : p.ll, heading = off && rawPos.heading != null ? rawPos.heading : p.heading;
  nav.avatar?.set(at, heading, nav.sim ? 480 : 900, nav.speed || 0);
  if (nav.elev) nav.elev.mark((p.along / (total || 1)) * nav.elev.km);
  if (nav.follow) {
    map.easeTo({ center: at, bearing: heading, pitch: 60, zoom: cfg.zoom, padding: navPad(), duration: nav.sim ? 480 : 900, easing: (t) => t });
  }
  drawNavLine(p);
  if (left <= cfg.arrive) navArrive();
}
const navPad = () => {
  const h = mapEl.clientHeight;
  return { top: Math.round(h * 0.42), bottom: nav.elev ? 140 : 90, left: 20, right: 20 };
};
function drawNavLine(p) {
  const rest = [p.ll, ...nav.d.line.slice(p.i + 1)];
  setSource("route", { type: "FeatureCollection", features: [
    { type: "Feature", geometry: { type: "LineString", coordinates: rest }, properties: {} },
    { type: "Feature", geometry: { type: "Point", coordinates: route.to.ll }, properties: { end: "to" } },
  ] });
}
function navArrive() {
  if (nav.arrived) return;
  nav.arrived = true;
  stopTracking();
  navBanner.innerHTML = "";
  const arrow = el("div", "lm-nav-arrow");
  arrow.innerHTML = arrowSVG(4, 44);
  navBanner.append(arrow, el("div", "lm-nav-text lm-nav-instr", {}, T.arrived));
  navSign.hidden = navThen.hidden = navLimit.hidden = true;
  const last = nav.d.steps[nav.d.steps.length - 1];
  if (!nav.spoken.has("end")) {
    nav.spoken.add("end");
    if (last?.pre) say1(last.pre);
    else say1(T.arrived, "en");
  }
}
function stopTracking() {
  if (nav.watch != null) navigator.geolocation.clearWatch(nav.watch);
  nav.watch = null;
  clearInterval(nav.timer);
  nav.timer = 0;
}
// a fix of the location: on the line, or off it (then, a few times running, a new route)
function onFix(pos) {
  if (!nav.on || nav.arrived) return;
  nav.lastFix = pos;
  const ll = [pos.coords.longitude, pos.coords.latitude];
  const first = nav.first; // the first fix on a new route counts as on it: the route starts at the road nearest to it
  const p = snap(ll, first);
  nav.first = false;
  const cfg = NAV[route.mode], acc = Math.min(pos.coords.accuracy || 0, 60);
  if (!first && p.dist > cfg.off + acc) {
    nav.off++;
    const h = pos.coords.heading;
    ll.heading = h != null && !Number.isNaN(h) && pos.coords.speed > 1 ? h : null;
    navUpdate(p, ll, true);
    if (nav.off >= 3 && !nav.rerouting) reroute(ll);
    return;
  }
  nav.off = 0;
  // the speed for the wheels and legs: from the device, or from the last two fixes
  const t = pos.timestamp || Date.now();
  nav.speed = pos.coords.speed != null && !Number.isNaN(pos.coords.speed) ? pos.coords.speed
    : nav.lastAlong != null && t > nav.lastT ? Math.abs(p.along - nav.lastAlong) / ((t - nav.lastT) / 1000) : 0;
  nav.lastAlong = p.along;
  nav.lastT = t;
  navUpdate(p, ll);
}
async function reroute(ll) {
  nav.rerouting = true;
  navBanner.querySelector(".lm-nav-instr")?.replaceChildren(T.rerouting);
  say1(T.rerouting.replace("…", ""), "en");
  try {
    const q = new URLSearchParams({ from: `${ll[1].toFixed(5)},${ll[0].toFixed(5)}`, to: `${route.to.ll[1].toFixed(5)},${route.to.ll[0].toFixed(5)}`, mode: route.mode, lang: LANG });
    const r = await fetch(`${ROUTE}?${q}`);
    const d = await r.json();
    if (r.ok && nav.on) {
      navLoad(d);
      nav.first = true;
      if (nav.lastFix) onFix(nav.lastFix); // the banner at once, not at the next fix
    }
  } catch {}
  setTimeout(() => { nav.rerouting = false; }, 10e3); // not more often than every 10 s
}
async function navStart(sim) {
  if (!route.answer) return;
  if (!nav.muted) unlockSound();
  preSay(route.answer.steps.slice(0, 2));
  nav.on = true;
  nav.sim = sim;
  nav.follow = true;
  nav.arrived = false;
  nav.first = true;
  navLoad(route.answer);
  root.classList.add("lm-navigating");
  navBox.hidden = false;
  navCenter.hidden = true;
  navMute.innerHTML = svg(nav.muted ? ICON.muted : ICON.sound, 22);
  nav.speed = 0;
  nav.lastAlong = null;
  nav.avatar = makeAvatar(route.mode);
  if (!AVATAR) loadAvatar().then(() => { // the 3D traveller once its code is here
    if (!nav.on || !AVATAR || nav.avatar.is3d) return;
    const a = makeAvatar(route.mode);
    if (!a.is3d) return a.remove();
    a.set(nav.avatar.at, nav.avatar.heading || 0);
    nav.avatar.remove();
    nav.avatar = a;
  });
  nav.avatar.set(nav.d.line[0], bearingOf(nav.d.line[0], nav.d.line[1] || nav.d.line[0]));
  setTimeout(() => map.resize(), 0);
  try { nav.lock = await navigator.wakeLock?.request("screen"); } catch {}
  if (sim) {
    // Preview: along the line at three times the speed of each step
    let along = 0, last = performance.now();
    const speedAt = () => {
      const st = [...nav.d.steps].reverse().find((x) => nav.cum[x.i] <= along) || nav.d.steps[0];
      return st.s > 0 ? (st.km * 1000) / st.s : 10;
    };
    nav.timer = setInterval(() => {
      const now = performance.now();
      nav.speed = speedAt() * 3;
      along += nav.speed * ((now - last) / 1000);
      last = now;
      const total = nav.cum[nav.cum.length - 1];
      const ll = pointAt(Math.min(along, total));
      const p = snap(ll, false);
      navUpdate(p, null);
      if (along >= total) navArrive();
    }, 500);
    navUpdate(snap(nav.d.line[0], true), null);
  } else {
    navBanner.innerHTML = "";
    navBanner.append(el("div", "lm-nav-text lm-nav-instr", {}, T.waitGPS));
    navSign.hidden = navThen.hidden = navLimit.hidden = true;
    nav.watch = navigator.geolocation.watchPosition(onFix, (err) => {
      if (err.code === 1) {
        navBanner.innerHTML = "";
        navBanner.append(el("div", "lm-nav-text lm-nav-instr", {}, T.noGPS));
      }
    }, { enableHighAccuracy: true, maximumAge: 1000, timeout: 20000 });
  }
}
function navStop() {
  nav.on = false;
  stopTracking();
  hush();
  nav.lock?.release?.().catch(() => {});
  nav.lock = null;
  nav.avatar?.remove();
  nav.avatar = null;
  navElev.hidden = true;
  navBox.hidden = true;
  root.classList.remove("lm-navigating");
  setTimeout(() => {
    map.resize();
    drawEnds(route.answer?.line || null);
    const b = route.answer && extent({ type: "LineString", coordinates: route.answer.line });
    if (b) fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: 16, pitch: 0, bearing: 0, duration: 700 }));
  }, 0);
}
navEnd.addEventListener("click", navStop);
navMute.addEventListener("click", () => {
  nav.muted = !nav.muted;
  store.set("hm-mute", nav.muted ? "1" : "0");
  navMute.innerHTML = svg(nav.muted ? ICON.muted : ICON.sound, 22);
  navMute.title = navMute.ariaLabel = nav.muted ? T.unmute : T.mute;
  if (nav.muted) hush();
});
navOverview.addEventListener("click", () => {
  nav.follow = false;
  navCenter.hidden = false;
  const b = extent({ type: "LineString", coordinates: nav.d.line.slice(nav.idx) });
  if (b) map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: { top: 140, bottom: 110, left: 40, right: 40 }, pitch: 0, bearing: 0, maxZoom: 16, duration: 700 });
});
navCenter.addEventListener("click", () => { nav.follow = true; navCenter.hidden = true; });
// the visitor moves the map: it stops following until Re-centre
for (const ev of ["pointerdown", "wheel", "touchstart"]) {
  mapEl.addEventListener(ev, (e) => {
    if (!nav.on || navBox.contains(e.target)) return;
    nav.follow = false;
    navCenter.hidden = false;
  }, { passive: true, capture: true });
}

// ------------------------------------------------------------------ start
map.once("load", () => {
  const u = new URLSearchParams(location.search);
  const q = u.get("q");
  if (q) {
    const extra = Object.fromEntries(extraKeys.map((k) => [k, u.get(k) || ""]));
    run(q, extra, { push: false, sel: u.get("sel") });
  } else {
    intro();
    if (!linkedView) showMe(true);
  }
});
if (!root.dataset.key) say("The map is not configured on this server: MAPS_KEY is missing.", true);
