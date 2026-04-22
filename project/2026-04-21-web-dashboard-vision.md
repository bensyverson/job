# Jobs Web Dashboard: Vision

**Date:** 2026-04-21
**Status:** Vision — pre-implementation
**Command:** `job serve`

---

## 1. Purpose

The CLI is built for agents first, humans second. The web dashboard is the inverse: it exists to give humans high-bandwidth, live observability into what their agents are doing, have done, and are stuck on. It is not a second interface for driving the task system — it is a window onto it.

When an agent fleet is grinding overnight, the dashboard is what you leave open on a second monitor. When something goes wrong, it's where you go to understand what happened and replay it.

---

## 2. Design Principles

1. **Local-first.** Binds to `127.0.0.1` by default. Single `.jobs.db` per server. No auth, no accounts, no remote access. Users who want to expose it can put Caddy in front.
2. **Live.** No refresh button. Updates arrive via Server-Sent Events tailing the event log. A footer heartbeat confirms the connection is alive.
3. **Read-only.** No writes to the task store from the dashboard. If a human wants to change something, they tell the agent. This keeps the agent's model of reality coherent with the store.
4. **SSR + progressive enhancement.** Pages render real HTML on the server at real URLs. WebComponents layer live updates on top. No SPA framework, no bundler runtime, no client router. Deep-linkable without trying.
5. **URL = state.** Every view, filter, and moment-in-time is expressible as a URL. `/actors/alice?at=<event_id>` renders exactly the state alice was in at that event. Shareable, bookmarkable, scrubbable.
6. **Desktop-first, mobile-graceful.** The density and multi-column layouts assume a wide screen. Mobile degrades to a single-column status view — enough to check "is my agent stuck" from a phone.
7. **Beautiful.** High-polish type, spacing, motion, and color. Dense without being cluttered. Dark mode by default (this is an always-open monitor tool). Visual inspiration TBD.
8. **Zero external runtime dependencies.** Templates and WebComponents ship with the Go binary. No npm, no CDN, no build step for the end user.

---

## 3. Views

### 3.1 Home: "Now" + live graph

The landing page answers "what is happening right now?" at a glance.

- Active claims with idle timers (per actor, with deterministic actor color).
- Recent completions in the last N minutes.
- Anything blocked waiting on something active.
- A compact live graph showing the dependency tree with active nodes lit up in their actor's color.

**Signal cards, not raw counts.** The home page should earn its prominence over the always-present footer metric strip (§4.2). Rather than restating "WIP: 14" / "Active actors: 6" (which the footer already shows), home cards highlight *signals* — things worth looking at:

- "3 actors idle >5min" — stuck signal, actionable.
- "2 tasks newly blocked in last 10m."
- "Longest active claim: `g7h8i` — 8m 47s."
- "Oldest todo: 2d."

Raw counts belong in the footer. Home cards answer "is anything wrong right now?"

**Ambient progress bars.** Each signal card carries a thin colored underline that doubles as a progress bar contextual to the card's signal. The "Longest active claim" card underlines fills as the claim age approaches a "stuck" threshold. "Idle actors" fills as the idle duration grows past the alert band. "Oldest todo" fills as the task's age grows. The bars are subtle — you don't notice them on first read — but they add a second dimension of at-a-glance information without adding chrome.

**Mini-graph: LTR, avatar nodes, auto-collapse.** The home graph uses the same primitives as the full graph view (see §8) at a fixed smaller size. Specifically:

- **Direction: left-to-right.** Dependencies flow rightward. Matches the phase-chain shape typical of real plans (Phase 1 → Phase 2 → … → Phase N), and rhymes with the timeline strip below the actors view. Plan view stays TTB-indented because it's an outline; graph views are LTR because they're flow.
- **Nodes are avatar circles** (same primitive as §5.1). Fill encodes identity:
    - Claimed / active → actor's hash color + first letter of actor name.
    - Unclaimed → neutral surface color, no letter.
    - Done → muted gray, no letter (or a subtle check glyph).
- **Ring encodes status**, independent of fill. One primitive, two axes.
- **Edges**: solid for parent/child hierarchy, dashed for blocker relationships (overlay arcs on top of the tree layout).
- **Labels** appear on the selected node (task ID pill near the node) and on hover for everything else. Keeps the default view uncluttered.

**Degradation by topological compression.** When the project grows, collapse rather than shrink:

- Fully-done phase subtree → collapses to a single checkmark disk.
- Not-yet-started phase subtree → collapses to a "+N" placeholder.
- Active phase subtree → expands to show current children, using the same collapse rules recursively.
- If top-level alone exceeds the space, show a summary and link to the full graph view.

