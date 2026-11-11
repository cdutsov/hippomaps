#!/usr/bin/env bash
# hippomaps on its site (default https://hippomaps.ch): the service, the page (for search engines), its files, the key of the
# map tiles, the pictures from Wikimedia Commons, directions in Switzerland by car with speed limits, lanes and traffic,
# heights on foot, the voice, traffic. About 25 requests to the site, one or two to Wikimedia (then kept for a day), two
# routes; the spoken turns of navigation, the 30 fps cap and the texts of each language in the source (tests/*.test.mjs, node in Docker). BROWSER=1 adds the page in headless Chromium (tests/browser.py, about five minutes; needs chromium and websocket-client).
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
# the map opens near the visitor: the place of their IP address in the page, which no cache shares; the database
# (GEOIP_DB) opens and is renewed monthly. The server is asked directly too, as by a visitor at 81.62.0.1 (Swisscom): a
# machine can reach its own server from a private address (docker-proxy)
[[ $(hdr Cache-Control) == *private* ]] && ok "the page is kept by no shared cache" || ko "Cache-Control of the page: $(hdr Cache-Control)"
if gdb=$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' hippomaps 2>/dev/null | sed -n 's/^GEOIP_DB=//p') && [[ -n $gdb ]]; then
	near=$(docker exec hippomaps wget -qO- --header 'X-Forwarded-For: 81.62.0.1' "http://127.0.0.1:8096$(sed -E 's#https?://[^/]*##' <<<"$PAGE")/" 2>&1 |
		grep -o 'data-near="[^"]*"' | head -n1 | cut -d'"' -f2)
	[[ $near =~ ^-?[0-9]{1,2}\.[0-9]{3},-?[0-9]{1,3}\.[0-9]{3},(5|11)$ ]] && ok "the page has the place of the IP address of a visitor ($near)" || ko "the place of the IP address of a visitor: '$near'"
	found=$(docker exec hippomaps hippomaps geoip "$gdb" 8.8.8.8 2>&1)
	[[ $found =~ ^8\.8\.8\.8\ -?[0-9.]+,-?[0-9.]+\ (5|11)$ ]] && ok "the database of IP addresses opens ($found)" || ko "the database of IP addresses: $found"
	# visitors over IPv4 and IPv6 reach the server with their own address: Caddy on a network with IPv6 (overpass1,
	# 2026-11-10); before, docker-proxy handed every IPv6 visitor over from the Docker gateway, one address for all
	for f in 4 6; do
		code=$(curl -s -"$f" -m 15 -o "$TMP/b" -w '%{http_code}' "$PAGE/")
		[[ $code == 000 ]] && continue # no way over IPv$f from here, or no address of the site
		p=$(grep -o 'data-near="[^"]*"' "$TMP/b" | head -n1 | cut -d'"' -f2)
		[[ -n $p ]] && ok "a visitor over IPv$f reaches the server with their address ($p)" || ko "over IPv$f the server sees no public address of the visitor (HTTP $code)"
	done
	age=$(( ($(date +%s) - $(docker exec hippomaps stat -c %Y "$gdb" 2>/dev/null || echo 0)) / 86400 ))
	((age <= 40)) && ok "the database of IP addresses is $age days old" || ko "the database of IP addresses is $age days old (monthly update)"
fi
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
# the privacy policy (Google Play asks for its address; the menu of the app links it): English and German, in the sitemap;
# it credits DB-IP with a link (CC BY 4.0; the map does not)
for pr in "privacy|Privacy policy" "de/privacy|Datenschutzerklärung"; do
	code=$(get "$PAGE/${pr%%|*}")
	[[ $code == 200 ]] && grep -qF "${pr#*|}" "$TMP/b" && grep -q 'info@overpass.ch' "$TMP/b" && grep -qF '<a href="https://db-ip.com">IP Geolocation by DB-IP</a>' "$TMP/b" && [[ -z $(hdr X-Robots-Tag) ]] && ok "the privacy policy /${pr%%|*}, with the credit of DB-IP" || ko "/${pr%%|*}: HTTP $code"
