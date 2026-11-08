// Package web holds the files of the map app: the page, its script and styles, and
// the libraries it loads (vendor/).
package web

import "embed"

//go:embed index.html app.js app.css icon.svg vendor
var Files embed.FS