Typical view: a chain of phase disks with the 1–2 currently active phases expanded — ~15 visible nodes, comfortable within the mini-graph's space.

"Full graph" is a separate v2 view with semantic zoom (see §8). The mini and full graphs share primitives (LTR, avatar nodes, ring-for-status, dashed-for-blocker) but use different scaling strategies: mini compresses topologically, full scales via zoom.

### 3.2 Plan

A hybrid of the Markdown-checklist and YAML formats:

- Nesting shown via indentation.
- Task hierarchy reinforced with heading sizes.
- Labels rendered as tag pills.
- Blockers linked inline.
- Status shown via **status pills** with icon + text + color (never color alone): todo = neutral, active = green, blocked = amber, done = muted/gray.

**Collapsible.** Every node with children can be collapsed/expanded.

- Default state: auto-expand branches containing any active/blocked/todo work; auto-collapse fully-done subtrees; everything else expanded.
- Persistence: per-user expand/collapse state in `localStorage`. For explicit sharing, `?expand=<ids>` overrides the stored state.
- Keyboard: space/enter toggles the focused node; left/right collapses/expands like a file tree.

**Archive filter.** "Archived" is derived, not stored: a top-level node whose entire subtree is done. It's a filter on the plan view, not a separate page.

- `/plan` (default) shows top-level nodes with any non-done work in their subtree.
- `/plan?show=archived` shows the complement: top-level nodes whose subtree is fully done.
- `/plan?show=all` shows both.
- Applies to top-level nodes only (v1). "Hide done leaves inside an active branch" is a possible v2 refinement, not a current need.
- Composes with time-travel: at `?at=<event_id>`, "archived" means "subtree fully done as of that event."

Scoping: `/plan`, `/plan/<task_id>` (subtree), `/plan?label=foo` (filter).

### 3.3 Actors + Timeline

Column view, one column per actor, chat-style bottom-pinned. Each actor's activity streams in chronologically: claim, note, done, release.

Below the columns sits a **timeline strip** — swimlanes with actors as rows, time as x-axis, bars for claim→done spans. The timeline is tied to the event scrubber (§3.6), so dragging the scrubber moves both the column view and the timeline through history together.

Scoping: `/actors`, `/actors/<name>`.

### 3.4 Log

Live event stream, filterable by task, actor, label, event type, time range. This is the power-user view — the one you reach for when something is weird.

Filters compose via query params: `/log?actor=alice&label=migration&since=<event_id>`.

### 3.5 Task detail and peek sheet

`/tasks/<id>` — the canonical page for a single task. Status, labels, notes (rendered as code blocks, see §6), blockers, blocked-by, full event history for this task, and a notification bell (§5).

**Peek sheet as the default task click behavior.** Clicking a task anywhere in the dashboard opens a side panel that slides in from the right with the task's details. The underlying view stays visible. URL state: `?preview=<id>` — so reloading or sharing the URL reopens the sheet in the same place. Esc closes it.

The sheet has an "Open full page" button that navigates to `/tasks/<id>` for the complete view. Cmd/ctrl-click on the task skips the sheet entirely and opens `/tasks/<id>` in a new tab — power-user shortcut.

Rationale: modals cramp rich content and don't compose with URLs; navigating to the detail page loses context. A peek sheet preserves context, stays deep-linkable, and offers escape hatches in both directions.

**Actor click** navigates directly to `/actors/<name>` — no peek sheet. An actor is itself a view, not a detail.

### 3.6 Time-travel / event scrubber

Every view accepts `?at=<event_id>` and renders the exact state of the world at that event. Sharing a URL with `?at=...` shares that frozen moment. Without it, views are live.

**Persistent scrubber UI.** A thin pill lives near the footer on every view, collapsed by default to show just "● Live". Clicking it expands into a full scrubber: play/pause, ±1 event arrows, draggable cursor, and a compact minimap of event density over time.

- **When scrubbing**, the entire view dims slightly and a banner appears: *"Viewing history — return to live"*. Visual commitment to "you are no longer in now."
- **Each view interprets the scrubber naturally.** Plan: tree morphs as tasks appear and close. Actors: columns and timeline strip scrub together. Log: cursor moves through events. Home: "Now" freezes to "then."
- **Client-side replay.** SSR renders the initial state; the scrubber mutates the DOM locally from a replay buffer prefetched around the cursor. This is the one place where client-side JS does real work.
- **Buffer sizing.** The client fetches a window of events around the cursor via `/events?since=X&limit=N`. Dragging near the edge prefetches more. Same endpoint that serves live updates — §6.3.

---

## 4. Cross-cutting UI

### 4.1 Header: search

