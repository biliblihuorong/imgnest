// Package cli wires ImgNest's commands and application dependencies.
package cli

import (
	"context"
	"io"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/spf13/cobra"
)

// Execute runs an ImgNest command with the supplied context and output
// streams; plugins are mounted only by the serve command.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, plugins ...extension.Plugin) error {
	return executeWithInput(ctx, args, nil, stdout, stderr, plugins...)
}

func executeWithInput(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, plugins ...extension.Plugin) error {
	root := &cobra.Command{
		Use:           "imgnest",
		Short:         "ImgNest image hosting server",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	var configPath string
	root.PersistentFlags().StringVar(&configPath, "config", "", "deployment YAML file")
	root.AddCommand(migrateCommand(&configPath), adminCommand(&configPath, false), adminCommand(&configPath, true), serveCommand(&configPath, plugins), localCommand(&configPath), storageCommand(&configPath), policyCommand(&configPath))
	if stdin != nil {
		root.SetIn(stdin)
	}
	return root.ExecuteContext(ctx)
}
