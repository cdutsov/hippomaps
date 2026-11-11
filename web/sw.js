// The service worker of hippomaps (served at {base}/sw.js, its scope the page). Without a connection it
// serves the page and its files as last loaded, and the map: the tiles of the route being navigated (the page
// puts them in the cache hm-route at Start; they are served from there first, also with a connection), and the
// styles, fonts and sprites as last loaded. Everything else goes to the network as without it.
const SHELL = "hm-shell-v1", ASSETS = "hm-assets-v1", ROUTE = "hm-route";
const scope = new URL(self.registration.scope);
const base = scope.pathname.replace(/\/$/, ""); // "" at the root of a site, else the path of the page
const pageKey = new URL(`${base}/`, scope.origin).href; // the page, whatever its query

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil((async () => {
  for (const k of await caches.keys()) if (![SHELL, ASSETS, ROUTE].includes(k)) await caches.delete(k);
  await self.clients.claim();
})()));

const TILE = /\/maps\/[^/]+\/\d+\/\d+\/\d+\.[a-z]+$/; // vector, terrain …: the HTTP cache of the browser keeps them
self.addEventListener("fetch", (e) => {
  const r = e.request;
  if (r.method !== "GET") return;
  const u = new URL(r.url);
  const ours = u.origin === scope.origin;
  if (r.mode === "navigate") {
    if (ours && u.pathname === (base || "/")) e.respondWith(page(e, r));
    return;
  }
  if (ours && u.pathname.startsWith(`${base}/static/`)) return e.respondWith(file(e, r, u));
  if (/^\/v1\/[^/]+\/maps\//.test(u.pathname) || u.pathname.startsWith("/maps/assets/")) e.respondWith(mapFile(e, r, u));
});

// the page: from the network, its copy when there is none (or none within 8 s)
async function page(e, r) {
  const c = await caches.open(SHELL);
  const net = fetch(r).then((res) => {
    if (res.ok) e.waitUntil(c.put(pageKey, res.clone()));
    return res;
  });
  const copy = await c.match(pageKey);
  if (!copy) return net;
  return Promise.race([net.catch(() => copy), new Promise((done) => setTimeout(() => done(copy), 8000))]);
}

// the files of the page: with their hash in the address (or a version in the path) from the copy, else from the
// network with the copy when there is none; one copy per path
async function file(e, r, u) {
  const c = await caches.open(SHELL);
  const fixed = u.searchParams.has("v") || u.pathname.includes("/vendor/");
  if (fixed) {
    const hit = await c.match(r, { ignoreVary: true });
    if (hit) return hit;
  }
  try {
    const res = await fetch(r);
    if (res.ok) e.waitUntil(keep(c, r.url, res.clone()));
    return res;
  } catch (err) {
    return (await c.match(r, { ignoreVary: true, ignoreSearch: !fixed })) || Response.error();
  }
}
async function keep(c, url, res) {
  const p = new URL(url).pathname;
  for (const k of await c.keys()) if (k.url !== url && new URL(k.url).pathname === p) await c.delete(k);
  await c.put(url, res);
}

// the map: a tile of the route from the cache of the route; else the network, and for styles, TileJSON, fonts and
// sprites a copy; without an answer, whatever copy there is
async function mapFile(e, r, u) {
  const hit = await caches.match(r.url, { cacheName: ROUTE, ignoreVary: true }); // (without making the cache)
  if (hit) return hit;
  try {
    const res = await fetch(r);
    if (res.ok && !TILE.test(u.pathname)) {
      const copy = res.clone(); // (now: the page reads the answer meanwhile)
      e.waitUntil(caches.open(ASSETS).then((c) => c.put(r.url, copy)));
    }
    return res;
  } catch (err) {
    return (await caches.match(r.url, { ignoreVary: true })) || Response.error();
  }
}
