// Package web holds the files of the map app: the page, its script and styles, and
// the libraries it loads (vendor/).
package web

import "embed"

//go:embed index.html app.js app.css icon-32.png icon-192.png apple-touch-icon.png overpass-logo.png overpass-logo-dark.png vendor
var Files embed.FS
