.PHONY: help fmt fmt-check vet test test-fresh test-race test-cover build quick check ci clean hooks

help:
	@echo "Available commands:"
	@echo "  make fmt          Format Go files"
	@echo "  make fmt-check    Check formatting without changing files"
	@echo "  make vet          Run go vet"
	@echo "  make test         Run tests"
	@echo "  make test-fresh   Run tests without cached results"
	@echo "  make test-race    Run tests with the race detector"
	@echo "  make test-cover   Generate a coverage report"
	@echo "  make build        Build all packages"
	@echo "  make quick        Fast checks before committing"
	@echo "  make check        Checks before pushing"
	@echo "  make ci           Complete CI verification"
	@echo "  make hooks        Enable repository Git hooks"
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

test-race:
	go test -race -count=1 ./...

test-cover:
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

build:
	go build ./...

quick: fmt-check vet test

check: fmt-check vet test-fresh build

ci: check test-race

hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks enabled from .githooks/"

clean:
	rm -f coverage.out

DB_DSN ?= sqlite://./thoughts.db

.PHONY: migrate/new
migrate/new:
	@echo "usage: make migrate/new name=create_subjects"
	migrate create -seq -ext=.sql -dir=./migrations $(name)

.PHONY: migrate/up
migrate/up:
	migrate -path=./migrations -database="$(DB_DSN)" up

.PHONY: migrate/down
migrate/down:
	migrate -path=./migrations -database="$(DB_DSN)" down 1

.PHONY: migrate/version
migrate/version:
	migrate -path=./migrations -database="$(DB_DSN)" version

