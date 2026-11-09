// hippomaps: a search box with suggestions, the results and the page of a place in a
// side bar, and the map beside it. The search is the API of overpass-search (data-api):
// /complete while typing, /query for the reading, /search for the results. Pictures and
// logos come from data-pictures, which reads Wikimedia Commons. Directions come from
// data-route (Valhalla, through this server) when the server has them. Text from OpenStreetMap
// only ever goes in as text, never as HTML.
import * as maplibregl from "./vendor/maplibre-gl-6.13.0/maplibre-gl.mjs";
import { languages, texts } from "./i18n.js";

maplibregl.setWorkerCount(Math.max(maplibregl.getWorkerCount(), 2));

const root = document.getElementById("app");
const side = root.querySelector(".lm-side");
const mapEl = root.querySelector(".lm-map");
const API = root.dataset.api.replace(/\/$/, "");
const PICTURES = root.dataset.pictures || "";
const ROUTE = root.dataset.route || "";
const ALONG = (ROUTE && root.dataset.along) || ""; // places along a route (Overpass, through this server)
// the language: chosen in the menu (hm-lang), else that of the browser. It sets the texts of the page
// (en, de, fr, it), the names on the map, the steps of directions and the voice.
// A change in the menu applies at once, without loading the page again (setLang).
const chosenLang = (() => { try { return localStorage.getItem("hm-lang") || ""; } catch { return ""; } })();
const langOf = (chosen) => chosen || (navigator.language || "en").slice(0, 2).toLowerCase();
let LANG = langOf(chosenLang);
let UI = languages.some(([k]) => k === LANG) ? LANG : "en";
document.documentElement.lang = UI;
const num = (n) => n.toLocaleString(UI);
const placeKeys = ["place", "place2", "region"];
const extraKeys = [...placeKeys, "what", "view"]; // view=1: the same search in the map view

const EN = {
  placeholder: "Search places, or what and where",
  // the hint of the empty search box: one of these at random; each runs on the search as written
  hints: ["restaurants near Lake Zurich", "gas stations along the A3", "bridges over the Rhine in Basel", "castles in Ticino",
    "mountain huts over 2500 m in Valais", "campsites by Lake Geneva", "beer gardens in Munich", "lighthouses in Brittany",
    "vegan restaurants in Zurich", "viewpoints near Lucerne", "drinking water in Geneva", "ice cream along the Limmat"],
  examples: ["cafés", "Migros", "museums in Bern", "playgrounds near me", "restaurants open now", "hotels along the Limmat in Zurich"],
  intro: "Search in plain words: a place, a chain, or what and where. A word alone searches the map view.",
  searching: "Searching…",
  locating: "Finding your location…",
  results: (n) => (n === 1 ? "1 result" : `${num(n)} results`),
  shown: (n, all) => `${num(n)} of ${num(all)} shown`,
  more: "Show more",
  noName: "Unnamed",
  back: "Results",
  close: "Close",
  save: "Save",
  saved: "Saved",
  starred: "Saved places",
  recent: "Recent searches",
  forgetRecent: "Forget recent searches",
  searchArea: "Search this area",
  myLocation: "Show my location",
  north: "Turn the map to north",
  inArea: (name, kind) => `In ${name}${kind ? ` (${kind})` : ""}`,
  here: "Here",
  around: "Around here",
  hereLooking: "Looking at this point…",
  mapLost: "The map stopped drawing in this browser: it took the graphics back. Reload the page; with 3D off the map needs less memory.",
  hereNothing: "Nothing with a name right here. Zoom in closer, or tap a symbol of the map.",
  navHereNothing: "Nothing with a name around this point.",
  locBlocked: "Location is blocked for this page. Allow it in the settings of the browser, then try again.",
  locFailed: "Your location could not be found.",
  locMissing: "This browser does not share a location.",
  otherReadings: "Or search for",
  otherPlaces: "Other places",
  tryHere: "Search here",
  query: "Overpass query",
  copy: "Copy",
  copied: "Copied",
  share: "Share",
  useAsTo: "Use as destination",
  useAsFrom: "Use as start",
  linkCopied: "Link copied",
  open: "Open",
  closed: "Closed",
  opens: (t) => `opens ${t}`,
  closes: (t) => `closes ${t}`,
  failed: "The search failed. Please try again.",
  tooLong: "The search took too long. Try a smaller place, or a less common feature.",
  errors: {
    zoom_in: "Zoom in to search the map view, or add a place to the search.",
    need_location: "Share your location, or zoom the map to where you are, or add a place to the search.",
    too_many: "Too many searches. Please wait a few minutes.",
  },
  directions: "Directions",
  turnByTurn: "Turn by turn",
  from: "From",
  to: "To",
  fromHint: "Start: a place, an address",
  toHint: "Destination: a place, an address",
  swap: "Swap start and destination",
  stop: "Stop",
  stopHint: "Stop: a place, an address",
  addStop: "Add stop",
  removeStop: "Remove this stop",
  useAsStop: "Add as stop",
  along: "Along the route",
  alongCats: { fuel: "Fuel", charging: "Charging", rest: "Rest areas", food: "Food", cafe: "Cafés", groceries: "Groceries",
    toilets: "Toilets", water: "Drinking water", hotel: "Hotels", bike: "Bike shops", pharmacy: "Pharmacies" },
  alongSearching: "Searching along the route…",
  alongNone: "Nothing of this kind near the route.",
  alongAfter: (km) => `after ${km}`,
  alongOff: (m) => `${m} off the route`,
  alongMore: (n, all) => `${num(n)} of ${num(all)} shown, spread over the route.`,
  myPlace: "Your location",
  mapPoint: "Point on the map",
  tapStart: "Tap the map to set the start, or use your location.",
  tapEnd: "Type the destination, or tap it on the map.",
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
  flatMap: "Flat map",
  tiltedMap: "Tilted map",
  arrival: "Arrival",
  left: "left",
  rerouting: "New route…",
  rerouted: "Route recalculated.",
  gpsLost: "GPS signal lost.",
  arrived: "You have arrived.",
  stopReached: "You have reached the stop.",
  noGPS: "Navigation needs your location. Allow it in the settings of the browser, then try again.",
  waitGPS: "Waiting for your location…",
  then: "Then",
  exit: "Exit",
  speedLimit: (v) => `Speed limit ${v}`,
  speedNow: (v) => `Speed ${v} km/h`,
  traffic: "Traffic",
  trafficNote: "Closures, queues and roadworks in Switzerland",
  trafficKinds: { closure: "Closed", jam: "Queue", works: "Roadworks", accident: "Accident", obstruction: "Obstruction", weather: "Weather", event: "Event", slow: "Slow traffic" },
  atNight: "at night",
  byDay: "by day",
  until: (d) => `until ${d}`,
  trafficCredit: "Traffic: Data source: Federal Roads Office (FEDRO) Traffic Data Platform (TDP).",
  delay: (m) => `+${m} min traffic`,
  transit: "Transit",
  transitNote: "Metro, light rail and tram lines in their colours",
  bubbles: [["restaurants", "Restaurants"], ["hotels", "Hotels"], ["transit", "Transit"], ["ATMs", "ATMs"],
    ["cafés", "Cafés", 1], ["supermarkets", "Supermarkets", 1], ["pharmacies", "Pharmacies", 1], ["parking", "Parking", 1], ["petrol stations", "Fuel", 1]],
  transitIn: (place) => `Public transport in ${place}`,
  transitNone: "No metro, light rail or tram lines here. Zoom in closer, or try another city.",
  transitKinds: { subway: "Metro", light_rail: "Light rail", tram: "Tram", monorail: "Monorail" },
  heights: "Height profile",
  up: (m) => `${num(m)} m up`,
  down: (m) => `${num(m)} m down`,
  climbLeft: (m) => `${num(m)} m still to climb`,
  withHours: (n, label, all) => `${num(n)} ${label}, of ${num(all)} with opening hours`,
  wheelchair: "Wheelchair",
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
  buildings3D: "3D buildings",
  buildings3DNote: "Buildings with their height",
  shading: "Relief shading",
  shadingNote: "Light and shadow of the mountains",
  styles: [["auto", "Day and night"], ["light", "Light"], ["dark", "Dark"], ["outdoor", "Outdoor"], ["relief", "Relief"], ["driving", "Driving"]],
  credit: "Map data © OpenStreetMap contributors (ODbL). Pictures from Wikimedia Commons. Chains from the name-suggestion-index.",
  voiceCredit: "Voices by Piper; the French voice from the SIWIS database (CC BY 4.0).",
  language: "Language",
  languageNote: "Page, names on the map, directions and voice",
  automatic: "Automatic",
  poweredBy: "Powered by",
  clear: "Clear",
  search: "Search",
  sheet: "Show or hide the list",
};
// T: the texts in the language of the page; setLang fills the same object again
const T = {};
const loadTexts = () => {
  const tr = texts(num)[UI] || {};
  for (const k of Object.keys(T)) delete T[k];
  Object.assign(T, EN, tr, { errors: { ...EN.errors, ...(tr.errors || {}) } });
};
loadTexts();

// ------------------------------------------------------------------ helpers
const el = (tag, cls, attrs = {}, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (text != null) e.textContent = text;
  return e;
};
// the text of an element, changed only when it differs (each change makes the browser lay out again)
const setText = (e, text) => { if (e.textContent !== text) e.textContent = text; };
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
  share: "M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4",
  star: "M12 3.2l2.6 5.5 6 .8-4.4 4.1 1.1 5.9L12 16.6l-5.3 2.9 1.1-5.9-4.4-4.1 6-.8z",
  recent: "M12 21a9 9 0 1 0-9-9M3 12V7M3 12h5M12 7v5l3 2",
  map: "M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2zM9 4v14M15 6v14",
  wheel: "M12 6a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM11 8v6h5l2 5M8 11a5 5 0 1 0 6 7",
  food: "M7 3v8M5 3v5a2 2 0 0 0 4 0V3M7 11v10M17 3c-2 0-3 2-3 5s1 4 3 4v9",
  book: "M5 4h10a4 4 0 0 1 4 4v12H9a4 4 0 0 1-4-4zM5 16a4 4 0 0 1 4-4h10",
  burger: "M4 7h16M4 12h16M4 17h16",
  locate: "M12 19a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM12 2v3M12 19v3M2 12h3M19 12h3",
  refresh: "M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7",
  navigate: "M12 2 20 21l-8-4-8 4z",
  directions: "M12 2.6 21.4 12 12 21.4 2.6 12zM9.3 14.6v-2.4a1.4 1.4 0 0 1 1.4-1.4h4.4M13.3 8.7l1.9 2.1-1.9 2.1",
  sound: "M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12",
  muted: "M4 9v6h4l5 4V5L8 9zM17 9l5 6M22 9l-5 6",
  overview: "M4 6h16M4 12h10M4 18h6",
  car: "M5 17h14v-5l-2-5H7l-2 5zM5 12h14M7.5 17v2M16.5 17v2M8 14.5h.01M16 14.5h.01",
  bike: "M5.5 18a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM18.5 18a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM5.5 14.5 9 8h6l3.5 6.5M9 8l3 6.5 3-6.5M8 5h3",
  walk: "M13 5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM10 21l2-6 3 3v3M12 15l-1-6 4 2 2 3M11 9l-3 2-1 3",
  swap: "M7 4v16M7 4 4 7M7 4l3 3M17 20V4M17 20l-3-3M17 20l3-3",
  plus: "M12 5v14M5 12h14",
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
// The style of the map, 3D terrain, 3D buildings and relief shading, kept in the browser. The
// page is dark with the dark and black styles; the first style follows the theme of the site.
const store = { get: (k) => { try { return localStorage.getItem(k); } catch { return null; } }, set: (k, v) => { try { localStorage.setItem(k, v); } catch {} } };
const look = {
  style: T.styles.some(([k]) => k === store.get("hm-style")) ? store.get("hm-style") : "auto",
  threeD: store.get("hm-3d") === "1", // the terrain and the map tilted: the 2D/3D button on the map
  buildings: (store.get("hm-buildings") ?? store.get("hm-3d")) === "1", // 3D buildings: the menu
  shading: store.get("hm-shading") === "1",
  traffic: store.get("hm-traffic") === "1", // off at first
  transit: store.get("hm-transit") === "1",
};
// the height of the sun in degrees at a place and time (to a few tenths of a degree; NOAA)
// the sun at a time: its declination, and its hour angle (radians) at longitude 0
function sunAt(when) {
  const rad = Math.PI / 180, d = when / 864e5 - 10957.5; // days since 2000-01-01 12:00 UTC
  const g = (357.529 + 0.98560028 * d) * rad, q = 280.459 + 0.98564736 * d;
  const L = (q + 1.915 * Math.sin(g) + 0.02 * Math.sin(2 * g)) * rad, e = (23.439 - 0.00000036 * d) * rad;
  const ra = Math.atan2(Math.cos(e) * Math.sin(L), Math.cos(L)), dec = Math.asin(Math.sin(e) * Math.sin(L));
  const gmst = (18.697374558 + 24.06570982441908 * d) % 24;
  return { dec, ha0: gmst * 15 * rad - ra };
}
function sunHeight(lon, lat, when = new Date()) {
  const rad = Math.PI / 180, { dec, ha0 } = sunAt(when), ha = ha0 + lon * rad;
  return Math.asin(Math.sin(lat * rad) * Math.sin(dec) + Math.cos(lat * rad) * Math.cos(dec) * Math.cos(ha)) / rad;
}
// the night side of the earth for the globe: where the sun is below 0°, -3° … -15° (twilight to night), one
// polygon each, so that their shades add up towards the middle of the night
function nightSide(when = new Date()) {
  const rad = Math.PI / 180, { dec, ha0 } = sunAt(when), darkSouth = dec >= 0, pole = darkSouth ? -85 : 85;
  const clamp = (v) => Math.max(-85, Math.min(85, v));
  const features = [0, -3, -6, -9, -12, -15].map((h) => {
    const c = Math.sin(h * rad), ring = [];
    for (let lon = -180; lon <= 180; lon += 2) {
      // the latitude where the sun is at h: sin(lat) sin(dec) + cos(lat) cos(dec) cos(ha) = sin(h)
      const a = Math.sin(dec), b = Math.cos(dec) * Math.cos(ha0 + lon * rad), r = Math.hypot(a, b), t = Math.atan2(b, a);
      let lat;
      if (c / r >= 1) lat = -pole; // the whole meridian below h
      else if (c / r <= -1) lat = pole; // none of it
      else {
        const s = Math.asin(c / r), norm = (x) => Math.atan2(Math.sin(x), Math.cos(x));
        const x1 = norm(s - t), x2 = norm(Math.PI - s - t);
        lat = (Math.abs(x1) <= Math.PI / 2 ? x1 : x2) / rad;
      }
      ring.push([lon, clamp(lat)]);
    }
    ring.push([180, pole], [-180, pole], ring[0]);
    return { type: "Feature", properties: { h }, geometry: { type: "Polygon", coordinates: [ring] } };
  });
  return { type: "FeatureCollection", features };
}
// the style shown: Automatic is the light style while the sun is up where the map is, the dark one after
let autoDark = false;
// by car, navigation shows the light style by day and the dark one at night and in tunnels (navLook)
let navLook = null;
// Day and night follows the sun with light and dark; Driving has its own night
const followsSun = () => look.style === "auto" || look.style === "driving";
const shownStyle = () => navLook || (look.style === "auto" ? (autoDark ? "dark" : "light")
  : look.style === "driving" && autoDark ? "driving-night" : look.style);
const isNight = (lon, lat) => sunHeight(lon, lat) < -0.833; // the sun below the horizon
// day or night where the visitor is, by their clock: the sun at their location once known, else the
// hours of the device (day 07–19); not where the map is (looking at America from Europe at night stays night)
let meForSun = null;
const nightNow = () => (meForSun ? isNight(meForSun[0], meForSun[1]) : ((h) => h < 7 || h >= 19)(new Date().getHours()));
autoDark = nightNow();
const theme = () => (["dark", "black", "driving-night"].includes(shownStyle()) ? "dark" : "light");
document.documentElement.dataset.theme = theme();
const styleURL = (name = shownStyle()) => {
  const p = new URLSearchParams({ lang: ["de", "fr", "it"].includes(LANG) ? LANG : "en" });
  if (look.shading) p.set("hillshade", "1");
  if (look.threeD) p.set("terrain", "1");
  if (look.buildings && name !== "relief") p.set("buildings", "1");
  return `${root.dataset.tiles}/v1/${root.dataset.key}/maps/styles/${name.replace(/-night$/, "")}.json?${p}`;
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
const burger = () => {
  const b = el("button", "lm-icon-btn lm-burger", { type: "button", title: T.menu, "aria-label": T.menu, "aria-controls": "lm-menu", "aria-expanded": "false" });
  b.innerHTML = svg(ICON.burger, 22);
  return b;
};
const menuBtn = burger();
const sheetBtn = el("button", "lm-sheet-btn", { type: "button", "aria-label": T.sheet });
top.append(menuBtn, logo());

const form = el("form", "lm-form", { role: "search" });
const combo = el("div", "lm-combo");
const input = el("input", "lm-input", { type: "search", role: "combobox", "aria-autocomplete": "list", "aria-expanded": "false",
  "aria-controls": "lm-list", "aria-label": T.placeholder, placeholder: T.placeholder, autocomplete: "off", spellcheck: "false", maxlength: "160", enterkeyhint: "search" });
const clearBtn = el("button", "lm-clear", { type: "button", "aria-label": T.clear });
clearBtn.innerHTML = svg(ICON.close, 16);
clearBtn.hidden = true;
// the hint of the empty box: an example of a search picked at random each time the box is empty
// again (the start, the X, the text deleted), not the one shown before
const hints = (T.hints || []).slice();
const newHint = () => {
  if (!hints.length) return;
  const others = hints.length > 1 ? hints.filter((h) => h !== input.placeholder) : hints;
  input.placeholder = others[Math.floor(Math.random() * others.length)];
};
newHint();
const goBtn = el("button", "lm-go", { type: "submit", "aria-label": T.search });
goBtn.innerHTML = svg(ICON.search, 20);
const list = el("ul", "lm-suggest", { id: "lm-list", role: "listbox" });
list.hidden = true;
combo.append(input, clearBtn, goBtn, list);
// Directions at all times, beside the search: the fields for start and destination
// (with a place selected or found: navigation to it at once, from the location of the visitor)
const dirBtn = el("button", "lm-dir-btn", { type: "button", title: T.directions, "aria-label": T.directions });
dirBtn.innerHTML = '<svg viewBox="0 0 24 24" width="26" height="26" aria-hidden="true"><path d="M12 2.4 21.6 12 12 21.6 2.4 12z" fill="#fff"/>' +
  '<path class="lm-dir-arrow" d="M9.3 14.6v-2.4a1.4 1.4 0 0 1 1.4-1.4h4.4M13.3 8.7l1.9 2.1-1.9 2.1" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';
dirBtn.hidden = !ROUTE;
dirBtn.addEventListener("click", () => {
  const to = placeNow();
  if (to) return directions(to); // the route with Preview and Start, not navigation at once
  if (routeView.hidden) directions(null);
  routeView.querySelector(route.to ? ".lm-end-from input" : ".lm-end-to input")?.focus();
});
// beside the search only while the panel has no Directions of its own (a place found or opened has)
function showDirBtn() {
  dirBtn.hidden = !ROUTE || !detailView.hidden || (!listView.hidden && placeActs.childElementCount > 0);
}
// the place on the screen: the one opened, or the one place a search found
function placeNow() {
  if (!hereView.hidden && state.herePoint) return state.herePoint;
  if (!detailView.hidden && state.selected) {
    const c = center(state.selected);
    if (c) return { ll: c, label: state.selected.properties.name || T.noName };
  }
  if (!listView.hidden && state.placeTarget) return state.placeTarget;
  return null;
}
form.append(combo, dirBtn);

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
  if (v !== routeView && !routeView.hidden && !route.keep) routeDone(); // a point looked at from the route keeps it
  if (v !== hereView) route.keep = false;
  listView.hidden = v !== listView;
  hereView.hidden = v !== hereView;
  detailView.hidden = v !== detailView;
  routeView.hidden = v !== routeView;
  root.classList.toggle("lm-planning", v === routeView); // directions have their own fields: no search bar above them
  showDirBtn();
  body.scrollTop = 0;
  if (v === listView) setTimeout(foldIfEmpty); // after the caller has filled or cleared the list
};
const footText = () => [T.credit, root.dataset.say && T.voiceCredit, root.dataset.traffic && T.trafficCredit].filter(Boolean).join(" ");
const foot = el("div", "lm-foot", {}, footText());
side.append(sheetBtn, top, form, status, body, foot);

