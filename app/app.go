// Package app is the public entry point for building an ImgNest binary,
// optionally with extension plugins. cmd/imgnest calls it with none.
package app

import (
	"context"
	"io"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/cli"
)

// Execute runs an ImgNest command with the supplied plugins mounted on the
// serve command's router.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, plugins ...extension.Plugin) error {
	return cli.Execute(ctx, args, stdout, stderr, plugins...)
}
