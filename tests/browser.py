#!/usr/bin/env python3
"""hippomaps in headless Chromium, as a visitor uses it (tests/site-test.sh with BROWSER=1: after a deploy, nightly).

    python3 tests/browser.py https://hippomaps.ch

Prints "ok <what>" per check and "FAIL <what>" per failure; exits 1 on a failure. The browser is
in Zurich (a granted location) on a phone screen. Checks: without the location the map opens at the place of the IP
address (of the machine of the test), without a credit of DB-IP on the map; the map starts at the location; a new visit
at the last location of the device (kept on it to about 100 m), a location far from it jumped to; the
Directions button opens start and destination, a typed destination gives a route by car with its
time; Preview navigates: the banner, the speed now, the speed limit, the traveller in 3D, the spoken
sentences of the server; the language changed while navigating (no new page); the language of directions and voice apart from
that of the page (Bulgarian); a long press while
navigating (the point and the places around, the point as a stop); the destination of a route dragged with a finger; the traffic button; the map drawn at 2 pixels per point at most, our sources with tiles to zoom 14 only, the
banner kept between updates of one turn, many lanes in the width of the top with the sign under them, the arrows of the turns ahead
over the line, pinches out of tilted navigation (the tilt after the gesture), dark and light switched in place (our layers and the route kept, no new load of the map); a stop and the places along a
route; navigation drawn at 30 frames a second at most, the camera following, tilted 50 degrees, the next turn right under the banner; arrival by bike with Start and fixes that stop short of the end (said once, navigation
ends); the X clears everything; the panel of a phone (empty: stays down; the suggestions scroll, a tap picks;
directions at their top when the keyboard goes; up and down in one movement when the keyboard comes and goes, a field of
directions takes it to the top); a place searched in navigation as the destination; a ride by bike with the location of the device (fixes at 5 m/s): each turn far
enough from the one before said ahead with its distance ("In 150 meters, …"), then at the turn, no sentence
twice (the voice answered with silence in the test); German texts after the language of the menu, also the words of
the search; the search box on one or two lines, Enter searching; the list of results after a move of the map: those in view, the map keeps all; the results as CSV and GeoJSON; without a connection the note over the map, a search, navigation off the route (no new route asked), the
map of a route saved at Start and shown from the device, the page opening (the service worker); the suggestions of
directions in their field, the panel at the height of its state when the keyboard goes; the signs along a route by car
(speed cameras in the Netherlands, none in Switzerland, stop signs in Berkeley) drawn beside it; the search box without the
focus at the start on a phone, with it on a computer (no list until a click); a place written in the search moved to the
destination by Directions; the dot of the location and the map on it following the visitor; a route on a small phone with
car, bike and walk and Start in view; the POIs of every kind with a name at the largest zooms; the popup of a POI (under the
mouse, clicked, tapped on a phone); the picture and menu of a restaurant from its website (its page, its card when found by name); fewer places at the middle zooms; the start: what the search reads with two searches, and searches of the map view (a river of
Zurich in them, a row under the box on a phone, a list on a computer, in German too), one tapped searches; the location blocked (the button says so, Directions asks for the start, the last location forgotten); the map
on the visitor staying for directions and for a search; the back button of the browser (a result, the list, the start) and forward; the compass of
the app for Android (a stand-in): the cone at the dot, the map turning with the phone from the button of the location and on the dot as it moves,
north up again, a finger ending it, the page hidden a while and back, none in navigation; the compass of a phone browser (Chrome with a
virtual orientation sensor: flat, held up, tilted, landscape, from true north; Safari with a stand-in: asked from a tap, turning with its
first heading, refused), none on a computer; a shared route with
9 stops (the first 8), a stop removed; other routes by car (a row under Start, their times in pills on the map, one picked from the row and
from a tap on its line; none with a stop); a business tapped in directions (its page back to Directions, as the
destination, a stop or the start); Preview of a short route to its end (arrived, navigation ends, the route and Start again); a route with a ferry on a small phone (Start in view); a shared place above the panel;
coordinates in the search box; the POIs with a name in the Outdoor style; public transport (a shared journey from Zurich to
Bern: its connections, the legs of the first, the rides and walks on the map, Later, an arrival by a time on the clocks of the
journey, back to the car; a start without a timetable); feedback (the form of the menu, a note of OpenStreetMap for a
mistake of the map, a message sent with and without the details of the page, in German, Report this problem under a search
that failed, none under a message that says what to do); and no errors
of the page on the way. Needs chromium and websocket-client.
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
import urllib.parse
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
    # a first visit, the location not given: the map opens at the place of the IP address; without a place (no database,
    # or a private address) in Zurich; the map never credits DB-IP (the privacy policy does)
    def drag(x, y0, y1, steps=8):
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": x, "y": y0}])
        for i in range(1, steps + 1):
            call("Input.dispatchTouchEvent", type="touchMove", touchPoints=[{"x": x, "y": y0 + (y1 - y0) * i / steps}])
            time.sleep(0.03)
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    def first_visit(what):
        call("Page.navigate", url=URL)
        wait("window.hmMap?.loaded()", 30)
        near = js("document.getElementById('app')?.dataset.near || ''")
        lat, lon, z = map(float, near.split(",")) if near else (47.37, 8.54, 13)
        check(js(f"(() => {{ const c = hmMap.getCenter(); return Math.abs(c.lat - {lat}) < 0.001 && Math.abs(c.lng - {lon}) < 0.001 && Math.abs(hmMap.getZoom() - {z}) < 0.01; }})()"),
              f"the location not given, the map opens at the place of the IP address ({what}{near})" if near else "the location not given and no place of the IP address: the map opens in Zurich")
        check(js("!document.querySelector('.maplibregl-ctrl-attrib a[href=\"https://db-ip.com\"]')"), "no credit of DB-IP on the map")
        # the map credits OpenStreetMap; the side panel and the menu link the licences and spell out no credit
        check(js("!!document.querySelector('.maplibregl-ctrl-attrib a[href=\"https://www.openstreetmap.org/copyright\"]')"), "the map credits OpenStreetMap")
        check(js("(() => { const f = document.querySelector('.lm-foot'), a = f.querySelector('a.lm-licences'), m = document.querySelector('.lm-menu a.lm-licences');"
                 " return f.textContent === 'Licences' && !!a && !!m && /\\/licences$/.test(new URL(a.href).pathname) && /\\/licences$/.test(new URL(m.href).pathname)"
                 " && !/Piper|FEDRO|CC BY|OpenStreetMap|Wikimedia/.test(document.querySelector('.lm-side').textContent + document.querySelector('.lm-menu').textContent); })()"),
              "no credits spelled out in the side panel and the menu: a small link to the licences in each")
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
    check(js("document.activeElement !== document.querySelector('.lm-input')"), "on a phone the search box without the focus at the start (no keyboard over the map)")
    check(wait("location.hash.includes('/47.37')", 20), "the map starts at the location")
    # the map drawn with 2 pixels per point at most (phones have 3; at 3 here the double tap and the focus fail in headless Chromium)
    check(wait("window.hmMap?.isStyleLoaded()", 30) and js("hmMap.getPixelRatio() <= 2"), "the map drawn at 2 pixels per point at most")
    # our sources (route, pins, results …): tiles up to zoom 14 only
    check(js("(() => { const s = Object.entries(hmMap.getStyle().sources).filter(([k, v]) => k.startsWith('lm-') && v.type === 'geojson'); return s.length > 5 && s.every(([, v]) => v.maxzoom === 14); })()"),
          "our sources with tiles to zoom 14")

    # the last location of the device kept on it to about 100 m; a new visit opens there at zoom 15, not at the place of
    # the IP address (a phone network or a SIM from another country gives a town far away), and a location out of view
    # is jumped to, no flight out and in
    check(wait("localStorage.getItem('hm-last-me') === '47.378,8.540'", 10),
          f"the last location kept on the device to about 100 m ({js('localStorage.getItem(`hm-last-me`)')})")
    js("localStorage.setItem('hm-last-me', '46.948,7.447')")  # Bern; the location of the browser is Zurich
    start_hook = call("Page.addScriptToEvaluateOnNewDocument", source="Object.defineProperty(window, 'hmMap', { configurable: true, set(m) {"
                      " Object.defineProperty(window, 'hmMap', { value: m, writable: true, configurable: true });"
                      " const s = window.hmStart = { c: m.getCenter().toArray(), z: m.getZoom(), zmin: m.getZoom(), moves: 0 };"
                      " m.on('movestart', () => { s.moves++; }); m.on('move', () => { s.zmin = Math.min(s.zmin, m.getZoom()); }); } })")
    js("window.hmOld = 1")
    call("Page.navigate", url=URL)
    wait("!window.hmOld && window.hmStart && window.hmMap?.loaded() && !hmMap.isMoving() && location.hash.includes('/47.37')", 30)
    st = js("JSON.stringify(window.hmStart || null)")
    s0 = json.loads(st) if st else None
    check(s0 and abs(s0["c"][1] - 46.948) < 0.001 and abs(s0["c"][0] - 7.447) < 0.001 and abs(s0["z"] - 15) < 0.01,
          f"a new visit opens at the last location of the device, zoom 15, not at the place of the IP address ({st})")
    at_me = js(f"(() => {{ const c = hmMap.getCenter(); return Math.hypot((c.lat - {ZURICH[0]}) * 111320, (c.lng - {ZURICH[1]}) * 75000) < 300; }})()")
    check(s0 and s0["zmin"] > 14.9 and at_me, f"the location 95 km from it: the map jumps there, no flight out and in ({st})")
    call("Page.removeScriptToEvaluateOnNewDocument", identifier=start_hook["identifier"])
    check(wait("localStorage.getItem('hm-last-me') === '47.378,8.540'", 10), "the last location kept again where the visitor is")

    # the dot moves with the visitor (the page watches the location), and the map with it while it is on them; once
    # a finger moved the map the dot alone moves; the button puts the map on the visitor again
    wait("!hmMap.isMoving() && hmMap.loaded()", 20)
    def near(expr, lat, lon, m=10):
        return f"(() => {{ const c = {expr}; return !!c && Math.hypot((c[1] - {lat}) * 111320, (c[0] - {lon}) * 75000) < {m}; }})()"
    dot = "hmMap.getStyle().sources['lm-me'].data.features[1]?.geometry.coordinates"
    mid = "hmMap.getCenter().toArray()"
    p = (ZURICH[0] + 0.0012, ZURICH[1] + 0.0015)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=10)
    check(wait(near(dot, *p), 10), "the visitor moves (no search, no directions): the dot of the location moves too")
    check(wait("!hmMap.isMoving() && " + near(mid, *p, 15), 10), f"the map on the visitor goes along ({js(mid)})")
    drag(195, 260, 330)
    wait("!hmMap.isMoving()", 5)
    c0 = js(mid)
    p = (ZURICH[0] + 0.002, ZURICH[1] + 0.0015)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=10)
    moved = wait(near(dot, *p), 10)
    time.sleep(1.2)
    check(moved and js(near(mid, c0[1], c0[0], 3)), f"after a finger moved the map: the dot moves, the map stays ({c0}, {js(mid)})")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    wait(near(dot, *ZURICH), 10)
    js("document.querySelector('.lm-locate').click()")
    check(wait("!hmMap.isMoving() && " + near(mid, *ZURICH, 15), 10), "the button of the location: the map on the visitor again")

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
    # the suggestions are part of the field, under what is typed; the rest of the panel moves down under them
    check(js("(() => { const l = document.querySelector('.lm-end-to > .lm-end-list:not([hidden])'), f = document.querySelector('.lm-end-to input'), m = document.querySelector('.lm-route-msg');"
             " if (!l || !m) return false; const b = l.getBoundingClientRect();"
             " return getComputedStyle(l).position === 'static' && b.top >= f.getBoundingClientRect().bottom && m.getBoundingClientRect().top >= b.bottom; })()"),
          "the suggestions in the destination field, under what is typed; the rest of the panel under them")
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
    # with start and destination set, a tap on the map shows the place; its card goes back to the route as it was. The
    # taps are clear of the lines of the routes (a tap on another route picks it) and of the start and destination (a
    # tap there names the same place), on no name or symbol of the map
    def off_routes(x0, y0):
        wait("!hmMap.isMoving()", 10)
        return js(f"""(() => {{ const ls = ['lm-route-line', 'lm-route-alt-line', 'lm-route-alt-label'].filter(l => hmMap.getLayer(l));
          const ends = hmMap.getStyle().sources['lm-route'].data.features.filter(f => f.properties.end).map(f => hmMap.project(f.geometry.coordinates));
          for (let d = 0; d < 300; d += 15) for (const [x, y] of [[{x0} + d, {y0}], [{x0}, {y0} + d], [{x0} + d, {y0} + d]])
            if (!hmMap.queryRenderedFeatures([[x - 30, y - 30], [x + 30, y + 30]], {{ layers: ls }}).length && ends.every(e => Math.hypot(e.x - x, e.y - y) > 80)
              && !hmMap.queryRenderedFeatures([[x - 40, y - 40], [x + 40, y + 40]]).some(f => f.layer.type === 'symbol'))
              return [x, y];
          return [{x0}, {y0}]; }})()""")
    x, y = off_routes(120, 160)
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": x, "y": y}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    check(wait("!document.querySelector('.lm-here').hidden", 10) and js("!!document.querySelector('.lm-here .lm-swap-acts')"),
          "a tap on the map with a route: the point shown, the route kept")
    js("document.querySelector('.lm-here:not([hidden]) .lm-back')?.click()")
    check(wait("!document.querySelector('.lm-route').hidden && /Paradeplatz/.test(document.querySelector('.lm-end-from input').value)", 10),
          "back to the route: the start kept")
    to_at = "JSON.stringify(hmMap.getStyle().sources['lm-route'].data.features.find(f => f.properties.end === 'to')?.geometry.coordinates)"
    to0 = js(to_at)  # where the destination is (a tap may name the same place: a station beside its label)
    time.sleep(0.6)  # two taps within 0.5 s and 30 px are a double tap for MapLibre: it zooms, the second shows nothing
    x, y = off_routes(140, 180)
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": x, "y": y}])
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    wait("!!document.querySelector('.lm-here:not([hidden]) .lm-swap-acts')", 10)
    js("document.querySelector('.lm-here:not([hidden]) .lm-swap-acts .lm-chip')?.click()")
    check(wait(f"!document.querySelector('.lm-route').hidden && {to_at} !== {json.dumps(to0)} && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 30),
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
      b = document.querySelector('.lm-nav-banner'), L = document.querySelector('.lm-nav-lanes'), was = [i.textContent, t.hidden, t.textContent, L.hidden], out = [];
      t.hidden = false; t.textContent = 'Then'; L.hidden = true; // (lanes before a turn come between them)
      for (const s of ['Turn right', 'Turn right onto Papiermühlestrasse toward Zürich, Basel and Biel/Bienne, then keep left']) {
        i.textContent = s; out.push(Math.round(t.getBoundingClientRect().top - b.getBoundingClientRect().bottom), Math.round(b.getBoundingClientRect().height));
      }
      [i.textContent, t.hidden, t.textContent, L.hidden] = was; return out; })()""")
    check(gaps and abs(gaps[0]) <= 1 and abs(gaps[2]) <= 1 and gaps[3] > gaps[1],
          f"the next turn right under the banner, one and two lines (gap, height: {gaps})")
    # many lanes (12, three arrows each) with the sign: all in the width of the top (smaller), the sign under them;
    # three lanes at full size with the sign beside them (fitLanes); in one go, as above
    lanes = js("""(() => { const s = document.querySelector('.lm-nav-sign'), L = document.querySelector('.lm-nav-lanes'), H = document.querySelector('.lm-nav-head');
      const was = [s.hidden, s.className, s.textContent], out = {};
      s.hidden = false; s.className = 'lm-nav-sign lm-sign-motorway'; s.textContent = 'A1 Bern Zürich';
      hmLanes(Array.from({ length: 12 }, (_, i) => ({ d: 2 | 32 | 64, a: i > 9 ? 64 : 0 })));
      out.many = [L.lastElementChild.getBoundingClientRect().right, L.getBoundingClientRect().right, H.getBoundingClientRect().right,
        s.getBoundingClientRect().top, L.getBoundingClientRect().bottom].map(Math.round);
      hmLanes([{ d: 2, a: 2 }, { d: 2 | 64, a: 64 }, { d: 64, a: 64 }]);
      out.few = [L.style.zoom, Math.round(s.getBoundingClientRect().top - L.getBoundingClientRect().top)];
      [s.hidden, s.className, s.textContent] = was; hmLanes([]); return out; })()""")
    m = (lanes or {}).get("many") or [1, 0, 0, 0, 1]
    check(m[0] <= m[1] <= m[2] + 1 and m[3] >= m[4], f"12 lanes and the sign: the last lane in view, the sign under the lanes ({lanes})")
    check(lanes and lanes["few"][0] == "" and abs(lanes["few"][1]) < 12, f"3 lanes and the sign: full size, side by side ({lanes})")
    # the arrows of the turns ahead over the line: a white line and its head, turned with the way on
    check(wait("""(() => { const fs = hmMap.getStyle().sources['lm-turns']?.data.features || [];
      return !!hmMap.getLayer('lm-turn-line') && !!hmMap.getLayer('lm-turn-head') && fs.some(f => f.geometry.type === 'LineString' && f.geometry.coordinates.length >= 2)
        && fs.filter(f => f.geometry.type === 'Point').every(f => typeof f.properties.rotate === 'number') && fs.some(f => f.geometry.type === 'Point'); })()""", 20),
          "the arrows of the turns ahead over the line")
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
    check(js("[...document.querySelectorAll('a.lm-licences')].length === 2 && [...document.querySelectorAll('a.lm-licences')].every(a => a.textContent === 'Lizenzen' && new URL(a.href).pathname.endsWith('/de/licences'))"),
          "the links to the licences in German for German")
    check(js("(() => { const p = document.querySelector('.lm-about-text'); return !!p && p.textContent.startsWith('HippoMaps ist freie Open-Source-Software,') && p.querySelectorAll('a').length === 1; })()"),
          "the free software text in German, with its link")
    check(js("document.querySelector('.lm-powered')?.href === 'https://overpass.ch/de/'"), "Powered by overpass.ch links overpass.ch, in German for German")
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = ''; s.dispatchEvent(new Event('change')) })(); localStorage.removeItem('hm-lang')")
    check(wait("document.documentElement.lang === 'en' && /search/i.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 10), "Automatic: the language of the browser again")
    check(js("(() => { const a = document.querySelector('.lm-menu-privacy'); return a.textContent === 'Privacy' && /\\/privacy$/.test(new URL(a.href).pathname) && !a.href.includes('/de/'); })()"),
          "the privacy link in English again")
    check(js("document.querySelector('.lm-powered')?.href === 'https://overpass.ch/'"), "Powered by overpass.ch links overpass.ch in English again")
    # below Forget recent searches: free software with the link to the source code; with donations a field of 10 francs
    # and Donate (the app for Android shows no donation: Google Play)
    about = js("""(() => { const p = document.querySelector('.lm-forget + .lm-about .lm-about-text'), a = p?.querySelector('a'),
      i = document.querySelector('.lm-about .lm-donate-amount'), b = document.querySelector('.lm-about .lm-donate-btn');
      return p && { text: p.textContent, link: a?.href, amount: i?.value, button: b?.textContent }; })()""")
    check(about and about["link"] == "https://gitlab.com/cdutsov/hippomaps" and about["text"].startswith("HippoMaps is free open-source software."),
          f"the menu: free software, linked to the source code ({about})")
    if js("!!document.getElementById('app').dataset.donate"):
        check(about["amount"] == "10" and about["button"] == "Donate" and "by donating" in about["text"],
              f"the menu: a donation of 10 francs to start ({about})")
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
    # the country borders 1.4 px wide (0.7 in the styles of the server), on the style loaded first and on each switch
    check(js("hmMap.getPaintProperty('boundaries_country', 'line-width') === 1.4"), "the country borders twice as wide")
    js("window.hmStyle = hmMap.style")
    for style, colour in (("light", "#7f1d1d"), ("dark", "#ffb4a9"), ("light", "#7f1d1d")):
        js(f"document.querySelector('.lm-style[data-style={style}]').click()")
        check(wait(f"document.documentElement.dataset.theme === '{'dark' if style == 'dark' else 'light'}' && hmMap.getPaintProperty('lm-label', 'text-color') === '{colour}'", 15)
              and js("hmMap.style === hmStyle && !!hmMap.getLayer('lm-route-line') && !!hmMap.getLayer('lm-night') && hmMap.getStyle().sources['lm-route'].data.features.length > 0"),
              f"the {style} style switched in place: our layers and the route kept")
        check(js("hmMap.getPaintProperty('boundaries_country', 'line-width') === 1.4"), f"the country borders twice as wide in the {style} style")
        pill = "lm-alt-pill-dark" if style == "dark" else "lm-alt-pill"
        check(js(f"hmMap.getLayoutProperty('lm-route-alt-label', 'icon-image') === '{pill}' && hmMap.hasImage('{pill}')"),
              f"the pill of the time of another route in the {style} style")
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
    # the destination dragged with a finger (from 8 px beside it): the map does not pan, the point where it was let go,
    # a new route asked; the release opens nothing
    js("(() => { const f = hmMap.getStyle().sources['lm-route'].data.features.find(f => f.properties.end === 'to'); hmMap.jumpTo({ center: f.geometry.coordinates, padding: { bottom: 420 } }); })()")
    time.sleep(1)
    to_px = js("(() => { const f = hmMap.getStyle().sources['lm-route'].data.features.find(f => f.properties.end === 'to'); const p = hmMap.project(f.geometry.coordinates); return [p.x, p.y]; })()")
    if to_px:
        want = js(f"(() => {{ const l = hmMap.unproject([{to_px[0] - 60}, {to_px[1] + 40}]); return [l.lng, l.lat]; }})()")
        # the routes the page asks for (the answer may also be a message: some streets of the old town close at night)
        js("(() => { window.hmRouteAsks = 0; const f0 = window.fetch; window.fetch = function (u) { if (String(u).includes('/api/route')) hmRouteAsks++; return f0.apply(this, arguments); }; })()")
        c0 = js("JSON.stringify(hmMap.getCenter())")
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": to_px[0] + 8, "y": to_px[1]}])
        for k in range(1, 9):
            call("Input.dispatchTouchEvent", type="touchMove", touchPoints=[{"x": to_px[0] + 8 - 60 * k / 8, "y": to_px[1] + 40 * k / 8}])
            time.sleep(0.03)
        moved = js("JSON.stringify(hmMap.getCenter())") != c0  # before the new route fits the map to it
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        check(wait("hmRouteAsks > 0 && (document.querySelector('.lm-route-sum')?.textContent.includes('min') || /[^…]$/.test(document.querySelector('.lm-route-msg')?.textContent || ''))", 30),
              "the destination dragged: a new route asked and answered")
        got = js("hmMap.getStyle().sources['lm-route'].data.features.find(f => f.properties.end === 'to')?.geometry.coordinates")
        check(not moved and got and abs(got[0] - want[0]) < 2e-4 and abs(got[1] - want[1]) < 2e-4,
              f"the destination where the finger let go, the map not panned ({got}, {want}, panned: {moved})")
        check(js("document.querySelector('.lm-here').hidden && document.querySelector('.lm-detail').hidden && !document.querySelector('.lm-route').hidden"),
              "the release of the drag opens no place")
    else:
        check(False, "the destination of the route on the map")

    # the X clears everything
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'cafés'; i.dispatchEvent(new Event('input')); document.querySelector('.lm-clear').hidden = false })()")
    js("document.querySelector('.lm-clear').click()")
    check(wait("document.querySelector('.lm-input').value === '' && !document.querySelector('.lm-listview').hidden && document.querySelector('.lm-route').hidden", 5),
          "the X clears the search and the views")

    # the start: what the search reads, with two searches in the sentence, and searches of the map view as a row of
    # pills right under the search box, the sheet down; by Zurich main station a river of the view in them (the Limmat
    # or the Sihl); a tap on one searches it, the X brings the start back
    js("document.querySelector('.lm-input').blur(); location.hash = '#map=14/47.3779/8.5403'")
    wait("!hmMap.isMoving() && hmMap.loaded()", 20)
    start_ok = wait("document.querySelectorAll('.lm-start-list .lm-start-idea').length >= 2 && document.querySelectorAll('.lm-start-text .lm-start-ex').length === 2"
                    " && [...document.querySelectorAll('.lm-start-label')].some(e => /the (Limmat|Sihl)/.test(e.textContent))", 20)
    ideas = js("JSON.stringify([...document.querySelectorAll('.lm-start-ex, .lm-start-label')].map(e => e.textContent))")
    check(start_ok, f"the start: the sentence with two searches, searches of the map view with its river ({ideas})")
    wait("document.getElementById('app').dataset.sheet === 'peek'", 5)
    check(js("(() => { const r = document.querySelector('.lm-start-list').getBoundingClientRect(), f = document.querySelector('.lm-form').getBoundingClientRect();"
             " return document.getElementById('app').dataset.sheet === 'peek' && r.top >= f.bottom - 1 && r.bottom <= innerHeight && r.height > 20; })()"),
          "on a phone the searches of the start in a row under the search box, the sheet down")
    # (the row scrolls sideways, and is drawn again when the map view shows other things, between two calls of the
    # test too: the pill found, brought into view and tapped where it is, and once more if the row was drawn again)
    wait("hmMap.loaded() && !hmMap.isMoving()", 10)
    pill = ("(() => { const b = [...document.querySelectorAll('.lm-start-idea')].find(b => /bridges over/.test(b.textContent)); if (!b) return null;"
            " b.scrollIntoView({ inline: 'center', block: 'nearest', behavior: 'instant' }); const r = b.getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()")
    searching = False
    for _ in range(4):
        at = js(pill)
        if not at:
            time.sleep(1)
            continue
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": at[0], "y": at[1]}])
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        if wait("/bridges over/.test(document.querySelector('.lm-input').value)", 3):  # (the search has begun: its words in the box)
            searching = True
            break
    if searching:
        check(wait("/Bridge/.test(document.querySelector('.lm-title').textContent) && document.querySelector('.lm-start').hidden", 60),
              f"a tap on a search of the start searches it ({js('document.querySelector(\'.lm-title\').textContent')})")
    else:
        check(False, f"a search of the start for the bridges of the river ({js('JSON.stringify([...document.querySelectorAll(\'.lm-start-idea\')].map(b => b.textContent))')})")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click(); document.querySelector('.lm-input').blur()")
    check(wait("!document.querySelector('.lm-start').hidden && document.querySelectorAll('.lm-start-idea').length >= 2", 10), "the X: the start again, with its searches")

    # the search box: one line, two when the text needs them, never more (the rest scrolls inside); Enter searches
    # and a line break pasted becomes a space
    lines = lambda: js("(() => { const i = document.querySelector('.lm-input'); return [Math.round(i.clientHeight / parseFloat(getComputedStyle(i).lineHeight)),"
                       " i.scrollWidth <= i.clientWidth, getComputedStyle(i).overflowY]; })()")
    empty_lines = lines()
    js("document.querySelector('.lm-input').focus()")
    call("Input.insertText", text="cafés")
    short_lines = lines()
    call("Input.insertText", text=" within 500 m of Zurich main station")
    long_lines = lines()
    check(empty_lines[0] == 1 and short_lines[0] == 1 and long_lines[0] == 2 and long_lines[1],
          f"the search box: one line, two for a long search ({empty_lines}, {short_lines}, {long_lines})")
    call("Input.insertText", text="\nopen now")
    typed = js("document.querySelector('.lm-input').value")
    check("\n" not in typed and typed.endswith("station open now"), f"the search box: a line break pasted becomes a space ({typed!r})")
    js("document.querySelector('.lm-input').value = 'restaurants along the Limmat in Zurich near the lake with outdoor seating and vegan food open on Sunday'")
    check(lines()[0] == 2 and lines()[2] == "auto", f"the search box: a longer text set by the page, two lines that scroll ({lines()})")
    js("document.querySelector('.lm-input').value = 'cafés within 500 m of Zurich main station'")
    js("document.querySelector('.lm-input').focus()")
    key("Enter", "Enter", 13)
    check(wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelector('.lm-input').value === 'cafés within 500 m of Zurich main station'", 60),
          "the search box: Enter searches (no new line)")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    check(wait("document.querySelector('.lm-input').value === '' && Math.round(document.querySelector('.lm-input').clientHeight / parseFloat(getComputedStyle(document.querySelector('.lm-input')).lineHeight)) === 1", 5),
          "the search box: one line again after the X")

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
    # the list follows the view: moved by a finger, the map keeps all results and the list shows those in view
    # (on a phone above the panel); a result picked and closed, the same list; zoomed out over all, all again
    listed = lambda: js("[...document.querySelectorAll('.lm-listview .lm-results .lm-card')].map((c) => c.dataset.id)")
    count_text = lambda: js("document.querySelector('.lm-count').textContent")
    first = count_text()
    check(first and "in view" not in first and len(listed()) > 0, f"the list after a search: all results ({first})")
    # the ids drawn in the map above the panel, the edges `m` pixels in (m < 0: out)
    drawn = lambda m: js("(() => { const w = hmMap.getContainer().clientWidth, s = document.querySelector('.lm-side').getBoundingClientRect().top;"
                         f" const fs = hmMap.queryRenderedFeatures([[{m}, {m}], [w - {m}, s - {m}]], {{ layers: ['lm-point', 'lm-dot', 'lm-line', 'lm-fill'].filter((l) => hmMap.getLayer(l)) }});"
                         " return [...new Set(fs.map((f) => f.properties.id))]; })()")
    on_map = lambda: js("hmMap.getSource('lm-results').getData().then((d) => d.features.length)")
    all_drawn = on_map()
    call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": 260, "y": 300}])
    for i in range(1, 9):
        call("Input.dispatchTouchEvent", type="touchMove", touchPoints=[{"x": 260 - 140 * i / 8, "y": 300 + 60 * i / 8}])
        time.sleep(0.05)
    time.sleep(0.3)
    call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
    moved = wait("!hmMap.isMoving() && /in view/.test(document.querySelector('.lm-count').textContent)", 10)
    time.sleep(0.5)
    now, ids = count_text(), listed()
    n_in = int(re.search(r"([\d,]+) in view", now).group(1).replace(",", "")) if re.search(r"[\d,]+ in view", now) else 0
    inner, outer = set(drawn(30)), set(drawn(-30))
    check(moved and now.startswith(first) and len(ids) == min(n_in, 40) and set(ids) <= outer and len(inner) <= n_in,
          f"moved by a finger: the list shows the results in view ({now}; listed {len(ids)}, drawn in view {len(inner)}..{len(outer)})")
    check(all_drawn and on_map() == all_drawn, f"moved by a finger: the map keeps all results ({all_drawn})")
    if ids:
        before = (now, ids)
        tap(".lm-listview .lm-results .lm-card")
        wait("!document.querySelector('.lm-detail').hidden", 5)
        wait("!hmMap.isMoving()", 10)
        time.sleep(0.5)
        js("document.querySelector('.lm-detail .lm-card-x').click()")
        wait("!document.querySelector('.lm-listview').hidden", 5)
        check((count_text(), listed()) == before, "a result picked and closed: the list as it was (the map flew to the result)")
    for _ in range(8):
        call("Input.dispatchMouseEvent", type="mouseWheel", x=190, y=220, deltaX=0, deltaY=240)
        time.sleep(0.08)
    check(wait("!hmMap.isMoving() && document.querySelector('.lm-count').textContent === " + json.dumps(first), 10),
          f"zoomed out over all results: all of them again ({count_text()})")
    # the results as a file under the query: CSV and GeoJSON, a row and a feature per result on the map
    got = js("(async () => { const row = document.querySelector('.lm-dl'), files = [], blobs = {}, c0 = HTMLAnchorElement.prototype.click, u0 = URL.createObjectURL;"
             " URL.createObjectURL = (b) => { const u = u0(b); blobs[u] = b; return u; };"
             " HTMLAnchorElement.prototype.click = function () { if (this.download) files.push(this); else c0.call(this); };"
             " try { for (const b of row.querySelectorAll('button')) b.click(); } finally { HTMLAnchorElement.prototype.click = c0; URL.createObjectURL = u0; }"
             " const t = await Promise.all(files.map((a) => blobs[a.href].arrayBuffer().then((b) => new TextDecoder('utf-8', { ignoreBOM: true }).decode(b))));"
             " return { shown: !row.hidden && row.offsetHeight > 0, names: files.map((a) => a.download), csv: t[0] || '', gj: t[1] || '' }; })()")
    rows = [l for l in got["csv"].split("\r\n") if l]
    gj = json.loads(got["gj"] or "{}")
    check(got["shown"] and [n.rsplit(".", 1)[-1] for n in got["names"]] == ["csv", "geojson"] and all(n.startswith("hippomaps-hotels") for n in got["names"])
          and rows and rows[0].startswith("\ufeffid,name,lat,lon") and len(rows) - 1 == all_drawn
          and len(gj.get("features", [])) == all_drawn and "OpenStreetMap" in gj.get("attribution", "") and gj.get("query"),
          f"the results as CSV and GeoJSON ({got['names']}, {len(rows) - 1} rows, {len(gj.get('features', []))} features, {all_drawn} on the map)")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    check(js("document.querySelector('.lm-dl').hidden"), "the X: no download without results")

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
          f"the Restaurants bubble searches the map view ({js('JSON.stringify([document.querySelector(\'.lm-status\')?.textContent, document.querySelector(\'.lm-count\')?.textContent, document.querySelector(\'.lm-input\').value, location.hash])')})")
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
    # Switzerland forbids warnings of speed cameras: none on the route (OpenStreetMap has more than ten within 20 m of it)
    marks = json.loads(js("hmMap.getSource('lm-route').getData().then((d) => JSON.stringify(d.features.filter((f) => f.properties.mark).map((f) => f.properties.mark)))") or "[]")
    check("camera" not in marks, f"no speed cameras on a route in Switzerland ({marks})")
    check(js("!document.querySelector('.lm-ferries')"), "no warning of a ferry on a route without one")
    check(js("!document.querySelector('.lm-alts') && !hmMap.getStyle().sources['lm-routealt']?.data.features?.length"), "no other routes with a stop on the way")
    if js("!!document.querySelector('.lm-along-cats')"):
        js("document.querySelector('.lm-along-cats [data-cat=rest]').click()")
        check(wait("document.querySelectorAll('.lm-along-item').length > 3", 30), "rest areas along the route")
        js("document.querySelector('.lm-along-item .lm-along-add').click()")
        check(wait("document.querySelectorAll('.lm-end-stop').length === 2 && document.querySelectorAll('.lm-step-stop').length === 2", 40),
              "a rest area added as a stop: the route by both")
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    # a boat on a route by car (Horgen - Meilen over the lake of Zurich): a warning above Start with its name
    call("Page.navigate", url=URL + "/?from=47.25970,8.59770&fn=Horgen&to=47.27000,8.64300&tn=Meilen&mode=car")
    check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && !!document.querySelector('.lm-ferries .lm-ferry-ferry')", 40)
          and js("(() => { const f = document.querySelector('.lm-ferries').getBoundingClientRect(), s = document.querySelector('.lm-nav-start').getBoundingClientRect();"
                 " return f.height > 0 && f.bottom <= s.top && /^Ferry: \\S/.test(document.querySelector('.lm-ferry-ferry').textContent); })()"),
          "a ferry on a route by car: a warning with its name above Start")
    js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    # other routes from start to destination (Zurich - Bern by car): a row under Start, the route shown first with its
    # time, the others with how much longer they take; paler on the map with that time; one picked from the row, one
    # from a tap on its line
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Zurich+HB&to=46.94900,7.43910&tn=Bern&mode=car")
    if wait("document.querySelectorAll('.lm-alts .lm-alt').length >= 2", 40):
        alts_now = """JSON.stringify((() => { const bs = [...document.querySelectorAll('.lm-alt')], s = document.querySelector('.lm-nav-start').getBoundingClientRect();
          return { on: bs.map(b => b.getAttribute('aria-checked')), times: bs.map(b => b.querySelector('.lm-alt-time').textContent),
            via: bs.map(b => b.querySelector('.lm-alt-via').textContent), sum: document.querySelector('.lm-sum-time').textContent,
            under: document.querySelector('.lm-alts').getBoundingClientRect().top >= s.bottom,
            map: (hmMap.getStyle().sources['lm-routealt']?.data.features || []).map(f => [f.geometry.type, f.properties.i, f.properties.text || '']) }; })())"""
        r = json.loads(js(alts_now))
        diff = re.compile(r"^([+\u2212]\d|Same time$)")
        nr = len(r["on"])
        check(r["on"][0] == "true" and r["on"].count("true") == 1 and r["times"][0] == r["sum"] and all(diff.match(t) for t in r["times"][1:])
              and all(r["via"]) and r["under"], f"other routes: a row under Start, the route first with its time, the others with + or − ({r})")
        lines = sorted(f[1] for f in r["map"] if f[0] == "LineString")
        labels = {f[1]: f[2] for f in r["map"] if f[0] == "Point"}
        check(lines == list(range(1, nr)) and all(labels.get(i) == r["times"][i] for i in range(1, nr)),
              f"the other routes on the map, each with how much longer it takes ({r['map']})")
        check(wait("hmMap.getLayoutProperty('lm-route-alt-label', 'icon-text-fit') === 'both' && hmMap.hasImage(hmMap.getLayoutProperty('lm-route-alt-label', 'icon-image'))"
                   " && hmMap.queryRenderedFeatures({ layers: ['lm-route-alt-label'] }).length >= 1", 10),
              "the time of each other route in a pill on the map")
        js("document.querySelectorAll('.lm-alt')[1].click()")
        time.sleep(0.5)
        q = json.loads(js(alts_now))
        check(q["on"][1] == "true" and q["on"].count("true") == 1 and q["times"][1] == q["sum"] and q["sum"] != r["sum"]
              and diff.match(q["times"][0]) and sorted(f[1] for f in q["map"] if f[0] == "LineString") == [i for i in range(nr) if i != 1]
              and js("document.querySelector('.lm-step') && !!document.querySelector('.lm-nav-start')"),
              f"another route picked in the row: its time, its steps and Start; the first one on the map as another ({q})")
        # a tap on the line of the first route on the map: it is shown again
        wait("!hmMap.isMoving()", 10)
        time.sleep(1)
        pt = js("""(() => { const f = hmMap.queryRenderedFeatures({ layers: ['lm-route-alt-line'] }).find(f => f.properties.i === 0); if (!f) return null;
          const top = document.querySelector('.lm-side').getBoundingClientRect().top, c = hmMap.getCanvas().getBoundingClientRect();
          let best = null, bd = Infinity;
          for (const ll of f.geometry.coordinates) { const p = hmMap.project(ll); if (!(p.y + c.top < top - 30 && p.y > 120 && p.x > 70 && p.x < c.width - 70)) continue;
            const at = hmMap.queryRenderedFeatures([[p.x - 12, p.y - 12], [p.x + 12, p.y + 12]], { layers: ['lm-route-line', 'lm-route-alt-line'] });
            if (at.length && at.every(g => g.layer.id === 'lm-route-alt-line' && g.properties.i === 0)) {
              const d = Math.abs(p.x - c.width / 2); if (d < bd) [best, bd] = [[p.x + c.left, p.y + c.top], d]; } }
          return best; })()""")
        if pt:
            call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": pt[0], "y": pt[1]}])
            call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
            check(wait("document.querySelectorAll('.lm-alt')[0]?.getAttribute('aria-checked') === 'true'", 5), f"a tap on another route on the map shows it ({pt})")
        else:
            check(False, "the line of another route in view above the panel, away from the route")
        # a business tapped in directions: its page has Directions to go back, and the place as the destination, a stop or
        # the start; the route stays on the map. Use as start: the route from there
        js("hmMap.jumpTo({ center: [8.5390, 47.3722], zoom: 17.5 })")
        wait("hmMap.loaded() && !hmMap.isMoving() && hmMap.areTilesLoaded()", 20)
        time.sleep(1)
        cands = json.loads(js("""JSON.stringify((() => { const top = document.querySelector('.lm-side').getBoundingClientRect().top, c = hmMap.getCanvas().getBoundingClientRect();
          const ls = hmMap.getStyle().layers.filter(l => /pois/.test(l.id) && l.type === 'symbol').map(l => l.id);
          const avoid = ['lm-route-line', 'lm-route-alt-line', 'lm-route-alt-label'].filter(l => hmMap.getLayer(l)), seen = new Set();
          return hmMap.queryRenderedFeatures({ layers: ls }).filter(f => f.properties.name && f.geometry.type === 'Point' && !seen.has(f.properties.name) && seen.add(f.properties.name))
            .map(f => [f.properties.name, hmMap.project(f.geometry.coordinates)])
            .filter(([, p]) => p.y > 120 && p.y + c.top < top - 40 && p.x > 40 && p.x < c.width - 90
              && !hmMap.queryRenderedFeatures([[p.x - 30, p.y - 30], [p.x + 30, p.y + 30]], { layers: avoid }).length)
            .slice(0, 6).map(([n, p]) => [n, p.x + c.left, p.y + c.top]); })())""") or "[]")
        got = None
        for name, x, y in cands:
            time.sleep(0.6)  # (no double tap with the tap before)
            call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": x, "y": y}])
            call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
            if wait(f"!document.querySelector('.lm-detail').hidden && document.querySelector('.lm-detail .lm-dname')?.textContent === {json.dumps(name)}", 10):
                got = name
                break
            js("document.querySelector('.lm-here:not([hidden]) .lm-back, .lm-detail:not([hidden]) .lm-back')?.click()")  # back to directions
            wait("!document.querySelector('.lm-route').hidden", 5)
        if got:
            page = json.loads(js("""JSON.stringify({ back: document.querySelector('.lm-detail .lm-back').textContent, chips: [...document.querySelectorAll('.lm-detail .lm-swap-acts .lm-chip')].map(c => c.textContent),
              line: hmMap.getStyle().sources['lm-route'].data.features.some(f => f.geometry.type === 'LineString') })"""))
            check(page["back"] == "Directions" and page["chips"] == ["Use as destination", "Add as stop", "Use as start"] and page["line"],
                  f"a business tapped in directions: its page goes back to Directions, as the destination, a stop or the start, the route kept ({got}: {page})")
            js("[...document.querySelectorAll('.lm-detail .lm-swap-acts .lm-chip')].pop().click()")
            check(wait(f"!document.querySelector('.lm-route').hidden && document.querySelector('.lm-end-from input').value === {json.dumps(got)}"
                       " && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 30),
                  f"Use as start on the page of a business: the route from {got}")
        else:
            check(False, f"a business tapped in directions opens its page ({[c[0] for c in cands]})")
        js("if (!document.querySelector('.lm-route').hidden) document.querySelector('.lm-route .lm-back').click()")
    else:
        check(False, "other routes from Zurich to Bern by car")

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
    # directions before the location is known (a car park under ground): the start is Your location, Start can be
    # tapped, and navigation starts by itself with the first location
    call("Emulation.setGeolocationOverride")  # no position
    js("window.hmOld = 1")
    call("Page.navigate", url=URL + "/?to=47.36690,8.54500&tn=Bellevue&mode=car")
    wait("!window.hmOld && window.hmMap?.loaded() && !document.querySelector('.lm-route').hidden", 40)
    waiting = wait("document.querySelector('.lm-nav-wait .lm-nav-start') && document.querySelector('.lm-end-from input')?.value === 'Your location'", 15)
    got = js("JSON.stringify([document.querySelector('.lm-end-from input')?.value, document.querySelector('.lm-route-msg')?.textContent, document.querySelector('.lm-nav-wait .lm-nav-start')?.textContent])")
    check(waiting and "Waiting for your location" in got and '"Start"' in got, f"directions without a location yet: from Your location, the message, Start to tap ({got})")
    tap(".lm-nav-wait .lm-nav-start")
    check(wait("document.querySelector('.lm-nav-wait .lm-nav-start')?.getAttribute('aria-pressed') === 'true' && /Starts when/.test(document.querySelector('.lm-nav-wait .lm-nav-start').textContent)", 5)
          and js("!document.getElementById('app').classList.contains('lm-navigating')"), "Start tapped ahead: it waits for the location")
    time.sleep(1)
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    check(wait("document.getElementById('app').classList.contains('lm-navigating')", 40), "the location found: navigation starts by itself")
    js("document.querySelector('.lm-nav-x')?.click()")
    wait("!document.getElementById('app').classList.contains('lm-navigating')", 5)

    # a name tapped on the map zoomed out: its name stays the title of the panel (or its page opens), the suburb or the
    # municipality around it does not take its place
    call("Page.navigate", url="about:blank")  # (a new page: an address that differs in its hash alone would not load again)
    call("Page.navigate", url=URL + "/#map=12.6/47.3779/8.5403")
    wait("window.hmMap?.loaded() && Math.abs(hmMap.getZoom() - 12.6) < 0.01", 40)
    time.sleep(1.5)
    poi = js("(() => { const m = document.querySelector('.lm-map').getBoundingClientRect(), side = document.querySelector('.lm-side').getBoundingClientRect().top;"
             " for (const f of hmMap.queryRenderedFeatures({ layers: ['pois', 'pois_more', 'lm-pois-near', 'lm-pois-more'].filter(l => hmMap.getLayer(l)) })) {"
             "   const p = hmMap.project(f.geometry.coordinates), n = f.properties.name;"
             "   if (n && p.x > 40 && p.x < m.width - 40 && p.y > 90 && p.y + m.top < side - 40) return [n, p.x + m.left, p.y + m.top]; } return null; })()")
    check(js("hmMap.queryRenderedFeatures({ layers: ['pois'] }).some(f => f.properties.kind === 'station')"), "the main station of Zurich on the map at zoom 12.6")
    if poi:
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": poi[1] + 4, "y": poi[2] + 3}])
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        opened = wait("!document.querySelector('.lm-here').hidden || !document.querySelector('.lm-detail').hidden", 10)
        wait("!document.querySelector('.lm-detail').hidden || document.querySelector('.lm-here-note')?.hidden || (document.querySelector('.lm-here-note') && !/…$/.test(document.querySelector('.lm-here-note').textContent))", 30)
        time.sleep(0.5)
        names = js("JSON.stringify([...document.querySelectorAll('.lm-here:not([hidden]) .lm-dname, .lm-detail:not([hidden]) .lm-dname, .lm-detail:not([hidden]) h2')].map(e => e.textContent))")
        first = js("document.querySelector('.lm-here:not([hidden]) .lm-dname')?.textContent || ''")
        check(opened and (not first or first == poi[0] or poi[0] in names), f"a name tapped at zoom 12.6 ({poi[0]}): it stays the title of the panel ({names})")
        # tapped beside the name (5 px off): one marker, on the place, none where the finger was
        marks = js("JSON.stringify((() => { const d = (id) => hmMap.getStyle().sources[id]?.data?.features || [], at = (f) => { const g = f.geometry, c = g.type === 'Point' ? g.coordinates : null; return c && hmMap.project(c); };"
                   " return [...d('lm-pin'), ...d('lm-selected')].map(at).filter(Boolean).map((p) => [Math.round(p.x), Math.round(p.y)]); })())")
        m = json.loads(marks)
        tapped = (poi[1] - js("document.querySelector('.lm-map').getBoundingClientRect().left"), poi[2] - js("document.querySelector('.lm-map').getBoundingClientRect().top"))
        check(len(m) <= 1 or all(abs(a[0] - m[0][0]) < 3 and abs(a[1] - m[0][1]) < 3 for a in m), f"a name tapped: one place marked, not the place and the finger ({marks}, tapped {tapped})")
        js("(document.querySelector('.lm-here:not([hidden]) .lm-card-x, .lm-detail:not([hidden]) .lm-card-x'))?.click()")
    else:
        check(False, "a POI with a name on the map at zoom 12.6")

    # a chain searched (Migros: the search looks in 50 by 50 km around the view): the map goes to the nearest and a
    # quarter beyond, not out to all of them
    call("Page.navigate", url="about:blank")  # (a new page: an address that differs in its hash alone would not load again)
    call("Page.navigate", url=URL + "/#map=15/47.3779/8.5403")
    wait("!window.hmOld && window.hmMap?.loaded()", 40)
    time.sleep(1)
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'Migros'; document.querySelector('.lm-form').requestSubmit() })()")
    found = wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '')", 60)
    wait("!hmMap.isMoving()", 10)
    time.sleep(1.5)
    got = js("JSON.stringify((() => { const b = hmMap.getBounds(), c = hmMap.getCenter(); return { zoom: +hmMap.getZoom().toFixed(1), km: Math.round((b.getNorth() - b.getSouth()) * 111), "
             "in: hmMap.queryRenderedFeatures({ layers: ['lm-point', 'lm-dot'].filter((l) => hmMap.getLayer(l)) }).length, count: document.querySelector('.lm-count').textContent, at: [+c.lat.toFixed(3), +c.lng.toFixed(3)] }; })())")
    g = json.loads(got)
    check(found and g["zoom"] >= 13 and g["km"] <= 8 and g["in"] >= 1 and abs(g["at"][0] - 47.378) < 0.02 and abs(g["at"][1] - 8.540) < 0.03,
          f"a chain searched in Zurich: the map at the nearest shops around the visitor, not out over 50 km ({got})")
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click(); document.querySelector('.lm-input').blur()")

    # a tablet (wider than a phone, a keyboard on the screen): a place picked in a field of directions gives up the
    # focus, so the keyboard goes
    call("Emulation.setDeviceMetricsOverride", width=1024, height=768, deviceScaleFactor=2, mobile=True)
    call("Page.navigate", url="about:blank")  # (a new page: an address that differs in its hash alone would not load again)
    call("Page.navigate", url=URL + "/#map=15/47.3779/8.5403")
    wait("!window.hmOld && window.hmMap?.loaded()", 40)
    if js("matchMedia('(pointer: coarse)').matches && !matchMedia('(max-width: 760px)').matches"):
        js("document.querySelector('.lm-dir-btn').click()")
        wait("!document.querySelector('.lm-route').hidden", 5)
        js("window.hmBlurs = 0; document.addEventListener('focusout', (e) => { if (e.target.matches?.('.lm-end-input')) hmBlurs++; }, true)")
        js("document.querySelector('.lm-end-to input').focus()")
        call("Input.insertText", text="Bahnhof Enge Zürich")
        js("(() => { const f = document.querySelector('.lm-end-to input'); if (document.activeElement !== f) { f.focus(); f.dispatchEvent(new Event('input')); } })()")
        if wait("[...document.querySelectorAll('.lm-end-list:not([hidden]) .lm-opt-label')].some(e => /Enge/.test(e.textContent))", 20):
            n0 = js("hmBlurs")
            tap(".lm-end-to .lm-end-list:not([hidden]) .lm-opt")
            check(wait(f"hmBlurs > {n0} && !document.activeElement?.matches?.('input, textarea')", 5), "on a tablet a place picked in directions: the field gives up the focus (the keyboard goes)")
        else:
            check(False, "suggestions in directions on a tablet")
    else:
        check(False, "a tablet in the test: a coarse pointer, wider than a phone")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)

    # a route found on a phone, small and large: car, bike and walk stay in view above start and destination (the
    # panel not scrolled), the time and Start in view under them: the sheet as high as they need
    for w, h in ((360, 640), (390, 844)):
        call("Emulation.setDeviceMetricsOverride", width=w, height=h, deviceScaleFactor=2, mobile=True)
        call("Page.navigate", url=URL + "/?to=47.36690,8.54500&tn=Bellevue&mode=car")
        wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40)
        time.sleep(1.2)  # the sheet at its height
        got = js("JSON.stringify((() => { const b = document.querySelector('.lm-body'), m = document.querySelector('.lm-modes').getBoundingClientRect(),"
                 " k = document.querySelector('.lm-route > .lm-back').getBoundingClientRect(), g = document.querySelector('.lm-nav-go').getBoundingClientRect(),"
                 " s = document.querySelector('.lm-side').getBoundingClientRect();"
                 " return { sheet: document.getElementById('app').dataset.sheet, scroll: b.scrollTop, modes: Math.round(m.top - k.bottom), start: Math.round(innerHeight - g.bottom), map: Math.round(s.top) }; })())")
        g = json.loads(got)
        check(g["sheet"] == "half" and g["scroll"] == 0 and g["modes"] >= 0 and g["start"] >= 0 and g["map"] >= 0.28 * h,
              f"a route on a phone of {w} by {h}: car, bike and walk in view, the panel not scrolled, Start in view, the map above ({got})")
    call("Emulation.setDeviceMetricsOverride", width=390, height=480, deviceScaleFactor=2, mobile=True)  # the keyboard up
    js("document.querySelector('.lm-end-to input').focus()")
    time.sleep(0.4)
    js("document.querySelector('.lm-end-to input').scrollIntoView({block: 'start'})")  # (the field at the top of the full panel: scrolled to the top of it)
    time.sleep(0.5)
    panel_at = "JSON.stringify([document.getElementById('app').dataset.sheet, document.querySelector('.lm-body').scrollTop, document.querySelector('.lm-body').scrollHeight, document.querySelector('.lm-body').clientHeight, Math.round(document.querySelector('.lm-modes').getBoundingClientRect().top - document.querySelector('.lm-body').getBoundingClientRect().top), document.activeElement?.className || ''])"
    before = js(panel_at)
    scrolled = js("document.querySelector('.lm-body').scrollTop > 20")
    js("document.querySelector('.lm-end-to input').blur()")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)  # the keyboard down
    check(scrolled and wait("document.querySelector('.lm-body').scrollTop === 0 && document.querySelector('.lm-modes').getBoundingClientRect().top >= document.querySelector('.lm-body').getBoundingClientRect().top", 5),
          f"the keyboard gone: directions back at their top, car, bike and walk in view (before: {before}, after: {js(panel_at)})")

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
    # the panel at the height of its state in the page as it is (half 48 %, or down to Start of a route up to 85 %; full
    # all but 56 px; peek at most 40 %), its bottom at the bottom of the page, the page not scrolled
    at_height = lambda: js("(() => { const s = document.querySelector('.lm-side').getBoundingClientRect(), H = document.getElementById('app').clientHeight,"
                           " r = document.querySelector('.lm-route'), go = r && !r.hidden && r.querySelector('.lm-nav-go'),"
                           " need = go ? Math.ceil(go.getBoundingClientRect().bottom + document.querySelector('.lm-body').scrollTop - s.top + 12) : 0,"
                           " st = document.getElementById('app').dataset.sheet, want = { half: Math.max(Math.round(H * 0.48), Math.min(Math.round(H * 0.85), need)), full: H - 56 }[st];"
                           " return Math.abs(s.bottom - innerHeight) <= 1 && H === innerHeight && scrollY === 0"
                           " && (want == null ? s.height <= H * 0.4 + 1 : Math.abs(s.height - want) <= 2); })()")
    panel = lambda: js("(() => { const s = document.querySelector('.lm-side').getBoundingClientRect(); return [document.getElementById('app').dataset.sheet, Math.round(s.top), Math.round(s.height), innerHeight]; })()")

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
    check(ok and sheet() == "full" and abs(js("document.querySelector('.lm-side').getBoundingClientRect().top") - 56) <= 2 and at_height(),
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
        check(ok and wait("/Bern/.test(document.querySelector('.lm-title')?.textContent || '')", 30) and at_height(),
              f"a suggestion picked, the keyboard goes: the panel down in one movement, to the height of its state ({t}, {panel()})")
        # Zürich to Bern: one flight out and in, after the keyboard went, no correction after it
        wait("hmMoves.length > 0 && hmMoves.every(m => m.t1)", 5)
        time.sleep(0.5)
        m = js("JSON.stringify({ moves: hmMoves.filter(m => !m.same).map(m => ({ ms: Math.round(m.t1 - m.t0), out: +(Math.min(m.z0, m.z1) - m.zmin).toFixed(2), after: Math.round(m.t0 - hmResized) })) })")
        mv = json.loads(m)["moves"]
        # (the flight is set to 0.9 to 1.5 s; drawn in software on a busy machine it has taken 1.75 s)
        check(len(mv) == 1 and mv[0]["after"] >= 0 and 800 <= mv[0]["ms"] <= 2500,
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
        check(ok and sheet() == "half" and wait("/Paradeplatz/.test(document.querySelector('.lm-end-to input').value)", 5) and at_height(),
              f"a place picked in directions, the keyboard goes: the panel down to half in one movement ({t}, {panel()})")
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
    check(ok and sheet() == "half" and at_height(), f"a tap away from a field of directions: the panel down in one movement ({t}, {panel()})")
    keyboard(False)
    # the keyboard put away with the field still in focus (the back button of Android), then a tap away: full in the
    # whole page, then half; and the keyboard going in two steps
    tap(".lm-end-from input")
    time.sleep(0.1)
    keyboard(True)
    time.sleep(1)
    keyboard(False)
    time.sleep(1)
    full_ok = sheet() == "full" and at_height()
    js("document.activeElement.blur()")
    time.sleep(1.2)
    check(full_ok and sheet() == "half" and at_height(), f"the keyboard put away in a field, then a tap away: the panel full, then half ({panel()})")
    tap(".lm-end-to input")
    time.sleep(0.1)
    keyboard(True)
    time.sleep(1)
    js("document.activeElement.blur()")
    time.sleep(0.05)
    call("Emulation.setDeviceMetricsOverride", width=390, height=700, deviceScaleFactor=2, mobile=True)
    time.sleep(0.05)
    keyboard(False)
    time.sleep(1.5)
    check(sheet() == "half" and at_height(), f"the keyboard going in two steps: the panel at half of the whole page ({panel()})")

    # no connection: the note at the top of the map in place of the bubbles, a search says why it failed, the note goes
    # when the connection is back. Online for the device but the server out of reach: the same, the server asked again
    # every 5 s. In navigation the note under the banner; off the route no new route is asked: the banner says why,
    # the voice once (said from a sound fetched at Start); the connection back, a new route.
    def offline(on):
        call("Network.emulateNetworkConditions", offline=on, latency=0, downloadThroughput=-1, uploadThroughput=-1)
    call("Network.enable")  # (without it the device is offline for the page, but its requests still go out)
    note = "(document.querySelector('.lm-offline:not([hidden])')?.offsetParent ? document.querySelector('.lm-offline').parentNode.className + ' | ' + document.querySelector('.lm-offline').textContent : '')"
    js("window.hmOld = 1")
    call("Page.navigate", url=URL)
    wait("!window.hmOld && window.hmMap?.loaded()", 30)
    time.sleep(0.5)
    check(js(f"!{note}"), "online: no note of the connection")
    offline(True)
    check(wait(f"/^lm-map.*No internet connection/.test({note})", 5) and js("getComputedStyle(document.querySelector('.lm-bubbles')).display === 'none'"),
          "offline: the note at the top of the map, in place of the bubbles")
    js("(() => { document.querySelector('.lm-input').value = 'museums'; document.querySelector('.lm-form').requestSubmit(); })()")
    check(wait("document.querySelector('.lm-status').textContent === 'No internet connection'", 10),
          f"offline: a search says there is no connection ({js('document.querySelector(\'.lm-status\').textContent')})")
    offline(False)
    check(wait(f"!{note}", 10), "the connection back: the note goes")
    call("Network.setBlockedURLs", urls=["*/api/*", "*/healthz*"])
    js("(() => { document.querySelector('.lm-input').value = 'museums'; document.querySelector('.lm-form').requestSubmit(); })()")
    check(wait(f"/No internet connection/.test({note})", 15), "the server out of reach: the note of the connection")
    time.sleep(6)  # (answers from the cache of the browser meanwhile, the tiles: the server asked again, still out of reach)
    check(js(f"/No internet connection/.test({note})"), "the server out of reach: the note stays")
    call("Network.setBlockedURLs", urls=[])
    check(wait(f"!{note}", 12), "the server answers again: the note goes")
    call("Page.navigate", url=URL + "/?to=47.36690,8.54500&tn=Bellevue&mode=car")
    if wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40):
        js("(() => { const f0 = window.fetch; window.hmRoutes = 0; window.fetch = function (u) { if (String(u).includes('/api/route')) hmRoutes++; return f0.apply(this, arguments); }; })()")
        js("document.querySelector('.lm-nav-start').click()")
        wait("document.getElementById('app').classList.contains('lm-navigating') && /\\d/.test(document.querySelector('.lm-nav-dist')?.textContent || '')", 20)
        time.sleep(2)
        offline(True)
        check(wait(f"/^lm-nav-head.*Navigation goes on/.test({note})", 5), "offline in navigation: the note under the banner")
        for i in range(6):  # 400 m off the route
            call("Emulation.setGeolocationOverride", latitude=ZURICH[0] + 0.004 + i * 0.0001, longitude=ZURICH[1] + 0.004, accuracy=10)
            time.sleep(1.2)
        said = js("(window.hmSaid || []).map(x => Array.isArray(x) ? x[0] : x)") or []
        check(js("/No connection for a new route/.test(document.querySelector('.lm-nav-banner').textContent) && hmRoutes === 0")
              and len([t for t in said if "No connection for a new route" in t]) == 1,
              f"offline off the route: no new route asked, the banner says why, said once ({said[-2:]})")
        offline(False)
        for i in range(5):
            call("Emulation.setGeolocationOverride", latitude=ZURICH[0] + 0.0046 + i * 0.0001, longitude=ZURICH[1] + 0.004, accuracy=10)
            time.sleep(1.2)
        check(wait(f"!{note} && hmRoutes > 0 && !/No connection/.test(document.querySelector('.lm-nav-banner').textContent)", 10),
              "the connection back in navigation: the note goes, a new route")
        js("document.querySelector('.lm-nav-x')?.click()")
        wait("!document.getElementById('app').classList.contains('lm-navigating')", 5)
    else:
        check(False, "a route for navigation without a connection")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)

    # the map of a route for the way without a connection: the service worker (web/sw.js) controls the page; Start puts
    # the tiles along the route in its cache (window.hmRouteCache). Without a connection (the page and the service worker:
    # the workers of the map fetch through it) the note says the map is saved, the map along the route loads from the
    # device, tilted as in navigation, and the page opens again.
    # (the service worker offline through a session of its own; closing the session ends it, also when the service
    # worker was stopped and started again meanwhile)
    sw_ws = []

    def offline_all(on):
        if on:
            for t in json.load(urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json")):
                if t["type"] == "service_worker" and t["url"].startswith(URL):
                    try:
                        w = websocket.create_connection(t["webSocketDebuggerUrl"], timeout=10)
                        sw_ws.append(w)
                        for i, (m, p) in enumerate([("Network.enable", {}), ("Network.emulateNetworkConditions", {"offline": True, "latency": 0, "downloadThroughput": -1, "uploadThroughput": -1})]):
                            w.send(json.dumps({"id": 900 + i, "method": m, "params": p}))
                            while json.loads(w.recv()).get("id") != 900 + i:
                                pass
                    except Exception as e:
                        print(f"  (the service worker offline: {e})", flush=True)
        else:
            for w in sw_ws:
                try:
                    w.close()
                except Exception:
                    pass
            sw_ws.clear()
        offline(on)
    js("window.hmOld = 1")
    call("Page.navigate", url=URL + "/?to=47.41150,8.54420&tn=Oerlikon&mode=car")
    check(wait("!window.hmOld && navigator.serviceWorker?.controller", 20), "the service worker controls the page")
    if wait("document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40):
        check(js("document.querySelector('.lm-save-route input')?.checked === true"), "the switch of the map of a route in the menu, on")
        js("document.querySelector('.lm-nav-start').click()")
        saved = wait("hmRouteCache.saved || hmRouteCache.failed", 120)
        rc = js("JSON.stringify(hmRouteCache, ['total', 'done', 'failed', 'saved'])")
        kept = js("caches.open('hm-route').then(c => c.keys()).then(k => k.length)")
        check(saved and js("hmRouteCache.saved && hmRouteCache.total > 50") and kept >= js("hmRouteCache.total"),
              f"Start: the map along the route saved on the device ({rc}, {kept} in the cache)")
        pts = js("(() => { const l = hmMap.getSource('lm-route')?.serialize?.().data?.features?.[0]?.geometry?.coordinates;"
                 " return l && l.length > 3 ? [0.3, 0.6, 0.95].map((f) => { const i = Math.floor(f * (l.length - 2)); return [l[i], l[i + 1]]; }) : null; })()") or []
        offline_all(True)
        check(wait(f"/its map saved/.test({note})", 10), "without a connection in navigation: the note says the map of the route is saved")
        js("document.querySelector('.lm-nav-x')?.click()")
        wait("!document.getElementById('app').classList.contains('lm-navigating')", 5)
        js("window.hmTileErrs = 0; hmMap.on('error', (e) => { if (e.sourceId === 'protomaps') hmTileErrs++; })")
        loaded = []
        for a, b2 in pts:
            brg = math.degrees(math.atan2((b2[0] - a[0]) * math.cos(math.radians(a[1])), b2[1] - a[1]))
            js(f"hmMap.jumpTo({{ center: {json.dumps(a)}, zoom: 16.5, pitch: 50, bearing: {brg} }})")
            loaded.append(wait("hmMap.areTilesLoaded()", 10))
        time.sleep(1)
        check(len(pts) == 3 and all(loaded) and js("hmTileErrs === 0"),
              f"without a connection the map along the route loads from the device, tilted ({loaded}, {js('hmTileErrs')} tiles failed)")
        js("window.hmOld = 1")
        call("Page.navigate", url=URL)
        check(wait("!window.hmOld && window.hmMap?.isStyleLoaded()", 30), "without a connection the page opens (the service worker)")
        offline_all(False)
        # the switch off: the map kept goes, and Start keeps none
        wait("window.hmMap?.loaded()", 20)
        js("(() => { const b = document.querySelector('.lm-save-route input'); b.checked = false; b.dispatchEvent(new Event('change')); })()")
        gone = wait("caches.open('hm-route').then(c => c.keys()).then(k => k.length === 0)", 10) and js("localStorage.getItem('hm-save-route') === '0'")
        js("window.hmOld = 1")
        call("Page.navigate", url=URL + "/?to=47.41150,8.54420&tn=Oerlikon&mode=car")
        wait("!window.hmOld && document.querySelector('.lm-nav-start') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40)
        js("document.querySelector('.lm-nav-start').click()")
        time.sleep(6)
        none = js("hmRouteCache.total === 0") and js("caches.open('hm-route').then(c => c.keys()).then(k => k.length === 0)")
        js("document.querySelector('.lm-nav-x')?.click()")
        js("(() => { const b = document.querySelector('.lm-save-route input'); b.checked = true; b.dispatchEvent(new Event('change')); })()")
        check(gone and none and js("localStorage.getItem('hm-save-route') === '1'"), "the switch off: the map of the route goes, Start keeps none; on again")
    else:
        check(False, "a route for the map kept for the way")

    # German
    js("(() => { const s = document.querySelector('.lm-lang-select'); s.value = 'de'; s.dispatchEvent(new Event('change')) })()")
    time.sleep(1)
    check(wait("document.documentElement.lang === 'de' && /suchen/.test(document.querySelector('.lm-input').getAttribute('aria-label'))", 30), "the page in German")
    # a word that is two labels in one language and two words in the next: each label keeps its own (Tankstellen is a
    # bubble and a kind along a route in German, Stations-service and Carburant in French; Cafés stays Cafés)
    def page_lang(lang):
        js(f"(() => {{ const s = document.querySelector('.lm-lang-select'); s.value = '{lang}'; s.dispatchEvent(new Event('change')) }})()")
        wait(f"document.documentElement.lang === '{lang}'", 10)
        return js("[...document.querySelectorAll('.lm-bubble')].map(b => b.textContent).join('|')") or ""
    fr_bubbles, en_bubbles, de_bubbles = page_lang("fr"), page_lang("en"), page_lang("de")
    check("Stations-service" in fr_bubbles and "Carburant" not in fr_bubbles and "Fuel" in en_bubbles and "Cafés" in en_bubbles
          and "Tankstellen" in de_bubbles and "Cafés" in de_bubbles and "Bancomaten" in de_bubbles,
          f"the bubbles keep their own words through French, English and German ({fr_bubbles}; {en_bubbles}; {de_bubbles})")
    # the start in German: the sentence and the searches of the view, no English in them
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click(); document.querySelector('.lm-input').blur()")
    de_ok = wait("/versteht Alltagssprache/.test(document.querySelector('.lm-start-text')?.textContent || '') && document.querySelectorAll('.lm-start-label').length >= 2", 15)
    de_ideas = js("[...document.querySelectorAll('.lm-start-ex, .lm-start-label')].map(e => e.textContent)") or []
    check(de_ok and not any(re.search(r"\b(along|near|over|open|the)\b", t) for t in de_ideas), f"the start in German ({de_ideas})")
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
    # Forget recent searches in the menu: off once they are forgotten, on again when a search came after
    def forget_off(then=""):
        js("document.querySelector('.lm-map-burger').click()")
        off = js(f"(() => {{ const b = document.querySelector('.lm-forget'), was = b.disabled; {then}; return was; }})()")
        js("document.querySelector('.lm-backdrop').click()")
        return off
    before = forget_off("b.click()")
    forgotten = forget_off() and js("localStorage.getItem('hm-recent') === '[]'")
    js("[...document.querySelectorAll('.lm-bubble')].find(b => /Hotels/.test(b.textContent))?.click()")
    check(before is False and forgotten and forget_off() is False, "Forget recent searches: off once forgotten, on again after the next search")
    js("localStorage.removeItem('hm-lang')")

    # the signs along a route by car: speed cameras where a navigation app may show them (the Netherlands), stop signs
    # (all-way stops in Berkeley), drawn beside the route when near
    def route_marks(link, what):
        js("window.hmOld = 1")
        call("Page.navigate", url=URL + link)
        if not wait("!window.hmOld && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40):
            check(False, f"a route by car {what}")
            return []
        return json.loads(js("hmMap.getSource('lm-route').getData().then((d) => JSON.stringify(d.features.filter((f) => f.properties.mark).map((f) => [f.properties.mark, f.geometry.coordinates])))") or "[]")
    def drawn(m, layer):
        js(f"hmMap.jumpTo({{ center: {json.dumps(m[1])}, zoom: 16 }})")
        return wait(f"hmMap.loaded() && hmMap.queryRenderedFeatures({{ layers: ['{layer}'] }}).length > 0", 15)
    marks = route_marks("/?from=52.37600,4.90410&fn=Amsterdam&to=52.09070,5.12140&tn=Utrecht&mode=car", "in the Netherlands")
    cams = [m for m in marks if m[0] == "camera"]
    check(len(cams) >= 1 and drawn(cams[0], "lm-marks-camera"), f"speed cameras along a route by car in the Netherlands, drawn beside it ({len(cams)})")
    marks = route_marks("/?from=37.87160,-122.27270&fn=A&to=37.85900,-122.29900&tn=B&mode=car", "in Berkeley")
    stops = [m for m in marks if m[0] == "stop"]
    check(len(stops) >= 1 and drawn(stops[0], "lm-marks"), f"stop signs along a route by car in Berkeley, drawn beside it ({len(stops)})")

    # navigation on a long route by car (Zurich to Winterthur, Preview), for what takes a while: pinches out of tilted
    # navigation; the language of directions and voice apart from that of the page. Last: before the panel on a phone
    # it made a tap on a suggestion there miss now and then
    js("window.hmOld = 1")
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Zurich+HB&to=47.50020,8.72380&tn=Winterthur&mode=car")
    if wait("!window.hmOld && document.querySelector('.lm-nav-go .lm-chip') && document.querySelector('.lm-route-sum')?.textContent.includes('min')", 40):
        js("document.querySelector('.lm-nav-go .lm-chip').click()")
        wait("document.getElementById('app').classList.contains('lm-navigating')", 5)
        time.sleep(3)
        # pinches out of tilted navigation, also below zoom 15 where the camera turns to look straight down: each pinch
        # zooms as far as the fingers go (300 px to 100 px: 1.5 zoom levels) and the tilt follows after it (a change of
        # the tilt during the gesture stopped it at every step); then Re-centre
        def pinch(a=45, b=345, c=145, d=245, y=480):
            call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": a, "y": y, "id": 1}, {"x": b, "y": y, "id": 2}])
            for k in range(1, 13):
                call("Input.dispatchTouchEvent", type="touchMove", touchPoints=[{"x": a + (c - a) * k / 12, "y": y, "id": 1}, {"x": b + (d - b) * k / 12, "y": y, "id": 2}])
                time.sleep(0.03)
            call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        outs = []
        for _ in range(4):
            z0 = js("hmMap.getZoom()")
            pinch()
            time.sleep(1.2)
            outs.append(round(z0 - js("hmMap.getZoom()"), 2))
        zp = js("[hmMap.getZoom(), hmMap.getPitch()]") or [0, -9]
        want = 50 * min(1, max(0, (zp[0] - 12) / 3))
        check(min(outs) > 1.2 and abs(zp[1] - want) < 2,
              f"pinches out of navigation: each zooms out as far as the fingers go, then the tilt of the zoom ({outs}, tilt {zp[1]:.1f} at zoom {zp[0]:.2f})")
        js("document.querySelector('.lm-nav-center')?.click()")
        wait("document.querySelector('.lm-nav-center').hidden && Math.abs(hmMap.getPitch() - 50) < 1", 10)
        # the language of navigation apart from that of the page: Bulgarian while navigating; the page stays in English,
        # the route and the voice are asked in Bulgarian; "Automatic" brings English back
        res = "performance.getEntriesByType('resource')"
        js("(() => { const s = document.querySelector('.lm-nav-lang-select'); s.value = 'bg'; s.dispatchEvent(new Event('change')) })()")
        check(wait(f"{res}.some(e => e.name.includes('/api/route') && e.name.includes('lang=bg'))", 20)
              and wait(f"{res}.some(e => e.name.includes('/api/say') && e.name.includes('lang=bg'))", 20)
              and js("document.documentElement.lang === 'en' && /search/i.test(document.querySelector('.lm-input').getAttribute('aria-label')) && localStorage.getItem('hm-nav-lang') === 'bg'")
              and js("document.getElementById('app').classList.contains('lm-navigating')"),
              "Directions and voice in Bulgarian while navigating: the page in English, the route and the voice in Bulgarian")
        en0 = js(f"{res}.filter(e => e.name.includes('/api/route') && e.name.includes('lang=en')).length")
        js("(() => { const s = document.querySelector('.lm-nav-lang-select'); s.value = ''; s.dispatchEvent(new Event('change')) })()")
        check(wait(f"{res}.filter(e => e.name.includes('/api/route') && e.name.includes('lang=en')).length > {en0}", 20)
              and js("localStorage.getItem('hm-nav-lang') === ''"), "Directions and voice Automatic (like the page): the route in English again")
        js("document.querySelector('.lm-nav-x')?.click()")
        wait("!document.getElementById('app').classList.contains('lm-navigating')", 5)
    else:
        check(False, "a long route by car for Preview")
    # on a computer (no touch screen) the cursor is in the search box at the start: typing goes there at once; the starred
    # places and last searches only after a click in it. Headless Chromium has no mouse ((hover: hover) and (pointer:
    # fine) do not match, and Emulation.setEmulatedMedia leaves them): a mouse given to the page in matchMedia
    call("Emulation.setTouchEmulationEnabled", enabled=False)
    call("Emulation.setDeviceMetricsOverride", width=1280, height=800, deviceScaleFactor=1, mobile=False)
    mouse = call("Page.addScriptToEvaluateOnNewDocument", source="(() => { const mm = window.matchMedia.bind(window);"
                 " window.matchMedia = (q) => (/\\((hover: hover|pointer: fine)\\)/.test(q) && !/none|coarse/.test(q) ? mm('all') : mm(q)); })()")
    js("window.hmOld = 1; localStorage.setItem('hm-recent', JSON.stringify([{ text: 'cafes in Zurich', extra: {} }]))")
    call("Page.navigate", url=URL)
    focused = wait("!window.hmOld && document.activeElement === document.querySelector('.lm-input')", 15)
    check(focused and js("document.querySelector('.lm-suggest').hidden"),
          f"on a computer the cursor in the search box at the start, no list over the map ({focused}, {js('JSON.stringify([document.activeElement?.className, document.querySelector(\'.lm-suggest\').hidden, !!window.hmOld])')})")
    check(wait("document.querySelectorAll('.lm-start-idea').length >= 2", 20) and js("document.querySelectorAll('.lm-start-text .lm-start-ex').length === 2"
          " && document.querySelector('.lm-start-head').offsetHeight > 0 && document.querySelector('.lm-start-list').getBoundingClientRect().bottom < innerHeight"),
          "on a computer the start under the search box: the sentence with two searches, In this area and its searches")
    # another tab, then back: the box gets the focus again, and that opens no list
    tab = call("Target.createTarget", url="about:blank").get("targetId")
    time.sleep(1)
    call("Page.bringToFront")
    time.sleep(1.5)
    check(js("document.activeElement === document.querySelector('.lm-input') && document.querySelector('.lm-suggest').hidden"),
          "on a computer back from another tab: the cursor in the search box, no list")
    call("Target.closeTarget", targetId=tab)
    r = js("(() => { const b = document.querySelector('.lm-input').getBoundingClientRect(); return [b.left + 20, b.top + b.height / 2]; })()")
    for t in ("mousePressed", "mouseReleased"):
        call("Input.dispatchMouseEvent", type=t, x=r[0], y=r[1], button="left", clickCount=1)
    check(wait("!document.querySelector('.lm-suggest').hidden && /cafes in Zurich/.test(document.querySelector('.lm-suggest').textContent)", 5),
          "a click in the search box: the last searches")
    call("Input.insertText", text="Bern")
    check(wait("document.querySelector('.lm-input').value === 'Bern'", 5), "on a computer typing at once goes into the search box")
    # a place written in the search (not searched), then Directions: the text moves to the destination, its suggestions there
    js("document.querySelector('.lm-dir-btn').click()")
    check(wait("!document.querySelector('.lm-route').hidden && document.querySelector('.lm-end-to input').value === 'Bern' && document.querySelector('.lm-input').value === ''", 10)
          and wait("[...document.querySelectorAll('.lm-end-to .lm-end-list:not([hidden]) .lm-opt-label')].some(e => /^Bern/.test(e.textContent))", 20),
          f"a place written in the search, then Directions: the text in the destination with its suggestions, the search box empty"
          f" ({js('JSON.stringify([document.querySelector(\'.lm-end-to input\')?.value, document.querySelector(\'.lm-input\').value])')})")
    # the POIs of every kind with a name, not only those of the list of the style: a dental clinic in Sofia by its
    # name, other kinds around it, nothing of the street furniture; in the dark style too
    js("hmMap.jumpTo({ center: [23.3126298, 42.707687], zoom: 18, pitch: 0, bearing: 0 })")
    pois = ("JSON.stringify((() => { const f = hmMap.queryRenderedFeatures({ layers: ['pois', 'pois_more', 'lm-pois-near', 'lm-pois-more'].filter(l => hmMap.getLayer(l)) }).map(x => x.properties);"
            " return { clinic: f.some(x => /Щирков/.test(x.name || '')), kinds: [...new Set(f.map(x => x.kind))].sort(), unnamed: f.filter(x => !x.name).map(x => x.kind) }; })())")
    for style in ("light", "dark"):
        js(f"document.querySelector('.lm-style[data-style={style}]').click()")
        wait(f"document.documentElement.dataset.theme === '{style}'", 15)
        wait("hmMap.loaded() && !hmMap.isMoving() && " + pois.replace("JSON.stringify(", "(", 1) + ".clinic", 30)
        g = json.loads(js(pois))
        street = {"crossing", "traffic_signals", "tree", "street_lamp", "waste_basket", "vacant", "stop", "platform", "zone"}
        check(g["clinic"] and "dentist" in g["kinds"] and len([k for k in g["kinds"] if k not in ("dentist", "restaurant", "cafe", "supermarket", "school", "park")]) >= 3
              and not street & set(g["kinds"]),
              f"the {style} map at its largest zooms: a dental clinic by its name, POIs of other kinds, no street furniture ({g['kinds']})")
    # benches: none at zoom 16.5, where the other places are; at zoom 18 they are there
    def benches(z):
        js(f"hmMap.jumpTo({{ center: [8.5437, 47.3725], zoom: {z}, pitch: 0, bearing: 0 }})")
        time.sleep(1)
        wait("hmMap.loaded() && !hmMap.isMoving()", 20)
        time.sleep(1)
        return json.loads(js("JSON.stringify((() => { const f = hmMap.queryRenderedFeatures({ layers: ['pois', 'pois_more', 'lm-pois-near', 'lm-pois-more', 'lm-pois-late'].filter(l => hmMap.getLayer(l)) });"
                             " return [f.filter(x => x.properties.kind === 'bench').length, f.filter(x => x.properties.kind !== 'bench').length]; })())"))
    b16, b18 = benches(16.5), benches(18.5)
    check(b16[0] == 0 and b16[1] > 10 and js("hmMap.getLayer('lm-pois-late')?.minzoom === 18 && !!hmMap.getLayer('lm-pois-late')"),
          f"benches only at the largest zooms: none at zoom 16.5 beside {b16[1]} places ({b16[0]} benches; at zoom 18.5: {b18[0]})")
    # the middle zooms are not full of places: at zoom 14.5 none the tiles draw from zoom 16 (restaurants), at 15.5
    # none from zoom 17 (shops), no icon without its name, and the icons keep their distance
    def middle(z):
        js(f"hmMap.jumpTo({{ center: [8.5437, 47.3725], zoom: {z}, pitch: 0, bearing: 0 }})")
        time.sleep(1)
        wait("hmMap.loaded() && !hmMap.isMoving()", 20)
        time.sleep(1)
        return json.loads(js("JSON.stringify((() => { const ls = ['pois', 'pois_more', 'lm-pois-near', 'lm-pois-more'].filter(l => hmMap.getLayer(l));"
                             " const f = hmMap.queryRenderedFeatures({ layers: ls }).map(x => x.properties.min_zoom);"
                             " return { n: f.length, top: Math.max(...f), gap: ls.every(l => JSON.stringify(hmMap.getLayoutProperty(l, 'icon-padding')).includes('interpolate')),"
                             " alone: ['pois_more', 'lm-pois-more'].filter(l => hmMap.getLayer(l)).every(l => JSON.stringify(hmMap.getLayoutProperty(l, 'text-optional')) === '[\"step\",[\"zoom\"],false,17,true]') }; })())"))
    m14, m15 = middle(14.5), middle(15.5)
    check(m14["n"] > 3 and m14["top"] <= 15 and m15["n"] > 3 and m15["top"] <= 16 and m14["gap"] and m15["gap"] and m15["alone"],
          f"the middle zooms are not full of places: zoom 14.5 {m14}, zoom 15.5 {m15}")
    # the popup of a POI: the mouse on it, its name and kind above it, the pointer a hand; off it, gone; a click, the popup
    # above the pin and the page of the place (one popup with the mouse still on it); the X of the card, gone
    # (directions closed first: there a click on the map sets the start or the destination)
    js("document.querySelector('.lm-route:not([hidden]) > .lm-back')?.click()")
    wait("document.querySelector('.lm-route').hidden", 5)
    POP_LAYERS = "['pois', 'pois_more', 'lm-pois-near', 'lm-pois-more', 'lm-pois-late']"
    # a named POI near the middle of the map, alone under a point of the page
    poi_near_mid = ("(() => { const c = hmMap.getCanvas().getBoundingClientRect(), ls = %s.filter((l) => hmMap.getLayer(l));"
                    " const fs = hmMap.queryRenderedFeatures({ layers: ls }).filter((f) => f.properties.name && f.geometry.type === 'Point')"
                    "  .map((f) => { const p = hmMap.project(f.geometry.coordinates); return { name: f.properties.name, x: p.x, y: p.y }; })"
                    "  .filter((f) => f.x > 80 && f.y > 160 && f.x < c.width - 80 && f.y < c.height - 80)"
                    "  .filter((f) => hmMap.queryRenderedFeatures([[f.x - 3, f.y - 3], [f.x + 3, f.y + 3]], { layers: ls }).find((g) => g.properties.name)?.properties.name === f.name)"
                    "  .sort((a, b) => Math.hypot(a.x - c.width / 2, a.y - c.height / 2) - Math.hypot(b.x - c.width / 2, b.y - c.height / 2));"
                    " return fs[0] ? { name: fs[0].name, px: c.left + fs[0].x, py: c.top + fs[0].y } : null; })()") % POP_LAYERS
    hover_pop, tap_pop = "document.querySelector('.lm-poi-popup.lm-poi-hover')", "document.querySelector('.lm-poi-popup:not(.lm-poi-hover)')"
    pop_box = "(() => { const b = %s?.getBoundingClientRect(); return b ? [b.left, b.right, b.bottom] : null; })()"
    def pointer(x, y, click=False):
        call("Input.dispatchMouseEvent", type="mouseMoved", x=x, y=y)
        if click:
            for t in ("mousePressed", "mouseReleased"):
                call("Input.dispatchMouseEvent", type=t, x=x, y=y, button="left", clickCount=1)
    js("hmMap.jumpTo({ center: [8.54235, 47.37185], zoom: 18, pitch: 0, bearing: 0 })")
    time.sleep(1)
    wait("hmMap.loaded() && !hmMap.isMoving() && hmMap.areTilesLoaded()", 20)
    time.sleep(1)
    pp = js(poi_near_mid)
    if pp:
        name = json.dumps(pp["name"])
        pointer(pp["px"] - 40, pp["py"] + 40)
        pointer(pp["px"], pp["py"])
        shown = wait(f"{hover_pop}?.querySelector('strong')?.textContent === {name}", 5)
        b = js(pop_box % hover_pop)
        check(shown and js(f"!!{hover_pop}.querySelector('.lm-poi-pop-sub')?.textContent") and js("hmMap.getCanvas().style.cursor === 'pointer'")
              and b and b[2] <= pp["py"] - 4 and b[0] < pp["px"] < b[1],
              f"the mouse on a POI: a popup above it with its name and kind, the pointer a hand ({pp['name']}, {b})")
        pointer(pp["px"], pp["py"] + 400 if pp["py"] < 380 else pp["py"] - 140)
        check(wait(f"!{hover_pop} || {hover_pop}.querySelector('strong')?.textContent !== {name}", 3), "the mouse off the POI: its popup goes")
        pointer(pp["px"], pp["py"], click=True)
        b = wait(f"{tap_pop}?.querySelector('strong')?.textContent === {name}", 5) and js(pop_box % tap_pop)
        check(b and b[2] <= pp["py"] - 36 and wait(f"!document.querySelector('.lm-detail').hidden || !document.querySelector('.lm-here').hidden", 20)
              and js("document.querySelectorAll('.lm-poi-popup').length === 1"),
              f"a click on the POI: its popup above the pin, its page in the panel, one popup with the mouse still on it ({b})")
        js("document.querySelector('.lm-detail:not([hidden]) .lm-card-x, .lm-here:not([hidden]) .lm-card-x')?.click()")
        check(wait(f"!{tap_pop}", 3), "the X of the page of the POI: its popup goes")
    else:
        check(False, "a POI with a name in Zurich at zoom 18 for its popup")
    js("document.querySelector('.lm-style[data-style=auto]').click()")
    js("localStorage.removeItem('hm-recent')")
    call("Page.removeScriptToEvaluateOnNewDocument", identifier=mouse.get("identifier", ""))
    # more journeys of a visitor on a phone, each on a new page
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)
    call("Emulation.setTouchEmulationEnabled", enabled=True)
    origin = URL.split("/labs")[0]
    def fresh(url, ready="window.hmMap?.loaded()", secs=40):  # a new page, not the one before
        js("window.hmOld = 1")
        call("Page.navigate", url=url)
        return wait(f"!window.hmOld && {ready}", secs)
    def close_to(expr, lat, lon, m=10):
        return f"(() => {{ const c = {expr}; return !!c && Math.hypot((c[1] - {lat}) * 111320, (c[0] - {lon}) * 75000) < {m}; }})()"
    me_dot = "hmMap.getStyle().sources['lm-me'].data.features[1]?.geometry.coordinates"
    map_mid = "hmMap.getCenter().toArray()"
    # on a phone a tap on a POI: its popup above the pin, within the screen, none under a finger alone
    fresh(URL)
    js("hmMap.jumpTo({ center: [8.54235, 47.37185], zoom: 18, pitch: 0, bearing: 0 })")
    time.sleep(1)
    wait("hmMap.loaded() && !hmMap.isMoving() && hmMap.areTilesLoaded()", 20)
    time.sleep(1)
    pp = js(poi_near_mid)
    if pp:
        call("Input.dispatchTouchEvent", type="touchStart", touchPoints=[{"x": pp["px"], "y": pp["py"]}])
        call("Input.dispatchTouchEvent", type="touchEnd", touchPoints=[])
        shown = wait(f"{tap_pop}?.querySelector('strong')?.textContent === {json.dumps(pp['name'])}", 5)
        wait("!document.querySelector('.lm-detail').hidden || !document.querySelector('.lm-here').hidden", 20)
        time.sleep(1)
        b = js(pop_box % tap_pop)
        check(shown and b and b[0] >= 0 and b[1] <= js("innerWidth") and js(f"!{hover_pop}"),
              f"on a phone a tap on a POI: its popup within the screen, no popup of a mouse ({pp['name']}, {b})")
        js("document.querySelector('.lm-detail:not([hidden]) .lm-card-x, .lm-here:not([hidden]) .lm-card-x')?.click()")
    else:
        check(False, "a POI with a name in Zurich at zoom 18 on a phone")
    # more about a place from its website and free pictures near it (data-place): the page of Ristorante Italia (Zurich) shows
    # what the server answers (the site may change): its picture with its source linked, Menu after Website; found by its
    # name, the same on its card
    if js("!!document.getElementById('app').dataset.place"):
        ans = json.loads(js("fetch(document.getElementById('app').dataset.place + '?osm=node/327453260&pic=1').then((r) => r.text())") or "{}")
        fresh(URL + "/?q=restaurants&sel=node%2F327453260#map=18/47.37637/8.52929", "window.hmMap?.loaded() && !document.querySelector('.lm-detail').hidden")
        pic = ans.get("picture")
        shown = wait("document.querySelector('.lm-detail .lm-pic:not([hidden]) img')?.naturalWidth > 0", 20) if pic else True
        link = js("document.querySelector('.lm-detail .lm-pic:not([hidden]) figcaption a')?.href || ''")
        menu = js("document.querySelector('.lm-detail [data-t=foodMenu]')?.closest('a')?.href || ''")
        after = js("document.querySelector('.lm-detail [data-t=foodMenu]')?.closest('a')?.previousElementSibling?.textContent.trim() || ''")
        check(shown and (not pic or link == pic["page"]) and menu == ans.get("menu", "") and (not menu or after == "Website"),
              f"the page of a restaurant: the picture and menu of its website, or near it ({ans.get('site')}, {pic and pic['source']}, {menu or 'no menu'})")
        fresh(URL + "/?q=" + urllib.parse.quote("Ristorante Italia Zürich") + "#map=16/47.3763/8.5293", "window.hmMap?.loaded() && document.querySelector('.lm-place-acts')?.children.length >= 2")
        card = wait("document.querySelector('.lm-place-pic:not([hidden]) img')?.naturalWidth > 0", 20) if pic else True
        acts = js("[...document.querySelectorAll('.lm-place-acts > *')].map((e) => e.textContent.trim()).join('|')") or ""
        check(card and "Website" in acts and ("Menu" in acts) == bool(ans.get("menu")), f"a restaurant found by its name: its picture, Website and menu on its card ({acts})")
    panel = ("JSON.stringify([!document.querySelector('.lm-listview').hidden, !document.querySelector('.lm-detail').hidden, !document.querySelector('.lm-start').hidden,"
             " document.querySelector('.lm-input').value, location.search, document.querySelectorAll('.lm-listview .lm-results .lm-card').length])")

    # the location blocked in the browser: the button says so, no dot, the map stays; Directions asks for the start
    call("Browser.setPermission", permission={"name": "geolocation"}, setting="denied", origin=origin)
    fresh(URL)
    wait("!hmMap.isMoving()", 10)
    time.sleep(1.0)
    c0 = js("JSON.stringify(hmMap.getCenter())")
    js("document.querySelector('.lm-locate').click()")
    check(wait("/blocked/i.test(document.querySelector('.lm-status').textContent) && document.querySelector('.lm-status').classList.contains('lm-bad')", 10)
          and js(f"JSON.stringify(hmMap.getCenter()) === {json.dumps(c0)} && !hmMap.getStyle().sources['lm-me'].data.features.length"),
          f"the location blocked: the button of the location says so, no dot, the map stays ({js('document.querySelector(\'.lm-status\').textContent')})")
    check(js("!document.querySelector('.lm-status .lm-report')"), "the location blocked: no Report this problem (the message says what to do)")
    check(js("localStorage.getItem('hm-last-me') === null"), "the location blocked: the last location kept on the device is forgotten")
    js("document.querySelector('.lm-dir-btn').click()")
    check(wait("!document.querySelector('.lm-route').hidden && /set the start/.test(document.querySelector('.lm-route-msg').textContent)"
               " && document.querySelector('.lm-end-from input').value === ''", 10),
          "the location blocked, Directions: the start empty and asked for")
    call("Browser.grantPermissions", origin=origin, permissions=["geolocation"])

    # the map on the visitor stops going along for directions and for a search: the dot moves, the map stays
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    fresh(URL)
    wait("location.hash.includes('/47.37')", 20)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    js("document.querySelector('.lm-dir-btn').click()")
    wait("!document.querySelector('.lm-route').hidden", 5)
    time.sleep(1.0)
    wait("!hmMap.isMoving()", 10)
    c0 = js(map_mid)
    p = (ZURICH[0] + 0.0012, ZURICH[1] + 0.0010)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=10)
    moved = wait(close_to(me_dot, *p), 10)
    time.sleep(1.2)
    check(moved and js(close_to(map_mid, c0[1], c0[0], 3)), f"directions open, the visitor moves: the dot moves, the map stays ({c0}, {js(map_mid)})")
    js("document.querySelector('.lm-route .lm-back').click()")
    wait("getComputedStyle(document.querySelector('.lm-form')).display !== 'none'", 5)
    js("document.querySelector('.lm-input').value = 'cafés'")
    js("document.querySelector('.lm-input').focus()")
    key("Enter", "Enter", 13)
    found = wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelectorAll('.lm-listview .lm-results .lm-card').length > 1", 60)
    js("document.querySelector('.lm-input').blur()")
    wait("!hmMap.isMoving()", 10)
    time.sleep(1.0)
    wait("!hmMap.isMoving()", 10)
    c0 = js(map_mid)
    p = (ZURICH[0] + 0.0020, ZURICH[1] + 0.0010)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=10)
    moved = wait(close_to(me_dot, *p), 10)
    time.sleep(1.2)
    check(found and moved and js(close_to(map_mid, c0[1], c0[0], 3)), f"a search shown, the visitor moves: the dot moves, the map stays ({c0}, {js(map_mid)})")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)

    # the back button of the browser: from a result to the list, from the list to the start; forward: the search again
    if found:
        shown = js("document.querySelectorAll('.lm-listview .lm-results .lm-card').length")
        name = js("document.querySelectorAll('.lm-listview .lm-results .lm-card-name')[1].textContent")
        js("document.querySelectorAll('.lm-listview .lm-results .lm-card')[1].click()")
        picked = wait(f"!document.querySelector('.lm-detail').hidden && document.querySelector('.lm-detail .lm-dname')?.textContent === {json.dumps(name)} && /[?&]sel=/.test(location.search)", 10)
        check(picked, f"a result of a search picked: its page, the result in the address ({js(panel)})")
        wait("!hmMap.isMoving()", 10)
        js("history.back()")
        check(wait(f"!document.querySelector('.lm-listview').hidden && document.querySelector('.lm-detail').hidden && !/[?&]sel=/.test(location.search) && /[?&]q=/.test(location.search)"
                   f" && document.querySelectorAll('.lm-listview .lm-results .lm-card').length === {shown} && hmMap.getStyle().sources['lm-selected'].data.features.length === 0", 10),
              f"back from a result: the list of the search as it was, no result selected on the map ({js(panel)})")
        js("history.back()")
        check(wait("location.search === '' && document.querySelector('.lm-input').value === '' && !document.querySelector('.lm-start').hidden"
                   " && hmMap.getStyle().sources['lm-results'].data.features.length === 0 && document.getElementById('app').dataset.sheet === 'peek'", 10),
              f"back from the list: the start, the search box empty, no results on the map ({js(panel)})")
        js("history.forward()")
        check(wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelector('.lm-input').value === 'cafés' && /[?&]q=/.test(location.search)", 60),
              f"forward: the search again ({js(panel)})")
        js("document.querySelector('.lm-clear').click()")
        js("document.querySelector('.lm-input').blur()")
    else:
        check(False, "a search for cafés for the back button")

    # the compass of the app for Android (window.hmCompass; here a stand-in the test turns with hmTurn): a cone from the
    # dot of the location where the phone points; with the map on the visitor the button of the location turns the map
    # with the phone, on the dot as it moves; tapped again, north up; a finger on the map ends the turning and leaves the
    # map turned; no compass in navigation. Without a compass (a browser) no cone, and the button shows the location.
    COMPASS = ("(() => { const fns = new Set(); window.hmCompassOn = 0;"
               " window.hmCompass = { watch(fn) { fns.add(fn); window.hmCompassOn = fns.size; return () => { fns.delete(fn); window.hmCompassOn = fns.size; }; } };"
               " window.hmTurn = (h, acc = 10) => { for (const f of [...fns]) f({ heading: h, accuracy: acc }); return fns.size; }; })();")
    fake = call("Page.addScriptToEvaluateOnNewDocument", source=COMPASS)
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    fresh(URL)
    wait("location.hash.includes('/47.37')", 20)
    on_me = wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    check(on_me and wait("window.hmCompassOn === 1", 10), "the app with a compass: the page watches it while it shows the location")
    bearing = "hmMap.getBearing()"
    def bearing_near(h, d=1.5):
        return f"Math.abs(((({bearing} - {h}) % 360) + 540) % 360 - 180) < {d}"
    cone = "document.querySelector('.lm-heading')"
    cone_turn = f"parseFloat(({cone}?.style.transform.match(/rotateZ\\((-?[0-9.e-]+)deg\\)/) || [])[1])"  # on the screen, from up
    def cone_near(h, d=0.5):
        return f"Math.abs(((({cone_turn} - {h}) % 360) + 540) % 360 - 180) < {d}"
    cone_on_dot = (f"(() => {{ const r = {cone}.getBoundingClientRect(), m = document.querySelector('.lm-map').getBoundingClientRect(), p = hmMap.project({me_dot});"
                   " return Math.hypot(r.left + r.width / 2 - m.left - p.x, r.top + r.height / 2 - m.top - p.y) < 3; })()")
    label = "[document.querySelector('.lm-locate').title, document.querySelector('.lm-locate').getAttribute('aria-pressed')]"
    js("hmTurn(90, 10)")
    check(wait(f"{cone} && {cone}.dataset.heading === '90.0' && {cone_near(90)}", 5) and js(bearing_near(0)) and js(cone_on_dot),
          f"a heading: the cone from the dot points east, the map stays north up ({js(cone + '?.style.transform')})")
    check(js(label) == ["Turn the map with the phone", "false"], f"the map on the visitor: the button of the location turns the map with the phone ({js(label)})")
    tap(".lm-locate")
    check(wait("document.querySelector('.lm-locate.lm-turning') && " + bearing_near(90), 5) and js(label) == ["Turn the map with the phone", "true"],
          f"the button tapped: the map turns with the phone ({js(bearing)}, {js(label)})")
    js("hmTurn(200, 10)")
    check(wait(bearing_near(200), 5) and wait(cone_near(0, 1.5), 3), f"the phone turned: the map with it, the cone up ({js(bearing)}, {js(cone_turn)})")
    p = (ZURICH[0] + 0.0012, ZURICH[1] + 0.0015)
    call("Emulation.setGeolocationOverride", latitude=p[0], longitude=p[1], accuracy=10)
    check(wait(close_to(me_dot, *p), 10) and wait(close_to(map_mid, *p, 3), 5) and js(bearing_near(200)) and js("!!document.querySelector('.lm-locate.lm-turning')") and js(cone_on_dot),
          f"the visitor moves: the map turned with the phone on the dot, the cone with it ({js(map_mid)}, {js(bearing)})")
    # another app in front (here another tab): the page hidden, the compass off; back, the map turns with the phone again
    tab = call("Target.createTarget", url="about:blank").get("targetId")
    time.sleep(1)
    hidden = js("document.visibilityState === 'hidden' && window.hmCompassOn === 0")
    call("Page.bringToFront")
    call("Target.closeTarget", targetId=tab)
    time.sleep(1.5)
    wait("document.visibilityState === 'visible' && window.hmCompassOn === 1", 5)
    js("hmTurn(150, 10)")
    check(hidden and wait("document.querySelector('.lm-locate.lm-turning') && " + bearing_near(150), 5),
          f"the page hidden a while: the compass off; back, the map turns with the phone again ({hidden}, {js(bearing)})")
    tap(".lm-locate")
    check(wait("!document.querySelector('.lm-locate.lm-turning') && !hmMap.isMoving() && " + bearing_near(0, 0.5), 5) and js(label) == ["Turn the map with the phone", "false"],
          f"tapped again: north up, the map on the visitor still ({js(bearing)}, {js(label)})")
    js("hmTurn(250, 10)")
    time.sleep(0.6)
    check(js(bearing_near(0, 0.5)) and js(cone_near(250)), f"north up: the cone turns with the phone, the map not ({js(bearing)})")
    tap(".lm-locate")
    turning = wait(bearing_near(250), 5)
    drag(195, 260, 330)
    time.sleep(0.5)
    b0 = js(bearing)
    js("hmTurn(300, 10)")
    time.sleep(0.8)
    b1 = js(bearing)
    check(turning and js("!document.querySelector('.lm-locate.lm-turning')") and abs((b1 - b0 + 540) % 360 - 180) < 0.5 and abs((b0 - 250 + 540) % 360 - 180) < 3
          and js(label) == ["Show my location", None],
          f"a finger moves the map: it stops turning with the phone and stays turned ({b0}, {b1}, {js(label)})")
    tap(".lm-locate")
    check(wait("!hmMap.isMoving() && " + close_to(map_mid, *p, 15), 10) and js("!document.querySelector('.lm-locate.lm-turning')")
          and wait(f"{json.dumps(['Turn the map with the phone', 'false'])}.join() === {label}.join()", 3),
          f"then the button: the map on the visitor again, not turning yet ({js(label)})")
    # navigation: no compass and no cone; after it both again
    js("localStorage.setItem('hm-mute', '1')")
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    fresh(URL + "/?from=47.37790,8.54030&fn=Zurich+HB&to=47.37650,8.54400&tn=Central&mode=car")
    if wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && document.querySelector('.lm-nav-go .lm-chip') && window.hmCompassOn === 1", 40):
        js("hmTurn(45, 10)")
        wait(f"{cone}", 3)
        js("document.querySelector('.lm-nav-go .lm-chip').click()")
        on = wait("document.getElementById('app').classList.contains('lm-navigating')", 5)
        check(on and wait(f"window.hmCompassOn === 0 && !{cone}", 5), f"navigation: the compass off, no cone ({js('window.hmCompassOn')})")
        js("document.querySelector('.lm-nav-x')?.click()")
        off = wait("!document.getElementById('app').classList.contains('lm-navigating')", 5)
        js("hmTurn(60, 10)")
        check(off and wait(f"window.hmCompassOn === 1 && {cone}?.dataset.heading === '60.0'", 5), f"navigation ended: the compass and the cone again ({js('window.hmCompassOn')})")
    else:
        check(False, "a route for navigation with the compass")
    js("localStorage.removeItem('hm-mute')")
    call("Page.removeScriptToEvaluateOnNewDocument", identifier=fake.get("identifier", ""))
    fresh(URL)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    time.sleep(0.5)
    check(js(f"!window.hmCompass && !{cone}") and js(label) == ["Show my location", None],
          f"a phone browser without an orientation sensor: no cone, the button shows the location ({js(label)})")

    # the compass of a phone browser: Chrome and the others on Android, here with a virtual orientation sensor of Chromium
    # (a quaternion from the axes of the phone to east, north, up): the cone toward the top of the phone from true north
    # (magnetic north and the declination of wmm.js), held flat, tilted, in landscape; the button turns the map with it.
    # Safari on the iPhone (a stand-in: its compass heading, its question allowed only from a tap): nothing until a tap of
    # the button with the map on the visitor; then the map turns with the first heading; refused, the button shows the
    # location. On a computer no compass.
    def q_about(ax, deg):
        h = math.radians(deg) / 2
        q = [0.0, 0.0, 0.0, math.cos(h)]
        q[ax] = math.sin(h)
        return q
    def q_then(a, b):  # a, then b about the axes of the world
        return [b[3] * a[0] + b[0] * a[3] + b[1] * a[2] - b[2] * a[1], b[3] * a[1] - b[0] * a[2] + b[1] * a[3] + b[2] * a[0],
                b[3] * a[2] + b[0] * a[1] - b[1] * a[0] + b[2] * a[3], b[3] * a[3] - b[0] * a[0] - b[1] * a[1] - b[2] * a[2]]
    def pose(q):
        call("Emulation.setSensorOverrideReadings", type="absolute-orientation", reading={"quaternion": {"x": q[0], "y": q[1], "z": q[2], "w": q[3]}})
    def cone_at(h, d=1.0):
        return f"(() => {{ const c = document.querySelector('.lm-heading'); return !!c && Math.abs(((+c.dataset.heading - {h}) % 360 + 540) % 360 - 180) < {d}; }})()"
    def bearing_at(h, d=1.5):
        return f"Math.abs(((hmMap.getBearing() - {h}) % 360 + 540) % 360 - 180) < {d}"
    cone_h = "document.querySelector('.lm-heading')?.dataset.heading"
    label = "[document.querySelector('.lm-locate').title, document.querySelector('.lm-locate').getAttribute('aria-pressed')]"
    origin = URL.split("/labs")[0]
    call("Browser.grantPermissions", origin=origin, permissions=["geolocation", "sensors"])
    call("Emulation.setSensorOverrideEnabled", enabled=True, type="absolute-orientation", metadata={"available": True})
    pose(q_about(2, -90))  # flat, the top to east
    call("Emulation.setGeolocationOverride", latitude=ZURICH[0], longitude=ZURICH[1], accuracy=20)
    fresh(URL)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    decl = js(f"import({json.dumps(URL + '/static/wmm.js')}).then((m) => m.declination({ZURICH[0]}, {ZURICH[1]}))")
    check(isinstance(decl, (int, float)) and 2 < decl < 6 and wait(cone_at(90 + decl), 10) and js(label) == ["Turn the map with the phone", "false"],
          f"a phone browser with an orientation sensor (Chrome): the cone toward the top of the phone from true north, flat to east ({js(cone_h)}, declination {decl})")
    decl = decl if isinstance(decl, (int, float)) else 0
    pose(q_then(q_about(0, 80), q_about(2, 90)))  # held up nearly upright, the back to west
    check(wait(cone_at(270 + decl), 5), f"held up, the back to west: the cone to west ({js(cone_h)})")
    tap(".lm-locate")
    check(wait("document.querySelector('.lm-locate.lm-turning') && " + bearing_at(270 + decl), 5), f"the button: the map turns with the phone ({js('hmMap.getBearing()')})")
    pose(q_then(q_about(0, 30), q_about(2, -150)))  # tilted, to south-south-east
    check(wait(bearing_at(150 + decl), 5), f"the phone tilted and turned to south-south-east: the map with it ({js('hmMap.getBearing()')})")
    tap(".lm-locate")
    wait("!document.querySelector('.lm-locate.lm-turning') && !hmMap.isMoving()", 5)
    # landscape, turned counterclockwise: the right side of the phone is the top of the screen; flat with its top to north
    pose([0, 0, 0, 1])
    upright = wait(cone_at(decl), 5)
    call("Emulation.setDeviceMetricsOverride", width=844, height=390, deviceScaleFactor=2, mobile=True, screenOrientation={"type": "landscapePrimary", "angle": 90})
    check(upright and wait(cone_at(90 + decl), 5),
          f"the screen turned to landscape, the phone still with its top to north: the cone to east at once, where the top of the screen points ({js(cone_h)}, {js('screen.orientation.angle')})")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)
    call("Emulation.setSensorOverrideEnabled", enabled=False, type="absolute-orientation")

    # Safari: no deviceorientationabsolute, its compass heading in deviceorientation, once allowed from a tap
    SAFARI = ("(() => { delete window.ondeviceorientationabsolute; delete Window.prototype.ondeviceorientationabsolute; window.hmAsked = 0; window.hmAllowed = false;"
              " DeviceOrientationEvent.requestPermission = () => { hmAsked++;"
              "  if (window.hmRefuse) return Promise.resolve('denied');"
              "  if (!navigator.userActivation.isActive) return Promise.reject(new DOMException('a tap first', 'NotAllowedError'));"
              "  hmAllowed = true; return Promise.resolve('granted'); };"
              " window.hmSafari = (h, acc) => { if (!hmAllowed) return false; const e = new Event('deviceorientation');"
              "  Object.assign(e, { alpha: 10, beta: 20, gamma: 0, absolute: false, webkitCompassHeading: h, webkitCompassAccuracy: acc }); dispatchEvent(e); return true; }; })();")
    safari = call("Page.addScriptToEvaluateOnNewDocument", source=SAFARI)
    fresh(URL)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    time.sleep(0.5)
    check(js("!('ondeviceorientationabsolute' in window) && !hmAllowed && !hmSafari(10, 5) && !document.querySelector('.lm-heading')")
          and js(label) == ["Turn the map with the phone", "false"],
          f"Safari: no cone before it is allowed; the button with the map on the visitor offers to turn the map ({js(label)}, asked {js('hmAsked')})")
    tap(".lm-locate")
    allowed = wait("hmAllowed", 5)
    js("hmSafari(120, 12)")
    check(allowed and wait("document.querySelector('.lm-locate.lm-turning') && " + bearing_at(120 + decl), 5) and wait(cone_at(120 + decl), 3),
          f"Safari: the tap asks; allowed, the map turns with its first heading, from true north ({js('hmMap.getBearing()')}, {js(cone_h)})")
    js("hmSafari(200, 12)")
    check(wait(bearing_at(200 + decl), 5), f"Safari: the phone turned, the map with it ({js('hmMap.getBearing()')})")
    tap(".lm-locate")
    check(wait("!document.querySelector('.lm-locate.lm-turning') && !hmMap.isMoving() && " + bearing_at(0, 0.5), 5), "Safari: tapped again, north up")
    # refused: the button shows the location again, a tap moves the map to the visitor
    fresh(URL)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    js("window.hmRefuse = true")
    tap(".lm-locate")
    check(wait("hmAsked >= 1", 5) and wait(f"{label}.join() === 'Show my location,'", 5) and not js("!!document.querySelector('.lm-locate.lm-turning')"),
          f"Safari: refused, no turning, the button shows the location ({js(label)})")
    call("Page.removeScriptToEvaluateOnNewDocument", identifier=safari.get("identifier", ""))
    call("Browser.grantPermissions", origin=origin, permissions=["geolocation"])
    # a computer: no touch screen, no compass (the button shows the location)
    call("Emulation.setTouchEmulationEnabled", enabled=False)
    call("Emulation.setDeviceMetricsOverride", width=1280, height=800, deviceScaleFactor=1, mobile=False)
    fresh(URL)
    wait("!hmMap.isMoving() && hmMap.loaded() && " + close_to(map_mid, *ZURICH, 15), 20)
    time.sleep(0.5)
    check(js("navigator.maxTouchPoints === 0 && !document.querySelector('.lm-heading')") and js(label) == ["Show my location", None],
          f"a computer: no compass ({js(label)})")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)
    call("Emulation.setTouchEmulationEnabled", enabled=True)

    # a shared route with more stops than a route takes (9): the first 8, numbered, a route through them, no stop more
    stops9 = "&".join(f"via={47.3779 - 0.0011 * (i + 1):.5f},{8.5403 + 0.0005 * (i + 1):.5f}&vn=Stop+{i + 1}" for i in range(9))
    fresh(URL + f"/?from=47.37790,8.54030&fn=Zurich+HB&{stops9}&to=47.36690,8.54500&tn=Bellevue&mode=walk")
    check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && document.querySelectorAll('.lm-end-stop').length === 8"
               " && document.querySelectorAll('.lm-step-stop').length === 8", 60)
          and js("[...document.querySelectorAll('.lm-end-stop input')].map(e => e.value).join() === 'Stop 1,Stop 2,Stop 3,Stop 4,Stop 5,Stop 6,Stop 7,Stop 8'"
                 " && document.querySelector('.lm-end-to input').value === 'Bellevue' && !document.querySelector('.lm-add-stop')"),
          f"a shared route with 9 stops: the first 8 in their order, a route through them, no button for a stop more"
          f" ({js('JSON.stringify([document.querySelectorAll(\'.lm-end-stop\').length, document.querySelector(\'.lm-route-sum\')?.textContent, !!document.querySelector(\'.lm-add-stop\')])')})")
    js("document.querySelector('.lm-end-remove').click()")
    check(wait("document.querySelectorAll('.lm-end-stop').length === 7 && document.querySelectorAll('.lm-step-stop').length === 7 && !!document.querySelector('.lm-add-stop')"
               " && document.querySelector('.lm-end-stop input').value === 'Stop 2'", 40),
          "a stop removed: 7 stops, a new route, the button for a stop is back")

    # Preview to its end, the voice off: You have arrived, navigation ends by itself, the route and Start are back, the map flat
    js("localStorage.setItem('hm-mute', '1')")
    fresh(URL + "/?from=47.37790,8.54030&fn=Zurich+HB&to=47.37650,8.54400&tn=Central&mode=car")
    if wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && document.querySelector('.lm-nav-go .lm-chip')", 40):
        js("document.querySelector('.lm-nav-go .lm-chip').click()")
        on = wait("document.getElementById('app').classList.contains('lm-navigating')", 5)
        check(on and wait("/arrived/i.test(document.querySelector('.lm-nav-banner')?.textContent || '')", 90), "Preview of a short route by car, to its end: You have arrived")
        check(wait("!document.getElementById('app').classList.contains('lm-navigating')", 25)
              and wait("!document.querySelector('.lm-route').hidden && document.querySelector('.lm-route-sum')?.textContent.includes('min') && hmMap.getPitch() < 1"
                       " && (() => { const b = document.querySelector('.lm-nav-start')?.getBoundingClientRect(); return !!b && b.top >= 0 && b.bottom <= innerHeight; })()", 10),
              f"after the arrival of Preview: navigation ends by itself, the route with Start in view again, the map flat ({js('hmMap.getPitch()')})")
        js("document.querySelector('.lm-route .lm-back').click()")
    else:
        check(False, "a short route by car for Preview to its end")
    js("localStorage.removeItem('hm-mute')")
    js("localStorage.removeItem('hm-recent')")
    # a route by car with a ferry on a small phone: the warning above Start, and Start still in view
    call("Emulation.setDeviceMetricsOverride", width=360, height=640, deviceScaleFactor=2, mobile=True)
    fresh(URL + "/?from=47.25970,8.59770&fn=Horgen&to=47.27000,8.64300&tn=Meilen&mode=car")
    if wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && !!document.querySelector('.lm-ferries .lm-ferry-ferry')", 40):
        time.sleep(1.0)
        g = json.loads(js("JSON.stringify((() => { const r = (s) => { const b = document.querySelector(s).getBoundingClientRect(); return [Math.round(b.top), Math.round(b.bottom)]; };"
                          " return { modes: r('.lm-modes'), ferry: r('.lm-ferries'), start: r('.lm-nav-start'), side: r('.lm-side'), h: innerHeight, scroll: document.querySelector('.lm-body').scrollTop }; })())"))
        check(g["scroll"] == 0 and g["modes"][0] >= g["side"][0] and g["ferry"][1] <= g["start"][0] and g["start"][1] <= g["h"],
              f"a route with a ferry on 360 by 640: car, bike and walk, the warning and Start in view ({g})")
    else:
        check(False, "a route by car with a ferry on a small phone")
    call("Emulation.setDeviceMetricsOverride", width=390, height=844, deviceScaleFactor=2, mobile=True)

    # a shared place on a phone: its pin in the part of the map above the panel, clear of the credits at its edge
    fresh(URL + "/?at=47.36690,8.54500&n=Bellevue#map=17/47.36690/8.54500")
    if wait("!document.querySelector('.lm-here').hidden || !document.querySelector('.lm-detail').hidden", 40):
        wait("!hmMap.isMoving()", 10)
        time.sleep(1.5)
        g = json.loads(js("JSON.stringify((() => { const p = hmMap.project([8.545, 47.3669]), m = document.querySelector('.lm-map').getBoundingClientRect();"
                          " const a = document.querySelector('.maplibregl-ctrl-attrib').getBoundingClientRect();"
                          " return { pin: [Math.round(p.x + m.left), Math.round(p.y + m.top)], panel: Math.round(document.querySelector('.lm-side').getBoundingClientRect().top), credits: [Math.round(a.left), Math.round(a.top)] }; })())"))
        check(g["pin"][1] < g["panel"] - 60 and not (g["pin"][0] >= g["credits"][0] and g["pin"][1] >= g["credits"][1] - 10),
              f"a shared place on a phone: the place on the map above the panel, not under the credits ({g})")
    else:
        check(False, "a shared place opens on a phone")

    # coordinates written in the search box: the map at that point, not at a place found by the numbers
    fresh(URL + "/#map=12/46.9480/7.4474")
    js("document.querySelector('.lm-input').value = '47.3769, 8.5417'")
    js("document.querySelector('.lm-input').focus()")
    key("Enter", "Enter", 13)
    wait(r"!!document.querySelector('.lm-title').textContent || !document.querySelector('.lm-here').hidden || /\d/.test(document.querySelector('.lm-count').textContent)"
         r" || (document.querySelector('.lm-status').textContent.length > 3 && !/…$/.test(document.querySelector('.lm-status').textContent))", 60)
    wait("!hmMap.isMoving()", 10)
    time.sleep(1.5)
    wait("!hmMap.isMoving()", 10)
    c = js("hmMap.getCenter().toArray().map((v) => +v.toFixed(4))")
    check(abs(c[1] - 47.3769) < 0.02 and abs(c[0] - 8.5417) < 0.02,
          f"47.3769, 8.5417 written in the search box: the map at that point in Zurich (the map at {c[1]}, {c[0]}: {js('document.querySelector(\'.lm-title\').textContent')})")
    js("document.querySelector('.lm-clear').click()")
    js("document.querySelector('.lm-input').blur()")
    js("localStorage.removeItem('hm-recent')")

    # the Outdoor style keeps the POIs with a name: restaurants and cafés of an old town, beside its own huts and peaks
    js("hmMap.jumpTo({ center: [8.5437, 47.3725], zoom: 17, pitch: 0, bearing: 0 })")
    named = "hmMap.queryRenderedFeatures({ layers: ['pois'] }).filter((f) => f.properties.name).map((f) => f.properties.kind)"
    counts = {}
    for style in ("light", "outdoor"):
        js(f"document.querySelector('.lm-style[data-style={style}]').click()")
        wait(f"performance.getEntriesByType('resource').some(e => e.name.includes('styles/{style}'))", 15)
        time.sleep(1.0)
        wait(f"hmMap.loaded() && hmMap.isStyleLoaded() && !hmMap.isMoving() && (hmMap.getLayer('pois_outdoor') ? {'true' if style == 'outdoor' else 'false'} : {'false' if style == 'outdoor' else 'true'}) && {named}.length > 10", 20)
        counts[style] = json.loads(js(f"JSON.stringify({named})"))
    check(len(counts["light"]) > 10 and len(counts["outdoor"]) > 10 and "restaurant" in counts["outdoor"],
          f"the Outdoor style in the old town of Zurich: restaurants and other POIs with a name, as in the light style ({len(counts['light'])} light, {len(counts['outdoor'])} outdoor)")
    js("document.querySelector('.lm-style[data-style=auto]').click()")
    # public transport, the mode Transit of directions (where the server has timetables): a shared journey from
    # Bellevue in Zurich to the Zytglogge in Bern
    if js("!!document.getElementById('app').dataset.journey"):
        fresh(URL + "/?from=47.36620,8.54860&fn=Bellevue&to=46.94800,7.44740&tn=Zytglogge&mode=transit")
        row_times = "[...document.querySelectorAll('.lm-trip-row .lm-trip-times')].map(e => e.textContent)"
        if wait("document.querySelectorAll('.lm-trip').length > 0 && !!document.querySelector('.lm-trip-on .lm-legs') && !hmMap.isMoving()", 60):
            time.sleep(1.5)
            g = json.loads(js("JSON.stringify((() => { const q = (s) => document.querySelector(s), all = (s) => [...document.querySelectorAll(s)];"
                              " const fs = hmMap.getStyle().sources['lm-journey'].data.features, rides = fs.filter(f => f.properties.colour), b = (s) => q(s).getBoundingClientRect();"
                              " return { modes: all('.lm-mode').map(m => m.getAttribute('aria-checked')), times: all('.lm-trip-row .lm-trip-times').map(e => e.textContent),"
                              " lines: all('.lm-trip-on .lm-trip-row .lm-line').map(e => e.textContent), legs: q('.lm-trip-on .lm-legs').innerText.replace(/\\s+/g, ' '),"
                              " rides: all('.lm-trip-on .lm-leg-ride').length, tracks: all('.lm-trip-on .lm-leg-track').length, start: !!q('.lm-nav-start'), stop: !!q('.lm-add-stop'), fields: all('.lm-end input').length,"
                              " drawn: { rides: rides.length, points: Math.max(0, ...rides.map(f => f.geometry.coordinates.length)), colours: rides.every(f => /^#[0-9a-f]{6}$/.test(f.properties.colour)),"
                              " walks: fs.filter(f => f.properties.walk).length, stops: fs.filter(f => f.properties.stop).length, names: fs.filter(f => f.properties.name).map(f => f.properties.name) },"
                              " row: Math.round(b('.lm-trip-row').bottom), modesTop: Math.round(b('.lm-modes').top), side: Math.round(b('.lm-side').top), h: innerHeight, scroll: q('.lm-body').scrollTop }; })())"))
            check(g["modes"] == ["false", "false", "false", "true"] and len(g["times"]) >= 2 and all(re.fullmatch(r"\d\d:\d\d – \d\d:\d\d", t) for t in g["times"])
                  and len(g["lines"]) >= 1 and all(g["lines"]) and g["rides"] == len(g["lines"]) and g["tracks"] >= 2 and "Bern" in g["legs"] and "Zytglogge" in g["legs"]
                  and not g["start"] and not g["stop"] and g["fields"] == 2,
                  f"a shared journey by public transport: {len(g['times'])} connections with their times, the first with {', '.join(g['lines'])}, its legs open with stops and tracks; no Start, no stops on the way ({g['legs'][:90]})")
            d = g["drawn"]
            check(d["rides"] == g["rides"] and d["colours"] and d["points"] > 500 and d["walks"] >= 2 and d["stops"] == 2 * d["rides"] and "Bern" in d["names"],
                  f"the journey on the map: each ride in the colour of its line along the rails ({d['points']} points), the walks dotted, the stops with their names ({d})")
            check(g["scroll"] == 0 and g["modesTop"] >= g["side"] and g["row"] <= g["h"],
                  f"on a phone the first connection is in view under the modes, the start and the destination ({ {k: g[k] for k in ('side', 'modesTop', 'row', 'h', 'scroll')} })")
            # Later: the connections after these
            before = js(row_times) or [""]
            js("document.querySelector('.lm-later').click()")
            first_row = "document.querySelector('.lm-trip-row .lm-trip-times')?.textContent"
            changed = wait(f"!!{first_row} && {first_row} !== {json.dumps(before[0])}", 30)
            after = js(row_times) or []
            check(changed and len(after) > 0 and not set(after) & set(before) and js("!!document.querySelector('.lm-earlier')"),
                  f"Later: the connections after those shown, none of them again ({before[0]} first before, {after[0] if after else 'none'} after)")
            # while connections are searched for: a turning ring before the message and three rows that pulse in place
            # of the list; gone with a failure (the message of it) and with the answer (its rows). The page holds each
            # request of /api/journey until hmHold(fail) lets it go (or answers it with a failure)
            js("(() => { const f = window.hmFetch = window.fetch; window.fetch = function (u, ...a) { if (!String(u).includes('/api/journey')) return f.call(this, u, ...a);"
               " return new Promise((ok) => { window.hmHold = (fail) => ok(fail ? new Response('{}', { status: 502, headers: { 'Content-Type': 'application/json' } }) : f.call(this, u, ...a)); }); }; })()")
            busy = ("(() => { const q = (s) => document.querySelector(s), ring = q('.lm-route-msg .lm-spinner'), rows = document.querySelectorAll('.lm-trips-wait .lm-trip');"
                    " return !!ring && getComputedStyle(ring).animationName === 'lm-spin' && q('.lm-route-msg').textContent === 'Finding connections…'"
                    " && rows.length === 3 && q('.lm-trips-wait').getAttribute('aria-hidden') === 'true' && getComputedStyle(q('.lm-wait-bar')).animationName === 'lm-pulse'"
                    " && !document.querySelector('.lm-trip-row .lm-trip-times'); })()")
            done = "!document.querySelector('.lm-route-msg .lm-spinner') && !document.querySelector('.lm-trips-wait')"
            js("window.hmHold = null; (() => { const k = document.querySelector('.lm-when-kind'); k.value = 'depart'; k.dispatchEvent(new Event('change', { bubbles: true })); })()")
            waiting = wait("!!window.hmHold && " + busy, 10)
            js("window.hmHold?.(true)")
            failed = wait(done + " && document.querySelector('.lm-route-msg').textContent.length > 0", 10)
            js("window.hmHold = null; (() => { const t = document.querySelector('.lm-when-time'); t.dispatchEvent(new Event('change', { bubbles: true })); })()")
            again = wait("!!window.hmHold && " + busy, 10)
            js("window.hmHold?.(false)")
            answered = wait(done + " && document.querySelectorAll('.lm-trip-row .lm-trip-times').length > 0 && document.querySelector('.lm-route-msg').textContent === ''", 40)
            js("window.fetch = window.hmFetch")
            check(waiting and failed and again and answered,
                  f"connections searched for: a turning ring before the message and three pulsing rows; gone with a failure and with the answer ({waiting}, {failed}, {again}, {answered})")
            # an arrival tomorrow by 09:00: on the clocks of the journey (Zurich), whatever the clock of the browser
            import datetime, zoneinfo
            day = datetime.date.today() + datetime.timedelta(days=1)
            utc = datetime.datetime.combine(day, datetime.time(9), zoneinfo.ZoneInfo("Europe/Zurich")).astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:00Z")
            js("(() => { const k = document.querySelector('.lm-when-kind'); k.value = 'arrive'; k.dispatchEvent(new Event('change', { bubbles: true }));"
               f" const t = document.querySelector('.lm-when-time'); t.value = '{day.isoformat()}T09:00'; t.dispatchEvent(new Event('change', {{ bubbles: true }})); }})()")
            asked = wait("performance.getEntriesByType('resource').some(e => e.name.includes('/api/journey') && e.name.includes('arrive=1') && e.name.includes('time=" + utc.replace(":", "%3A") + "'))", 20)
            shown = wait("!!document.querySelector('.lm-trip-day') && document.querySelectorAll('.lm-trip').length > 0", 30)
            ends = [t.split(" – ")[1] for t in (js(row_times) or [])]
            check(asked and shown and ends and all(e <= "09:00" for e in ends) and not js("document.querySelector('.lm-when-time').hidden"),
                  f"an arrival tomorrow by 09:00 in Zurich: asked as {utc}, every connection there by then ({ends}), under the day ({js('document.querySelector(\'.lm-trip-day\')?.textContent')})")
            # by car again: the route, Start, nothing of the journey left on the map or in the panel
            js("window.hmRing = false; new MutationObserver(() => { if (document.querySelector('.lm-route-msg .lm-spinner')) window.hmRing = true; })"
               ".observe(document.body, { subtree: true, childList: true })")
            js("document.querySelector('.lm-mode').click()")
            check(wait("document.querySelector('.lm-route-sum')?.textContent.includes('min') && !!document.querySelector('.lm-nav-start')", 40)
                  and js("!hmMap.getStyle().sources['lm-journey'].data.features.length && !document.querySelector('.lm-journey').childElementCount && !document.querySelector('.lm-when')"),
                  "from Transit to Car: the route by car with Start, no connection left in the panel or on the map")
            check(js("window.hmRing && !document.querySelector('.lm-route-msg .lm-spinner')"), "the route by car searched for: the turning ring before its message, gone with the answer")
        else:
            check(False, f"a shared journey by public transport opens its connections ({js('document.querySelector(\'.lm-route-msg\')?.textContent')})")
        # no timetable near the start (N'Djamena): the message names where there is one (the countries, or Europe), in
        # the language of the page
        fresh(URL + "/?from=12.13480,15.05570&fn=N%27Djamena&to=12.11000,15.09000&tn=Farcha&mode=transit")
        msg = "document.querySelector('.lm-route-msg')?.textContent || ''"
        # (a part of the world, 150, by the name of the page: this browser names countries only)
        names = ("document.getElementById('app').dataset.journeyRegions.split(',').map(c => c === '150' ? { en: 'Europe', de: 'Europa' }[document.documentElement.lang]"
                 " : new Intl.DisplayNames([document.documentElement.lang], { type: 'region' }).of(c)).join(', ')")
        en_names = js(names)
        en = wait(f"({msg}).includes({json.dumps(en_names)})", 40) and js(msg)
        page_lang("de")
        de_names = js(names)
        de = wait(f"({msg}).includes({json.dumps(de_names)})", 20) and js(msg)
        check(bool(en) and bool(en_names) and "start" in en and bool(de) and de_names != en_names and "Fahrpläne" in de and js("document.querySelectorAll('.lm-mode')[3].title") == "ÖV",
              f"a start without a timetable: the message names where there is one, in German after a change of language ({en} / {de})")
        page_lang("en")
        js("localStorage.removeItem('hm-lang')")
    # feedback (data-feedback): the row of the menu opens the form in place of the menu; "Wrong on the map" links a note
    # of OpenStreetMap at the map view; an empty message is not sent; a message (answered in the page: nothing reaches
    # the server, the test runs every night) carries its kind, text, address, language and, with the details on, a link
    # to the page with the map view, the browser and the errors, as "What is sent" lists them; without them none; the
    # form in German after a change of language; "Report this problem" under a search that failed opens the form with
    # Problem chosen and the failure in the errors
    if js("!!document.getElementById('app').dataset.feedback"):
        fresh(URL)
        js("""(() => { window.hmFb = []; const f = window.fetch; window.hmFetchFb = f;
          window.fetch = function (u, o) {
            if (String(u).includes('/api/feedback')) { hmFb.push(JSON.parse(o.body)); return Promise.resolve(new Response(null, { status: 204 })); }
            if (window.hmFailSearch && String(u).includes('/search?')) return Promise.resolve(window.hmFailSearch === 'zoom'
              ? new Response('{"error":{"code":"zoom_in","message":"zoom in"}}', { status: 400 }) : new Response('{}', { status: 502 }));
            return f.apply(this, arguments); }; })()""")
        def fb_tap(sel):  # a finger on it, once scrolled into the menu (the list of What is sent pushes Send down)
            js(f"document.querySelector({json.dumps(sel)})?.scrollIntoView({{ block: 'center' }})")
            time.sleep(0.2)
            return tap(sel)
        def fb_open():
            js("document.querySelector('.lm-map-burger').click()")
            wait("!document.querySelector('.lm-menu').hidden", 3)
            time.sleep(0.4)  # the menu slides in
            fb_tap(".lm-fb-row")
            return wait("!document.querySelector('.lm-fb').hidden && document.querySelector('.lm-menu-main').hidden && !document.querySelector('.lm-menu').hidden", 5)
        check(fb_open() and js("document.querySelectorAll('.lm-fb-kinds [role=radio]').length") == 4,
              "Feedback in the menu: the form in place of the menu, four kinds")
        fb_tap(".lm-fb-kinds [data-kind=map]")
        osm = js("document.querySelector('.lm-fb-osm:not([hidden]) a')?.href || ''")
        check(bool(re.match(r"^https://www\.openstreetmap\.org/note/new#map=\d+/-?\d+\.\d{5}/-?\d+\.\d{5}$", osm))
              and js("document.querySelector('.lm-fb-text').placeholder") == "What is wrong, and where?",
              f"wrong on the map: its question, and a note of OpenStreetMap at the map view ({osm})")
        fb_tap(".lm-fb-send")
        time.sleep(0.5)
        check(js("hmFb.length") == 0 and js("document.querySelector('.lm-fb-done').hidden"), "an empty message is not sent")
        js("document.querySelector('.lm-fb-text').value = 'a test of tests/browser.py'; document.querySelector('.lm-fb-email').value = 'test@example.invalid';"
           " document.querySelector('.lm-fb-what').open = true")
        listed = js("document.querySelector('.lm-fb-what-list').textContent") or ""
        fb_tap(".lm-fb-send")
        sent = wait("hmFb.length === 1 && !document.querySelector('.lm-fb-done').hidden", 5) and js("hmFb[0]") or {}
        ctx = sent.get("ctx") or {}
        check(sent.get("kind") == "map" and sent.get("text") == "a test of tests/browser.py" and sent.get("email") == "test@example.invalid"
              and sent.get("lang") == "en" and ctx.get("link", "").startswith(URL + "/") and "#map=" in ctx.get("link", "") and "Chrome" in ctx.get("agent", "")
              and bool(re.match(r"^\d+x\d+@", ctx.get("screen", ""))) and isinstance(ctx.get("errors"), list) and bool(ctx.get("style")),
              f"a message sent: its kind, text, address, language, the page with the map view, the browser ({json.dumps(sent)[:300]})")
        check("#map=" in listed and "Chrome" in listed, "What is sent lists the page and the browser")
        fb_tap(".lm-fb-done .lm-chip")
        check(wait("document.querySelector('.lm-menu').hidden", 3), "Close after the thanks: the menu goes")
        # again, the details off: none sent; the form empty and back from the thanks
        check(fb_open() and js("document.querySelector('.lm-fb-text').value === '' && !document.querySelector('.lm-fb-form').hidden && document.querySelector('.lm-fb-done').hidden"),
              "the form again: empty, not the thanks")
        js("document.querySelector('.lm-fb-details input').click(); document.querySelector('.lm-fb-text').value = 'no details'")
        fb_tap(".lm-fb-send")
        sent = wait("hmFb.length === 2", 5) and js("hmFb[1]") or {}
        check(sent.get("kind") == "other" and sent.get("text") == "no details" and "ctx" not in sent and js("document.querySelector('.lm-fb-what').hidden"),
              f"the details off: none sent, no list; no kind chosen: other ({sent})")
        js("document.querySelector('.lm-fb-done .lm-chip').click()")
        js("document.querySelector('.lm-fb-details input').click()")  # the details on again (the switch keeps the choice of the visitor)
        # in German
        page_lang("de")
        fb_open()
        check(js("document.querySelector('.lm-fb-label').textContent") == "Worum geht es?" and js("document.querySelector('.lm-fb-send').textContent") == "Senden",
              "the form in German")
        js("document.querySelector('.lm-backdrop').click()")
        page_lang("en")
        js("localStorage.removeItem('hm-lang')")
        # a search that failed: Report this problem, the form with Problem and the failure in the errors
        js("window.hmFailSearch = true; document.querySelector('.lm-input').value = 'bakeries in Bern'; document.querySelector('.lm-form').requestSubmit()")
        rep = wait("!!document.querySelector('.lm-status:not([hidden]) .lm-report')", 20)
        js("window.hmFailSearch = false; document.querySelector('.lm-status .lm-report')?.click()")
        check(rep and wait("!document.querySelector('.lm-fb').hidden && document.querySelector('.lm-fb-kinds [data-kind=bug]').getAttribute('aria-checked') === 'true'", 5)
              and js("/search 502/.test(document.querySelector('.lm-fb-what-list').textContent) && /shown: The search failed/.test(document.querySelector('.lm-fb-what-list').textContent)"),
              "a search that failed: Report this problem opens the form with Problem chosen and the failure in the errors")
        js("document.querySelector('.lm-backdrop').click()")
        # a search the server answers with what to do (zoom in): no Report this problem
        js("window.hmFailSearch = 'zoom'; document.querySelector('.lm-input').value = 'benches'; document.querySelector('.lm-form').requestSubmit()")
        zoom = wait("/Zoom in/.test(document.querySelector('.lm-status:not([hidden])')?.textContent || '')", 20)
        check(zoom and js("!document.querySelector('.lm-status .lm-report')"), "a search answered with what to do (zoom in): no Report this problem")
        js("window.hmFailSearch = false; window.fetch = window.hmFetchFb")
    else:
        print("  (no feedback on this server)", flush=True)
    real = [e for e in errors if "Map cannot fit" not in e]
    check(not real, f"no errors of the page{': ' + real[0] if real else ''}")
finally:
    chrome.terminate()
    shutil.rmtree(prof, ignore_errors=True)
sys.exit(1 if fails else 0)
