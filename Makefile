.PHONY: build run test lint docker-up docker-down docker-logs docker-rebuild clean

# ── Local build ───────────────────────────────────────────────────────────────
build:
	@echo "🔨 Building NexusGuard..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -ldflags="-s -w" -o bin/nexusguard_bot ./cmd/bot
	@echo "✅ Binary at bin/nexusguard_bot"

run: build
	@echo "🚀 Running NexusGuard..."
	bin/nexusguard_bot

# ── Testing ───────────────────────────────────────────────────────────────────
test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

# ── Docker ────────────────────────────────────────────────────────────────────
docker-up:
	@echo "🐳 Starting Docker stack..."
	docker compose up -d --build
	@echo "✅ Stack is up! Logs: make docker-logs"

docker-down:
	docker compose down

docker-rebuild:
	docker compose down
	docker compose up -d --build --force-recreate

docker-logs:
	docker compose logs -f bot

docker-db-logs:
	docker compose logs -f postgres

# ── Cleanup ───────────────────────────────────────────────────────────────────
clean:
	rm -rf bin/
	docker compose down -v --remove-orphans
