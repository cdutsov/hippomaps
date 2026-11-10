#!/usr/bin/env python3
"""hippomaps in headless Chromium, as a visitor uses it (tests/site-test.sh with BROWSER=1: after a deploy, nightly).

    python3 tests/browser.py https://hippomaps.ch

Prints "ok <what>" per check and "FAIL <what>" per failure; exits 1 on a failure. The browser is
in Zurich (a granted location) on a phone screen. Checks: without the location the map opens at the place of the IP
address (of the machine of the test), with the credit of DB-IP; the map starts at the location; the
Directions button opens start and destination, a typed destination gives a route by car with its
time; Preview navigates: the banner, the speed now, the speed limit, the traveller in 3D, the spoken
sentences of the server; the language changed while navigating (no new page); a long press while
navigating (the point and the places around, the point as a stop); the traffic button; the map drawn at 2 pixels per point at most, our sources with tiles to zoom 14 only, the
banner kept between updates of one turn, dark and light switched in place (our layers and the route kept, no new load of the map); a stop and the places along a
route; navigation drawn at 30 frames a second at most, the camera following, tilted 50 degrees, the next turn right under the banner; arrival by bike with Start and fixes that stop short of the end (said once, navigation
ends); the X clears everything; the panel of a phone (empty: stays down; the suggestions scroll, a tap picks;
directions at their top when the keyboard goes; up and down in one movement when the keyboard comes and goes, a field of
directions takes it to the top); a place searched in navigation as the destination; a ride by bike with the location of the device (fixes at 5 m/s): each turn far
enough from the one before said ahead with its distance ("In 150 meters, …"), then at the turn, no sentence
twice (the voice answered with silence in the test); German texts after the language of the menu, also the words of
the search; and no
errors of the page on the way. Needs chromium and websocket-client.
"""
import base64
import json
import math
import os
import random
import re
import struct
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

import websocket

URL = sys.argv[1].rstrip("/")
PORT = 9341
ZURICH = (47.3779, 8.5403)
fails = []
# the sentences spoken (window.hmSaid): the sounds of the server played, the texts of the browser voice
SAID_HOOK = r"""(() => {
  const said = window.hmSaid = [], text = {};
  const f0 = window.fetch, c0 = URL.createObjectURL, p0 = HTMLMediaElement.prototype.play;
  window.fetch = function (u) {
    const r = f0.apply(this, arguments);
    if (!String(u).includes('/api/say')) return r;
    const t = new URL(String(u), location.href).searchParams.get('text');
    return r.then((res) => { const b0 = res.blob.bind(res); res.blob = () => b0().then((b) => { b.hmText = t; return b; }); return res; });
  };
  URL.createObjectURL = function (b) { const u = c0.apply(this, arguments); if (b && b.hmText) text[u] = b.hmText; return u; };
  HTMLMediaElement.prototype.play = function () { if (text[this.src]) said.push(text[this.src]); return p0.apply(this, arguments); };
  if (window.speechSynthesis) {
    const s0 = speechSynthesis.speak.bind(speechSynthesis);
    speechSynthesis.speak = (u) => { if (u.text.trim()) said.push(u.text); return s0(u); };
  }
})();"""
# a WAV of 0.3 s of silence: the answer of /api/say in the ride by bike
SILENCE = base64.b64encode(b"RIFF" + struct.pack("<I", 36 + 4800) + b"WAVEfmt " + struct.pack("<IHHIIHH", 16, 1, 1, 8000, 16000, 2, 16)
                           + b"data" + struct.pack("<I", 4800) + bytes(4800)).decode()


def check(cond, what):
    print(("ok " if cond else "FAIL ") + what, flush=True)
    if not cond:
        fails.append(what)


