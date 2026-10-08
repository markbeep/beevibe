# 1 — Tech Stack

> **Purpose:** the technologies that MUST be used, and the configuration surface.

## 1. Shape

One deployable unit plus one sidecar service:

- **Frontend** — SolidJS + Vite SPA, built to `dist/`, served by the Go backend.
- **Backend** — Go: HTTP + WebSocket, room/session/auth management, SQLite, static file serving,
  speech-to-text, LLM-agent orchestration.
- **Renderer** — a separate headless-Chromium service that captures admin-grid previews
  (see [2-architecture §6](2-architecture.md)).

## 2. Toolchain — pinned by `mise.toml`

The repo ships a `mise.toml`; these are the pinned tools. Nothing else is assumed present.

| Tool | Pinned | Used for |
|------|--------|----------|
| Go | 1.27.1 | backend (needs `os.Root`) |
| Node | 26.11.1 | frontend build (Vite) |
| goose | 3.28.0 | numbered SQL migrations |
| sqlc | 1.31.1 | typed Go from SQL queries |
| just | latest | task runner (`just build`, `just dev`, `just migrate-up`, …) |

### 2.1 Project layout

```
mise.toml                 pinned toolchain
go.mod                    module github.com/mark/beevibe
justfile                  task runner (recipes for sqlc, goose, build, dev)
sqlc.yaml                 sqlc config — schema: internal/db/migrations, queries: internal/db/queries
Dockerfile                multi-stage image (Node build → Go build → slim runtime)
docker-compose.yml        backend + renderer + volume, for local runs
cmd/beevibe/              backend entry point
internal/db/migrations/   goose migrations  (00001_init.sql, …)
internal/db/queries/      sqlc query files  (*.sql)
internal/db/gen/          sqlc output (package dbgen; committed so builds don't need sqlc)
web/                      SolidJS + Vite frontend
data/                     runtime: beevibe.db, rooms/, templates/  (gitignored)
```

### 2.2 Migrations & queries workflow

| Task | Command |
|------|---------|
| Apply all pending migrations | `just migrate-up` |
| **Roll back all** migrations | `just migrate-down-all` |
| Reset (down all, then up all) | `just migrate-reset` |
| Roll back one / re-run one | `just migrate-down` · `just migrate-redo` |
| Status / current version | `just migrate-status` · `just migrate-version` |
| New sequential migration | `just migrate-create add_room_state` |
| Regenerate typed Go | `just generate` |
| Validate queries against the schema | `just generate-check` |

Conventions (verified against goose 3.28.0 and sqlc 1.31.1):

- Filenames: `NNNNN_name.sql`, **underscore** separator, 5-digit padding — goose rejects a hyphen
  (`no filename separator '_' found`) and `goose create -s` emits 5-digit names.
- sqlc reads the goose migrations as its schema: `-- +goose Up` is parsed, `-- +goose Down` is
  ignored.
- Generated code is committed; CI runs only `sqlc compile` + `sqlc vet` (`just generate-check`).


### 2.3 Database, build and deploy

| Concern | Decision |
|---------|----------|
| SQLite DSN | `file:$DATA_DIR/beevibe.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` — **`foreign_keys(1)` is required**, otherwise the schema's `ON DELETE CASCADE` is silently ignored. |
| Connection pool | one writer at a time, concurrent readers (WAL); goose applies embedded migrations at startup from `//go:embed migrations/*.sql`. |
| Frontend assets | `web/dist` embedded with `//go:embed all:web/dist`; `just build` depends on `just web-build`. |
| SPA routing | any `GET` that is not `/api/*`, not `/ws/*`, not `/rooms/*` and not `/healthz`/`/readyz` serves `index.html` so client routes work on reload. |
| Health | `GET /healthz` (process up) and `GET /readyz` (DB ping + renderer reachable). |
| Shutdown | on SIGTERM: stop accepting, close WebSockets with a normal close frame, mark running agent runs `failed`, close the DB. |
| Deploy artifacts | `Dockerfile` (multi-stage: Node build → Go build with CGO → slim runtime with the whisper model) + `docker-compose.yml` (backend, renderer, data volume). |
| Tests | Go unit tests plus integration tests against a temporary SQLite database: migrations, sqlc queries, auth, room lifecycle and agent logic. **No frontend test tooling.** |

## 3. Frontend

