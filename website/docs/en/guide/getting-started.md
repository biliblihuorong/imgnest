# Getting started

This page gets ImgNest running from the Docker image with SQLite and uploads a first image.

## Before you start

- Docker with the Compose plugin
- The compose files from the repository: `git clone https://github.com/biliblihuorong/imgnest`

Run every command from the repository root. The image is `ghcr.io/biliblihuorong/imgnest`, built for linux/amd64 and linux/arm64.

## 1. Start the server

::: code-group

```bash [Bash]
IMGNEST_TAG=edge docker compose -f deploy/compose.sqlite.yaml up -d
```

```powershell [PowerShell]
$env:IMGNEST_TAG = 'edge'
docker compose -f deploy/compose.sqlite.yaml up -d
```

:::

The container runs database migrations on start. Data lives in a volume named `imgnest-data`: the database, local image files and the thumbnail cache.

::: tip Why edge
`edge` follows the latest commit on `main`. Once a stable version is released there will be `latest` and versioned tags, and `IMGNEST_TAG` can be dropped.
:::

## 2. Create the administrator

The password is read from standard input and never appears in arguments or logs. It must be 12 to 72 bytes long.

::: code-group

```bash [Bash]
read -rsp 'Administrator password: ' password; echo
printf '%s' "$password" | docker compose -f deploy/compose.sqlite.yaml exec -T imgnest \
  imgnest init-admin --username admin --email admin@example.com
unset password
```

```powershell [PowerShell]
$password = Read-Host 'Administrator password' -AsSecureString
$plain = [System.Net.NetworkCredential]::new('', $password).Password
$plain | docker compose -f deploy/compose.sqlite.yaml exec -T imgnest `
  imgnest init-admin --username admin --email admin@example.com
Remove-Variable plain, password
```

:::

Replace the username and email with your own. Running the command again does not overwrite an existing administrator.

## 3. Create a local storage

```bash
docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-local --base-url http://localhost:8080
```

This creates a local storage and a default upload policy, and binds them to the default user group. `--base-url` is the address other people use to reach your image host; use your domain in a real deployment.

## 4. Sign in and upload

Open <http://localhost:8080>, sign in with the email and password from step 2, open **Upload** and drop an image in. You get three addresses back: the original, the WebP copy and the thumbnail. The WebP address is copied by default.

To check that the server is ready:

```bash
curl http://localhost:8080/healthz
```

::: tip Registration is off by default
A fresh site does not accept sign-ups. An administrator can turn registration on under **Administration → Site settings**.
:::

## Common tasks

| Task | How |
| --- | --- |
| Change the port | Set `IMGNEST_PORT` before starting; the default is 8080 |
| Stop the server | `docker compose -f deploy/compose.sqlite.yaml down` |
| Use PostgreSQL | Use `deploy/compose.postgres.yaml` and set `POSTGRES_PASSWORD` first |
| Build the image yourself | `docker build -f deploy/Dockerfile -t imgnest:local .` |

::: warning Run a single instance
Do not start more than one ImgNest container against the same data.
:::

## Next

- To change upload limits or the database connection, read [Configuration](./configuration).
- To store images in an S3-compatible service, read [Storage and policies](./storage-and-policies).
- To work on the code, read [`docs/development.md`](https://github.com/biliblihuorong/imgnest/blob/main/docs/development.md) in the repository.
