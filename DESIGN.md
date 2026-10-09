---
name: beevibe
description: A voice-only workshop switchboard — every participant a lit line on the board, one operator watching them all.
colors:
  # Sepia Slate — the board itself (app.css owns these; they replace Bulma's scheme surfaces)
  sepia-deepest: "#100d09"
  sepia-base: "#15110b"
  sepia-raised: "#1d1711"
  sepia-surface: "#231d15"
  sepia-well: "#19140e"
  sepia-hover: "#2f271f"
  sepia-border: "#3c342a"
  sepia-border-strong: "#71675b"
  # Ink — text and icons sitting on Sepia Slate
  ink-bright: "#f8f3ea"
  ink-muted: "#aca397"
  ink-faint: "#887f73"
  lamp-off: "#595148"
  # Status lamps — the soft register, used only for state on text, icons and rings
  lamp-green: "#56cd7e"
  lamp-orange: "#fc9e47"
  lamp-red: "#f75e54"
  lamp-blue: "#5ca7d9"
  lamp-gold: "#efca61"
  lamp-record: "#f75e54"
  # Palette — Bulma's saturated register, used by controls (buttons, tags, progress)
  bee-yellow: "#f4c92e"
  palette-success: "#62da9e"
  palette-warning: "#f7af4a"
  palette-danger: "#f2725f"
  palette-link: "#e79f23"
  palette-info: "#83bae2"
  # Ink on a lit lamp — near-black fills in the lamp's own hue
  on-bee-yellow: "#291e00"
  on-palette-success: "#0b2d1c"
  on-palette-warning: "#362002"
  on-palette-danger: "#1c0602"
  on-palette-link: "#1c1303"
  # Danger surface — the inline error box on the login board
  danger-bg: "#381611"
  danger-ink: "#ffc2b3"
  danger-border: "#703229"
typography:
  display:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "2.5rem"
    fontWeight: 800
    lineHeight: 1.125
  headline:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 800
    lineHeight: 1.125
  title:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 800
    lineHeight: 1.125
  subtitle:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.25
  body:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
  chat:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.9rem"
    fontWeight: 400
    lineHeight: 1.35
  sender:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.7rem"
    fontWeight: 600
    letterSpacing: "0.02em"
  meta:
    fontFamily: "Inter, SF Pro, Segoe UI, Roboto, Oxygen, Ubuntu, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace"
    fontSize: "0.85em"
rounded:
  square: "0"
  control: "4px"
  default: "0.375rem"
  tile: "8px"
  panel: "0.75rem"
  pill: "9999px"
spacing:
  xs: "0.35rem"
  sm: "0.5rem"
  md: "0.75rem"
  lg: "1rem"
  xl: "1.5rem"
  xxl: "2rem"
  page: "4rem"
components:
  button-primary:
    backgroundColor: "{colors.bee-yellow}"
    textColor: "{colors.on-bee-yellow}"
    rounded: "{rounded.square}"
    padding: "0.5em 1em"
    height: "2.5em"
  button-danger:
    backgroundColor: "{colors.palette-danger}"
    textColor: "{colors.on-palette-danger}"
    rounded: "{rounded.square}"
    padding: "0.5em 1em"
    height: "2.5em"
  button-warning:
    backgroundColor: "{colors.palette-warning}"
    textColor: "{colors.on-palette-warning}"
    rounded: "{rounded.square}"
    padding: "0.5em 1em"
    height: "2.5em"
  button-plain:
    backgroundColor: "{colors.sepia-well}"
    textColor: "{colors.ink-bright}"
    rounded: "{rounded.square}"
    padding: "0.5em 1em"
    height: "2.5em"
  button-ghost:
    textColor: "{colors.palette-link}"
    rounded: "{rounded.square}"
    padding: "0.25rem 0.5rem"
    height: "1.5em"
  input-field:
    backgroundColor: "{colors.sepia-well}"
    textColor: "{colors.ink-bright}"
    rounded: "{rounded.default}"
    padding: "calc(0.75em - 1px) calc(0.75em - 1px)"
    height: "2.5em"
  tag-state:
    backgroundColor: "{colors.bee-yellow}"
    textColor: "{colors.on-bee-yellow}"
    rounded: "{rounded.default}"
    padding: "0.25em 0.75em"
    height: "1.5em"
  box-panel:
    backgroundColor: "{colors.sepia-surface}"
    textColor: "{colors.ink-bright}"
    rounded: "{rounded.panel}"
    padding: "1.25rem"
  preview-tile:
    backgroundColor: "{colors.sepia-surface}"
    textColor: "{colors.ink-bright}"
    rounded: "{rounded.tile}"
    padding: "0"
    width: "320px"
  chat-entry:
    backgroundColor: "{colors.sepia-raised}"
    textColor: "{colors.ink-bright}"
    rounded: "{rounded.square}"
    padding: "0.5rem"
  mic-button:
    backgroundColor: "{colors.bee-yellow}"
    textColor: "{colors.on-bee-yellow}"
    rounded: "{rounded.square}"
    padding: "0.5em 1em"
    width: "12rem"
  mic-button-recording:
    backgroundColor: "{colors.palette-danger}"
    textColor: "{colors.on-palette-danger}"
