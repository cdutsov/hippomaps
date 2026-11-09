#!/bin/sh
# The feature graphic of Google Play (1024 x 500) per language, from feature-graphic.html and the
# route screenshot (phoneScreenshots/4.png) of that language. Needs chromium.
set -eu
cd "$(dirname "$0")"
for pair in "en-US|Maps, search and navigation" "de-DE|Karten, Suche und Navigation"; do
	loc=${pair%%|*}; tag=${pair#*|}
	sed "s#LOCALE#$loc#; s#TAGLINE#$tag#" feature-graphic.html >".fg-$loc.html"
	chromium --headless=new --no-sandbox --disable-gpu --hide-scrollbars --window-size=1024,500 \
		--screenshot="$PWD/metadata/android/$loc/images/featureGraphic.png" "file://$PWD/.fg-$loc.html" 2>/dev/null
	rm ".fg-$loc.html"
	echo "metadata/android/$loc/images/featureGraphic.png"
done