// ------------------------------------------------------------------ the map
// a link with a place on the map (#map=) opens there; otherwise, and when the place is just the
// last view of this browser (a reload, a return), the map starts where the visitor is
const lastView = (() => { try { return localStorage.getItem("hm-last-view"); } catch { return null; } })();
const linkedView = /(^|&)map=/.test(location.hash.slice(1)) && location.hash !== lastView;
// the tilt of the map in navigation: 50 degrees shows the way ahead; more draws many far tiles
const NAV_PITCH = 50;
const map = new maplibregl.Map({
  container: mapEl,
  style: styleURL(),
  center: [8.54, 47.37],
  zoom: 13,
  hash: "map",
  maxPitch: 85,
  attributionControl: { compact: true },
  // at most two pixels of the screen to one of the map: three or more draw a phone screen with more than twice the work
  pixelRatio: Math.min(devicePixelRatio, 2),
});
window.hmMap = map; // for tests/browser.py: the sources and layers of the map
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
// 2D or 3D, the button names the other: on the map the terrain and the tilt; in navigation the map that
// follows, flat or tilted (hm-nav-2d)
const dimBtn = el("button", "lm-dim", { type: "button" });
function showDim() {
  const flat = nav.on ? nav.flat : !look.threeD;
  dimBtn.textContent = flat ? "3D" : "2D";
  dimBtn.title = dimBtn.ariaLabel = flat ? T.tiltedMap : T.flatMap;
}
dimBtn.addEventListener("click", () => {
  if (!nav.on) return setThreeD(!look.threeD);
  nav.flat = !nav.flat;
  store.set("hm-nav-2d", nav.flat ? "1" : "0");
  showDim();
  if (nav.follow) map.easeTo({ pitch: nav.flat ? 0 : NAV_PITCH, duration: 500 });
});
mapEl.append(areaBtn, compassBtn, dimBtn, locBtn);
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
const sources = { night: nightSide(), transit: empty, stars: empty, me: empty, results: empty, lines: empty, selected: empty, hover: empty, pin: empty, area: empty, route: empty, traffic: empty, routetraffic: empty, along: empty };
const ACCENT = "#e8590c"; // the warm complement of the hippo blue (--warm)
const STOP_COLOUR = "#e8590c"; // stops on a route, and the places along it
// POIs earlier: each POI of the tiles has a min_zoom (restaurants and cafés 16, bus stops 18), and the
// tiles of zoom 14 leave the small ones out. Between zoom 14 and 15 the POIs come from a tile of zoom 14
// put together here from the POIs of its four tiles of zoom 15 (hmpoi://); from 14 on they show two
// zooms before their min_zoom, so a village seen whole shows its restaurants. Symbols that would
// overlap still give way.
let poiTiles = null;
const pbf = {
  read(b, f) { // the fields of a message: [field, wire type, value or bytes, start, end]
    const out = [];
    let i = 0;
    const vint = () => { let v = 0, m = 1, c; do { c = b[i++]; v += (c & 127) * m; m *= 128; } while (c & 128); return v; };
    while (i < b.length) {
      const s = i, k = vint(), w = k & 7;
      let v;
      if (w === 0) v = vint();
      else if (w === 2) { const n = vint(); v = b.subarray(i, i + n); i += n; }
      else if (w === 5) { v = b.subarray(i, i + 4); i += 4; }
      else if (w === 1) { v = b.subarray(i, i + 8); i += 8; }
      else break;
      if (!f || f(k >> 3)) out.push([k >> 3, w, v, s, i]);
    }
    return out;
  },
  packed(b) { const v = []; let i = 0; while (i < b.length) { let x = 0, m = 1, c; do { c = b[i++]; x += (c & 127) * m; m *= 128; } while (c & 128); v.push(x); } return v; },
  vint(out, v) { while (v > 127) { out.push((v % 128) | 128); v = Math.floor(v / 128); } out.push(v); },
  field(out, f, bytes) { pbf.vint(out, (f << 3) | 2); pbf.vint(out, bytes.length); for (const x of bytes) out.push(x); },
};
const zz = (n) => (n % 2 ? -(n + 1) / 2 : n / 2), unzz = (n) => (n < 0 ? -2 * n - 1 : 2 * n);
async function poiTile(z, x, y, signal) {
  const keys = [], keyAt = new Map(), vals = [], valAt = new Map(), feats = [];
  const idx = (list, at, k, raw) => { if (!at.has(k)) { at.set(k, list.length); list.push(raw); } return at.get(k); };
  await Promise.all([0, 1, 2, 3].map(async (c) => {
    const dx = c & 1, dy = c >> 1;
    const url = poiTiles[0].replace("{z}", z + 1).replace("{x}", 2 * x + dx).replace("{y}", 2 * y + dy);
    const r = await fetch(url, { signal });
    if (!r.ok) return;
    let b = new Uint8Array(await r.arrayBuffer());
    if (b[0] === 0x1f && b[1] === 0x8b) b = new Uint8Array(await new Response(new Blob([b]).stream().pipeThrough(new DecompressionStream("gzip"))).arrayBuffer());
    for (const [, , lb] of pbf.read(b, (f) => f === 3)) {
      const fs = pbf.read(lb);
      if (new TextDecoder().decode(fs.find((f) => f[0] === 1)?.[2] || new Uint8Array()) !== "pois") continue;
      const ext = fs.find((f) => f[0] === 5)?.[2] || 4096;
      const ks = fs.filter((f) => f[0] === 3).map((f) => f[2]), vs = fs.filter((f) => f[0] === 4).map((f) => f[2]);
      for (const [, , fb] of fs.filter((f) => f[0] === 2)) {
        const ff = pbf.read(fb);
        if ((ff.find((f) => f[0] === 3)?.[2] || 0) !== 1) continue; // points only
        const g = pbf.packed(ff.find((f) => f[0] === 4)?.[2] || new Uint8Array());
        const pts = [];
        for (let i = 0, px = 0, py = 0; i < g.length;) {
          const cmd = g[i] & 7, n = g[i] >> 3;
          i++;
          for (let k = 0; k < n && cmd === 1; k++) { px += zz(g[i++]); py += zz(g[i++]); pts.push([px, py]); }
          if (cmd !== 1) break;
        }
        // in this child only (the buffer of the tiles would draw a POI twice), then into the parent
        const keep = pts.filter(([a, b2]) => a >= 0 && a < ext && b2 >= 0 && b2 < ext)
          .map(([a, b2]) => [Math.round((a + dx * ext) * 2048 / ext), Math.round((b2 + dy * ext) * 2048 / ext)]);
        if (!keep.length) continue;
        const tags = pbf.packed(ff.find((f) => f[0] === 2)?.[2] || new Uint8Array());
        const nt = [];
        for (let t = 0; t + 1 < tags.length; t += 2) {
          const kb = ks[tags[t]], vb = vs[tags[t + 1]];
          if (!kb || !vb) continue;
          nt.push(idx(keys, keyAt, new TextDecoder().decode(kb), kb), idx(vals, valAt, Array.from(vb).join(","), vb));
        }
        feats.push({ id: ff.find((f) => f[0] === 1), tags: nt, pts: keep });
      }
    }
  }));
  const layer = [];
  pbf.vint(layer, (15 << 3) | 0); pbf.vint(layer, 2);
  pbf.field(layer, 1, new TextEncoder().encode("pois"));
  for (const f of feats) {
    const fb = [];
    if (f.id) { pbf.vint(fb, (1 << 3) | 0); pbf.vint(fb, f.id[2]); }
    const tb = []; for (const t of f.tags) pbf.vint(tb, t);
    pbf.field(fb, 2, tb);
    pbf.vint(fb, (3 << 3) | 0); pbf.vint(fb, 1);
    const gb = [];
    pbf.vint(gb, (f.pts.length << 3) | 1);
    let px = 0, py = 0;
    for (const [a, b2] of f.pts) { pbf.vint(gb, unzz(a - px)); pbf.vint(gb, unzz(b2 - py)); px = a; py = b2; }
    pbf.field(fb, 4, gb);
    pbf.field(layer, 2, fb);
  }
  for (const k of keys) pbf.field(layer, 3, k);
  for (const v of vals) pbf.field(layer, 4, v);
  pbf.vint(layer, (5 << 3) | 0); pbf.vint(layer, 4096);
  const tile = [];
  pbf.field(tile, 3, layer);
  return new Uint8Array(tile).buffer;
}
maplibregl.addProtocol("hmpoi", async (params, ctl) => {
  const [z, x, y] = params.url.replace("hmpoi://", "").split("/").map(Number);
  return { data: await poiTile(z, x, y, ctl.signal) };
});
async function poisEarlier() {
  const st = map.getStyle(), pois = st?.layers.find((l) => l.id === "pois");
  const src = pois && st.sources[pois.source];
  if (!src || map.getLayer("lm-pois-near")) return;
  try {
    poiTiles ||= src.tiles || (await (await fetch(src.url)).json()).tiles;
  } catch { return; }
  if (!poiTiles?.length || !map.getLayer("pois") || map.getLayer("lm-pois-near")) return;
  const near = nearPois(pois);
  if (!near) return;
  if (!map.getSource("lm-pois-near")) map.addSource("lm-pois-near", NEAR_SOURCE);
  map.addLayer(near.layer, "pois");
  map.setFilter("pois", near.filter);
}
const NEAR_SOURCE = { type: "vector", tiles: ["hmpoi://{z}/{x}/{y}"], minzoom: 14, maxzoom: 14, attribution: "" };
// the layer of the POIs between zoom 14 and 15 from the layer pois of a style, and the filter of pois without them;
// null for a style without min_zoom (as it is)
function nearPois(pois) {
  const earlier = (f) => Array.isArray(f) && f[0] === ">=" && JSON.stringify(f[1]) === '["zoom"]' && JSON.stringify(f[2]?.[1]) === '["get","min_zoom"]'
    ? [">=", ["zoom"], ["-", ["get", "min_zoom"], 2]]
    : Array.isArray(f) ? f.map(earlier) : f;
  const filter = earlier(pois.filter);
  if (JSON.stringify(filter) === JSON.stringify(pois.filter)) return null;
  const layer = { ...pois, id: "lm-pois-near", source: "lm-pois-near", minzoom: 14, maxzoom: 15, filter };
  delete layer.metadata;
  return { layer, filter: ["all", ["any", ["<", ["zoom"], 14], [">=", ["zoom"], 15]], filter] };
}
// the day map: the light style of overpass.ch, recoloured here in the manner of Swiss topographic maps. DAY names
// the colours (steps in OKLCH: land almost white, the greens one hue family from grass through parks to forest so that
// denser vegetation reads darker, water a clear light blue, motorways orange with a darker casing of the same hue,
// boundaries a muted magenta, labels a dark cool grey). PASTEL maps them per layer (first match) from the colours of
// the style.
const DAY = {
  land: "#fbfaf7", building: "#d9d3ce", grass: "#ddefcc", park: "#cde8c0", forest: "#b1d7a9", urbanGreen: "#d5ebc6",
  water: "#9ed3f2", waterLabel: "#006c99", sand: "#fcf3da", glacier: "#f0f8fc", hospital: "#ffebeb",
  school: "#fff1e0", industrial: "#edebf5", aerodrome: "#edeef5", zoo: "#cce5db", military: "#e0ecef",
  pedestrian: "#f6f5f2", runway: "#f4f5f9", highway: "#ffcc8e", highwayCase: "#e4a067", casing: "#dfdcd4",
  tunnel: "#fafaf9", tunnelCase: "#ededea", rail: "#b2afbd", boundary: "#b278ac", label: "#282e38",
};
const PASTEL = [
  [/^(background|earth)$/, { "#cccccc": DAY.land, "#e2dfda": DAY.land }],
  [/^landuse_(park|forest)$/, { "#9cd3b4": DAY.park, "#a0d9a0": DAY.forest, "#99d2bb": DAY.grass, "#e7e7e7": DAY.glacier,
    "#e2e0d7": DAY.sand, "#c6dcdc": DAY.military, "#e2dfda": DAY.land }],
  [/^landuse_urban_green$/, { "#9cd3b4": DAY.urbanGreen }],
  [/^landuse_hospital$/, { "#e4dad9": DAY.hospital }],
  [/^landuse_industrial$/, { "#d1dde1": DAY.industrial }],
  [/^landuse_school$/, { "#e4ded7": DAY.school }],
  [/^landuse_beach$/, { "#e8e4d0": DAY.sand }],
  [/^landuse_zoo$/, { "#c6dcdc": DAY.zoo }],
  [/^landuse_aerodrome$/, { "#dadbdf": DAY.aerodrome }],
  [/^(landuse_runway|roads_runway|roads_taxiway)$/, { "#e9e9ed": DAY.runway }],
  [/^landuse_(pedestrian|pier)$/, { "#e3e0d4": DAY.pedestrian, "#e0e0e0": DAY.tunnelCase }],
  [/^water/, { "#80deea": DAY.water, "#728dd4": DAY.waterLabel }],
  [/^buildings$/, { "#cccccc": DAY.building }],
  [/^roads_tunnels/, { "#e0e0e0": DAY.tunnelCase, "#d5d5d5": DAY.tunnel }],
  [/^roads_.*highway.*casing/, { "#e0e0e0": DAY.highwayCase }],
  [/^roads_.*highway/, { "#ffffff": DAY.highway }],
  [/casing/, { "#e0e0e0": DAY.casing }],
  [/^roads_rail$/, { "#a7b1b3": DAY.rail }],
  [/^boundaries/, { "#adadad": DAY.boundary }],
  [/^(places_|earth_label)/, { "#e0e0e0": DAY.land, "#e2dfda": DAY.land, "#5c5c5c": DAY.label }],
  [/^pois$/, { "#e2dfda": DAY.land }],
];
const repaint = (v, to) => (typeof v === "string" ? to[v.toLowerCase()] || v : Array.isArray(v) ? v.map((x) => repaint(x, to)) : v);
// pastel recolours the light style on the map; pastelStyle the same in a style JSON
const pastelPaint = (l, set) => {
  const to = PASTEL.find(([re]) => re.test(l.id))?.[1];
  if (!to) return;
  for (const [k, v] of Object.entries(l.paint || {})) {
    if (!k.endsWith("color")) continue;
    const w = repaint(v, to);
    if (JSON.stringify(w) !== JSON.stringify(v)) set(k, w);
  }
};
const pastel = () => { for (const l of map.getStyle().layers) pastelPaint(l, (k, w) => map.setPaintProperty(l.id, k, w)); };
const pastelStyle = (st) => { for (const l of st.layers) pastelPaint(l, (k, w) => { l.paint[k] = w; }); };

// icons drawn here (a new style drops them): a star for the starred places, a pin for the place shown
const drawIcon = (name, w, h, draw) => {
  if (map.hasImage(name)) return;
  const k = 2, c = document.createElement("canvas");
  c.width = w * k; c.height = h * k;
  const g = c.getContext("2d");
  g.scale(k, k);
  draw(g);
  map.addImage(name, g.getImageData(0, 0, c.width, c.height), { pixelRatio: k });
};
const addIcons = () => {
  drawIcon("lm-star-icon", 26, 26, (g) => {
    g.beginPath();
    for (let i = 0; i < 10; i++) {
      const r = i % 2 ? 4.8 : 11, a = -Math.PI / 2 + (i * Math.PI) / 5;
      g.lineTo(13 + r * Math.cos(a), 13.5 + r * Math.sin(a));
    }
    g.closePath();
    g.shadowColor = "rgba(0,0,0,.3)"; g.shadowBlur = 2; g.shadowOffsetY = 1;
    g.fillStyle = "#f5b301"; g.fill();
    g.shadowColor = "transparent";
    g.lineJoin = "round"; g.lineWidth = 1.8; g.strokeStyle = "#fff"; g.stroke();
  });
  drawIcon("lm-pin-icon", 32, 42, (g) => {
    g.beginPath();
    g.moveTo(16, 40);
    g.bezierCurveTo(13, 33, 4, 26, 4, 16);
    g.arc(16, 16, 12, Math.PI, 0);
    g.bezierCurveTo(28, 26, 19, 33, 16, 40);
    g.closePath();
    g.shadowColor = "rgba(0,0,0,.35)"; g.shadowBlur = 3; g.shadowOffsetY = 1.5;
    g.fillStyle = "#0369a1"; g.fill();
    g.shadowColor = "transparent";
    g.lineWidth = 2; g.strokeStyle = "#fff"; g.stroke();
    g.beginPath(); g.arc(16, 16, 4.5, 0, 2 * Math.PI); g.fillStyle = "#fff"; g.fill();
  });
};
const pinLayer = (id, source, filter) => ({ id, type: "symbol", source, ...(filter ? { filter } : {}),
  layout: { "icon-image": "lm-pin-icon", "icon-anchor": "bottom", "icon-allow-overlap": true, "icon-ignore-placement": true } });

