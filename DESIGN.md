---
version: alpha
name: Jobs
description: Design system for the Jobs web dashboard — a read-only, live observability view onto a hierarchical task store used primarily by AI agents. Local-first, desktop-first, dark-by-default.

colors:
  # Surface tiers — tonal layering, not shadows
  background:               '#0b1113'
  surface:                  '#11181a'
  surface-raised:           '#171f21'
  surface-popover:          '#1d262a'
  outline:                  '#2a3438'
  outline-strong:           '#3b474d'

  # Text
  on-surface:               '#e6ecea'
  on-surface-muted:         '#a8b4b2'
  on-surface-dim:           '#6f7b79'

  # Primary accent — chrome only (tabs, focus, primary buttons, heartbeat)
  primary:                  '#3cddc7'
  primary-dim:              '#1fa997'
  on-primary:               '#00201c'

  # Status — semantic; always paired with icon + text
  status-todo:              '#8b9a97'
  status-active:            '#3dd280'
  status-blocked:           '#e8b14a'
  status-done:              '#5a6967'

  # Signals — aging / stuck warnings; distinct from the formal blocked state
  signal-warn:              '#e8865c'
  signal-alert:             '#e84d4d'

  # Error — retained per spec recommendation (validation, connection loss)
  error:                    '#ff9b94'
  on-error:                 '#3e0000'

typography:
  display:
    fontFamily: system-ui
    fontSize: 28px
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: -0.02em
  heading-lg:
    fontFamily: system-ui
    fontSize: 20px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: -0.01em
  heading-md:
    fontFamily: system-ui
    fontSize: 15px
    fontWeight: 600
    lineHeight: 1.3
  body-md:
    fontFamily: system-ui
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
  body-sm:
    fontFamily: system-ui
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.4
  label-caps:
    fontFamily: system-ui
    fontSize: 11px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: 0.06em
  data-md:
    fontFamily: ui-monospace
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
  data-sm:
    fontFamily: ui-monospace
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.4
  data-id:
    fontFamily: ui-monospace
    fontSize: 11px
    fontWeight: 500
    lineHeight: 1.3

rounded:
  none: 0
  sm: 3px
  md: 6px
  lg: 10px
  xl: 16px
  full: 9999px

spacing:
  unit: 4px
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
  xxl: 40px
  gutter: 16px
  container-padding: 24px

# Extension: motion tokens (not in the base spec; preserved as domain-specific).
motion:
  duration-fast: 120ms
  duration-base: 200ms
  duration-slow: 320ms
  ease-out: cubic-bezier(0.2, 0.8, 0.2, 1)
  ease-in-out: cubic-bezier(0.4, 0, 0.2, 1)

components:
  # Avatars — the canonical actor primitive, five sizes
  avatar-dot:
    size: 6px
    rounded: '{rounded.full}'
  avatar-sm:
    size: 20px
    rounded: '{rounded.full}'
    typography: '{typography.data-id}'
  avatar-md:
    size: 24px
    rounded: '{rounded.full}'
    typography: '{typography.data-id}'
  avatar-lg:
    size: 32px
    rounded: '{rounded.full}'
    typography: '{typography.data-sm}'

  # Status pill — icon + text, 10%-opacity fill tinted by status color
  status-pill:
    rounded: '{rounded.full}'
    padding: 4px 10px
    typography: '{typography.label-caps}'

  # Signal card — home page; thin colored progress underline
  signal-card:
    backgroundColor: '{colors.surface}'
    rounded: '{rounded.lg}'
    padding: 20px
  signal-card-underline:
    height: 2px
    rounded: '{rounded.full}'

  # Graph node — 32px avatar disk, ring = status
  graph-node:
    size: 32px
    rounded: '{rounded.full}'

  # Peek sheet — slides from right, URL ?preview=<id>
  peek-sheet:
    backgroundColor: '{colors.surface-popover}'
    width: 440px
    padding: 24px

  # Scrubber — collapsed pill near footer, expands into full scrubber
  scrubber-pill:
    backgroundColor: '{colors.surface-raised}'
    rounded: '{rounded.full}'
    padding: 6px 12px
    typography: '{typography.label-caps}'

  # Buttons — read-only UI needs only two variants
  button-primary:
    backgroundColor: '{colors.primary}'
    textColor: '{colors.on-primary}'
    rounded: '{rounded.md}'
    padding: 8px 14px
    typography: '{typography.label-caps}'
  button-primary-hover:
    backgroundColor: '{colors.primary-dim}'
  button-ghost:
    backgroundColor: transparent
    textColor: '{colors.on-surface-muted}'
    rounded: '{rounded.md}'
    padding: 8px 14px
    typography: '{typography.label-caps}'
  button-ghost-hover:
    backgroundColor: '{colors.surface-raised}'
    textColor: '{colors.on-surface}'

  # Input (search)
  input:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.on-surface}'
    rounded: '{rounded.md}'
    padding: 8px 12px
    typography: '{typography.body-md}'

  # Data row — dense tables in Active Claims, Recent Completions, Log
  data-row:
    height: 36px
    padding: 0 12px
    typography: '{typography.body-sm}'
  data-row-hover:
    backgroundColor: '{colors.surface-raised}'

  # ID pill — monospace short ID rendered as a compact chip
  id-pill:
    backgroundColor: '{colors.surface-raised}'
    textColor: '{colors.on-surface}'
    rounded: '{rounded.sm}'
    padding: 2px 6px
    typography: '{typography.data-id}'

  # Footer metric strip
  metric-strip:
    backgroundColor: '{colors.background}'
    height: 36px
    padding: 0 24px
    typography: '{typography.label-caps}'
