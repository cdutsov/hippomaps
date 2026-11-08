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
- navigation goes on with the screen locked or another app in front (below)
- the back button closes what the page has open (menu, place card, directions), then goes back
  in the page history, then leaves the app; while navigating it puts the app in the background
- a native screen when the page cannot load, which loads the page again once the phone is online

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

Package `ch.hippomaps.app`, version 1.1.0 (code 2), minSdk 26 (Android 8), targetSdk 35. Kotlin, one activity
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

## Opening hippomaps.ch links in the app

For Android to open https://hippomaps.ch links in the app without asking, the site must serve
`assetlinks.json` (written by `build.sh` from the certificate of the release key) at
`https://hippomaps.ch/.well-known/assetlinks.json` with `Content-Type: application/json`. Until
then the links open in the browser, or in the app after the user enables the link in the app
settings ("Open by default"). With Play App Signing, add the SHA-256 of the Play signing key to
the file as well.

## Testing

`test/Dockerfile` and `test/avd.sh` run a headless emulator (API 34, Google APIs, x86_64) with
KVM:

```
docker build -t hippomaps-android-emulator:1 android/test
docker run -d --name hm-emu --device /dev/kvm hippomaps-android-emulator:1
docker exec hm-emu adb wait-for-device
docker cp android/dist/hippomaps.apk hm-emu:/tmp/ && docker exec hm-emu adb install -r /tmp/hippomaps.apk
docker exec hm-emu adb emu geo fix 8.5403 47.3779
docker exec hm-emu adb exec-out screencap -p > shot.png
docker rm -f hm-emu
```

`test/cdp.py '<expression>'` (in the emulator container) evaluates JavaScript in the page of the
debug build over the DevTools protocol, e.g. the banner text while the screen is off.

## Not done yet

- Background navigation is tested in the emulator only (WebView 113, screen off for about a
  minute). On real phones, check long trips with the screen off and aggressive battery savers
  (Samsung, Xiaomi), which can still stop a foreground service.
- `navigator.permissions.query({name: "geolocation"})` still answers from the WebView, not from
  the app permission.
- The launcher and splash icons come from the 192 px web icon; a larger or vector source would
  make them sharper on high-density screens.
- No themed (monochrome) launcher icon for Android 13 and newer.
- No offline maps; the app needs a connection like the website.
- Google Play raises the required targetSdk every year (API 36 for updates from late 2026): check
  before the first upload. Lint lists newer AGP and AndroidX versions than the pinned ones.
- Play Store listing (texts, screenshots, privacy notes) and Play App Signing.
