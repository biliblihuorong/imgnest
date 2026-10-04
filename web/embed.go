// Package web embeds the built Vue single-page app so the server binary can
// serve it without external files. The embed requires web/dist to exist at
// compile time; web/dist/.gitkeep keeps a clean checkout buildable, and the
// release build replaces it with real assets via `make release`/`make fe-build`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the built SPA rooted at dist/. The only failure mode is a
// malformed embed layout, which the go:embed directive itself normally makes
// a compile-time error.
func DistFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
