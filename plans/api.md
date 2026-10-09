# API — HTTP & WebSocket Wire Formats

> **Status: FROZEN** (product owner). Shapes are implemented as written; changing one is a spec change.

## 1. Conventions

| Topic | Rule |
|-------|------|
| Encoding | JSON, UTF-8, `Content-Type: application/json`. |
| Timestamps | Unix **epoch seconds**, JSON integer (matches the DB). Never strings. |
| Absent/nullable | Nullable fields are always present and explicitly `null`. |
| IDs | `roomId` string (6 chars), `userId` int64 (sequential). |
| Auth | `beevibe_session` cookie: an **HMAC-SHA256-signed** payload `{role, roomId, userId, iat}` keyed by `SESSION_SECRET`, set `HttpOnly; SameSite=Strict; Path=/` (+ `Secure` when `X-Forwarded-Proto=https`). No `Authorization` header. A request fails with `401 unauthorized` on a bad signature, on expiry, or when the referenced user/room no longer exists (re-checked in the DB on every request). |
| CSRF | State-changing requests MUST be same-origin: the server validates `Origin` and `Sec-Fetch-Site`. |
| Errors | `{"error":{"code":"<code>","message":"<human text>"}}`. |
| Error codes | `bad_request` 400 · `unauthorized` 401 · `forbidden` 403 · `not_found` 404 · `conflict` 409 · `payload_too_large` 413 · `unsupported_media_type` 415 · `rate_limited` 429 (+`Retry-After`) · `internal` 500 |
| Rate limits | login 30 rejected/min/IP · WS 10 msg/s per connection (exceeding closes the socket with code 1008). No limit on other API routes. |
| Async operations | Resets, template uploads, bulk actions and preview refreshes return **202 Accepted**; the result arrives over the WebSocket. |

## 2. Objects

```jsonc
// Room
{ "id": "AB12CD", "name": "Friday demo" | null, "state": "open|started|closed|archived",
  "model": "deepseek-flash", "userCount": 12, "hasTemplate": true,
  "createdAt": 1760000000, "startedAt": 1760000100 | null,
  "closedAt": null, "archivedAt": null }

// User (always admin-scoped; `token` is intentionally included so the admin can distribute it)
{ "id": 42, "roomId": "AB12CD", "name": "Alice", "token": "a1B2c3D4",
  "createdAt": 1760000000, "kickedAt": null,
  "tokenLimit": null, "tokensUsed": 12345,
  "online": true, "micOn": false, "helpPending": false,
  "agentState": "idle|queued|thinking|editing|error", "queueDepth": 0,
  "previewState": "ok|missing|failed" }

// Message (chat entry)
{ "id": 99, "kind": "transcript|agent_status|admin|system|help",
  "text": "make the heading bigger", "at": 1760000123 }

// Stats
{ "tokensUsed": 12345, "tokenLimit": null, "loc": 210,
  "sitePath": "/rooms/AB12CD/42/", "agentContextMessages": 14 }
```

## 3. HTTP endpoints

### Auth

| Method | Path | Request | Response |
|--------|------|---------|----------|
| POST | `/api/login` | `{"token":"a1B2c3D4"}` (the raw admin password goes in the same field) | 200 `{"role":"user"\|"admin","roomId":..,"userId":..,"name":..,"roomState":..}` + `Set-Cookie` |
| POST | `/api/logout` | — | 204 |
| GET | `/api/me` | — | 200 `{"role":..,"roomId":..,"userId":..,"name":..,"roomState":..}`; for a user also `Stats` |

### Rooms (admin)

| Method | Path | Request | Response |
|--------|------|---------|----------|
| GET | `/api/rooms` | — | `{"rooms":[Room]}` |
| POST | `/api/rooms` | `{"name":string\|null}` | 201 `{"room":Room}` |
| GET | `/api/rooms/{roomId}` | — | `{"room":Room}` |
| PATCH | `/api/rooms/{roomId}` | `{"name"?:string\|null,"model"?:string}` | `{"room":Room}` |
| POST | `/api/rooms/{roomId}/start` | — | `{"room":Room}` |
| POST | `/api/rooms/{roomId}/close` | — | `{"room":Room}` |
| POST | `/api/rooms/{roomId}/reopen` | — | `{"room":Room}` |
| POST | `/api/rooms/{roomId}/archive` | — | `{"room":Room}` |
| DELETE | `/api/rooms/{roomId}` | — | 204 (409 unless `archived`) |

### Users (admin)

| Method | Path | Request | Response |
|--------|------|---------|----------|
| GET | `/api/rooms/{roomId}/users` | — | `{"users":[User]}` |
| POST | `/api/rooms/{roomId}/users` | `{"name":string,"tokenLimit":int\|null}` | 201 `{"user":User}` — token generated, subdirectory seeded, first preview queued |
| PATCH | `/api/rooms/{roomId}/users/{userId}` | `{"tokenLimit":int\|null}` | `{"user":User}` |
| DELETE | `/api/rooms/{roomId}/users/{userId}` | — | 204 (cancels runs, removes user, subdirectory and history) |
| POST | `/api/rooms/{roomId}/users/{userId}/kick` | — | 204 |
| POST | `/api/rooms/{roomId}/users/{userId}/cancel` | — | 204 (409 when no run is in flight) |
| POST | `/api/rooms/{roomId}/users/{userId}/reset` | — | 202 `{"status":"resetting"}` |
| POST | `/api/rooms/{roomId}/users/bulk` | `{"userIds":[int] \| "all","action":"kick"\|"delete"\|"message"\|"set-token-limit"\|"reset","text"?:string,"tokenLimit"?:int\|null}` | 202 `{"affected":int}` |

