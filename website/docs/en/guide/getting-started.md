# Getting started

This page gets ImgNest running locally with SQLite and uploads a first image.

## Before you start

- Docker Desktop with the Linux engine
- The source: `git clone https://github.com/biliblihuorong/imgnest`

Run every command from the repository root. The development image pins the Go and libvips versions, so nothing else needs to be installed.

## 1. Build the image and initialise the database

```bash
docker compose -f deploy/compose.dev.yaml build dev
docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest migrate
```

The server refuses to start on an uninitialised database, so `migrate` has to run first. Data is stored in `data/imgnest.db` by default.

## 2. Create the administrator

The password is read from standard input and never appears in arguments, files or logs. It must be 12 to 72 bytes long.

::: code-group

```powershell [PowerShell]
$password = Read-Host 'Administrator password' -AsSecureString
$plain = [System.Net.NetworkCredential]::new('', $password).Password
$plain | docker compose -f deploy/compose.dev.yaml run --rm -T dev `
  go run ./cmd/imgnest init-admin --username admin --email admin@example.com
Remove-Variable plain, password
```

```bash [Bash]
read -rsp 'Administrator password: ' password; echo
printf '%s' "$password" | docker compose -f deploy/compose.dev.yaml run --rm -T dev \
  go run ./cmd/imgnest init-admin --username admin --email admin@example.com
unset password
```

:::

Replace the username and email with your own. Running the command again does not overwrite an existing administrator.

## 3. Create a local storage

```bash
docker compose -f deploy/compose.dev.yaml run --rm dev \
  go run ./cmd/imgnest init-local --base-url http://localhost:18080
```

This creates a local storage and a default upload policy, and binds them to the default user group.

## 4. Start the server

```bash
docker compose -f deploy/compose.dev.yaml run --rm --service-ports dev go run ./cmd/imgnest serve
```

The server listens on `127.0.0.1:18080`. Open <http://127.0.0.1:18080> and sign in with the email and password from step 2.

To check that it is ready:

```bash
curl http://127.0.0.1:18080/healthz
```

Press `Ctrl+C` to stop the server.

## 5. Upload an image

After signing in, open **Upload** and drop an image in. You get three addresses back: the original, the WebP copy and the thumbnail. The WebP address is copied by default.

::: tip Registration is off by default
A fresh site does not accept sign-ups. An administrator can turn registration on under **Administration → Site settings**.
:::

## Use the new interface

The default build embeds the classic frontend. To get the interface with the new layout, build the other binary:

```bash
make release-vben
```

The result is `bin/imgnest-vben`. [Interface and themes](./frontends) explains the difference.

## Next

- To change the port, upload limits or switch to PostgreSQL, read [Configuration](./configuration).
- To store images in an S3-compatible service, read [Storage and policies](./storage-and-policies).
