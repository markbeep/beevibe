# 3 — Security

> **Purpose:** the threat model, the mitigations that MUST be implemented, and the risks explicitly
> accepted. Deployed on the **public internet behind a TLS-terminating reverse proxy** with
> 5 login attempts/min/IP, 30 API requests/min/user, 10 WS messages/s.

## 1. Assets

| Asset | Why it matters |
|-------|----------------|
| Admin password (env var) | Full control of every room. |
| User tokens (8 chars, base62) | Impersonate a user; edit their site. |
| Session cookie (signed, `HttpOnly`, `SameSite=Strict`) | Access as user/admin. |
| `SESSION_SECRET` (HMAC key) | Forging a session cookie; if leaked, full impersonation. |
| LLM API key | Cost + capability. |
| Subdirectory contents | User work product (publicly readable by design). |

## 2. Threat model

| # | Threat | Mitigation / decision | Status |
|---|--------|-----------------------|--------|
| T1 | **User HTML/JS attacks the app** (XSS → steal credentials) | Serve user sites only in an iframe with `sandbox="allow-scripts"`; **never** `allow-same-origin`, **never** `allow-top-navigation`. The frame gets an opaque origin: no cookies, no `localStorage`, no parent DOM. | decided |
| T1a | Opaque-origin iframe calls the API | Session cookie is `HttpOnly` + `SameSite=Strict`, so the frame can neither read nor attach it. No permissive CORS; `Origin`/`Sec-Fetch-Site` validated on state-changing requests. | decided |
| T1b | UGC served with a loose content type | Correct MIME per extension, `.md` as `text/plain`, `X-Content-Type-Options: nosniff`, directory listing disabled. CSP: `default-src 'self' 'unsafe-inline' data:;` with **no `connect-src` to the app API**. | decided |
| T2 | **Path traversal** (agent tools or static serving) | All file access confined with Go `os.Root`; reject `..`, absolute paths, symlinks. | required |
| T3 | **Prompt injection** escaping the subdirectory | Only subdirectory-scoped file tools; no exec/shell/network; the root path is fixed by the server, never model-supplied. | decided |
| T4 | **Token brute force** | 8 chars base62 (~47.6 bits), `crypto/rand`; 30 *rejected* logins/min/IP (successful logins are never counted, so a room behind one NAT address can sign in together); constant-time lookup; failures logged without secrets. | decided |
| T5 | **Admin password brute force** | Env-only; constant-time compare; same rejected-login limit; never logged or sent to the LLM. | decided |
| T6 | **Secret leakage into LLM context or logs** | No env secrets, tokens or server paths in prompts, tool output or zap fields; GoAI hook data sanitised before logging. | required |
| T7 | **Resource exhaustion** (agent, sites, requests) | Cumulative per-user token budget; file caps (64 / 256 KiB / 4 MiB, also for templates); 30 s audio clip cap; WS message rate limit; capacity caps 100 users/room, 20 rooms. There is deliberately no blanket HTTP API limit: it throttled the single operator and protected nothing brute-forceable. | decided |
| T8 | **Cross-site WebSocket hijacking** | `Origin` verified on upgrade; cookie-authenticated (browsers attach cookies to the handshake, so the Origin check is mandatory). | required |
| T16 | **Session cookie forgery / fixation** | The cookie carries only an **HMAC-SHA256-signed** payload `{role, roomId, userId, iat}` keyed by `SESSION_SECRET`; any signature mismatch or expiry is rejected, no session id is ever accepted from the client, and the referenced user/room is re-checked in the DB on every request (so deleting a user revokes access immediately). | required |
| T9 | SQL injection | Parameterised statements only (sqlc-generated). | required |
| T10 | **Anyone viewing a user's site** | **Accepted:** `/rooms/{roomId}/{userId}/` is publicly readable with no session check; user IDs are sequential, so URLs are enumerable; users are shown their own URL to share. | accepted |
| T11 | SSRF by the agent | No network-capable agent tools. | decided |
| T12 | Runaway site pinning a viewer's CPU | The admin grid renders **images**, not iframes, so it no longer loads user JS. A user's own tab (and an admin tab opened from a preview) can still be pinned by a `while(true)` loop — accepted, no mitigation available (an iframe cannot be force-killed). | accepted |
| T13 | Credential interception on the wire | TLS terminated by the reverse proxy; cookie `Secure` when `X-Forwarded-Proto=https`; HSTS. | decided |
| T14 | **SSRF / exfiltration via the preview renderer** | The renderer loads attacker-controlled HTML with **JavaScript enabled and unrestricted outbound network**, so a user's page can make the renderer fetch internal or arbitrary URLs and encode data into requests. Mitigation: renderer runs as its **own service with no credentials, no access to the backend API, no access to `DATA_DIR`/SQLite**, one capture at a time, per-capture timeout and memory cap. Network access is otherwise **not** restricted. | accepted |
| T15 | Renderer resource exhaustion | Serial captures (one at a time), 15 s per-capture timeout, 1 vCPU / 1 GiB, debounced capture after runs; a wedged instance is restarted without affecting the backend. | required |

## 3. Decisions already taken

- Same-origin UGC in a sandboxed iframe without `allow-same-origin`.
- Public, no-auth static route with sequential user IDs; share links built client-side.
- Both LLM and STT keys/models stay server-side.
- The agent has no code-execution or network tools.
- The admin grid uses server-rendered previews instead of live iframes.
- Sessions are HMAC-signed cookies, so a backend restart logs nobody out.
- `README.md` is publicly served like any other file in the directory.

## 4. Accepted risks

- **Public, enumerable read access to user sites** (T10), including deliberate sharing of URLs.
- **Unrestricted egress from the preview renderer** (T14). A malicious user site can use it to
  probe the internal network and to exfiltrate data through request payloads. Bounded only by the
  renderer having no secrets, no data volume, and no API access.
- **CPU pinning of a user's own tab** (T12).
- **No backups and no multi-replica durability** — a lost volume loses all rooms, sites and usage.
- **Cost**: prompts consume tokens up to each user's limit; users are unlimited until the admin
  sets one.

## 5. Open questions

See [open-questions.md](open-questions.md).
