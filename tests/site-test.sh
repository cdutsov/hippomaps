#!/usr/bin/env bash
# hippomaps on its site (default https://hippomaps.ch): the service, the page (for search engines), its files, the key of the
# map tiles, the pictures from Wikimedia Commons, directions in Switzerland by car with speed limits, lanes and traffic,
# heights on foot, the voice, traffic. About 22 requests to the site, one or two to Wikimedia (then kept for a day), two
# routes. BROWSER=1 adds the page in headless Chromium (tests/browser.py, two minutes; needs chromium and websocket-client).
#   tests/site-test.sh                       PAGE=https://… tests/site-test.sh
# Ends with "hippomaps test: N passed, M failed"; exit 1 on a failure.
set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
PAGE=${PAGE:-https://hippomaps.ch}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
pass=0 fail=0
ok() { echo "  PASS  $*"; pass=$((pass + 1)); }
ko() { echo "  FAIL  $*"; fail=$((fail + 1)); }
hdr() { grep -i "^$1:" "$TMP/h" | head -n1 | cut -d' ' -f2- | tr -d '\r'; } # a header of the last answer
get() { curl -s -m 30 -o "$TMP/b" -D "$TMP/h" -w '%{http_code}' "$@"; }

echo "hippomaps on $PAGE"

echo "Service"
code=$(get "$PAGE/healthz")
[[ $code == 200 ]] && ok "the service answers" || ko "the service: HTTP $code"
if docker inspect hippomaps >/dev/null 2>&1; then
	health=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}no health check{{end}}' hippomaps)
	[[ $health == healthy ]] && ok "the container is healthy" || ko "the container: $health"
	ports=$(docker port hippomaps)
	[[ -z $ports ]] && ok "the container publishes no port" || ko "the container publishes $ports"
fi

echo "Page"
code=$(get "$PAGE")
[[ $code == 200 ]] && ok "the page answers" || ko "the page: HTTP $code"
cp "$TMP/b" "$TMP/page"
# its own site is for search engines: no noindex, a canonical link, robots.txt with the sitemap
! grep -q noindex "$TMP/page" && [[ -z $(hdr X-Robots-Tag) ]] && ok "the page is for search engines" || ko "noindex: $(hdr X-Robots-Tag)"
grep -qF '<link rel="canonical" href="'"$PAGE"'/">' "$TMP/page" && ok "a canonical link" || ko "no canonical link to $PAGE/"
[[ $(hdr Permissions-Policy) == *'geolocation=(self)'* ]] && ok "the page may ask for the location" || ko "Permissions-Policy: $(hdr Permissions-Policy)"
csp=$(hdr Content-Security-Policy)
[[ $csp == *"script-src 'self'"* && $csp != *unsafe* ]] && ok "a Content-Security-Policy without unsafe sources" || ko "Content-Security-Policy: $csp"
code=$(get "$PAGE/robots.txt")
[[ $code == 200 ]] && grep -q "^Sitemap: $PAGE/sitemap.xml" "$TMP/b" && grep -q "^Disallow: /api/" "$TMP/b" && ok "robots.txt with the sitemap" || ko "robots.txt: HTTP $code"
code=$(get "$PAGE/sitemap.xml")
[[ $code == 200 ]] && grep -qF "<loc>$PAGE/</loc>" "$TMP/b" && ok "sitemap.xml" || ko "sitemap.xml: HTTP $code"
# the app for Android opens links of the site: its signing key in assetlinks.json
code=$(get "$PAGE/.well-known/assetlinks.json")
[[ $code == 200 ]] && jq -e '.[0].target.package_name == "ch.hippomaps.app" and (.[0].target.sha256_cert_fingerprints | length) > 0' "$TMP/b" >/dev/null && ok "assetlinks.json for the app" || ko "assetlinks.json: HTTP $code"

echo "Files"
app=$(grep -o '/static/app.js?v=[0-9a-f]*' "$TMP/page" | head -n1)
code=$(get "$PAGE${app#$(sed -E 's#https?://[^/]*##' <<<"$PAGE")}")
[[ $code == 200 && $(hdr Cache-Control) == *immutable* ]] && ok "the script, kept for a year" || ko "$app: HTTP $code, $(hdr Cache-Control)"
code=$(get "$PAGE/static/vendor/maplibre-gl-6.13.0/maplibre-gl.mjs")
[[ $code == 200 ]] && ok "MapLibre" || ko "MapLibre: HTTP $code"

echo "Map tiles"
# the key of the page, as the browser sends it: with the site as Referer (it is not printed)
tiles=$(grep -o 'data-tiles="[^"]*"' "$TMP/page" | cut -d'"' -f2)
key=$(grep -o 'data-key="[^"]*"' "$TMP/page" | cut -d'"' -f2)
if [[ -n $key ]]; then
	code=$(get -H "Referer: $PAGE/" "$tiles/v1/$key/maps/styles/light.json")
	[[ $code == 200 ]] && ok "the key of the page gets the style" || ko "the style: HTTP $code"
else
	ko "the page does not have a key for the map tiles (MAPS_KEY)"
fi

echo "Pictures"
code=$(get "$PAGE/api/picture?wikidata=42")
[[ $code == 400 ]] && ok "a request without a picture: 400" || ko "a bad request: HTTP $code"
code=$(get "$PAGE/api/picture?brand=Q680727") # the logo of Migros
case $code in
200) [[ $(hdr Content-Type) == image/png && $(hdr X-Picture-Kind) == logo ]] && ok "a logo from Commons" || ko "the logo: $(hdr Content-Type) $(hdr X-Picture-Kind)" ;;
502 | 503) echo "  SKIP  Wikimedia did not answer (HTTP $code)" ;;
*) ko "the logo: HTTP $code" ;;
esac

