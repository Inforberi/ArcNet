//go:build !arcvpn_embed_react

package web

import (
	"errors"
	"io/fs"
)

func ReactAssets() (fs.FS, error) {
	return nil, errors.New("this development binary has no embedded React bundle; build with scripts/build-release.sh")
}
