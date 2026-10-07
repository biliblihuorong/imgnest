package cli

import (
	"io/fs"

	webvben "github.com/biliblihuorong/imgnest/web-vben"
)

func frontendDistFS() (fs.FS, error) {
	return webvben.DistFS()
}
