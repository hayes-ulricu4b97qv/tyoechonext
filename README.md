# Typecho Next

Typecho Next is the new runtime for this repository: a Go API with a small Vue 3 + TypeScript client. It deliberately starts a new architecture instead of keeping the old Typecho request model or plugin compatibility layer.

## Product entry points

- `/setting` — first deployment setup and site settings
- `/admin` — administrator login, dashboard and writing
- `/user` — public reader experience
- `/api/v1` — versioned JSON API

The service redirects every page to `/setting` until the first administrator and site settings are created. Runtime data is stored in SQLite under `APP_DATA_DIR/app.db`; an existing `app.json` from the first migration step is imported automatically on first startup.

## Quick start

```bash
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

Open <http://localhost:8080/setting> and complete the setup form. For development, run Vite with `cd web && npm run dev`; `/api` is proxied to the Go service.

## Container

```bash
docker build -t typecho-next .
docker run --rm -p 8080:8080 -v typecho-data:/app/data typecho-next
```

在受限网络的构建环境中，把环境提供的代理 CA 作为 BuildKit secret 传入：

```bash
docker build --secret id=proxy_ca,src="$CODEX_PROXY_CERT" -t typecho-next .
```

See [docs/architecture.md](docs/architecture.md) for the module boundaries and the migration order. The original PHP implementation remains in the repository temporarily as a migration reference and is not used by the new server.

## Runtime requirements

* Go 1.22 or newer
* Node.js 20 or newer for the Vue build
* GCC or another C toolchain for local SQLite builds (the Docker image includes it)
* A writable `APP_DATA_DIR` (the container uses `/app/data`)

SQLite is the default single-node persistence adapter. The repository interface keeps the API boundary ready for a PostgreSQL adapter when multi-node deployment is needed.

Article listing accepts `q`, `page`, and `pageSize`. Sessions are persisted in SQLite with only a SHA-256 token hash stored server side.

## Contributing

Please see [CONTRIBUTING](CONTRIBUTING.md) for details.