---

# Design System: beevibe

## Overview

**Creative North Star: "The Workshop Switchboard"**

A room full of people talking at once, and one operator reading the board. Every participant is a
line: a tile with a name, a mic lamp, a state lamp, a token count and a fresh picture of what they
are building. Nothing in this interface competes with the voices — it is warm brown-black, it is
quiet, and the only chroma it spends is on telling you what is happening right now.

The aesthetic is equipment, not product: 1px hairline separators instead of shadows pretending to
be space, square-cornered controls, system typography at two weights, and status conveyed as a lamp
(green lit, orange working, red recording, blue needs attention). Approachability comes from plain
language and generous padding, never from decoration — the copy is conversational ("Scan the token
you were given") while the chrome stays flat and unhurried.

Two layers are knowingly stacked here: **Bulma 1.0.4's dark scheme** supplies the saturated control
palette and the component behaviour, and **`web/src/app.css`** overrides the surfaces with its own
neutral family, Sepia Slate, and its own soft status lamps. Both are normative. New work belongs in
the app layer unless it is a stock Bulma control.

**Key Characteristics:**
- Warm brown-black board (`#15110b`) with six tonal steps, separated by 1px `#3c342a` hairlines.
- One bee-yellow accent (`#f4c92e`); chroma is otherwise a state channel, and the single exception
  is the per-participant hue on a preview fallback tile.
- Square-cornered buttons; radius is spent only on containers (6px inputs, 8px tiles, 12px panels).
- One system typeface, two weights (400 body / 800 titles); hierarchy by size and weight only.
- Elevation is a soft warm glow, never a black drop shadow.
- Responsiveness by `flex-wrap` and `auto-fill` grids; there are no custom media queries.
- Motion is limited to one recording pulse and Bulma's 294 ms state transitions.

## Colors

A warm brown-black board — Sepia Slate — carrying two colour registers: soft lamps for status text
and icons, Bulma's saturated palette for controls. The accent is a single bee yellow; every other
warm value is the board itself, tinted toward the same brown hue.

### Primary
- **Bee Yellow** (`#f4c92e`, Bulma `--bulma-primary`, hue 47): affirmative controls only —
  `Send all`, `Create room`, `Add user`, and the login board's `Enter`. It is also the mic button's
  idle fill, where it means "loaded and ready", not "brand".

### Secondary
- **Palette Link — Honey** (`#e79f23`, Bulma `--bulma-link`, hue 38): navigation and progress — the
  `Live overview` button, the `is-link` progress bar, the `help` tag. It is the darker, browner
  member of the yellow pair, and like every other fill it carries near-black ink.
- **Palette Danger — Coral** (`#f2725f`, Bulma `--bulma-danger`, hue 8): destructive controls
  (`Delete room`, `Delete`, `Delete permanently`) and every `<code>` run — room IDs and tokens read
  coral because Bulma sets `--bulma-code` from the danger hue.

### Tertiary
- **Palette Warning — Orange** (`#f7af4a`, Bulma `--bulma-warning`, hue 35): intervention — `Clear
  help`, the queued badge, and the cold-press state of the mic. It stays orange rather than amber so
  it can never be mistaken for the yellow action.
- **Palette Success — Green** (`#62da9e`, Bulma `--bulma-success`, hue 150): the `started` room tag.
- **Palette Info — Blue** (`#83bae2`, Bulma `--bulma-info`, hue 205): the `open` room tag, the
  bulk-selection bar, and the one cool note in an otherwise warm board.

### Neutral
- **Sepia Deepest** (`#100d09`): the well behind content — the preview tile body and the iframe area.
- **Sepia Base** (`#15110b`): the page. Everything else is a step above it.
- **Sepia Raised** (`#1d1711`): the chat panel, the bottom bar and the drawer.
- **Sepia Surface** (`#231d15`): boxes, cards, tiles and the login panel — the containers that hold
  content.
- **Sepia Well** (`#19140e`): inputs and chips, a shade under the surface.
- **Sepia Hover** (`#2f271f`): table row hover, the token chip.
- **Sepia Border** (`#3c342a`): every hairline, without exception.
- **Sepia Border Strong** (`#71675b`): the outline of a control that has to be found — the token
  input on the login board.
- **Ink Bright** (`#f8f3ea`): body and table text — bone, not white.
- **Ink Muted** (`#aca397`): helper text and the login blurb.
- **Ink Faint** (`#887f73`): the muted microphone icon on a tile.
- **Lamp Off** (`#595148`): the offline presence dot — a lamp that is simply not lit.

### Named Rules
**The Lit Line Rule.** Chroma means state. Green = live/online, orange = working or waiting, red =
recording or failed, blue = needs attention, gold = the admin speaking. Yellow is the action, and the
warm board itself is never a state. If a pixel is coloured and nothing is happening there, it is a
bug.

**The Two Registers Rule.** Status text and icons use the soft lamps (`#56cd7e`, `#fc9e47`,
`#f75e54`, `#5ca7d9`, `#efca61`); controls use Bulma's saturated palette (`#f4c92e`, `#f7af4a`,
`#f2725f`, `#e79f23`). The two sets are the same traffic light at two volumes — never mix a saturated
hue into status text or a soft lamp into a button fill.

