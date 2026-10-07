// Package webvben embeds the built Vue single-page app so the server binary
// can serve it without external files. The embed requires web-vben/dist to
// exist at compile time; dist/.gitkeep keeps a clean checkout buildable.
package webvben

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the built Vben SPA rooted at its own dist/ directory.
func DistFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
