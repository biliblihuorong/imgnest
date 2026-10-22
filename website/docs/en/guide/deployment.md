# Deployment

ImgNest can be deployed in two ways:

- **Docker** (recommended): the official image `ghcr.io/biliblihuorong/imgnest` for linux/amd64 and linux/arm64, with PostgreSQL by default or SQLite for personal use.
- **Binary**: build a single `imgnest` executable yourself (the web interface is embedded) and run it with systemd or similar. Prebuilt binaries are not published yet.

Both run the same program and the same database migrations, so you can switch between them later.

## Choosing a database

| | PostgreSQL (default) | SQLite |
| --- | --- | --- |
| Suited to | Production, many users or many images | Personal use, small sites, trying it out |
| Extra service | A PostgreSQL container or server (included in the compose file) | None; the database is a file in the data volume |
| Backup | Back up the `imgnest-data` volume plus `pg_dump` | Back up the `imgnest-data` volume |
| Lsky migration | Recommended; the `import-lsky` guide uses it | Also supported |

One image supports both. `IMGNEST_DATABASE_DRIVER` and `IMGNEST_DATABASE_DSN` select the database at run time.

## Docker

Install Docker with the Compose plugin, create a directory (for example `/opt/imgnest`) and save the following as `compose.yaml` (PostgreSQL by default; switch to the SQLite tab for SQLite). They match `deploy/compose.sqlite.yaml` and `deploy/compose.postgres.yaml` in the repository.

::: code-group

```yaml [PostgreSQL]
# ImgNest with PostgreSQL. Suited to multi-user deployments.
# Set POSTGRES_PASSWORD in the environment or an .env file next to this one.
name: imgnest
services:
  imgnest:
    image: ghcr.io/biliblihuorong/imgnest:${IMGNEST_TAG:-latest}
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "${IMGNEST_PORT:-8080}:8080"
    environment:
      IMGNEST_DATABASE_DRIVER: postgres
      IMGNEST_DATABASE_DSN: host=postgres user=imgnest password=${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD} dbname=imgnest sslmode=disable
      # Only needed for S3-compatible storage: base64 of 32 random bytes.
      IMGNEST_SECURITY_MASTER_KEY: ${IMGNEST_SECURITY_MASTER_KEY:-}
      # Comma-separated reverse proxy addresses allowed to set the client IP.
      IMGNEST_SERVER_TRUSTED_PROXIES: ${IMGNEST_SERVER_TRUSTED_PROXIES:-}
      # Comma-separated sites (such as your blog) allowed to call /api/variants from the browser.
      IMGNEST_IMAGES_VARIANT_CORS_ORIGINS: ${IMGNEST_IMAGES_VARIANT_CORS_ORIGINS:-}
    volumes:
      # Local image objects and the thumbnail cache.
      - imgnest-data:/app/data
  postgres:
    image: postgres:18.6-bookworm
    restart: unless-stopped
    environment:
      POSTGRES_USER: imgnest
      POSTGRES_DB: imgnest
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U imgnest -d imgnest"]
      interval: 5s
      timeout: 3s
      retries: 30
    volumes:
      - postgres-data:/var/lib/postgresql
volumes:
  imgnest-data:
  postgres-data:
```

```yaml [SQLite]
# Single container with SQLite. Suited to personal or small deployments.
name: imgnest
services:
  imgnest:
    image: ghcr.io/biliblihuorong/imgnest:${IMGNEST_TAG:-latest}
    restart: unless-stopped
    ports:
      - "${IMGNEST_PORT:-8080}:8080"
    environment:
      # Only needed for S3-compatible storage: base64 of 32 random bytes.
      IMGNEST_SECURITY_MASTER_KEY: ${IMGNEST_SECURITY_MASTER_KEY:-}
      # Comma-separated reverse proxy addresses allowed to set the client IP.
      IMGNEST_SERVER_TRUSTED_PROXIES: ${IMGNEST_SERVER_TRUSTED_PROXIES:-}
      # Comma-separated sites (such as your blog) allowed to call /api/variants from the browser.
      IMGNEST_IMAGES_VARIANT_CORS_ORIGINS: ${IMGNEST_IMAGES_VARIANT_CORS_ORIGINS:-}
    volumes:
      # Database, local image objects and the thumbnail cache.
      - imgnest-data:/app/data
volumes:
  imgnest-data:
```

:::

Create `.env` in the same directory (PostgreSQL needs `POSTGRES_PASSWORD`; never commit this file):

```ini
IMGNEST_TAG=0.1.6
IMGNEST_PORT=8080
# Required for PostgreSQL; generate with: openssl rand -base64 24
POSTGRES_PASSWORD=use-a-long-random-password
# Needed for S3 / B2 / R2 / COS storage; generate with: openssl rand -base64 32
IMGNEST_SECURITY_MASTER_KEY=
# Set when your blog looks up responsive images from the browser, e.g. https://blog.example.com
IMGNEST_IMAGES_VARIANT_CORS_ORIGINS=
```

Start it:

```bash
docker compose up -d
```

The container migrates the database on start. Then create the administrator and local storage as in [Getting started](./getting-started.md), dropping the quick start's `-f deploy/compose.sqlite.yaml` (Compose picks up `compose.yaml` in the current directory):

```bash
read -rsp 'Admin password: ' password; echo
printf '%s' "$password" | docker compose exec -T imgnest \
  imgnest init-admin --username admin --email admin@example.com
unset password
docker compose exec imgnest imgnest init-local --base-url https://img.example.com
```

