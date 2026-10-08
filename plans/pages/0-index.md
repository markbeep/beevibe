# Page — Index (login / landing)

| Field | Value |
|-------|-------|
| Route | `/` |
| Actor | anonymous |
| Code | `IDX-*` |

## Purpose

Entry point. A title and **one short paragraph** explaining the event app, then the login form.
A user logs in with their personal token; the admin logs in by typing the raw admin password.
On success a user goes to the user room ([4-user-room](4-user-room.md)), the admin to the room
list ([1-admin-list](1-admin-list.md)).

## Layout

1. Title + one-paragraph description.
2. Login card: a single text input + submit.
3. "Remember me" checkbox (optional).
4. Inline error region.

## Requirements

- **IDX-1** — MUST show a single-field login form (one input; no separate username/password).
- **IDX-2** — MUST resolve the submitted value: a valid *user token* → user room; the raw
  **admin password** (matched against `ADMIN_PASSWORD`) → room list.
- **IDX-3** — MUST show a generic error and stay on the page on failure.
- **IDX-4** — MUST rate-limit login attempts to 5/min/IP.
- **IDX-5** — MUST set an `HttpOnly` `SameSite=Strict` session cookie on login so the identity
  survives navigation, reload and the WS handshake.
- **IDX-6** — The optional "remember me" checkbox MUST store the **user token** client-side
  (`localStorage`) and pre-fill the field on the next visit; unchecked stores nothing. It MUST NOT
  be offered for — or store — the admin password.

## Server dependencies

`POST /api/login`, `POST /api/logout`, `GET /api/me`.
