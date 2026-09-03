.PHONY: build run test docker-up docker-down

build:
	go build -o bin/nexusguard_bot ./cmd/bot

run: build
	./bin/nexusguard_bot

test:
	go test ./...

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