Global search bar (`/` to focus). Searches across task titles, notes, labels, and actor names. **Searching a label transforms the current view into a label-filtered version of itself** — searching "migration" on the plan view filters the plan, on the log view filters the log, etc. Search is a lens, not a separate destination.

### 4.2 Footer: metrics + heartbeat

Thin strip across the bottom of every view:

- Live metrics: active actors, WIP, events/min, throughput.
- Heartbeat: "last event 3s ago" with a pulse dot — the "is it alive" affordance.
- Connection status (SSE connected / reconnecting / offline).

### 4.3 Notification bell

Every task detail page has a bell. Clicking it subscribes that browser tab to that task's completion event via the Notification API. Entirely client-side — no server-side user state, no accounts. The subscription dies when the tab closes. Simple, zero infrastructure, exactly the right amount of feature.

### 4.4 Favicon as status

Dynamic favicon reflects overall state: idle, active (N agents working), stuck (something idle-claimed beyond threshold). Makes a pinned tab useful peripheral vision.

---

## 5. Visual Language

### 5.1 Actor identity: color + avatar

**Deterministic actor color.** Hash actor name → stable color. Same "alice" is always the same color across every view, every session, every user. Use HSL with:

- **H** from the hash (full 360° range).
- **S** fixed around 60–70% (saturated enough to read, not garish).
- **L** constrained to a band that maintains contrast against both light and dark backgrounds (roughly 45–60%).

This guarantees that no actor renders as light yellow on white or dark blue on black.

**Actor avatar as the canonical primitive.** Every representation of an actor in the UI is a size of the same atom: a circular disk filled with the actor's hash color, containing the first letter of the actor's name. This atom is used everywhere actors appear — never naked names, never naked dots unless space is truly critical.

Sizes:

- **4–8px** (bare dot): tiny inline indicators where even a letter is too much.
- **20px** (small avatar): next to names in tables, logs, event rows.
- **32px** (medium avatar): column headers in the Actors view, task cards in the graph.
- **Avatar stack/group**: when multiple actors have touched a task, a horizontally overlapped stack.

One atom, many sizes — this is what gives the dashboard visual cohesion.

### 5.2 Status pills

Icon + short text + color, always all three. Never color alone — this is both accessibility and at-a-glance clarity.

### 5.3 Typography and density

Dense but breathable. A monospace for IDs, timestamps, and event payloads. A humanist sans for task titles, notes, and chrome. Generous line-height in notes, tight line-height in lists and timeline strips.

### 5.4 Dark mode default

This is a tool you leave open. Dark mode default, light mode available, respects `prefers-color-scheme` on first load.

### 5.5 Note rendering

All notes render as **code blocks** — monospace, preserved whitespace, horizontal scroll for long lines. Markdown will look mostly fine, diffs will look reasonable, stack traces will look like stack traces. This is the simplest rule that handles every realistic input an agent might emit.

Exception: auto-linkify inside notes.

### 5.6 Auto-linkify

Inside titles and notes, detect and link:

- Task IDs (5-char base62 → `/tasks/<id>`).
- URLs (`http(s)://...`).
- Relative file paths (open in the configured editor via the `file://` protocol or a custom scheme if we want).
- Actor names prefixed with `@` → `/actors/<name>`.

Makes the dashboard feel like live hypertext rather than a log viewer.

---

## 6. Architecture

### 6.1 Stack

- **Server:** Go, standard library `net/http`, `html/template` for SSR.
- **Live updates:** Server-Sent Events (`/events` endpoint).
- **Client:** WebComponents + vanilla JS. No framework, no bundler, no npm.
- **Storage:** The same SQLite `.jobs.db` the CLI uses. Read-only access.
- **Routing:** Real server routes. No client-side router.

### 6.2 Live updates via SSE

SSE is the right primitive for this: read-only, unidirectional, reconnects for free, survives proxies, no protocol overhead.

The server tails the events table. Each new event is pushed to connected clients as an SSE message. Clients subscribe with `?since=<event_id>` so reconnects resume cleanly.

### 6.3 `/events` as a public API

Because the SSE endpoint is stable and useful, we design it as a documented API from day one. Other tools (terminal TUIs, Slack bots, editor integrations) can consume it.

Proposed shape:

```
GET /events?since=<event_id>&limit=<n>&actor=<name>&task=<id>&label=<l>&type=<t>
```

- Returns a stream of `data: {json}\n\n` SSE frames, one per event.
- With `limit` and no `Accept: text/event-stream`, degrades to a normal JSON array — replay mode.
- With `Accept: text/event-stream`, streams live — initial backfill from `since` through current, then live tail.
- Filters are AND-composed, repeatable where they make sense (`&label=a&label=b`).
- Each frame includes the full event: id, timestamp, actor, task_id, type, payload.

