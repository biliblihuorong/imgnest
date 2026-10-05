// Package webvben embeds the independently built Vben frontend. The CLI imports
// this package only when built with -tags vben; the default binary uses web.
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
