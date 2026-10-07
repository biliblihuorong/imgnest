#!/bin/sh
# Starts the production image on SQLite and checks that it migrates, serves
# /healthz and the embedded frontend, and runs as a non-root user.
set -eu
image=${1:?usage: smoke-image.sh IMAGE}
name=imgnest-smoke-$$
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$name" -p 127.0.0.1::8080 "$image" >/dev/null
port=$(docker port "$name" 8080/tcp | head -n 1 | sed 's/.*://')
attempt=0
until curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 30 ]; then
        docker logs "$name" >&2
        echo 'image did not become healthy' >&2
        exit 1
    fi
    sleep 1
done
curl -fsS "http://127.0.0.1:$port/" | grep -qi '<!doctype html'
[ "$(docker exec "$name" id -u)" = 10001 ]
echo "smoke test passed: $image"
