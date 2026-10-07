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

FROM ghcr.io/cshum/imagor-base:vips8.18.6-r14-dev AS build
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:/go/bin:/opt/imagor/bin:$PATH
ENV CGO_ENABLED=1 GOTOOLCHAIN=local \
    GOPATH=/go GOMODCACHE=/go/pkg/mod GOCACHE=/root/.cache/go-build \
    PKG_CONFIG_PATH=/opt/imagor/lib/pkgconfig \
    CGO_CFLAGS=-I/opt/imagor/include \
    CGO_LDFLAGS="-L/opt/imagor/lib -Wl,-rpath,/opt/imagor/lib" \
    LD_LIBRARY_PATH=/opt/imagor/lib
# Keep this libvips build identical to deploy/Dockerfile.dev: the tests run
# against that build, and the pinned imagor image cannot load BMP without Magick.
ADD --checksum=sha256:3c41e1d5458081bfa4a5bc54e116c46259c75c6760a18027764555632b9dda3e https://github.com/libvips/libvips/releases/download/v8.18.6/vips-8.18.6.tar.xz /tmp/vips.tar.xz
RUN apt-get update && apt-get install -y --no-install-recommends meson ninja-build && \
    rm -rf /var/lib/apt/lists/*
RUN tar -xf /tmp/vips.tar.xz -C /tmp && \
    meson setup /tmp/vips-build /tmp/vips-8.18.6 --prefix=/opt/imagor --libdir=lib \
      -Dmagick=enabled -Dintrospection=disabled -Ddeprecated=false && \
    ninja -C /tmp/vips-build -j 2 && ninja -C /tmp/vips-build install && \
    rm -rf /tmp/vips-build /tmp/vips-8.18.6 /tmp/vips.tar.xz && \
    mkdir /out && cp -a /opt/imagor/lib/libvips.so* /opt/imagor/lib/vips-modules-8.18 /out/
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web-vben/embed.go web-vben/embed.go
COPY --from=frontend /src/dist/ web-vben/dist/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false -trimpath -ldflags "-s -w" -o /out/imgnest ./cmd/imgnest

FROM ghcr.io/cshum/imagor-base:vips8.18.6-r14 AS runtime
# Shared libraries the Magick-enabled libvips links beyond the base image.
RUN apt-get update && \
    apt-get install -y --no-install-recommends libmagickcore-6.q16-7t64 libopenexr-3-1-30 && \
    rm -rf /var/lib/apt/lists/* && \
    ln -s "/usr/lib/$(uname -m)-linux-gnu/libjemalloc.so.2" /usr/local/lib/libjemalloc.so.2 && \
    useradd --system --uid 10001 --user-group --no-create-home --shell /usr/sbin/nologin imgnest && \
    mkdir -p /app/data && chown imgnest:imgnest /app/data
COPY --from=build /out/libvips.so* /opt/imagor/lib/
COPY --from=build /out/vips-modules-8.18/ /opt/imagor/lib/vips-modules-8.18/
COPY --from=build /out/imgnest /usr/local/bin/imgnest
COPY --chmod=0755 deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
# Fail the build rather than the first upload if any library is unresolved.
RUN if ldd /usr/local/bin/imgnest /opt/imagor/lib/libvips.so.42 /opt/imagor/lib/vips-modules-8.18/*.so | grep 'not found'; then exit 1; fi
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
