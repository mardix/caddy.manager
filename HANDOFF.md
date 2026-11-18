# Handoff: caddy.manager

A single-user web UI that manages a Caddy reverse proxy. One Go binary serves the UI and supervises Caddy.
This file covers how it's built, what has been verified, and what is still open.

## Status

**Working, verified locally against real Caddy 2.11.9:**
- Every host type, routing, access control, backups and restore, uploading your own Caddyfile, stats and logs.
- The full UI in a headless browser at desktop and phone widths.

**Not yet verified:**
- **Docker image:** the build itself. There was no Docker daemon or Docker Hub access in the build environment.
- **GitHub Actions:** the workflow hasn't run yet.
- **Real certificates:** Let's Encrypt issuance on a public domain.
- **Wildcard certificates:** the Cloudflare DNS plugin.
- **AI:** real providers. Only a mock that speaks both the Anthropic and OpenAI formats was used.

**First things to do:**
1. Push to GitHub and watch the workflow.
2. Run the image on a server with a real domain.
3. Try a real AI key under Settings → Test connection.

## Stack

- **Backend:** Go 1.24, standard library only, no third-party modules.
- **Frontend:** plain HTML, CSS and JavaScript in `web/`, embedded with `go:embed`. No framework, no build step.
- **Runtime:**
  - The manager starts `caddy run` as a child process and reloads it with `caddy reload`.
  - Caddy's admin API listens only on `localhost:2019`.
  - The Docker image is the official `caddy:2` with Caddy rebuilt to include `caddy-dns/cloudflare`.

## File map

| File | What it does |
| --- | --- |
| `main.go` | HTTP server, auth, all `/api/*` handlers: preview/apply, backups, status, stats, logs, settings, cert upload |
| `state.go` | Data model (`Host`, `AccessList`, `State`, `Settings`), load/save, **input validation**, password-hash merging |
| `caddyfile.go` | Generates the Caddyfile from state, plus per-host and per-list snippets for the live preview |
| `caddy.go` | Runs the Caddy child process; validate, reload, `hash-password`; captures Caddy's log into an event list |
| `monitor.go` | Upstream TCP health checks, certificate scan of Caddy's storage, incremental access-log reader |
| `ai.go` | Provider calls (Anthropic Messages API, OpenAI-compatible chat completions), redaction |
| `aihandler.go` | `/api/ai`: builds context for `logs`, `explain` and `ask`; turns "ask" replies into a host proposal |
| `web/app.js` | The whole UI: hash router, pages, review modal, diff, AI panels |
| `web/app.css`, `web/index.html` | Styles and shell |
| `importer.go` | Caddyfile parser and converter for imports |
| `gen_test.go`, `import_test.go` | Generator and import output validated by real `caddy validate`, injection rejection, redaction |
| `run.sh` | Local run without Docker (`./run.sh`, `./run.sh test`) |
| `Dockerfile`, `docker-compose.yml`, `.github/workflows/docker.yml` | Image, compose file, CI (test → build to GHCR → optional SSH deploy) |
| `docs/screenshots/` | README screenshots (demo data) |

## How a change flows

1. The UI sends the full new host list and/or access lists to `POST /api/preview`.
2. The server works out the new config:
   - It keeps existing password hashes for users sent without a new password (`mergeHashes`).
   - It **validates** everything (`validateState`).
   - It hashes new passwords with `caddy hash-password` (bcrypt, cost 12).
   - It generates the Caddyfile.
3. The server checks it:
   - It runs `caddy validate` on a temporary copy.
   - It probes upstreams and DNS, but **only for hosts that changed**.
   - It returns the checks, the old and new Caddyfile, and a one-time token (stored in memory for 30 minutes).
