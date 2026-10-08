-- +goose Up

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

-- +goose Down

DROP INDEX IF EXISTS idx_usage_events_user;
DROP INDEX IF EXISTS idx_agent_runs_user;
DROP INDEX IF EXISTS idx_agent_messages_user;
DROP INDEX IF EXISTS idx_messages_room;
DROP INDEX IF EXISTS idx_messages_user;
DROP INDEX IF EXISTS idx_users_room;

DROP TABLE IF EXISTS usage_events;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_messages;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS rooms;
