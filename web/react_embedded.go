//go:build arcvpn_embed_react

package web

import (
	"embed"
	"io/fs"
)

// reactFiles contains the built Vite application, never handwritten placeholders.
// The release builder copies frontend/dist to web/react-dist before compilation.
//
//go:embed react-dist
var reactFiles embed.FS

func ReactAssets() (fs.FS, error) {
	return fs.Sub(reactFiles, "react-dist")
}