| Concern | Choice | Notes |
|---------|--------|-------|
| Framework | **SolidJS** | pinned via mise/package.json |
| Build | **Vite** | dev server proxies the API |
| Router | **`@solidjs/router`** | routes: `/` · `/admin` · `/admin/rooms/{id}` · `/admin/rooms/{id}/live` · `/room/{roomId}` |
| CSS | **[Bulma](https://bulma.io/)** | |
| Icons | **[solid-icons](https://solid-icons.vercel.app/)** | |
| Mic capture | `AudioWorklet` → raw PCM/WAV, 16 kHz mono | device select, input gain, level meter (no VAD gate) |
| Share links | built **client-side** from `window.location.origin` (`/rooms/{roomId}/{userId}/`) | no base-URL env var |

## 4. Backend

| Concern | Choice | Notes |
|---------|--------|-------|
| Language | Go 1.27 | |
| HTTP router | `gorilla/mux` | |
| WebSocket | `gorilla/websocket` | |
| Logging | `uber-go/zap` | |
| LLM SDK | `github.com/zendev-sh/goai` | DeepSeek provider |
| Database | SQLite via `modernc.org/sqlite` | pure-Go driver (CGO is required only for whisper) |
| Migrations | **goose**, `00001_init.sql`… | `internal/db/migrations/`; see [schema.md](schema.md) |
| Queries | **sqlc** | generated from `queries/*.sql` |
| Static serving | `net/http` + `os.Root` confinement | correct MIME types, `.md` as `text/plain`, `nosniff` |
| STT | **whisper.cpp**, CGO binding | `base.en`, English only, single binary |
| Preview renderer | **separate headless-Chromium service** | reached over HTTP (`RENDERER_URL`); **1 vCPU / 1 GiB, one capture at a time, 15 s timeout**; see [2-architecture §6](2-architecture.md) |

### 4.1 GoAI capabilities the design depends on

Verified against the SDK docs ([tools](https://goai.sh/concepts/tools)):

- `goai.WithMaxSteps(n)` — auto tool loop (model → tools → results → re-invoke).
- `goai.WithOnToolCallStart` / `WithOnToolCall` — per-call hooks (`ToolName`, `Step`, `Input`,
  `Output`, `Duration`, `Error`) → **agent status**, taken directly, no extra model calls.
- `goai.StepResult.Usage` — per-step token usage → `usage_events`.
- `goai.NewTool` — typed tools; `GenerateText` / `StreamText`; DeepSeek is a first-class provider.
- Tool `Execute` errors are forwarded to the model — MUST NOT contain credentials or server paths.

## 5. Speech-to-text

**whisper.cpp compiled in via CGO** using the official `whisper.cpp/bindings/go` package, with the
model loaded once at startup. The browser records the PTT clip and uploads raw PCM/WAV; the backend
transcribes and returns text; the client posts the transcript to the agent.

| Aspect | Decision |
|--------|----------|
| Where | in-process, in-image `whisper.cpp` |
| When | whole clip, on PTT release (no streaming) |
| Format | raw PCM/WAV, 16 kHz mono, captured with an AudioWorklet |
| Model / language | `base.en`, English only |
| Max clip | **30 s**, then auto-send |
| Interim text | none — the transcript appears after release |
| Failure | error entry in the user's chat; the agent is **not** called |

## 6. Identifiers, secrets, auth

| Item | Decision |
|------|----------|
| User token | 8 chars, base62 (~47.6 bits), `crypto/rand`, globally unique |
| Room ID | 6 chars, unambiguous alphabet (no `0/O/1/l/I`) |
| User ID | sequential integer — it is the public URL segment |
| Admin | single global admin, password from `ADMIN_PASSWORD` |
| Session | `HttpOnly` `SameSite=Strict` cookie; `Secure` when `X-Forwarded-Proto=https` |

## 7. Environment variables

Flat uppercase names (decided).

| Var | Purpose | Default |
|-----|---------|---------|
| `ADMIN_PASSWORD` | admin login (required) | — |
| `DEEPSEEK_API_KEY` | LLM provider auth (required) | — |
| `PORT` | HTTP listen port | `8080` |
| `DATA_DIR` | SQLite file + subdirectories + templates | `./data` |
| `WHISPER_MODEL` | path to the `ggml-base.en.bin` model | image default |
| `RENDERER_URL` | base URL of the preview renderer service | `http://renderer:9000` |
| `AGENT_MODEL` | default model for new rooms | `deepseek-flash` |
| `AGENT_MAX_STEPS` | tool-loop cap | `20` |
| `AGENT_CONTEXT_TURNS` | sliding-window size in turns | `50` |
| `MAX_USERS_PER_ROOM` | capacity cap | `100` |
| `MAX_ROOMS` | capacity cap | `20` |
| `PREVIEW_DEBOUNCE_MS` | preview debounce after a run | `2000` |
| `SESSION_SECRET` | HMAC key for the signed session cookie — **required** | — |
| `MAX_CONCURRENT_RUNS` | global cap on simultaneously running agents | `8` |
| `AGENT_RUN_TIMEOUT_S` | per-run agent timeout | `120` |
| `APP_URL` | base URL the **renderer service** uses to reach the app's static route | `http://backend:8080` |

All defaults above are confirmed by the product owner (Q-DEPLOY-5).

## 8. Rate limits (fixed)

| Scope | Limit |
|-------|-------|
| Login | 5 attempts/min/IP |
| API (per user) | 30 requests/min |
| WebSocket (per connection) | 10 messages/s |

## 9. Out of scope (v1)

User↔user visibility · video · user text input · agent code execution · multi-replica HA ·
multiple admins · in-app TLS termination · native mobile apps · backups/restore · i18n
(English only).
