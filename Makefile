.PHONY: dev dev-api dev-web build build-web build-api clean migrate test

# Development - run both backend and frontend
dev:
	@echo "Starting Jerboa development servers..."
	@make -j2 dev-api dev-web

dev-api:
	go run -tags dev ./cmd/jerboa

dev-web:
	cd web && npm run dev

# Production build
build: build-web build-api

build-web:
	cd web && npm ci && npm run build

build-api: build-web
	CGO_ENABLED=0 go build -o bin/jerboa ./cmd/jerboa

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
