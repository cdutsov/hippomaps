#!/usr/bin/env python3
"""hippomaps in headless Chromium, as a visitor uses it (tests/site-test.sh with BROWSER=1: after a deploy, nightly).

    python3 tests/browser.py https://hippomaps.ch

Prints "ok <what>" per check and "FAIL <what>" per failure; exits 1 on a failure. The browser is
in Zurich (a granted location) on a phone screen. Checks: the map starts at the location; the
Directions button opens start and destination, a typed destination gives a route by car with its
time; Preview navigates: the banner, the speed now, the speed limit, the traveller in 3D, the spoken
sentences of the server; the language changed while navigating (no new page); a long press while
navigating (the point and the places around, the point as a stop); the traffic button; the map drawn at 2 pixels per point at most, our sources with tiles to zoom 14 only, the
banner kept between updates of one turn, dark and light switched in place (our layers and the route kept, no new load of the map); a stop and the places along a
route; navigation drawn at 30 frames a second at most, the camera following, tilted 50 degrees; arrival by bike with Start and fixes that stop short of the end (said once, navigation
ends); the X clears everything; a ride by bike with the location of the device (fixes at 5 m/s): each turn far
enough from the one before said ahead with its distance ("In 150 meters, …"), then at the turn, no sentence
twice (the voice answered with silence in the test); German texts after the language of the menu; and no
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
    n, errors, said_n = [0], [], [0]

    def call(method, **params):
        n[0] += 1
        ws.send(json.dumps({"id": n[0], "method": method, "params": params}))
        while True:
            m = json.loads(ws.recv())
            if m.get("method") == "Runtime.exceptionThrown":
                errors.append(m["params"]["exceptionDetails"].get("exception", {}).get("description", "")[:200])
            if m.get("method") == "Fetch.requestPaused":  # the voice, answered here with a short silence (no load on Piper)
                ws.send(json.dumps({"id": 10**7 + n[0], "method": "Fetch.fulfillRequest", "params": {"requestId": m["params"]["requestId"],
                    "responseCode": 200, "responseHeaders": [{"name": "Content-Type", "value": "audio/wav"}], "body": SILENCE}}))
                said_n[0] += 1
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
    box = js("(() => { const r = document.querySelector('.lm-map').getBoundingClientRect(); return [r.left + r.width / 2, r.top + 150]; })()")
    z0 = js("+location.hash.split('/')[0].slice(5)")
    for _ in range(2):
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": box[0], "y": box[1]}])
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        time.sleep(0.08)
    time.sleep(1.5)
    check(js("document.querySelector('.lm-here').hidden && document.querySelector('.lm-detail').hidden") and js(f"+location.hash.split('/')[0].slice(5) > {z0}"),
          "a double tap zooms in and opens nothing")
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
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = ''; s.dispatchEvent(new Event('change')) })(); localStorage.removeItem('hm-lang')")
    check(wait("document.documentElement.lang === 'en' && /search/i.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 10), "Automatic: the language of the browser again")
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

    # a suggestion picked by a finger: the finger lifting lands on the map (the keyboard went) and opens nothing there
    js("document.querySelector('.lm-input').focus()")
    call("Input.insertText", text="Bern")
    wait("[...document.querySelectorAll('.lm-suggest .lm-opt-place')].some(o => /^Bern/.test(o.textContent))", 20)
    spot = js("(() => { for (let y = 5; y < 400; y += 5) for (const x of [40, 120, 195, 270, 350]) { const e = document.elementFromPoint(x, y);"
              " if (e?.classList.contains('maplibregl-canvas')) return [x, y]; } return null; })()")
    js("[...document.querySelectorAll('.lm-suggest .lm-opt-place')].find(o => /^Bern/.test(o.textContent))"
       ".dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true }))")
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
    check(wait("/\\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelector('.lm-input').value === 'restaurants'", 60),
          "the Restaurants bubble searches the map view")
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

    # German
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = 'de'; s.dispatchEvent(new Event('change')) })()")
    time.sleep(1)
    check(wait("document.documentElement.lang === 'de' && /suchen/.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 30), "the page in German")
    js("localStorage.removeItem('hm-lang')")

    real = [e for e in errors if "Map cannot fit" not in e]
    check(not real, f"no errors of the page{': ' + real[0] if real else ''}")
finally:
    chrome.terminate()
    shutil.rmtree(prof, ignore_errors=True)
sys.exit(1 if fails else 0)
