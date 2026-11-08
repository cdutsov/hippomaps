# hippomaps

A map app: a search box with suggestions, the results and the page of a place in a side bar, and the map beside it. Pictures of
places and logos of chains come from Wikimedia Commons. On a phone the side bar is a sheet over the lower part of the map.

It is a client of [overpass-search](https://gitlab.com/cdutsov/overpass-search): the browser calls its API for the reading, the
results and the completions. This server serves the page and its files, reads pictures from Wikidata and Commons, and asks
Valhalla for directions, so that the browser of a visitor asks only the site it is on.

```
go run ./cmd/hippomaps            # :8096, the page on /labs/maps
```

## What it does

- Languages English, German, French and Italian (`web/i18n.js`): the language of the browser, or the one chosen in the menu
  (`hm-lang`; the page loads again). It sets the texts, the names on the map, the steps of directions and the voice. The
  readings of the search come from overpass-search and stay English.
- A 2D/3D button above the compass: on the map the 3D of the menu (terrain, buildings, the map tilted); in navigation the
  map that follows, flat or tilted.
- The map opens where the visitor is: the browser asks for the location once (a link with `#map=` opens there instead; no
  message when the visitor says no).
- Suggestions while typing: words of the list, chains and places (`/complete`).
- The reading of the search at once (`/query`), then the results (`/search`), named first, 40 at a time.
- A card per result: name, kind, chain, address, distance from the visitor once known, open or closed with the next change
  (opening_hours.js, loaded when a result has hours), and a thumbnail: the photo of the place, else the logo of the chain, else
  a letter.
- The page of a place: the picture with its credit, open or closed, directions, website, phone, e-mail, address, opening hours,
  wheelchair access, Wikipedia, and all tags.
- Directions (with `ROUTING_URL`; without it the button opens OpenStreetMap): to a place, or to a tapped point, by car, bike or
  on foot. Start and destination are fields that suggest places and addresses from the geocoder of overpass.ch
  (Photon, `/v1/{key}/geocode/api` with the key of the map, near the map view; the location of the visitor first); the start
  can also be a tap on the map; a button swaps start and destination. The line on
  the map, time and length, and the steps in the language of the browser; a tap on a step moves the map there. The mode is
  kept in the browser (`hm-mode`). By bike and on foot, the height profile (with `ELEVATION_KEY`): up and down, coloured by
  how steep each part is; under the mouse or a finger the height and distance, and the point on the map. In navigation a strip
  of it shows the point reached and the climb still to come.
- Navigation, turn by turn, from the directions: Start follows the location of the browser, Preview drives the route at three
  times its speed (on a desktop, and for trying it). The map alone, tilted and turned with the way, follows a car, a cyclist or a
  walker in 3D (`web/avatar.js`, three.js, loaded with the first route; wheels, pedals and legs move with the speed; the flat
  sprite when WebGL fails; larger on a flat map); the 2D/3D button turns the map flat or tilted, kept as `hm-nav-2d`; the next turn with its arrow, its distance and the street; by car the sign of the junction in its
  last 2 km (green on motorways, blue on other roads: exit number, route numbers, destinations) and the speed limit; arrival
  time, time and distance left. The instructions of Valhalla are spoken in the language of the browser: with `TTS_URL` by
  the Piper voices of the server in German, English, French and Italian (`/api/say`, fetched up to three turns ahead), else, or
  when a sentence is late (2.5 s), by the best voice of the browser (natural, enhanced and Google voices first; not eSpeak or
  the novelty voices of Apple). A button turns the sound off, kept as `hm-mute`. Off the route for three fixes, a new route from where the visitor is (at most every
  10 s). A move of the map stops the following until Re-centre; Whole route shows the rest of it. The screen stays on (Wake
  Lock) while the page is open.
- The map: results as points and shapes, the lines of the answer (a border, a road, a river, the circles around the places of a
  chain), hover and selection in step with the list.
- "Search this area": after the visitor moves the map, the same search in the view (`view=1`; the place of the sentence goes).
- The location of the visitor: a button that asks the browser once, shows a dot with its accuracy and moves the map there; the
  searches with "near me" and "nearest" ask for it too.
- What is here: the first tap on the map closes the search box; a tap then drops a pin and shows the address and the named
  things around the point (`/here` of overpass-search), with pictures; a tapped symbol of the map opens that place. The area
  of the tap (a country far out, a lake, a village or a park closer in) is outlined in the blue of overpass.ch.
- A compass above the location button: its needle turns with the map, and a tap turns the map to north.
- On a phone: the side bar is a sheet at three heights (the title, half, almost all), dragged by its handle and its title; a
  search closes the keyboard and leaves the map in view; only the map zooms, and pull to refresh is off.
- The address holds the search and the place shown: `?q=…&sel=node/1#map=15/47.37/8.54`; Back goes from a place to the list.
- A menu (the burger; closed at first): the style of the map (light, dark, outdoor, relief, driving, marine, white, grayscale,
  black), 3D (terrain and buildings) and relief shading, kept in the browser (`hm-style`, `hm-3d`, `hm-shading`). The page is
  dark with the dark and black styles; the first style follows the theme of the site (`theme`).
- The logo and the icons are the hippo of overpass.ch (`icon-32.png`, `icon-192.png`, `apple-touch-icon.png`).

## Pictures

`GET {base}/api/picture?wikidata=Q…&brand=Q…&commons=File:…&w=120|500` answers the thumbnail of the first that has one:

1. `commons`: the file of `wikimedia_commons`, or of an `image` tag on Commons (the page passes it): a photo
2. `wikidata`: the image (P18) of the item of the place: a photo
3. `brand`: the logo (P154), else the small logo (P8972), of the item of the chain
4. `wikidata`: the logo of the item of the place

The answer is the bytes (PNG, JPEG, GIF or WebP; SVG files come as the PNG that Commons draws, never as SVG), with
`X-Picture-Kind` (`photo` or `logo`), `X-Picture-Credit` (author and licence, URL-encoded) and `X-Picture-Page` (the page of
the file on Commons). 204 when the place does not have one, 400 when the request does not name one, 429 over the limit of a
visitor, 503 when the calls to Wikimedia stay taken, 502 when Wikimedia failed. Four calls to Wikimedia at a time; the answers
of Wikidata and Commons and the thumbnails are kept in memory for a day.

## Directions

`GET {base}/api/route?from=lat,lon&to=lat,lon&mode=car|bike|walk&lang=de` asks Valhalla (`ROUTING_URL` + `/route`) and answers
`{"km","s","line":[[lon,lat],…],"steps":[…],"limits":[[from,to,kmh],…]}`. A step has its text, length and time, where it
starts and ends on the line (`at`, `i`, `end`), the spoken texts (`alert`, `pre`, `post`), the street, and what the sign of the
junction shows (`exit`, `exit_name`, `routes`, `toward`). `limits` are the speed limits by car, in indexes of `line`, from a
second call to Valhalla (`trace_attributes`); missing when that call fails. Errors are
`{"error":{"code","message"}}`: 400 `bad_request` or `too_far` (more than 1500 km in a line by car, 300 by bike, 100 on foot),
404 `no_route`, 429 `too_many`, 503 `busy`, 502 `failed`. Four calls to Valhalla at a time.

`GET {base}/api/say?lang=de|en|fr|it&text=…` (at most 400 characters) asks `TTS_URL` + `/say` and answers audio/mpeg, kept a
week by the browser and in memory here; 400, 429, 503 or 502 otherwise. Four calls at a time.

By bike and on foot the answer has `elev`: `{"up","down","min","max","pts":[[km,m],…]}`, from the elevation API of overpass.ch
(`ELEVATION_URL` + `/v1/elevation/profile`, 300 samples at most, one every 10 m at least) with `ELEVATION_KEY`, a key of type
"All APIs" (a maps key is refused); missing when that call fails.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `LISTEN` | `:8096` | address of the server |
| `BASE_PATH` | `/labs/maps` | the path of the page; its files are under `{base}/static/`, the pictures under `{base}/api/picture` |
| `SEARCH_API` | `/labs/search/api` | the API of overpass-search, as the browser reaches it: `{api}/complete`, `{api}/query`, `{api}/search` |
| `TILES_URL` | `https://api.overpass.ch` | the map tiles; the style is `{tiles}/v1/{key}/maps/styles/light.json` (or `dark`) |
| `MAPS_KEY` | | a key of the tiles API for the page |
| `PICTURES` | `true` | pictures from Wikimedia Commons |
| `PICTURES_PER_IP` | `600` | pictures a visitor gets in 10 minutes |
| `PICTURE_CACHE_MB` | `64` | memory for the answers of Wikimedia and the thumbnails |
| `ROUTING_URL` | | Valhalla, for directions; empty: no directions |
| `ROUTES_PER_IP` | `120` | directions a visitor gets in 10 minutes |
| `ELEVATION_URL` | `https://api.overpass.ch` | the elevation API, for height profiles |
| `ELEVATION_KEY` | | its key, server side only; empty: no height profiles |
| `TTS_URL` | | a Piper server (`/say?lang=&text=`, audio/mpeg) for the spoken instructions; empty: the voices of the browser |
| `SAYS_PER_IP` | `600` | new sentences a visitor gets spoken in 10 minutes (those in memory are free) |
| `USER_AGENT` | `hippomaps/<version> (+https://gitlab.com/cdutsov/hippomaps)` | for Wikimedia, which asks for a way to reach the operator |

The page sends a Content-Security-Policy that allows this site and the tiles API, `noindex`, and `geolocation=(self)`. The
proxy in front sets `X-Forwarded-For`, which the limit of pictures counts by.

`hippomaps healthcheck` asks the running server for `/healthz`.

## Layout

```
cmd/hippomaps/   settings from the environment, listen, healthcheck
server/          the page, its files, the pictures, directions; tests with a fake Wikimedia and a fake Valhalla
web/             index.html, app.js, app.css, icon.svg, and vendor/
```

`web/vendor/` holds maplibre-gl 6.13.0 (BSD-3-Clause, `LICENSE.txt`), three.js 0.170.0 (MIT, `LICENSE`) and opening_hours 3.15.0 (LGPL-3.0-only) with suncalc
2.0.1 (BSD-2-Clause), unchanged from npm except that opening_hours imports `./suncalc.mjs`. The directories carry the version:
browsers keep their files for a year.

Map data © OpenStreetMap contributors (ODbL). Pictures from Wikimedia Commons, each under the licence its credit names.
