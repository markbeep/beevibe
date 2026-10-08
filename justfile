# beevibe — task runner.  Run `just` (or `just --list`) to see everything.
#
# Assumes the layout described in plans/1-techstack.md §2.1:
#   cmd/beevibe/            backend entry point
#   cmd/beevibe-renderer/   headless-Chromium preview renderer entry point
#   internal/db/migrations/ goose migrations   (00001_init.sql, …)
#   internal/db/queries/    sqlc query files   (*.sql)
#   internal/db/gen/        sqlc output (committed)
#   web/                    SolidJS + Vite frontend
#
# The database lives at $DATA_DIR/beevibe.db (DATA_DIR defaults to ./data).
#
# whisper.cpp is vendored into third_party/ and built as static libraries by
# `just whisper-lib`; the Go binding is wired in through a `replace` directive,
# so EVERY Go recipe depends on whisper-lib (cgo needs the headers and libs even
# to typecheck).

set shell := ["bash", "-uc"]

DATA_DIR := env("DATA_DIR", "data")
DB_PATH := DATA_DIR / "beevibe.db"
MIGRATIONS := "internal/db/migrations"
BACKEND := "cmd/beevibe"
RENDERER := "cmd/beevibe-renderer"
WEB := "web"

WHISPER_DIR := "third_party/whisper.cpp"
WHISPER_BUILD := "third_party/whisper.cpp/build_go"
WHISPER_MODEL_PATH := env("WHISPER_MODEL", "models/ggml-base.en.bin")
WHISPER_MODEL_URL := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"

# cgo needs the whisper/ggml headers and the static libraries built by `whisper-lib`.
export C_INCLUDE_PATH := justfile_directory() + "/third_party/whisper.cpp/include:" + justfile_directory() + "/third_party/whisper.cpp/ggml/include"
export LIBRARY_PATH := justfile_directory() + "/third_party/whisper.cpp/build_go/src:" + justfile_directory() + "/third_party/whisper.cpp/build_go/ggml/src"

default:
    @just --list

# --- whisper.cpp (vendored) --------------------------------------------------

# clone + cmake-build the whisper.cpp static libraries (idempotent)
whisper-lib:
    test -d {{ WHISPER_DIR }} || git clone --depth 1 https://github.com/ggml-org/whisper.cpp {{ WHISPER_DIR }}
    cmake -S {{ WHISPER_DIR }} -B {{ WHISPER_BUILD }} -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=OFF -DWHISPER_BUILD_SERVER=OFF
    cmake --build {{ WHISPER_BUILD }} --target whisper -- -j$(nproc)

# download the base.en whisper model used for speech-to-text
whisper-model:
    mkdir -p $(dirname {{ WHISPER_MODEL_PATH }})
    curl -fL -o {{ WHISPER_MODEL_PATH }} {{ WHISPER_MODEL_URL }}

# --- bootstrap ---------------------------------------------------------------

# install dependencies, generate queries, apply migrations
setup: deps generate migrate-up

# fetch Go modules and install frontend dependencies
deps: whisper-lib
    go mod download
    cd {{ WEB }} && npm ci

# --- generated code (sqlc) ---------------------------------------------------

# regenerate typed Go from the queries against the migrations schema
generate: whisper-lib
    sqlc generate

# check the queries compile against the schema, without writing files
generate-check: whisper-lib
    sqlc compile
    sqlc vet

# --- migrations (goose) ------------------------------------------------------

# apply ALL pending migrations
migrate-up: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} up

# apply only the next single migration
migrate-up-one: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} up-by-one

# apply migrations up to and including VERSION:  just migrate-up-to 3
migrate-up-to version: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} up-to {{ version }}

# roll back the most recent migration
migrate-down: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} down

# roll back the most recent migration, then apply it again
migrate-redo: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} redo

# roll back migrations down to and excluding VERSION:  just migrate-down-to 3
migrate-down-to version: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} down-to {{ version }}

# roll back ALL migrations (down to version 0)
migrate-down-all: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} down-to 0

# roll back ALL migrations, then re-apply every migration from scratch
migrate-reset: migrate-down-all migrate-up

# show the status of every migration (applied / pending)
migrate-status: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} status

# print the current schema version
migrate-version: _dbdir
    goose -dir {{ MIGRATIONS }} sqlite3 {{ DB_PATH }} version

# create a new sequential migration:  just migrate-create add_room_state
migrate-create name:
    goose -dir {{ MIGRATIONS }} create -s {{ name }} sql

# renumber migration files sequentially (repairs gaps/ordering after merges)
migrate-fix:
    goose -dir {{ MIGRATIONS }} fix

# (private) make sure the data directory exists
[private]
_dbdir:
    @mkdir -p {{ DATA_DIR }}

# --- backend -----------------------------------------------------------------

# build the backend and the renderer to bin/
build: whisper-lib web-build
    mkdir -p bin
    CGO_ENABLED=1 go build -o bin/beevibe ./{{ BACKEND }}
    CGO_ENABLED=1 go build -o bin/beevibe-renderer ./{{ RENDERER }}

# build the preview renderer only
renderer-build:
    mkdir -p bin
    CGO_ENABLED=0 go build -o bin/beevibe-renderer ./{{ RENDERER }}

# run the backend (expects dbs migrated; use `just migrate-up` first)
run: whisper-lib
    CGO_ENABLED=1 go run ./{{ BACKEND }}

# run the preview renderer
renderer-run:
    CGO_ENABLED=0 go run ./{{ RENDERER }}

test: whisper-lib
    CGO_ENABLED=1 go test ./...

fmt:
    gofmt -l -w .

vet: whisper-lib
    CGO_ENABLED=1 go vet ./...

# --- frontend ----------------------------------------------------------------

web-install:
    cd {{ WEB }} && npm ci

web-dev:
    cd {{ WEB }} && npm run dev

web-build:
    cd {{ WEB }} && npm run build

web-typecheck:
    cd {{ WEB }} && npm run typecheck

# --- composed ----------------------------------------------------------------

# backend and Vite together; Ctrl-C stops both
dev: migrate-up whisper-lib
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'kill 0' EXIT
    ( cd {{ WEB }} && npm run dev ) &
    go run ./{{ BACKEND }}

# everything CI should run
check: whisper-lib generate-check vet test web-build

# remove build artifacts
clean:
    rm -rf bin {{ WEB }}/dist