echo "Directions"
if grep -q "data-route=" "$TMP/page"; then
	code=$(get "$PAGE/api/route?from=47.3779,8.5403&to=91,7&mode=car")
	[[ $code == 400 ]] && ok "a bad point: 400" || ko "a bad point: HTTP $code"
	code=$(get "$PAGE/api/route?from=47.3779,8.5403&to=46.9490,7.4391&mode=car") # Zurich - Bern
	km=$(jq -r ".km // empty" "$TMP/b" 2>/dev/null)
	if [[ $code == 200 ]] && awk -v k="$km" "BEGIN { exit !(k > 100 && k < 160) }"; then
		ok "Zurich - Bern by car: $km km, $(jq ".steps | length" "$TMP/b") steps"
	else
		ko "Zurich - Bern: HTTP $code $(head -c 200 "$TMP/b")"
	fi
	# by car the route carries the speed limits and the lanes of its junctions (navigation shows them)
	n=$(jq "(.limits // []) | length" "$TMP/b" 2>/dev/null)
	[[ ${n:-0} -ge 5 ]] && ok "Zurich - Bern: $n stretches with a speed limit" || ko "Zurich - Bern: ${n:-no} speed limits"
	n=$(jq "[.steps[] | select(.lanes)] | length" "$TMP/b" 2>/dev/null)
	[[ ${n:-0} -ge 1 ]] && ok "Zurich - Bern: $n turns with lanes" || ko "Zurich - Bern: no lanes at the turns"
	jq -e "(.delay // 0) >= 0 and ((.traffic // []) | all(length == 3))" "$TMP/b" >/dev/null 2>&1 && ok "the traffic of the route: $(jq "(.traffic // []) | length" "$TMP/b") slowed stretches, $(jq ".delay // 0" "$TMP/b") s" || ko "the traffic of the route: $(jq -c "{traffic, delay}" "$TMP/b")"
	if grep -q "data-route=" "$TMP/page"; then
		code=$(get "$PAGE/api/route?from=46.5936,7.9086&to=46.6058,7.9218&mode=walk") # Lauterbrunnen - Wengen, 500 m up
		up=$(jq -r ".elev.up // empty" "$TMP/b" 2>/dev/null)
		if [[ $code == 200 && -n $up ]] && awk -v u="$up" "BEGIN { exit !(u > 300 && u < 700) }"; then
			ok "Lauterbrunnen - Wengen on foot: $up m up, $(jq ".elev.pts | length" "$TMP/b") heights"
		else
			ko "the heights on foot: HTTP $code, up ${up:-none}"
		fi
	fi
else
	echo "  SKIP  no directions (ROUTING_URL not set)"
fi

echo "Voice"
if grep -q "data-say=" "$TMP/page"; then
	code=$(get "$PAGE/api/say?lang=de&text=Rechts%20abbiegen.")
	size=$(stat -c %s "$TMP/b")
	[[ $code == 200 && $(hdr Content-Type) == audio/mpeg && $size -gt 2000 ]] && ok "a spoken sentence: $size bytes of audio" || ko "the voice: HTTP $code, $(hdr Content-Type), $size bytes"
	code=$(get "$PAGE/api/say?lang=xx&text=Hallo")
	[[ $code == 400 ]] && ok "a language without a voice: 400" || ko "a language without a voice: HTTP $code"
else
	echo "  SKIP  no voice (TTS_URL not set)"
fi

echo "Traffic"
if grep -q "data-traffic=" "$TMP/page"; then
	code=$(get "$PAGE/api/traffic")
	if [[ $code == 200 ]] && jq -e '.items and .flow and (.source | test("FEDRO"))' "$TMP/b" >/dev/null 2>&1; then
		ok "traffic in Switzerland: $(jq ".items | length" "$TMP/b") events, $(jq "[.items[] | select(.closed)] | length" "$TMP/b") closed now"
	else
		ko "traffic: HTTP $code $(head -c 200 "$TMP/b")"
	fi
else
	echo "  SKIP  no traffic (TRAFFIC_URL not set)"
fi
for f in avatar.js i18n.js vendor/three-0.170.0/three.module.min.js; do
	code=$(get "$PAGE/static/$f")
	[[ $code == 200 ]] && ok "$f" || ko "$f: HTTP $code"
done

# the page in a browser (headless Chromium): BROWSER=1, as the nightly run does; about two minutes
if [[ ${BROWSER:-0} == 1 ]]; then
	echo "Browser"
	# once more when it fails: a slow tile or answer of a server once is no failure of the page
	if out=$(timeout 300 python3 -W ignore "$HERE/browser.py" "$PAGE" 2>&1) || out=$(timeout 300 python3 -W ignore "$HERE/browser.py" "$PAGE" 2>&1); then
		while read -r line; do ok "$line"; done < <(grep "^ok " <<<"$out" | cut -c4-)
	else
		while read -r line; do ok "$line"; done < <(grep "^ok " <<<"$out" | cut -c4-)
		while read -r line; do ko "$line"; done < <(grep "^FAIL " <<<"$out" | cut -c6-)
		grep -q "^FAIL " <<<"$out" || ko "the browser test stopped: $(tail -n 2 <<<"$out" | tr '\n' ' ')"
	fi
fi

echo "hippomaps test: $pass passed, $fail failed"
((fail == 0))
