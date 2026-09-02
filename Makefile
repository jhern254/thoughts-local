.PHONY: help fmt fmt-check vet test test-fresh test-integration test-race test-cover build quick check ci clean hooks run dev migrate/new migrate/up migrate/down migrate/version

help:
	@echo "Available commands:"
	@echo "  make fmt          Format Go files"
	@echo "  make fmt-check    Check formatting without changing files"
	@echo "  make vet          Run go vet"
	@echo "  make test         Run tests"
	@echo "  make test-fresh   Run tests without cached results"
	@echo "  make test-integration Run integration tests"
	@echo "  make test-race    Run tests with the race detector"
	@echo "  make test-cover   Generate a coverage report"
	@echo "  make build        Build all packages"
	@echo "  make quick        Fast checks before committing"
	@echo "  make check        Checks before pushing"
	@echo "  make ci           Complete CI verification"
	@echo "  make hooks        Enable repository Git hooks"
	@echo "  make run          Start the API without applying migrations"
	@echo "  make dev          Apply local migrations, then start the API"
	@echo "  make migrate/new  Create a migration (name=<description>)"
	@echo "  make migrate/up   Apply all pending migrations"
	@echo "  make migrate/down Roll back one migration"
	@echo "  make migrate/version Show the current migration version"
	@echo "  make clean        Remove generated test files"

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"; \
	if [ -n "$$files" ]; then \
		echo "The following files need gofmt:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test ./...

test-fresh:
	go test -count=1 ./...

test-integration:
	go test -tags=integration -count=1 ./integration/...

test-race:
	go test -race -count=1 ./...

test-cover:
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

build:
	go build ./...

quick: fmt-check vet test

check: fmt-check vet test-fresh test-integration build

ci: check test-race

hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks enabled from .githooks/"

clean:
	rm -f coverage.out

DB_PATH ?= ./data/thoughts.db
MIGRATE_DSN ?= sqlite://$(DB_PATH)
APP_DSN ?= file:$(DB_PATH)
DB_DIR := $(dir $(DB_PATH))

migrate/new:
	@test -n "$(name)" || { echo "usage: make migrate/new name=create_subjects"; exit 1; }
	migrate create -seq -ext=.sql -dir=./migrations $(name)

migrate/up:
	@mkdir -p "$(DB_DIR)"
	migrate -path=./migrations -database="$(MIGRATE_DSN)" up

migrate/down:
	migrate -path=./migrations -database="$(MIGRATE_DSN)" down 1

migrate/version:
	migrate -path=./migrations -database="$(MIGRATE_DSN)" version

run:
	go run ./cmd/api -db-dsn "$(APP_DSN)"

dev: migrate/up
	go run ./cmd/api -db-dsn "$(APP_DSN)"
