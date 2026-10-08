# Page — User: room view

| Field | Value |
|-------|-------|
| Route | `/room/{roomId}` |
| Actor | user |
| Code | `UROOM-*` |

## Purpose

The user's entire experience. The main area is a **sandboxed iframe** of their own site; a
Zoom-like bottom bar holds all controls. There is **no text input** anywhere — every prompt is
spoken. Before the room is `started` the user sees their (initially empty) site, but the mic and
agent are inert.

## Layout

- Main area: sandboxed iframe of the user's site + a manual refresh button.
- Bottom bar: mic (PTT) + mic settings, cancel, help, chat, leave, stats.

## Controls

### Mic (push-to-talk) + settings
- **Hold `Space` (or hold the mic button) to record; release to send.** No persistent mic-on state.
- A **30 s cap**: a countdown appears near the end and the clip is auto-sent at the limit.
- No interim text; the transcript appears in the chat after release.
- Settings: input **device**, **input gain**, and a live **input level meter**. There is no VAD
  threshold — everything between press and release is uploaded.

### Cancel
- A **Cancel** button, enabled only while a run is in flight. Aborts the run and clears the queued
  prompts; the cancellation appears in the chat.

### New session
- A **New session** button in the bottom bar. After a confirmation modal ("This clears the agent's
  memory of your conversation — your site and your chat log are kept"), it starts a fresh agent
  session: the in-flight run is cancelled, the queue cleared, and the agent's conversation memory
  deleted. Files, chat log and token usage are untouched.

### Help
- Sends a "raise hand" request. It shows as a badge and a highlighted tile in the admin grid, and
  as an entry in the admin's per-user drawer.

### Chat (read-only)
- Transcripts of what was understood, live agent-status entries ("Writing index.html…"), admin
  messages (broadcast and targeted), and system entries (cancellations, resets, errors).
- Auto-scrolls to the newest entry, shows timestamps, and pauses auto-scroll while the user scrolls
  up. Full history is restored on rejoin and on browser reload.
- **No manual text input.**

### Leave room
- Returns to the index page. The token, subdirectory and agent history are kept, so the user can
  rejoin later.

### Stats
- Token usage and the limit (`unlimited` when the admin has not set one), the total **lines of
  code** in the subdirectory (every line of every file except `README.md`), and the user's own
  **site URL** as a shareable link with a **copy button** (built client-side from the current
  origin; the route is public).

## Requirements

- **UROOM-1** — MUST show the site in a sandboxed iframe and perform a **full reload after each
  agent tool call**, plus offer a manual refresh.
- **UROOM-2** — MUST capture audio only while PTT is held and upload it on release; MUST stop and
  auto-send at 30 s.
- **UROOM-3** — MUST show the transcript of the prompt in the chat after release.
- **UROOM-4** — MUST show live agent-status entries while the agent works, and when a prompt is
  queued.
- **UROOM-5** — MUST show admin messages (broadcast and targeted) in the chat.
- **UROOM-6** — MUST provide mic device selection, input gain, and a live input level meter.
- **UROOM-7** — Leave MUST preserve token, subdirectory and agent history (rejoin possible).
- **UROOM-8** — MUST show token usage, token limit (or "unlimited") and subdirectory LOC.
- **UROOM-9** — MUST NOT expose any free-text input to the user.
- **UROOM-10** — MUST indicate when prompting is unavailable: token limit reached, reset in
  progress, room not started, or room closed.
- **UROOM-11** — Before the room is `started`, MUST show the site but keep prompting and the mic inert.
- **UROOM-12** — A browser reload MUST restore full state (chat history, queued/running prompt, WS
  connection) with no progress lost.
- **UROOM-13** — MUST show in the chat when a prompt is cancelled, including cancellation by an admin.
- **UROOM-14** — The stats panel MUST show the user's own site URL as a link with a copy button so
  it can be shared.
- **UROOM-15** — While the user's subdirectory is being reset, MUST replace the site with a
  "Resetting your site…" overlay and disable PTT.
- **UROOM-16** — MUST let the user start a new agent session (bottom-bar button + confirmation
  modal). Files, the visible chat log and token usage MUST be preserved; the in-flight run and the
  queue MUST be cancelled first.

## Server dependencies

`POST /api/stt`, `POST /api/prompt`, `POST /api/me/reset-context`, `GET .../messages`,
`GET .../stats`, WS `chat.append`, `agent.state`, `site.updated`.
