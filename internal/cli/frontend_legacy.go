//go:build !vben

package cli

import (
	"io/fs"

	"github.com/biliblihuorong/imgnest/web"
)

func frontendDistFS() (fs.FS, error) {
	return web.DistFS()
}
