# Contributing

This repository is a Go API (`cmd/server`, `internal/`) and a Vue client (`web/`).

## Reporting bugs

Describe what you did, the request you sent, and the result you expected. A failing `go test` case or a short HTTP transcript is the most useful report.

## Code

* Read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before sending a change.
* Read [docs/architecture.md](docs/architecture.md) for the module boundaries and the public routes.
* Go changes belong under `cmd/` and `internal/`. Run `go test ./...` from the repository root.
* Vue changes belong under `web/`. From that directory, run `npm run build`.
* Keep the public routes stable: `/setting`, `/admin`, `/user`, `/user/{slug}`, `/page/{slug}`, `/api/v1`, `/feed.xml`, `/healthz`, and `/readyz`.

## Local run

```bash
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

Open `http://localhost:8080/setting` for the first setup. For UI work, `cd web && npm run dev` proxies `/api` to the Go service.
