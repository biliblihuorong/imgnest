package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/cli"
)

func TestExecuteHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := cli.Execute(t.Context(), []string{"--help"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ImgNest") {
		t.Fatalf("missing application identity: %q", out.String())
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := cli.Execute(t.Context(), []string{"does-not-exist"}, &out, &errOut); err == nil {
		t.Fatal("unknown command succeeded")
	}
}
