# Page — Admin: room live overview (preview grid)

| Field | Value |
|-------|-------|
| Route | `/admin/rooms/{roomId}/live` |
| Actor | admin |
| Code | `AOV-*` |

## Purpose

The main in-room admin page: every user as a **screenshot tile** with their live agent state,
plus broadcast messaging. Tiles are **images**, not live iframes — this keeps the grid cheap and
removes the "user site pins the admin's browser" risk
(see [2-architecture §6](../2-architecture.md)).

## Layout

- Toolbar: room state, name filter, page controls, "message all" input + send.
- Responsive tile grid: each tile keeps a **16:9 aspect ratio** and a sensible minimum size; the
  layout **fits as many tiles as possible** for the current viewport and pushes the rest to the
  next page — so it works on mobile and on window resize.
- Each tile shows: the **preview image** (1280×720 WebP), user name, agent state
  (idle / queued / thinking / editing / error, plus queue depth), mic state, token usage,
  preview age, and an "offline" styling for users who left.
- **Fallback:** when there is no usable image — never captured, capture failed, or the renderer is
  unreachable — the tile shows a **colour tile (colour derived from the user id) with the user's
  name centred in large type**, like Zoom with the camera off. Every other tile element stays
  visible.
- A **help badge** with the tile highlighted while a raise-hand request is pending.

## Interactions

- Clicking a tile's preview **opens that user's live site in a new browser tab**.
- Clicking elsewhere on a tile opens the **per-user detail drawer**.

## Per-user detail drawer

- Full chat history (transcripts, agent status, admin messages, system entries such as
  cancellations and resets).
- Send a targeted message to that user.

## Requirements

- **AOV-1** — MUST render every user as a **server-rendered preview image**, never a live iframe.
- **AOV-2** — MUST show each user's current agent state and queue depth.
- **AOV-3** — MUST show mic state, token usage and online/offline status.
- **AOV-4** — MUST lay tiles out responsively (16:9, minimum size, fill the viewport, overflow to
  the next page) with paging controls.
- **AOV-5** — MUST allow filtering users by name.
- **AOV-6** — MUST allow broadcasting a message to all users; it appears in every user's chat and is
  persisted.
- **AOV-7** — MUST update live over the WebSocket (`user.update`).
- **AOV-8** — MUST provide a per-user detail drawer with the full chat history and a targeted
  message box.
- **AOV-9** — MUST highlight tiles with a pending help request until it is dismissed.
- **AOV-10** — Clicking a preview MUST open the user's live site in a new tab (the public URL).
- **AOV-11** — MUST refresh a tile's preview when its revision changes (`user.update.previewRev`).
- **AOV-12** — When a tile has no usable preview image (never captured, capture failed, or the
  renderer is unreachable) it MUST fall back to a colour tile showing the user's name centred; all
  other tile metadata MUST remain visible.
- **AOV-13** — Each tile MUST offer a manual "refresh preview" control that forces a re-capture.

## Server dependencies

`GET /api/rooms/{id}/users/{uid}/preview`, `.../messages`, `POST /api/rooms/{id}/broadcast`, WS
`user.update` / `room.update`.
