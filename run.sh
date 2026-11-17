#!/usr/bin/env sh
# Run caddy-manager locally without Docker.
#
#   ./run.sh                 build and run (reads ADMIN_PASSWORD from .env or the environment)
#   ./run.sh test            run the Go tests
#
# Needs: Go 1.24+ and the caddy binary (https://caddyserver.com/docs/install).
# Caddy listens on ports 80 and 443, so run as root or give caddy the capability once:
#   sudo setcap cap_net_bind_service=+ep "$(command -v caddy)"
set -eu
cd "$(dirname "$0")"

# settings from .env, without overriding what's already exported
if [ -f .env ]; then
  while IFS='=' read -r key value; do
    case "$key" in ''|\#*|*[!A-Za-z0-9_]*) continue ;; esac
    value=${value%\"}; value=${value#\"}; value=${value%\'}; value=${value#\'}
    eval "current=\${$key:-}"
    [ -z "$current" ] && export "$key=$value"
  done < .env
fi

CADDY_BIN="${CADDY_BIN:-$(command -v caddy || true)}"
if [ -z "$CADDY_BIN" ] || [ ! -x "$CADDY_BIN" ]; then
  echo "caddy not found. Install it, or set CADDY_BIN=/path/to/caddy" >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "Go is not installed (https://go.dev/dl/)" >&2
  exit 1
fi

if [ "${1:-}" = "test" ]; then
  CADDY_BIN="$CADDY_BIN" exec go test ./...
fi

if [ -z "${ADMIN_PASSWORD:-}" ]; then
  echo "Set ADMIN_PASSWORD (at least 8 characters) in .env or the environment." >&2
  echo "  cp .env.example .env   and edit it" >&2
  exit 1
fi

DATA_DIR="${DATA_DIR:-$PWD/data}"
mkdir -p "$DATA_DIR" "$PWD/config" bin
echo "Building…"
CGO_ENABLED=0 go build -trimpath -o bin/caddy-manager .

export ADMIN_PASSWORD CADDY_BIN DATA_DIR
export XDG_DATA_HOME="${XDG_DATA_HOME:-$DATA_DIR}"      # Caddy keeps certificates in $XDG_DATA_HOME/caddy
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$PWD/config}"
export LISTEN="${LISTEN:-127.0.0.1:8080}"

echo "Admin UI: http://$LISTEN   (data in $DATA_DIR)"
exec ./bin/caddy-manager
