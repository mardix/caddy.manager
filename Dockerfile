# syntax=docker/dockerfile:1

# 1. Caddy with the Cloudflare DNS plugin (needed for wildcard certificates)
FROM caddy:2-builder AS caddy
RUN xcaddy build --with github.com/caddy-dns/cloudflare

# 2. The manager: one static Go binary with the web UI embedded
FROM golang:1.24-alpine AS app
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/caddy-manager .

# 3. Runtime: the official Caddy image (CA certs, /data and /config volumes)
FROM caddy:2
COPY --from=caddy /usr/bin/caddy /usr/bin/caddy
COPY --from=app /out/caddy-manager /usr/bin/caddy-manager
ENV DATA_DIR=/data \
    LISTEN=:8080
EXPOSE 80 443 443/udp 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1
ENTRYPOINT ["/usr/bin/caddy-manager"]
CMD []