4. The UI shows the checks and a line diff. **Apply** sends the token to `POST /api/apply`.
5. Apply happens under a lock:
   - It backs up the current `state.json` and Caddyfile to `data/backups/<timestamp>/` and keeps the last 30.
   - It writes the new Caddyfile and runs `caddy reload`.
   - **If the reload fails, it puts back the previous file and reloads again**, so the old config keeps running.
   - State is only saved after a successful reload.

Restoring a backup goes through the same preview → apply path.

**Import (`importer.go`):** `POST /api/import` analyzes an uploaded file; preview with `import: {caddyfile, rewriteLocalhost}` applies it.
- A small Caddyfile lexer and parser builds a tree. Snippets are expanded. Heredocs and single-site files without braces are refused (use run as-is).
- A site becomes a managed host only if **every** directive maps faithfully (`convertSite`); otherwise the whole site is kept
  verbatim (with comments) in `State.ExtraConfig`. This is deliberate: a partial conversion could silently drop auth or routing.
  `header`/`request_header`/`request_body` variants that don't map are kept per host as custom directives.
- Global options: `email` goes to Settings if none is set, `admin` is dropped, everything else goes to `State.GlobalExtra`.
- Imported hosts are added; existing hosts with the same domain are replaced. `ExtraConfig`/`GlobalExtra` are written right after the
  global block and are editable in Settings.

**Manual mode (run as-is):** an uploaded Caddyfile is stored as `ManualCaddyfile` and used verbatim instead of the generated one.
- Hosts are kept but not applied.
- "Switch back to managed hosts" previews with `managed: true`.
- Uploaded files are **not** parsed into hosts. That's deliberate, to keep the app simple.

## Generated Caddyfile: things worth knowing

- **One `route { }` block per proxy host** holds exploit blocking, access rules, maintenance, encode, headers and the
  `handle`/`handle_path` blocks. `route` keeps directives in written order, so IP and login checks always run before proxying.
  Caddy's default directive order would not guarantee that.
- **Access lists** become snippets (`(access_<id>)`) and hosts `import` them. Only lists that are in use are written.
- **Access modes:**
  - "Either" means an allowed IP skips the login: `basic_auth @needauth` with `not remote_ip`.
  - "Both" means blocked IPs get a 403 before the login.
- **Paths:** routes match `<path>/*` (e.g. `/api/*`). A bare `/api` with no trailing slash falls through to the main upstream.
- **Access log:** every site logs JSON to `data/logs/access.log` (rolls at 50 MiB, keeps 3). Stats and logs read only the current file.
- **HTTPS:** with "Force HTTPS" off, the host gets both `http://` and `https://` addresses, so there's no redirect.
- **DNS challenge:** uses `dns cloudflare {env.CF_API_TOKEN}`. The token is passed as an environment variable to the Caddy process
  (changing it restarts Caddy), so it never appears in the Caddyfile or downloads.

## Security decisions

- **Login:**
  - There's one password, from `ADMIN_PASSWORD` (8+ characters; the app refuses to start otherwise). It's compared in constant time.
  - Failed logins are slowed and serialized (1 s).
- **Sessions:**
  - The session is an HMAC-signed cookie (`HttpOnly`, `SameSite=Strict`, 30 days).
  - The key is `data/.secret` plus the password, so **changing the password signs everyone out**.
- **CSRF:** every non-GET API call must send `X-Requested-With: caddy-manager`, and the cookie is `SameSite=Strict`.
- **Caddyfile injection:** all user input that reaches the Caddyfile is validated with strict regexes in `state.go`
  (domains, upstreams, paths, header names, CIDRs, usernames, URLs). Free text values are quoted and refused if they
  contain quotes or newlines. The only raw input is the per-host "Custom Caddy directives" box, by design, and it's still validated by Caddy.
- **Secrets stay put:**
  - Password hashes are stripped before state is sent to the browser.
  - The AI API key and DNS token are never returned; the UI only gets `hasKey` / `hasDnsToken`.
  - The DNS token is kept out of the Caddyfile.