**The One Warm Board Rule.** Every neutral is tinted toward the same brown — Bulma's scheme hue 34,
app.css's OKLCH hue 72 — so there is no blue-grey in the system. A cool grey step is as wrong here as
a decorative colour would be.

**The One Identity Exception.** A participant with no usable preview gets a tile filled with
`hsl(userId × 137.508°, 48%, 40%)` — the golden-angle hue wheel, muted so a wall of tiles still sits
inside the board — so tiles stay distinguishable at a glance. It is the only arbitrary colour in the
system; do not extend the technique elsewhere.

## Typography

**Display Font:** system UI stack, Inter first (`Inter, SF Pro, Segoe UI, Roboto, …`)
**Body Font:** the same stack
**Label/Mono Font:** the same stack; `ui-monospace, SFMono-Regular, Menlo, monospace` for token text

**Character:** one family, two weights. Nothing is loaded from a webfont, so the interface renders
in the operator's own OS voice; titles are heavy (800) and everything else is 400 or 600, which
makes hierarchy come from mass rather than from a second typeface.

### Hierarchy
- **Display** (800, 2.5rem, 1.125): the `beevibe` wordmark on the login board, and nothing else.
- **Headline** (800, 1.5rem, 1.125): every page head — `Rooms`, `Room`, with the lifecycle state as
  an inline pale chip.
- **Title** (800, 1rem): section heads inside a panel (`Subdirectory template`).
- **Subtitle** (400, 1rem, 1.25): the line under a page head, carrying the room ID as `<code>` and a
  live count of participants.
- **Body** (400, 1rem, 1.5): labels, table cells, form copy.
- **Label** (600, 1rem): form labels; `is-small` (0.75rem) inside the mic popover.
- **Chat** (400, 0.9rem, 1.35): every chat entry; transcripts are bumped to 600, agent status lines
  are italic at 70% opacity.
- **Sender** (600, 0.7rem, letter-spacing 0.02em, uppercase, 55% opacity): the `YOU` / `AGENT` /
  `ADMIN` / `SYSTEM` tag above a chat line.
- **Meta** (400, 0.75rem): tile stats, timestamps, helper text (`has-text-grey`).
- **Mono**: tokens in the room table, `ui-monospace` at 0.85em inside the `#2f271f` chip.

