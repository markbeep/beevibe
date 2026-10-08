# Page — Admin: room list

| Field | Value |
|-------|-------|
| Route | `/admin` |
| Actor | admin |
| Code | `ALIST-*` |

## Purpose

Landing page after admin login. Lists **all rooms** with their optional name, 6-char ID, user
count and state. The admin can create a room (→ room settings), rename one, and delete an
`archived` room.

## Layout

- Header: app title, logout.
- Toolbar: "Create room" (prompts for the optional name).
- Table of rooms: name · ID · user count · state · created-at · actions (open, rename,
  close/reopen, archive, delete when archived).
- Updates live over the WebSocket.

## Requirements

- **ALIST-1** — MUST list every room with name, ID, user count and state.
- **ALIST-2** — MUST create a room with an optional human-readable name; on success navigate to the
  room settings page.
- **ALIST-3** — MUST rename a room at any time.
- **ALIST-4** — MUST reflect room/user changes **live** (WS `room.update`), not only on refresh.
- **ALIST-5** — MUST allow deleting a room **only when it is `archived`**, with a confirmation that
  states that users, subdirectories and messages will be removed.
- **ALIST-6** — MUST show the room's state transitions available at that moment (start, close,
  reopen, archive) and never offer an invalid one.

## Server dependencies

`GET|POST /api/rooms`, `PATCH|DELETE /api/rooms/{id}`, `POST /api/rooms/{id}/{start|close|reopen|archive}`.
Room states: [0-overview §5](../0-overview.md).
