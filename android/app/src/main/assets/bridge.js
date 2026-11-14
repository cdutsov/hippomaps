// Injected at document start into https://hippomaps.ch only. Connects the page to the app through
// the message channel "hmNative" (WebViewCompat.addWebMessageListener, same origin restriction).
(() => {
  "use strict";
  if (window.__hmAndroid) return;
  window.__hmAndroid = true;
  const port = window.hmNative;
  if (!port) return;
  const send = (msg) => port.postMessage(JSON.stringify(msg));

  // answers from the app: { id, ok, error }
  let nextId = 1;
  const waiting = new Map();
  const handlers = {}; // messages from the app by type; answers to a request carry its id
  port.onmessage = (e) => {
    let m;
    try { m = JSON.parse(e.data); } catch { return; }
    if (m.type && handlers[m.type]) return handlers[m.type](m);
    const w = waiting.get(m.id);
    if (!w) return;
    waiting.delete(m.id);
    if (m.ok) w.resolve();
    else w.reject(new DOMException(m.error || "Share failed", m.error === "AbortError" ? "AbortError" : "NotAllowedError"));
  };
  const ask = (msg) => new Promise((resolve, reject) => {
    const id = nextId++;
    waiting.set(id, { resolve, reject });
    send({ ...msg, id });
  });

  // navigator.share: the Android share sheet
  const share = (data = {}) => {
    const url = data.url != null ? new URL(String(data.url), location.href).href : "";
    const title = data.title != null ? String(data.title) : "";
    const text = data.text != null ? String(data.text) : "";
    if (!url && !text && !title) return Promise.reject(new TypeError("Nothing to share"));
    return ask({ type: "share", title, text, url });
  };
  const define = (obj, name, value) => {
    try { Object.defineProperty(obj, name, { value, configurable: true, writable: true }); } catch {}
  };
  define(navigator, "share", share);
  define(navigator, "canShare", (data = {}) => !data.files && (data.url != null || data.text != null || data.title != null));

  // navigator.wakeLock: the screen stays on while one lock is held (FLAG_KEEP_SCREEN_ON)
  const held = new Set();
  class Sentinel extends EventTarget {
    constructor(id) { super(); this.id = id; this.released = false; this.type = "screen"; this.onrelease = null; }
    release() {
      if (!this.released) {
        this.released = true;
        held.delete(this);
        send({ type: "wake", id: this.id, on: false });
        const ev = new Event("release");
        this.dispatchEvent(ev);
        if (typeof this.onrelease === "function") this.onrelease(ev);
      }
      return Promise.resolve();
    }
  }
  define(navigator, "wakeLock", {
    request(type = "screen") {
      if (type !== "screen") return Promise.reject(new TypeError("Only screen wake locks"));
      if (document.visibilityState !== "visible") return Promise.reject(new DOMException("Page not visible", "NotAllowedError"));
      const s = new Sentinel(nextId++);
      held.add(s);
      send({ type: "wake", id: s.id, on: true });
      return Promise.resolve(s);
    },
  });
  // as in a browser: a hidden page loses its locks (the page asks again when it is shown)
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") for (const s of [...held]) s.release();
  });

  // the page theme: light or dark, for the system bars
  const html = document.documentElement;
  let lastTheme = null;
  const reportTheme = () => {
    const dark = html.dataset.theme === "dark";
    if (dark === lastTheme) return;
    lastTheme = dark;
    send({ type: "theme", dark });
  };
  new MutationObserver(reportTheme).observe(html, { attributes: true, attributeFilter: ["data-theme"] });
  document.addEventListener("DOMContentLoaded", reportTheme);

  // long press on the map: no text selection or callout
  const style = document.createElement("style");
  style.textContent = ".lm-map canvas, .maplibregl-canvas-container { -webkit-user-select: none; user-select: none; -webkit-touch-callout: none; }";
  (document.head || html).appendChild(style);

  // navigator.geolocation: positions from the Android location service (GPS and network), pushed
  // by the app. They keep coming with the screen off while the page navigates (foreground service).
  const geo = (() => {
    const watchers = new Map(); // id -> { ok, fail, maxAge, once, timer }
    let last = null; // the last position the app pushed
    let seq = 1;
    const error = (code, message) => ({ code, message, PERMISSION_DENIED: 1, POSITION_UNAVAILABLE: 2, TIMEOUT: 3 });
    const position = (p) => {
      const coords = {
        latitude: p.lat, longitude: p.lon, accuracy: p.acc,
        altitude: p.alt ?? null, altitudeAccuracy: p.altAcc ?? null,
        heading: p.heading ?? null, speed: p.speed ?? null,
      };
      coords.toJSON = () => ({ ...coords, toJSON: undefined });
      return { coords, timestamp: p.t, toJSON: () => ({ coords: coords.toJSON(), timestamp: p.t }) };
    };
    const sync = () => send({ type: "geo", watch: watchers.size > 0 });
    const remove = (id) => {
      const w = watchers.get(id);
      if (!w) return;
      clearTimeout(w.timer);
      watchers.delete(id);
      sync();
    };
    // a live fix goes to every watcher; a remembered one only where it is recent enough
    handlers.pos = (m) => {
      last = m;
      for (const [id, w] of [...watchers]) {
        if (!m.fresh && Date.now() - m.t > w.maxAge) continue;
        if (w.once) remove(id);
        try { w.ok(position(m)); } catch (err) { setTimeout(() => { throw err; }); }
      }
    };
    handlers.geoerr = (m) => {
      for (const [id, w] of [...watchers]) {
        if (m.code !== 1 && !w.once) continue; // no fix yet: a watch waits
        remove(id);
        try { w.fail?.(error(m.code, m.message || "")); } catch {}
      }
    };
    const start = (ok, fail, opts, once) => {
      opts = opts || {};
      const id = seq++;
      const ma = Number(opts.maximumAge); // Infinity: any remembered position
      const maxAge = ma > 0 ? ma : 0;
      if (once && last && Date.now() - last.t <= maxAge) {
        const p = last;
        setTimeout(() => ok(position(p)), 0);
        return id;
      }
      const w = { ok, fail, maxAge, once };
      if (once && Number.isFinite(opts.timeout)) {
        w.timer = setTimeout(() => { remove(id); fail?.(error(3, "Timeout expired")); }, Math.max(0, opts.timeout));
      }
      watchers.set(id, w);
      sync();
      return id;
    };
    return {
      getCurrentPosition(ok, fail, opts) { start(ok, fail, opts, true); },
      watchPosition(ok, fail, opts) { return start(ok, fail, opts, false); },
      clearWatch(id) { remove(id); },
    };
  })();
  define(navigator, "geolocation", geo);
  addEventListener("pagehide", () => send({ type: "geo", watch: false }));

  // window.hmCompass: the heading of the phone from its rotation sensor, while the page watches it.
  // watch(fn) calls fn({ heading, accuracy }) (degrees clockwise from true north toward which the top
  // of the screen points; how far off it may be, in degrees) and returns the function that stops it.
  const compass = (() => {
    const fns = new Set();
    const sync = () => send({ type: "compass", on: fns.size > 0 });
    handlers.heading = (m) => {
      for (const fn of [...fns]) {
        try { fn({ heading: m.h, accuracy: m.acc }); } catch (err) { setTimeout(() => { throw err; }); }
      }
    };
    return {
      watch(fn) {
        if (typeof fn !== "function") throw new TypeError("hmCompass.watch needs a function");
        const f = (h) => fn(h); // one entry per watch, also for the same function twice
        fns.add(f);
        if (fns.size === 1) sync();
        return () => { if (fns.delete(f) && fns.size === 0) sync(); };
      },
    };
  })();
  define(window, "hmCompass", Object.freeze(compass));
  addEventListener("pagehide", () => send({ type: "compass", on: false }));

  // navigation: on and off from the class of #app, and the banner (distance, instruction) for the
  // notification of the app
  let navOn = false, lastInfo = "";
  const watchNav = (app) => {
    const banner = () => app.querySelector(".lm-nav-banner");
    const info = () => {
      const b = banner();
      const dist = b?.querySelector(".lm-nav-dist")?.textContent?.trim() || "";
      const instr = b?.querySelector(".lm-nav-instr")?.textContent?.trim() || "";
      const key = `${dist}\n${instr}`;
      if (key === lastInfo) return;
      lastInfo = key;
      send({ type: "navinfo", dist, instr });
    };
    let bannerObserver = null;
    const check = () => {
      const on = app.classList.contains("lm-navigating");
      if (on !== navOn) {
        navOn = on;
        lastInfo = "";
        send({ type: "nav", on });
      }
      if (on && !bannerObserver && banner()) {
        bannerObserver = new MutationObserver(info);
        bannerObserver.observe(banner(), { childList: true, subtree: true, characterData: true });
      }
      if (on) info();
    };
    new MutationObserver(check).observe(app, { attributes: true, attributeFilter: ["class"] });
    check();
  };
  const findApp = () => {
    const app = document.getElementById("app");
    if (app) { watchNav(app); return true; }
    return false;
  };
  if (!findApp()) {
    const o = new MutationObserver(() => { if (findApp()) o.disconnect(); });
    o.observe(html, { childList: true, subtree: true });
  }
  window.__hmEndNav = () => { document.querySelector(".lm-nav-x")?.click(); };

  send({ type: "hello" });

  // the back button of the phone: what the page has open closes first.
  // "nav" while navigating, "handled" when the page closed something, "none" otherwise.
  const shown = (e) => !!e && e.getClientRects().length > 0;
  window.__hmBack = () => {
    const app = document.getElementById("app");
    if (!app) return "none";
    if (app.classList.contains("lm-navigating")) return "nav";
    const menu = document.querySelector(".lm-menu");
    if (shown(menu)) {
      const backdrop = document.querySelector(".lm-backdrop");
      (shown(backdrop) ? backdrop : menu.querySelector(".lm-menu-head .lm-icon-btn"))?.click();
      return "handled";
    }
    const focused = document.activeElement;
    if (focused && focused.matches("input, textarea") && [...document.querySelectorAll(".lm-suggest")].some(shown)) {
      focused.blur();
      return "handled";
    }
    const button = [...document.querySelectorAll(".lm-back")].find(shown) || [...document.querySelectorAll(".lm-card-x")].find(shown);
    if (button) { button.click(); return "handled"; }
    return "none";
  };
})();
