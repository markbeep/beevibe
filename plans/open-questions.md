# Decisions & Open Questions

> **Status.** All 50 original questions were answered by the product owner and are recorded below.
> What remains is a short list of items that are either **deferred to implementation** or need
> text/sizing that only the owner can supply. Spec files reference remaining items as `❓Q-...`.

## 1. Resolved decisions

### Identity & auth

| ID | Decision |
|----|----------|
| Q-AUTH-1 | Single global admin; password from `ADMIN_PASSWORD`. |
| Q-AUTH-2 | Admin types the raw password into the same single login field as users. |
| Q-SEC-1 | User token: 8 chars base62 (`crypto/rand`). Room ID: 6 chars, unambiguous alphabet (no `0/O/1/l/I`). |
| Q-SEC-2 | Rate limits: login 30 rejected/min/IP (successes uncounted), WS 10 msg/s; no API limit. |
| Q-SEC-4 | Session = `HttpOnly` `SameSite=Strict` cookie (`Secure` behind the TLS proxy); authenticates WS handshakes. |
| Q-UI-3 | Optional "remember me" stores the token in `localStorage`; unchecked stores nothing. |

### Rooms

| ID | Decision |
|----|----------|
| Q-ROOM-1 | States `open` / `started` / `closed` / `archived`; joins allowed while `open` **and** `started`. |
| Q-ROOM-2 | Partially reversible: `closed`→`started` (reopen), `archived`→`closed` (un-archive). |
| Q-ROOM-4 | Rename at any time; delete only once `archived` (removes users, subdirectories, messages). |
| Q-ROOM-5 | "Start room" lives on the room settings page. |
| Q-ROOM-6 | Users may join before start: they see their (empty) site but cannot prompt. |
| Q-ROOM-7 | `closed`: users stay connected, get a notice, agents disabled. `archived`: users are ejected to the index page. |
| Q-ROOM-8 | Re-starting keeps subdirectory contents exactly as they are. |
| Q-ROOM-3 | Capacity caps: 100 users/room, 20 rooms (env-configurable). |
| Q-UI-4 | Rooms have an optional human-readable name in addition to the 6-char ID. |
| Q-WS-2 | The admin room list updates live. |

### Users

| ID | Decision |
|----|----------|
| Q-USER-1 | Kick = cancel runs + disconnect, token stays valid. Delete = permanent (token invalid, subdirectory + history removed). |
| Q-USER-2 | Both broadcast messages and per-user targeted messages. |
| Q-USER-3 | Duplicate display names allowed; the UI disambiguates. |
| Q-USER-4 | During a reset the user sees a "Resetting your site…" overlay with PTT disabled. |
| Q-USER-5 | Neither rename nor token rotation — delete and re-create. |
| Q-USER-6 | Offline users stay in the grid, greyed and labelled "offline". |
| Q-UI-5 | Token delivery: per-user copy button + CSV export of all room tokens. |
| Q-UI-6 | Room-settings user list: client-side name filter, no paging. |

### Subdirectory, template, files

| ID | Decision |
|----|----------|
| Q-FILE-1 | Text-only whitelist: `.html .css .js .json .svg .md .txt`. |
| Q-FILE-3 | Caps: 64 files / 256 KiB per file / 4 MiB per subdirectory. |
| Q-FILE-2 | The user's iframe reloads fully **after each agent tool call**; manual refresh also available. |
| Q-FILE-6 | Writes are atomic (temp + rename); a browser reload loses no state. |
| Q-FILE-7 | Correct MIME per extension, `.md` as `text/plain`, `nosniff` everywhere, no directory listing. |
| Q-FILE-8 | Admin templates obey the same text-only whitelist + caps. |
| Q-FILE-9 | Uploading/replacing a template re-seeds **every** user's subdirectory (confirmed modal + progress, room locked). |
| Q-FILE-10 | Missing `index.html` / `README.md` in a template are backfilled from the built-in defaults. |
| Q-FILE-4 | `README.md` counts toward the caps. |
| Q-FILE-5 | A reset (or template re-seed) also wipes that user's agent history. |
| Q-SEC-9 | Everything in the directory is public, including `README.md`. |
| Q-DATA-3 | Nothing is purged automatically; data lives until the admin deletes the room. |

### Agent

