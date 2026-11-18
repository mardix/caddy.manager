# caddy.manager

A small, single-user web UI for running [Caddy](https://caddyserver.com) as a reverse proxy.
Add a domain, point it at a service, and Caddy handles HTTPS. Every change is checked by Caddy,
shown as a diff, backed up, and applied with a zero-downtime reload.
An optional AI assistant helps you understand what's going on: it explains errors in plain language,
answers questions about your traffic and logs, and suggests fixes for you to review.

![Overview](docs/screenshots/overview.png)

## Features

- **Hosts:** proxy, redirect (301/302/307/308) and 404 hosts; several domains per host; wildcards (`*.example.com`)
- **Path routing:** send `/api` to another service, with optional prefix stripping
- **Access control:** password logins and IP allow/deny rules, per host or as reusable access lists
- **HTTPS:** automatic (Let’s Encrypt / ZeroSSL), DNS challenge via Cloudflare, self-signed, your own files, or plain HTTP
- **Extras:** load balancing with health checks, custom headers, maintenance mode, exploit blocking, compression, static caching
- **Safe changes:** `caddy validate` before every apply, a line-by-line diff, automatic backups with one-click restore
- **Caddyfile:** download the active file, or upload an existing one to **import its sites as managed hosts** (or run it as-is)
- **Monitoring:** upstream health checks, traffic stats on the overview, and a searchable, downloadable log per host
- **Certificates:** one page listing every certificate with its status, issuer and expiry, plus **Retry now** when issuing fails
- **Optional AI assistant:** Anthropic, OpenAI, any OpenAI-compatible endpoint, or Ollama. It explains errors, answers questions about your logs, and drafts host changes for you to review. It never applies anything itself.

## Quick start (Docker)

```sh
cp .env.example .env        # set ADMIN_PASSWORD (8+ characters)
docker compose up -d --build # or: docker compose pull && docker compose up -d   (uses the published image)
```

Open `http://<server>:8080` and sign in with `ADMIN_PASSWORD`.
Ports 80 and 443 must reach this machine for automatic HTTPS.

## Build and deploy with GitHub Actions

`.github/workflows/docker.yml` runs on every push and pull request:

1. **Test:** `gofmt`, `go vet` and the Go tests, validating generated Caddyfiles against a real Caddy.
2. **Build:** a multi-arch image (amd64 and arm64). Pushes to `main` publish `ghcr.io/mardix/caddy.manager:latest`;
   tags like `v1.2.0` also publish `1.2.0` and `1.2`. Pull requests build but don't push.
3. **Deploy (optional):** SSH into your server, `docker compose pull` and restart. Skipped until you configure it.

To turn on deploys, add these in the repository's **Settings → Secrets and variables → Actions**:

| Name | Kind | Value |
| --- | --- | --- |
| `DEPLOY_HOST` | Variable | Server hostname or IP |
| `DEPLOY_USER` | Variable | SSH user (must be able to run `docker`) |
| `DEPLOY_PATH` | Variable | Folder holding `docker-compose.yml` and `.env` (default `~/caddy-manager`) |
| `DEPLOY_PORT` | Variable | SSH port, if not 22 |
| `DEPLOY_SSH_KEY` | Secret | Private key whose public key is in the server's `authorized_keys` |
| `GHCR_TOKEN` | Secret | Only if the package is private: a token with `read:packages` |

On the server, the folder only needs `docker-compose.yml` and `.env`. Deploys run in a `production` environment,
so you can require an approval under **Settings → Environments**. New packages on GHCR start private: either make
the package public or set `GHCR_TOKEN`.

## Run locally (without Docker)

You need Go 1.24+ and the [caddy](https://caddyserver.com/docs/install) binary.

```sh
cp .env.example .env        # set ADMIN_PASSWORD
./run.sh                    # builds and starts the UI on http://127.0.0.1:8080
./run.sh test               # runs the Go tests
```

`run.sh` reads `.env`, keeps data in `./data`, and finds `caddy` on your `PATH` (or use `CADDY_BIN=/path/to/caddy`).
Caddy listens on ports 80 and 443, so either run as root or allow it once:
`sudo setcap cap_net_bind_service=+ep "$(command -v caddy)"`.
Override `DATA_DIR` or `LISTEN` the same way if you need to.

## Screenshots

Screenshots use demo data.

### Hosts
All hosts in one list, filtered by type, with certificate, access and health at a glance.
Switch to **Caddyfile** to view, download or upload the file itself.

![Hosts](docs/screenshots/hosts.png)
![Caddyfile view](docs/screenshots/caddyfile.png)

### Editing a host
Pick **Proxy**, **Redirect** or **404 host**, then fill in the tabs. The panel on the right shows the
host's Caddyfile block as you type.

![Host details](docs/screenshots/editor-details.png)

| Path routing | Access | SSL |
| --- | --- | --- |
| ![Paths](docs/screenshots/editor-paths.png) | ![Access](docs/screenshots/editor-access.png) | ![SSL](docs/screenshots/editor-ssl.png) |

### Review before applying
Caddy validates the new config, upstreams are checked, and you see exactly what changes.
The current Caddyfile is backed up first.

![Review](docs/screenshots/review.png)

### Certificates
Every host's certificate in one list. If one fails (DNS not pointing here yet, port 80 blocked), fix the cause and press
**Retry now**: Caddy requests it again immediately instead of waiting out its back-off.

![Certificates](docs/screenshots/certificates.png)

### Access lists
Logins and IP rules defined once and reused on any host.

![Access lists](docs/screenshots/access-lists.png)

### Logs
Every host has its own log page with stats, a filterable request log and the AI helper.
**Logs & stats** shows the same for all hosts together.

![Host log](docs/screenshots/host-log.png)
![Logs & stats](docs/screenshots/logs.png)

### AI assistant (optional)
**Explain** turns errors into plain language with concrete fixes. **Ask** (⌘K / Ctrl+K) drafts a change
that opens in the editor for you to review. Password hashes and API tokens are never sent, and IPs can be masked.

| Explain | Ask |
| --- | --- |
| ![Explain](docs/screenshots/explain.png) | ![Ask](docs/screenshots/ask.png) |

![Settings](docs/screenshots/settings.png)

### On a phone

| Overview | Editor |
| --- | --- |
| ![Phone overview](docs/screenshots/phone-overview.png) | ![Phone editor](docs/screenshots/phone-editor.png) |

## Notes

- **Services on the Docker host:** forward to `host.docker.internal:PORT`, not `localhost` (that's the container).
- **Wildcard certificates:** add a Cloudflare API token (Zone → DNS → Edit) in Settings, then choose **DNS challenge** on the host.
- **Admin UI over HTTPS:** add a proxy host such as `admin.example.com → localhost:8080` and protect it with an access list.
  To keep port 8080 off the network, change it to `"127.0.0.1:8080:8080"` in `docker-compose.yml`.
- **Migrating an existing Caddyfile:** upload it from Hosts → Caddyfile and choose **Review import**. Each site becomes a managed host
  (proxy, redirect, 404, path routes, logins, load balancing, headers, `tls internal` / Cloudflare DNS). Sites using anything the editor
  doesn't manage (e.g. `file_server`, matchers, `header_up`, certificate files) are kept exactly as written under
  Settings → Extra Caddyfile config, so nothing is lost. `localhost` upstreams can be pointed at `host.docker.internal` during import.
  **Run as-is instead** keeps the old behaviour: the file runs unchanged and nothing is listed as hosts.
- **Data:** everything lives in `./data`: `state.json` (hosts and access lists), `settings.json`, the active `Caddyfile`,
  certificates (`data/caddy`), logs and the last 30 backups.
- **Password:** changing `ADMIN_PASSWORD` and restarting signs out existing sessions.

## Backlog

- **AI that reacts to live behaviour.** Today the assistant only explains and suggests when you ask. In the future it could watch
  live traffic and health (an upstream going down, a burst of 5xx errors, a scanner hammering one host) and propose changes on its own,
  such as blocking an IP, switching to a backup upstream or turning on maintenance mode. Changes would still go through the same
  validation and backups, with a clear opt-in and a review or auto-revert step before anything stays applied.

## Development

Backend: Go 1.24, standard library only. Frontend: plain HTML, CSS and JavaScript in `web/`, embedded in the binary. No build step.

```sh
ADMIN_PASSWORD=devpassword DATA_DIR=./data CADDY_BIN=$(which caddy) LISTEN=127.0.0.1:8080 go run .
CADDY_BIN=$(which caddy) go test ./...
```
