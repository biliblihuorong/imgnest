//go:build !vben

package cli

import "testing"

func TestFrontendDistFSLegacy(t *testing.T) {
	t.Parallel()
	assertFrontendDistFS(t, "../../web/dist")
}
