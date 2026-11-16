// Package web holds the files of the map app: the page, its script and styles, and
// the libraries it loads (vendor/), the privacy policy and the licences in English and German, and the pages about the app (pages/).
package web

import "embed"

//go:embed assetlinks.json index.html privacy.html privacy-de.html licences.html licences-de.html privacy.css app.js early.js avatar.js i18n.js wmm.js sw.js sw-off.js app.css icon-32.png icon-192.png apple-touch-icon.png overpass-logo.png overpass-logo-dark.png vendor pages
var Files embed.FS