| ID | Decision |
|----|----------|
| Q-AGENT-1 | Full per-user LLM history persisted in SQLite (`agent_messages`). |
| Q-AGENT-2 | Token limit = cumulative input+output per user, blocked at the limit. |
| Q-AGENT-9 | **No default limit** — unlimited until the admin sets one. |
| Q-AGENT-3 | Tools: `list_files`, `read_file`, `write_file`, `delete_file`, subdirectory-scoped. |
| Q-AGENT-4 | Default model `deepseek-flash`, selectable per room. |
| Q-AGENT-5 | `MaxSteps = 20` (env `AGENT_MAX_STEPS`). |
| Q-AGENT-6 | Agent status derived directly from GoAI tool-call hooks; no extra model calls. |
| Q-AGENT-7 | Prompts queue per user and run sequentially. |
| Q-AGENT-8 | Template `README.md` per user + a global system prompt in the binary. |
| Q-AGENT-11 | Sliding window of the last `AGENT_CONTEXT_TURNS` (50) turns. |
| Q-AGENT-12 | Each run's context includes the current file listing (names + sizes). |
| Q-AGENT-13 | If the model id is rejected, fall back to `deepseek-chat` and log. |
| Q-AGENT-14 | Admin cancel = abort the run + clear that user's queue (same as the user's Cancel). |
| Q-AGENT-15 | Cancelled runs keep whatever files were already written (no rollback). |
| Q-AGENT-16 | Users can start a **new agent session** from the bottom bar (confirm modal): cancels the in-flight run and queue, **hard-deletes** their `agent_messages`, and keeps files, the visible chat log and the token budget. User-only — no admin equivalent. |
| Q-UI-18 | The admin's cancel control lives in the **room-settings user list**, not the grid. |

### Speech-to-text

| ID | Decision |
|----|----------|
| Q-STT-1/6 | Bundled `whisper.cpp` via a **CGO binding** (single binary). |
| Q-STT-2 | Raw PCM/WAV via `AudioWorklet`, 16 kHz mono. |
| Q-STT-3 | Whole clip, transcribed on PTT release; no streaming. |
| Q-STT-4 | `base.en`, English only. |
| Q-STT-7 | 30 s maximum clip, with a countdown and auto-send. |
| Q-STT-8 | No interim transcription while holding PTT. |
| Q-STT-5 | On failure/empty text: an error entry in the chat; the agent is not called. |

### Admin preview grid

| ID | Decision |
|----|----------|
| Q-SEC-5 | The grid no longer hosts live iframes; it shows **server-rendered screenshots**, so user JS never runs in the admin's browser. A user's own tab can still be CPU-pinned — accepted. |
| Q-PREVIEW-1 | Capture is done by a **separate headless-Chromium renderer service**, not the backend. |
| Q-PREVIEW-2 | Capture happens **after each completed agent run**, debounced. |
| Q-PREVIEW-3 | 1280×720 → WebP thumbnail, latest-only, kept in memory. |
| Q-PREVIEW-4 | The renderer **executes JavaScript** and has **unrestricted outbound network** (accepted risk T14). |
| Q-PREVIEW-6 | (was "staleness UX") — superseded: fallback is driven by "no usable image", not a staleness threshold. |
| Q-PREVIEW-7 | Fallback tile = colour tile (colour from the user id) with the user's **name centred** — the Zoom "camera off" look; other tile metadata unchanged. |
| Q-PREVIEW-8 | Fall back **whenever there is no usable image**: never captured, capture failed, or renderer unreachable. |
| Q-PREVIEW-9 | Capture on **seed** too — user creation, room start, reset completion — not only after agent runs. |
| Q-PREVIEW-10 | Recovery: automatic on the next run **plus** a per-tile manual re-capture control. |
| Q-UI-7 | Grid layout is **dynamic/responsive**: 16:9 tiles with a minimum size, fit as many per page as the viewport allows, overflow to the next page; works on mobile and on resize. |
| Q-UI-8 | Admin reads a user's chat in a **per-user detail drawer**. |
| Q-UI-16 | Not applicable — the grid shows images, so there is no per-tile iframe reload to scale. |
| Q-UI-9 | Help/raise-hand → badge + highlighted tile in the grid, plus a drawer entry. |

### UI & misc