### Named Rules
**The One Typeface Rule.** No webfont, no second family, no display face for flavour. Hierarchy is
size plus weight: 800 for anything that names a thing, 400 for anything that explains it.

**The Tabular Values Rule.** Anything that changes in place — the 30 s countdown, chat timestamps,
the mic level meter — uses `font-variant-numeric: tabular-nums` so digits do not jitter.

## Layout

Two container widths and one grid do most of the work.

- **Admin pages** (`1200px`) and the **live grid** (`1500px`) are centred with `1.5rem` padding; the
  login board is a `62rem` flex row that wraps — a `19rem` brand lockup (wordmark, accent rule,
  blurb) beside a `25rem` sign-in panel — centred in the viewport with `4rem` of headroom.
- **The tile grid** is `repeat(auto-fill, minmax(320px, 1fr))` with a `0.75rem` gap and
  `min-height: 70vh`; tiles are 16:9 with their meta row underneath, and overflow pages through a
  simple `Prev 1/1 Next` pager rather than scrolling.
- **Tables** are full-width and hoverable, cells `0.5em 0.75em`, middle-aligned, hairline-separated.
- **A participant's room** is a `100vh` column: site area takes the remaining height, the chat panel
  is a fixed `40vh` block, and the bar with the mic button sits below both. The site iframe is
  `#fff` on `#100d09`.
- **Overlays** are a fixed right-hand drawer at `min(30rem, 100vw)`, a Bulma modal card, and a mic
  popover at `20rem`, `1rem` from the right edge.

The spacing rhythm is small and tight: `0.35–0.6rem` inside a component, `0.75–1rem` between blocks,
`1.5rem` at page scale, `2rem` inside a modal head.

### Named Rules
**The Wrap, Don't Query Rule.** Adaptability comes from `flex-wrap` and `auto-fill`/`minmax` grids;
`app.css` contains no media queries at all. The drawer already collapses via `min(30rem, 100vw)`, the
toolbars wrap, and the grid reflows. Reach for those before adding a breakpoint.

## Elevation & Depth

Bulma's dark scheme gives every surface a **warm glow** — `0 0.5em 1em -0.125em
hsla(48,80%,70%,0.1)` plus a `1px` inset ring at 2% — so nothing casts a grey drop shadow in a brown
room. Boxes, cards and modal heads carry it; buttons carry a two-layer 5% warm micro-shadow
(`0 0.0625em 0.125em` and `0 0.125em 0.25em`). The glow hue is Bulma's `--bulma-shadow-h/s`, set to
the accent's family so the lift reads as lamplight rather than as grey.

Beyond that, depth is earned rather than ambient: a surface rises only to announce a state. The
direction for new work is **softly lifted everywhere** — overlays should sit above the board with a
real elevation (a stronger warm glow plus the existing backdrop), which the drawer and the user
room's bottom bar do not yet do; today the drawer is separated by a `1px` left border and a
`oklch(12% 0.02 60 / 0.62)` backdrop only. Close that gap when working in those surfaces.

### Shadow Vocabulary
- **Panel glow** (`0 0.5em 1em -0.125em hsla(48,80%,70%,0.1), 0 0 0 1px hsla(48,80%,70%,0.02)`): the
  Bulma `--bulma-shadow` used by boxes, cards, tiles' containers and modal heads.
- **Accent glow** (`0 1.5rem 2.5rem -1.25rem oklch(60% 0.13 88 / 0.4), inset 0 1px 0 oklch(100% 0 0 / 0.05)`):
  the login sign-in panel, the one surface that lifts on atmosphere rather than state.
- **Control whisper** (`0 0.0625em 0.125em hsla(48,80%,70%,0.05), 0 0.125em 0.25em hsla(48,80%,70%,0.05)`):
  every button at rest.
- **Help ring** (`0 0 0 3px #5ca7d9`): a tile whose participant raised a hand.
- **Cold-press ring** (`0 0 0 3px oklch(85% 0.163 92 / 0.45)`): the mic while the device is being
  acquired.
- **Recording pulse** (`0 0 0 0 → 0 0 0 8px rgba(247,94,84,0)` over 1 s, infinite): the mic while it
  is actually recording.

### Named Rules
**The Glow, Not Shadow Rule.** In a brown room, elevation is lamplight, not darkness. Never add a
black or grey drop shadow; lift with the warm values above.

