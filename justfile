# beevibe — task runner.  Run `just` (or `just --list`) to see everything.
#
# Assumes the layout described in plans/1-techstack.md §2.1:
#   cmd/beevibe/            backend entry point
#   internal/db/migrations/ goose migrations   (00001_init.sql, …)
#   internal/db/queries/    sqlc query files   (*.sql)
#   internal/db/gen/        sqlc output (committed)
#   web/                    SolidJS + Vite frontend
#
# The database lives at $DATA_DIR/beevibe.db (DATA_DIR defaults to ./data).

set shell := ["bash", "-uc"]

DATA_DIR   := env_var_or_default("DATA_DIR", "data")
DB_PATH    := DATA_DIR / "beevibe.db"
MIGRATIONS := "internal/db/migrations"
QUERIES    := "internal/db/queries"
GENERATED  := "internal/db/gen"
BACKEND    := "cmd/beevibe"
WEB        := "web"

default:
    @just --list

# --- bootstrap ---------------------------------------------------------------

# install dependencies, generate queries, apply migrations
setup: deps generate migrate-up

# fetch Go modules and install frontend dependencies
deps:
    go mod download
    cd {{WEB}} && npm ci

# --- generated code (sqlc) ---------------------------------------------------

# regenerate typed Go from the queries against the migrations schema
generate:
    sqlc generate

# check the queries compile against the schema, without writing files
generate-check:
    sqlc compile
    sqlc vet

# --- migrations (goose) ------------------------------------------------------

# apply ALL pending migrations
migrate-up: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} up

# apply only the next single migration
migrate-up-one: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} up-by-one

# apply migrations up to and including VERSION:  just migrate-up-to 3
migrate-up-to version: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} up-to {{version}}

# roll back the most recent migration
migrate-down: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} down

# roll back the most recent migration, then apply it again
migrate-redo: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} redo

# roll back migrations down to and excluding VERSION:  just migrate-down-to 3
migrate-down-to version: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} down-to {{version}}

# roll back ALL migrations (down to version 0)
migrate-down-all: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} down-to 0

# roll back ALL migrations, then re-apply every migration from scratch
migrate-reset: migrate-down-all migrate-up

# show the status of every migration (applied / pending)
migrate-status: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} status

# print the current schema version
migrate-version: _dbdir
    goose -dir {{MIGRATIONS}} sqlite3 {{DB_PATH}} version

# create a new sequential migration:  just migrate-create add_room_state
migrate-create name:
    goose -dir {{MIGRATIONS}} create -s {{name}} sql

# renumber migration files sequentially (repairs gaps/ordering after merges)
migrate-fix:
    goose -dir {{MIGRATIONS}} fix

# (private) make sure the data directory exists
[private]
_dbdir:
    @mkdir -p {{DATA_DIR}}

# --- backend -----------------------------------------------------------------

# compile the backend to bin/beevibe
build:
    mkdir -p bin
    go build -o bin/beevibe ./{{BACKEND}}

# run the backend (expects dbs migrated; use `just migrate-up` first)
run:
    go run ./{{BACKEND}}

test:
    go test ./...

fmt:
    gofmt -l -w .

vet:
    go vet ./...

# --- frontend ----------------------------------------------------------------

web-install:
    cd {{WEB}} && npm ci

web-dev:
    cd {{WEB}} && npm run dev

web-build:
    cd {{WEB}} && npm run build

web-test:
    cd {{WEB}} && npm test

# --- composed ----------------------------------------------------------------

# backend and Vite together; Ctrl-C stops both
dev: migrate-up
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'kill 0' EXIT
    ( cd {{WEB}} && npm run dev ) &
    go run ./{{BACKEND}}

# everything CI should run
check: generate-check vet test web-build

# remove build artifacts
clean:
    rm -rf bin {{WEB}}/dist
