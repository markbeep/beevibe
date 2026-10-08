# Frozen Schema

> **Status: FROZEN** (decided by the product owner). Changes after this point go through a new
> numbered goose migration, never by editing `00001_init.sql`.
>
> Engine: SQLite (driver `modernc.org/sqlite`). Migrations: **goose**. Query layer: **sqlc**.
>
> **Paths** (see [1-techstack §2.1](1-techstack.md)): migrations `internal/db/migrations/`,
> queries `internal/db/queries/`, generated `internal/db/gen/` (package `dbgen`), config
> `sqlc.yaml` at the repo root. sqlc's `schema:` points at the migrations directory.
>
> **Verified conventions:**
> - Filenames are `NNNNN_name.sql` with an **underscore** separator. `0001-init.sql` (hyphen) is
>   rejected by goose: `no filename separator '_' found`.
> - Use **5-digit** padding (`00001_init.sql`) because `goose create -s` generates 5-digit names,
>   so hand-written files stay consistent and `goose fix` is a no-op.
> - sqlc parses goose files directly: it reads the `-- +goose Up` section and ignores
>   `-- +goose Down` (verified with a real `sqlc generate` run).

## 1. Conventions

- All timestamps are Unix epoch seconds, `INTEGER NOT NULL`.
- All foreign keys use `ON DELETE CASCADE` so deleting a room removes its users, messages and runs.
- `users.id` is a **sequential integer** — it is the public URL segment
  (`/rooms/{roomId}/{userId}/`).
- Bounded enums use `CHECK` constraints.

## 2. Tables

```sql
-- 00001_init.sql

CREATE TABLE rooms (
    id          TEXT PRIMARY KEY,              -- 6 chars, unambiguous alphabet
    name        TEXT,                          -- optional human-readable name
    state       TEXT NOT NULL DEFAULT 'open'
                CHECK (state IN ('open','started','closed','archived')),
    model       TEXT NOT NULL DEFAULT 'deepseek-flash',
    created_at  INTEGER NOT NULL,
    started_at  INTEGER,
    closed_at   INTEGER,
    archived_at INTEGER
);

CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,   -- public URL segment
    room_id     TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,                       -- display name, not unique
    token       TEXT NOT NULL UNIQUE,                -- 8 chars, base62, globally unique
    created_at  INTEGER NOT NULL,
    kicked_at   INTEGER,                             -- last forced disconnect (advisory)
    token_limit INTEGER,                             -- NULL = unlimited
    tokens_used INTEGER NOT NULL DEFAULT 0,
    last_seen_at INTEGER
);

CREATE TABLE messages (                              -- the chat log
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    room_id    TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,  -- NULL = room-wide/broadcast
    kind       TEXT NOT NULL
               CHECK (kind IN ('transcript','agent_status','admin','system','help')),
    text       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE agent_messages (                        -- LLM conversation history
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,                     -- monotonic per user
    role       TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool')),
    content    TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE agent_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    prompt      TEXT NOT NULL,                       -- the transcript that started it
    status      TEXT NOT NULL
                CHECK (status IN ('queued','running','finished','cancelled','failed')),
    started_at  INTEGER,
    finished_at INTEGER,
    steps       INTEGER
);

CREATE TABLE usage_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    input_tokens  INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    created_at    INTEGER NOT NULL
);

CREATE INDEX idx_users_room            ON users(room_id);
CREATE INDEX idx_messages_user         ON messages(user_id, id);
CREATE INDEX idx_messages_room         ON messages(room_id, id);
CREATE INDEX idx_agent_messages_user   ON agent_messages(user_id, seq);
CREATE INDEX idx_agent_runs_user       ON agent_runs(user_id, id);
CREATE INDEX idx_usage_events_user     ON usage_events(user_id);
```

## 3. Not in the database

| Data | Where | Why |
|------|-------|-----|
| Subdirectory files | `DATA_DIR/rooms/<room_id>/<user_id>/` | Filesystem; served directly. |
| Room template | `DATA_DIR/templates/<room_id>/` | Filesystem; copied on seed. |
| Latest grid preview | In memory, keyed by user id | 1280×720 WebP, latest-only (see [2-architecture §6](2-architecture.md)). |
| Live session/queue state | In memory | Runs and prompt queues do not survive a restart (in-flight runs are marked `failed`). |

## 4. Derived values

| Value | Computation |
|-------|-------------|
| `users.tokens_used` | Sum of `usage_events` for that user (maintained on write, authoritative in `usage_events`). |
| Subdirectory LOC (user-facing stat) | Every line (including blank) of every file except `README.md`. |
| Room user count | `COUNT(users)` for that room. |

## 5. Lifecycle notes

- **Agent context reset** (user-initiated, "New session"): `DELETE FROM agent_messages WHERE
  user_id = ?`. **No schema change**; `seq` restarts at 1 for that user. `agent_runs`,
  `usage_events` and `messages` are retained, so usage accounting and the visible chat log survive.
- **Subdirectory reset / template re-seed** (admin): deletes the files **and** `agent_messages` for
  every affected user (Q-FILE-5), and re-seeds from the room template.

## 6. sqlc queries (to write)

`queries/rooms.sql`, `queries/users.sql`, `queries/messages.sql`, `queries/agent.sql`,
`queries/usage.sql`: create/rename/state-change room; create/delete/rotate user; bump
`tokens_used`; insert and page messages; append/paginate `agent_messages`; run lifecycle and
usage inserts; per-user and per-room aggregates.