**The Earned Lift Rule.** A surface may rise for state (attention, recording, acquisition) or as an
overlay. Static content sits flat on the board.

## Shapes

**Square by default.** Bulma 1.0.4's `.button` declares no corner radius, and the app keeps it: over
60 buttons across the admin surface are true rectangles with a 1px border. This is the system's
strongest signature and its "equipment" tell — do not round it.

Radius appears only where a shape is a container or a chip:
- **4px** — the token chip, the tile's floating Chat/refresh buttons, the `help` badge.
- **0.375rem (6px)** — Bulma's default: inputs, selects, tags, notifications.
- **8px** — preview tiles, with `overflow: hidden` so the 16:9 site image clips to the corner.
- **0.75rem (12px)** — Bulma boxes and cards (`.box`), the login sign-in panel (`.index-panel`),
  and the `1.5rem 2rem` blocked overlay card at 10px.
- **9999px** — progress bars only.
- **50%** — the 0.6rem presence dot and the round mic glyph.

Borders are always `1px solid #3c342a`; there is no dashed, doubled or coloured border anywhere.
The only other geometry in the system is the 16:9 preview ratio and the fixed `40vh` chat block.

## Components

### Buttons
- **Shape:** true rectangles — `0` radius, `1px` border, `0.5em 1em` padding, `2.5em` height, weight
  500, 294 ms `ease-out` transitions.
- **Primary:** Bee Yellow fill with near-black brown ink (`#291e00`); the affirmative action on
  each surface — `Enter`, `Create room`, `Add user`, `Send all`.
- **Danger:** Palette Coral fill with `#1c0602` ink, reserved for destructive commit
  (`Delete room`), with `is-light` for the softer destructive variant.
- **Warning:** Palette Orange fill with `#362002` ink — `Clear help` only. It is an intervention, not
  a warning message.
- **Ghost:** `is-ghost`, no fill, honey text — the low-emphasis row actions (`rename`,
  `settings`, `live`, `Copy token`).
- **Plain:** default button — Sepia Base fill, hairline border, ink text — cancels and closes
  (`Cancel`, `Close`), often paired `is-small` inside a modal footer.
- **Disabled:** 50% opacity, no shadow.

### Chips and Tags
- **State chips:** a page head carries its lifecycle as `tag is-light` — a pale bone chip with dark
  ink, deliberately quiet so the heavy `is-4` title stays the loudest thing on the line.
- **Semantic tags:** the room table maps state to a saturated tag — `open` info, `started` success,
  `closed` warning, `archived` dark — and queue depth uses `tag is-warning is-light` while a
  raised hand uses `tag is-link`. **This is the traffic light applied to lifecycle.**
- **Shape:** 0.375rem radius, 0.75em horizontal padding, 1.5em height, 0.75rem type.
- **Presence:** the tile's `offline` chip is `tag is-dark` — grey, never red. Being away is not a
  problem.

### Cards / Containers
- **Corner Style:** 0.75rem (Bulma `--bulma-radius-large`); preview tiles use 8px.
- **Background:** Sepia Surface `#231d15` on the Sepia Base page; tiles hold a Sepia Deepest
  `#100d09` well for their 16:9 image.
- **Border:** `1px solid #3c342a` on boxes, the login panel and tiles.
- **Shadow Strategy:** the panel glow above; the login panel adds the accent glow; tiles themselves
  stay border-only.
- **Internal Padding:** `1.25rem` (Bulma box), `1.5rem` (card content), `0.5rem 0.6rem` for a tile
  meta row.

### Inputs / Fields
- **Style:** 2.5em tall, `1px` hairline border, `0.375rem` radius, scheme background, `0.75em`
  horizontal padding; labels are 600 weight with `0.5em` below. The room table's token chip and the
  login token input share the mono face.
- **Focus:** Bulma's ring — `0 0 0 0.1875em` bee yellow at 25% plus a yellow border shift.
- **Error:** never inline on the field; errors surface as `notification is-danger is-light` (a pale
  fill with strong ink) above the form or pinned above the user's bottom bar.
- **In the mic popover:** `select is-small is-fullwidth`, a range slider for input gain, and a
  `progress is-small is-link` level meter — informational only, with no threshold to set.

