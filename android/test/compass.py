#!/usr/bin/env python3
"""The compass of the app in the emulator of test/Dockerfile: poses of the phone given to the sensors of the emulator,
the heading the page gets from window.hmCompass (degrees from true north toward which the top of the screen points),
with the screen upright and turned; the rotation sensor on only while the page watches and the app is in front.

    docker cp android/test/compass.py hm-emu:/tmp/ && docker exec hm-emu python3 /tmp/compass.py

Needs the debug build installed and its page open (cdp.py), with the location allowed. Prints "ok <what>" and
"FAIL <what>"; exits 1 on a failure. Leaves the screen upright with rotation by the sensors off."""
import json
import subprocess
import sys
import time

fails = []
G = 9.81
# the field of the earth near Zurich in the axes of the world (east, north, up), in microtesla
FIELD = (0.0, 22.0, -42.0)


def check(cond, what):
    print(("ok " if cond else "FAIL ") + what, flush=True)
    if not cond:
        fails.append(what)


def adb(*args):
    return subprocess.run(["adb", *args], capture_output=True, text=True).stdout


def js(expr):
    out = subprocess.run(["cdp.py", expr], capture_output=True, text=True).stdout.strip()
    return json.loads(out) if out else None


def pose(x, y, z):
    """The sensors for a phone whose axes (right, top, out of the screen) point along x, y, z of the world."""
    dot = lambda a, b: sum(p * q for p, q in zip(a, b))
    up = (0.0, 0.0, 1.0)
    adb("emu", "sensor", "set", "acceleration", ":".join(f"{G * dot(a, up):.3f}" for a in (x, y, z)))
    adb("emu", "sensor", "set", "magnetic-field", ":".join(f"{dot(a, FIELD):.3f}" for a in (x, y, z)))


def heading():
    """The last heading the page gets within 2.5 s of a new watch (the sensors 3 s to settle)."""
    time.sleep(3)
    r = js("new Promise((ok) => { const got = []; const stop = hmCompass.watch((h) => got.push(h));"
           " setTimeout(() => { stop(); ok(JSON.stringify(got[got.length - 1] || null)); }, 2500); })")
    h = json.loads(r) if r else None
    return h and h["heading"]


def apart(a, b):
    return abs((a - b + 540) % 360 - 180)


def screen(rotation):
    adb("shell", "settings", "put", "system", "accelerometer_rotation", "0")
    adb("shell", "settings", "put", "system", "user_rotation", str(rotation))
    time.sleep(2.5)


def active():
    """The app has a listener on a sensor now (an open event connection of ch.hippomaps.app.Compass)."""
    d = adb("shell", "dumpsys", "sensorservice")
    return "ch.hippomaps.app.Compass" in d.split("Previous Registrations:")[0]


E, N, U, W, S, D = (1, 0, 0), (0, 1, 0), (0, 0, 1), (-1, 0, 0), (0, -1, 0), (0, 0, -1)
check(js("typeof window.hmCompass?.watch") == "function", "the page has window.hmCompass")
adb("emu", "geo", "fix", "8.5403", "47.3779")
screen(0)

pose(E, U, S)  # held up, the back to north
north = heading()
# the declination near Zurich: about 4 degrees east (2026)
check(north is not None and 2 <= north <= 6, f"held up, the back to north: north and the declination ({north})")
north = north or 0
pose(S, U, W)  # the back to east
h = heading()
check(h is not None and apart(h, 90 + north) < 1.5, f"held up, the back to east ({h})")
pose(N, W, U)  # flat on a table, the top to west
h = heading()
check(h is not None and apart(h, 270 + north) < 1.5, f"flat, the top to west ({h})")
pose(E, S, D)  # held over the face lying down: the screen down, the top to south
h = heading()
check(h is not None and apart(h, 180 + north) < 1.5, f"over the face, the screen down, the top to south ({h})")
pose(N, W, U)
screen(1)  # turned counterclockwise: the right side of the phone is the top of the screen, to north
h = heading()
check(h is not None and apart(h, north) < 1.5, f"landscape (90), flat, the right side to north ({h})")
screen(3)  # turned clockwise: the left side, to south
h = heading()
check(h is not None and apart(h, 180 + north) < 1.5, f"landscape (270), flat, the left side to south ({h})")
screen(0)

# the sensor: on while the page watches, off in the background, on again in front; the page may watch it itself
# (it does while it shows the location)
own = active()
js("window.__hmTestStop = hmCompass.watch(() => {}); 1")
time.sleep(1)
check(active(), "a watch of the page: the rotation sensor on")
adb("shell", "input", "keyevent", "KEYCODE_HOME")
time.sleep(2)
check(not active(), "the app in the background: the sensor off")
adb("shell", "am", "start", "-n", "ch.hippomaps.app.debug/ch.hippomaps.app.MainActivity")
time.sleep(3)
check(active(), "the app in front again: the sensor on")
js("window.__hmTestStop(); 1")
time.sleep(1)
check(active() == own, f"the watch stopped: the sensor as before it ({'on: the page watches it too' if own else 'off'})")
adb("shell", "settings", "put", "system", "accelerometer_rotation", "1")
sys.exit(1 if fails else 0)
