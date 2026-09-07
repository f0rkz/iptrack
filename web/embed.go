package web

import "embed"

//go:embed index.html app.css identity.css app.js
var Files embed.FS
