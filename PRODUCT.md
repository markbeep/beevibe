# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

**Participants.** Attendees of a small hands-on vibecoding workshop, one room, roughly 10–30 people,
instructor-led. Each sits at their own machine and builds a private static website by speaking to a
per-user LLM agent. They have no text input of any kind: push-to-talk is the entire input surface.
Each participant sees only their own site and their own read-only chat (transcripts, automatic agent
status lines, admin messages).

**Admin.** The single operator who runs the session. Authenticates with `ADMIN_PASSWORD`, owns the
room, creates participants, hands out login tokens, starts and closes the room, watches the preview
grid, reads any participant's chat, cancels runs, and resets or re-seeds sites.

The audience is workshops only — not hackathons, classrooms, conferences, or client demos. Larger
and longer-running events are out of the intended envelope rather than a scaling target.

## Product Purpose

beevibe lets a room full of people build and iterate on their own web pages by speaking, with no
typing and no code. A participant holds a key, says what they want, and their live site changes;
the operator can see every participant's site, state and token usage on one screen at the same
time.

Success means: the whole room has a working, individually editable site within a session, and the
operator can tell what everyone is doing without walking the room.

## Positioning

Voice-only editing with no text fallback at all, one sandboxed static site per participant, and a
live server-rendered preview grid of the entire room — the combination, not any one part. A
chat-based site builder assumes typing; a general coding agent assumes one user at a terminal;
neither presents the whole room to an operator as it happens.

## Operating Context

- A physical room where many people talk at once, sometimes simultaneously ("call center vibes").
- A session is short and discrete: create room → create participants → distribute tokens → start →
  edit by voice → close.
- Tokens are delivered by per-participant copy button or a CSV export of the room; participants
  join by pasting an 8-character token into the same single login field the admin uses.
- Participants may join before the room starts (they see their site, cannot prompt) and may still
  join after it starts.
- Rooms are closed events: no public signup, no directory, no participant-to-participant visibility.
- Single-instance deployment, internet-facing behind a TLS-terminating reverse proxy; a separate
  headless-Chromium renderer service produces the previews.
- Files and agent history survive leaving and rejoining; the room ends by closing (sites stay
  viewable, edits stop) or archiving (participants ejected).

## Capabilities and Constraints

Confirmed behaviour:

- Rooms with lifecycle `open` → `started` → `closed` → `archived`; `closed` → `started` and
  `archived` → `closed` are reversible; deletion only after archiving.
- Per-user static subdirectory (`index.html` plus a `README.md` for the agent), optionally seeded
  from a room template that the admin uploads; replacing the template re-seeds every participant's
  files and wipes their agent history.
- Per-user LLM agent with `list_files` / `read_file` / `write_file` / `delete_file` scoped to that
  subdirectory; prompts queue per user and run sequentially; the iframe reloads after each tool call.
- Bundled `whisper.cpp` (`base.en`, English only), whole-clip transcription on PTT release, 30 s
  maximum clip with countdown and auto-send.
- Server-rendered previews (1280×720 WebP, latest-only) refreshed after each completed agent run;
  colour tile with the participant's name centred whenever there is no usable image.
- Token accounting per user: cumulative input+output, unlimited until the admin sets a limit.
- Admin broadcast and per-participant messages, help/raise-hand signal, and run cancellation from
  both sides.

Hard constraints:

- **All user input is voice.** There is no user-facing text input anywhere; the chat is read-only.
- The agent can only edit files inside the participant's subdirectory — no code execution, no shell,
  no network access from the agent.
- Text-only file whitelist: `.html .css .js .json .svg .md .txt`, max 64 files, 256 KiB per file,
  4 MiB per subdirectory.
- The agent is silent: no model-written prose in chat; status comes from tool-call hooks.
- Single instance, SQLite; ≤100 users per room and ≤20 rooms (env-configurable); ≤8 concurrent agent
  runs instance-wide, 120 s per run.
- Renderer sized at 1 vCPU / 1 GiB, one capture at a time, 15 s timeout — large rooms fill
  progressively.
- English only.

Explicitly undecided: the text of the global agent system prompt (spec `Q-AGENT-10`) — the product
owner supplies it; `internal/agent/system_prompt.txt` currently carries a placeholder.

Out of scope (confirmed): participant-to-participant visibility · video · user text input ·
agent code execution · multi-replica HA · multiple admins · in-app TLS termination · native mobile
apps · backups/restore · i18n.

## Brand Commitments

The name **beevibe** is fixed. The screenshots in `assets/` are the only committed visual evidence
of the shipped product. Copy voice and the visual world are deliberately still open — the informal
tone in the README is not a binding commitment.

## Evidence on Hand

- `plans/` — the product owner's decided specification: overview, tech stack, architecture,
  security, frozen schema and wire formats, per-page specs, and the resolved-question log. This is
  the authoritative product record; treat unmarked statements there as decided.
- `assets/admin-overview.png`, `assets/subdirectories.png`, `assets/participant.png` — real
  screenshots of the running app.
- `data/beevibe.db`, `data/rooms/` — a local development dataset.
- The working implementation (`cmd/`, `internal/`, `web/`, `bin/`) demonstrates current behaviour;
  where it disagrees with `plans/`, the shipped behaviour is what exists and the spec is the intent.

Absent, and not to be fabricated: testimonials, named customers, benchmarks, pricing pages, press
coverage, or deployment/licensing claims.

## Product Principles

1. **Voice is the product.** Every interaction decision assumes speaking is the only way in; never
   add a text path to smooth over a rough spot, and design the rough spots instead.
2. **One room, seen whole.** The operator's grid is a first-class surface, not an admin afterthought;
   the room's health must be readable at a glance while everyone works.
3. **Private work, visible operator.** Participants never see each other; the operator sees
   everything. Both halves are load-bearing.
4. **The agent is a silent editor.** It changes files and reports through tool calls — it does not
   converse, explain, or apologise in prose.
5. **Workshop-grade, not enterprise-grade.** Single instance, tokens and one password, no HA, no
   accounts, no migrations to run. Friction is the enemy at the door.

## Accessibility & Inclusion

Target: **WCAG 2.2 AA** — measurable contrast, visible and ordered focus, full keyboard reach for
every control (login, token entry, mic button, mic settings, cancel, new session, admin tables and
drawer), and live-region (or equivalent) announcement of realtime chat, agent status and room-state
changes, since the chat is the only channel carrying that information.

Deliberately excluded: there is **no non-voice input path**. A participant who cannot speak, or who
is in a room too loud for transcription, cannot use the product. This is a known, recorded
limitation of the product's core constraint, not an oversight — do not remove it silently, and do
not add a text fallback without the owner changing that constraint.
