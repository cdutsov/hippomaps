// What the map needs before its first tile, asked for as the page starts: this small script runs while the large
// ones (MapLibre, app.js) are still on their way, and the map then finds the answers in the cache of the browser
// instead of asking one after the other once it runs. Nothing here is needed for the page to work.
//  - what the map asked for before its first tile at the last visit (hm-early, kept by app.js): its style, the
//    TileJSON of its sources and its icons; at a first visit the light and the dark style in the language of the
//    browser, one of which the map takes, and their TileJSON (kept in window.hmEarly for app.js, which so knows the
//    tiles and the fonts of its first view as it starts)
//  - the worker of MapLibre, which reads the tiles (data-worker), unless the service worker has the files of the page
(() => {
  const d = document.currentScript?.dataset;
  if (!d || !window.fetch) return;
  const whole = (u) => fetch(u).then((r) => r.arrayBuffer()).catch(() => {}); // read to the end: only then is it kept
  if (d.worker && !navigator.serviceWorker?.controller) whole(d.worker);
  if (!d.tiles || !d.key) return;
  const styles = `${d.tiles}/v1/${d.key}/maps/styles/`;
  let last = [];
  try { last = JSON.parse(localStorage.getItem("hm-early")) || []; } catch {}
  if (Array.isArray(last) && typeof last[0] === "string" && last[0].startsWith(styles)) {
    for (const u of last) if (typeof u === "string" && u.startsWith(`${d.tiles}/`)) whole(u);
    return;
  }
  const l = (navigator.language || "en").slice(0, 2).toLowerCase();
  const lang = ["de", "fr", "it"].includes(l) ? l : "en";
  // (what a first visit reads, by address: app.js takes the tiles and the fonts of its first view from it)
  const got = window.hmEarly = { styles: {}, sources: {} };
  const json = (u) => fetch(u).then((r) => (r.ok ? r.json() : null)).catch(() => null);
  for (const name of ["light", "dark"]) {
    const url = `${styles}${name}.json?lang=${lang}`;
    json(url).then((st) => {
      if (!st) return;
      got.styles[url] = st;
      for (const s of Object.values(st.sources || {})) {
        if (typeof s.url !== "string" || !s.url.startsWith(`${d.tiles}/`) || s.url in got.sources) continue;
        got.sources[s.url] = null;
        json(s.url).then((tj) => { got.sources[s.url] = tj; });
      }
    });
  }
})();
