# 0 — Overview

> **Purpose:** the product, its vocabulary, who uses it, and the flows that must work.

## 1. Summary

beevibe is a web application for **closed-room events** (workshops, hackathons, demos). A single
**admin** opens a **room**, creates **users**, and — once everyone has joined — **starts** it.
Each user gets a private static website (a *subdirectory*) and edits it exclusively by **speaking**
to a per-user LLM **agent**. The admin monitors everyone from a **preview grid** and can broadcast
messages.

Non-negotiable product constraints:

- **All user input is voice.** There is no user-facing text input anywhere (chat is read-only).
- Users see only their own website; they never see each other.
- The agent can only edit files inside that user's subdirectory — no code execution, no shell, no
  network.
- Deployed as a containerised backend (single instance) with a headless-Chromium renderer service.

## 2. Glossary

| Term | Meaning |
|------|---------|
| **Admin** | The single operator. Authenticates with `ADMIN_PASSWORD`. Manages all rooms. |
| **Room** | One event session. 6-char ID, optional name, lifecycle state (§5), per-room agent model. |
| **User** | A participant created by the admin. Display name (not unique) + 8-char login **token**, scoped to one room. Sequential integer ID used in the public URL. |
| **Token** | Unique secret used to log in. A user token resolves to one user; the admin password resolves to the admin. |
| **Subdirectory** | Per-user directory holding the static site (`index.html`) plus `README.md` for the agent. |
| **Template** | Optional room-level file set uploaded by the admin that seeds subdirectories; absent → built-in `README.md` + `index.html`. |
| **Agent** | The per-user LLM instance editing only that subdirectory. Own conversation context. |
| **User session** | A user's authenticated, connected presence in a room. |
| **Agent session** | The agent's conversation memory for one user. Starting a new one clears it; files and the chat log are untouched. |
| **PTT** | Push-to-talk: hold `Space` (or the mic button) to record; release to send. |
| **Transcript** | Speech-to-text result of a PTT press. Shown in chat and submitted to the agent. |
| **Agent status** | Human-readable summary of what the agent is doing, derived directly from tool calls. |
| **Preview** | Server-rendered 1280×720 WebP screenshot of a user's site, shown in the admin grid. |
| **Renderer** | The separate headless-Chromium service that produces previews. |

## 3. Personas

### 3.1 Admin
- Logs in with the admin password → room list.
- Creates/renames/archives/deletes rooms; creates, kicks, resets and deletes users.
- Uploads a subdirectory template; sets per-room agent model and per-user token limits.
- Starts and closes the room.
- Watches the preview grid, reads any user's chat in a drawer, broadcasts messages, cancels prompts.

### 3.2 User
- Logs in with a personal 8-char token.
- Sees only their own website in an iframe.
- Holds PTT to speak a prompt; the agent edits the site.
- Reads a read-only chat (transcripts, agent status, admin messages, system notices).
- Can share their site URL from the stats panel.

## 4. Core flows

| # | Flow |
|---|------|
| 1 | **Admin login** — raw admin password → room list. |
| 2 | **Create room** — optional name → auto-navigate to room settings. |
| 3 | **Create user** — one name → unique token generated, subdirectory seeded from the room template (or defaults) → admin copies the token (or exports the room's CSV). |
| 4 | **User join** — token → their room view. Before the room starts they see their site but cannot prompt. Users may still join after the room has started. |
| 5 | **Start room** — agents enabled for every user; users created later are seeded and enabled immediately. |
| 6 | **Voice edit** — hold PTT (≤30 s) → release → server transcribes → transcript queued for that user's agent → agent edits files (iframe reloads after each tool call) → transcript + status appear in chat. |
| 7 | **Monitor** — the admin grid shows a preview image per user with agent state, mic state and token usage; clicking a preview opens the live site in a new tab. |
| 8 | **Leave / rejoin** — token, subdirectory and agent history are kept so the user can rejoin. |
| 9 | **Close / archive** — closing disables all agents (sites stay viewable, no new joins); archiving ejects users and hides the room from them. Reopen and un-archive are possible. |
| 10 | **Cancel a run** — the user (Cancel button) or the admin (room settings) aborts an in-flight prompt; the cancellation appears in the user's chat and partial edits are kept. Kick/delete and close/archive cancel too. |
| 11 | **Upload template** — the admin uploads a base file set; every existing user's subdirectory is re-seeded (destructive, confirmed, room locked) and new users are seeded from it. |
| 12 | **New agent session** — from the bottom bar (with confirmation) the user clears the agent's conversation memory; the site, the chat log and the token budget are untouched. |

## 5. Room states

| State | New joins | Agents | Sites visible to users | Notes |
|-------|-----------|--------|------------------------|-------|
| `open` | allowed | disabled | yes | Created, not started. |
| `started` | **allowed** | **enabled** | yes | Live; users may still join. |
| `closed` | blocked | disabled | yes | Users stay connected and get a notice; no further edits. |
| `archived` | blocked | disabled | **no** | Users are ejected to the index page; admin-only. |

Transitions: `open` → `started` → `closed` → `archived`, and reversible in the direction
`closed` → `started` (reopen) and `archived` → `closed` (un-archive). A room can be deleted only
once archived. Re-starting keeps subdirectory contents exactly as they are.

## 6. Scope

**In scope (v1):** admin room/user management, per-user static-site generation with an optional
room template, server-side STT, per-user LLM agent with file tools, realtime WS updates,
server-rendered preview grid, token accounting, prebuilt default prompts, single-instance
deployment with a renderer service.

**Out of scope (confirmed):** user↔user visibility · video · user text input · agent code execution ·
multi-replica HA · multiple admins · in-app TLS termination · native mobile apps · backups/restore ·
i18n (English only).

## 7. Open questions

See [open-questions.md](open-questions.md) — only a few remain (agent instruction text and preview
service sizing).