// the colours of our names, by theme: places along the route, and the results
const TEXT_COLOUR = { "lm-along-name": ["#ffd8a8", "#7c2d12"], "lm-label": ["#ffb4a9", "#7f1d1d"] };
const textPaint = (id) => ({ "text-color": TEXT_COLOUR[id][theme() === "dark" ? 0 : 1], "text-halo-color": theme() === "dark" ? "#111" : "#fff" });
map.on("style.load", () => {
  if (shownStyle() === "light") pastel();
  addIcons();
  // a globe when zoomed out; MapLibre turns it into the flat map from about zoom 12 (a new style resets it)
  map.setProjection({ type: "globe" });
  // 3D buildings see-through, so the map and the route behind them stay in view
  for (const l of map.getStyle().layers) if (l.type === "fill-extrusion") map.setPaintProperty(l.id, "fill-extrusion-opacity", 0.6);
  poisEarlier();
  const add = (l, before) => { if (!map.getLayer(l.id)) map.addLayer(l, before); };
  // tiles of our layers up to zoom 14 only, drawn larger beyond (0.3 m steps there, a pixel or two at zoom 19): tiles
  // to zoom 18 cost the phone much work while the camera moves, and empty sources too
  for (const [name, d] of Object.entries(sources)) if (!map.getSource(`lm-${name}`)) map.addSource(`lm-${name}`, { type: "geojson", data: d, maxzoom: 14, tolerance: 0.1 });
  const below = map.getStyle().layers.find((l) => l.type === "symbol")?.id;
  const dashed = ["in", ["get", "kind"], ["literal", ["border", "circle"]]];
  const pts = ["in", ["geometry-type"], ["literal", ["Point", "MultiPoint"]]];
  const polys = ["in", ["geometry-type"], ["literal", ["Polygon", "MultiPolygon"]]];
  add({ id: "lm-night", type: "fill", source: "lm-night", maxzoom: 7,
    paint: { "fill-color": "#0b1626", "fill-antialias": false, "fill-opacity": ["interpolate", ["linear"], ["zoom"], 3, 0.11, 6.5, 0] } }, below);
  add({ id: "lm-drawn-dashed", type: "line", source: "lm-lines", filter: dashed,
    paint: { "line-color": "#1f5fbf", "line-width": ["match", ["get", "kind"], "circle", 1.2, 2], "line-dasharray": [3, 2], "line-opacity": 0.8 } }, below);
  add({ id: "lm-drawn", type: "line", source: "lm-lines", filter: ["!", dashed], layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ["match", ["get", "kind"], "road", "#c2410c", "river", "#0284c7", "#1f5fbf"], "line-width": ["match", ["get", "kind"], "shape", 1.5, 3], "line-opacity": 0.85 } }, below);
  add({ id: "lm-fill", type: "fill", source: "lm-results", filter: polys, paint: { "fill-color": ACCENT, "fill-opacity": 0.15 } }, below);
  add({ id: "lm-line", type: "line", source: "lm-results", filter: ["!", pts], layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ACCENT, "line-width": ["interpolate", ["linear"], ["zoom"], 8, 1.5, 16, 4] } }, below);
  add({ id: "lm-point", type: "circle", source: "lm-results", filter: pts, paint: {
    "circle-radius": ["interpolate", ["linear"], ["zoom"], 6, ["case", ["boolean", ["get", "place"], false], 8, 3], 15, ["case", ["boolean", ["get", "place"], false], 10, 7]],
    "circle-color": ["case", ["boolean", ["get", "place"], false], "#1f5fbf", ACCENT],
    "circle-stroke-color": "#fff", "circle-stroke-width": 1.5 } });
  add({ id: "lm-hover", type: "circle", source: "lm-hover", filter: pts,
    paint: { "circle-radius": 11, "circle-color": ACCENT, "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 } });
  add({ id: "lm-hover-line", type: "line", source: "lm-hover", filter: ["!", pts], paint: { "line-color": "#111827", "line-width": 4, "line-opacity": 0.5 } });
  add({ id: "lm-sel-line", type: "line", source: "lm-selected", filter: ["!", pts], paint: { "line-color": "#111827", "line-width": 5, "line-opacity": 0.65 } });
  add(pinLayer("lm-sel", "lm-selected", pts));
  add({ id: "lm-me-area", type: "fill", source: "lm-me", filter: polys, paint: { "fill-color": "#1a73e8", "fill-opacity": 0.12 } });
  add({ id: "lm-me", type: "circle", source: "lm-me", filter: pts,
    paint: { "circle-radius": 7, "circle-color": "#1a73e8", "circle-stroke-color": "#fff", "circle-stroke-width": 2.5 } });
  // the area of a tap: its outline dashed in the blue of overpass.ch
  add({ id: "lm-area-line", type: "line", source: "lm-area", layout: { "line-join": "round", "line-cap": "butt" },
    paint: { "line-color": "#0284c7", "line-width": 2.5, "line-opacity": 0.95, "line-dasharray": [3, 2] } }, below);
  // directions: the line with a casing, the start (white) and the destination (red)
  const line = ["==", ["geometry-type"], "LineString"];
  // traffic under the route: closures dark red, queues red, slow traffic and roadworks orange
  const tline = ["==", ["geometry-type"], "LineString"];
  // transit: each line in its colour, metro wider than tram; its number along it when near
  add({ id: "lm-transit-line", type: "line", source: "lm-transit", layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ["to-color", ["get", "colour"], "#7a7f87"], "line-opacity": 0.9,
      "line-width": ["interpolate", ["linear"], ["zoom"], 10, ["match", ["get", "kind"], "subway", 2.5, 1.5], 15, ["match", ["get", "kind"], "subway", 6, 3.5]] } }, below);
  const font = map.getStyle().layers.find((l) => l.layout?.["text-font"])?.layout["text-font"];
  if (font) add({ id: "lm-transit-ref", type: "symbol", source: "lm-transit", minzoom: 13, filter: ["has", "ref"],
    layout: { "symbol-placement": "line", "text-field": ["get", "ref"], "text-font": font, "text-size": 12, "symbol-spacing": 300 },
    paint: { "text-color": "#fff", "text-halo-color": ["to-color", ["get", "colour"], "#7a7f87"], "text-halo-width": 2.5 } });
  add({ id: "lm-traffic-line", type: "line", source: "lm-traffic", filter: ["all", tline, ["!=", ["get", "t"], "works"]],
    layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ["match", ["get", "t"], "closed", "#7f1d1d", "closure", "#9ca3af", "jam", "#dc2626", "#f08c00"],
      "line-width": ["interpolate", ["linear"], ["zoom"], 7, 2, 14, 6], "line-opacity": 0.85 } }, below);
  add({ id: "lm-traffic-works", type: "line", source: "lm-traffic", minzoom: 11, filter: ["all", tline, ["==", ["get", "t"], "works"]],
    paint: { "line-color": "#f08c00", "line-width": 3, "line-dasharray": [2, 1.5], "line-opacity": 0.8 } }, below);
  const tpt = ["==", ["geometry-type"], "Point"];
  add({ id: "lm-traffic-pt", type: "circle", source: "lm-traffic", minzoom: 7, filter: ["all", tpt, ["!=", ["get", "t"], "works"]], paint: {
    "circle-radius": ["interpolate", ["linear"], ["zoom"], 7, 4, 14, 8], "circle-stroke-width": 2, "circle-stroke-color": "#fff",
    "circle-color": ["match", ["get", "t"], "closed", "#7f1d1d", "closure", "#6b7280", "jam", "#dc2626", "accident", "#dc2626", "#f08c00"] } });
  add({ id: "lm-traffic-works-pt", type: "circle", source: "lm-traffic", minzoom: 11, filter: ["all", tpt, ["==", ["get", "t"], "works"]], paint: {
    "circle-radius": 6, "circle-stroke-width": 2, "circle-stroke-color": "#fff", "circle-color": "#f08c00" } });
  add({ id: "lm-stars", type: "symbol", source: "lm-stars", layout: { "icon-image": "lm-star-icon",
    "icon-size": ["interpolate", ["linear"], ["zoom"], 5, 0.6, 15, 1], "icon-allow-overlap": true, "icon-ignore-placement": true } });
  add({ id: "lm-route-case", type: "line", source: "lm-route", filter: line, layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": "#0b3d91", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 5, 16, 11], "line-opacity": 0.9 } }, below);
  add({ id: "lm-route-line", type: "line", source: "lm-route", filter: line, layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": "#1a73e8", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 3, 16, 8] } }, below);
  add({ id: "lm-route-traffic", type: "line", source: "lm-routetraffic", layout: { "line-cap": "round", "line-join": "round" },
    paint: { "line-color": ["match", ["get", "level"], 2, "#d93025", "#f29900"], "line-width": ["interpolate", ["linear"], ["zoom"], 6, 3, 16, 8] } }, below);
  // places along the route (search along the route): orange, their name when near
  add({ id: "lm-along", type: "circle", source: "lm-along", paint: {
    "circle-radius": ["interpolate", ["linear"], ["zoom"], 6, 4, 15, 8], "circle-color": STOP_COLOUR, "circle-stroke-color": "#fff", "circle-stroke-width": 2 } });
  if (font) add({ id: "lm-along-name", type: "symbol", source: "lm-along", minzoom: 13, filter: ["has", "name"],
    layout: { "text-field": ["get", "name"], "text-font": font, "text-size": 12, "text-offset": [0, 1], "text-anchor": "top", "text-max-width": 10, "text-optional": true },
    paint: { ...textPaint("lm-along-name"), "text-halo-width": 1.5 } });
  // the start (white), the stops (orange, numbered) and the destination (red)
  add({ id: "lm-route-ends", type: "circle", source: "lm-route", filter: ["!", line], paint: {
    "circle-radius": ["match", ["get", "end"], "stop", 9, 7], "circle-color": ["match", ["get", "end"], "from", "#fff", "stop", STOP_COLOUR, "#d0312d"],
    "circle-stroke-color": ["match", ["get", "end"], "from", "#111827", "#fff"], "circle-stroke-width": ["match", ["get", "end"], "stop", 2, 3] } });
  if (font) add({ id: "lm-route-stop-n", type: "symbol", source: "lm-route", filter: ["==", ["get", "end"], "stop"],
    layout: { "text-field": ["get", "n"], "text-font": font, "text-size": 11, "text-allow-overlap": true, "text-ignore-placement": true },
    paint: { "text-color": "#fff" } });
  add(pinLayer("lm-pin", "lm-pin"));
  add({ id: "lm-label", type: "symbol", source: "lm-results", minzoom: 14, filter: ["all", ["has", "name"], pts],
    layout: { "text-field": ["get", "name"], "text-font": ["Noto Sans Medium"], "text-size": 12, "text-offset": [0, 1], "text-anchor": "top",
      "text-max-width": 10, "text-optional": true },
    paint: { ...textPaint("lm-label"), "text-halo-width": 1.5 } });
});
const setSource = (name, d) => {
  sources[name] = d || empty;
  map.getSource(`lm-${name}`)?.setData(sources[name]);
};
setInterval(() => setSource("night", nightSide()), 5 * 60e3); // the night moves 1.25° in five minutes
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

// ------------------------------------------------------------------ traffic
// Closures, queues, roadworks and slow traffic in Switzerland (data-traffic: the Traffic Data
// Platform of FEDRO, through Valhalla's traffic service), refreshed every two minutes while the
// switch of the menu is on (hm-traffic). Routes take them into account in any case.
const TRAFFIC = root.dataset.traffic || "";
let trafficTimer = 0, trafficById = new Map();
async function loadTraffic() {
  clearTimeout(trafficTimer);
  if (!TRAFFIC || !look.traffic) return setSource("traffic", empty);
  try {
    const d = await (await fetch(TRAFFIC)).json();
    const fs = [];
    trafficById = new Map();
    for (const it of d.items || []) {
      const t = it.kind === "closure" ? (it.closed ? "closed" : "closure") : it.kind;
      trafficById.set(it.id, it);
      for (const ln of it.lines || []) fs.push({ type: "Feature", geometry: { type: "LineString", coordinates: ln }, properties: { t, id: it.id } });
      if (it.at) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: it.at }, properties: { t, id: it.id } });
    }
    for (const f of d.flow || []) {
      fs.push({ type: "Feature", geometry: { type: "LineString", coordinates: f.line }, properties: { t: f.level === "jam" ? "jam" : "slow", kph: f.kph } });
    }
    if (look.traffic) setSource("traffic", { type: "FeatureCollection", features: fs });
  } catch {}
  trafficTimer = setTimeout(loadTraffic, 120e3);
}
// on and off: the button on the map and the switch of the menu
function setTraffic(on) {
  look.traffic = on;
  store.set("hm-traffic", on ? "1" : "0");
  trafficBtn.classList.toggle("lm-on", on);
  trafficBtn.setAttribute("aria-pressed", String(on));
  if (trafficBox) trafficBox.checked = on;
  loadTraffic();
}
const trafficBtn = el("button", "lm-traffic-btn", { type: "button", title: T.traffic, "aria-label": T.traffic, "aria-pressed": String(look.traffic) });
trafficBtn.innerHTML = '<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><rect x="7.5" y="2.5" width="9" height="19" rx="3" fill="none" stroke="currentColor" stroke-width="1.8"/>' +
  '<circle cx="12" cy="7" r="1.8" class="lm-tl-r"/><circle cx="12" cy="12" r="1.8" class="lm-tl-y"/><circle cx="12" cy="17" r="1.8" class="lm-tl-g"/></svg>';
trafficBtn.classList.toggle("lm-on", look.traffic);
trafficBtn.hidden = !TRAFFIC;
trafficBtn.addEventListener("click", () => setTraffic(!look.traffic));
mapEl.append(trafficBtn);
let trafficBox = null;
// a tap on traffic shows its message; true when it did
function trafficAt(e) {
  if (!TRAFFIC || !look.traffic) return false;
  const layers = ["lm-traffic-pt", "lm-traffic-works-pt", "lm-traffic-line", "lm-traffic-works"].filter((l) => map.getLayer(l));
  const box = [[e.point.x - 6, e.point.y - 6], [e.point.x + 6, e.point.y + 6]];
  const f = map.queryRenderedFeatures(box, { layers })[0];
  if (!f) return false;
  const it = trafficById.get(f.properties.id);
  const box2 = el("div", "lm-traffic-pop");
  const kind = it ? (it.kind === "closure" && !it.closed ? "closure" : it.kind) : f.properties.t;
  const head = el("strong", "", {}, T.trafficKinds[kind] || T.trafficKinds[f.properties.t] || T.traffic);
  box2.append(head);
  if (it?.when) head.append(` · ${it.when === "duringTheNight" ? T.atNight : T.byDay}`);
  if (it) {
    const tx = it.text?.[UI] || it.text?.de || it.text?.fr || it.text?.it || "";
    box2.append(el("p", "", {}, tx.replace(/^(Freigegeben|Libéré|Approvato|Released):\s*/, "")));
  } else if (f.properties.kph != null) {
    box2.append(el("p", "", {}, `${f.properties.kph} km/h`));
  }
  new maplibregl.Popup({ maxWidth: "320px" }).setLngLat(e.lngLat).setDOMContent(box2).addTo(map);
  return true;
}
for (const l of ["lm-traffic-pt", "lm-traffic-works-pt", "lm-traffic-line", "lm-traffic-works"]) {
  map.on("mouseenter", l, () => { map.getCanvas().style.cursor = "pointer"; });
  map.on("mouseleave", l, () => { map.getCanvas().style.cursor = ""; });
}

// ------------------------------------------------------------------ transit
// The lines of metro, light rail, monorail and tram in their colours (data-transit: from Overpass,
// through this server, by tiles of zoom 10), from zoom 10 on, while the switch is on (hm-transit).
const TRANSIT = root.dataset.transit || "";
const transitTiles = new Map(); // "x/y" -> a promise of the features of the tile
let transitTimer = 0;
const tileXY = (lon, lat, z) => {
  const n = 2 ** z, r = (lat * Math.PI) / 180;
  return [Math.floor(((lon + 180) / 360) * n), Math.floor(((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * n)];
};
async function loadTransit() {
  if (!TRANSIT || !look.transit) return setSource("transit", empty), [];
  if (map.getZoom() < 9.5) return [];
  const b = map.getBounds();
  const [x0, y0] = tileXY(b.getWest(), b.getNorth(), 10), [x1, y1] = tileXY(b.getEast(), b.getSouth(), 10);
  const keys = [];
  for (let x = x0; x <= x1; x++) for (let y = y0; y <= y1; y++) keys.push(`${x}/${y}`);
  if (keys.length > 12) return [];
  for (const k of keys) {
    if (!transitTiles.has(k)) transitTiles.set(k, fetch(`${TRANSIT}?tile=10/${k}`).then((r) => (r.ok ? r.json() : null)).then((d) => d?.features || null).catch(() => null));
  }
  const all = [];
  for (const [k, p] of transitTiles) {
    const fs = await p;
    if (fs === null) transitTiles.delete(k); // asked again next time
    else all.push(...fs);
  }
  if (look.transit) setSource("transit", { type: "FeatureCollection", features: all });
  return all;
}
map.on("moveend", () => {
  clearTimeout(transitTimer);
  transitTimer = setTimeout(loadTransit, 250);
});
function setTransit(on) {
  look.transit = on;
  store.set("hm-transit", on ? "1" : "0");
  transitBox && (transitBox.checked = on);
  document.querySelectorAll(".lm-bubble-transit").forEach((b) => b.setAttribute("aria-pressed", String(on)));
  loadTransit();
}
let transitBox = null;
// a tap on a line: its name
function transitAt(e) {
  if (!TRANSIT || !look.transit || !map.getLayer("lm-transit-line")) return false;
  const f = map.queryRenderedFeatures([[e.point.x - 6, e.point.y - 6], [e.point.x + 6, e.point.y + 6]], { layers: ["lm-transit-line"] })[0];
  if (!f) return false;
  const p = f.properties, box = el("div", "lm-traffic-pop");
  box.append(el("strong", "", {}, [T.transitKinds[p.kind] || p.kind, p.ref].filter(Boolean).join(" ")));
  if (p.name) box.append(el("p", "", {}, p.name));
  if (p.network) box.append(el("p", "", {}, p.network));
  new maplibregl.Popup({ maxWidth: "300px" }).setLngLat(e.lngLat).setDOMContent(box).addTo(map);
  return true;
}
// "metro lines in Lausanne", "U-Bahn in Berlin", "tram in Zürich": the lines of that city, on the map and listed
const TRANSIT_WORDS = /^(?:the\s+)?(metro|subway|underground|u-?bahn|m[ée]tro|metropolitana|tram|trams|tramway|strassenbahn|straßenbahn|tranvia|light rail|stadtbahn|public transport|transit|öv|öffentlicher verkehr|transports? publics?|trasporti? pubblici?)(?:\s+(?:lines?|network|map|linien|netz|lignes?|réseau|linee|rete))?\s+(?:in|à|a|im|in der|de|di)\s+(.+)$/i;
function transitQuery(text) {
  const m = TRANSIT_WORDS.exec(text.trim());
  if (!m) return null;
  const w = m[1].toLowerCase();
  const kinds = /tram|strassen|straßen|tranvia/.test(w) ? ["tram"] : /light|stadt/.test(w) ? ["light_rail"]
    : /metro|métro|subway|underground|u-?bahn/.test(w) ? ["subway", "light_rail", "monorail"] : ["subway", "light_rail", "monorail", "tram"];
  return { kinds, place: m[2].trim() };
}
async function transitSearch(q, text) {
  clear();
  showView(listView);
  title.textContent = text;
  notes.textContent = "";
  say(T.searching);
  setTransit(true);
  let place = null;
  try {
    const r = await fetch(`${root.dataset.tiles}/v1/${root.dataset.key}/geocode/api?${new URLSearchParams({ q: q.place, limit: "1", lang: ["de", "fr"].includes(UI) ? UI : "en" })}`);
    place = (await r.json()).features?.[0];
  } catch {}
  if (!place) return say(T.failed, true);
  const name = place.properties.name || q.place, ext = place.properties.extent; // [west, north, east, south]
  title.textContent = T.transitIn(name);
  const done = new Promise((ok) => map.once("moveend", ok));
  if (ext) fitBox([ext[0], ext[3], ext[2], ext[1]], 13);
  else fit(() => map.flyTo({ center: place.geometry.coordinates, zoom: 12, duration: 900, padding: pad() }));
  await done;
  const fs = await loadTransit();
  say("");
  // the lines, once each: by kind, then by number
  const seen = new Map();
  // the lines that run in the city (tiles of other places may be loaded), with a number or a name
  const box = ext ? [ext[0], ext[3], ext[2], ext[1]] : (() => { const b = map.getBounds(); return [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()]; })();
  const inside = (f) => f.geometry.coordinates.some((ln) => ln.some(([x, y]) => x >= box[0] && x <= box[2] && y >= box[1] && y <= box[3]));
  for (const f of fs) {
    const p = f.properties;
    if (!q.kinds.includes(p.kind) || !(p.ref || p.name) || !inside(f)) continue;
    const k = `${p.kind}|${p.ref || p.name}`;
    if (!seen.has(k)) seen.set(k, p);
  }
  const lines = [...seen.values()].sort((a, b) => q.kinds.indexOf(a.kind) - q.kinds.indexOf(b.kind) || String(a.ref || a.name).localeCompare(String(b.ref || b.name), undefined, { numeric: true }));
  if (!lines.length) return (notes.textContent = T.transitNone);
  count.textContent = T.results(lines.length);
  for (const p of lines) {
    const li = el("li", "lm-transit-item");
    const sw = el("span", "lm-transit-ref", {}, p.ref || "–");
    sw.style.background = p.colour || "#7a7f87";
    li.append(sw, el("span", "lm-transit-name", {}, [T.transitKinds[p.kind], p.name && p.name !== p.ref ? p.name : ""].filter(Boolean).join(" · ")));
    results.append(li);
  }
  setSheet("half");
}

// ------------------------------------------------------------------ the bubbles
// Over the map: searches of the map view in a tap (fewer on a phone), and Transit, which switches its lines
const bubbles = el("div", "lm-bubbles");
for (const [q, label, wide] of T.bubbles) {
  if (q === "transit" && !TRANSIT) continue;
  const b = el("button", `lm-bubble${wide ? " lm-wide-only" : ""}${q === "transit" ? " lm-bubble-transit" : ""}`, { type: "button" }, label);
  if (q === "transit") {
    b.setAttribute("aria-pressed", String(look.transit));
    b.addEventListener("click", () => setTransit(!look.transit));
  } else b.addEventListener("click", () => run(q, {}));
  bubbles.append(b);
}
mapEl.append(bubbles);
// viewKm2 is about the size of the map view; the search does not search views over 1,000 km² (a
// phone at zoom 10): the searches of the bubbles go when the map is out further, Transit stays
const viewKm2 = () => {
  const b = map.getBounds(), lat = (b.getNorth() + b.getSouth()) / 2;
  return (b.getEast() - b.getWest()) * 111.32 * Math.cos(lat * Math.PI / 180) * (b.getNorth() - b.getSouth()) * 110.57;
};
const MAX_VIEW_KM2 = 800;
const farOut = () => viewKm2() > MAX_VIEW_KM2;
const markFar = () => bubbles.classList.toggle("lm-far", farOut());
map.on("moveend", markFar);
map.on("resize", markFar);
map.once("load", markFar);

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
    if (key === "auto" || key === "driving") autoDark = nightNow();
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
// 3D: the terrain and the map tilted, from the 2D/3D button on the map; 3D buildings are a
// switch of the menu of their own (setBuildings)
function setThreeD(on) {
  look.threeD = on;
  store.set("hm-3d", on ? "1" : "0");
  // the new style stops a camera move: tilt again once it is there
  const tilt = () => {
    if (on && map.getPitch() < 30) map.easeTo({ pitch: 60, duration: 800 });
    if (!on) map.easeTo({ pitch: 0, bearing: 0, duration: 800 });
  };
  restyle(tilt);
  tilt();
  showDim();
}
function setBuildings(on) {
  look.buildings = on;
  store.set("hm-buildings", on ? "1" : "0");
  restyle();
}
const buildingsRow = toggle(T.buildings3D, T.buildings3DNote, look.buildings, setBuildings);
const forgetBtn = el("button", "lm-chip lm-forget", { type: "button" }, T.forgetRecent);
forgetBtn.addEventListener("click", () => {
  saved.recent = [];
  store.set("hm-recent", "[]");
  forgetBtn.disabled = true;
});
const trafficRow = toggle(T.traffic, T.trafficNote, look.traffic, setTraffic);
const transitRow = toggle(T.transit, T.transitNote, look.transit, setTransit);
transitBox = transitRow.querySelector("input");
trafficBox = trafficRow.querySelector("input");
// the language; a change applies at once (setLang)
const langSel = el("select", "lm-lang-select", { "aria-label": T.language });
for (const [k, name] of [["", T.automatic], ...languages]) {
  const o = el("option", "", { value: k }, name);
  if (k === chosenLang) o.selected = true;
  langSel.append(o);
}
langSel.addEventListener("change", () => {
  store.set("hm-lang", langSel.value);
  setLang(langSel.value);
});
const menuFootText = () => (TRAFFIC ? `${T.credit} ${T.trafficCredit}` : T.credit);
const menuFoot = el("p", "lm-menu-foot", {}, menuFootText());
const langRow = el("label", "lm-toggle lm-lang");
const langText = el("span", "lm-toggle-text");
langText.append(el("span", "", {}, T.language), el("span", "lm-toggle-note", {}, T.languageNote));
langRow.append(langText, langSel);
menu.append(menuHead, el("h2", "lm-menu-title", {}, T.mapStyle), styleGrid, el("h2", "lm-menu-title", {}, T.show),
  buildingsRow,
  toggle(T.shading, T.shadingNote, look.shading, (on) => {
    look.shading = on;
    store.set("hm-shading", on ? "1" : "0");
    restyle();
  }),
  ...(TRAFFIC ? [trafficRow] : []),
  ...(TRANSIT ? [transitRow] : []),
  langRow,
  forgetBtn,
  poweredBy(), menuFoot);
root.append(backdrop, menu);
// poweredBy is the line "Powered by overpass.ch" of the menu, with the logo of overpass.ch
function poweredBy() {
  const a = el("a", "lm-powered", { href: "/", title: "overpass.ch" });
  const light = el("img", "lm-powered-light", { src: root.dataset.overpass, alt: "overpass.ch", height: "22" });
  const dark = el("img", "lm-powered-dark", { src: root.dataset.overpassDark, alt: "overpass.ch", height: "22" });
  a.append(el("span", "", {}, T.poweredBy), light, dark);
  return a;
}
// Day and night and Driving follow sunrise and sunset where the visitor is (nightNow), checked every 5 minutes
function checkDayNight() {
  if (!followsSun() || navLook) return;
  const dark = nightNow();
  if (dark === autoDark) return;
  autoDark = dark;
  document.documentElement.dataset.theme = theme();
  restyle();
}
setInterval(checkDayNight, 300e3);

// A new style (another look, a tunnel, sunset) goes onto the map as a diff of the one shown: the tiles of the sources
// both have stay, and only the layers that differ change. The styles are kept here once fetched (and turned
// dark), so a switch back and forth in tunnels asks the server nothing. then: after the switch.
let restyleN = 0;
const styleCache = new Map(); // the URL of a style (with " night": turned dark) → a promise of its JSON
function styleJSON(url, night = false) {
  const key = night ? `${url} night` : url;
  if (!styleCache.has(key)) {
    // the night of a style that has none (driving): its colours turned dark
    const p = night ? styleJSON(url).then((st) => nightStyle(structuredClone(st)))
      : fetch(url).then((r) => { if (!r.ok) throw new Error(`style ${r.status}`); return r.json(); });
    p.catch(() => styleCache.delete(key));
    styleCache.set(key, p);
  }
  return styleCache.get(key);
}
function restyle(then) {
  const n = ++restyleN, url = styleURL();
  styleJSON(url, shownStyle().endsWith("-night")).then((st) => {
    if (n !== restyleN) return;
    const was = map.style;
    // style.load comes after a diff too (our layers are there, and kept); a style that cannot be a diff is built
    // anew, with style.load once it is there
    map.setStyle(st, { diff: true, transformStyle: ourStyle });
    if (map.style !== was) return then && map.once("style.load", then);
    then?.();
  }).catch(() => {
    if (n !== restyleN) return;
    map.setStyle(url, { diff: false });
    if (then) map.once("style.load", then);
  });
}
// ourStyle makes the style st (from the server) the one the map shows: the light style recoloured (pastel), 3D
// buildings see-through, the globe, and our sources and layers of the style shown (prev) carried over, each
// before the same layer of the style as before, the names in the colours of the theme
function ourStyle(prev, st) {
  st = structuredClone(st);
  if (shownStyle() === "light") pastelStyle(st);
  for (const l of st.layers) if (l.type === "fill-extrusion") l.paint = { ...l.paint, "fill-extrusion-opacity": 0.6 };
  st.projection = { type: "globe" };
  if (!prev) return st;
  for (const [id, src] of Object.entries(prev.sources || {})) if (id.startsWith("lm-")) st.sources[id] = src;
  const ids = new Set(st.layers.map((l) => l.id));
  const firstSymbol = st.layers.find((l) => l.type === "symbol")?.id;
  // ours by the layer of the style that came next (null: at the top)
  const before = new Map();
  let ours = [];
  for (const l of prev.layers || []) {
    if (l.id === "lm-pois-near") continue; // made again from the pois of the new style
    if (l.id.startsWith("lm-")) {
      ours.push(TEXT_COLOUR[l.id] ? { ...l, paint: { ...l.paint, ...textPaint(l.id) } } : l);
      continue;
    }
    if (!ours.length) continue;
    const at = ids.has(l.id) ? l.id : l.type === "symbol" ? firstSymbol || null : null;
    before.set(at, [...(before.get(at) || []), ...ours]);
    ours = [];
  }
  before.set(null, [...(before.get(null) || []), ...ours]);
  const pois = st.layers.find((l) => l.id === "pois");
  const near = pois && poiTiles?.length && prev.sources?.["lm-pois-near"] && nearPois(pois);
  if (near) {
    st.sources["lm-pois-near"] ||= NEAR_SOURCE;
    pois.filter = near.filter;
    before.set("pois", [...(before.get("pois") || []), near.layer]);
  }
  st.layers = [...st.layers.flatMap((l) => [...(before.get(l.id) || []), l]), ...before.get(null)];
  return st;
}
// nightStyle turns the colours of a style around in lightness, keeping their hue: light ground and
// white roads go dark, dark text goes light, orange motorways stay orange but darker
function nightStyle(st) {
  const hsl = (r, g, b) => {
    r /= 255; g /= 255; b /= 255;
    const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min;
    if (!d) return [0, 0, l];
    const sat = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    const h = max === r ? (g - b) / d + (g < b ? 6 : 0) : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
    return [h * 60, sat, l];
  };
  const night = (c) => {
    let m, rgb, a = 1;
    if ((m = /^#([0-9a-f]{3,8})$/i.exec(c))) {
      let h = m[1];
      if (h.length <= 4) h = [...h].map((x) => x + x).join("");
      rgb = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
      if (h.length === 8) a = parseInt(h.slice(6, 8), 16) / 255;
    } else if ((m = /^rgba?\(([^)]+)\)$/i.exec(c))) {
      const v = m[1].split(/[\s,/]+/).filter(Boolean).map(parseFloat);
      rgb = v.slice(0, 3);
      if (v.length > 3) a = v[3];
    } else if ((m = /^hsla?\(([^)]+)\)$/i.exec(c))) {
      const v = m[1].split(/[\s,/]+/).filter(Boolean).map(parseFloat);
      const [h, sat, l] = [v[0], v[1] / 100, v[2] / 100];
      return `hsla(${h}, ${Math.round(sat * 80)}%, ${Math.round((0.08 + (1 - l) * 0.77) * 100)}%, ${v.length > 3 ? v[3] : 1})`;
    } else return c;
    const [h, sat, l] = hsl(...rgb);
    return `hsla(${Math.round(h)}, ${Math.round(sat * 80)}%, ${Math.round((0.08 + (1 - l) * 0.77) * 100)}%, ${a})`;
  };
  const walk = (v) => (typeof v === "string" ? night(v) : Array.isArray(v) ? v.map(walk) : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).map(([k, x]) => [k, walk(x)])) : v);
  for (const l of st.layers || []) if (l.paint) l.paint = walk(l.paint);
  return st;
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
  // peek: the search box and the title under it, with the buttons of a place found; the search box alone while nothing is under it
  peek: Math.min(form.offsetTop + form.offsetHeight + (title.textContent || notes.textContent ? 64 : 14) + (root.querySelector(".lm-place-acts:not(:empty)") ? 44 : 0), innerHeight * 0.4),
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

