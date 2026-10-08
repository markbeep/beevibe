# Page — Admin: room settings / edit

| Field | Value |
|-------|-------|
| Route | `/admin/rooms/{roomId}` |
| Actor | admin |
| Code | `AREDIT-*` |

## Purpose

Room configuration and user management: room lifecycle, agent model, subdirectory template, and
per-user and bulk user actions.

## Room-level

- **Lifecycle** — start (enables agents, `open` → `started`), close (`started` → `closed`, agents
  disabled), reopen (`closed` → `started`), archive (`closed` → `archived`, users ejected), delete
  (archived only).
- **Agent model** — the DeepSeek model for all agents in this room (default `deepseek-flash`).
- **Subdirectory template** — upload/replace/delete a base file set. Missing `index.html` /
  `README.md` are backfilled from the defaults; files obey the text-only whitelist and caps.
  **Uploading or replacing re-seeds every existing user's subdirectory**, wiping their files and
  their agent history, so it requires a confirmation modal (affected user count) plus a progress
  bar, and locks the room meanwhile.
- **Token-limit default** for new users (default: unlimited).

## Single-user actions

- **Create user** — one name input; the server generates a globally unique token and immediately
  seeds the subdirectory from the room template (or the defaults).
- **Cancel prompt** — abort the user's in-flight agent run and clear their queue. Recorded in that
  user's chat. Enabled only while a run is in flight.
- **Set token limit** — per user; cumulative input+output tokens; `unlimited` until set.
- **Reset subdirectory** — delete the directory, re-seed from the template, and delete the user's
  agent history, locking that user until it completes.
- **Kick user** — cancel their runs, then force-disconnect. The token stays valid (rejoin allowed).
- **Delete user** — cancel their runs, then remove the user, invalidate the token, delete their
  subdirectory and history. **Permanent.**
- **Send message** — a targeted message to that user's chat.

Names need not be unique; the admin UI MUST disambiguate (e.g. show a token suffix). There is
**no** user rename and **no** token rotation — delete and re-create instead.

## Bulk actions (on some or all selected users)

Kick · delete · send message · set token limit · reset subdirectory.

## Token delivery

Per-user **copy button**, plus an **"export all tokens" CSV** for the room.

## Requirements

- **AREDIT-1** — MUST create a user from a single name input, generate a globally unique token, and
  seed the subdirectory from the room template (falling back to the built-in `README.md` +
  `index.html`).
- **AREDIT-2** — MUST offer a copy button per user and a CSV export of all room tokens.
- **AREDIT-3** — Deleting a user MUST delete their subdirectory, their chat history and their agent
  history, and invalidate the token.
- **AREDIT-4** — MUST support multi-select (including "select all filtered") and the five bulk actions.
- **AREDIT-5** — A reset MUST lock that user out of prompting until it completes.
- **AREDIT-6** — MUST NOT issue the same token twice (global uniqueness).
- **AREDIT-7** — MUST filter the user list **client-side** by name as the admin types; no paging is
  required at the 100-user cap.
- **AREDIT-8** — MUST allow uploading, replacing and deleting the room's subdirectory template.
- **AREDIT-9** — Uploading or replacing the template MUST re-seed every existing user's subdirectory
  immediately, wipe each user's agent history, lock the room until it completes, and require a
  confirmation modal stating how many users will be reset (with a progress bar).
- **AREDIT-10** — A template upload containing files that violate the whitelist or the caps MUST be
  rejected as a whole.
- **AREDIT-11** — Every seeded subdirectory MUST contain `index.html` and `README.md`; missing ones
  are backfilled from the built-in defaults.
- **AREDIT-12** — Removing a user (kick or delete) MUST cancel that user's running run and clear
  their queue before the removal completes.
- **AREDIT-13** — MUST let the admin cancel a user's in-flight prompt (aborting the run and clearing
  the queue). The cancellation MUST appear in that user's chat and files already written MUST be kept.
- **AREDIT-14** — MUST apply the template when seeding a new user's subdirectory and whenever a
  reset runs.

## Server dependencies

Room CRUD + lifecycle, user CRUD, bulk, `.../users/{uid}/{kick|cancel|reset}`, `PUT|DELETE
/api/rooms/{id}/template`.