### Room header *(admin)*
Every room-scoped admin view opens with the same header: a quiet `All rooms` link back to the room
list, then the room's identity (name, lifecycle chip, `id · n users`) beside its controls. The room
view switch — `Settings` | `Overview` — is always the **rightmost** element of that header, so it
lands on the same pixel whichever view is showing; the room's own controls (`Rename`, the lifecycle
transitions, `Delete room`) sit to its left and belong to Settings alone. The switch marks the
current view with a pale `is-light` segment, never a lit accent: it is wayfinding, not an action.
Both views share one container width (`admin-wide`, 1500px) so the header geometry is identical.

### Navigation
There is no nav bar. Movement is by page head and button: the room list (`Rooms`) links into a room,
the room header carries the `Settings`/`Overview` switch, and its `All rooms` link returns to the
list. The only persistent chrome is the participant's bottom bar: a full-width mic button, `Help`,
`Mic settings` and `New session`.

### Chat Entries
The system's densest repeating unit: `0.9rem` text on Sepia Raised, with a 0.75rem tabular
timestamp in a 45%-opacity column, a 0.7rem uppercase sender above the body, `pre-wrap` body text,
and `0.4rem` between entries. Kind is carried by treatment, not by a bubble: transcripts 600 weight,
agent status italic at 70%, admin messages gold, system and error lines orange, help requests blue
and 600 weight. The panel auto-scrolls while the reader is within 40px of the bottom and stops the
moment they scroll up.

### Preview Tile *(signature component)*
The board's unit of attention, and the clearest expression of the whole design: 16:9, Sepia Surface,
8px corners, a Sepia Deepest well holding either the server-rendered WebP or a golden-angle hue
carrying the participant's name centred in 1.5rem 600 white with a soft text-shadow. Two floating
buttons (Chat, refresh) sit at 0.35rem from the top-right in a 60%-black pill; the help badge sits
top-left in blue with deep-brown ink. The meta row carries name, `offline` chip, mic lamp (green on /
faint muted), then state, queue depth, `n tok` and a right-aligned relative age in an 0.8rem row at
80% opacity. Offline dims the whole tile to 60% and marked rows to 55% — never hides them.

### Mic Button *(signature component)*
A lamp in the shape of a button, `12rem` wide, square, with a microphone glyph and a live label
(`Hold to talk` → `Starting…` → `Release to send`, plus the countdown). Its fill **is** its state
readout: bee yellow when idle, a yellow ring on a cold press while the device is acquired, red with
the 1 s pulse while recording. It is held by pointer or by `Space`, and it captures the pointer so a
sloppy release still ends the recording.

## Do's and Don'ts

### Do:
- **Do** keep every separation at `1px solid #3c342a` and every hover at `#2f271f`.
- **Do** carry state in the soft lamps — `.state-thinking` / `.state-editing` orange, `.state-error`
  red, `.tile-icon.is-mic` green, `.tile-help` blue, `.chat-admin` gold.
- **Do** use `<code>` for room IDs and tokens; the coral is the identifier's native ink.
- **Do** keep page heads as `title is-4` (1.5rem, 800) with the lifecycle chip inline, and reserve
  the 2.5rem/800 display for the login wordmark (`.index-mark`).
- **Do** keep the room view switch the rightmost element of a room header, and both room views on
  the same container width (`admin-wide`), so it never moves when the view changes.
- **Do** use tabular numerals for the countdown, timestamps and the mic level.
- **Do** dim an absent participant (`opacity: 0.55` on rows, `0.6` on tiles) rather than removing
  them — the operator must still see the whole room.
- **Do** express layering through the tonal ramp (Base → Raised → Surface) before reaching for a
  shadow.

### Don't:
- **Don't** add a second typeface, a webfont, or a display face.
- **Don't** add a black or grey drop shadow; in this brown room, elevation is a warm glow.
- **Don't** round the buttons. The square corner is the system's signature.
- **Don't** use chroma decoratively — no coloured headers, no gradient accents, no tinted panels.
  Only state, plus the per-participant fallback hue.
- **Don't** mix the two registers: Bulma's saturated palette fills controls, the soft lamps colour
  status text.
- **Don't** add custom `@media` breakpoints; wrap the flex row or let the grid auto-fill.
- **Don't** let it become an arcade: no neon, no glow-on-hover RGB, no animated gradients — dark must
  not read as gaming.
