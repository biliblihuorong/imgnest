# Deployment

ImgNest can be deployed in two ways:

- **Docker** (recommended): the official image `ghcr.io/biliblihuorong/imgnest` for linux/amd64 and linux/arm64, with SQLite or PostgreSQL.
- **Binary**: build a single `imgnest` executable yourself (the web interface is embedded) and run it with systemd or similar. Prebuilt binaries are not published yet.

Both run the same program and the same database migrations, so you can switch between them later.

## Choosing a database

| | SQLite | PostgreSQL |
| --- | --- | --- |
| Suited to | Personal use, small sites | Many users or many images |
| Extra service | None; the database is a file in the data volume | A PostgreSQL container or server |
| Backup | Back up the `imgnest-data` volume | Back up the `imgnest-data` volume plus `pg_dump` |

One image supports both. `IMGNEST_DATABASE_DRIVER` and `IMGNEST_DATABASE_DSN` select the database at run time.

## Docker

Install Docker with the Compose plugin, create a directory (for example `/opt/imgnest`) and save one of the following as `compose.yaml`. They match `deploy/compose.sqlite.yaml` and `deploy/compose.postgres.yaml` in the repository.

::: code-group

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
    volumes:
      # Database, local image objects and the thumbnail cache.
      - imgnest-data:/app/data
volumes:
  imgnest-data:
```

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

:::

Create `.env` in the same directory (PostgreSQL needs `POSTGRES_PASSWORD`; never commit this file):

```ini
IMGNEST_TAG=0.0.3
IMGNEST_PORT=8080
# Required for PostgreSQL
POSTGRES_PASSWORD=use-a-long-random-password
# Needed for S3 / B2 / R2 / COS storage; generate with: openssl rand -base64 32
IMGNEST_SECURITY_MASTER_KEY=
```

Start it:

```bash
docker compose up -d
```

The container migrates the database on start. Then create the administrator and local storage as in [Getting started](./getting-started), dropping `-f deploy/compose.sqlite.yaml` (Compose picks up `compose.yaml` in the current directory):

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
git checkout v0.0.3
make release   # builds the frontend, then writes bin/imgnest
```

Without Docker, install Go 1.27.1, Node 24, pnpm 12.9.1 and libvips 8.18.x (with development headers) and run:

```bash
cd web-vben && pnpm install --frozen-lockfile && pnpm build && cd ..
go build -trimpath -o bin/imgnest ./cmd/imgnest
```

### 2. Configure

Copy `bin/imgnest` to the server, for example `/opt/imgnest/imgnest`, and copy `deploy/config.example.yaml` from the repository to `/opt/imgnest/config.yaml`. Relative paths (`data/imgnest.db`, `data/thumbs`, and `data/images` for local storage) are resolved from the working directory. Without a config file every setting uses its default.

- **SQLite**: the example configuration works as is; the database is `data/imgnest.db`.
- **PostgreSQL**: select it with environment variables and keep the password out of the file:

  ```bash
  IMGNEST_DATABASE_DRIVER=postgres
  IMGNEST_DATABASE_DSN="host=127.0.0.1 user=imgnest password=... dbname=imgnest sslmode=disable"
  ```

  Remove the SQLite-only `max_open: 1` and `max_idle: 1` from the file so the PostgreSQL pool defaults apply.

Every setting is listed in [Configuration](./configuration).

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

## Reverse proxy

Either way, put Nginx, Caddy or similar in front for HTTPS and list the proxy in `IMGNEST_SERVER_TRUSTED_PROXIES`, otherwise every upload is recorded with the proxy's IP. ImgNest enforces the upload size limit; keep the proxy's body limit (Nginx `client_max_body_size`) at least as large.

::: warning Run a single instance
Never run more than one ImgNest against the same data, and never point the Docker and binary versions at the same database at once.
:::