// ------------------------------------------------------------------ saved in the browser
// Starred places (hm-stars) and the last searches (hm-recent), kept in the browser like the settings
// (map style, 3D, language, traffic, mode, sound): nothing of it goes to the server.
const readList = (k) => { try { const v = JSON.parse(store.get(k) || "[]"); return Array.isArray(v) ? v : []; } catch { return []; } };
const saved = { stars: readList("hm-stars"), recent: readList("hm-recent") };
const starKey = (ll) => `${ll[0].toFixed(5)},${ll[1].toFixed(5)}`;
const isStarred = (ll) => saved.stars.some((x) => starKey(x.ll) === starKey(ll));
function toggleStar(place) {
  const k = starKey(place.ll);
  if (isStarred(place.ll)) saved.stars = saved.stars.filter((x) => starKey(x.ll) !== k);
  else saved.stars = [{ name: place.name, detail: place.detail || "", ll: [+place.ll[0].toFixed(5), +place.ll[1].toFixed(5)] }, ...saved.stars].slice(0, 200);
  store.set("hm-stars", JSON.stringify(saved.stars));
  drawStars();
}
function drawStars() {
  setSource("stars", { type: "FeatureCollection", features: saved.stars.map((x) => ({ type: "Feature", geometry: { type: "Point", coordinates: x.ll }, properties: { name: x.name } })) });
}
function addRecent(text, extra) {
  const t = text.trim();
  if (!t) return;
  saved.recent = [{ text: t, extra }, ...saved.recent.filter((x) => x.text !== t)].slice(0, 10);
  store.set("hm-recent", JSON.stringify(saved.recent));
}
// the star of a place: Save, or Saved (a tap again forgets it); place: {ll, name, detail}, or a
// function giving it (the name of a tapped point comes later)
function starButton(place) {
  const get = typeof place === "function" ? place : () => place;
  const b = el("button", "lm-action lm-star", { type: "button" });
  const show = () => {
    const on = isStarred(get().ll);
    b.classList.toggle("lm-on", on);
    b.setAttribute("aria-pressed", String(on));
    b.innerHTML = svg(ICON.star, 18);
    b.append(el("span", "", {}, on ? T.saved : T.save));
    b.title = on ? T.saved : T.save;
  };
  show();
  b.addEventListener("click", () => { toggleStar(get()); show(); });
  return b;
}
// ------------------------------------------------------------------ sharing
// Share: the share sheet of the phone, else the link copied. Links: a point or place
// ?at=lat,lon&n=name, a route ?from=lat,lon&via=lat,lon&to=lat,lon&mode=car (without from: the location
// of the visitor; via and its name vn for each stop), a search with its place ?q=…&sel=… (the address the page writes itself)
const pageURL = () => location.origin + location.pathname;
const ll5 = (ll) => `${ll[1].toFixed(5)},${ll[0].toFixed(5)}`;
function pointLink(ll, name) {
  const q = new URLSearchParams({ at: ll5(ll) });
  if (name) q.set("n", name);
  return `${pageURL()}?${q}#map=17/${ll[1].toFixed(5)}/${ll[0].toFixed(5)}`;
}
function routeLink() {
  const q = new URLSearchParams();
  if (route.from && !route.from.me) q.set("from", ll5(route.from.ll)), q.set("fn", route.from.label || "");
  for (const x of route.stops) if (x) q.append("via", ll5(x.ll)), q.append("vn", x.label || "");
  if (route.to) q.set("to", ll5(route.to.ll)), q.set("tn", route.to.label || "");
  q.set("mode", route.mode);
  return `${pageURL()}?${q}`;
}
// what: () => {title, url}
function shareButton(what, iconOnly = false) {
  const b = el("button", `lm-action lm-share${iconOnly ? " lm-icon-only" : ""}`, { type: "button", title: T.share, "aria-label": T.share });
  b.innerHTML = svg(ICON.share, 18);
  const label = el("span", "", {}, T.share);
  b.append(label);
  b.addEventListener("click", async () => {
    const { title, url } = what();
    if (navigator.share) {
      try { await navigator.share({ title, url }); return; } catch (err) { if (err?.name === "AbortError") return; }
    }
    try {
      await navigator.clipboard.writeText(url);
      label.textContent = T.linkCopied;
      b.classList.add("lm-done");
      setTimeout(() => { label.textContent = T.share; b.classList.remove("lm-done"); }, 2000);
    } catch { prompt(T.share, url); }
  });
  return b;
}

