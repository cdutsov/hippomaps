#!/usr/bin/env python3
"""Phone screenshots of hippomaps.ch for the store listings (F-Droid, Google Play):
   fastlane/screenshots.py fastlane/metadata/android/en-US/images/phoneScreenshots en"""
import base64, json, os, shutil, subprocess, sys, tempfile, time, urllib.request
import websocket

OUT = sys.argv[1]
LANG = sys.argv[2] if len(sys.argv) > 2 else "en"
URL = "https://hippomaps.ch"
PORT = 9351
W, H, DPR = 360, 640, 3   # 1080 x 1920: 9:16, Google Play takes at most 2:1

prof = tempfile.mkdtemp(prefix="hm-shots-")
chrome = subprocess.Popen(["chromium", "--headless=new", "--no-sandbox", "--disable-gpu", "--use-angle=swiftshader", "--enable-unsafe-swiftshader",
                           f"--remote-debugging-port={PORT}", f"--user-data-dir={prof}", f"--lang={LANG}",
                           f"--window-size={W},{H}", "about:blank"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(100):
        try:
            tabs = json.load(urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json")); break
        except Exception:
            time.sleep(0.2)
    ws = websocket.create_connection([t for t in tabs if t["type"] == "page"][0]["webSocketDebuggerUrl"], timeout=120)
    n = [0]

    def call(method, **params):
        n[0] += 1
        ws.send(json.dumps({"id": n[0], "method": method, "params": params}))
        while True:
            m = json.loads(ws.recv())
            if m.get("id") == n[0]:
                return m.get("result", m.get("error"))

    def js(expr):
        return call("Runtime.evaluate", expression=expr, awaitPromise=True, returnByValue=True).get("result", {}).get("value")

    def wait(expr, secs=40):
        end = time.time() + secs
        while time.time() < end:
            if js(f"!!({expr})"):
                return True
            time.sleep(0.3)
        print("timeout:", expr[:80]); return False

    def shot(name, settle=4):
        time.sleep(settle)
        d = call("Page.captureScreenshot", format="png")["data"]
        open(os.path.join(OUT, name), "wb").write(base64.b64decode(d))
        print("wrote", name)

    call("Runtime.enable"); call("Page.enable")
    call("Emulation.setDeviceMetricsOverride", width=W, height=H, deviceScaleFactor=DPR, mobile=True)
    call("Emulation.setTouchEmulationEnabled", enabled=True)
    call("Emulation.setLocaleOverride", locale=LANG)
    call("Network.enable")
    call("Network.setUserAgentOverride", userAgent="Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36",
         acceptLanguage=LANG)
    call("Browser.grantPermissions", origin=URL, permissions=["geolocation"])
    call("Emulation.setGeolocationOverride", latitude=47.3717, longitude=8.5423, accuracy=20)
    # the clock of the page at 11:00 in Zurich, so the map is in its day colours whenever this runs
    call("Page.addScriptToEvaluateOnNewDocument", source="""(() => {
      const shift = Date.UTC(2026, 5, 15, 9, 0, 0) - Date.now(), D = Date;
      class FixedDate extends D { constructor(...a) { a.length ? super(...a) : super(D.now() + shift); } static now() { return D.now() + shift; } }
      globalThis.Date = FixedDate;
    })()""")
    # language and a light map, fixed
    call("Page.navigate", url=URL + "/")
    wait("document.querySelector('.maplibregl-canvas')")
    js(f"localStorage.setItem('hm-lang', '{LANG}')")

    # 1 the map where the visitor is
    call("Page.navigate", url=URL + "/#map=15.5/47.3717/8.5423")
    wait("document.querySelector('.maplibregl-canvas')"); js("document.querySelector('.lm-input').blur()")
    shot("1.png", 8)

    # 2 search results
    js("[...document.querySelectorAll('.lm-bubble')][0].click()")
    wait(r"/\d+ /.test(document.querySelector('.lm-count')?.textContent || '') && document.querySelectorAll('.lm-card img').length > 2", 60)
    shot("2.png", 8)

    # 3 a place with its picture
    js("document.querySelector('.lm-clear').hidden = false; document.querySelector('.lm-clear').click()")
    js("(() => { const i = document.querySelector('.lm-input'); i.value = 'Grossmünster'; document.querySelector('.lm-form').requestSubmit() })()")
    wait("document.querySelector('.lm-place-pic img')?.naturalWidth > 0", 60)
    shot("3.png", 8)

    # 4 a route by car with a stop
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Z%C3%BCrich+HB&via=47.05020,8.30930&vn=Luzern&to=46.94900,7.43910&tn=Bern&mode=car")
    wait("document.querySelector('.lm-route-sum')?.textContent.includes('min')", 60)
    shot("4.png", 6)

    # 5 by bike: the height profile
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Z%C3%BCrich+HB&to=47.34940,8.49190&tn=Uetliberg&mode=bike")
    wait("document.querySelector('.lm-route-sum')?.textContent.includes('min')", 60)
    wait("document.querySelector('.lm-elev')", 30)
    js("document.querySelector('.lm-elev').scrollIntoView({block: 'center'})")
    shot("5.png", 6)

    # 6 navigation (preview)
    call("Page.navigate", url=URL + "/?from=47.37790,8.54030&fn=Z%C3%BCrich+HB&to=47.36690,8.54500&tn=Bellevue&mode=car")
    wait("document.querySelector('.lm-nav-go .lm-chip')", 60)
    js("document.querySelector('.lm-nav-go .lm-chip').click()")
    wait("document.getElementById('app').classList.contains('lm-navigating')", 10)
    shot("6.png", 10)
finally:
    chrome.terminate()
    shutil.rmtree(prof, ignore_errors=True)