done
code=$(get "$PAGE/sitemap.xml")
grep -qF "<loc>$PAGE/de/privacy</loc>" "$TMP/b" && ok "the privacy policy in the sitemap" || ko "the sitemap does not list the privacy policy"
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
	# fuel along the route (Overpass of overpass.ch), when the page offers the search along
	if grep -q "data-along=" "$TMP/page"; then
		jq -c '{line: .line, cat: "fuel", mode: "car"}' "$TMP/b" >"$TMP/line"
		code=$(get -X POST -H 'Content-Type: application/json' --data-binary "@$TMP/line" "$PAGE/api/along")
		n=$(jq "(.features // []) | length" "$TMP/b" 2>/dev/null)
		[[ $code == 200 && ${n:-0} -ge 10 ]] && jq -e '[.features[].properties.at] | . == sort' "$TMP/b" >/dev/null &&
			ok "Zurich - Bern: $n fuel stations along, in the order of the route" || ko "fuel along Zurich - Bern: HTTP $code, ${n:-no} places"
	fi
	# by Lucerne: two legs joined, the stop at the joint
	code=$(get "$PAGE/api/route?from=47.3779,8.5403&via=47.0502,8.3093&to=46.9490,7.4391&mode=car")
	if [[ $code == 200 ]] && jq -e '(.stops | length) == 1 and ([.steps[] | select(.stop == 1)] | length) == 1 and .stops[0] > 0 and .stops[0] < (.line | length)' "$TMP/b" >/dev/null; then
		ok "Zurich - Lucerne - Bern: $(jq -r .km "$TMP/b") km, the stop at point $(jq '.stops[0]' "$TMP/b") of $(jq '.line | length' "$TMP/b")"
	else
		ko "a route with a stop: HTTP $code $(head -c 200 "$TMP/b")"
	fi
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

echo "Donations"
# no checkout is opened here: only what the server refuses before it asks Stripe
if grep -q 'data-donate=' "$TMP/page"; then
	code=$(get -X POST --data 'amount=1&lang=de' "$PAGE/api/donate")
	[[ $code == 400 ]] && ok "a donation under 2 francs: 400" || ko "a donation under 2 francs: HTTP $code"
	code=$(get -X POST --data 'amount=10.50' "$PAGE/api/donate")
	[[ $code == 400 ]] && ok "a donation in rappen: 400" || ko "a donation in rappen: HTTP $code"
	code=$(get "$PAGE/api/donate?amount=10")
	[[ $code == 405 ]] && ok "a donation by GET: 405" || ko "a donation by GET: HTTP $code"
else
	echo "  SKIP  no donations (STRIPE_KEY not set)"
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

