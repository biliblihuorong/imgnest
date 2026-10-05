//go:build vben

package cli

import "testing"

func TestFrontendDistFSVben(t *testing.T) {
	t.Parallel()
	assertFrontendDistFS(t, "../../web-vben/dist")
}