---

## Overview

Jobs is a read-only live dashboard onto a hierarchical task store. The CLI serves agents; this dashboard serves the humans watching them. The design aims to be **focused and quietly polished** — a window onto work, not a tool for directing it.

The aesthetic is calm, precise, unobtrusive. Dense when you look at it, invisible when you don't. Typography carries rhythm, motion is fast and purposeful, color is reserved and earned. The interface should feel like a well-built native app: restrained surfaces, confident spacing, no decorative weight.

Dark mode is the default — this is a tool you leave open on a second monitor through a long session. Light mode exists but is not the primary target.

The personality is that of a precise instrument, not an enterprise dashboard. No "system status" chrome, no versioned runtime labels, no generic admin sidebars.

## Colors

Three color axes are kept independent and must not be confused:

1. **Chrome** — the single primary accent, a teal (`primary #3cddc7`). It drives active tab indicators, focus rings, primary button fills, the live-heartbeat pulse, and the selected-tab bar. Chrome never encodes semantic state.
2. **Status** — four semantic colors for `todo`, `active`, `blocked`, `done`. Always paired with an icon and a text label; never used alone.
3. **Actor identity** — deterministic hash-derived HSL, fixed at S 65% and L 55% for stable vibrance across hues. Used *only* inside avatar circles. Because an actor's hash may land on any hue (including green), status **always** occupies a different visual axis — the ring/outline around the avatar — never its fill.
4. **Label identity** — deterministic hash-derived HSL on the same hue space as actors, but with lower saturation (S 40%, L 50%) and rendered as a 15%-opacity fill with the full-chroma value as a 1px outline. Lower saturation keeps labels from competing visually with actor avatars on the same row. Like actors, the same label name always renders the same color everywhere.

A **signal palette** separates aging / stuck warnings from the formal blocked state:

- `signal-warn` (warm orange) — idle actors, oldest todos, longest claims, and any ambient progress bar approaching a threshold.
- `signal-alert` (red) — reserved for extreme cases; should appear rarely. If `signal-alert` is filling the screen, something is genuinely wrong.

**Surface tiers** are tonal, not shadowed:

- `background` — the canvas.
- `surface` — cards, panels, table backgrounds.
- `surface-raised` — hover and active rows, slightly elevated blocks.
- `surface-popover` — the peek sheet, popovers, menus. Only this level uses a shadow.

Each step is a small lightness increase against a cool-neutral base. Outlines (`outline`, `outline-strong`) are hairline borders that define structure without visual noise.

## Typography

A dual-font strategy distinguishes orchestration from payload, using **system fonts** so the dashboard feels native on every platform and the binary stays lean (no font embedding, no network fetch).

- **`system-ui`** (UI): all navigation, headings, body copy, labels, button text. Resolves to SF Pro on macOS, Segoe UI on Windows, platform default on Linux. Tracking is tightened at display and heading levels (−0.01 to −0.02em) for a controlled, grid-aligned feel.
- **`ui-monospace`** (data): task IDs, timestamps, event payloads, and the bodies of notes — which render as preformatted code blocks per the vision doc. Resolves to SF Mono on macOS, Consolas on Windows, platform default on Linux. A monospace is required here because tabular alignment and preserved whitespace matter.

`label-caps` uses letter-spaced sentence case, not shouty all-caps. Use full uppercase only for short anchor labels (single-word section headers); default to letter-spaced sentence case elsewhere.

Line height is generous (1.4–1.5) in body and data styles to support scanning long lists and multi-line notes. Heading line heights are tighter (1.2–1.3) for typographic density.