- **AI redaction:** bcrypt hashes are always removed before anything is sent. IPv4 masking is optional (`maskIps`) and leaves loopback alone.
- **Files and IDs:** files in `data/` are written `0600`. Backup IDs and host IDs are format-checked to block path traversal.
- **Admin UI over the network:** it's plain HTTP on 8080. For HTTPS, proxy it through Caddy itself (`admin.example.com → localhost:8080`)
  behind an access list. Compose comments show how to bind it to localhost only.

## Data (`DATA_DIR`, `/data` in Docker)

| File | Contents |
| --- | --- |
| `state.json` | Hosts and access lists. Password hashes only, never plaintext |
| `settings.json` | ACME email, Cloudflare token, AI settings (incl. API key) |
| `Caddyfile` | The active file |
| `.secret` | Session signing key |
| `logs/access.log` | Caddy access log |
| `logs/events.log` | Caddy events and the manager's own notes (last 1000 shown) |
| `backups/` | Last 30 snapshots |
| `certs/<hostid>.crt/.key` | Uploaded certificates |
| `caddy/` | Caddy's own storage: certificates, ACME accounts, local CA. Follows `XDG_DATA_HOME` |

## AI assistant

- **Providers:**
  - `anthropic` posts to `/v1/messages`.
  - `openai`, `compat` and `ollama` all post to OpenAI-style `/chat/completions`.
  - Ollama's default base URL is `http://host.docker.internal:11434/v1`. The model is free text.
- **Modes:**
  - `logs`: a stats summary, top IPs and paths, up to 80 error lines and 60 other lines, and Caddy events, for a host or all hosts.
  - `explain`: the error text, the host's Caddyfile block, live DNS and upstream checks, and recent events.
  - `ask`: a one-line summary of every host. The model may answer with a ```` ```json {"domain","changes"} ``` ```` block.
    The server merges only allowed fields into a copy of the host (`buildProposal`). The UI opens it in the editor, and
    applying still needs Review. The AI can't set passwords and can't apply anything.
- **Not implemented:** request limits, budgets and a log of AI requests. They were in the mockups and dropped to keep it simple.

## Testing

- **Unit tests:** `CADDY_BIN=$(which caddy) go test ./...`. Without `CADDY_BIN`, the real-Caddy test is skipped.
- **During development, also checked by hand:**
  - Real requests through Caddy for every host type and option.
  - The full browser walkthrough with Playwright (Node) at 1366 px and 390 px wide.
  - The test helpers (fake upstreams, mock AI) were throwaway and aren't in the repo.
  - Worth adding a small Go integration test later, e.g. start the binary against a temp dir and drive `/api/*`.

## Known limitations

- **Stats and logs** read only the current `access.log` (up to 50 MiB, 300k entries in memory). Rolled files are ignored,
  so "7 days" may be partial on busy sites.
- **Health checks** are TCP connects every 30 s, not HTTP requests.
- **Certificate "expiring"** means less than 20 % of the certificate's lifetime is left. Caddy renews at about 33 %, so this signals that renewal is behind.
- **DNS challenge** supports Cloudflare only. Other providers need another plugin in the Dockerfile and a matching option in the UI.
- **No TCP/UDP streams:** they would need the `caddy-l4` plugin.
- **IPv6** works in rules and upstreams, but AI masking only covers IPv4.
- **Single user only.**

## Backlog

- AI that reacts to live behaviour and proposes changes (opt-in, with review or auto-revert); see the README.
- Read rolled access logs for longer ranges.
- More DNS providers.
- HTTP health checks (status code and path).
- A Go integration test suite.

## Conventions

- Keep the backend on the standard library.
- Keep the frontend build-free. `app.js` uses event delegation via `data-a` (actions) and `data-f`/`data-lf`/`data-sf` (field bindings).
- Every config change goes through preview → apply. Don't add endpoints that write the Caddyfile any other way.
- Validate anything that reaches the Caddyfile in `state.go`.
