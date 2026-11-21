# hippomaps for Android

An Android app that shows https://hippomaps.ch in a WebView. The web app itself is unchanged; the app
adds what a browser tab lacks:

- launcher icon, splash screen until the page has loaded, edge to edge with the system bars in the
  colours of the page (light or dark, following `data-theme` of the page)
- links to hippomaps.ch (shared places, routes, searches) open in the app; other links, phone
  numbers, mail and geo: links open in the apps for them
- `navigator.share` opens the Android share sheet; `navigator.wakeLock` keeps the screen on
- location from the Android location service (GPS and network providers, no Google Play
  services), asked for with the Android permission dialog
- the compass: the heading of the phone for the page (below)
- navigation goes on with the screen locked or another app in front (below)
- the back button closes what the page has open (menu, place card, directions), then goes back
  in the page history, then leaves the app; while navigating it puts the app in the background
- a native screen when the page cannot load, which loads the page again once the phone is online

## Install

The APK of each version is on the releases page of GitLab:
https://gitlab.com/cdutsov/hippomaps/-/releases (`hippomaps-<version>.apk`, signed with our key). The app is
not yet on F-Droid: its inclusion policy asks for three months of development, so the request follows in
February 2027 (the recipe and the steps are ready, below). Google Play: in review.

## Compass

While the page shows the location in view and does not navigate, it watches the heading of the
phone (`window.hmCompass.watch(fn)`, from `bridge.js`) and the app runs `Compass.kt`:

- the rotation vector sensor (gyroscope, accelerometer and magnetometer fused by Android; the
  geomagnetic one without a gyroscope) at 50 Hz, smoothed over about 0.1 s, at most 25 headings a
  second to the page and only for a change of half a degree
- the heading is level and square to the left-right of the screen, also with the screen turned
  to landscape: lying flat, the way the top points; held up, the way the back faces. A phone
  tipped forward, tilted sideways or turned in its own plane points the same way. Degrees from
  true north: the declination of the magnetic field where the phone is (`GeomagneticField`, from
  the positions of the location) is added
- with each heading how far off it may be (the estimate of the sensor, else its accuracy), which
  the page shows as a wider cone
- the sensor is off while the app is in the background, when the page stops watching and when a
  page loads

The page draws a cone from the dot of the location. With the map on the visitor the location
button turns the map with the phone; tapped again, north up. A touch of the map, a search,
directions or navigation end the turning. In a phone browser the page reads the orientation of
the browser itself, with the same heading (`headingOf` in `web/app.js`).

## Navigation in the background

While the page navigates (`#app.lm-navigating`), the app runs a foreground service of type
location (`NavigationService.kt`):

- an ongoing notification with the next instruction and distance from the page banner; a tap
  opens the app, Stop ends the navigation in the page (it clicks `.lm-nav-x`)
- the WebView is not paused and its renderer keeps its priority, so the page keeps its timers
  and speaks the turns
- positions come from the app (`LocationFeed.kt`): the bridge replaces `navigator.geolocation`
  for the whole page and the app pushes each fix through the message channel; GPS fixes win over
  network fixes for 10 s
- a partial wake lock (at most 6 h) keeps the processor awake

Ending the navigation in the page, the Stop action, closing the app from the recent apps or a
reload stops the service, the notification, the location updates and the wake lock. The app asks
for the notification permission once, at the first navigation (Android 13 and newer). It does
not ask for background location: the service starts while the app is in front.

The page and the app talk through a script injected at document start, for https://hippomaps.ch
only (`app/src/main/assets/bridge.js`, `WebViewCompat.addWebMessageListener`). The user agent ends
in `hippomaps-android/<version>`.

Package `ch.hippomaps.app`, version 1.4.1 (code 6), minSdk 26 (Android 8), targetSdk 36. Kotlin, one activity
(`MainActivity.kt`) and one service, AndroidX core, activity, webkit and
splashscreen.

## Build

```
android/build.sh
```

Builds in Docker (`Dockerfile`: Temurin 17, Android command line tools, platform 35, build tools
35.0.0, Gradle 8.10.2), runs lint, and writes the signed `dist/hippomaps.apk` and
`dist/hippomaps.aab` (for the Play Store). The Gradle cache is in the Docker volume
`hippomaps-android-gradle`. `CPUS` (default 4) limits the build; `COPY_TO=<file>` copies the APK
there as well. Versions of plugins and libraries are in `gradle/libs.versions.toml`.

Without Docker, `./gradlew lint assembleDebug` works with a local JDK 17 and Android SDK, and
Android Studio opens the folder as a Gradle project; a release build needs `HIPPOMAPS_SIGNING`
(below). The debug build (`ch.hippomaps.app.debug`, installs beside the release app) has
WebView debugging on, for `chrome://inspect` or `test/cdp.py`.

## Release key

