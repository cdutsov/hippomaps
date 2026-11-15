# Changelog

What changed in hippomaps, newest first. The site at https://hippomaps.ch runs the latest commit of `main`; the
version numbers are the releases of the Android app (`android/`), which shows the same page.

## Unreleased

### Added

- A popup above a place of the map when it is tapped or clicked, and on a computer while the mouse is on it: its name,
  its kind and, once its page is there, whether it is open and its address.
- More on the page of a place that OpenStreetMap does not have, looked up by the server: a picture (a photo on
  Wikimedia Commons taken there that names the place, the picture its website shows when shared, a picture of
  Panoramax looking at it, or the logo of the website), the menu of a restaurant, café or bar, and the opening hours of
  the website when the place has none. The card of a place found by its name gets the picture, Website and Menu. The
  privacy and licences pages say so.
- The results of a search as a file, under the Overpass query: CSV (a row per result with its point and tags) or
  GeoJSON (with the query and the credit to OpenStreetMap). Not in the Android app yet.
- Other routes in directions: up to two more from start to destination, paler on the map with how much longer each
  takes in a pill ("+12 min"), and in a row under Start. A tap on one, in the row or on the map, shows it with its steps, and
  Start navigates it. Their times count live traffic and closures, as that of the route does. None with stops on the
  way.
- Feedback in the menu: a problem, a mistake of the map (with a link to leave a note on OpenStreetMap there), an idea or
  anything else, with an e-mail address for an answer if you like. "Report this problem" under a search or a route that
  failed opens it. Unless turned off, the message carries the page as it was (the search, place or route and the map
  view), the browser and the last errors of the page; "What is sent" lists them.

### Changed

- The menu: no line under 3D buildings, Relief shading and Directions and voice; the switch of the traffic reads
  "Traffic (Switzerland only)"; shorter lines under Transit and Language.
- The page starts faster. A first visit downloads MapLibre once, not twice (300 kB less), and what the map needs
  before its first tile at the same time as the scripts: the worker that reads the tiles, the style and its TileJSON.
  The tiles of the first view and the letters of its names are asked for as the script starts, not after the style has
  loaded. A later visit asks the server for the page alone before the map is there. The map comes up in its own
  colours at once, and the page no longer has MapLibre check the style and each of its layers at every start.

### Fixed

- The map opens where you were the last time the page knew your location (kept on the device to about 100 m), not at the
  town of your IP address, which a phone network, a SIM from another country or a VPN can put far away. The location is
  asked for as the page starts, and a location out of view is jumped to rather than flown to.
- A business or other place tapped on the map in directions: its page has Use as destination, Add as stop and Use as
  start, as a point of the map has, and goes back to Directions; the route stays on the map meanwhile.

## 1.4.1 (2026-11-14)

### Added

- The compass in phone browsers too: Chrome and the other browsers on Android, Safari on the iPhone and iPad (it asks
  from a tap on the location button). True north from the World Magnetic Model 2025.

### Fixed

- Android: the compass keeps its heading when the phone is tilted sideways; in 1.4.0 it moved by about as much as the
  tilt.

## 1.4.0 (2026-11-14)

### Added

- Android: a compass. A cone at the dot of the location shows where the phone points; with the map on the location,
  the location button turns the map with the phone, and tapped again puts north up.
- Counts of visits that last over restarts (`STATS_DIR`): per day the page loads, the visitors and the sites they came
  from, with no cookie and no address kept; `hippomaps stats` prints them. The privacy page says so.
- Public transport in directions (Transit), across Europe: the connections by train, tram, metro, bus,
  boat and cable car, leaving now or with a departure or an arrival at a chosen time; each with its lines, changes,
  tracks and walks, delays where an operator publishes them, and drawn on the map in the colours of its lines.
- The start panel says what the search reads, with two searches of the map view in the sentence, and more of them under
  "In this area" (along the river, its bridges, the lake, a sight, the town).
- Directions and voice in a language of their own, apart from the page: 18 languages, each with a Piper voice.
- Arrows over the line at the turns ahead in navigation; the start, the stops and the destination can be dragged on
  the map.
- Signs beside a route by car: stop signs, speed bumps, and speed cameras in the countries that allow a navigation app
  to show them.
- A warning above Start for ferries and car trains on a route by car, with the name of each.
- Without a connection: a note over the map, the page as last loaded, and the map along the route saved on the device
  at Start, so navigation goes on along the route. A switch in the menu turns the saving off.
