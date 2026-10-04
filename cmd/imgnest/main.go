package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/biliblihuorong/imgnest/internal/cli"
	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		// Only command-layer redacted errors reach this boundary.
		if _, writeErr := os.Stderr.WriteString(err.Error() + "\n"); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
