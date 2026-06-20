.PHONY: run run-worker build build-worker test test-integration lint migrate-up migrate-down keys docker-up docker-down

# ── Dev ──────────────────────────────────────────────────────────────────────

run:
	go run ./cmd/api

run-worker:
	WORKER_ENABLED=true go run ./cmd/worker

build:
	go build -ldflags="-s -w" -o bin/api ./cmd/api
	go build -ldflags="-s -w" -o bin/migrate ./cmd/migrate

build-worker:
	go build -ldflags="-s -w" -o bin/worker ./cmd/worker

test:
	go test ./... -count=1

test-integration:
	go test -tags=integration ./test/integration/... -count=1

test-cover:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

# ── Database ─────────────────────────────────────────────────────────────────

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

# ── Keys ─────────────────────────────────────────────────────────────────────

keys:
	@mkdir -p secrets
	openssl genrsa -out secrets/private.pem 4096
	openssl rsa -in secrets/private.pem -pubout -out secrets/public.pem
	@echo "Keys written to secrets/. Add secrets/ to .gitignore."

# ── Docker ───────────────────────────────────────────────────────────────────

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v
