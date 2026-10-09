# syntax=docker/dockerfile:1
# Production image. One image serves both SQLite and PostgreSQL; the database
# is selected at runtime through IMGNEST_DATABASE_DRIVER / IMGNEST_DATABASE_DSN.

# The SPA bundle is architecture independent, so build it once on the host.
FROM --platform=$BUILDPLATFORM node:24.21.0-bookworm AS frontend
RUN npm install -g pnpm@12.9.1 && npm cache clean --force
WORKDIR /src
COPY web-vben/ ./
RUN pnpm install --frozen-lockfile && pnpm build

FROM golang:1.27.1-bookworm@sha256:8d48e12ec56735e9358640898b9d9b9fcca110612ed8a5567438c0a1baa24e66 AS go-toolchain

# Magick-enabled libvips 8.18.6, built once by deploy/Dockerfile.vips. The dev
# image and CI link this same build.
FROM ghcr.io/biliblihuorong/imgnest-vips:8.18.6-r1-dev@sha256:e80eb82e32bff708d47acad906d112496025b845c673c62b886861b73efab38e AS build
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:/go/bin:/opt/imagor/bin:$PATH
ENV CGO_ENABLED=1 GOTOOLCHAIN=local \
    GOPATH=/go GOMODCACHE=/go/pkg/mod GOCACHE=/root/.cache/go-build \
    PKG_CONFIG_PATH=/opt/imagor/lib/pkgconfig \
    CGO_CFLAGS=-I/opt/imagor/include \
    CGO_LDFLAGS="-L/opt/imagor/lib -Wl,-rpath,/opt/imagor/lib" \
    LD_LIBRARY_PATH=/opt/imagor/lib
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY app/ app/
COPY cmd/ cmd/
COPY extension/ extension/
COPY internal/ internal/
COPY web-vben/embed.go web-vben/embed.go
COPY --from=frontend /src/dist/ web-vben/dist/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false -trimpath -ldflags "-s -w" -o /out/imgnest ./cmd/imgnest

FROM ghcr.io/biliblihuorong/imgnest-vips:8.18.6-r1@sha256:8d44c0cbea219cdc09d7e5a63655d0d8a64539901e0c42fa7bbb030a73632104 AS runtime
RUN useradd --system --uid 10001 --user-group --no-create-home --shell /usr/sbin/nologin imgnest &&     mkdir -p /app/data && chown imgnest:imgnest /app/data
COPY --from=build /out/imgnest /usr/local/bin/imgnest
COPY --chmod=0755 deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
# Restrict ImageMagick to the BMP reader libvips needs, with resource caps.
COPY --chmod=0644 deploy/imagemagick-policy.xml /etc/ImageMagick-6/policy.xml
# Fail the build rather than the first upload if any library is unresolved.
RUN if ldd /usr/local/bin/imgnest | grep 'not found'; then exit 1; fi
ENV MALLOC_ARENA_MAX=2 \
    LD_PRELOAD=/usr/local/lib/libjemalloc.so.2
WORKDIR /app
USER imgnest
VOLUME /app/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD bash -c 'exec 3<>/dev/tcp/127.0.0.1/8080 && printf "GET /healthz HTTP/1.0\r\n\r\n" >&3 && grep -q " 200 " <&3'
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["serve"]
