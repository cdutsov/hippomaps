#!/usr/bin/env python3
"""Evaluates a JavaScript expression in the hippomaps page of a debug build (WebView debugging on).
   cdp.py '<expression>'      prints the JSON value
Needs adb; forwards tcp:9222 to the WebView DevTools socket of ch.hippomaps.app.debug."""
import base64, json, os, socket, struct, subprocess, sys, urllib.request

def forward():
    pid = subprocess.run(["adb", "shell", "pidof", "ch.hippomaps.app.debug"], capture_output=True, text=True).stdout.strip()
    if not pid:
        sys.exit("app not running")
    subprocess.run(["adb", "forward", "tcp:9222", f"localabstract:webview_devtools_remote_{pid}"], check=True, capture_output=True)

def ws_connect(url):
    rest = url.split("://", 1)[1]
    hostport, path = rest.split("/", 1)
    host, port = hostport.split(":")
    s = socket.create_connection((host, int(port)), timeout=20)
    key = base64.b64encode(os.urandom(16)).decode()
    s.sendall((f"GET /{path} HTTP/1.1\r\nHost: {hostport}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
               f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
    resp = b""
    while b"\r\n\r\n" not in resp:
        resp += s.recv(1)
    if b" 101 " not in resp.split(b"\r\n")[0]:
        sys.exit(resp.decode(errors="replace"))
    return s

def send(s, text):
    data = text.encode(); mask = os.urandom(4)
    hdr = bytes([0x81]) + (bytes([0x80 | len(data)]) if len(data) < 126 else bytes([0x80 | 126]) + struct.pack(">H", len(data)))
    s.sendall(hdr + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(data)))

def recv(s):
    def exact(n):
        b = b""
        while len(b) < n:
            c = s.recv(n - len(b))
            if not c:
                raise EOFError
            b += c
        return b
    msg = b""
    while True:
        h = exact(2); fin = h[0] & 0x80; n = h[1] & 0x7F
        if n == 126: n = struct.unpack(">H", exact(2))[0]
        elif n == 127: n = struct.unpack(">Q", exact(8))[0]
        msg += exact(n)
        if fin:
            return msg.decode()

forward()
pages = json.load(urllib.request.urlopen("http://127.0.0.1:9222/json"))
page = next(p for p in pages if p.get("type") == "page" and "hippomaps.ch" in p.get("url", ""))
s = ws_connect(page["webSocketDebuggerUrl"])
send(s, json.dumps({"id": 1, "method": "Runtime.evaluate", "params": {"expression": sys.argv[1], "returnByValue": True, "awaitPromise": True}}))
while True:
    m = json.loads(recv(s))
    if m.get("id") == 1:
        r = m.get("result", {})
        print(json.dumps(r.get("result", {}).get("value", r), ensure_ascii=False))
        break