::: warning Back up IMGNEST_SECURITY_MASTER_KEY
S3 storage credentials are stored encrypted with it. If you lose it, existing S3 storages cannot be decrypted and their keys must be entered again.
:::

### Upgrading

Change `IMGNEST_TAG` in `.env`, then run:

```bash
docker compose pull
docker compose up -d
```

The new container migrates the database on start. Back up the volumes first (plus a `pg_dump` for PostgreSQL).

## Binary

The binary is one file with the web interface embedded. It links libvips dynamically, so **the server must have libvips 8.18.x installed** (the same version it was built with).

### 1. Build

You need Git and Docker. Check out the version to deploy and build it in the development image, which contains Go, Node, pnpm and libvips:

```bash
git clone https://github.com/biliblihuorong/imgnest
cd imgnest
git checkout v0.1.6
make release   # builds the frontend, then writes bin/imgnest
```

Without Docker, install Go 1.27.3, Node 24, pnpm 12.9.1 and libvips 8.18.x (with development headers) and run:

```bash
cd web-vben && pnpm install --frozen-lockfile && pnpm build && cd ..
go build -trimpath -o bin/imgnest ./cmd/imgnest
```

### 2. Configure

Copy `bin/imgnest` to the server, for example `/opt/imgnest/imgnest`, and copy `deploy/config.example.yaml` from the repository to `/opt/imgnest/config.yaml`. Relative paths (`data/imgnest.db`, `data/thumbs`, and `data/images` for local storage) are resolved from the working directory. Without a config file every setting uses its default.

- **PostgreSQL (recommended)**: select it with environment variables and keep the password out of the file:

  ```bash
  IMGNEST_DATABASE_DRIVER=postgres
  IMGNEST_DATABASE_DSN="host=127.0.0.1 user=imgnest password=... dbname=imgnest sslmode=disable"
  ```

  Remove the SQLite-only `max_open: 1` and `max_idle: 1` from the file so the PostgreSQL pool defaults apply.
- **SQLite**: leave those two variables unset; the example configuration works as is and the database is `data/imgnest.db`.

Every setting is listed in [Configuration](./configuration.md).

### 3. Initialize and run

The binary does not migrate on its own; run `migrate` before the first start and before every upgrade:

```bash
cd /opt/imgnest
mkdir -p data
./imgnest --config config.yaml migrate
read -rsp 'Admin password: ' password; echo
printf '%s' "$password" | ./imgnest --config config.yaml init-admin --username admin --email admin@example.com
unset password
./imgnest --config config.yaml init-local --base-url https://img.example.com
./imgnest --config config.yaml serve
```

To keep it running with systemd, save this as `/etc/systemd/system/imgnest.service`:

```ini
[Unit]
Description=ImgNest
After=network-online.target
Wants=network-online.target

[Service]
User=imgnest
WorkingDirectory=/opt/imgnest
# PostgreSQL DSN, S3 master key and so on; chmod 600
EnvironmentFile=-/opt/imgnest/imgnest.env
ExecStartPre=/opt/imgnest/imgnest --config config.yaml migrate
ExecStart=/opt/imgnest/imgnest --config config.yaml serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --home /opt/imgnest imgnest
sudo chown -R imgnest:imgnest /opt/imgnest
sudo systemctl daemon-reload
sudo systemctl enable --now imgnest
```

To upgrade, replace `/opt/imgnest/imgnest` and run `sudo systemctl restart imgnest`; `ExecStartPre` migrates the database first.

## Enabling responsive images

Responsive images arrived in 0.0.2. When enabled, each new still image also gets WebP versions 400, 800 and 1600 pixels wide besides the original, WebP and thumbnails, so a blog can use `srcset` to serve small files to phones and large ones to big screens. They are off by default; turn them on in three steps:

1. **Enable them in a rule**: in the admin panel edit a policy, open its "Transcoding" tab and turn on "Responsive variants". New uploads get them automatically, and both the upload API and the Lsky v1 API return an extra `variants` field.
2. **Backfill existing images**: start "Responsive variant backfill" at the bottom of the policies page. It can be paused and resumed and continues after a restart; images not processed yet keep working as before.
3. **Let your blog look them up from the browser** (optional): a blog that only knows image links can call the public `GET /api/variants?url=image-link` endpoint. Add the blog's origin to `IMGNEST_IMAGES_VARIANT_CORS_ORIGINS` (already in the `.env` above; for the binary, set the variable or `images.variant_cors_origins` in the config) and restart.

Files are named like `{path}_w800_v1.webp`, live in the same storage as the original, count toward quota, and are deleted and restored with it. See [Storage and policies](./storage-and-policies.md) for the details and `docs/examples/responsive-images.html` in the repository for a blog example.

## Reverse proxy

Either way, put Nginx, Caddy or similar in front for HTTPS and list the proxy in `IMGNEST_SERVER_TRUSTED_PROXIES`, otherwise every upload is recorded with the proxy's IP. ImgNest enforces the upload size limit; keep the proxy's body limit (Nginx `client_max_body_size`) at least as large, and raise it too if you raise a user group's per-file limit. Large files are held in memory in full while they upload, convert and have their EXIF processed, so leave the server enough.

::: warning Run a single instance
Never run more than one ImgNest against the same data, and never point the Docker and binary versions at the same database at once.
:::
