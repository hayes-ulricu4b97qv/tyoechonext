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
RUN groupadd --system app && useradd --system --gid app --home-dir /app app
WORKDIR /app
COPY --from=api-build /out/typecho-next /app/typecho-next
COPY --from=web-build /src/web/dist /app/web/dist
RUN mkdir -p /app/data && chown -R app:app /app
USER app
ENV APP_PORT=8080 APP_WEB_DIR=/app/web/dist APP_DATA_DIR=/app/data
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/typecho-next"]