This doubles as the solution to the 100-agent backlog problem: clients that want "summary then tail" use `since=<recent>` and get a small backfill; clients that want full history page through with `limit`.

### 6.4 URL map

```
/                           Home: Now + mini-graph
/plan                       Plan view (full tree)
/plan/<id>                  Plan subtree rooted at <id>
/actors                     All actors (columns + timeline)
/actors/<name>              Single actor
/tasks/<id>                 Task detail
/log                        Event log
/events                     SSE endpoint (and JSON replay)
/labels/<name>              Label view (alias for filtered plan)
```

All views accept:
- `?at=<event_id>` — time-travel to that moment.
- `?label=<name>` — scope to a label.
- `?actor=<name>` — scope to an actor.
- `?q=<query>` — text search.

### 6.5 Performance targets

- **Cold load** of any view under 200ms on a project with 10k events.
- **Live update latency** under 100ms from event write to DOM update on localhost.
- **100-agent stress target**: dashboard remains usable with 100 concurrent actors and a growing event stream. Achieved via paginated backfill and virtualized lists, not raw throughput.

### 6.6 Default port

`127.0.0.1:7823`. Unused by any major dev tool we know of. (Mnemonic: T9-ish "JOB" ≈ 562, but 7823 scans cleaner; open to alternatives.) Never binds `0.0.0.0` without an explicit `--bind` flag.

---

## 7. What the dashboard is NOT

- **Not a write interface.** No creating, editing, claiming, or closing tasks from the web. Tell the agent.
- **Not multi-tenant.** One server, one DB, one user.
- **Not a replacement for the CLI.** Agents continue to use the CLI. The dashboard is for humans.
- **Not a daemon.** `job serve` runs in the foreground like every other `job` command.
- **Not authenticated.** Local-first means trust the local machine.

---

## 8. Deferred / v2

- **Full graph view.** The mini-graph on home handles the at-a-glance case. A full interactive DAG view — zoomable, clickable nodes, filterable edges — is a v2 feature. When we build it:
  - **LTR layout** for the same reasons as the mini-graph: dependency flow reads rightward, phase chains lay out cleanly across dashboard width, and the timeline view rhymes with it. Plan stays TTB-indented; graph stays LTR.
  - Use the parent/child hierarchy for layout (tree layers, trivial to compute). Draw blocker relationships as overlay arcs. Avoids pulling in a graph-layout dependency.
  - **Semantic zoom**: far out = avatar dots colored by actor/status; mid = dots with labels; close = full cards with notes preview. Same graph, different detail at different zoom levels — how Figma/Miro/Kibana handle unbounded node counts.
  - **Node primitives and color system** match the mini-graph (§3.1): avatar-disk nodes with fill = identity, ring = status, dashed edges = blockers, solid edges = parent/child. The only difference between mini and full is scale behavior (topological compression vs semantic zoom).
  - **Fanout collapse**: if a node has more than N direct children, group the overflow into a "+M more" placeholder that expands on demand. Same collapse idea as the plan view, applied to graph siblings.
- **Human annotations.** The event store already supports append-only notes, so when we want humans to annotate tasks without rewriting them, we add a new event type (or a human-actor note) and render annotations in the UI. No architectural changes needed now.
- **Print/export.** A "snapshot this view as standalone HTML" export for post-mortems and PR attachments. Cheap given SSR.
- **Writes.** If we ever reopen this question, the first candidate is annotations, not edits. Edits should stay out.

---

## 9. Open questions

1. **Notification scope.** Should the bell support subscribing to more than task-complete? E.g., "notify me when anything blocked on X unblocks," "notify me when actor Y goes idle for 10 min." The infrastructure is the same; it's a question of how much UI surface we want.
2. **Time-travel granularity.** Scrub by event, by minute, or both? Events give precision; minutes give intuition.
3. **Editor integration for auto-linkified paths.** `file://` URLs don't open in editors by default. Do we invent a `job://` scheme, shell out to `$EDITOR`, or leave it as copy-to-clipboard?
4. **Theme beyond dark/light.** Is there appetite for user-tunable accent colors, or does the deterministic actor palette + one accent cover it?
5. **Mobile status view specifics.** What's the minimum-viable mobile view — home only, or also task detail for following a bell notification?

---

## 10. Next steps

1. User reviews this vision doc; provides visual inspiration.
2. Converge on MVP scope: which views ship first, which wait.
3. Spec the `/events` API formally (once MVP views are fixed — the API should be shaped by its consumers).
4. Prototype the SSR + WebComponents skeleton with one view (probably Home or Log).
5. TDD implementation following the phased plan pattern used elsewhere in `project/`.
