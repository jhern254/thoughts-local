.PHONY: help fmt fmt-check vet test test-fresh test-integration test-race test-cover build quick check ci clean hooks run dev dev/seed tui tui/demo tui/demo/seeded tui/build migrate/build migrate/new migrate/up migrate/down migrate/version
.PHONY: tui/browser/demo/spider-man
.PHONY: tui/demo/stress
.PHONY: tui/browser tui/browser/demo tui/browser/demo/seeded tui/browser/demo/stress
.PHONY: speech/setup speech/ensure tui/package

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
	@echo "  make check        Full fresh tests, integration, and build"
	@echo "  make ci           Complete CI verification"
	@echo "  make hooks        Enable repository Git hooks"
	@echo "  make run          Start the API without applying migrations"
	@echo "  make dev          Migrate, seed, then start the development API"
	@echo "  make dev/seed     Apply migrations and seed the development user"
	@echo "  make tui          Migrate, build, and start the persistent TUI"
	@echo "  make tui/demo     Start the TUI with a disposable migrated database"
	@echo "  make tui/demo/seeded Start a disposable TUI with sample events and thoughts"
	@echo "  make tui/demo/stress Start a disposable agent diary (48 goals, 32 live subjects, 721 events, 20,000 thoughts)"
	@echo "  make tui/build    Build the TUI binary"
	@echo "  make tui/browser Migrate, build, and serve the TUI in a local browser"
	@echo "  make speech/setup Prepare the pinned local speech engine (explicit download)"
	@echo "  make tui/package Build a browser app bundle with its speech engine"
	@echo "  make tui/browser/demo Serve a disposable seeded browser demo"
	@echo "  make tui/browser/demo/seeded Serve sample events and thoughts in the browser"
	@echo "  make tui/browser/demo/spider-man Start a persistent Peter Parker diary with browser voice"
	@echo "  make tui/browser/demo/stress Serve 48 goals, 32 live subjects, 721 events and 20,000 thoughts in the browser"
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

check: fmt-check vet test-fresh test-integration build migrate/build

ci: check test-race

hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks enabled from .githooks/"

clean:
	rm -f coverage.out "$(TUI_BIN)" "$(MIGRATE_BIN)"

DB_PATH ?= ./data/thoughts.db
SPIDER_DEMO_DB ?= $(HOME)/Documents/Thoughts/demos/spider-man/thoughts.db
MIGRATE_DSN ?= sqlite://$(DB_PATH)
APP_DSN ?= file:$(DB_PATH)
TUI_BIN ?= ./bin/thoughts-tui
TUI_ARGS ?=
DB_DIR := $(dir $(DB_PATH))
MIGRATE_BIN := ./bin/migrate

# Build the standard CLI with the same SQLite version as the application.
# A globally installed migrate may embed SQLite too old for this schema.
migrate/build: $(MIGRATE_BIN)

$(MIGRATE_BIN): tools/migrate/go.mod tools/migrate/go.sum
	@mkdir -p ./bin
	go -C tools/migrate build -mod=readonly -tags sqlite -o ../../bin/migrate github.com/golang-migrate/migrate/v4/cmd/migrate

migrate/new: $(MIGRATE_BIN)
	@test -n "$(name)" || { echo "usage: make migrate/new name=create_subjects"; exit 1; }
	"$(MIGRATE_BIN)" create -seq -ext=.sql -dir=./migrations $(name)

migrate/up: $(MIGRATE_BIN)
	@mkdir -p "$(DB_DIR)"
	"$(MIGRATE_BIN)" -path=./migrations -database="$(MIGRATE_DSN)" up

migrate/down: $(MIGRATE_BIN)
	"$(MIGRATE_BIN)" -path=./migrations -database="$(MIGRATE_DSN)" down 1

migrate/version: $(MIGRATE_BIN)
	"$(MIGRATE_BIN)" -path=./migrations -database="$(MIGRATE_DSN)" version

dev/seed: migrate/up
	sqlite3 "$(DB_PATH)" "INSERT INTO users (user_id) VALUES ('test-user') ON CONFLICT (user_id) DO NOTHING;"

run:
	go run ./cmd/api -db-dsn "$(APP_DSN)"

dev: dev/seed
	go run ./cmd/api -db-dsn "$(APP_DSN)"

tui/build:
	@mkdir -p "$(dir $(TUI_BIN))"
	go build -o "$(TUI_BIN)" ./cmd/thoughts-tui

tui: migrate/up tui/build
	"$(TUI_BIN)" --db-dsn "$(APP_DSN)"

speech/setup:
	bash ./scripts/moonshine-dev.sh setup

speech/ensure:
	bash ./scripts/moonshine-dev.sh ensure

tui/package:
	bash ./scripts/package-browser.sh

tui/browser: migrate/up speech/ensure
	@mkdir -p "$(dir $(TUI_BIN))"
	bash ./scripts/moonshine-dev.sh exec go build -o "$(TUI_BIN)" ./cmd/thoughts-tui
	bash ./scripts/moonshine-dev.sh exec "$(TUI_BIN)" --browser --db-dsn "$(APP_DSN)"

tui/browser/demo tui/browser/demo/seeded: TUI_ARGS = --browser
tui/browser/demo tui/browser/demo/seeded: tui/demo/seeded

tui/browser/demo/stress: TUI_ARGS = --browser
tui/browser/demo/stress: tui/demo/stress

tui/demo/seeded: DEMO_SEED = ./scripts/demo.sql
tui/demo/seeded: tui/demo

tui/demo/stress: DEMO_SEED = ./scripts/demo_stress.sql
tui/demo/stress: tui/demo

tui/demo: tui/build $(MIGRATE_BIN)
	@set -eu; \
	demo_dir="$$(mktemp -d)"; \
	cleanup() { trap - 0 1 2 15; rm -rf "$$demo_dir"; }; \
	trap cleanup 0 1 2 15; \
	demo_db="$$demo_dir/thoughts.db"; \
	echo "Starting disposable TUI with $$demo_db (deleted on exit)."; \
	"$(MIGRATE_BIN)" -path=./migrations -database="sqlite://$$demo_db" up; \
	if [ -n "$(DEMO_SEED)" ]; then sqlite3 -bail "$$demo_db" < "$(DEMO_SEED)"; fi; \
	"$(TUI_BIN)" --db-dsn "file:$$demo_db" $(TUI_ARGS)

# Unlike disposable demos, seed once and retain the diary outside the checkout.
tui/browser/demo/spider-man: $(MIGRATE_BIN) speech/ensure
	@set -eu; umask 077; \
	mkdir -p "$$(dirname -- "$(SPIDER_DEMO_DB)")" "$(dir $(TUI_BIN))"; \
	"$(MIGRATE_BIN)" -path=./migrations -database="sqlite://file:$(SPIDER_DEMO_DB)" up; \
	sqlite3 -bail "$(SPIDER_DEMO_DB)" < ./scripts/demo_spider_man.sql; \
	echo "Starting persistent Spider-Man diary: $(SPIDER_DEMO_DB)"; \
	echo "Existing diary entries are retained; seed content is added only to an empty database."; \
	bash ./scripts/moonshine-dev.sh exec go build -o "$(TUI_BIN)" ./cmd/thoughts-tui; \
	bash ./scripts/moonshine-dev.sh exec "$(TUI_BIN)" --browser --db-dsn "file:$(SPIDER_DEMO_DB)" $(TUI_ARGS)