- The map opens at the town of the IP address of the visitor until the device tells where they are.
- Places of more kinds on the map with a name (dentists, pharmacies, banks, hotels, bakeries and others), under the
  street names.
- Coordinates written in the search box or a field of directions open that point.
- The list of results follows the map: once the map is zoomed or moved it shows the results in view.
- Near or far from a kind of place: the places the search measured from are drawn, with the band of the radius along a
  road, a river or the edge of a forest.
- The search box grows to two lines for a long search.
- A licences page (`/licences`, `/de/licences`) for the voices, traffic, map data, pictures and libraries.
- Donations in Swiss francs from the menu, through Stripe Checkout (not in the Android app).

### Changed

- Fewer places on the map at zoom 14 to 16: supermarkets and hotels from 14, restaurants from 15, shops from 16, with
  a distance between the icons. Street names have room again.
- Benches are drawn from zoom 18 only.
- A search around the view for a chain (Migros) puts the map on the nearest result.
- Directions start from "Your location" before it is known: Start can be tapped ahead and navigation starts with the
  first location.
- Search bubbles search the words they show, in the language of the page; lines by their number in the page language.
- Country borders twice as wide.
- The limits per visitor count an IPv6 visitor by the /64 of their address.
- While connections or a route are searched for, a turning ring before the message, and rows that pulse where the
  connections will be.
- Later, Earlier, another time and another language find connections sooner: the stops and walks around the start
  and the destination are kept for five minutes.

### Fixed

- A name tapped on the map stays the title of the panel, with one marker on the place.
- The Outdoor style keeps its places with a name.
- Start stays in view on a small phone when a route has a ferry warning.
- A search that takes longer than 40 s is no longer cut off.
- A route on foot no longer fails when the elevation API is down.
- The traffic service is asked again after a visitor left during the request.
- Long routes: the phrases of Valhalla and the search along a route no longer take seconds on thousands of steps or
  points.
- A change of language keeps the right word for bubbles and kinds along a route.
- "Forget recent searches" is available again after the next search.
- End tapped while navigation was being set up stops the location watch and the timers.

## 1.3.0 (2026-11-10)

### Added

- A globe with the night side when zoomed out.
- The day map in the colours of Swiss topographic maps; starred places as stars, the place shown as a pin.
- Navigation: a long press shows the point and the places around it, each can be a stop; a place can be searched
  during navigation as a stop or the new destination.
- Labels of the search in German, French and Italian.
- Android: built for Android 16, a sharper icon, and a themed icon on Android 13 and newer.

### Changed

- The language changes at once, without a new page, so navigation goes on.
- Turns are said ahead with their distance ("In 150 meters, …"), then at the turn; arrival is said once and navigation
  ends.
- Navigation is lighter on the device: the map at 30 frames a second, at most 2 pixels per point.
- Zoomed out in navigation, the camera turns from tilted to straight down.
- The banner, the lanes, the next turn and the junction sign stand in one column.
- Map moves: a short glide to a place near the view, a flight to one far away.
- The panel of a phone: empty, it folds and stays down; the suggestions scroll; it moves in one direction when the
  keyboard opens or closes.

### Fixed

- Directions pick a village before the area of the same name around it, and say "closed at this time" for a road closed
  by date (a pass in winter).

## 1.2.0 (2026-11-09)

The first release, and the first on F-Droid.

### Added

- Search in plain words beside the map, with suggestions, quick searches, "search this area", and pictures from
  Wikimedia Commons.
- Place pages with opening hours, website, phone, address, wheelchair access and all tags; a tap on the map shows the
  address and the named places around it.
- Directions from Valhalla by car, bike and on foot, with up to 8 stops, places along the route, lanes, junction signs,
  speed limits, and a height profile by bike and on foot.
- Navigation turn by turn: Start with the location or Preview, a 3D traveller, spoken instructions from a Piper server
  or the browser, a new route when off the route.
- Live traffic in Switzerland on the map and in the route time.
- Transit lines in their colours, starred places, recent searches, shareable links.
- Map styles, 3D terrain and buildings, relief shading, day and night by the sun.
- The page in English, German, French and Italian.
- The Android app: the site in a WebView with native location, navigation in the background with spoken turns, and
  links of other map apps (`geo:`, Google Maps, OpenStreetMap, Apple Maps).
- A site of its own (`BASE_PATH=/`), indexable with `PUBLIC_URL`; an Overpass API of its own with `OVERPASS_URL`.
- Limits per visitor for searches, routes, pictures and spoken sentences.
- Licence GPL-3.0-or-later.