`SIGNING_DIR` (default `/root/.config/hippomaps-android/`, mode 700, outside the repository): `release.p12` and
`signing.properties` (mode 600, holds the passwords). `build.sh` creates them on its first run and
passes the properties file to Gradle in `HIPPOMAPS_SIGNING`. Losing the key means the app cannot
be updated under the same name: keep a copy in the backups.

## Install

```
adb install -r android/dist/hippomaps.apk
```

## F-Droid

Not published there yet (Install above). Once the recipe is in fdroiddata, F-Droid builds the app from
the tag `v<versionName>` (recipe `metadata/ch.hippomaps.app.yml`) and publishes it with our signature: the build is reproducible, so F-Droid copies the
signature of our APK onto its own build and publishes only when the two are the same. Users can
switch between the F-Droid APK and ours, and `assetlinks.json` needs no second fingerprint. The
texts, icon, screenshots and changelogs are in `fastlane/metadata/android/` at the top of the
repository.

A release:

1. raise `versionCode` and `versionName` in `app/build.gradle.kts`; add
   `fastlane/metadata/android/{en-US,de-DE}/changelogs/<versionCode>.txt` (500 characters at most)
2. commit, tag `v<versionName>`, push the commit and the tag
3. `android/build.sh` in a checkout of the tag
4. a GitLab release for the tag with `dist/hippomaps.apk` attached as `hippomaps-<versionName>.apk`
   (the `Binaries` URL of the recipe: `/-/releases/v<versionName>/downloads/hippomaps-<versionName>.apk`)

F-Droid finds the new tag by itself (`UpdateCheckMode: Tags`) and builds it within a few days. A
change of the JDK, Gradle or the Android Gradle plugin can break reproducibility: check with
`fdroid build` in the image `registry.gitlab.com/fdroid/fdroidserver:buildserver-trixie` and
`apksigcopier compare` before the tag.

## Google Play

`PLAY.md`: signing with the same key (`play-key.sh`), the account, the listing and the answers to the
forms of Play Console. The icons come from `icon/hippo.svg` (`icon/render.sh`): launcher and splash at
320 px, a themed (monochrome) layer for Android 13 and newer, and the 512 px store icon.

## Opening hippomaps.ch links in the app

For Android to open https://hippomaps.ch links in the app without asking, the site must serve
`assetlinks.json` (written by `build.sh` from the certificate of the release key) at
`https://hippomaps.ch/.well-known/assetlinks.json` with `Content-Type: application/json`. Until
then the links open in the browser, or in the app after the user enables the link in the app
settings ("Open by default"). With Play App Signing, add the SHA-256 of the Play signing key to
the file as well.

## Map links of other apps

The app is offered (not verified, the user picks it) for `geo:` coordinates, `google.navigation:`,
Google Maps links (`maps.google.*`, `www.google.*/maps`, short links `maps.app.goo.gl` and
`goo.gl/maps`), OpenStreetMap (`openstreetmap.org`, `osm.org`), Apple Maps (`maps.apple.com`), and
hippomaps.ch over http and www. `MapLinks.kt` turns each into a place (`?at=`), a route (`?to=`), a
search (`?q=`) or a view (`#map=`) of hippomaps.ch; the app follows the redirects of a short link
itself. A link with no place says so in a toast. Unit tests: `app/src/test` (run by `build.sh`).

## Testing

`test/Dockerfile` and `test/avd.sh` run a headless emulator (API 34, Google APIs, x86_64) with
KVM:

```
docker build -t hippomaps-android-emulator:1 android/test
docker run -d --name hm-emu --device /dev/kvm hippomaps-android-emulator:1
# Android 16: the build arguments at the top of test/Dockerfile, image hippomaps-android-emulator:36
docker exec hm-emu adb wait-for-device
docker cp android/dist/hippomaps.apk hm-emu:/tmp/ && docker exec hm-emu adb install -r /tmp/hippomaps.apk
docker exec hm-emu adb emu geo fix 8.5403 47.3779
docker exec hm-emu adb exec-out screencap -p > shot.png
docker rm -f hm-emu
```

`test/cdp.py '<expression>'` (in the emulator container) evaluates JavaScript in the page of the
debug build over the DevTools protocol, e.g. the banner text while the screen is off.

`test/compass.py` checks the compass in the emulator with the debug build open: poses of the phone
given to the sensors of the emulator and the headings the page gets, upright, flat, over the face
and in landscape; the sensor on and off with the watch of the page and the app in the background:

```
docker cp android/test/compass.py hm-emu:/tmp/ && docker exec hm-emu python3 /tmp/compass.py
```

## Not done yet

- Background navigation is tested in the emulator only (WebView 113, screen off for about a
  minute). On real phones, check long trips with the screen off and aggressive battery savers
  (Samsung, Xiaomi), which can still stop a foreground service.
- `navigator.permissions.query({name: "geolocation"})` still answers from the WebView, not from
  the app permission.
- No offline maps; the app needs a connection like the website.
- Lint lists newer AGP and AndroidX versions than the pinned ones (AGP 9 needs a migration).
