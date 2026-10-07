#!/bin/sh
# serve refuses to start on an unmigrated schema, so apply the versioned
# migrations first. They are idempotent; other subcommands run unchanged.
set -eu
if [ "${1:-}" = serve ]; then
    imgnest migrate
fi
exec imgnest "$@"
