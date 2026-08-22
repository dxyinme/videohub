package web

import "embed"

// FS contains the frontend static assets.
//
//go:embed index.html app.js style.css
var FS embed.FS