prof = tempfile.mkdtemp(prefix="hm-browser-")
chrome = subprocess.Popen(["chromium", "--headless=new", "--no-sandbox", "--disable-gpu", "--use-angle=swiftshader", "--enable-unsafe-swiftshader",
                           "--autoplay-policy=no-user-gesture-required", f"--remote-debugging-port={PORT}", f"--user-data-dir={prof}",
                           "--window-size=390,844", "about:blank"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(100):
        try:
            tabs = json.load(urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json"))
            break
        except Exception:
            time.sleep(0.2)
    ws = websocket.create_connection([t for t in tabs if t["type"] == "page"][0]["webSocketDebuggerUrl"], timeout=60)
    n, errors, said_n, doc = [0], [], [0], [None]

    def call(method, **params):
        n[0] += 1
        ws.send(json.dumps({"id": n[0], "method": method, "params": params}))
        while True:
            m = json.loads(ws.recv())
            if m.get("method") == "Runtime.exceptionThrown":
                errors.append(m["params"]["exceptionDetails"].get("exception", {}).get("description", "")[:200])
            if m.get("method") == "Fetch.requestPaused":
                if doc[0] and m["params"].get("resourceType") == "Document":  # the page as given in doc
                    body, ctype = base64.b64encode(doc[0].encode()).decode(), "text/html; charset=utf-8"
                else:  # the voice, answered here with a short silence (no load on Piper)
                    body, ctype = SILENCE, "audio/wav"
                    said_n[0] += 1
                ws.send(json.dumps({"id": 10**7 + n[0], "method": "Fetch.fulfillRequest", "params": {"requestId": m["params"]["requestId"],
                    "responseCode": 200, "responseHeaders": [{"name": "Content-Type", "value": ctype}], "body": body}}))
                continue
            if m.get("method") == "Runtime.consoleAPICalled" and m["params"]["type"] == "error":
                errors.append(" ".join(str(a.get("value", a.get("description", ""))) for a in m["params"]["args"])[:200])
            if m.get("id") == n[0]:
                return m.get("result", m.get("error"))

    def js(expr):
        r = call("Runtime.evaluate", expression=expr, awaitPromise=True, returnByValue=True)
        return r.get("result", {}).get("value")

    def wait(expr, secs=30):
        end = time.time() + secs
        while time.time() < end:
            if js(f"!!({expr})"):
                return True
            time.sleep(0.3)
        return False

    def tap(selector):  # a finger on the middle of an element, as on a phone
        r = js(f"(() => {{ const e = document.querySelector({json.dumps(selector)}); if (!e) return null; const b = e.getBoundingClientRect(); return [b.left + b.width / 2, b.top + b.height / 2]; }})()")
        if not r:
            return False
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": r[0], "y": r[1]}])
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        return True

    def key(name, code, vk):
        for t in ("keyDown", "keyUp"):
            call("Input.dispatchKeyEvent", type=t, key=name, code=code, windowsVirtualKeyCode=vk)

    call("Runtime.enable")
    call("Page.enable")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)
    call("Emulation.setTouchEmulationEnabled", enabled=True)
    # a first visit, the location not given: the map opens at the place of the IP address, with the credit of DB-IP;
    # without a place (no database, or a private address) in Zurich, without the credit
    def first_visit(what):
        call("Page.navigate", url=URL)
        wait("window.hmMap?.loaded()", 30)
        near = js("document.getElementById('app')?.dataset.near || ''")
        lat, lon, z = map(float, near.split(",")) if near else (47.37, 8.54, 13)
        check(js(f"(() => {{ const c = hmMap.getCenter(); return Math.abs(c.lat - {lat}) < 0.001 && Math.abs(c.lng - {lon}) < 0.001 && Math.abs(hmMap.getZoom() - {z}) < 0.01; }})()"),
              f"the location not given, the map opens at the place of the IP address ({what}{near})" if near else "the location not given and no place of the IP address: the map opens in Zurich")
        check(js("!!document.querySelector('.maplibregl-ctrl-attrib a[href=\"https://db-ip.com\"]')") == bool(near), "the credit of DB-IP with the place of the IP address" if near else "no credit of DB-IP without a place")
        return near
    if not first_visit(""):
        # no place for this machine (a server without the database, or reached from a private address through
        # docker-proxy): the page once more, as the server gives it to a visitor in Sydney
        page = urllib.request.urlopen(URL + "/").read().decode()
        doc[0] = page.replace('<div class="lm" id="app"', '<div class="lm" id="app" data-near="-33.869,151.209,11"', 1)
        call("Fetch.enable", patterns=[{"urlPattern": URL + "/*", "resourceType": "Document"}])
        first_visit("a visitor in Sydney: ")
        call("Fetch.disable")
        doc[0] = None
    call("Browser.grantPermissions", origin=URL.split("/labs")[0], permissions=["geolocation"])
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    call("Page.navigate", url=URL)

    check(wait("document.querySelector('.maplibregl-canvas')", 30), "the map draws")
    check(wait("location.hash.includes('/47.37')", 20), "the map starts at the location")
    # the map drawn with 2 pixels per point at most (phones have 3; at 3 here the double tap and the focus fail in headless Chromium)
    check(wait("window.hmMap?.isStyleLoaded()", 30) and js("hmMap.getPixelRatio() <= 2"), "the map drawn at 2 pixels per point at most")
    # our sources (route, pins, results …): tiles up to zoom 14 only
    check(js("(() => { const s = Object.entries(hmMap.getStyle().sources).filter(([k, v]) => k.startsWith('lm-') && v.type === 'geojson'); return s.length > 5 && s.every(([, v]) => v.maxzoom === 14); })()"),
          "our sources with tiles to zoom 14")

    # directions: the button, a typed destination, the route by car
    # a double tap zooms the map and opens nothing
    # (the flight to the location and its tiles: from the place of the IP address, which can be hundreds of km away)
    wait("!hmMap.isMoving() && hmMap.loaded()", 20)
    box = js("(() => { const r = document.querySelector('.lm-map').getBoundingClientRect(); return [r.left + r.width / 2, r.top + 150]; })()")
    z0 = js("+location.hash.split('/')[0].slice(5)")
    call("Input.synthesizeTapGesture", x=box[0], y=box[1], tapCount=2, gestureSourceType="touch")  # (two touches 80 ms apart came too late on a busy page)
    zoomed = wait(f"+location.hash.split('/')[0].slice(5) > {z0}", 8)  # a busy page (after a deploy) zooms late
    time.sleep(1.0)  # what a tap would open has come by now
    dt = js("JSON.stringify([location.hash, document.querySelector('.lm-here').hidden, document.querySelector('.lm-detail').hidden])")
    check(zoomed and js("document.querySelector('.lm-here').hidden && document.querySelector('.lm-detail').hidden"),
          f"a double tap zooms in and opens nothing (zoom {z0}, after: {dt})")
    js("document.querySelector('.lm-dir-btn').click()")
    check(wait("!document.querySelector('.lm-route').hidden", 5), "the Directions button opens start and destination")
    check(js("getComputedStyle(document.querySelector('.lm-form')).display === 'none'"), "no search bar above start and destination")
    check(wait("document.activeElement?.closest('.lm-end-to')", 15), "the destination field is ready to type")
    js("[...document.querySelectorAll('.lm-mode')][0].click()")
    wait("document.activeElement?.closest('.lm-end-to')", 5) or js("document.querySelector('.lm-end-to input').focus()")
    call("Input.insertText", text="Bahnhof Enge Zürich")
    # headless Chromium can lose the focus of the page: give it back, the field asks again
    js("(() => { const f = document.querySelector('.lm-end-to input'); if (document.activeElement !== f) { f.focus(); f.dispatchEvent(new Event('input')); } })()")
    check(wait("[...document.querySelectorAll('.lm-end-list:not([hidden]) .lm-opt-label')].some(e => /Enge/.test(e.textContent))", 20),
          "the destination field suggests places")
    # the first suggestion (a click: the page may have lost the focus)
    tap(".lm-end-list:not([hidden]) .lm-opt")
    check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min')", 30), "a route by car with its time")
    check(js("document.querySelectorAll('.lm-step').length") >= 2, "the steps of the route")
    # another start typed over "Your location", its suggestion tapped: the route starts there
    tap(".lm-end-from input")
    call("Input.insertText", text="Paradeplatz Zürich")
    wait("[...document.querySelectorAll('.lm-end-list:not([hidden]) .lm-opt-label')].some(e => /Paradeplatz/.test(e.textContent))", 20)
    tap(".lm-end-list:not([hidden]) .lm-opt")
    check(wait("/Paradeplatz/.test(document.querySelector('.lm-end-from input').value) && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 30),
          "a start typed over your location and tapped: the route starts there")
    # with start and destination set, a tap on the map shows the place; its card goes back to the route as it was
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": 120, "y": 160}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    check(wait("!document.querySelector('.lm-here').hidden", 10) and js("!!document.querySelector('.lm-here .lm-swap-acts')"),
          "a tap on the map with a route: the point shown, the route kept")
    js("document.querySelector('.lm-here:not([hidden]) .lm-back')?.click()")
    check(wait("!document.querySelector('.lm-route').hidden && /Paradeplatz/.test(document.querySelector('.lm-end-from input').value)", 10),
          "back to the route: the start kept")
    to0 = js("document.querySelector('.lm-end-to input').value")
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": 140, "y": 180}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    wait("!!document.querySelector('.lm-here:not([hidden]) .lm-swap-acts')", 10)
    js("document.querySelector('.lm-here:not([hidden]) .lm-swap-acts .lm-chip')?.click()")
    check(wait(f"!document.querySelector('.lm-route').hidden && document.querySelector('.lm-end-to input').value !== {json.dumps(to0)} && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 30),
          "a point of the map as the new destination")
    # (the route may be asked once more when the location comes: wait for the summary)
    check(wait(r"/\d{1,2}:\d{2}/.test(document.querySelector('.lm-sum-eta')?.textContent || '')", 20), "the arrival time of the route")
    check(wait("(() => { const b = document.querySelector('.lm-nav-start')?.getBoundingClientRect(); return !!b && b.height >= 44 && b.top >= 0 && b.bottom <= innerHeight; })()", 10),
          "the Start button, large and in view")

    # navigation: Preview
    js("document.querySelector('.lm-nav-go .lm-chip').click()")
    check(wait("document.getElementById('app').classList.contains('lm-navigating')", 5), "Preview starts navigation")
    check(wait("/\\d/.test(document.querySelector('.lm-nav-dist')?.textContent || '')", 10), "the banner shows the next turn")
    # between updates of one turn only the distance changes: the same element, another text (an update each 0.5 s)
    same = False
    for _ in range(4):
        js("window.hmDist = document.querySelector('.lm-nav-dist'); window.hmDistText = hmDist.textContent; window.hmInstr = document.querySelector('.lm-nav-instr')?.textContent")
        time.sleep(1.6)
        if js("document.querySelector('.lm-nav-instr')?.textContent === hmInstr && hmDist.textContent !== hmDistText"):
            same = js("document.querySelector('.lm-nav-dist') === hmDist")
            break
    check(same, "the banner kept between updates, its distance changed")
    check(wait("!document.querySelector('.lm-nav-speed').hidden && /km\\/h/.test(document.querySelector('.lm-nav-speed').textContent)", 15),
          "the speed now")
    check(wait(r"document.getElementById('app').classList.contains('lm-navigating') && /^\d+$/.test(document.querySelector('.lm-nav-limit').textContent) && !document.querySelector('.lm-nav-limit').hidden", 40), "the speed limit")
    check(js("!document.querySelector('.lm-sprite')"), "the traveller in 3D")
    # navigation draws at most 30 frames a second (fpsCap in app.js), and the camera follows the traveller
    check(js("Object.hasOwn(window.hmMap, 'triggerRepaint')"), "while navigating the map repaints through the 30 fps cap")
    c0 = js("JSON.stringify([hmMap.getCenter(), hmMap.getBearing()])")
    js("window.hmRenders = 0; window.hmCount = () => hmRenders++; hmMap.on('render', hmCount); window.hmT0 = performance.now()")
    check(wait(f"JSON.stringify([hmMap.getCenter(), hmMap.getBearing()]) !== {json.dumps(c0)}", 10), "the camera follows during navigation")
    check(wait("Math.abs(hmMap.getPitch() - 50) < 1", 10), "the map tilted 50 degrees in navigation")
    time.sleep(2)
    fps = js("(() => { hmMap.off('render', hmCount); return hmRenders * 1000 / (performance.now() - hmT0); })()")
    check(fps is not None and fps <= 33, f"the map draws at most 30 frames a second while navigating ({fps or 0:.1f})")
    # the next turn ("then") hangs right under the banner, whatever its height: one and two lines of instruction, in
    # one go (the next update sets them again)
    gaps = js("""(() => { const i = document.querySelector('.lm-nav-instr'), t = document.querySelector('.lm-nav-then'),
      b = document.querySelector('.lm-nav-banner'), was = [i.textContent, t.hidden, t.textContent], out = [];
      t.hidden = false; t.textContent = 'Then';
      for (const s of ['Turn right', 'Turn right onto Papiermühlestrasse toward Zürich, Basel and Biel/Bienne, then keep left']) {
        i.textContent = s; out.push(Math.round(t.getBoundingClientRect().top - b.getBoundingClientRect().bottom), Math.round(b.getBoundingClientRect().height));
      }
      [i.textContent, t.hidden, t.textContent] = was; return out; })()""")
    check(gaps and abs(gaps[0]) <= 1 and abs(gaps[2]) <= 1 and gaps[3] > gaps[1],
          f"the next turn right under the banner, one and two lines (gap, height: {gaps})")
    # the menu while navigating: the map keeps following
    js("document.querySelector('.lm-map-burger').click()")
    time.sleep(0.5)
    js("document.querySelector('.lm-menu .lm-icon-btn')?.click()")
    time.sleep(0.5)
    check(js("document.querySelector('.lm-nav-center').hidden"), "the menu while navigating leaves the map following")
    # another language while navigating: no new page, navigation goes on, the steps in that language
    js("window.hmSamePage = 1; (() => { const s = document.querySelector('.lm-lang-select'); s.value = 'de'; s.dispatchEvent(new Event('change')) })()")
    check(wait("performance.getEntriesByType('resource').some(e => e.name.includes('/api/route') && e.name.includes('lang=de'))", 20)
          and js("window.hmSamePage === 1 && document.documentElement.lang === 'de' && document.getElementById('app').classList.contains('lm-navigating')")
          and js("/suchen/i.test(document.querySelector('.lm-input').getAttribute('aria-label'))"),
          "another language while navigating: the same page, navigation goes on, the route asked in German")
    check(js("(() => { const a = document.querySelector('.lm-menu-privacy'); return !!a && a.textContent === 'Datenschutz' && new URL(a.href).pathname.endsWith('/de/privacy'); })()"),
          "the menu links the privacy policy, in German for German")
    check(js("document.querySelector('.lm-powered')?.href === 'https://overpass.ch/de/'"), "Powered by overpass.ch links overpass.ch, in German for German")
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = ''; s.dispatchEvent(new Event('change')) })(); localStorage.removeItem('hm-lang')")
    check(wait("document.documentElement.lang === 'en' && /search/i.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 10), "Automatic: the language of the browser again")
    check(js("(() => { const a = document.querySelector('.lm-menu-privacy'); return a.textContent === 'Privacy' && /\\/privacy$/.test(new URL(a.href).pathname) && !a.href.includes('/de/'); })()"),
          "the privacy link in English again")
    check(js("document.querySelector('.lm-powered')?.href === 'https://overpass.ch/'"), "Powered by overpass.ch links overpass.ch in English again")
    if js("!!document.getElementById('app').dataset.say"):
        check(wait("performance.getEntriesByType('resource').some(e => e.name.includes('/api/say'))", 20), "the voice of the server speaks")
    # a finger held on the map while navigating: the point and the places around it; the point as a stop
    r = js("(() => { const b = document.querySelector('.maplibregl-canvas').getBoundingClientRect(); return [b.left + b.width * 0.3, b.top + b.height * 0.4]; })()")
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": r[0], "y": r[1]}])
    time.sleep(0.8)
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    check(wait("document.querySelector('.lm-nav-along')?.dataset.kind === 'point' && !document.querySelector('.lm-nav-along').hidden && !document.querySelector('.lm-nav-along .lm-notes')?.textContent.includes('…')", 20),
          "a long press while navigating opens the point")
    js("window.hmRoutes = performance.getEntriesByType('resource').filter(e => e.name.includes('/api/route')).length; document.querySelector('.lm-nav-point-add').click()")
    check(wait("document.querySelector('.lm-nav-along').hidden && performance.getEntriesByType('resource').some((e, i, a) => e.name.includes('/api/route') && e.name.includes('via=') && a.filter(x => x.name.includes('/api/route')).length > window.hmRoutes)", 30)
          and js("document.getElementById('app').classList.contains('lm-navigating')"),
          "the point pressed as a stop: a new route through it, navigation goes on")
    # the magnifying glass in navigation: a field for a place or address above the kinds along the route; a
    # place found as the new destination
    time.sleep(1)
    js("[...document.querySelectorAll('.lm-nav-bottom button')].find(b => b.querySelector('svg path')?.getAttribute('d')?.startsWith('M10.5 17'))?.click()")
    check(wait("!document.querySelector('.lm-nav-along').hidden && !!document.querySelector('.lm-nav-along .lm-nav-find-input') && !!document.querySelector('.lm-nav-along .lm-along-cats')", 5),
          "the search in navigation: a field for a place and the kinds along the route")
    js("document.querySelector('.lm-nav-find-input').focus()")
    call("Input.insertText", text="Kunsthaus Zürich")
    js("(() => { const f = document.querySelector('.lm-nav-find-input'); if (document.activeElement !== f) f.dispatchEvent(new Event('input')); })()")
    check(wait("document.querySelectorAll('.lm-nav-find-list .lm-along-item .lm-along-dest').length > 0 && document.querySelector('.lm-nav-along .lm-along-body').hidden", 20),
          "a place typed in navigation: places found, each a stop or the destination")
    js("window.hmRoutes = performance.getEntriesByType('resource').filter(e => e.name.includes('/api/route')).length; document.querySelector('.lm-nav-find-list .lm-along-dest').click()")
    check(wait("document.querySelector('.lm-nav-along').hidden && performance.getEntriesByType('resource').filter(e => e.name.includes('/api/route')).length > window.hmRoutes", 30)
          and wait("(() => { const r = performance.getEntriesByType('resource').filter(e => e.name.includes('/api/route')).pop(); const to = new URL(r.name).searchParams.get('to').split(',').map(Number);"
                   " return Math.abs(to[0] - 47.370) < 0.01 && Math.abs(to[1] - 8.548) < 0.01; })()", 5)
          and js("document.getElementById('app').classList.contains('lm-navigating')"),
          "a place found in navigation as the destination: a new route to it, navigation goes on")
    js("document.querySelector('.lm-nav-x').click()")
    check(wait("!document.getElementById('app').classList.contains('lm-navigating')", 5), "navigation ends")
    check(js("!Object.hasOwn(hmMap, 'triggerRepaint') && hmMap.triggerRepaint === Object.getPrototypeOf(hmMap).triggerRepaint"),
          "after navigation the map repaints without the cap")
    check(wait("(location.hash.split('/')[4] || '0') === '0'", 5), "the map is flat again after navigation")
    # light and dark from the menu, the route on the map: the new style as a diff of the one shown (the same style object,
    # no new load of the map and its tiles), our layers and the route kept, our names in the colours of the theme
    js("window.hmStyle = hmMap.style")
    for style, colour in (("light", "#7f1d1d"), ("dark", "#ffb4a9"), ("light", "#7f1d1d")):
        js(f"document.querySelector('.lm-style[data-style={style}]').click()")
        check(wait(f"document.documentElement.dataset.theme === '{'dark' if style == 'dark' else 'light'}' && hmMap.getPaintProperty('lm-label', 'text-color') === '{colour}'", 15)
              and js("hmMap.style === hmStyle && !!hmMap.getLayer('lm-route-line') && !!hmMap.getLayer('lm-night') && hmMap.getStyle().sources['lm-route'].data.features.length > 0"),
              f"the {style} style switched in place: our layers and the route kept")
    js("document.querySelector('.lm-style[data-style=auto]').click()")

    # traffic on and off
    if js("!!document.getElementById('app').dataset.traffic"):
        before = js("document.querySelector('.lm-traffic-btn').getAttribute('aria-pressed')")
        js("document.querySelector('.lm-traffic-btn').click()")
        check(js("document.querySelector('.lm-traffic-btn').getAttribute('aria-pressed')") != before, "the traffic button switches")
        check(wait("performance.getEntriesByType('resource').some(e => e.name.includes('/api/traffic'))", 15), "traffic loads")
        js("document.querySelector('.lm-traffic-btn').click()")

    # a tap on the map, then the Directions button: navigation to that point
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    check(wait("getComputedStyle(document.querySelector('.lm-form')).display !== 'none'", 5), "the search bar is back after directions")
    js("location.hash = '#map=17/47.3700/8.5400'")
    time.sleep(1.5)
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": box[0] + 60, "y": box[1] + 40}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    wait("!document.querySelector('.lm-here').hidden || !document.querySelector('.lm-detail').hidden", 10)
    js("document.querySelector('.lm-dir-btn').click()")
    check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && document.querySelector('.lm-nav-go .lm-chip')", 40)
          and not js("document.getElementById('app').classList.contains('lm-navigating')"),
          "a tapped point and the Directions button: the route to it, with Preview and Start")

    # the X clears everything
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'cafés'; i.dispatchEvent(new Event('input')); document.querySelector('.lm-clear').hidden = false })()")
    js("document.querySelector('.lm-clear').click()")
    check(wait("document.querySelector('.lm-input').value === '' && !document.querySelector('.lm-listview').hidden && document.querySelector('.lm-route').hidden", 5),
          "the X clears the search and the views")

    # a search far from where the map is: the map moves to its results (Brugg -> Zurich)
    js("location.hash = '#map=14/47.481/8.211'")
    wait("location.hash.includes('/47.481')", 5)
    time.sleep(1)
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'hotels along the Limmat in Zurich'; document.querySelector('.lm-form').requestSubmit() })()")
    check(wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && /\/47\.3[6-9]/.test(location.hash)", 60),
          "a search far away moves the map to its results")
    # along a river: the band of the radius along it, in metres (wider zoomed in)
    check(wait("!hmMap.isMoving() && hmMap.queryRenderedFeatures({ layers: ['lm-band'] }).some((f) => f.properties.radius > 0)", 20),
          "along the Limmat: the band of the radius along the river")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")

    # near a kind of place: the results, the places they are near (each stop a point) and the circle of the radius around each
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'Migros near a bus stop in Zurich'; document.querySelector('.lm-form').requestSubmit() })()")
    check(wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '')", 60)
          and wait("!hmMap.isMoving() && hmMap.queryRenderedFeatures({ layers: ['lm-target'] }).length > 0"
                   " && hmMap.queryRenderedFeatures({ layers: ['lm-drawn-dashed'] }).some((f) => f.properties.kind === 'circle')"
                   " && hmMap.queryRenderedFeatures({ layers: ['lm-point'] }).length > 0", 20),
          "Migros near a bus stop: the shops, the stops near them and a circle of 300 m around each stop")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")

    # areas found are dots until they are large enough to see (zoom 16); a tap on a dot opens the area
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'church buildings in Zurich'; document.querySelector('.lm-form').requestSubmit() })()")
    check(wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '')", 60)
          and wait("!hmMap.isMoving() && hmMap.getZoom() < 16 && hmMap.queryRenderedFeatures({ layers: ['lm-dot'] }).length > 0", 20),
          "church buildings in Zurich: a dot for each building at the zoom of the city")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")

    # a town: its photo, and no second Directions beside the search
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'Thun'; document.querySelector('.lm-form').requestSubmit(); })()")
    check(wait("document.querySelector('.lm-place-acts .lm-action')", 40) and wait("document.querySelector('.lm-dir-btn').hidden", 3),
          "a place found: Directions under its name only")
    check(wait("!document.querySelector('.lm-place-pic').hidden && document.querySelector('.lm-place-pic img')?.naturalWidth > 0", 30), "a place found: its photo")
    check(js("document.querySelector('.lm-place-pic').getBoundingClientRect().top >= document.querySelector('.lm-place-acts').getBoundingClientRect().bottom - 1"),
          "a place found: its photo under the buttons")
    check(js("(() => { const t = document.querySelector('.lm-title').getBoundingClientRect(), a = document.querySelector('.lm-place-acts').getBoundingClientRect();"
             " return a.top >= t.bottom - 1; })()"), "a place found: its buttons under its name")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    check(wait("!document.querySelector('.lm-dir-btn').hidden", 3), "the Directions button back after the X")

    # a suggestion picked by a finger (down and up on it): the click after lands on the map (the keyboard went) and opens nothing there
    js("document.querySelector('.lm-input').focus()")
    call("Input.insertText", text="Bern")
    wait("[...document.querySelectorAll('.lm-suggest .lm-opt-place')].some(o => /^Bern/.test(o.textContent))", 20)
    spot = js("(() => { for (let y = 5; y < 400; y += 5) for (const x of [40, 120, 195, 270, 350]) { const e = document.elementFromPoint(x, y);"
              " if (e?.classList.contains('maplibregl-canvas')) return [x, y]; } return null; })()")
    js("(() => { const o = [...document.querySelectorAll('.lm-suggest .lm-opt-place')].find(o => /^Bern/.test(o.textContent));"
       " for (const t of ['pointerdown', 'pointerup']) o.dispatchEvent(new PointerEvent(t, { bubbles: true, cancelable: true })); })()")
    for t in ("mousePressed", "mouseReleased") if spot else ():
        call("Input.dispatchMouseEvent", type=t, x=spot[0], y=spot[1], button="left", clickCount=1)
    check(wait("/Bern/.test(document.querySelector('.lm-title')?.textContent || '')", 40) and not wait("!document.querySelector('.lm-here').hidden", 2),
          "a suggestion tapped opens it, not the map under the finger")
    # Back to no search: the panel down to the search box
    for _ in range(6):
        js("history.back()")
        if wait("!location.search", 4):
            break
    check(wait("document.querySelector('.lm-input').value === '' && document.getElementById('app').dataset.sheet === 'peek'", 10),
          "Back to no search: the panel down to the search box")

    # a point tapped with no search, then its <: nothing under the empty box, the panel folds
    js("location.hash = '#map=17/47.3700/8.5400'")
    time.sleep(1.5)
    js("document.querySelector('.lm-input').blur()")
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": 120, "y": 300}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    if wait("!document.querySelector('.lm-here').hidden || !document.querySelector('.lm-detail').hidden", 10):
        for _ in range(3):  # its < (or its X) until the list
            js("(document.querySelector('.lm-here:not([hidden]) .lm-back, .lm-detail:not([hidden]) .lm-back') || document.querySelector('.lm-here:not([hidden]) .lm-card-x, .lm-detail:not([hidden]) .lm-card-x'))?.click()")
            if wait("!document.querySelector('.lm-listview').hidden", 2):
                break
    check(wait("!document.querySelector('.lm-listview').hidden && document.getElementById('app').dataset.sheet === 'peek'", 10),
          f"< back to nothing under the empty search box: the panel folds ({js('document.getElementById(\'app\').dataset.sheet')})")

    # the empty search box: an example of a search, another one after the X
    first = js("document.querySelector('.lm-input').placeholder")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    check(js(f"document.querySelector('.lm-input').placeholder !== {json.dumps(first)} && document.querySelector('.lm-input').placeholder.length > 0"),
          "the empty search box: another example after the X")
    js("document.querySelector('.lm-input').blur()")

    # a place starred: in the empty search box, and on the map; the last search there too
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'Thun'; document.querySelector('.lm-form').requestSubmit(); })()")
    wait("document.querySelector('.lm-place-acts .lm-star')", 40)
    js("document.querySelector('.lm-place-acts .lm-star').click()")
    check(js("document.querySelector('.lm-place-acts .lm-star').getAttribute('aria-pressed') === 'true' && JSON.parse(localStorage.getItem('hm-stars') || '[]').some(s => s.name === 'Thun')"),
          "a place starred and kept")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    js("document.querySelector('.lm-input').focus()")
    check(wait("[...document.querySelectorAll('.lm-suggest:not([hidden]) .lm-opt-star .lm-opt-label')].some(e => e.textContent === 'Thun') && [...document.querySelectorAll('.lm-suggest:not([hidden]) .lm-opt-recent .lm-opt-label')].some(e => e.textContent === 'Thun')", 5),
          "the empty search box offers the starred place and the last search")
    js("document.querySelector('.lm-input').blur()")

    # the bubbles: a search of the map view; Transit switches its lines
    js("[...document.querySelectorAll('.lm-bubble')].find(b => /Restaurants/.test(b.textContent))?.click()")
    check(wait("/\\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelector('.lm-input').value === 'Restaurants'", 60),
          "the Restaurants bubble searches the map view")
    # in another language, a bubble searches the words it shows (Ristoranti, not restaurants)
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = 'it'; s.dispatchEvent(new Event('change')) })()")
    js("[...document.querySelectorAll('.lm-bubble')].find(b => b.textContent === 'Ristoranti')?.click()")
    check(wait("/\\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelector('.lm-input').value === 'Ristoranti'", 60),
          "in Italian the bubble searches Ristoranti, the word it shows")
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = ''; s.dispatchEvent(new Event('change')) })(); localStorage.removeItem('hm-lang')")
    if js("!!document.getElementById('app').dataset.transit"):
        js("document.querySelector('.lm-bubble-transit').click()")
        check(js("document.querySelector('.lm-bubble-transit').getAttribute('aria-pressed') === 'true'"), "the Transit bubble switches the lines on")
        js("document.querySelector('.lm-bubble-transit').click()")
        # a city of its lines
        js("(() => { const i = document.querySelector('.lm-input'); i.value = 'metro lines in Lausanne'; document.querySelector('.lm-form').requestSubmit(); })()")
        check(wait("[...document.querySelectorAll('.lm-transit-ref')].some(e => /^m2$/i.test(e.textContent)) && ![...document.querySelectorAll('.lm-transit-ref')].some(e => /^(10|11)$/.test(e.textContent))", 60),
              "metro lines in Lausanne: its lines (m2), none of Zurich")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    js("document.querySelector('.lm-input').blur()")

    # the X of a card: the card and its highlight go
    js("location.hash = '#map=16/47.3700/8.5400'")
    time.sleep(1.5)
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": 200, "y": 200}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    wait("!document.querySelector('.lm-here').hidden || !document.querySelector('.lm-detail').hidden", 10)
    js("document.querySelector('.lm-here:not([hidden]) .lm-card-x, .lm-detail:not([hidden]) .lm-card-x')?.click()")
    check(wait("document.querySelector('.lm-here').hidden && document.querySelector('.lm-detail').hidden", 5), "the X closes the card of a point")
    # day and night follow the visitor, not the map: the style stays when the map goes to New York
    theme0 = js("document.documentElement.dataset.theme")
    js("location.hash = '#map=12/40.7128/-74.0060'")
    time.sleep(3)
    check(js("document.documentElement.dataset.theme") == theme0, "day and night stay with the visitor when the map goes far away")

    # out far, the searches of the bubbles go (Transit stays); close in, they are back
    js("location.hash = '#map=8/47.0/8.0'")
    check(wait("document.querySelector('.lm-bubbles').classList.contains('lm-far') && [...document.querySelectorAll('.lm-bubble')].every(b => b.classList.contains('lm-bubble-transit') || !b.offsetParent)", 10),
          "the map out far: no search bubbles")
    js("location.hash = '#map=14/47.3769/8.5417'")
    check(wait("!document.querySelector('.lm-bubbles').classList.contains('lm-far') && [...document.querySelectorAll('.lm-bubble')].some(b => !b.classList.contains('lm-bubble-transit') && b.offsetParent)", 10),
          "the map close in: the search bubbles back")
    # the 2D/3D button: the terrain without the buildings; the buildings are the switch of the menu
    js("performance.clearResourceTimings()")
    js("document.querySelector('.lm-dim').click()")
    check(wait("performance.getEntriesByType('resource').some(e => /styles\\/.*terrain=1/.test(e.name) && !/buildings=1/.test(e.name))", 15),
          "the 3D button: terrain, not buildings")
    js("document.querySelector('.lm-dim').click()")
    # POIs before their zoom: at 14.3 the tile of POIs is put together from tiles of zoom 15
    js("location.hash = '#map=14.3/47.3717/8.5390'")
    check(wait("performance.getEntriesByType('resource').some(e => /maps\\/vector\\/15\\//.test(e.name))", 30), "POIs at zoom 14 from the tiles of zoom 15")

    # shared links: a place, a route
    call("Page.navigate", url=URL + "/?at=46.94797,7.44745&n=Zytglogge")
    # (the point opens; when a place of that name is there, its page follows)
    check(wait("[...document.querySelectorAll('.lm-here:not([hidden]) .lm-dname, .lm-detail:not([hidden]) .lm-dname')].some(e => /Zytglogge/.test(e.textContent))"
               " && !!document.querySelector('.lm-here:not([hidden]) .lm-share, .lm-detail:not([hidden]) .lm-share')", 60),
          "a shared place opens with its name and Share")
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Zurich+HB&to=47.36690,8.54500&tn=Bellevue&mode=walk")
    check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && /Zurich HB/.test(document.querySelector('.lm-end-from input')?.value || '') && document.querySelector('.lm-mode[aria-checked=true]')?.textContent.includes('Walk')", 40),
          "a shared route opens with its start, destination and mode")
    check(js("!!document.querySelector('.lm-nav-go .lm-share')"), "Share beside Start")
    # arrival by bike with Start: fixes stop 40 m short of the end of the route and 10 m beside it
    # (no fix within the reach of 30 m): "You have arrived" said once, then navigation ends by itself
    call("Page.addScriptToEvaluateOnNewDocument", source=SAID_HOOK)
    a, b = (47.37000, 8.54000), (47.37300, 8.54300)
    line = json.load(urllib.request.urlopen(f"{URL}/api/route?from={a[0]},{a[1]}&to={b[0]},{b[1]}&mode=bike&lang=en", timeout=30))["line"]

    def short_of(m, side=0.0):  # the point m metres before the end of the line, side metres to its left
        kx, ky = 111320 * math.cos(math.radians(line[-1][1])), 111320
        left = m
        for i in range(len(line) - 1, 0, -1):
            (x0, y0), (x1, y1) = line[i - 1], line[i]
            dx, dy = (x0 - x1) * kx, (y0 - y1) * ky
            seg = math.hypot(dx, dy)
            if seg >= left or i == 1:
                t = min(1, left / seg) if seg else 0
                return y1 + (dy * t - dx / (seg or 1) * side) / ky, x1 + (dx * t + dy / (seg or 1) * side) / kx
            left -= seg
    p = short_of(100)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=15)
    call("Page.navigate", url=URL + f"/?from={a[0]},{a[1]}&to={b[0]},{b[1]}&mode=bike")
    wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40)
    js("document.querySelector('.lm-nav-start').click()")
    wait("document.getElementById('app').classList.contains('lm-navigating')", 5)
    for m, side in [(85, 0), (70, 0), (55, 5), (45, 10)] + [(40, 10)] * 8:
        time.sleep(1)
        p = short_of(m, side)
        call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=15)
    check(wait("/arrived/i.test(document.querySelector('.lm-nav-banner')?.textContent || '')", 10), "by bike, stopped 40 m short: You have arrived")
    check(wait("!document.getElementById('app').classList.contains('lm-navigating')", 25), "after the arrival navigation ends by itself")
    said = js("window.hmSaid || []")
    check(len([t for t in said if re.search(r"have arrived", t, re.I)]) == 1, f"the arrival said once ({said[-3:]})")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    # a stop on the way, and the places along the route
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Zurich+HB&via=47.05020,8.30930&vn=Lucerne&to=46.94900,7.43910&tn=Bern&mode=car")
    check(wait("document.querySelectorAll('.lm-end-stop').length === 1 && document.querySelectorAll('.lm-step-stop').length === 1", 40),
          "a shared route with a stop: its field and its step")
    if js("!!document.querySelector('.lm-along-cats')"):
        js("document.querySelector('.lm-along-cats [data-cat=rest]').click()")
        check(wait("document.querySelectorAll('.lm-along-item').length > 3", 30), "rest areas along the route")
        js("document.querySelector('.lm-along-item .lm-along-add').click()")
        check(wait("document.querySelectorAll('.lm-end-stop').length === 2 && document.querySelectorAll('.lm-step-stop').length === 2", 40),
              "a rest area added as a stop: the route by both")
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")

    # a ride by bike with the location of the device: fixes every 5 m at 5 m/s (five a second, to be quick), a
    # little noise. The sentences spoken (the texts of /api/say, answered here with silence) with the
    # distance on the banner: a turn far enough from the one before is said ahead ("In 150 meters, turn
    # left …", at about that distance), then at the turn (40 m or nearer); no sentence twice in a row.
    BIKE = ((47.37550, 8.53050), (47.37000, 8.52700))  # St. Jakobstrasse to Hallwylstrasse: 770 m, two turns 200 m and more apart
    call("Emulation.setGeolocationOverride", latitude=BIKE[0][0], longitude=BIKE[0][1], accuracy=8)
    js("window.hmOld = 1")  # (the page before must not count as the new one)
    call("Page.navigate", url=URL + f"/?from={BIKE[0][0]},{BIKE[0][1]}&fn=A&to={BIKE[1][0]},{BIKE[1][1]}&tn=B&mode=bike")
    if wait("!window.hmOld && document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40):
        call("Fetch.enable", patterns=[{"urlPattern": "*/api/say*"}])
        # what is played: the text of each sound of /api/say (or of the voice of the browser), with the banner
        js("""(() => { window.hmSaid = []; const text = new WeakMap(), byUrl = {}, f = window.fetch;
          window.fetch = function (u, ...a) { const p = f.call(this, u, ...a); if (!String(u).includes('/api/say')) return p;
            const t = new URL(u, location).searchParams.get('text');
            return p.then((r) => { const b = r.blob.bind(r); r.blob = () => b().then((x) => { text.set(x, t); return x; }); return r; }); };
          const co = URL.createObjectURL; URL.createObjectURL = function (b) { const u = co.call(this, b); if (text.has(b)) byUrl[u] = text.get(b); return u; };
          const dist = () => document.querySelector('.lm-nav-dist')?.textContent || '';
          const play = HTMLMediaElement.prototype.play; HTMLMediaElement.prototype.play = function () { if (byUrl[this.src]) hmSaid.push([byUrl[this.src], dist()]); return play.call(this); };
          if (window.speechSynthesis) { const sp = speechSynthesis.speak.bind(speechSynthesis); speechSynthesis.speak = (u) => { if (u.text.trim()) hmSaid.push([u.text, dist()]); return sp(u); }; }
        })()""")
        line = json.load(urllib.request.urlopen(f"{URL}/api/route?from={BIKE[0][0]},{BIKE[0][1]}&to={BIKE[1][0]},{BIKE[1][1]}&mode=bike&lang=en", timeout=30))["line"]
        pts, carry = [], 0.0
        for a, b in zip(line, line[1:]):
            dx, dy = (b[0] - a[0]) * 111320 * math.cos(math.radians(a[1])), (b[1] - a[1]) * 111320
            seg, x = math.hypot(dx, dy), carry
            while x < seg:
                pts.append((a[1] + (b[1] - a[1]) * x / seg, a[0] + (b[0] - a[0]) * x / seg, math.degrees(math.atan2(dx, dy)) % 360))
                x += 5
            carry = x - seg
        js("document.querySelector('.lm-nav-start').click()")
        check(wait("document.getElementById('app').classList.contains('lm-navigating')", 5), "by bike: Start navigates")
        rnd = random.Random(1)
        for lat, lon, h in pts:
            call("Emulation.setGeolocationOverride", latitude=lat + rnd.gauss(0, 2) / 111320, longitude=lon + rnd.gauss(0, 2) / 111320 / math.cos(math.radians(lat)),
                 accuracy=8, heading=h, speed=5 + rnd.gauss(0, 0.3))
            time.sleep(0.2)
        time.sleep(1)
        said = js("window.hmSaid") or []
        print("  spoken: " + " | ".join(f"{d} {t}" for t, d in said), flush=True)
        # the distance on the banner in metres; none when arrived (the banner without a distance)
        metres = lambda d: float(d.split()[0]) * (1000 if d.endswith("km") else 1) if re.match(r"^[\d.]+ k?m$", d) else 0 if d == "" else None
        ahead = [(i, int(m.group(1)), metres(d)) for i, (t, d) in enumerate(said) for m in [re.match(r"In (\d+) meters, ", t)] if m]
        check(len(ahead) >= 1, "by bike, a turn said ahead with its distance (In N meters, …)")
        check(all(d is not None and n - 20 <= d <= n + 10 for _, n, d in ahead), "said ahead at about that distance")
        # the sentence after one said ahead, past "Continue for …": the turn itself, near it
        near = [next(((t, metres(d)) for t, d in said[i + 1:] if not t.startswith("Continue")), (None, None)) for i, _, _ in ahead]
        check(bool(near) and all(t and not t.startswith("In ") and d is not None and d <= 40 for t, d in near), "then the turn itself, 40 m or nearer")
        check(all(a[0] != b[0] for a, b in zip(said, said[1:])), "no sentence twice in a row")
        call("Fetch.disable")
        js("document.querySelector('.lm-nav-x')?.click()")
    else:
        check(False, "a route by bike for the ride")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)

    # the panel on a phone: empty, it stays down after a drag up; a finger moved on the suggestions
    # neither picks one nor moves the map, a tap picks; the keyboard gone, directions back at their top
    def drag(x, y0, y1, steps=8):
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": x, "y": y0}])
        for i in range(1, steps + 1):
            call("Input.dispatchTouchEvent", type="touchMove", touchPoints=[{"x": x, "y": y0 + (y1 - y0) * i / steps}])
            time.sleep(0.03)
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    call("Page.navigate", url=URL)
    wait("window.hmMap?.loaded() && document.getElementById('app').dataset.sheet === 'peek'", 30)
    time.sleep(1)
    b = js("(() => { const r = document.querySelector('.lm-side').getBoundingClientRect(); return [r.left + r.width / 2, r.top + 10]; })()")
    drag(b[0], b[1], b[1] - 300)
    time.sleep(0.6)
    check(js("document.getElementById('app').dataset.sheet === 'peek'"), "the empty panel stays down after a drag up")
    js("(() => { const i = document.querySelector('.lm-input'); i.focus(); i.value = 'Bahnhofstrasse'; i.dispatchEvent(new Event('input', {bubbles: true})); })()")
    if wait("document.querySelectorAll('.lm-suggest:not([hidden]) [role=option]').length > 0", 15):
        L = js("(() => { const r = document.querySelector('.lm-suggest:not([hidden]) [role=option]').getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()")
        c0 = js("JSON.stringify(hmMap.getCenter())")
        drag(L[0], L[1], L[1] - 60, 5)
        time.sleep(0.6)
        check(js(f"!!document.querySelector('.lm-suggest:not([hidden])') && document.querySelector('.lm-input').value === 'Bahnhofstrasse' && JSON.stringify(hmMap.getCenter()) === {json.dumps(c0)}"),
              "a finger moved on the suggestions picks none and leaves the map")
        tap(".lm-suggest:not([hidden]) [role=option]")
        check(wait("document.querySelector('.lm-input').value !== 'Bahnhofstrasse' || !document.querySelector('.lm-suggest:not([hidden])')", 5), "a tap on a suggestion picks it")
    else:
        check(False, "suggestions for Bahnhofstrasse")
    call("Page.navigate", url=URL + "/?to=47.36690,8.54500&tn=Bellevue&mode=car")
    wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40)
    call("Emulation.setDeviceMetricsOverride", width=390, height=480, deviceScaleFactor=2, mobile=True)  # the keyboard up
    js("document.querySelector('.lm-end-to input').focus()")
    time.sleep(0.4)
    js("document.querySelector('.lm-end-to input').scrollIntoView({block: 'start'})")  # (the field at the top of the full panel: scrolled to the top of it)
    time.sleep(0.5)
    scrolled = js("document.querySelector('.lm-body').scrollTop > 20")
    js("document.querySelector('.lm-end-to input').blur()")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)  # the keyboard down
    check(scrolled and wait("document.querySelector('.lm-body').scrollTop === 0 && document.querySelector('.lm-modes').getBoundingClientRect().top >= document.querySelector('.lm-body').getBoundingClientRect().top", 5),
          "the keyboard gone: directions back at their top, car, bike and walk in view")

    # the panel moves in one direction when the keyboard comes or goes (the app resizes the page,
    # as here): its top in each frame of the page, never back. The keyboard comes just after the
    # focus, and goes just after the field loses it.
    def tops(ms):  # the top of the panel in each frame from now, in window.hmTops
        js("(() => { const s = document.querySelector('.lm-side'), t0 = performance.now(); window.hmTops = [];"
           f" (function f() {{ hmTops.push(Math.round(s.getBoundingClientRect().top)); if (performance.now() - t0 < {ms}) requestAnimationFrame(f); }})(); }})()")

    def keyboard(up):
        call("Emulation.setDeviceMetricsOverride", width=390, height=544 if up else 844, deviceScaleFactor=2, mobile=True)

    def keyboard_goes(field):  # once the field lost the focus
        wait(f"document.activeElement !== document.querySelector({json.dumps(field)})", 2)
        time.sleep(0.08)
        keyboard(False)

    def one_way(up):  # the tops of the frames: up only, or down only (a pixel of rounding)
        t = js("window.hmTops") or []
        ok = len(t) > 3 and all((b <= a + 1) if up else (b >= a - 1) for a, b in zip(t, t[1:]))
        return ok, t[::max(1, len(t) // 12)]

    sheet = lambda: js("document.getElementById('app').dataset.sheet")

    # the moves of the map from now, in window.hmMoves: start and end in ms, zoom at both ends and the
    # lowest between; window.hmResized: when the page last changed its height (the keyboard)
    def moves():
        js("(() => { window.hmMoves = []; let m = null; window.hmResized = 0; addEventListener('resize', () => { hmResized = performance.now(); });"
           " const at = () => JSON.stringify([hmMap.getCenter(), hmMap.getZoom()]);"
           " hmMap.on('movestart', () => { const z = hmMap.getZoom(); m = { t0: performance.now(), z0: z, zmin: z, c0: at() }; hmMoves.push(m); });"
           " hmMap.on('move', () => { if (m) m.zmin = Math.min(m.zmin, hmMap.getZoom()); });"
           " hmMap.on('moveend', () => { if (m) { m.t1 = performance.now(); m.z1 = hmMap.getZoom(); m.same = at() === m.c0; m = null; } }); })()")

    call("Page.navigate", url=URL)
    wait("window.hmMap?.loaded() && document.getElementById('app').dataset.sheet === 'peek'", 30)
    time.sleep(1)
    tops(1500)
    tap(".lm-input")
    time.sleep(0.1)
    keyboard(True)
    time.sleep(1.7)
    ok, t = one_way(True)
    check(ok and sheet() == "full" and abs(js("document.querySelector('.lm-side').getBoundingClientRect().top") - 56) <= 2,
          f"the search box tapped, the keyboard comes: the panel up to the top in one movement ({t})")
    call("Input.insertText", text="Bern")
    js("(() => { const i = document.querySelector('.lm-input'); if (document.activeElement !== i) { i.focus(); i.dispatchEvent(new Event('input')); } })()")
    if wait("[...document.querySelectorAll('.lm-suggest:not([hidden]) .lm-opt-place')].some(o => /^Bern/.test(o.textContent))", 20):
        time.sleep(0.3)
        # the answer a second later, as on a slow network: the keyboard goes first
        js("(() => { const f0 = window.fetch; window.fetch = function (u) { const p = f0.apply(this, arguments);"
           " return /\\/api\\/search\\/(query|search)/.test(String(u)) ? p.then((r) => new Promise((ok) => setTimeout(() => ok(r), 1000))) : p; }; })()")
        tops(3500)
        moves()
        tap(".lm-suggest:not([hidden]) .lm-opt-place")
        keyboard_goes(".lm-input")
        time.sleep(3.6)
        ok, t = one_way(False)
        check(ok and wait("/Bern/.test(document.querySelector('.lm-title')?.textContent || '')", 30),
              f"a suggestion picked, the keyboard goes: the panel down in one movement ({t})")
        # Zürich to Bern: one flight out and in, after the keyboard went, no correction after it
        wait("hmMoves.length > 0 && hmMoves.every(m => m.t1)", 5)
        time.sleep(0.5)
        m = js("JSON.stringify({ moves: hmMoves.filter(m => !m.same).map(m => ({ ms: Math.round(m.t1 - m.t0), out: +(Math.min(m.z0, m.z1) - m.zmin).toFixed(2), after: Math.round(m.t0 - hmResized) })) })")
        mv = json.loads(m)["moves"]
        check(len(mv) == 1 and mv[0]["after"] >= 0 and 800 <= mv[0]["ms"] <= 1700,
              f"a far place: the map moves once, after the keyboard, a flight of 0.9 to 1.5 s ({mv})")
    else:
        check(False, "suggestions for Bern")
    call("Page.navigate", url=URL + "/?to=47.36690,8.54500&tn=Bellevue&mode=car")
    wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40)
    time.sleep(1)
    tops(1500)
    tap(".lm-end-to input")
    time.sleep(0.1)
    keyboard(True)
    time.sleep(1.7)
    ok, t = one_way(True)
    check(ok and sheet() == "full" and abs(js("document.querySelector('.lm-side').getBoundingClientRect().top") - 56) <= 2,
          f"a field of directions tapped: the panel up to the top in one movement ({t})")
    call("Input.insertText", text="Paradeplatz Zürich")
    js("(() => { const f = document.querySelector('.lm-end-to input'); if (document.activeElement !== f) { f.focus(); f.dispatchEvent(new Event('input')); } })()")
    if wait("[...document.querySelectorAll('.lm-end-list:not([hidden]) .lm-opt-label')].some(e => /Paradeplatz/.test(e.textContent))", 20):
        time.sleep(0.3)
        tops(2500)
        tap(".lm-end-list:not([hidden]) .lm-opt")
        keyboard_goes(".lm-end-to input")
        time.sleep(2.6)
        ok, t = one_way(False)
        check(ok and sheet() == "half" and wait("/Paradeplatz/.test(document.querySelector('.lm-end-to input').value)", 5),
              f"a place picked in directions, the keyboard goes: the panel down to half in one movement ({t})")
    else:
        check(False, "suggestions for Paradeplatz in directions")
    tap(".lm-end-from input")
    time.sleep(0.1)
    keyboard(True)
    time.sleep(1)
    tops(2000)
    tap(".lm-route-sum")
    keyboard_goes(".lm-end-from input")
    time.sleep(2.1)
    ok, t = one_way(False)
    check(ok and sheet() == "half", f"a tap away from a field of directions: the panel down in one movement ({t})")
    keyboard(False)

    # German
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = 'de'; s.dispatchEvent(new Event('change')) })()")
    time.sleep(1)
    check(wait("document.documentElement.lang === 'de' && /suchen/.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 30), "the page in German")
    # the words of the search in German too: what it understood, the other readings, the kinds and days of the places
    js("location.hash = '#map=16/47.3720/8.5390'")
    time.sleep(1.5)
    js("document.querySelector('.lm-input').blur(); [...document.querySelectorAll('.lm-bubble')].find(b => /Restaurants/.test(b.textContent))?.click()")
    if wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelectorAll('.lm-chiprow .lm-chip').length > 0", 60):
        side = js("document.querySelector('.lm-title').textContent + ' | ' + [...document.querySelectorAll('.lm-chiprow, .lm-results .lm-card-sub, .lm-results [data-hours]')].map(e => e.textContent).join(' | ')")
        english = re.findall(r"in the view|Or search for|Sushi Fast Food|Outdoor Seating|Food Court|\b(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun) \d", side)
        check("Kartenausschnitt" in side and "Schnellimbiss" in side and not english, f"the Restaurants bubble in German: German words ({english or side[:120]})")
        # a place of the list, in the view: a short glide, no flight out
        time.sleep(1.5)
        moves()
        js("document.querySelector('.lm-results .lm-card').click()")
        wait("hmMoves.length > 0 && hmMoves.every(m => m.t1)", 5)
        time.sleep(0.5)
        mv = json.loads(js("JSON.stringify(hmMoves.filter(m => !m.same).map(m => ({ ms: Math.round(m.t1 - m.t0), out: +(Math.min(m.z0, m.z1) - m.zmin).toFixed(2) })))"))
        check(len(mv) == 1 and mv[0]["ms"] <= 600 and mv[0]["out"] <= 0.05, f"a near place: one short glide, no flight out ({mv})")
    else:
        check(False, "the Restaurants bubble in German: no results")
    js("localStorage.removeItem('hm-lang')")

    real = [e for e in errors if "Map cannot fit" not in e]
    check(not real, f"no errors of the page{': ' + real[0] if real else ''}")
finally:
    chrome.terminate()
    shutil.rmtree(prof, ignore_errors=True)
sys.exit(1 if fails else 0)
