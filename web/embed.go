// Package web holds the files of the map app: the page, its script and styles, and
// the libraries it loads (vendor/), and the privacy policy in English and German.
package web

import "embed"

//go:embed assetlinks.json index.html privacy.html privacy-de.html privacy.css app.js avatar.js i18n.js app.css icon-32.png icon-192.png apple-touch-icon.png overpass-logo.png overpass-logo-dark.png vendor
var Files embed.FS