# the tests of the source next to this test (tests/*.test.mjs: spoken turns, 30 fps cap, texts): node in a
# container, when there is Docker; a few seconds, no request to the site
if [[ -f $HERE/nav-announce.test.mjs && -f $HERE/../web/app.js ]] && command -v docker >/dev/null; then
	echo "Source"
	if out=$(docker run --rm -v "$(cd "$HERE/.." && pwd)":/w:ro -w /w node:24.21.0-alpine3.23 node --test 'tests/*.test.mjs' 2>&1); then
		ok "the spoken turns of navigation, the 30 fps cap, the texts in each language ($(grep -o '^ℹ pass [0-9]*' <<<"$out" | cut -d' ' -f3) node tests)"
	else
		ko "the node tests of the source: $(grep -m1 -A2 '^✖' <<<"$out" | tr '\n' ' ')"
	fi
	# the bubbles over the map search the words they show: each, in each language, is a feature
	# searched in the view. /query reads the sentence without Overpass; on the server the search
	# container itself, so that the 32 requests do not count against the searches of this address
	search="$PAGE/api/search"
	ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' hippomaps-search 2>/dev/null)
	[[ -n ${ip%% *} ]] && search="http://${ip%% *}:8095/labs"
	labels=$(docker run --rm -v "$(cd "$HERE/.." && pwd)":/w:ro -w /w node:24.21.0-alpine3.23 node --input-type=module -e '
import { texts } from "./web/i18n.js"; import { readFileSync } from "node:fs";
const en = new Function(`return ${readFileSync("web/app.js", "utf8").match(/\n  bubbles: (\[\[[\s\S]*?\]\]),\n/)[1]}`)();
const all = texts(String);
for (const [l, b] of [["en", en], ...["de", "fr", "it"].map((l) => [l, all[l].bubbles])]) for (const [q, label] of b) if (q !== "transit") console.log(`${l}\t${label}`);' 2>&1)
	bad=() n=0
	while IFS=$'\t' read -r l label; do
		n=$((n + 1))
		get -G "$search/query" --data-urlencode "q=$label" --data-urlencode "lang=$l" --data-urlencode "bbox=8.50,47.36,8.56,47.39" >/dev/null
		jq -e '(.query // "") | contains("(47.36000,8.50000,47.39000,8.56000)") and (contains("brand") | not)' "$TMP/b" >/dev/null 2>&1 || bad+=("$l $label")
	done <<<"$labels"
	((n >= 32 && ${#bad[@]} == 0)) && ok "the bubbles search what they show, in the view ($n labels in 4 languages)" || ko "bubbles not searched in the view ($n labels): ${bad[*]:-none read}"
	# near a kind of place: Overpass keeps all of them near (around.class1, the set of that kind of place); the answer
	# draws each place near a result (a point) with the circle of the radius around it; "far from highways" reads
	# highways as roads
	get -G "$search/search" --data-urlencode "q=Migros near a bus stop in Zurich" --data-urlencode "lang=en" >/dev/null
	jq -e '(.query | contains("(around.class1:300)")) and .total > 0
		and ([.lines.features[] | select(.properties.kind == "target" and .geometry.type == "Point")] | length > 0)
		and ([.lines.features[] | select(.properties.kind == "circle")] | length > 0)' "$TMP/b" >/dev/null 2>&1 &&
		ok "Migros near a bus stop: the stops near them, each with its circle" || ko "Migros near a bus stop: $(head -c 300 "$TMP/b")"
	# a sentence of several parts, in any order: along the river in the town and near a lake, the river and the lake drawn
	get -G "$search/search" --data-urlencode "q=restaurants in zurich near the lake along the limmat" --data-urlencode "lang=en" >/dev/null
	jq -e '.total > 0 and (.understood | test("within 300 m of the lake, along Limmat"))
		and ([.lines.features[].properties.kind] | index("river") != null and index("shape") != null)' "$TMP/b" >/dev/null 2>&1 &&
		ok "restaurants along the Limmat near the lake: $(jq -r .total "$TMP/b") found" || ko "along the Limmat near the lake: $(head -c 300 "$TMP/b")"
	get -G "$search/query" --data-urlencode "q=cafés far from highways" --data-urlencode "lang=en" --data-urlencode "bbox=8.50,47.36,8.56,47.39" >/dev/null
	jq -e '(.understood | test("more than 300 m from highways")) and (.query | contains("motorway|trunk|primary"))' "$TMP/b" >/dev/null 2>&1 &&
		ok "cafés far from highways: highways are roads" || ko "cafés far from highways: $(jq -c .understood "$TMP/b" 2>/dev/null)"
fi

# the page in a browser (headless Chromium): BROWSER=1, as the nightly run does; about five minutes, at most eight
if [[ ${BROWSER:-0} == 1 ]]; then
	echo "Browser"
	# once more when it fails: a slow tile or answer of a server once is no failure of the page
	if out=$(timeout 480 python3 -W ignore "$HERE/browser.py" "$PAGE" 2>&1) || out=$(timeout 480 python3 -W ignore "$HERE/browser.py" "$PAGE" 2>&1); then
		while read -r line; do ok "$line"; done < <(grep "^ok " <<<"$out" | cut -c4-)
	else
		while read -r line; do ok "$line"; done < <(grep "^ok " <<<"$out" | cut -c4-)
		while read -r line; do ko "$line"; done < <(grep "^FAIL " <<<"$out" | cut -c6-)
		grep -q "^FAIL " <<<"$out" || ko "the browser test stopped: $(tail -n 2 <<<"$out" | tr '\n' ' ')"
	fi
fi

echo "hippomaps test: $pass passed, $fail failed"
((fail == 0))
