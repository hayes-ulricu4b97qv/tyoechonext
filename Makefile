.PHONY: dev web-build api-run build clean

dev:
	cd web && npm run dev

web-build:
	cd web && npm ci && npm run build

api-run:
	go run ./cmd/server

build: web-build
	go build ./cmd/server

clean:
	rm -rf web/dist data
