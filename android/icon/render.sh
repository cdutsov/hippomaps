#!/bin/sh
# Draws the icons of the app and of the store listings from hippo.svg, in Docker:
#   fastlane/metadata/android/en-US/images/icon.png             512 px, the full blue square (Google Play rounds it)
#   android/app/src/main/res/drawable-nodpi/hm_icon.png          320 px, rounded: launcher and splash (80 dp at xxxhdpi)
#   android/app/src/main/res/drawable-nodpi/hm_icon_mono.png     320 px, the hippo alone: themed icon of Android 13+
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
docker run --rm -v "$repo:/r" python:3.13.1-slim-bookworm sh -c '
apt-get update -qq >/dev/null && apt-get install -y -qq libcairo2 >/dev/null 2>&1
pip install -q --root-user-action=ignore cairosvg==2.7.1 2>/dev/null
python - <<PY
import re, cairosvg
svg = open("/r/android/icon/hippo.svg").read()
full = re.sub(r"<rect id=\"bg\"[^>]*/>", "<rect x=\"0\" y=\"0\" width=\"1536\" height=\"1536\" fill=\"#0284c7\"/>", svg)
mono = re.sub(r"<rect id=\"bg\"[^>]*/>", "", svg)
res = "/r/android/app/src/main/res/drawable-nodpi/"
for s, out, n in ((full, "/r/fastlane/metadata/android/en-US/images/icon.png", 512),
                  (svg, res + "hm_icon.png", 320), (mono, res + "hm_icon_mono.png", 320)):
    cairosvg.svg2png(bytestring=s.encode(), write_to=out, output_width=n, output_height=n)
    print(out, n)
PY'
