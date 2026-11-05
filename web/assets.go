package web

import "embed"

// Assets is the local no-CDN compatibility UI. The target React/Vite frontend
// is in frontend/ and will replace the bundle after npm dependencies are built.
//
//go:embed index.html style.css app.js node-install.sh
var Assets embed.FS
