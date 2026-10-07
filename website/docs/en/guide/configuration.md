# Configuration

Deployment settings live in a config file or environment variables. Site-level settings such as the site name or the registration switch are changed in the admin panel and stored in the database.

## Where settings come from

With Docker, the simplest way is to set environment variables under `environment` in the compose file. `deploy/compose.sqlite.yaml` and `deploy/compose.postgres.yaml` in the repository already wire up the common ones.

A config file works too. `deploy/config.example.yaml` is the example; copy it and load it explicitly with `--config`:

```bash
imgnest serve --config config.yaml
```

`--config` works on every subcommand. Pass it to `migrate`, `init-admin` and the others when they should read the same file.

Three sources apply in this order, later ones overriding earlier ones:

1. Built-in defaults
2. The config file
3. Environment variables starting with `IMGNEST_`

A variable name is the config path in upper case joined by underscores: `database.max_open` becomes `IMGNEST_DATABASE_MAX_OPEN`.

::: warning The server will not start on a bad config
A missing config file, invalid YAML, an invalid number or an unknown database driver stops startup. Error messages never echo secrets or the whole config.
:::

## Server

| Key | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `server.addr` | `IMGNEST_SERVER_ADDR` | `:8080` | Listen address |
| `server.trusted_proxies` | `IMGNEST_SERVER_TRUSTED_PROXIES` | empty | When empty, client IPs in forwarding headers are ignored |
| `server.read_header_timeout` | `IMGNEST_SERVER_READ_HEADER_TIMEOUT` | `5s` | Timeout for reading request headers |
| `server.shutdown_timeout` | `IMGNEST_SERVER_SHUTDOWN_TIMEOUT` | `10s` | How long to wait for requests when stopping |

## Upload limits

These settings cap the resources the server spends on uploads.

| Key | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `server.max_upload_mb` | `IMGNEST_SERVER_MAX_UPLOAD_MB` | `20` | Size limit for one file, 1 to 20 |
| `server.max_request_mb` | `IMGNEST_SERVER_MAX_REQUEST_MB` | `64` | Size limit for the whole request, at most 256 |
| `server.upload_concurrency` | `IMGNEST_SERVER_UPLOAD_CONCURRENCY` | `2` | Upload requests processed at once |
| `server.processing_timeout` | `IMGNEST_SERVER_PROCESSING_TIMEOUT` | `5m` | Time limit for one upload, at most 30 minutes |
| `server.max_pixels` | `IMGNEST_SERVER_MAX_PIXELS` | `100000000` | Total pixel limit; animations count every frame |

One request can carry at most 20 files. These limits are checked before a file is read in full.

## Database

| Key | Environment variable | Default |
| --- | --- | --- |
| `database.driver` | `IMGNEST_DATABASE_DRIVER` | `sqlite` |
| `database.dsn` | `IMGNEST_DATABASE_DSN` | `data/imgnest.db` |
| `database.max_open` / `max_idle` | `IMGNEST_DATABASE_MAX_OPEN` / `MAX_IDLE` | 1 / 1 for SQLite, 25 / 10 for PostgreSQL |
| `database.max_lifetime` | `IMGNEST_DATABASE_MAX_LIFETIME` | 0 for SQLite, `5m` for PostgreSQL |
| `database.busy_timeout` | `IMGNEST_DATABASE_BUSY_TIMEOUT` | `5s` |

### SQLite

Good for personal use with no extra service to run. ImgNest turns on WAL and foreign keys and uses a single connection.

### PostgreSQL

Switch to PostgreSQL when there are many users or many concurrent uploads. Set the driver to `postgres` and inject the connection string through the environment rather than a committed file:

```ini
IMGNEST_DATABASE_DRIVER=postgres
IMGNEST_DATABASE_DSN=postgres://user:password@host:5432/imgnest
```

`deploy/compose.postgres.yaml` in the repository already sets up the database container and both variables; you only provide `POSTGRES_PASSWORD`. After switching databases, create the administrator and storage again.

::: tip Migrations are built in
Migration scripts are compiled into the program, and running `migrate` again writes nothing twice. A modified published migration, a missing version or an unknown version is reported as an error.
:::

## Other settings

| Key | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `images.thumb_cache` | `IMGNEST_IMAGES_THUMB_CACHE` | `data/thumbs` | Local thumbnail directory; safe to delete and rebuild |
| `security.master_key` | `IMGNEST_SECURITY_MASTER_KEY` | empty | Master key that encrypts S3 credentials; may stay empty with local storage only |

S3 storage requires a master key: 32 random bytes, base64-encoded. Keep it together with your database backups. Without it, saved storage credentials cannot be decrypted.

```bash
openssl rand -base64 32
```
