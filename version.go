// Package imgnest exposes build metadata of the ImgNest core module.
package imgnest

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version is the release this build was made from, as written in VERSION by
// scripts/bump-version.py.
func Version() string { return strings.TrimSpace(version) }
