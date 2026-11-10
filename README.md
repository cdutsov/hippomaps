# hippomaps

A map app for the web and Android: search in plain words, place pages, directions and turn-by-turn navigation, on
OpenStreetMap data. Live at https://hippomaps.ch.

## Features

- **Search** in plain words ("cafés in Zurich old town", "peaks over 4000 m in Valais") with suggestions while typing,
  results on the map and in a list, "search this area", and quick searches for restaurants, hotels, fuel and more
- **Place pages**: photo or logo from Wikimedia Commons with its credit, opening hours with the next change, website, phone,
  address, wheelchair access, Wikipedia, all tags
- **What is here**: a tap on the map shows the address and the named places around it
- **Directions** by car, bike and on foot, with up to 8 stops, places along the route (fuel, charging, rest areas, food,
  drinking water…), lanes, junction signs and speed limits, and a height profile by bike and on foot
- **Navigation** turn by turn, spoken in four languages, with re-routing, a 3D traveller and a preview mode
- **Live traffic** in Switzerland: closures, queues and roadworks on the map and in the route time
- **Transit lines** (metro, light rail, tram) in their colours
- **Map**: opens near the visitor (the town of their IP address) until the device tells where they are; a globe with the night side when zoomed out; nine styles, the default day map in the colours of Swiss
  topographic maps, day and night by the sun, 3D terrain and buildings, relief shading; starred places as stars
- **Languages**: English, German, French, Italian
- **Privacy**: settings, starred places and recent searches stay in the browser
- **Shareable links** for places, searches and routes
- **Android app** (`android/`): the site in a WebView with native location, background navigation, and links from other
  map apps (`geo:`, Google Maps, OpenStreetMap, Apple Maps)

## How it works

A Go server serves the page (`web/`, plain JavaScript and MapLibre, no build step) and relays what the browser needs
to other services:

