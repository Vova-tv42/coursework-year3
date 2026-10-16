-include .env
POSTGRES_HOST_PORT ?= 5432
DEV_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_HOST_PORT)/$(POSTGRES_DB)?sslmode=disable

.PHONY: dev test lint up down

dev:
	docker compose up -d postgres
	trap 'kill 0' EXIT; (cd web && bun run dev) & DATABASE_URL="$(DEV_DATABASE_URL)" go run ./cmd/server

test:
	go test ./...

lint:
	golangci-lint run ./...
	cd web && bun run lint

up:
	docker compose up --build -d

down:
	docker compose down
