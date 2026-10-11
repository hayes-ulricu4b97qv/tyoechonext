# syntax=docker/dockerfile:1
# Build the Vue shell first, then ship a small Go runtime image.
FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/proxy_ca; fi; \
    npm ci --strict-ssl=true
COPY web/ ./
RUN npm run build

FROM golang:1.23-bookworm AS api-build
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then \
      cat /etc/ssl/certs/ca-certificates.crt /run/secrets/proxy_ca > /tmp/combined-ca.pem; \
      SSL_CERT_FILE=/tmp/combined-ca.pem go mod download; \
    else \
      go mod download; \
    fi
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/typecho-next ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends wget \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system app \
    && useradd --system --gid app --home-dir /app app
WORKDIR /app
COPY --from=api-build /out/typecho-next /app/typecho-next
COPY --from=web-build /src/web/dist /app/web/dist
RUN mkdir -p /app/data && chown -R app:app /app
USER app
ENV APP_PORT=8080 APP_WEB_DIR=/app/web/dist APP_DATA_DIR=/app/data APP_MEDIA_DIR=/app/data/uploads
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD ["/bin/sh", "-c", "wget -q -O - http://127.0.0.1:8080/healthz >/dev/null || exit 1"]
ENTRYPOINT ["/app/typecho-next"]
