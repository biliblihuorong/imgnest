package cli

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// A reconfigured backend replaces its cached driver instead of adding one
// more entry per configuration for the life of the process.
func TestDriverFactoryReplacesSupersededDriver(t *testing.T) {
	factory, err := newDriverFactory(t.Context(), base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{t.TempDir(), t.TempDir(), t.TempDir()} {
		config, _ := json.Marshal(map[string]string{"root": root})
		if _, err := factory.DriverFor(t.Context(), model.Storage{ID: 7, Driver: "local", Config: config}); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := json.Marshal(map[string]string{"root": t.TempDir()})
	if _, err := factory.DriverFor(t.Context(), model.Storage{ID: 70, Driver: "local", Config: other}); err != nil {
		t.Fatal(err)
	}
	if len(factory.drivers) != 2 {
		t.Fatalf("cached drivers = %d, want one per backend", len(factory.drivers))
	}
}