// a starred place opened: the map there, and the card of the point with its name
function openStar(x) {
  fit(() => map.flyTo({ center: x.ll, zoom: Math.max(map.getZoom(), 16), duration: 900, padding: pad() }));
  here({ lng: x.ll[0], lat: x.ll[1] }, x.name);
}

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
// swallowTap: a list picked at the finger going down closes at once, and the finger lifting would
// tap what was under it, the map (the place there replaced the one picked) or a card
let ghostUntil = 0;
const swallowTap = () => {
  ghostUntil = Date.now() + 700;
  const eat = (e) => {
    if (Date.now() < ghostUntil) { e.preventDefault(); e.stopPropagation(); }
    removeEventListener("click", eat, true);
  };
  addEventListener("click", eat, true);
  setTimeout(() => removeEventListener("click", eat, true), 800);
};
const placeOf = (s) => Object.fromEntries(placeKeys.filter((k) => s[k]).map((k) => [k, s[k]]));
const renderList = (items) => {
  options = items;
  list.innerHTML = "";
  if (!items.length) return closeList();
  items.forEach((s, i) => {
    const li = el("li", `lm-opt lm-opt-${s.kind}`, { role: "option", id: `lm-opt-${i}`, "aria-selected": "false" });
    const ic = el("span", "lm-opt-icon");
    ic.innerHTML = svg(s.kind === "place" ? ICON.pin : s.kind === "star" ? ICON.star : s.kind === "recent" ? ICON.recent : ICON.search, 16);
    const txt = el("span", "lm-opt-text");
    txt.append(el("span", "lm-opt-label", {}, s.label));
    if (s.detail) txt.append(el("span", "lm-opt-detail", {}, s.detail));
    li.append(ic, txt);
    li.addEventListener("pointerdown", (e) => { e.preventDefault(); swallowTap(); pick(i); }); // at once (a phone: before the box loses its focus)
    list.append(li);
  });
  list.hidden = false;
  input.setAttribute("aria-expanded", "true");
  markActive(-1);
};
const suggest = () => {
  const q = input.value;
  if (!q.trim() && document.activeElement === input) return Promise.resolve(renderList(savedItems()));
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
// on an empty search box: the starred places, then the last searches
const savedItems = () => [
  ...saved.stars.slice(0, 8).map((x) => ({ kind: "star", label: x.name, detail: x.detail, star: x })),
  ...saved.recent.map((x) => ({ kind: "recent", label: x.text, text: x.text, extra: x.extra || {} })),
];
const pick = (i) => {
  const s = options[i];
  if (!s) return;
  if (s.kind === "star") {
    closeList();
    if (phone()) input.blur();
    return openStar(s.star);
  }
  if (s.kind === "recent") {
    input.value = s.text;
    closeList();
    if (phone()) input.blur();
    return run(s.text, s.extra);
  }
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
  if (!input.value) newHint();
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
input.addEventListener("focus", () => {
  setSheet("full");
  if (!input.value.trim()) renderList(savedItems());
});
input.addEventListener("blur", () => setTimeout(closeList, 120));
// the X clears it all: the text, the results, the place shown, directions; back to the start
clearBtn.addEventListener("click", () => {
  input.value = "";
  newHint();
  clearBtn.hidden = true;
  closeList();
  runCtl?.abort();
  state.text = "";
  state.extra = {};
  say("");
  hereDone();
  showView(listView);
  clear();
  intro();
  setSource("route", empty);
  setSource("routetraffic", empty);
  showOnRoute(null);
  writeURL(null);
  input.focus();
  setSheet("full"); // the box stays in focus (a tap on the X keeps it): the starred places and last searches
  renderList(savedItems());
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
  meForSun = [pos.coords.longitude, pos.coords.latitude];
  setTimeout(checkDayNight, 0); // the sun where the visitor is
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
    { enableHighAccuracy: true, timeout: quiet ? 60000 : 15000, maximumAge: 60000 }, // at the start, time to answer the question
  );
}
locBtn.addEventListener("click", () => showMe(false));
// allowed later (the question of the browser answered after a while, or in its settings): the map goes there
navigator.permissions?.query({ name: "geolocation" }).then((st) => {
  st.addEventListener("change", () => { if (st.state === "granted" && !nav.on) showMe(false); });
}).catch(() => {});
map.once("dragstart", () => { state.touched = true; });
map.on("moveend", () => setTimeout(() => store.set("hm-last-view", location.hash), 0)); // after the hash follows the map

// run searches a text; extra holds place, place2, region and what, as the API takes them
async function run(text, extra = {}, { push = true, bbox = null, sel = null } = {}) {
  runCtl?.abort();
  const ctl = runCtl = new AbortController();
  input.value = text;
  clearBtn.hidden = !text;
  if (push) addRecent(text, extra);
  const tq = TRANSIT && transitQuery(text);
  if (tq) {
    state.text = text;
    state.extra = {};
    if (push) writeURL(null);
    return transitSearch(tq, text);
  }
  const params = new URLSearchParams({ q: text, bbox: bbox || viewBox(), lang: LANG }); // places in the language of the reader ("Tessin")
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
  else { // back to no search: the panel folds down to the search box, unless directions are open
    state.text = "";
    state.extra = {};
    input.value = "";
    newHint();
    clearBtn.hidden = true;
    closeList();
    clear();
    intro();
    if (routeView.hidden) {
      hereDone();
      showView(listView);
      if (document.activeElement === input) input.blur();
      setSheet("peek");
    }
  }
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
const placeActs = el("div", "lm-actions lm-place-acts"); // Directions when the search found one place
head.append(title, notes, placeActs);
const placePic = el("figure", "lm-pic lm-place-pic"); // the photo of a town or region found
placePic.hidden = true;
listView.append(head, placePic, chips, count, results, moreBtn, query);
qCopy.addEventListener("click", () => navigator.clipboard?.writeText(qPre.textContent).then(() => {
  qCopy.textContent = T.copied;
  setTimeout(() => { qCopy.textContent = T.copy; }, 1500);
}));

// an empty search box with nothing under it: the panel folds down to the box (a phone)
function foldIfEmpty() {
  if (listView.hidden || input.value || document.activeElement === input) return;
  if (title.textContent || notes.textContent || results.childElementCount || !status.hidden) return;
  setSheet("peek");
}
// the start: the map and the search box, nothing under it
function intro() {
  clear();
  title.textContent = "";
  notes.textContent = "";
  if (document.activeElement !== input) setSheet("peek"); // on a phone the map, and the search box at the bottom
}
function clear() {
  state.answer = null; // no search: a card has no list to go back to
  placeActs.innerHTML = "";
  showDirBtn();
  placePic.hidden = true;
  placePic.innerHTML = "";
  state.placeTarget = null;
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
// The padding of an easeTo or flyTo stays with the map, and fitBounds adds its own to it: with
// both, a box may not fit at all and the map stays put. Every move of the page starts from none.
const fit = (fn, ms = 1300) => {
  quietUntil = Date.now() + ms;
  const p = map.getPadding();
  if (p.top || p.bottom || p.left || p.right) map.setPadding({ top: 0, bottom: 0, left: 0, right: 0 });
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
  if (touched && Date.now() > quietUntil && state.searchable && !farOut()) areaBtn.hidden = false;
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
    const c = center(place);
    if (c) state.placeTarget = { ll: c, label: place.properties.name || d.text };
    if (c && ROUTE) placeActs.append(routeButton(() => directions(state.placeTarget)));
    if (c) placeActs.append(starButton({ ll: c, name: place.properties.name || d.text, detail: place.properties.detail || "" }));
    placeActs.append(shareButton(() => ({ title: place.properties.name || d.text, url: location.href }), true));
    showDirBtn();
    setSheet("peek"); // again: with the buttons
    const id = place.properties.id;
    if (PICTURES && /^relation\/\d+$/.test(id || "")) {
      picture({}, 500, id).then((pic) => {
        if (!pic || state.answer !== d) return;
        placePic.hidden = false;
        placePic.append(el("img", "", { src: pic.url, alt: place.properties.name || "" }));
        const cap = el("figcaption");
        cap.append(`${T.photo}${pic.credit ? `: ${pic.credit}` : ""} · `);
        cap.append(el("a", "", { href: pic.page, target: "_blank", rel: "noopener" }, "Wikimedia Commons"));
        placePic.append(cap);
      });
    }
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
// pad keeps what the map fits to out from under the sheet on a phone. It leaves at least 60 pixels
// of map each way: with more padding than map MapLibre does not move at all ("Map cannot fit")
const pad = () => {
  const p = phone()
    ? { top: 40, left: 30, right: 30, bottom: Math.min(innerHeight - 140, sheetHeights()[sheet.state] + 20) }
    : { top: 50, left: 50, right: 50, bottom: 50 };
  const h = mapEl.clientHeight, w = mapEl.clientWidth;
  if (h && p.top + p.bottom > h - 60) p.bottom = Math.max(0, h - 60 - p.top);
  if (w && p.left + p.right > w - 60) p.left = p.right = Math.max(0, (w - 60) / 2);
  return p;
};

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
// t: the tags of a place; osm: a relation ("relation/1682682") whose item of Wikidata has the picture
function picture(t, w, osm) {
  const q = new URLSearchParams();
  if (osm) q.set("osm", osm);
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
  // within a week: without an end, opening_hours.js looks for the next change year after year, with
  // the holidays of each year, for hours that never change (24/7 with holidays): the fuel stations
  // around Zurich froze the page for a minute
  const next = oh.getNextChange(now, new Date(now.getTime() + 8 * 86400e3));
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
  count.textContent = T.withHours(state.features.length, o.label, all);
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
  detailView.append(back, closeX());
  const fig = el("figure", "lm-pic"); // under the buttons: a picture arriving late does not move them
  fig.hidden = true;
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
  if (c) actions.append(starButton({ ll: c, name: p.name || kindOf(p).label || T.noName, detail: kindOf(p).label }));
  if (c) actions.append(shareButton(() => ({ title: p.name || kindOf(p).label || "hippomaps", url: location.href.includes("sel=") ? location.href : pointLink(c, p.name) })));
  const web = safeURL(t.website || t["contact:website"]);
  if (web) action(T.website, web, ICON.globe);
  const phone = (t.phone || t["contact:phone"] || "").split(";")[0].trim();
  if (phone) action(T.call, `tel:${phone.replace(/[^+\d]/g, "")}`, ICON.phone);
  if (/^(node|way|relation)\/\d+$/.test(p.id || "")) action(T.osm, `https://www.openstreetmap.org/${p.id}`, ICON.map);
  detailView.append(actions, fig);

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
  if (t.wheelchair) row(ICON.wheel, el("span", "", {}, `${T.wheelchair}: ${human(t.wheelchair)}`));
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
// A tap acts after 280 ms, unless it was the start of a double tap or of a zoom with one finger
// (tap, then drag): those zoom the map and do nothing else.
let tapTimer = 0;
map.on("dblclick", () => clearTimeout(tapTimer));
map.on("zoomstart", (e) => { if (e.originalEvent) clearTimeout(tapTimer); });
map.on("click", (e) => {
  if (e.defaultPrevented || Date.now() < ghostUntil) return;
  clearTimeout(tapTimer);
  tapTimer = setTimeout(() => onTap(e), 280);
});
function onTap(e) {
  if (trafficAt(e)) return;
  if (!nav.on && routeView.hidden && transitAt(e)) return;
  if (!nav.on && routeView.hidden && map.getLayer("lm-stars")) { // a starred place
    const r = 12, f = map.queryRenderedFeatures([[e.point.x - r, e.point.y - r], [e.point.x + r, e.point.y + r]], { layers: ["lm-stars"] })[0];
    const x = f && saved.stars.find((s) => starKey(s.ll) === starKey(f.geometry.coordinates));
    if (x) return openStar(x);
  }
  if ((nav.on || !routeView.hidden) && alongAt(e.point)) return;
  if (nav.on) return;
  if (document.activeElement === input || !list.hidden) {
    closeList();
    input.blur();
    setSheet(state.answer || !hereView.hidden ? "half" : "peek");
    return;
  }
  const hit = resultAt(e.point);
  // in directions a tap fills in a start or destination still missing; with both, it shows the place
  if (!routeView.hidden && (!route.from || !route.to)) {
    // a tap sets the start, or the destination while only that is missing
    const c = hit && center(hit);
    const p = c ? { ll: c, label: hit.properties.name || T.mapPoint }
      : { ll: [+e.lngLat.lng.toFixed(5), +e.lngLat.lat.toFixed(5)], label: symbolAt(e.point) || T.mapPoint };
    if (route.from && !route.to) {
      route.to = p;
      renderRoute();
      return findRoute();
    }
    return setFrom(p);
  }
  const fromRoute = !routeView.hidden || route.keep; // the place's card goes back to the route
  if (hit) select(hit, { fly: false });
  else here(e.lngLat, symbolAt(e.point), fromRoute);
  keepInView(e.lngLat);
}
// keepInView moves the map on a phone when the sheet, risen with the place tapped, covers it:
// the place comes to the middle of the map still in view
function keepInView(ll) {
  if (!phone() || sheet.state === "full") return;
  const p = map.project(ll), seen = mapEl.clientHeight - sheetHeights()[sheet.state];
  if (p.y > 60 && p.y < seen - 60) return;
  map.panBy([0, p.y - Math.max(seen / 2, 60)], { duration: 400 });
}
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
  state.herePoint = null;
  setSource("pin", empty);
  setSource("area", empty);
}
// the X of a card: the card and what it highlights go; back to the results, or to the start
function closeCard() {
  if (route.keep) { route.keep = false; routeDone(); }
  hereDone();
  setSource("selected", empty);
  state.selected = null;
  if (state.answer) { showList(); setSheet("half"); }
  else { showView(listView); intro(); }
}
const closeX = () => {
  const x = el("button", "lm-icon-btn lm-card-x", { type: "button", title: T.close, "aria-label": T.close });
  x.innerHTML = svg(ICON.close, 18);
  x.addEventListener("click", closeCard);
  return x;
};
// back: from directions, the card has Directions to go back to the route
async function here(ll, symbol, backToRoute = false) {
  hereCtl?.abort();
  const ctl = hereCtl = new AbortController();
  const lat = +ll.lat.toFixed(5), lon = +ll.lng.toFixed(5);
  setSource("pin", one({ type: "Feature", geometry: { type: "Point", coordinates: [lon, lat] }, properties: {} }));
  setSource("area", empty);
  setSource("selected", empty);
  state.selected = null;
  hereView.innerHTML = "";
  route.keep = backToRoute;
  if (backToRoute) {
    const back = el("button", "lm-back", { type: "button" });
    back.innerHTML = svg(ICON.back, 18);
    back.append(T.directions);
    const toRoute = () => { hereDone(); showView(routeView); renderRoute(); findRoute(); };
    back.addEventListener("click", toRoute);
    hereView.append(back);
    // the point instead of the destination, or of the start
    const swap = el("div", "lm-swap-acts");
    for (const [label, end] of [[T.useAsTo, "to"], [T.useAsStop, "stop"], [T.useAsFrom, "from"]]) {
      if (end === "stop" && (!route.from || !route.to || route.stops.length >= MAX_STOPS)) continue;
      const b = el("button", "lm-chip", { type: "button" }, label);
      b.addEventListener("click", () => {
        const p = { ll: [+ll.lng.toFixed(5), +ll.lat.toFixed(5)], label: hereView.querySelector(".lm-dname")?.textContent || T.mapPoint };
        if (end === "stop") insertStop(p); else route[end] = p;
        toRoute();
      });
      swap.append(b);
    }
    hereView.append(swap);
  } else if (state.answer) {
    const back = el("button", "lm-back", { type: "button" });
    back.innerHTML = svg(ICON.back, 18);
    back.append(T.back);
    back.addEventListener("click", () => { hereDone(); showView(listView); });
    hereView.append(back);
  }
  hereView.append(closeX());
  const h = el("h2", "lm-dname", {}, symbol || T.here);
  state.herePoint = { ll: [lon, lat], get label() { return h.textContent; } };
  const sub = el("p", "lm-dsub", {}, `${lat.toFixed(5)}, ${lon.toFixed(5)}`);
  const note = el("p", "lm-notes lm-here-note", {}, T.hereLooking);
  hereView.append(h, sub);
  if (ROUTE) {
    const acts = el("div", "lm-actions lm-here-actions");
    acts.append(routeButton(() => directions({ ll: [lon, lat], label: h.textContent })));
    acts.append(starButton(() => ({ ll: [lon, lat], name: h.textContent, detail: `${lat.toFixed(5)}, ${lon.toFixed(5)}` })));
    acts.append(shareButton(() => ({ title: h.textContent, url: pointLink([lon, lat], h.textContent) })));
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
  // the picture of the town or area (of the place close in), when its item of Wikidata has one
  const rel = (area && (zoom < 14 || !d.place) ? area : d.place || area)?.place;
  if (PICTURES && /^relation\/\d+$/.test(rel || "")) {
    const fig = el("figure", "lm-pic lm-place-pic");
    fig.hidden = true;
    const acts = hereView.querySelector(".lm-here-actions");
    if (acts) acts.after(fig); else note.before(fig); // under the buttons: arriving late, it does not move them
    picture({}, 500, rel).then((pic) => {
      if (!pic || ctl.signal.aborted || hereView.hidden) return;
      fig.hidden = false;
      fig.append(el("img", "", { src: pic.url, alt: h.textContent }));
      const cap = el("figcaption");
      cap.append(`${T.photo}${pic.credit ? `: ${pic.credit}` : ""} · `);
      cap.append(el("a", "", { href: pic.page, target: "_blank", rel: "noopener" }, "Wikimedia Commons"));
      fig.append(cap);
    });
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
const route = { keep: false, from: null, to: null, stops: [], mode: ["car", "bike", "walk"].includes(store.get("hm-mode")) ? store.get("hm-mode") : "car", back: null, ctl: null };
function routeButton(go) {
  const b = el("button", "lm-action", { type: "button" });
  b.innerHTML = svg(ICON.directions, 18);
  b.append(el("span", "", {}, T.directions));
  b.addEventListener("click", go);
  return b;
}
function routeDone() {
  unfollowMe();
  route.ctl?.abort();
  alongDone();
  setSource("route", empty);
}
// the stretches of a route slowed by traffic (d.traffic: [from, to, level] in indexes of the line),
// from point index `from` on
function drawRouteTraffic(d, from = 0, at = null) {
  const fs = [];
  for (const [a, b, level] of d?.traffic || []) {
    if (b <= from) continue;
    const pts = d.line.slice(Math.max(a, from), b + 1);
    if (a < from && at) pts.unshift(at);
    if (pts.length > 1) fs.push({ type: "Feature", geometry: { type: "LineString", coordinates: pts }, properties: { level } });
  }
  if (!fs.length && !sources.routetraffic.features.length) return; // none, as before
  setSource("routetraffic", { type: "FeatureCollection", features: fs });
}
// the colour of an arrival time: green, orange with slow traffic ahead, red with a queue ahead
function trafficClass(d, from = 0) {
  const ahead = (d?.traffic || []).filter(([, b]) => b > from).map(([, , level]) => level);
  return ahead.includes(2) ? "lm-late" : ahead.includes(1) ? "lm-slow" : "lm-ontime";
}
function drawEnds(line) {
  if (!line) setSource("routetraffic", empty);
  const fs = [];
  if (line) fs.push({ type: "Feature", geometry: { type: "LineString", coordinates: line }, properties: {} });
  if (route.from) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: route.from.ll }, properties: { end: "from" } });
  route.stops.forEach((x, i) => { if (x) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: x.ll }, properties: { end: "stop", n: String(i + 1) } }); });
  if (route.to) fs.push({ type: "Feature", geometry: { type: "Point", coordinates: route.to.ll }, properties: { end: "to" } });
  setSource("route", { type: "FeatureCollection", features: fs });
}
// go: start navigation once the route from the location is there
async function directions(to, go = false) {
  route.back = [listView, hereView, detailView].find((v) => !v.hidden) || listView;
  route.to = to;
  route.stops = [];
  route.alongCat = "";
  route.go = go;
  route.from = state.me ? { ll: state.me.at2, label: T.myPlace, me: true } : null;
  hereDone();
  showView(routeView);
  setSheet("half");
  renderRoute();
  if (!route.from) { // ask the browser once; the visitor can tap the map instead
    const me = await locate();
    if (me && !route.from && !routeView.hidden) setFrom({ ll: state.me.at2, label: T.myPlace, me: true });
    if (!route.to && !routeView.hidden) routeView.querySelector(".lm-end-to input")?.focus(); // the fields were drawn again
  } else findRoute();
}
// Directions from (or to) the location of the visitor follow it while they are open: the first
// location may be old or rough, and the phone may move before Start. A new route when the location
// moved by more than 30 m (and more than its accuracy), at most every 15 s.
const PLAN_MOVE_M = 30, PLAN_EVERY_MS = 15e3;
function followMe() {
  const wanted = () => !nav.on && !routeView.hidden && (route.from?.me || route.to?.me);
  if (!wanted() || !navigator.geolocation || route.watch != null) return;
  route.watch = navigator.geolocation.watchPosition((pos) => {
    if (!wanted()) return unfollowMe();
    meAt(pos);
    const end = route.from?.me ? route.from : route.to;
    const c = state.me.at2, [ax, ay] = flat(end.ll, c[1]), [bx, by] = flat(c, c[1]);
    // the first fix of the watch counts at 30 m: the location taken before may be old or rough
    const acc = pos.coords.accuracy || 0;
    if (Math.hypot(ax - bx, ay - by) <= (end.acc == null ? PLAN_MOVE_M : Math.max(PLAN_MOVE_M, Math.min(acc, 200)))) return;
    end.ll = c;
    end.acc = acc;
    clearTimeout(route.moveTimer);
    route.moveTimer = setTimeout(() => {
      route.moved = Date.now();
      if (wanted() && route.from && route.to) findRoute({ quiet: !!route.answer });
    }, Math.max(0, (route.moved || 0) + PLAN_EVERY_MS - Date.now()));
  }, () => {}, { enableHighAccuracy: true, maximumAge: 5000, timeout: 60000 });
}
function unfollowMe() {
  if (route.watch != null) navigator.geolocation.clearWatch(route.watch);
  route.watch = null;
  clearTimeout(route.moveTimer);
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
// the steps of a route, folded at first (their number in the title)
const stepsBox = el("details", "lm-steps-box");
const stepsTitle = el("summary", "lm-steps-title");
stepsBox.append(stepsTitle, routeSteps);
stepsBox.hidden = true;
function renderRoute() {
  routeView.innerHTML = "";
  routeHead.innerHTML = "";
  const back = el("button", "lm-back", { type: "button" });
  back.innerHTML = svg(ICON.back, 18);
  back.append(route.back === detailView ? (state.selected?.properties.name || T.back) : route.back === hereView ? T.here : state.answer ? T.back : T.close);
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
  ends.append(placeField("from"), ...route.stops.map((_, i) => placeField(i)), placeField("to"));
  const swap = el("button", "lm-icon-btn lm-swap", { type: "button", title: T.swap, "aria-label": T.swap });
  swap.innerHTML = svg(ICON.swap, 18);
  swap.disabled = !route.from || !route.to;
  swap.addEventListener("click", () => {
    [route.from, route.to] = [route.to, route.from];
    route.stops.reverse();
    renderRoute();
    findRoute();
  });
  ends.append(swap);
  routeHead.append(modes, ends);
  const acts = el("div", "lm-route-acts");
  if (route.stops.length < MAX_STOPS) {
    const add = el("button", "lm-chip lm-add-stop", { type: "button" });
    add.innerHTML = svg(ICON.plus, 16);
    add.append(T.addStop);
    add.addEventListener("click", () => {
      route.stops.push(null);
      renderRoute();
      findRoute();
      routeView.querySelectorAll(".lm-end-stop input")[route.stops.length - 1]?.focus();
    });
    acts.append(add);
  }
  routeHead.append(acts);
  if (!route.from?.me && navigator.geolocation) {
    const use = el("button", "lm-chip lm-use-me", { type: "button" }, T.useLocation);
    acts.prepend(use);
    use.addEventListener("click", async () => {
      const me = await locate();
      if (me) setFrom({ ll: state.me.at2, label: T.myPlace, me: true });
      else routeMsg.textContent = T.locBlocked;
    });
  }
  routeMsg.textContent = !route.from ? T.tapStart : !route.to ? T.tapEnd : "";
  routeSum.textContent = "";
  route.answer = null;
  routeSteps.innerHTML = "";
  stepsBox.hidden = true;
  routeView.append(back, routeHead, routeMsg, routeSum, stepsBox);
  drawEnds(null);
  if (route.from?.me || route.to?.me) followMe(); else unfollowMe();
}
// ------------------------------------------------------------------ start and destination
// Each is a field that suggests places and addresses from the geocoder of overpass.ch (Photon,
// with the key of the map, near the map view), with the location of the visitor first.
let fieldN = 0;
const geoLang = () => (["de", "fr"].includes(UI) ? UI : "en");
function placeLabel(p) {
  const street = [p.street, p.housenumber].filter(Boolean).join(" ");
  const town = p.city || p.town || p.village || p.district || "";
  const label = p.name || street || town || p.state || p.country || "";
  const where = [p.name && street, p.postcode && town ? `${p.postcode} ${town}` : town, p.country].filter((x) => x && x !== label).join(", ");
  const kind = p.osm_value && p.osm_value !== "yes" && p.type !== "house" ? human(p.osm_value) : "";
  return { label, detail: [kind, where].filter(Boolean).join(" · ") };
}
const MAX_STOPS = 8;
const endOf = (which) => (typeof which === "number" ? route.stops[which] : route[which]);
function placeField(which) {
  const isStop = typeof which === "number";
  const v = endOf(which);
  const row = el("div", `lm-end lm-end-${isStop ? "stop" : which}`);
  const id = `lm-end-list-${++fieldN}`;
  const box = el("div", "lm-end-box");
  const name = isStop ? T.stop : which === "from" ? T.from : T.to;
  const field = el("input", "lm-end-input", { type: "search", value: v ? v.label : "", placeholder: isStop ? T.stopHint : which === "from" ? T.fromHint : T.toHint,
    "aria-label": isStop ? `${T.stop} ${which + 1}` : name, role: "combobox", "aria-autocomplete": "list", "aria-expanded": "false", "aria-controls": id,
    autocomplete: "off", spellcheck: "false", enterkeyhint: "search", maxlength: "160" });
  const opts = el("ul", "lm-suggest lm-end-list", { id, role: "listbox" });
  opts.hidden = true;
  box.append(field, opts);
  row.append(el("span", "lm-end-dot", {}, isStop ? String(which + 1) : ""), el("span", "lm-end-name", {}, name), box);
  if (isStop) {
    const x = el("button", "lm-icon-btn lm-end-remove", { type: "button", title: T.removeStop, "aria-label": T.removeStop });
    x.innerHTML = svg(ICON.close, 16);
    x.addEventListener("click", () => {
      route.stops.splice(which, 1);
      renderRoute();
      findRoute();
    });
    row.append(x);
  }
  let items = [], active = -1, timer = 0, ctl = null;
  const close = () => { opts.hidden = true; field.setAttribute("aria-expanded", "false"); active = -1; };
  const mark = (i) => {
    active = i;
    opts.querySelectorAll(".lm-opt").forEach((li, j) => li.setAttribute("aria-selected", String(j === i)));
    if (i >= 0) field.setAttribute("aria-activedescendant", `${id}-${i}`); else field.removeAttribute("aria-activedescendant");
  };
  const choose = async (i) => {
    const it = items[i];
    if (!it) return;
    close();
    let place = it;
    if (it.me) {
      const me = await locate();
      if (!me) { routeMsg.textContent = T.locBlocked; return; }
      place = { ll: state.me.at2, label: T.myPlace, me: true };
    }
    const end = { ll: place.ll, label: place.label, me: place.me };
    if (isStop) route.stops[which] = end; else route[which] = end;
    renderRoute();
    findRoute();
  };
  const show = (list) => {
    items = list;
    opts.innerHTML = "";
    if (!list.length) return close();
    list.forEach((it, i) => {
      const li = el("li", "lm-opt", { role: "option", id: `${id}-${i}`, "aria-selected": "false" });
      const ic = el("span", "lm-opt-icon");
      ic.innerHTML = svg(it.me ? ICON.locate : it.star ? ICON.star : ICON.pin, 16);
      const txt = el("span", "lm-opt-text");
      txt.append(el("span", "lm-opt-label", {}, it.label));
      if (it.detail) txt.append(el("span", "lm-opt-detail", {}, it.detail));
      li.append(ic, txt);
      li.addEventListener("pointerdown", (e) => { e.preventDefault(); swallowTap(); choose(i); }); // at once: on a phone the field loses its focus before a mousedown comes
      opts.append(li);
    });
    opts.hidden = false;
    field.setAttribute("aria-expanded", "true");
    mark(-1);
  };
  const meItem = [...(navigator.geolocation && !v?.me ? [{ me: true, label: T.myPlace }] : []),
    ...saved.stars.slice(0, 6).map((x) => ({ star: true, label: x.name, detail: x.detail, ll: x.ll }))];
  const ask = async () => {
    const q = field.value.trim();
    ctl?.abort();
    if (q.length < 2) return show(meItem);
    ctl = new AbortController();
    const c = map.getCenter();
    try {
      const r = await fetch(`${root.dataset.tiles}/v1/${root.dataset.key}/geocode/api?${new URLSearchParams({ q, limit: "6", lang: geoLang(),
        lat: c.lat.toFixed(4), lon: c.lng.toFixed(4) })}`, { signal: ctl.signal });
      const d = await r.json();
      if (!field.isConnected || field.value.trim() !== q) return; // typed on meanwhile, or gone
      const seen = new Set();
      show((d.features || []).map((f) => ({ ...placeLabel(f.properties || {}), ll: f.geometry.coordinates }))
        .filter((it) => it.label && !seen.has(`${it.label}\n${it.detail}`) && seen.add(`${it.label}\n${it.detail}`)));
    } catch {}
  };
  field.addEventListener("input", () => {
    clearTimeout(timer);
    if (field.value.trim().length >= 2) close(); // not "Your location" under the text being typed
    timer = setTimeout(ask, 250);
  });
  // a tap empties the field to type (the place stays as the hint): selecting its text would open the
  // copy and paste menu of mobile browsers
  const hint = field.placeholder;
  field.addEventListener("focus", () => {
    const now = endOf(which);
    if (now && field.value === now.label) {
      field.placeholder = now.label;
      field.value = "";
    }
    show(meItem);
  });
  field.addEventListener("blur", () => setTimeout(() => {
    close();
    field.placeholder = hint;
    const now = endOf(which);
    if (now && field.value !== now.label) field.value = now.label; // back to the place chosen
  }, 150));
  field.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" && !opts.hidden) { e.preventDefault(); mark(Math.min(items.length - 1, active + 1)); }
    else if (e.key === "ArrowUp" && !opts.hidden) { e.preventDefault(); mark(Math.max(-1, active - 1)); }
    else if (e.key === "Enter") { e.preventDefault(); if (!opts.hidden && items.length) choose(active >= 0 ? active : 0); }
    else if (e.key === "Escape") { close(); field.blur(); }
  });
  return row;
}
// quiet: the location moved under a route shown; the route stays until the new one is there (and
// stays if that fails), and the map keeps its view
async function findRoute({ quiet = false } = {}) {
  if (!route.from || !route.to) return;
  route.ctl?.abort();
  const ctl = route.ctl = new AbortController();
  const ll = (v) => `${v.ll[1].toFixed(5)},${v.ll[0].toFixed(5)}`;
  const clearRoute = () => {
    routeSum.textContent = "";
    routeSteps.innerHTML = "";
    stepsBox.hidden = true;
    routeView.querySelector(".lm-nav-go")?.remove();
    routeView.querySelector(".lm-elev")?.remove();
    routeView.querySelector(".lm-along")?.remove();
    alongDone();
    showOnRoute(null);
    route.answer = null;
    drawEnds(null);
  };
  if (!quiet) {
    routeMsg.textContent = T.routing;
    clearRoute();
  }
  let d;
  try {
    const stops = route.stops.filter(Boolean);
    const q = new URLSearchParams({ from: ll(route.from), to: ll(route.to), mode: route.mode, lang: LANG });
    for (const x of stops) q.append("via", ll(x));
    const r = await fetch(`${ROUTE}?${q}`, { signal: ctl.signal });
    d = await r.json().catch(() => ({}));
    if (ctl.signal.aborted) return;
    if (!r.ok) {
      if (!quiet) routeMsg.textContent = d.error?.message || T.routeFailed;
      return;
    }
  } catch (err) {
    if (err.name !== "AbortError" && !quiet) routeMsg.textContent = T.routeFailed;
    return;
  }
  if (quiet) {
    if (nav.on) return; // navigation started on the route before: it keeps that one
    clearRoute();
  }
  routeMsg.textContent = "";
  d.via = route.stops.filter(Boolean); // the stops of this answer, in the order of d.stops
  route.answer = d;
  // the time (coloured by the traffic on the way), the distance, the arrival if leaving now
  routeSum.replaceChildren(el("span", `lm-sum-time ${trafficClass(d)}`, {}, T.duration(d.s)), el("span", "lm-sum-km", {}, fmtKm(d.km)));
  if (d.delay >= 60) routeSum.append(el("span", `lm-delay ${trafficClass(d)}`, {}, T.delay(Math.round(d.delay / 60))));
  routeSum.append(el("span", "lm-sum-eta", {}, `${T.arrival} ${clock(d.s)}`));
  const go = el("div", "lm-nav-go");
  const startBtn = el("button", "lm-nav-start", { type: "button" });
  startBtn.innerHTML = svg(ICON.navigate, 20);
  startBtn.append(T.start);
  startBtn.addEventListener("click", () => navStart(false));
  const simBtn = el("button", "lm-chip", { type: "button", title: T.previewNote }, T.preview);
  simBtn.addEventListener("click", () => navStart(true));
  go.append(startBtn, simBtn, shareButton(() => ({ title: `${route.from?.label || ""} → ${route.to?.label || ""}`, url: routeLink() }), true));
  routeSum.after(go);
  // the time and Start in view: on a phone the sheet shows only its top
  if (!quiet) requestAnimationFrame(() => {
    const over = go.getBoundingClientRect().bottom + 12 - innerHeight;
    const room = routeSum.getBoundingClientRect().top - body.getBoundingClientRect().top - 8;
    if (over > 0 && room > 0) body.scrollTop += Math.min(over, room);
  });
  if (d.elev?.pts?.length > 1) go.after(heightChart(d.elev, { onPoint: showOnRoute }).node);
  if (ALONG) stepsBox.before(alongBox(d));
  loadAvatar();
  if (SAY) fetchSay(d.steps[0]?.pre || d.steps[0]?.text); // the first sentence, ready for Start
  for (const st of d.steps) {
    const li = el("li", `lm-step${st.stop ? " lm-step-stop" : ""}`);
    const b = el("button", "", { type: "button" });
    b.append(el("span", "lm-step-text", {}, st.stop ? `${T.stop} ${st.stop}: ${d.via[st.stop - 1]?.label || ""}` : st.text));
    if (st.km > 0) b.append(el("span", "lm-step-km", {}, fmtKm(st.km)));
    b.addEventListener("click", () => fit(() => map.easeTo({ center: st.at, zoom: Math.max(map.getZoom(), 16), duration: 600, padding: pad() })));
    li.append(b);
    routeSteps.append(li);
  }
  stepsTitle.textContent = `${T.turnByTurn} (${d.steps.length})`;
  stepsBox.hidden = !d.steps.length;
  drawEnds(d.line);
  drawRouteTraffic(d);
  if (route.go && route.from?.me) {
    route.go = false;
    return navStart(false);
  }
  route.go = false;
  const b = extent({ type: "LineString", coordinates: d.line });
  if (b && !quiet) fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: 16, duration: 700 }));
}

