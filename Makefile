.PHONY: dev dev-api dev-web build build-web build-api build-sync clean migrate test

# Development - run both backend and frontend
dev:
	@echo "Starting Jerboa development servers..."
	@make -j2 dev-api dev-web

dev-api:
	go run -tags dev ./cmd/jerboa

dev-web:
	cd web && npm run dev

# Production build
build: build-web build-api build-sync

build-web:
	cd web && npm ci && npm run build

build-api: build-web
	CGO_ENABLED=0 go build -o bin/jerboa ./cmd/jerboa

build-sync:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -o bin/jerboa-sync-linux-amd64   ./cmd/jerboa-sync
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/jerboa-sync-windows-amd64.exe ./cmd/jerboa-sync
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o bin/jerboa-sync-darwin-arm64   ./cmd/jerboa-sync
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o bin/jerboa-sync-darwin-amd64   ./cmd/jerboa-sync
	@echo "sync binaries built"

# Database
migrate:
	go run ./cmd/jerboa migrate

# Test
test:
	go test ./...

# Clean
clean:
	rm -rf bin/ web/dist/ web/node_modules/

# Docker
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f
