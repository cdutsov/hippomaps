# Google Play

What Google Play needs for HippoMaps (`ch.hippomaps.app`), and the answers to its forms. The texts,
icon, feature graphic and screenshots are in `fastlane/metadata/android/` (shared with F-Droid); the
plain-text descriptions for Play are in `android/play/`.

## Build

`android/build.sh` writes `android/dist/hippomaps.aab` (the bundle Play takes) beside the APK, signed
with the release key. Play has required targetSdk 36 (Android 16) for new apps and updates since
31 August 2026; Google raises it every August, so check
<https://developer.android.com/google/play/requirements/target-sdk> before each upload.

## Signing: the same key everywhere

Play App Signing is compulsory: Google signs the APKs it delivers. Give Google our release key
(the one of `android/build.sh`, `/root/.config/hippomaps-android/release.p12`) instead of letting it make
a new one. The app from Play, from F-Droid and from the GitLab release then carry the same certificate
(SHA-256 `51:63:C5:AF:…:67:DE`): users can switch between them without uninstalling, and
`assetlinks.json` keeps one fingerprint. The same key is then also the upload key.

In Play Console, at the first release: App integrity → App signing → "Use a different key" →
"Export and upload a key from Java keystore". Download `pepk.jar` and the encryption public key
(`encryption_public_key.pem`) into one directory on the server, then:

```
android/play-key.sh /path/to/that/directory
```

It asks twice for the password (the `storePassword` line of
`/root/.config/hippomaps-android/signing.properties`) and writes `encrypted_private_key.zip` there;
upload that file. The choice of key cannot be undone once the app is published.

## Account

- A Play developer account costs USD 25 once. Personal accounts created after 13 November 2023 must
  run a closed test with at least 12 testers opted in for 14 days in a row before they can publish to
  production. Organisation accounts skip this but need a D-U-N-S number (free, from Dun & Bradstreet,
  for a business in the commercial register; it takes a few days to a few weeks).
- The developer name, address and e-mail are shown on the store page.

## Store listing

| Field | Value |
|---|---|
| App name | HippoMaps (`fastlane/metadata/android/*/title.txt`) |
| Short description | `fastlane/metadata/android/*/short_description.txt` (80 characters at most) |
| Full description | `android/play/description-{en-US,de-DE}.txt` (plain text; the fastlane files are HTML for F-Droid) |
| App icon | `fastlane/metadata/android/en-US/images/icon.png`, 512 × 512, full square: Play rounds it |
| Feature graphic | `fastlane/metadata/android/{en-US,de-DE}/images/featureGraphic.png`, 1024 × 500 |
| Phone screenshots | `fastlane/metadata/android/{en-US,de-DE}/images/phoneScreenshots/1–6.png`, 1080 × 1920 |
| Category | Maps & Navigation |
| Contact | info@overpass.ch, website https://hippomaps.ch |
| Privacy policy | https://hippomaps.ch/privacy (German: /de/privacy) |

Default language English (United States), translation German (Germany). The screenshots come from
`fastlane/screenshots.py <dir> <en|de>` (Chromium against hippomaps.ch, the clock fixed at 11:00 in
Zurich for the day colours); the feature graphic from `fastlane/feature-graphic.sh`; the icons from
`android/icon/render.sh` (source `android/icon/hippo.svg`).

## App content (Policy → App content)

Privacy policy: https://hippomaps.ch/privacy

Ads: no ads.

App access: all functionality is available without special access (no login).

Content rating (IARC questionnaire): category "All other app types"; no violence, sexuality,
language, controlled substances or gambling; users cannot interact or exchange content; the app does
not share the location of a user with other users; no purchases. Expected: PEGI 3 / Everyone.

Target audience: 13 and over (13–15, 16–17, 18 and over). Below 13 the Families policy applies.

News app: no. Health: no. Financial features: none. Government app: no.

Data safety:

| Question | Answer |
|---|---|
| Does the app collect or share any of the required user data types? | Yes |
| Is all collected data encrypted in transit? | Yes |
| Do you provide a way for users to request that their data be deleted? | No account: nothing is stored on our side. Answer "No" (or, if asked, explain that nothing is kept) |
| Location → Approximate location | Collected, not shared; processed ephemerally; optional; purpose: App functionality |
| Location → Precise location | Collected, not shared; processed ephemerally; optional; purpose: App functionality |
| App activity → In-app search history | Collected, not shared; processed ephemerally; optional; purpose: App functionality |
| Everything else (personal info, financial, health, messages, photos, audio, files, calendar, contacts, web browsing, app info and performance, device or other IDs) | Not collected |

"Collected" because the position of a route start and the words of a search leave the device for our
server; "ephemerally" because the server only answers and does not store them (privacy policy,
section 3). Saved places and recent searches stay on the device and do not count.

Foreground service permissions: the app declares `FOREGROUND_SERVICE_LOCATION` for
`NavigationService`. Type: Location; use case: Navigation ("turn-by-turn navigation keeps receiving the
location while the screen is off or another app is open, with an ongoing notification and a Stop
button; it ends with the navigation"). Play wants a video link: upload
`foreground-service-navigation.mp4` (in `/root/work/hippomaps-release/play/`, recorded in the
Android 16 emulator: Start, the home screen, the notification, back to the app) to YouTube as
unlisted, and paste the link. The app does not ask for `ACCESS_BACKGROUND_LOCATION`, so there is no
background location declaration.

## A release

1. As for F-Droid (android/README.md): raise `versionCode` and `versionName`, changelogs
   `fastlane/metadata/android/{en-US,de-DE}/changelogs/<versionCode>.txt`, commit, tag, push.
2. `android/build.sh` in a checkout of the tag.
3. Play Console → the track (first the closed test, later production) → Create new release → upload
   `android/dist/hippomaps.aab`; release notes from the changelogs (500 characters at most).
4. Review usually takes a few days for a new app, hours to a day for an update.