| ID | Decision |
|----|----------|
| Q-UI-2 | Index page: title + one short paragraph + the token field. |
| Q-UI-10 | LOC = every line (including blank) of all files except `README.md`. |
| Q-UI-11 | Full chat history restored on rejoin and reload. |
| Q-UI-13 | Mic "sensitivity" = VAD threshold **and** input gain, shown against a level meter. |
| Q-UI-14 | Chat auto-scrolls, pauses while the user scrolls up, shows timestamps, full history. |
| Q-UI-15 | Admin broadcasts are persisted as messages. |
| Q-UI-17 | Template re-seed confirmation: simple confirm dialog + per-user progress bar. |
| Q-API-2 | Site URL `/rooms/{roomId}/{userId}/`, public, no session check. |
| Q-SEC-7 | User IDs are sequential integers — enumerable. Accepted. |
| Q-SEC-8 | UGC CSP `default-src 'self' 'unsafe-inline' data:` with no API `connect-src`. |
| Q-SCOPE-1 | Out of scope: user↔user visibility, video, user text input, agent code execution, multi-replica HA, multiple admins, in-app TLS, native apps, backups, i18n. |
| Q-DATA-1 | SQLite driver `modernc.org/sqlite`. |
| Q-DATA-2 | Schema **frozen** in [schema.md](schema.md). |
| Q-DATA-4 | goose numbered migrations (`00001_init.sql`, `_` separator, 5-digit padding), applied at startup; `just migrate-*` recipes for up/down/status/create. |
| Q-DEPLOY-2 | Internet-facing behind a TLS-terminating reverse proxy. |
| Q-DEPLOY-3 | Proxy terminates TLS; the app sets `Secure` when `X-Forwarded-Proto=https`; HSTS. |
| Q-DEPLOY-4 | Share links are built **client-side** from the current origin — no base-URL env var. |
| Q-BUILD-1 | Toolchain pinned via `mise.toml` (Go 1.27.1, Node 26.11.1, goose 3.28.0, sqlc 1.31.1, just). |
| Q-BUILD-2 | `@solidjs/router`. |
| Q-API-1 | Endpoint list adopted (see [2-architecture §8](2-architecture.md)). |
| Q-WS-1 | Message set: `chat.append`, `agent.state`, `site.updated`, `user.update`, `room.update`, `mic.state`, `help.request`, `agent.cancel`, `ping`. |
| Q-WS-3 | On a WS drop the run continues server-side; the client re-syncs over REST. No event buffering. |

### Agent behaviour, wire formats & previews

| ID | Decision |
|----|----------|
| Q-AGENT-10b | An unintelligible transcript → make **no changes** and post a chat line asking the user to say it again. |
| Q-AGENT-10c | The agent is **silent**: no model-written prose; the chat carries transcripts plus automatic per-tool status lines. |
| Q-DATA-5 | sqlc queries written and generated — `internal/db/queries/*.sql` (6 files) → `internal/db/gen`; `just generate` succeeds. |
| Q-API-4 | All HTTP request/response bodies frozen in [api.md](api.md). |
| Q-WS-4 | All WebSocket payloads frozen in [api.md §4](api.md). |
| Q-PREVIEW-5 | Renderer sized at **1 vCPU / 1 GiB, one capture at a time, 15 s timeout**; large rooms fill progressively. |
| Q-DEPLOY-5 | All proposed env defaults accepted (see [1-techstack §7](1-techstack.md)). |

### Pre-implementation gaps (closed)

| ID | Decision |
|----|----------|
| Q-DEPLOY-6 | Go module path `github.com/mark/beevibe`; `go.mod` created. |
| Q-STT-9 | whisper.cpp via the official `whisper.cpp/bindings/go` **cgo** package, model loaded once at startup. |
| Q-SEC-10 | Session = **HMAC-signed stateless cookie** keyed by `SESSION_SECRET`; survives restarts, DB re-checked per request. |
| Q-UI-19 | "Remember me" applies to **user tokens only**; the admin password is never stored in the browser. |
| Q-STT-10 | **No VAD threshold** — mic settings are device, input gain and a level meter. |
| Q-AGENT-17 | At most **8** agent runs concurrent instance-wide; per-run timeout **120 s**. |
| Q-DEPLOY-7 | Deploy artifacts: **Dockerfile + docker-compose.yml**. |
| Q-BUILD-4 | Tests: Go unit + SQLite integration. No frontend test tooling. |
| Q-DATA-6 | SQLite DSN enables `foreign_keys(1)`, WAL and `busy_timeout(5000)` — **required** for `ON DELETE CASCADE` to work. |
| Q-DEPLOY-8 | `/healthz` + `/readyz`, graceful SIGTERM shutdown, `web/dist` embedded, SPA fallback for client routes. |
| Q-PREVIEW-11 | Preview cache is a bounded LRU (512 entries ≈ 40 MB); evicted users are re-captured on demand. |

## 2. Remaining (small)

| ID | Item | Owner / note |
|----|------|--------------|
| ❓Q-AGENT-10 | The **global system prompt** text (tool rules, file policy, transcript handling). The seeded `README.md` is written at `internal/seed/README.md` — this item now covers only the system prompt. | Product owner writes it before implementation. Placeholder in [2-architecture §9](2-architecture.md). |