### Data (admin)

| Method | Path | Request | Response |
|--------|------|---------|----------|
| GET | `/api/rooms/{roomId}/users/{userId}/messages` | `?limit=50&before=<id>` | `{"messages":[Message]}` (oldest-first within the page) |
| GET | `/api/rooms/{roomId}/users/{userId}/stats` | — | `{"stats":Stats}` |
| GET | `/api/rooms/{roomId}/users/{userId}/preview` | — | 200 `image/webp` (`Cache-Control: no-store`, `ETag` = previewRev) · **404 when no usable image** |
| POST | `/api/rooms/{roomId}/users/{userId}/preview/refresh` | — | 202 |
| POST | `/api/rooms/{roomId}/broadcast` | `{"text":string}` | 201 `{"message":Message}` |

### Template (admin)

| Method | Path | Request | Response |
|--------|------|---------|----------|
| PUT | `/api/rooms/{roomId}/template` | `multipart/form-data`, repeated `files` parts (relative names) | 202 `{"usersReset":int}` · 400 `bad_request` when any file violates the whitelist or caps (upload rejected as a whole) |
| DELETE | `/api/rooms/{roomId}/template` | — | 204 |

### User-side

| Method | Path | Request | Response |
|--------|------|---------|----------|
| POST | `/api/stt` | body = raw PCM/WAV bytes (`Content-Type: audio/wav` or `application/octet-stream`), ≤ 30 s | 200 `{"text":string,"durationMs":int}` · 400 `bad_request` when the transcript is empty · 413 `payload_too_large` · 415 `unsupported_media_type` |
| POST | `/api/prompt` | `{"text":string}` | 202 `{"runId":int,"queued":bool,"queueDepth":int}` · 409 `conflict` when blocked (limit reached, resetting, room not started/closed) |
| POST | `/api/me/reset-context` | — | 204 (new agent session) |
| GET | `/api/me/messages` | `?limit=50&before=<id>` | `{"messages":[Message]}` |
| GET | `/api/me/stats` | — | `{"stats":Stats}` |

### Ops

| Method | Path | Response |
|--------|------|----------|
| GET | `/healthz` | 200 `{"status":"ok"}` — liveness only, checks no dependencies |
| GET | `/readyz` | 200 when the DB pings **and** the renderer is reachable; 503 with the error envelope otherwise |

### Static (public, no auth)

| Method | Path | Response |
|--------|------|----------|
| GET | `/rooms/{roomId}/{userId}/{path...}` | the file; `index.html` by default; correct MIME, `.md` as `text/plain`, `nosniff`, no directory listing |

Any other `GET` that is not `/api/*` or `/ws/*` serves the SPA's `index.html`, so client routes
(`/admin/...`, `/room/...`) survive a browser reload.

## 4. WebSocket

Endpoints: `GET /ws/user` and `GET /ws/admin` (upgrade; auth = session cookie + `Origin` check).
Envelope: a flat JSON object with a `type` discriminator. Unknown `type`s MUST be ignored.

### Hello / lifecycle

| Direction | Message |
|-----------|---------|
| server→client | `{"type":"hello","role":"user"\|"admin","roomId":"AB12CD","userId":42\|null,"roomState":"started","agentState":"idle","queueDepth":0,"blocked":false}` |
| client→server | `{"type":"ping"}` → `{"type":"pong"}` |
| server→client | `{"type":"error","code":"<code>","message":"…"}` |

### Server → user

| Type | Payload |
|------|---------|
| `chat.append` | `{"type":"chat.append","message":Message}` |
| `agent.state` | `{"type":"agent.state","state":"idle\|queued\|thinking\|editing\|error","queueDepth":0,"runId":7\|null,"blocked":false}` |
| `site.updated` | `{"type":"site.updated","rev":12}` — triggers the full iframe reload |
| `room.state` | `{"type":"room.state","state":"open\|started\|closed\|archived"}` — the close notice |

### Server → admin

| Type | Payload |
|------|---------|
| `user.update` | `{"type":"user.update","user":User}` |
| `room.update` | `{"type":"room.update","room":Room}` |

### Client → server (both channels)

| Type | Payload |
|------|---------|
| `mic.state` | `{"type":"mic.state","on":true}` |
| `help.request` | `{"type":"help.request"}` |
| `agent.cancel` | `{"type":"agent.cancel"}` — user channel only |

### Reconnect

On `hello` the client re-syncs over REST (`/api/me`, `…/messages`, `…/stats`, preview) and resumes
listening. The server does **not** buffer events; an in-flight run continues server-side.
