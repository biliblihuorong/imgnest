#!/bin/sh
# Real Go dependency-graph and selected filesystem checks; no libvips link needed.
# This is deliberately narrower than building/testing the complete server.
set -eu
cd "$(dirname "$0")/.."
GO=${GO:-go}
node scripts/check-legacy-source.mjs
module=github.com/biliblihuorong/imgnest
legacy=$($GO list -mod=readonly -deps -f '{{.ImportPath}}' ./cmd/imgnest)
vben=$($GO list -mod=readonly -tags vben -deps -f '{{.ImportPath}}' ./cmd/imgnest)
printf '%s\n' "$legacy" | grep -Fx "$module/web" >/dev/null
if printf '%s\n' "$legacy" | grep -Fx "$module/web-vben" >/dev/null; then
    echo 'legacy dependency graph includes Vben assets' >&2
    exit 1
fi
printf '%s\n' "$vben" | grep -Fx "$module/web-vben" >/dev/null
if printf '%s\n' "$vben" | grep -Fx "$module/web" >/dev/null; then
    echo 'Vben dependency graph includes legacy assets' >&2
    exit 1
fi
(
    cd internal/cli
    "$GO" test -mod=readonly frontend_legacy.go frontend_selection_test.go frontend_legacy_test.go
    "$GO" test -mod=readonly -tags vben frontend_vben.go frontend_selection_test.go frontend_vben_test.go
)
echo 'verified exclusive legacy/Vben dependency graphs and selected embedded files'