## Layout

The dashboard follows a **top-navigation-only** model. No sidebars. The structure is:

- **Header** (slim): tabs (Home / Plan / Actors / Log), global search (`/`-focus), theme toggle, notification bell.
- **Main** (flexible): view-specific content on a content-driven column layout. Home is three-panel; Plan is a single indented tree; Actors is column-per-actor with a timeline strip; Log is a single dense list.
- **Footer** (thin, persistent): metric strip, heartbeat, connection status, and the collapsed scrubber pill. Every view has this footer.

4px baseline rhythm governs all spacing. The spacing scale is deliberately small — a dashboard rarely needs anything above 40px.

- Container padding: 24px.
- Gutter between major blocks: 16px.
- Internal padding within dense components (table rows, signal card internals): 8–12px.

Information density is **dense but breathable**: tight internal padding within components, generous margins between blocks. Row heights hold to 32px or 36px depending on metadata weight. Wide viewports are the primary target; mobile degrades to a single-column status view.

## Elevation & Depth

Depth is achieved through **tonal layering** and **hairline outlines**. Shadows are reserved for true overlays.

- **Level 0 — canvas.** `background`. The page behind everything.
- **Level 1 — cards, panels.** `surface` with a 1px `outline` border. No shadow.
- **Level 2 — hover, raised rows, elevated sections.** `surface-raised`. No shadow; the lightness shift alone does the work.
- **Level 3 — peek sheet, popovers, menus.** `surface-popover` with a large-radius ambient shadow (`0 8px 32px rgba(0, 0, 0, 0.5)`). Level 3 is the only tier that floats over the view.

Hover on list rows and cards brightens the background one step (Level 1 → Level 2). Focus outlines the element with `primary` at 2px and a soft outer ring at 40% opacity.

## Shapes

The shape language is **soft-technical**: rounded enough to feel calm, sharp enough to feel engineered.

- `sm` (3px) — inline pills, chips, ID pills.
- `md` (6px) — buttons, inputs, most cards.
- `lg` (10px) — signal cards, major panels.
- `xl` (16px) — reserved for prominent feature containers; used sparingly.
- `full` — avatars at every size, the collapsed scrubber pill, status pills, and the selected-tab underline cap.

A single view should not mix `md` and `xl` radii. `sm` may accompany `md` for small details inside a `md` container (an ID pill inside a button-sized card, for example).

## Components

**Avatars — the canonical actor primitive.** Every representation of an actor in the UI is a size of the same atom: a circular disk filled with the actor's hash color, containing the first letter of the actor's name in white. Five sizes:

- `avatar-dot` (6px) — inline attribution in tight spaces.
- `avatar-sm` (20px) — next to names in tables, logs, event rows.
- `avatar-md` (24px) — inline with names at body size.
- `avatar-lg` (32px) — column headers in the Actors view and graph nodes.
- **Avatar stack** — 8px overlap when multiple actors have touched a task.

Never render an actor as a naked name; always include at least the dot form.

**Status pills.** Icon (inline SVG) + short text ("Active", "Blocked", "Done", "Todo") + a 10%-opacity fill tinted by the status color and a 1px border of the same color at full opacity. Typography is `label-caps`. Used on every task row, task card, and task detail.

**Signal card.** The three home-page cards (idle, blocked, longest-claim, oldest-todo). Internal layout: icon + uppercase label + `display` value + `body-sm` context line. A 2px colored underline (`signal-card-underline`) sits at the bottom of the card, tinted by the card's signal color (`signal-warn`, `signal-alert`, or `primary`). The underline is also a progress bar: it fills as the metric approaches its threshold. Ambient — you don't notice it on first read — but it adds a second dimension of information without chrome.

**Graph node.** The 32px avatar disk reused as a graph node. Two independent axes:

- **Fill** = identity. Actor color + letter for claimed/active nodes. Neutral surface color for unclaimed. Muted gray for done (optionally with a subtle check glyph).
- **Ring** = status. A 2px outline colored by status. Selected nodes also carry a monospace ID pill immediately below; unselected labels appear on hover.

Edges: solid curves for parent/child, dashed curves for blocker relationships (overlay arcs on top of the tree layout). Graph direction is left-to-right; dependency flows rightward.

**Peek sheet.** Slides from the right at 440px wide at Level 3 elevation. URL state: `?preview=<task-id>`. Reloading the URL reopens the sheet. Escape key or clicking the dimmed overlay closes. The sheet contains the task's status, labels, notes (as code blocks), blockers, blocked-by, event history, and a notification bell. A prominent "Open full page" link navigates to `/tasks/<id>` for the full view.

