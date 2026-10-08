# beevibe — Planning Spec

**beevibe** is a voice-driven web app for **closed-room events**. An admin opens rooms and
creates users; each user gets a private static website (their *subdirectory*) that a per-user
LLM **agent** edits in response to **voice** prompts. The admin monitors every user from a
live grid.

> **Status: pre-implementation.** `plans/` is the specification; the repo also carries the
> scaffolding (`mise.toml`, `justfile`, `sqlc.yaml`, empty `internal/db/{migrations,queries}`).
> No application code exists yet. Anything marked `❓OPEN` or `⚠️ PROPOSED` is not settled.

## Repo layout

The repository now contains scaffolding alongside the spec:

```
mise.toml                 pinned toolchain (Go, Node, goose, sqlc, just)
go.mod                    module github.com/mark/beevibe
justfile                  task runner — run `just` to list recipes
sqlc.yaml                 sqlc config (schema = the goose migrations dir)
Dockerfile                multi-stage image (to be written)
docker-compose.yml        backend + renderer + volume (to be written)
internal/db/migrations/   00001_init.sql — the frozen schema, applied by goose
internal/db/queries/      6 sqlc query files (rooms, users, messages, agent, usage)
internal/db/gen/          sqlc output (package dbgen) — generated, committed
internal/seed/            embedded defaults seeded into each subdirectory (README.md, index.html)
cmd/beevibe/              backend entry point (to be written)
web/                      SolidJS + Vite frontend (to be written)
plans/                    this specification
```

Tooling conventions and verified constraints: [1-techstack.md §2](1-techstack.md),
[schema.md](schema.md).

## Reading order

| File | Defines |
|------|---------|
| [0-overview.md](0-overview.md) | Vision, glossary, personas, core flows, scope |
| [1-techstack.md](1-techstack.md) | Technologies, pinned toolchain (`mise.toml`), env vars, limits |
| [2-architecture.md](2-architecture.md) | Components, storage, static serving, preview pipeline, WS protocol, API, agent design |
| [3-security.md](3-security.md) | Threat model, mandatory mitigations, accepted risks |
| [schema.md](schema.md) | **Frozen** database schema (goose migrations, sqlc queries) |
| [api.md](api.md) | **Frozen** HTTP + WebSocket wire formats |
| [open-questions.md](open-questions.md) | Resolved decisions + the two remaining questions |
| [pages/](pages/) | Per-page specifications |

Page specs, in navigation order:
[`0-index`](pages/0-index.md) · [`1-admin-list`](pages/1-admin-list.md) ·
[`2-admin-room-edit`](pages/2-admin-room-edit.md) ·
[`3-admin-overview`](pages/3-admin-overview.md) ·
[`4-user-room`](pages/4-user-room.md)

## Conventions

- **Requirement IDs** are stable: `AREA-n` (e.g. `AUTH-3`, `AGENT-7`). Never renumber a
  released ID; deprecate it instead. IDs are unique across the whole spec.
- **RFC 2119 keywords**: MUST, MUST NOT, SHOULD, SHOULD NOT, MAY.
- Every requirement MUST be externally observable / testable.
- `> ❓ OPEN (Q-AREA-n)` — unresolved; full text lives in [open-questions.md](open-questions.md).
- `> ⚠️ PROPOSED` — drafted for review; **not** agreed. Do not implement as fact.
- Statements with no marker are **decided** by the product owner.
- Prefer tables and enumerations over prose; agents parse structure better than paragraphs.

## Requirement areas

`AUTH` identity/login · `ROOM` room lifecycle · `USER` user management · `PAGE-*` specific page ·
`AGENT` LLM agent · `STT` speech-to-text · `DATA` persistence · `API` HTTP · `WS` websocket ·
`FILE` subdirectory/files · `SEC` security · `DEPLOY` deployment/config · `BUILD` tooling.