| Service | Used for | Setting |
|---|---|---|
| Map tiles API ([overpass.ch](https://overpass.ch) maps, or a gateway with the same paths) | vector and terrain tiles, styles, geocoding | `TILES_URL`, `MAPS_KEY` |
| [overpass-search](https://gitlab.com/cdutsov/overpass-search) | the search | `SEARCH_UPSTREAM` |
| [Valhalla](https://github.com/valhalla/valhalla) | directions, lanes, speed limits | `ROUTING_URL` |
| Overpass API | transit lines, places along a route | `OVERPASS_URL`, or overpass.ch with `ELEVATION_KEY` |
| overpass.ch elevation API | height profiles | `ELEVATION_URL`, `ELEVATION_KEY` |
| [Piper](https://github.com/rhasspy/piper) behind `/say?lang=&text=` | spoken navigation (else the voices of the browser) | `TTS_URL` |
| A traffic service with `/incidents.json` | live traffic | `TRAFFIC_URL` |
| Wikidata and Wikimedia Commons | photos and logos | `PICTURES` |
| [DB-IP City Lite](https://db-ip.com/db/download/ip-to-city-lite) (MMDB file, monthly, CC BY 4.0) | the map opens at the town of the IP address of the visitor | `GEOIP_DB` |

Except for the tiles, each service is optional: without its setting the feature is left out of the page.

## Run

```
go run ./cmd/hippomaps                        # :8096, the page on /labs/maps
docker build -t hippomaps . && docker run -p 8096:8096 -e BASE_PATH=/ hippomaps
```

The Docker build runs `go vet` and the tests.

## Settings

| Setting | Default | |
|---|---|---|
| `LISTEN` | `:8096` | address |
| `BASE_PATH` | `/labs/maps` | path of the page; `/` for a site of its own |
| `PUBLIC_URL` | | public address of the page: makes it indexable (canonical link, `robots.txt`, `sitemap.xml`) |
| `TILES_URL` | `https://api.overpass.ch` | map tiles API |
| `MAPS_KEY` | | key of the tiles API, sent to the browser |
| `SEARCH_UPSTREAM` | | overpass-search, relayed under `{base}/api/search` |
| `SEARCH_API` | `/labs/search/api` | search API as the browser reaches it, when not relayed |
| `ROUTING_URL` | | Valhalla |
| `OVERPASS_URL` | | Overpass API, asked without a key |
| `ELEVATION_URL` | `https://api.overpass.ch` | elevation API |
| `ELEVATION_KEY` | | overpass.ch key for elevation (and Overpass without `OVERPASS_URL`), server side only |
| `TTS_URL` | | Piper server |
| `TRAFFIC_URL` | | traffic service |
| `GEOIP_DB` | | a MaxMind DB file with a location per network (DB-IP City Lite; GeoLite2 City has the same fields): the map opens at the place of the IP address of the visitor, else in Zurich. A new file renamed over it is opened within a minute. `hippomaps geoip FILE ADDRESS…` looks addresses up in a file |
| `PICTURES` | `true` | photos and logos from Wikimedia |
| `PICTURE_CACHE_MB` | `64` | memory for Wikimedia answers and thumbnails |
| `PICTURES_PER_IP`, `ROUTES_PER_IP`, `SAYS_PER_IP`, `SEARCHES_PER_IP` | `600`, `120`, `600`, `100` | per visitor in 10 minutes (map taps: three times the searches) |
| `USER_AGENT` | `hippomaps/<version> (+https://gitlab.com/cdutsov/hippomaps)` | for Wikimedia and the other services |

Limits count by `X-Forwarded-For`, so the server belongs behind a proxy that sets it.

## Server API

Under `{base}/api/`; errors are `{"error":{"code","message"}}`.

| | |
|---|---|
| `GET picture?wikidata=Q…&brand=Q…&commons=File:…&w=120` | thumbnail of the place photo, else the chain logo, with credit headers |
| `GET route?from=lat,lon&via=…&to=…&mode=car\|bike\|walk&lang=` | line, steps, stops, speed limits, lanes, traffic, tunnels, height profile |
| `POST along` `{"line","cat","mode","lang"}` | places of a kind near a route, in route order |
| `GET transit?tile=10/x/y` | transit lines of a tile as GeoJSON |
| `GET say?lang=&text=` | spoken sentence (audio/mpeg) |
| `GET traffic` | traffic incidents |
| `GET search/{search,query,complete,here}` | relayed overpass-search; with `lang=de`, `fr` or `it` its English words (the reading, other readings, notes, kinds of places) in that language (`server/labels.go`) |

`GET /privacy` and `/de/privacy` serve the privacy policy (`web/privacy*.html`), linked from the menu.

`GET /healthz` answers `ok`; `hippomaps healthcheck` asks it (the Docker health check).

## Android

`android/` holds the app (Kotlin, one activity, AndroidX only, no Google Play services). `android/build.sh` builds the
signed APK and AAB in Docker; the build is reproducible, and F-Droid publishes it with the same signature. Google Play:
[android/PLAY.md](android/PLAY.md). Store texts and
screenshots are in `fastlane/`. Details: [android/README.md](android/README.md).

## Tests

- `go test ./...`: the server against fakes of Valhalla, Wikimedia, Overpass, the elevation API, Piper, the traffic service
  and overpass-search
- `tests/site-test.sh` (`PAGE=https://…`): a running site end to end; `BROWSER=1` adds `tests/browser.py`, the page in
  headless Chromium on a phone screen, from the first search to navigation

## Layout

```
cmd/hippomaps/   settings, server, healthcheck
server/          page, pictures, directions, search relay, transit, traffic, voice
web/             index.html, app.js, i18n.js, avatar.js (3D traveller), app.css, icons, vendor/
android/         the Android app
fastlane/        F-Droid texts and screenshots
tests/           tests of a running site
```

## Licence

GPL-3.0-or-later (`LICENSE`). `web/vendor/` holds maplibre-gl 6.13.0 (BSD-3-Clause), three.js 0.170.0 (MIT),
opening_hours 3.15.0 (LGPL-3.0-only) and suncalc 2.0.1 (BSD-2-Clause), each with its licence. `server/labels/presets.json` holds
the names of the iD presets (id-tagging-schema 7.2.0, ISC), built by `server/labels/gen.py`, licence in `server/labels/LICENSE-id-tagging-schema.md`.

Map data © OpenStreetMap contributors (ODbL). Pictures from Wikimedia Commons under the licence each credit names. Traffic:
Federal Roads Office (FEDRO) Traffic Data Platform (TDP). Places of IP addresses: IP Geolocation by DB-IP (CC BY 4.0, not in the
repository; the map credits it with a link when it opens there).