// ------------------------------------------------------------------ stops and places along the route
// A stop found on the map, in a field or along the route goes between the stops where it falls on
// the route shown; without a route, last.
function insertStop(p) {
  route.stops = route.stops.filter(Boolean);
  if (route.stops.length >= MAX_STOPS) return;
  const d = route.answer;
  if (!d?.line?.length) return void route.stops.push(p);
  const cum = cumOf(d.line), at = (ll) => projectOn(d.line, cum, ll);
  const a = at(p.ll), i = route.stops.findIndex((x) => at(x.ll) > a);
  route.stops.splice(i < 0 ? route.stops.length : i, 0, p);
}
// how far along a line (metres, cum of cumOf) the point nearest to p is
function projectOn(line, cum, p) {
  const P = flat(p, p[1]);
  let best = Infinity, along = 0;
  for (let i = 0; i + 1 < line.length; i++) {
    const A = flat(line[i], p[1]), B = flat(line[i + 1], p[1]);
    const dx = B[0] - A[0], dy = B[1] - A[1], len2 = dx * dx + dy * dy;
    const t = len2 ? Math.max(0, Math.min(1, ((P[0] - A[0]) * dx + (P[1] - A[1]) * dy) / len2)) : 0;
    const dist = Math.hypot(A[0] + t * dx - P[0], A[1] + t * dy - P[1]);
    if (dist < best) { best = dist; along = cum[i] + (cum[i + 1] - cum[i]) * t; }
  }
  return along;
}
// Search along the route: the kinds of places of each mode, near the line (data-along). The answers
// of a route are kept with it; the kind chosen stays for the next route (a stop added).
const ALONG_CATS = {
  car: ["fuel", "charging", "rest", "food", "cafe", "groceries", "toilets", "hotel"],
  bike: ["water", "food", "cafe", "groceries", "toilets", "bike", "hotel"],
  walk: ["water", "food", "cafe", "toilets", "groceries", "pharmacy"],
};
const along = { ctl: null };
function alongDone() {
  along.ctl?.abort();
  setSource("along", empty);
}
async function alongFetch(d, cat) {
  d.along ||= {};
  if (d.along[cat]) return d.along[cat];
  along.ctl?.abort();
  const ctl = along.ctl = new AbortController();
  // the line, a point every 15 m at most: a long route has tens of thousands
  const line = [];
  let last = null;
  for (const p of d.line) {
    if (last && km(last, p) < 0.015) continue;
    line.push([+p[0].toFixed(5), +p[1].toFixed(5)]);
    last = p;
  }
  const end = d.line[d.line.length - 1];
  if (last !== end) line.push([+end[0].toFixed(5), +end[1].toFixed(5)]);
  const r = await fetch(ALONG, { method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ line, cat, mode: route.mode }), signal: ctl.signal });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error?.message || T.failed);
  const fs = j.features || [];
  fs.total = j.total || fs.length;
  for (const f of fs) state.byId.set(f.properties.id, f);
  return (d.along[cat] = fs);
}
const alongName = (f) => f.properties.name || kindOf(f.properties).label || T.noName;
// a place along the route: its name, kind, where on the route, hours; a button adds it as a stop
function alongItem(f, { ahead = null, onAdd } = {}) {
  const p = f.properties, t = tagsOf(p), k = kindOf(p);
  const li = el("li", "lm-along-item");
  const b = el("button", "lm-card", { type: "button", "data-id": p.id });
  const text = el("span", "lm-card-text");
  text.append(el("span", "lm-card-name", {}, alongName(f)));
  const sub = [p.name ? k.label : "", t.brand && t.brand !== p.name ? t.brand : ""].filter(Boolean).join(" · ");
  if (sub) text.append(el("span", "lm-card-sub", {}, sub));
  const at = ahead == null ? p.at : Math.max(0, p.at - ahead);
  const where = [at >= 0.1 ? T.alongAfter(fmtKm(at)) : "", p.off >= 50 ? T.alongOff(fmtKm(p.off / 1000)) : ""].filter(Boolean).join(" · ");
  if (where) text.append(el("span", "lm-card-addr", {}, where));
  if (p.opening_hours) text.append(el("span", "lm-card-hours", { "data-hours": "" }));
  b.append(text);
  b.addEventListener("click", () => {
    setSource("selected", one(f));
    if (nav.on) { nav.follow = false; navCenter.hidden = false; }
    fit(() => map.easeTo({ center: f.geometry.coordinates, zoom: Math.max(map.getZoom(), 15), duration: 600, padding: nav.on ? navPad() : pad() }));
  });
  b.addEventListener("mouseenter", () => setSource("hover", one(f)));
  b.addEventListener("mouseleave", () => setSource("hover", empty));
  const add = el("button", "lm-icon-btn lm-along-add", { type: "button", title: T.useAsStop, "aria-label": `${T.useAsStop}: ${alongName(f)}` });
  add.innerHTML = svg(ICON.plus, 20);
  add.addEventListener("click", () => onAdd({ ll: f.geometry.coordinates, label: alongName(f) }));
  li.append(b, add);
  return li;
}
// the chips of the kinds, the state and the list, for a route answer
function alongPanel(d, { ahead = () => null, onAdd, chosen = "", onChoose = () => {} }) {
  const box = el("div", "lm-along-body");
  const cats = el("div", "lm-chips lm-along-cats", { role: "group", "aria-label": T.along });
  const msg = el("p", "lm-notes lm-along-msg", { role: "status", "aria-live": "polite" });
  const ol = el("ol", "lm-results lm-along-list");
  let cur = "";
  const choose = async (cat) => {
    cur = cur === cat ? "" : cat;
    onChoose(cur);
    for (const c of cats.children) c.setAttribute("aria-pressed", String(c.dataset.cat === cur));
    ol.innerHTML = "";
    msg.textContent = "";
    setSource("along", empty);
    if (!cur) return along.ctl?.abort();
    msg.textContent = T.alongSearching;
    let fs;
    try { fs = await alongFetch(d, cur); } catch (err) {
      if (err.name !== "AbortError" && cur === cat) msg.textContent = err.message || T.failed;
      return;
    }
    if (cur !== cat || !box.isConnected) return;
    const from = ahead();
    const shown = from == null ? fs : fs.filter((f) => f.properties.at > from);
    setSource("along", { type: "FeatureCollection", features: shown });
    msg.textContent = !shown.length ? T.alongNone : fs.total > fs.length && from == null ? T.alongMore(fs.length, fs.total) : "";
    for (const f of shown.slice(0, from == null ? 200 : 30)) ol.append(alongItem(f, { ahead: from, onAdd }));
    if (shown.some((f) => f.properties.opening_hours)) loadHours().then(() => markHours(ol));
  };
  for (const cat of ALONG_CATS[route.mode] || ALONG_CATS.car) {
    const c = el("button", "lm-chip", { type: "button", "aria-pressed": "false", "data-cat": cat }, T.alongCats[cat]);
    c.addEventListener("click", () => choose(cat));
    cats.append(c);
  }
  box.append(cats, msg, ol);
  if (chosen) choose(chosen);
  return box;
}
// in directions, under the summary
function alongBox(d) {
  const sec = el("section", "lm-along");
  sec.append(el("h3", "lm-along-title", {}, T.along));
  sec.append(alongPanel(d, {
    chosen: route.alongCat,
    onChoose: (cat) => { route.alongCat = cat; },
    onAdd: (p) => {
      insertStop(p);
      renderRoute();
      findRoute();
    },
  }));
  return sec;
}
// a tap on a place along the route: its line in the list, in view
function alongAt(pt) {
  if (!map.getLayer("lm-along")) return false;
  const r = 10, f = map.queryRenderedFeatures([[pt.x - r, pt.y - r], [pt.x + r, pt.y + r]], { layers: ["lm-along"] })[0];
  if (!f) return false;
  const scope = nav.on ? navAlong : routeView;
  const b = scope.querySelector(`.lm-along-item [data-id="${CSS.escape(f.properties.id)}"]`);
  if (b) {
    scope.querySelectorAll(".lm-along-item .lm-hot").forEach((x) => x.classList.remove("lm-hot"));
    b.classList.add("lm-hot");
    b.scrollIntoView({ block: "nearest", behavior: "smooth" });
    const full = state.byId.get(f.properties.id);
    if (full) setSource("selected", one(full));
  }
  return true;
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
// how far off the line is off the route, in metres: off: metres from the line that count as off the
// route, plus the accuracy of the fix up to acc; fixes: that many fixes off in a row bring a new route;
// away: off sooner (at half the distance) when the traveller moves at more than 2 m/s at 60 degrees or
// more from the way of the route (a turn missed); reach: this near the destination or a stop counts as
// there (arrived). v: the speed in m/s when none is known yet.
// A turn is said twice: ahead with how far ("In 150 metres, turn right onto X"), and at the turn
// (pre). The turn is said `pre` seconds before it, between preMin and preMax metres. Ahead: at `ahead`
// metres, or `aheadS` seconds before at the speed, whichever comes first, up to aheadMax; by car the
// distance goes by the speed (town, country road, motorway). Not below aheadMin metres: two turns
// close together have no "in x metres" for the second (the first says "Then …").
const NAV = {
  car: { off: 45, acc: 60, fixes: 3, away: true, arrive: 40, reach: 50, v: 14, pre: 6, preMin: 60, preMax: 200,
    ahead: (v) => (v < 15 ? 200 : v < 24 ? 500 : 1000), aheadS: 0, aheadMax: 1000, aheadMin: 100 },
  bike: { off: 25, acc: 25, fixes: 2, away: true, arrive: 20, reach: 30, v: 5, pre: 5, preMin: 20, preMax: 30,
    ahead: () => 150, aheadS: 12, aheadMax: 300, aheadMin: 50 },
  walk: { off: 25, acc: 20, fixes: 3, away: false, arrive: 15, reach: 25, v: 1.4, pre: 7, preMin: 10, preMax: 15,
    ahead: () => 60, aheadS: 30, aheadMax: 100, aheadMin: 30 },
};
// "In 150 metres, " in the language of the steps; the instruction of Valhalla follows (in German after
// a colon, with its capital)
const SAY_IN = {
  en: (d, km) => `In ${d} ${km ? (d === "1" ? "kilometer" : "kilometers") : "meters"}, `,
  de: (d, km) => `In ${d} ${km ? "Kilometern" : "Metern"}: `.replace(/^In 1 Kilometern/, "In einem Kilometer"),
  fr: (d, km) => `Dans ${d} ${km ? (d === "1" ? "kilomètre" : "kilomètres") : "mètres"}, `,
  it: (d, km) => `Tra ${d} ${km ? (d === "1" ? "chilometro" : "chilometri") : "metri"}, `,
  es: (d, km) => `En ${d} ${km ? (d === "1" ? "kilómetro" : "kilómetros") : "metros"}, `,
  nl: (d, km) => `Over ${d} ${km ? "kilometer" : "meter"}, `,
  pt: (d, km) => `Em ${d} ${km ? (d === "1" ? "quilómetro" : "quilómetros") : "metros"}, `,
};
// a distance in metres rounded down to one that is said: 10s up to 100 m, 50s up to 300 m, 100s up
// to 1 km, then half kilometres
const sayRound = (m) => (m < 100 ? Math.floor(m / 10) * 10 : m < 300 ? Math.floor(m / 50) * 50 : m < 1000 ? Math.floor(m / 100) * 100 : Math.floor(m / 500) * 500);
// the speed for the timing of the sentences: of the device (Preview drives three times as fast), else
// that of the step by Valhalla by car, else of the mode
function sayV(st) {
  const cfg = NAV[route.mode];
  const ms = nav.on ? (nav.sim ? (nav.speed || 0) / 3 : nav.speed) : null;
  const plan = st?.s > 0 ? (st.km * 1000) / st.s : 0;
  if (route.mode === "car") return Math.max(ms || 0, plan || cfg.v);
  return ms > 1 ? ms : cfg.v;
}
// the turn of step n said ahead: at metres before it, and the sentence ({at, text}); null when there
// is no room for it after the turn before (or nothing to say: straight on). v: the speed in m/s; by
// car the speed of the road by Valhalla, so that the sentence is fetched before it is needed.
function aheadOf(steps, n, v) {
  const st = steps[n], prev = steps[n - 1], cfg = NAV[route.mode];
  if (!st || !prev || !st.alert || [7, 8, 22].includes(st.type)) return null;
  if (route.mode === "car") v = prev.s > 0 ? (prev.km * 1000) / prev.s : cfg.v;
  const gap = prev.km * 1000, pre = preAt(v);
  const want = Math.min(cfg.aheadMax, Math.max(cfg.ahead(v), v * cfg.aheadS));
  // room after the turn before: its instruction has been said (some 2 s)
  const at = sayRound(Math.min(want, gap - Math.max(10, v * 2)));
  if (at < cfg.aheadMin || at < pre * 2) return null;
  const km = at >= 1000, d = km ? String(at / 1000).replace(".", STEP_LANG === "en" ? "." : ",") : String(at);
  const lead = (SAY_IN[STEP_LANG] || SAY_IN.en)(d, km);
  const text = lead + (STEP_LANG === "de" ? st.alert : st.alert.replace(/^\p{Lu}(?=\p{Ll})/u, (c) => c.toLowerCase()));
  return { at, text };
}
// how near the turn its instruction is said, in metres
function preAt(v) {
  const cfg = NAV[route.mode];
  return Math.min(cfg.preMax, Math.max(cfg.preMin, v * cfg.pre));
}
// flat: false, navigation follows tilted (its 2D/3D button is gone)
const nav = { on: false, sim: false, follow: true, muted: store.get("hm-mute") === "1", flat: false, spoken: new Set(), off: 0, rerouting: false };

// During navigation the camera eases from fix to fix and never stops, and the traveller asks for a
// frame every frame: on a 90 or 120 Hz screen the map would draw 90 or 120 times a second. Capped
// at 30: every repaint MapLibre asks for (camera, traveller, tiles, gestures) waits for our own
// animation frame and draws there with map.redraw() once 1/30 s has passed since the last draw.
// The camera and the traveller place themselves by the clock, so a skipped frame does not slow them.
const fpsCap = (() => {
  const own = map.triggerRepaint; // Map.prototype.triggerRepaint
  const gap = 1000 / 30;
  let on = false, pending = 0, next = 0;
  const tick = (ts) => {
    pending = 0;
    if (!on) return own.call(map);
    if (ts < next - 4) { pending = requestAnimationFrame(tick); return; } // too soon: the next frame
    next = (ts - next < gap ? next : ts) + gap; // 30 a second on average, also at 90 or 144 Hz; restart after a pause
    map.redraw();
  };
  return (yes) => {
    if (yes === on) return;
    on = yes;
    if (on) map.triggerRepaint = () => { if (!pending) pending = requestAnimationFrame(tick); };
    else {
      delete map.triggerRepaint; // back to the prototype
      if (pending) { cancelAnimationFrame(pending); pending = 0; }
      map.triggerRepaint();
    }
  };
})();

const navBox = el("div", "lm-nav", { "aria-live": "polite" });
navBox.hidden = true;
const navBanner = el("div", "lm-nav-banner");
const navSign = el("div", "lm-nav-sign");
const navThen = el("div", "lm-nav-then");
const navLanes = el("div", "lm-nav-lanes", { "aria-hidden": "true" }); // the lanes before a turn, by car
navLanes.hidden = true;
// the arrows of a lane (bits of Valhalla) as arrows of ARROW, from left to right
const LANE_ARROWS = [[256, "uturnLeft"], [4, "sharpLeft"], [8, "left"], [16, "slightLeft"], [512, "slightLeft"], [2, "straight"],
  [1024, "slightRight"], [32, "slightRight"], [64, "right"], [128, "sharpRight"]];
let shownLanes = null;
function renderLanes(st, toNext) {
  const lanes = route.mode === "car" && toNext < 1200 ? st.lanes : null;
  navLanes.hidden = !lanes?.length;
  if (navLanes.hidden || lanes === shownLanes) return; // the lanes shown
  shownLanes = lanes;
  navLanes.innerHTML = "";
  for (const l of lanes) {
    const take = l.a || l.v || 0;
    const box = el("div", `lm-lane${take ? " lm-lane-take" : ""}`);
    let any = false;
    for (const [bit, arrow] of LANE_ARROWS) {
      if (!(l.d & bit)) continue;
      any = true;
      const a = el("span", `lm-lane-arrow${take & bit ? " lm-lane-on" : ""}`);
      a.innerHTML = `<svg viewBox="0 0 24 24" width="26" height="26"><path d="${ARROW[arrow]}" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
      box.append(a);
    }
    if (!any) box.append(el("span", "lm-lane-arrow", {}, "·"));
    navLanes.append(box);
  }
}
const navLimit = el("div", "lm-nav-limit");
const navSpeed = el("div", "lm-nav-speed"); // the speed now, by car and by bike
navSpeed.hidden = true;
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
const navAlongBtn = navBtn(ICON.search, T.along);
navAlongBtn.hidden = !ALONG;
navBottom.append(navMute, navEta, navAlongBtn, navOverview, navEnd);
// search along the route in navigation: the kinds, and the next places ahead
const navAlong = el("div", "lm-nav-along");
navAlong.hidden = true;

navBox.append(navBanner, navLanes, navSign, navThen, navLimit, navSpeed, navElev, navCenter, navAlong, navBottom);
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
  nav.stops = (d.stops || []).map((i, j) => ({ i, ...(d.via?.[j] || {}) })); // the stops ahead, by index of the line
  nav.simAlong = 0;
  if (navAlong.dataset.kind === "along") navAlongOpen(); // the places along the new line
  navElev.innerHTML = "";
  nav.elev = d.elev?.pts?.length > 1 ? heightChart(d.elev, { compact: true }) : null;
  navElev.hidden = !nav.elev;
  root.classList.toggle("lm-nav-elev-on", !!nav.elev);
  if (nav.elev) navElev.append(nav.elev.node);
  nav.idx = 0;
  nav.spoken = new Set();
  nav.off = 0;
  nav.endMin = Infinity; // the nearest the traveller came to the destination, in metres
  nav.endAway = 0;
  nav.endTrail = [];
  nav.slowT = null;
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
let STEP_LANG = LOCALES[LANG] ? LANG : "en"; // the language of the texts of Valhalla
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
// it resolves when the browser has said the text (or gave up)
function browserSay(text, lang) {
  if (!hasSpeech) return Promise.resolve();
  const u = new SpeechSynthesisUtterance(text);
  u.lang = LOCALES[lang] || "en-US";
  const v = bestVoice(lang);
  if (v) u.voice = v;
  speechSynthesis.cancel();
  return new Promise((done) => {
    u.onend = u.onerror = () => done();
    setTimeout(done, 15e3); // some browsers never end an utterance
    speechSynthesis.speak(u);
  });
}
const player = new Audio();
// MP3, or WAV where the browser plays no MP3 (Firefox on Linux without the codecs of the system)
let sayFormat = player.canPlayType("audio/mpeg") ? "mp3" : "wav";
const sayCache = new Map(); // lang and text → a promise of a blob URL, or null
const sayDone = new Map(); // the same, once there: the blob URL, or null
// the sentences of the route being followed (preSayAll), kept however many: they are still to be said
let sayKeep = new Set();
const SAY_MAX = 150; // sentences kept besides, the oldest go (and their sound with them)
function fetchSay(text, lang = STEP_LANG) {
  if (!SAY || !text || !OUR_VOICES.includes(lang)) return null;
  const key = `${sayFormat}\n${lang}\n${text}`;
  if (!sayCache.has(key)) {
    const p = fetch(`${SAY}?${new URLSearchParams({ lang, text, format: sayFormat })}`)
      .then((r) => (r.ok ? r.blob() : null)).then((b) => (b ? URL.createObjectURL(b) : null)).catch(() => null);
    p.then((url) => { if (sayCache.get(key) === p) sayDone.set(key, url); else if (url) URL.revokeObjectURL(url); });
    sayCache.set(key, p);
    sayTrim();
  }
  return sayCache.get(key);
}
// sayTrim drops the oldest sentences beyond SAY_MAX: not one on its way, nor one of the route, nor the one playing
function sayTrim() {
  let over = sayCache.size - SAY_MAX;
  for (const key of sayCache.keys()) {
    if (over <= 0) return;
    if (!sayDone.has(key)) continue;
    const url = sayDone.get(key);
    if (key.startsWith(`${sayFormat}\n`) && sayKeep.has(key.slice(key.indexOf("\n", sayFormat.length + 1) + 1))) continue;
    if (url && player.src === url) continue;
    sayCache.delete(key);
    sayDone.delete(key);
    if (url) URL.revokeObjectURL(url);
    over--;
  }
}
// the sentences of the next turns, before they are needed
function preSay(steps, all = nav.d?.steps || route.answer?.steps || []) {
  for (const st of steps) if (st) for (const t of sentencesOf(all, st)) fetchSay(t);
}
// the sentences of a step: said ahead (at the speed now), at the turn, after it
function sentencesOf(all, st) {
  const n = all.indexOf(st);
  return [n > 0 && aheadOf(all, n, sayV(all[n - 1]))?.text, st.pre || st.text, st.post].filter(Boolean);
}
// all the sentences of a route, in its order, two at a time: a new sentence may take the voice of
// the server several seconds, so they are there before the turns come (the next turns go first,
// through preSay)
let sayQueue = [], sayQueueRunning = 0;
function preSayAll(steps) {
  sayQueue = steps.flatMap((st) => sentencesOf(steps, st));
  sayKeep = new Set([...sayQueue, T.gpsLost, T.rerouted]);
  const next = () => {
    while (sayQueueRunning < 2 && sayQueue.length) {
      const p = fetchSay(sayQueue.shift());
      if (!p) continue;
      sayQueueRunning++;
      p.finally(() => { sayQueueRunning--; next(); });
    }
  };
  next();
}
// the sound of the player has ended: played to its end, paused (a new sentence, Sound off) or failed
const played = () => new Promise((done) => {
  const end = () => { for (const e of ["ended", "pause", "error"]) player.removeEventListener(e, end); done(); };
  for (const e of ["ended", "pause", "error"]) player.addEventListener(e, end);
});
let sayN = 0, sayChain = Promise.resolve();
// one sentence, to its end, unless a newer one (sayN) takes the voice
async function sayOne(text, lang, n) {
  const p = fetchSay(text, lang);
  if (p) {
    // late: the voice of the browser after 2.5 s, if it has one for the language; else the server's up to 8 s
    const wait = hasSpeech && bestVoice(lang) ? 2500 : 8000;
    const url = await Promise.race([p, new Promise((done) => setTimeout(() => done(null), wait))]);
    if (n !== sayN || nav.muted) return;
    if (url) {
      player.src = url;
      try {
        await player.play();
        return played();
      } catch (err) {
        // it could not play this sound: WAV from now on, and this sentence again in it
        if (err?.name === "NotSupportedError" && sayFormat === "mp3") {
          sayFormat = "wav";
          const wav = await Promise.race([fetchSay(text, lang), new Promise((done) => setTimeout(() => done(null), 2500))]);
          if (n !== sayN || nav.muted) return;
          if (wav) {
            player.src = wav;
            try {
              await player.play();
              return played();
            } catch {}
          }
        }
      }
    }
  }
  if (n === sayN && !nav.muted) return browserSay(text, lang);
}
// speak says sentences ([text, lang] or text in the language of the steps) one after the other. It
// stops what is being said, unless after: then they come when it is done (Continue for 2 km after the
// turn, not over it).
function speak(sentences, { after = false } = {}) {
  if (nav.muted) return;
  const list = sentences.map((x) => (Array.isArray(x) ? x : [x, STEP_LANG])).filter(([t]) => t);
  if (!list.length) return;
  if (!after) hush();
  const n = sayN;
  const run = async () => {
    for (const [t, l] of list) {
      if (n !== sayN || nav.muted) return;
      await sayOne(t, l, n);
    }
  };
  sayChain = after ? sayChain.then(run) : run();
  sayChain = sayChain.catch(() => {});
}
const say1 = (text, lang = STEP_LANG) => speak([[text, lang]]);
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
let shownSign = null;
function renderSign(st, toNext) {
  const show = route.mode === "car" && toNext < 2000 && (st.exit || st.toward?.length || st.routes?.length || st.exit_name);
  navSign.hidden = !show;
  if (!show || st === shownSign) return; // the sign shown
  shownSign = st;
  navSign.innerHTML = "";
  navSign.className = `lm-nav-sign ${st.highway || st.exit ? "lm-sign-motorway" : "lm-sign-road"}`;
  const top = el("div", "lm-sign-top");
  if (st.exit) top.append(el("span", "lm-sign-exit", {}, `${T.exit} ${st.exit}`));
  for (const r of (st.routes || []).slice(0, 3)) top.append(el("span", shieldClass(r), {}, r));
  if (top.children.length) navSign.append(top);
  if (st.exit_name) navSign.append(el("div", "lm-sign-name", {}, st.exit_name));
  for (const t of (st.toward || []).slice(0, 3)) navSign.append(el("div", "lm-sign-to", {}, t));
}
// by car: dark at night where the car is, and in a tunnel of 200 m or more; light otherwise
function navDayNight(p, at) {
  const want = route.mode !== "car" ? null
    : isNight(at[0], at[1]) || (nav.d.tunnels || []).some(([a, b, m]) => m >= 200 && p.i >= a && p.i < b) ? "driving-night" : "driving";
  if (want === navLook) return;
  navLook = want;
  document.documentElement.dataset.theme = theme();
  restyle();
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
  // the banner: the turn, how far, onto what; made anew for a new turn (or after a message in it), else
  // only the distance changes
  // the street to turn into; a bare route number ("1") says less than the instruction
  const instr = next.roundabout ? next.text : next.stop ? `${T.stop}: ${d.via?.[next.stop - 1]?.label || next.text}`
    : next.street && !/^[\d\s/]+$/.test(next.street) && next.type !== 4 ? next.street : next.text;
  const shown = nav.banner;
  if (shown?.type === next.type && shown.instr.textContent === instr && shown.dist.isConnected) {
    setText(shown.dist, fmtKm(toNext / 1000));
  } else {
    navBanner.innerHTML = "";
    const arrow = el("div", "lm-nav-arrow");
    arrow.innerHTML = arrowSVG(next.type, 44);
    const txt = el("div", "lm-nav-text");
    const dist = el("div", "lm-nav-dist", {}, fmtKm(toNext / 1000));
    txt.append(dist, el("div", "lm-nav-instr", {}, instr));
    navBanner.append(arrow, txt);
    nav.banner = { type: next.type, instr: txt.lastChild, dist };
  }
  renderSign(next, toNext);
  renderLanes(next, toNext);
  // the turn after, when it comes soon after this one
  const after = steps[k + 2];
  navThen.hidden = !(after && toNext < 1000 && nav.cum[after.i] - nav.cum[next.i] < 250);
  if (!navThen.hidden && navThen.dataset.shown !== `${after.type} ${T.then}`) {
    navThen.innerHTML = "";
    navThen.dataset.shown = `${after.type} ${T.then}`;
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
  // the speed now: of the device, or of Preview (which drives three times as fast)
  const ms = nav.sim ? (nav.speed || 0) / 3 : nav.speed;
  navSpeed.hidden = route.mode === "walk" || ms == null || nav.arrived;
  if (!navSpeed.hidden) {
    const kmh = Math.round(ms * 3.6);
    navSpeed.replaceChildren(el("strong", "", {}, String(kmh)), el("span", "", {}, "km/h"));
    navSpeed.classList.toggle("lm-over", !!lim && kmh > lim[2] + 3);
    navSpeed.title = navSpeed.ariaLabel = T.speedNow(kmh);
  }
  if (!navEta.firstChild) navEta.append(el("strong"), el("span"));
  if (navEta.firstChild.className !== trafficClass(d, p.i)) navEta.firstChild.className = trafficClass(d, p.i);
  setText(navEta.firstChild, clock(s));
  setText(navEta.lastChild, `${T.duration(s)} · ${fmtKm(left / 1000)}`);
  // the spoken instructions: early, then at the turn; after the turn what comes
  const n = steps.indexOf(next);
  preSay([cur, next, steps[k + 2], steps[k + 3]]);
  // ahead: once, when the turn is about that far ("In 150 metres, …"), not when it is already much
  // nearer (a fix after a gap); at the turn: once, a few seconds before it
  const v = sayV(cur), pre = preAt(v), ahead = n > k ? aheadOf(steps, n, v) : null;
  if (reachStop(p, rawPos)) {
    // "You have reached the stop", then the way on
  } else if (k === 0 && !nav.spoken.has("0")) {
    nav.spoken.add("0");
    say1(steps[0].pre || steps[0].text);
  } else if (toNext <= pre && !nav.spoken.has(`${n}p`)) {
    // an arrival (a stop, the destination) is said when the traveller is there (reachStop, navArrive):
    // before it "You will arrive", unless said already (ahead)
    const arrival = next.type >= 4 && next.type <= 6, alerted = nav.spoken.has(`${n}a`);
    nav.spoken.add(`${n}p`).add(`${n}a`);
    if (!arrival) say1(next.pre || next.text);
    else if (!alerted && next.alert) say1(next.alert);
  } else if (ahead && toNext <= ahead.at + Math.max(3, v) && !nav.spoken.has(`${n}a`)) {
    nav.spoken.add(`${n}a`);
    if (toNext >= ahead.at * 0.7 && toNext > pre + v * 3) say1(ahead.text);
  } else if (cur.post && !nav.spoken.has(`${k}post`) && p.along - nav.cum[cur.i] >= cfg.arrive && toNext > (ahead?.at || pre) + v * 6 + 20) {
    // after a turn, with the next one far: how far the way goes on ("Continue for 2 kilometres"), when
    // the instruction of the turn is said
    nav.spoken.add(`${k}post`);
    speak([cur.post], { after: true });
  }
  // the camera and the sprite
  // off the route: the real position, and the heading of the device when it has one
  const at = off ? rawPos : p.ll, heading = off && rawPos.heading != null ? rawPos.heading : p.heading;
  nav.avatar?.set(at, heading, nav.sim ? 480 : 900, nav.speed || 0);
  nav.at = at;
  nav.heading = heading;
  nav.along = p.along;
  navDayNight(p, at);
  if (nav.elev) nav.elev.mark((p.along / (total || 1)) * nav.elev.km);
  if (nav.follow) {
    map.easeTo({ center: at, bearing: heading, pitch: nav.flat ? 0 : NAV_PITCH, zoom: navZoom(at), padding: navPad(), duration: nav.sim ? 480 : 900, easing: (t) => t });
  }
  drawNavLine(p);
  if (nav.sim && left <= cfg.arrive) navArrive(); // with real fixes onFix decides (arrivedNow)
}
// distance in metres between two points near each other
const metres = (a, b) => {
  const A = flat(a, a[1]), B = flat(b, a[1]);
  return Math.hypot(A[0] - B[0], A[1] - B[1]);
};
// A stop reached: within reach of it, or at it along the line; said once ("You have reached the
// stop", then the way on), and no new route back to it. ll: the fix, null in Preview.
function reachStop(p, ll) {
  const cfg = NAV[route.mode], j = nav.stops.findIndex((x) => !x.done && x.i >= nav.idx - 3);
  const x = nav.stops[j];
  if (!x) return false;
  const ahead = nav.cum[x.i] - p.along;
  if (ahead > 3 * cfg.reach) return false;
  const near = ahead <= (nav.sim ? cfg.arrive : cfg.reach) || (ll && Math.min(metres(ll, nav.d.line[x.i]), x.ll ? metres(ll, x.ll) : Infinity) <= cfg.reach);
  if (!near) return false;
  x.done = true;
  const steps = nav.d.steps, k = steps.findIndex((st) => st.stop === j + 1), on = k >= 0 ? steps[k + 1] : null;
  if (on) nav.spoken.add(`${k}p`).add(`${k}a`).add(`${k + 1}p`).add(`${k + 1}a`);
  speak([[T.stopReached, UI], on && (on.pre || on.text)]);
  return true;
}
// Arrived, with real fixes (5 to 20 m off, one a second): the traveller stops a little before the end,
// rides past it, stands on the other side of the street, or goes to a destination off the road (the
// line ends on the road nearest to it). Within reach of the end of the line or of the destination,
// or at the end of the line; or near (1.5 times reach) and stopped for 5 s, or (twice reach) going away again for two
// fixes. Only on the last stretch of the route: a round trip starts where it ends.
function arrivedNow(p, ll, pos, acc, onLine) {
  const cfg = NAV[route.mode], total = nav.cum[nav.cum.length - 1], left = Math.max(0, total - p.along);
  if (left > 3 * cfg.reach + acc) {
    nav.endMin = Infinity;
    nav.endTrail = [];
    return false;
  }
  const d = Math.min(metres(ll, nav.d.line[nav.d.line.length - 1]), route.to?.ll ? metres(ll, route.to.ll) : Infinity, onLine ? left : Infinity);
  if (d <= cfg.reach) return true;
  const t = pos.timestamp || Date.now();
  nav.endMin = Math.min(nav.endMin, d);
  nav.endAway = nav.endMin <= 2 * cfg.reach + acc / 2 && d >= nav.endMin + Math.max(15, acc) ? nav.endAway + 1 : 0;
  if (nav.endAway >= 2) return true;
  // stopped: the speed of the device, else the fixes of the last 5 s within a few metres
  nav.endTrail = nav.endTrail.filter((f) => t - f.t <= 6000).concat({ t, ll });
  if (d > 1.5 * cfg.reach + acc / 2) return (nav.slowT = null, false);
  const sp = pos.coords.speed, oldest = nav.endTrail[0];
  if (sp != null && !Number.isNaN(sp)) {
    if (sp >= 1) nav.slowT = null;
    else if (nav.slowT == null) nav.slowT = t;
    return nav.slowT != null && t - nav.slowT >= 5000;
  }
  return t - oldest.t >= 5000 && nav.endTrail.every((f) => metres(f.ll, oldest.ll) <= Math.max(10, acc));
}
// The zoom of navigation follows the speed: the map shows the next 25 seconds of the way ahead of
// the traveller (a common rule of navigation apps), 120 m at least (on foot) and 1 km at most (fast
// on a motorway), and it changes gently. Ahead is the map above the traveller, which sits low; tilted
// at 60 degrees the map shows about twice the ground of a flat one there. By bike and on foot the
// map is a zoom closer (the metres of a pixel are of tiles of 512 px there, 256 by car): streets and
// paths, not the town.
// the height of the map, read once after each resize: a read each second makes the browser lay out the page
let mapHeight = 0;
map.on("resize", () => { mapHeight = 0; });
const mapH = () => mapHeight || (mapHeight = mapEl.clientHeight);
function navZoom(at) {
  const ms = Math.max(0, nav.sim ? (nav.speed || 0) / 3 : nav.speed || 0);
  const ahead = Math.min(1000, Math.max(120, ms * 25));
  const px = mapH() * 0.55 * (nav.flat ? 1 : 2);
  const slow = route.mode !== "car";
  const target = Math.min(slow ? 19.5 : 18.6, Math.max(slow ? 16 : 15,
    Math.log2(((slow ? 156543 : 78271.5) * Math.cos((at[1] * Math.PI) / 180) * px) / ahead)));
  nav.zoom = nav.zoom == null ? target : nav.zoom + (target - nav.zoom) * 0.25;
  return nav.zoom;
}
const navPad = () => {
  const h = mapH();
  return { top: Math.round(h * 0.42), bottom: nav.elev ? 140 : 90, left: 20, right: 20 };
};
function drawNavLine(p) {
  const key = `${p.i} ${p.ll}`;
  if (nav.lineAt === key && nav.lineOf === nav.d) return; // standing still: the same line
  nav.lineAt = key;
  nav.lineOf = nav.d;
  drawRouteTraffic(nav.d, p.i, p.ll);
  const rest = [p.ll, ...nav.d.line.slice(p.i + 1)];
  setSource("route", { type: "FeatureCollection", features: [
    { type: "Feature", geometry: { type: "LineString", coordinates: rest }, properties: {} },
    ...nav.stops.map((x, j) => x.i > p.i && { type: "Feature", geometry: { type: "Point", coordinates: nav.d.line[x.i] }, properties: { end: "stop", n: String(j + 1) } }).filter(Boolean),
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
  navSign.hidden = navThen.hidden = navLimit.hidden = navSpeed.hidden = navLanes.hidden = true;
  const last = nav.d.steps[nav.d.steps.length - 1];
  if (!nav.spoken.has("end")) {
    nav.spoken.add("end");
    if (last?.pre) say1(last.pre);
    else say1(T.arrived, UI);
  }
  // navigation ends when the sentence is said, 4 s after it (6 s at least, 20 s at most)
  const trip = nav.trip, wait = (ms) => new Promise((done) => setTimeout(done, ms));
  Promise.all([wait(6000), Promise.race([sayChain.then(() => wait(4000)), wait(20e3)])]).then(() => {
    if (nav.on && nav.arrived && nav.trip === trip) navStop();
  });
}
function stopTracking() {
  if (nav.watch != null) navigator.geolocation.clearWatch(nav.watch);
  nav.watch = null;
  clearInterval(nav.timer);
  nav.timer = 0;
  clearInterval(nav.watchdog);
  nav.watchdog = 0;
  gpsLost(false);
}
// No fix for a while (a tunnel, a deep valley, a browser that stopped): the banner and the voice say
// so, once. The next fix brings the banner back. Not while the page is hidden: the browser sends no
// fixes then.
const GPS_LOST_S = 12;
function gpsLost(on) {
  if (on === !!nav.lost) return;
  nav.lost = on;
  navBox.classList.toggle("lm-nav-lost", on);
  if (!on) return;
  navBanner.querySelector(".lm-nav-instr")?.replaceChildren(T.gpsLost);
  navSign.hidden = navThen.hidden = navLanes.hidden = true;
  say1(T.gpsLost, UI);
}
function checkFix() {
  if (!nav.on || nav.sim || nav.arrived || nav.fixT == null || document.visibilityState !== "visible") return;
  if (Date.now() - nav.fixT > GPS_LOST_S * 1000) gpsLost(true);
}
document.addEventListener("visibilitychange", () => { if (nav.fixT != null) nav.fixT = Date.now(); });
// a fix of the location: on the line, or off it (then, a few times running, a new route)
function onFix(pos) {
  if (!nav.on || nav.arrived) return;
  nav.lastFix = pos;
  nav.fixT = Date.now();
  gpsLost(false);
  const ll = [pos.coords.longitude, pos.coords.latitude];
  const first = nav.first; // the first fix on a new route counts as on it: the route starts at the road nearest to it
  const p = snap(ll, first);
  nav.first = false;
  const cfg = NAV[route.mode], acc = Math.min(pos.coords.accuracy || 0, cfg.acc);
  const h = pos.coords.heading;
  const moving = h != null && !Number.isNaN(h) && pos.coords.speed > 1 ? h : null;
  const turn = moving == null ? 0 : Math.abs(((moving - p.heading + 540) % 360) - 180);
  const away = cfg.away && moving != null && pos.coords.speed > 2 && turn >= 60 && p.dist > (cfg.off + acc) / 2;
  const off = !first && (p.dist > cfg.off + acc || away);
  if (!first && arrivedNow(p, ll, pos, acc, !off)) {
    nav.at = p.dist <= cfg.off + acc ? p.ll : ll;
    nav.avatar?.set(nav.at, p.heading, 900, 0);
    return navArrive();
  }
  if (off) {
    nav.off++;
    ll.heading = moving;
    navUpdate(p, ll, true);
    // no new route on the last stretch: the traveller goes to a door, a gate or a yard off the road
    const last = nav.cum[nav.cum.length - 1] - p.along <= 3 * cfg.reach + acc;
    if (nav.off >= cfg.fixes && !nav.rerouting && !last) reroute(ll);
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
// stops: the stops still ahead ({ll, label}), else those of the route not yet passed; quiet: the same
// way in another language (setLang), with no "Route recalculated"
async function reroute(ll, stops = null, { quiet = false } = {}) {
  nav.rerouting = nav.routing = true;
  if (!quiet) navBanner.querySelector(".lm-nav-instr")?.replaceChildren(T.rerouting);
  const via = stops || nav.stops.filter((x) => x.i > nav.idx && x.ll && !x.done).map(({ ll, label }) => ({ ll, label }));
  try {
    const q = new URLSearchParams({ from: `${ll[1].toFixed(5)},${ll[0].toFixed(5)}`, to: `${route.to.ll[1].toFixed(5)},${route.to.ll[0].toFixed(5)}`, mode: route.mode, lang: LANG });
    for (const x of via) q.append("via", `${x.ll[1].toFixed(5)},${x.ll[0].toFixed(5)}`);
    // the way the traveller is going: no new route that starts with turning round (by car)
    if (ll.heading != null) q.set("heading", String(Math.round((ll.heading + 360) % 360)));
    const r = await fetch(`${ROUTE}?${q}`);
    const d = await r.json();
    if (r.ok && nav.on && !nav.arrived) {
      d.via = via;
      navLoad(d);
      preSayAll(d.steps);
      nav.first = true;
      // "Route recalculated", then the first instruction of the new route
      nav.spoken.add("0");
      if (!quiet) speak([[T.rerouted, UI], d.steps[0]?.pre || d.steps[0]?.text]);
      if (nav.lastFix) onFix(nav.lastFix); // the banner at once, not at the next fix
    }
  } catch {}
  nav.routing = false;
  setTimeout(() => { nav.rerouting = false; }, 10e3); // not more often than every 10 s
}
async function navStart(sim) {
  if (!route.answer) return;
  if (!nav.muted) unlockSound();
  preSay(route.answer.steps.slice(0, 2), route.answer.steps);
  preSayAll(route.answer.steps);
  fetchSay(T.gpsLost, UI);
  fetchSay(T.rerouted, UI);
  // the styles of day and night by car (tunnels) and the one to come back to, before they are needed
  const styles = route.mode === "car" ? [shownStyle(), "driving", "driving-night"] : [shownStyle()];
  for (const k of styles) styleJSON(styleURL(k), k.endsWith("-night")).catch(() => {});
  nav.on = true;
  fpsCap(true);
  nav.trip = (nav.trip || 0) + 1;
  unfollowMe();
  alongDone();
  nav.sim = sim;
  nav.follow = true;
  nav.arrived = false;
  nav.first = true;
  nav.fixT = null;
  nav.lastFix = null;
  navLoad(route.answer);
  root.classList.add("lm-navigating");
  showDim();
  navBox.hidden = false;
  navCenter.hidden = true;
  navMute.innerHTML = svg(nav.muted ? ICON.muted : ICON.sound, 22);
  nav.speed = sim ? 0 : null;
  nav.zoom = null;
  nav.lastAlong = null;
  navSpeed.hidden = true;
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
  await keepAwake();
  if (sim) {
    // Preview: along the line at three times the speed of each step
    // nav.simAlong: navLoad sets it to 0, so a new route (a stop added) drives on from its start
    let last = performance.now();
    const speedAt = () => {
      const st = [...nav.d.steps].reverse().find((x) => nav.cum[x.i] <= nav.simAlong) || nav.d.steps[0];
      return st.s > 0 ? (st.km * 1000) / st.s : 10;
    };
    nav.timer = setInterval(() => {
      const now = performance.now();
      nav.speed = speedAt() * 3;
      nav.simAlong += nav.speed * ((now - last) / 1000);
      last = now;
      if (nav.routing) return; // a new route on its way
      const total = nav.cum[nav.cum.length - 1];
      const ll = pointAt(Math.min(nav.simAlong, total));
      const p = snap(ll, false);
      navUpdate(p, null);
      if (nav.simAlong >= total) navArrive();
    }, 500);
    navUpdate(snap(nav.d.line[0], true), null);
  } else {
    navBanner.innerHTML = "";
    navBanner.append(el("div", "lm-nav-text lm-nav-instr", {}, T.waitGPS));
    navSign.hidden = navThen.hidden = navLimit.hidden = navLanes.hidden = true;
    nav.watch = navigator.geolocation.watchPosition(onFix, (err) => {
      if (err.code === 1) {
        navBanner.innerHTML = "";
        navBanner.append(el("div", "lm-nav-text lm-nav-instr", {}, T.noGPS));
      } // 2 (no position) and 3 (none in time) come while fixes still come too: the watchdog decides
    }, { enableHighAccuracy: true, maximumAge: 1000, timeout: 20000 });
    nav.watchdog = setInterval(checkFix, 2000);
  }
}
// the screen stays on while navigating; the browser drops the lock when the page is hidden, so it
// is asked again when the page comes back. With the screen locked a browser does not navigate.
async function keepAwake() {
  if (!nav.on || nav.lock || document.visibilityState !== "visible") return;
  try {
    nav.lock = await navigator.wakeLock?.request("screen");
    nav.lock?.addEventListener("release", () => { nav.lock = null; });
  } catch {}
}
document.addEventListener("visibilitychange", keepAwake);
function navStop() {
  nav.on = false;
  fpsCap(false);
  navAlongClose();
  if (navLook) { // back to the style of the menu
    navLook = null;
    document.documentElement.dataset.theme = theme();
    restyle();
  }
  stopTracking();
  hush();
  nav.lock?.release?.().catch(() => {});
  nav.lock = null;
  nav.avatar?.remove();
  nav.avatar = null;
  sayQueue = [];
  navElev.hidden = true;
  navBox.hidden = true;
  root.classList.remove("lm-navigating", "lm-nav-elev-on");
  showDim();
  followMe();
  setTimeout(() => {
    map.resize();
    if (route.changed && !routeView.hidden) { // a stop added in navigation: the route of directions with it
      route.changed = false;
      map.jumpTo({ pitch: look.threeD ? 50 : 0, bearing: 0 }); // flat again, as below
      renderRoute();
      return findRoute();
    }
    route.changed = false;
    drawEnds(route.answer?.line || null);
    drawRouteTraffic(route.answer);
    const fs = route.alongCat && route.answer?.along?.[route.alongCat]; // the places along chosen in directions
    if (fs && !routeView.hidden) setSource("along", { type: "FeatureCollection", features: fs });
    const b = route.answer && extent({ type: "LineString", coordinates: route.answer.line });
    // back to the whole route, flat unless the menu has 3D on
    if (b) fit(() => map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: pad(), maxZoom: 16, pitch: look.threeD ? 50 : 0, bearing: 0, duration: 700 }));
  }, 0);
}
navEnd.addEventListener("click", navStop);
// search along the route in navigation: places ahead; one added as a stop gives a new route from here
let navAlongCat = "";
function navAlongOpen() {
  navAlong.innerHTML = "";
  const head = el("div", "lm-nav-along-head");
  head.append(el("strong", "", {}, T.along));
  const x = el("button", "lm-icon-btn", { type: "button", title: T.close, "aria-label": T.close });
  x.innerHTML = svg(ICON.close, 18);
  x.addEventListener("click", navAlongClose);
  head.append(x);
  navAlong.append(head, alongPanel(nav.d, {
    ahead: () => (nav.along || 0) / 1000,
    chosen: navAlongCat,
    onChoose: (cat) => { navAlongCat = cat; },
    onAdd: navAddStop,
  }));
  navAlong.dataset.kind = "along";
  navAlong.hidden = false;
  root.classList.add("lm-nav-along-on");
}
// a stop added in navigation: a new route from here, through the stops ahead with the new one where
// it falls on the route
function navAddStop(p) {
  if (!nav.at || nav.routing) return;
  const ahead = nav.stops.filter((x) => x.i > nav.idx && x.ll && !x.done);
  const a = projectOn(nav.d.line, nav.cum, p.ll);
  const k = ahead.findIndex((x) => nav.cum[x.i] > a);
  ahead.splice(k < 0 ? ahead.length : k, 0, p);
  const via = ahead.slice(0, MAX_STOPS).map(({ ll, label }) => ({ ll, label }));
  // the stop goes into directions too: after navigation they show the route with it
  insertStop({ ll: p.ll, label: p.label });
  route.changed = true;
  navAlongClose();
  const at = [...nav.at];
  at.heading = nav.sim ? null : nav.heading;
  reroute(at, via);
}
function navAlongClose() {
  navAlong.hidden = true;
  navAlong.innerHTML = "";
  delete navAlong.dataset.kind;
  root.classList.remove("lm-nav-along-on");
  alongDone();
  hereCtl?.abort();
  setSource("pin", empty);
  setSource("selected", empty);
}
navAlongBtn.addEventListener("click", () => (navAlong.dataset.kind === "along" ? navAlongClose() : navAlongOpen()));
// A long press on the map in navigation (or a right click): the point, its address and the named
// places around it, in the panel of the places along; the point or one of them as a stop.
async function navPointOpen(ll, symbol) {
  hereCtl?.abort();
  const ctl = hereCtl = new AbortController();
  alongDone();
  const lat = +ll.lat.toFixed(5), lon = +ll.lng.toFixed(5), coords = `${lat.toFixed(5)}, ${lon.toFixed(5)}`;
  setSource("pin", one({ type: "Feature", geometry: { type: "Point", coordinates: [lon, lat] }, properties: {} }));
  setSource("selected", empty);
  navAlong.innerHTML = "";
  navAlong.dataset.kind = "point";
  const head = el("div", "lm-nav-along-head");
  const name = el("strong", "", {}, symbol || T.here);
  const x = el("button", "lm-icon-btn", { type: "button", title: T.close, "aria-label": T.close });
  x.innerHTML = svg(ICON.close, 18);
  x.addEventListener("click", navAlongClose);
  head.append(name, x);
  const sub = el("p", "lm-dsub lm-nav-point-sub", {}, coords);
  const acts = el("div", "lm-chips lm-nav-point-acts");
  const add = el("button", "lm-chip lm-nav-point-add", { type: "button" });
  add.innerHTML = svg(ICON.plus, 16);
  add.append(T.useAsStop);
  add.disabled = nav.stops.filter((s) => s.i > nav.idx && s.ll).length >= MAX_STOPS;
  add.addEventListener("click", () => navAddStop({ ll: [lon, lat], label: name.textContent }));
  acts.append(add);
  const note = el("p", "lm-notes lm-along-msg", { role: "status", "aria-live": "polite" }, T.hereLooking);
  navAlong.append(head, sub, acts, note);
  navAlong.hidden = false;
  root.classList.add("lm-nav-along-on");
  let d;
  try {
    const r = await fetch(`${API}/here?${new URLSearchParams({ lat, lon, zoom: String(Math.max(16, Math.round(map.getZoom()))) })}`, { signal: ctl.signal });
    d = await r.json().catch(() => ({}));
    if (!r.ok) { note.textContent = problem(d); return; }
  } catch (err) {
    if (err.name !== "AbortError") note.textContent = T.failed;
    return;
  }
  if (ctl.signal.aborted || navAlong.dataset.kind !== "point") return;
  name.textContent = d.place?.label || symbol || d.area?.label || T.here;
  sub.textContent = [d.place?.detail, coords].filter(Boolean).join(" · ");
  const fs = d.results?.features || [];
  for (const f of fs) state.byId.set(f.properties.id, f);
  note.textContent = fs.length ? "" : T.navHereNothing;
  note.hidden = fs.length > 0;
  if (!fs.length) return;
  // the places around on the map as the places along are, a tap on one finds its line in the list
  setSource("along", { type: "FeatureCollection", features: fs.filter((f) => f.geometry?.type === "Point") });
  navAlong.append(el("p", "lm-count", {}, T.around));
  const ol = el("ol", "lm-results lm-along-list");
  for (const f of fs) {
    const c = f.geometry?.type === "Point" ? f : { ...f, geometry: { type: "Point", coordinates: center(f) } };
    ol.append(alongItem(c, { onAdd: navAddStop }));
  }
  navAlong.append(ol);
  if (fs.some((f) => f.properties.opening_hours)) loadHours().then(() => markHours(ol));
}
// A finger held still on the map for half a second; a second finger or a move of more than 10 px is
// a gesture of the map. The click after it does nothing.
{
  let t = 0, start = null;
  const cancel = () => { clearTimeout(t); start = null; };
  let pressedAt = 0;
  const press = (x, y) => {
    if (!nav.on) return;
    pressedAt = Date.now();
    const b = map.getCanvasContainer().getBoundingClientRect();
    const pt = { x: x - b.left, y: y - b.top };
    navPointOpen(map.unproject([pt.x, pt.y]), symbolAt(pt));
  };
  const canvas = map.getCanvasContainer();
  canvas.addEventListener("touchstart", (e) => {
    cancel();
    if (!nav.on || e.touches.length !== 1) return;
    const { clientX: x, clientY: y } = e.touches[0];
    start = [x, y];
    t = setTimeout(() => { start = null; ghostUntil = Date.now() + 700; press(x, y); }, 500);
  }, { passive: true });
  canvas.addEventListener("touchmove", (e) => {
    if (!start) return;
    const p = e.touches[0];
    if (e.touches.length !== 1 || Math.hypot(p.clientX - start[0], p.clientY - start[1]) > 10) cancel();
  }, { passive: true });
  canvas.addEventListener("touchend", cancel, { passive: true });
  canvas.addEventListener("touchcancel", cancel, { passive: true });
  // a right click; the long press of a phone sends one too, after the press above
  map.on("contextmenu", (e) => {
    if (!nav.on) return;
    e.preventDefault();
    e.originalEvent?.preventDefault?.();
    if (Date.now() - pressedAt < 1500) return;
    press(e.originalEvent.clientX, e.originalEvent.clientY);
  });
}
showDim();
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
    // the map itself, not the buttons over it (the menu, traffic, the compass)
    if (!nav.on || !e.target.closest?.(".maplibregl-canvas-container")) return;
    nav.follow = false;
    navCenter.hidden = false;
  }, { passive: true, capture: true });
}

// ------------------------------------------------------------------ the language
// A new language at once, with what is open kept: the texts built once at the start are found in the
// page by their old wording and given the new one; the map names, the place, the route and the steps
// of navigation are asked again in the new language. Navigation goes on from where it is.
function setLang(chosen) {
  const old = JSON.parse(JSON.stringify(T)); // the strings; functions drop out
  LANG = langOf(chosen);
  UI = languages.some(([k]) => k === LANG) ? LANG : "en";
  STEP_LANG = LOCALES[LANG] ? LANG : "en";
  document.documentElement.lang = UI;
  loadTexts();
  const swap = new Map();
  const pair = (a, b) => {
    if (typeof a === "string" && typeof b === "string") { if (a !== b && !swap.has(a)) swap.set(a, b); }
    else if (a && b && typeof a === "object" && typeof b === "object") for (const k of Object.keys(a)) pair(a[k], b[k]);
  };
  pair(old, JSON.parse(JSON.stringify(T)));
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n = walk.nextNode(); n; n = walk.nextNode()) {
    const t = n.data.trim(), to = t && swap.get(t);
    if (to) n.data = n.data.replace(t, to);
  }
  for (const e of document.body.querySelectorAll("[title], [aria-label], [placeholder], [alt]")) {
    for (const a of ["title", "aria-label", "placeholder", "alt"]) {
      const to = e.hasAttribute(a) && swap.get(e.getAttribute(a));
      if (to) e.setAttribute(a, to);
    }
  }
  foot.textContent = footText();
  menuFoot.textContent = menuFootText();
  hints.splice(0, hints.length, ...(T.hints || []));
  if (!input.value) newHint();
  for (const end of [route.from, route.to]) if (end?.me) end.label = T.myPlace;
  for (const f of routeView.querySelectorAll("input")) { const to = swap.get(f.value); if (to) f.value = to; }
  restyle(); // the names on the map
  if (nav.on && !nav.arrived && nav.at) {
    const at = [...nav.at];
    at.heading = nav.heading;
    reroute(at, null, { quiet: true });
  } else if (route.from && route.to && !routeView.hidden) findRoute({ quiet: true });
  if (!detailView.hidden && state.selected) renderDetail(state.selected);
}
// ------------------------------------------------------------------ start
// the intro at once (before the map loads, a visitor may already open directions); a search of the
// address once the map is there, as it searches the map view
const startQ = new URLSearchParams(location.search).get("q");
if (!startQ) intro();
map.once("load", () => {
  loadTraffic();
  drawStars();
  loadTransit();
  const u = new URLSearchParams(location.search);
  const pt = (v) => { const m = /^(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$/.exec(v || ""); return m && Math.abs(+m[1]) <= 90 && Math.abs(+m[2]) <= 180 ? [+m[2], +m[1]] : null; };
  const at = pt(u.get("at")), to = pt(u.get("to"));
  if (startQ) {
    const extra = Object.fromEntries(extraKeys.map((k) => [k, u.get(k) || ""]));
    run(startQ, extra, { push: false, sel: u.get("sel") });
  } else if (to) { // a shared route
    if (["car", "bike", "walk"].includes(u.get("mode"))) route.mode = u.get("mode");
    const from = pt(u.get("from"));
    directions({ ll: to, label: u.get("tn") || T.mapPoint });
    const names = u.getAll("vn");
    route.stops = u.getAll("via").map((v, i) => { const p = pt(v); return p && { ll: p, label: names[i] || T.mapPoint }; }).filter(Boolean).slice(0, MAX_STOPS);
    if (route.stops.length) {
      renderRoute();
      if (!from && route.from) findRoute(); // directions found the route from the location before the stops were there
    }
    if (from) setFrom({ ll: from, label: u.get("fn") || T.mapPoint }); // before the location: it fills only a start still missing
  } else if (at) { // a shared place
    fit(() => map.jumpTo({ center: at, zoom: Math.max(map.getZoom(), 16) }));
    here({ lng: at[0], lat: at[1] }, u.get("n") || "");
  } else if (!linkedView) showMe(true);
});
if (!root.dataset.key) say("The map is not configured on this server: MAPS_KEY is missing.", true);