**Scrubber pill.** Collapsed by default as a small `full`-radius pill in the footer showing `● Live` (small green dot + label). Clicking expands it into a horizontal bar with play/pause, ±1 event arrows, and a draggable cursor over an event-density minimap. When scrubbing, the main viewport dims to 0.85 opacity and a banner appears: *"Viewing history — return to live."*

**Buttons.** Two variants only.

- **Primary**: solid `primary` fill, `on-primary` text. Reserved for singular affordances where they exist.
- **Ghost**: transparent fill, muted text; hover brightens to `surface-raised` with full-color text. Use for secondary actions (theme toggle, dismiss, navigation affordances).

No tertiary, no destructive, no large/small variants unless a view genuinely needs them. This is a read-only UI; button surface area is intentionally minimal.

**Input (search).** The global search field sits in the header. Transparent background, hairline bottom border. Focus: border transitions to `primary` with a soft 2px outer ring. Placeholder text uses `on-surface-dim`. The "/" keyboard shortcut focuses it from anywhere.

**Data rows.** 36px tall, 12px horizontal padding. Monospace (`data-sm`) for IDs, timestamps, and numeric values; sans (`body-sm`) for titles, actor names, and labels. Hover state uses `surface-raised`. Selected rows carry a 3px vertical `primary` bar on the far left.

**ID pill.** A compact monospace chip used wherever a 5-character task ID appears. `surface-raised` background, `data-id` typography, `sm` radius. Auto-links to the task via the peek sheet.

**Label/tag pill.** Same shape as the ID pill but with `body-sm` typography. Deterministically colored per the label-identity rule in §Colors: a 15%-opacity fill and a full-chroma 1px outline in the label's hashed hue. Lower saturation than actor avatars so labels read as supporting metadata, not identity.

**Footer metric strip.** Thin horizontal bar (36px tall), persistent across every view. Left: metric cluster (active actors, WIP, events/min, throughput). Center: the scrubber pill. Right: heartbeat (small pulse dot + "last event Ns ago") + connection status (SSE connected / reconnecting / offline). The metric strip is the single place raw counts live — home-page cards carry signals, not restatements of these numbers.

**Notification bell.** Icon button in the header. On a task's peek sheet or detail page, the bell toggles a per-tab browser-notification subscription for that task's completion. Toggled state uses `primary` fill.

**Favicon.** Dynamic and readable at 16px. Idle = monochrome dot on background. Active = `primary` pulse. Stuck = `signal-warn` tint. Supports the pinned-tab use case.

**Timeline strip.** Swimlane visualization beneath the Actors view. One row per actor, time as x-axis, bars for claim→done spans colored by actor. Tied to the scrubber: dragging the scrubber moves the cursor along the timeline.

**Tabs (top nav).** `label-caps` typography. Active tab shows a 2px `primary` underline with a `full`-radius cap. Inactive tabs use `on-surface-muted` text; hover brings them to `on-surface`.

## Do's and Don'ts

- **Do** layer depth tonally. Reserve shadows for Level 3.
- **Do** pair every status color with an icon and a word.
- **Do** treat the avatar circle as the canonical actor primitive at every scale.
- **Do** keep fill and ring on separate semantic axes in graph nodes: fill = who, ring = what state.
- **Do** render all notes as code blocks (monospace, preserved whitespace).
- **Do** keep the footer thin and persistent on every view — it is the "is it alive" affordance.
- **Do** use the accent sparingly. A screen should rarely show more than a handful of `primary` instances.
- **Do** use LTR flow for graph views and TTB indented flow for the plan view. The difference reinforces what each view is *for*.

- **Don't** introduce sidebars. Top-nav only.
- **Don't** add write actions — no create, edit, claim, close, or new-job buttons anywhere. The dashboard is read-only.
- **Don't** mix `md` and `xl` corner radii in the same view.
- **Don't** let `status-blocked` amber and `signal-warn` orange sit adjacent. They are distinct semantic categories and must not read as synonyms.
- **Don't** encode meaning in color alone. Every color-coded state needs a glyph or a word.
- **Don't** use decorative motion. Transitions exist to confirm state changes — fast, ease-out, never longer than `duration-slow`.
- **Don't** use shadows to layer Level 1 or 2 surfaces.
- **Don't** default headings to full uppercase. `label-caps` is letter-spaced sentence case; reserve full uppercase for single-word anchors.
- **Don't** duplicate raw metrics between the header and the footer. Home-page cards show *signals*; the footer strip owns the counts.
- **Don't** version the UI with "v2.4.0-stable" chrome or label the dashboard with "SYSTEM OK" status. This is not a monitoring console.
