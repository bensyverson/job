# Web Dashboard Implementation Plan

**Date:** 2026-04-22\
**Scope:** v1 of `job serve` — the read-only live web dashboard.\
**Inputs:** [Vision doc](./2026-04-21-web-dashboard-vision.md), [DESIGN.md](../DESIGN.md).

---

## Approach

Phased delivery. Each phase is independently meaningful and mostly shippable on its own. The overall sequence goes:

1. **Static HTML prototype** — Home + Plan (document mode) + peek-sheet-open state, using embedded assets and DESIGN.md tokens. No server behind it yet. This is the design acceptance gate before committing to the full build.
2. **Server skeleton** — `job serve` command, `go:embed` asset pipeline, SSR of the first real view (Log — simplest layout, proves routing + templates + tokens).
3. **Live updates** — `/events` SSE endpoint backed by a 1-Hz poll-based broadcaster (same cadence as `job tail`), reusable by every live view.
4. **Plan view** — full document-mode implementation including collapse, labels filter, archive filter, peek-sheet entry point.
5. **Log view** — filters + live tail (promoted from skeleton phase).
6. **Home view** — signal cards with ambient progress bars, active claims, recent completions, blocked strip, mini-graph.
7. **Actors view** — column chat layout, live pinned updates. Timeline strip arrives with scrubber.
8. **Peek sheet** — progressive-enhancement JS intercepting task clicks, fragment endpoint, slide animation, `?preview=` URL state.
9. **Time-travel scrubber** — collapsed pill, expanded UI, replay buffer, dim-while-scrubbing banner. Brings timeline strip with it.
10. **Cross-cutting affordances** — global search, keyboard navigation, notification bell, dynamic favicon.
11. **Polish** — empty states, error/reconnect UX, accessibility sweep, mobile status view.

**Deferred to v2**: full graph view with semantic zoom, Plan view's table-mode toggle, light-mode theme, human annotations, print/export.

**Architecture lock-ins**:
- Server: `net/http` + `html/template` (stdlib only).
- Client: vanilla JS + WebComponents. No framework, no bundler, no npm. Peek sheet and scrubber use tiny progressive-enhancement modules; everything else degrades to full-page SSR.
- Assets: `go:embed` with system fonts (`system-ui`, `ui-monospace`) — no font files bundled.
- Default bind: `127.0.0.1:7823`.
- DB access: read-only, same `.jobs.db` as the CLI.
- Live updates: poll events table at 1 Hz, broadcast to SSE subscribers.

---

## Tasks

```yaml
tasks:
  - title: Web dashboard v1
    ref: web-v1
    desc: |
      Read-only live web dashboard for the Jobs task store. Bound to
      127.0.0.1:7823 by default. SSR + progressive enhancement, SSE
      for live updates, no JS framework. See project/2026-04-21-
      web-dashboard-vision.md and DESIGN.md for full spec.
    labels: [web, dashboard]
    children:

      - title: Phase 1 — Static HTML prototype
        ref: phase-1
        desc: |
          Build a static HTML prototype of the Home view and the Plan
          view (document mode) with a peek sheet shown in an open
          state. Embedded CSS uses custom properties translated from
          DESIGN.md tokens. System fonts only. Validates visual design
          in a real browser before committing to the full build.
        labels: [web, design, prototype]
        children:
          - title: Translate DESIGN.md tokens to CSS custom properties
            desc: Create assets/css/tokens.css mapping every color, typography, rounded, spacing, and motion token to CSS variables.
            labels: [web, css]
            ref: p1-tokens
          - title: Base stylesheet — layout, surfaces, typography, focus styles
            desc: assets/css/base.css. Body background, surface tiers, default heading and body styles, focus-ring rule, scrollbar styling for dark mode.
            labels: [web, css]
            ref: p1-base
            blockedBy: [p1-tokens]
          - title: Component stylesheet — avatars, pills, cards, rows, buttons, sheet, scrubber
            desc: assets/css/components.css. Every component spec'd in DESIGN.md §Components.
            labels: [web, css]
            ref: p1-components
            blockedBy: [p1-base]
          - title: Deterministic color helpers (actors, labels) as inline JS
            desc: |
              One small JS module exposing hash-to-HSL for actor names
              (S65 L55) and labels (S40 L50). Used by the prototype to
              color static elements; graduates to production use later.
            labels: [web, js]
            ref: p1-colors-js
          - title: Home view prototype HTML
            desc: |
              Static Home page with: header (tabs, search, bell, theme
              toggle), three signal cards with ambient progress
              underlines, active claims table, recent completions
              list, blocked strip, mini-graph placeholder (static
              illustration), footer metric strip with collapsed
              scrubber pill.
            labels: [web, prototype]
            ref: p1-home-html
            blockedBy: [p1-components, p1-colors-js]
          - title: Plan view prototype HTML (document mode)
            desc: |
              Static Plan page demonstrating document-style rendering:
              indented tree, status pills, actor avatars inline,
              updated timestamps, label pills with hashed colors,
              inline "Blocked by" sub-rows, collapsible affordances,
              keyboard shortcut hint in footer.
            labels: [web, prototype]
            ref: p1-plan-html
            blockedBy: [p1-components, p1-colors-js]
          - title: Peek sheet prototype HTML
            desc: |
              Static rendering of the peek sheet in its open state on
              top of the Plan view. Includes task status, labels,
              notes rendered as code blocks, blockers, event history,
              notification bell. Demonstrates the Level 3 elevation
              and dim-underlay treatment.
            labels: [web, prototype]
            ref: p1-peek-html
            blockedBy: [p1-plan-html]
          - title: Empty-state examples
            desc: |
              Show subtle empty states on Home (no active claims) and
              Plan (no tasks) in the prototype. One quiet line of
              muted text per empty region — not a blank rectangle.
            labels: [web, prototype]
            ref: p1-empty
            blockedBy: [p1-home-html, p1-plan-html]
          - title: Design review gate
            desc: |
              Open the prototype in a browser, walk through every
              state, capture any visual adjustments needed, iterate
              on tokens and CSS until aligned. Do not proceed to
              Phase 2 until the prototype is accepted.
            labels: [web, review]
            ref: p1-review
            blockedBy: [p1-home-html, p1-plan-html, p1-peek-html, p1-empty]

      - title: Phase 2 — Server skeleton + SSR foundation
        ref: phase-2
        desc: |
          Introduce `job serve`. Package layout under internal/web/.
          go:embed pipeline for HTML templates, CSS, and JS. Real
          routes, real HTML, no live updates yet. Ship the Log view
          as the first SSR target because its layout is the simplest.
        labels: [web, server]
        blockedBy: [p1-review]
        children:
          - title: internal/web package skeleton
            desc: |
              Sub-packages: internal/web/server (http.Server setup,
              routes), internal/web/templates (html/template funcs +
              embed), internal/web/assets (css/js/fonts embed),
              internal/web/handlers (one file per view), internal/
              web/render (shared render helpers, actor color fn).
            labels: [web, server]
            ref: p2-skeleton
          - title: job serve command
            desc: |
              cmd/job/serve.go — binds 127.0.0.1:7823 by default,
              --bind flag for other addresses (refuses 0.0.0.0 without
              explicit --bind 0.0.0.0:N). Foreground process. Prints
              the URL on startup. Graceful shutdown on SIGINT.
            labels: [cli, server]
            ref: p2-serve-cmd
            blockedBy: [p2-skeleton]
          - title: Asset embedding and serving
            desc: |
              go:embed all:internal/web/assets. Serve with
              http.FileServer under /static/. Long cache-control on
              hashed asset URLs.
            labels: [web, server]
            ref: p2-assets
            blockedBy: [p2-skeleton]
          - title: Template layout + shared chrome
            desc: |
              Base template with header, main, footer. Header
              template with tabs, search, bell, theme toggle. Footer
              template with metric strip and collapsed scrubber pill.
              Partial templates included by every view.
            labels: [web, templates]
            ref: p2-layout
            blockedBy: [p2-skeleton]
          - title: Log view SSR (initial render only, no live tail)
            desc: |
              GET /log renders the full current event list with server-
              side filters (query params: actor, task, label, type,
              since). Event rows use data-row component. Filter
              controls above the list.
            labels: [web, log]
            ref: p2-log-ssr
            blockedBy: [p2-layout, p2-assets]
          - title: Task detail SSR
            desc: |
              GET /tasks/<id> renders a full-page task detail. Same
              content as the peek sheet, full-bleed. Serves as the
              cmd-click target from Plan and as the fallback when
              progressive-enhancement JS is disabled.
            labels: [web, tasks]
            ref: p2-task-ssr
            blockedBy: [p2-layout]
          - title: 404 and error pages
            desc: Styled 404 and 500 responses using DESIGN.md tone. Quiet, not alarming.
            labels: [web, errors]
            ref: p2-errors
            blockedBy: [p2-layout]
          - title: Unit tests — routes, template rendering, asset serving
            desc: Go httptest coverage for each handler. Table-driven tests for filter parsing.
            labels: [web, tests]
            ref: p2-tests
            blockedBy: [p2-log-ssr, p2-task-ssr, p2-errors]

      - title: Phase 3 — Live updates via SSE
        ref: phase-3
        desc: |
          /events SSE endpoint backed by a 1-Hz poll-based broadcaster
          (same cadence as `job tail`). Event fanout to subscribers.
          Reconnect via ?since=<event_id>. Degrades to JSON array
          without Accept: text/event-stream. Serves both live updates
          and future time-travel replay.
        labels: [web, sse]
        blockedBy: [phase-2]
        children:
          - title: Broadcaster
            desc: |
              internal/web/broadcast. One goroutine polls the events
              table every 1s using the existing tail primitive. Fans
              out new events to a slice of subscriber channels.
              Thread-safe subscribe/unsubscribe.
            labels: [web, sse]
            ref: p3-broadcast
          - title: /events SSE handler
            desc: |
              GET /events with Accept: text/event-stream opens an SSE
              stream. Handshake replays events since ?since= (paged
              with ?limit=), then subscribes to the broadcaster for
              live tail. Filter params: actor, task, label, type.
            labels: [web, sse]
            ref: p3-sse
            blockedBy: [p3-broadcast]
          - title: /events JSON replay mode
            desc: |
              Same endpoint without Accept: text/event-stream returns
              a JSON array of events. Supports the same filter and
              pagination params. Documented as a stable API surface.
            labels: [web, api]
            ref: p3-replay
            blockedBy: [p3-sse]
          - title: Live-updates JS module
            desc: |
              assets/js/live.js — small WebComponent <live-region
              src="/events?..."> that subscribes to the SSE stream and
              fires DOM update hooks. Used by Log, Home, Actors.
              Handles reconnect, heartbeat, last-event-id restore.
            labels: [web, js]
            ref: p3-live-js
            blockedBy: [p3-sse]
          - title: Log view upgraded to live tail
            desc: New events stream into the top of the Log view via live.js. Filter state preserved on update.
            labels: [web, log]
            ref: p3-log-live
            blockedBy: [p3-live-js, p2-log-ssr]
          - title: Footer heartbeat live
            desc: |
              Footer shows "last event Ns ago" and connection state
              (connected / reconnecting / offline). Both update in
              real time via live.js.
            labels: [web, footer]
            ref: p3-heartbeat
            blockedBy: [p3-live-js]
          - title: Tests — broadcaster fanout, SSE handshake, reconnect
            desc: Unit tests for the broadcaster, integration tests for the SSE endpoint using httptest.
            labels: [web, tests]
            ref: p3-tests
            blockedBy: [p3-sse, p3-broadcast]

      - title: Phase 4 — Plan view (document mode)
        ref: phase-4
        desc: |
          Full Plan view: indented tree, document-style rendering,
          collapse/expand with localStorage persistence, labels
          filter (?label=), archive filter (?show=active|archived|
          all), live updates so newly added or completed tasks
          reshape the tree without reload.
        labels: [web, plan]
        blockedBy: [phase-3]
        children:
          - title: GET /plan — SSR indented tree
            desc: Recursive template rendering the full task tree in document mode. Auto-expand branches with non-done work, auto-collapse fully-done subtrees.
            labels: [web, plan]
            ref: p4-ssr
          - title: Collapse/expand with localStorage persistence
            desc: Small JS module managing per-user expand state. ?expand=<ids> overrides localStorage.
            labels: [web, plan, js]
            ref: p4-collapse
            blockedBy: [p4-ssr]
          - title: Labels filter
            desc: ?label=<name> scopes the tree to tasks matching the label, keeping ancestor chain visible for context.
            labels: [web, plan]
            ref: p4-labels
            blockedBy: [p4-ssr]
          - title: Archive filter
            desc: ?show=active (default) | archived | all. Derived from subtree completion state. Composes with ?at= for time-travel semantics.
            labels: [web, plan]
            ref: p4-archive
            blockedBy: [p4-ssr]
          - title: Plan view live updates
            desc: live.js wires the plan tree to the SSE stream so tasks added/claimed/closed reshape the view without reload.
            labels: [web, plan]
            ref: p4-live
            blockedBy: [p4-ssr, p3-live-js]
          - title: Keyboard navigation (j/k, space, arrows)
            desc: Vanilla JS — j/k move focus, space/enter toggles collapse, left/right collapse/expand. Focus ring uses primary color per DESIGN.md.
            labels: [web, plan, a11y]
            ref: p4-keyboard
            blockedBy: [p4-ssr]
          - title: Tests — recursive rendering, filter composition, archive semantics
            desc: Golden-file tests for rendering; table-driven tests for filter composition against time-travel.
            labels: [web, plan, tests]
            ref: p4-tests
            blockedBy: [p4-ssr, p4-archive]

      - title: Phase 5 — Home view
        ref: phase-5
        desc: |
          Home page landing view. Signal cards with ambient progress
          underlines, active claims table, recent completions list,
          blocked strip, compact mini-graph (LTR, avatar nodes,
          auto-collapse). Live via SSE.
        labels: [web, home]
        blockedBy: [phase-3, phase-4]
        children:
          - title: Signal card computation — server side
            desc: |
              internal/web/signals. Computes "idle actors >5min",
              "newly blocked in last 10m", "longest active claim",
              "oldest todo". Returns structured signal values with
              threshold info for the progress underlines.
            labels: [web, home]
            ref: p5-signals
          - title: Signal cards render + ambient progress bar
            desc: Four cards in a horizontal grid, each with its own signal color, icon, value, context line, and threshold progress underline.
            labels: [web, home]
            ref: p5-cards
            blockedBy: [p5-signals]
          - title: Active claims table (live)
            desc: Current claims with actor, idle timer, claimed-at. Live-updating rows via live.js.
            labels: [web, home]
            ref: p5-claims
            blockedBy: [p3-live-js]
          - title: Recent completions list (live)
            desc: Last N completions with actor attribution. Streams in via SSE.
            labels: [web, home]
            ref: p5-recent
            blockedBy: [p3-live-js]
          - title: Blocked strip
            desc: Tasks currently blocked, with linked "waiting on" tasks.
            labels: [web, home]
            ref: p5-blocked
          - title: Mini-graph — LTR avatar nodes
            desc: |
              SVG rendering of the dependency graph. LTR layout, tree
              layers for parent/child, dashed arcs for blockers.
              Nodes are avatar disks: fill = actor color + letter /
              neutral / muted gray; ring = status. Auto-collapse rules
              (done → check disk, upcoming → +N placeholder, active →
              expanded). Click a node opens peek sheet (wired in
              Phase 7).
            labels: [web, home, graph]
            ref: p5-graph
            blockedBy: [p1-colors-js]
          - title: Tests — signal computation, edge cases
            desc: Unit tests for the signal computations with synthetic event logs.
            labels: [web, home, tests]
            ref: p5-tests
            blockedBy: [p5-signals]

      - title: Phase 6 — Actors view
        ref: phase-6
        desc: |
          Column-per-actor chat-style layout. Bottom-pinned activity
          stream per column. Live via SSE. Timeline strip arrives
          with the scrubber in Phase 8.
        labels: [web, actors]
        blockedBy: [phase-3]
        children:
          - title: GET /actors — column layout SSR
            desc: One column per actor with recent activity. Actor column headers use avatar-lg. Chat-style, bottom-pinned list of claim/note/done events.
            labels: [web, actors]
            ref: p6-columns
          - title: GET /actors/<name> — single-actor view
            desc: Full-width view of one actor's activity and currently held claims.
            labels: [web, actors]
            ref: p6-single
          - title: Live updates
            desc: live.js wires the actor columns to the SSE stream.
            labels: [web, actors]
            ref: p6-live
            blockedBy: [p6-columns]
          - title: Tests
            desc: Handler tests for /actors and /actors/<name>.
            labels: [web, actors, tests]
            ref: p6-tests
            blockedBy: [p6-columns, p6-single]

      - title: Phase 7 — Peek sheet
        ref: phase-7
        desc: |
          Click a task anywhere to open a side-panel peek sheet. URL
          state ?preview=<id>. Vanilla JS interception; full-page
          /tasks/<id> remains the no-JS fallback and cmd-click target.
        labels: [web, peek]
        blockedBy: [phase-4]
        children:
          - title: /tasks/<id>/peek fragment endpoint
            desc: Returns the peek sheet body as an HTML fragment (no layout chrome).
            labels: [web, peek]
            ref: p7-fragment
          - title: Peek sheet WebComponent
            desc: |
              <peek-sheet> custom element with open/close, slide
              animation (motion.duration-base, ease-out), overlay dim,
              Escape-to-close. Reads ?preview= on load to auto-open.
            labels: [web, peek, js]
            ref: p7-component
            blockedBy: [p7-fragment]
          - title: Task-click interception (progressive enhancement)
            desc: |
              Small delegator: any <a data-peek href="/tasks/<id>">
              click triggers the peek sheet and pushState to
              ?preview=<id>. Cmd/ctrl-click passes through to native
              navigation. No JS → full navigation to /tasks/<id>.
            labels: [web, peek, js]
            ref: p7-intercept
            blockedBy: [p7-component]
          - title: Notification bell on peek sheet + task detail
            desc: |
              Toggle per-tab browser-notification subscription for
              this task's completion. Client-side only. Toggled state
              uses primary accent.
            labels: [web, peek, notifications]
            ref: p7-bell
            blockedBy: [p7-component]
          - title: Tests
            desc: Fragment-endpoint tests; manual browser verification of peek behavior documented in the PR.
            labels: [web, peek, tests]
            ref: p7-tests
            blockedBy: [p7-fragment, p7-component]

      - title: Phase 8 — Time-travel scrubber + timeline strip
        ref: phase-8
        desc: |
          Collapsed scrubber pill in the footer expands into a play/
          pause/drag control. Dragging the cursor sets ?at=<event_id>
          on the current view; view dims and a banner appears.
          Replay buffer fetches via /events?since=X&limit=N. Ships
          the timeline strip for the Actors view as part of the same
          phase because they use the same replay buffer.
        labels: [web, scrubber]
        blockedBy: [phase-3, phase-6]
        children:
          - title: ?at=<event_id> on every view (server side)
            desc: |
              Each view handler accepts ?at and renders the state of
              the world at that event. Implemented as a filter on
              event replay up to and including that event id.
            labels: [web, scrubber]
            ref: p8-at-server
          - title: Replay buffer client module
            desc: |
              assets/js/replay.js — fetches event windows around the
              cursor via /events?since=X&limit=N. Prefetches on drag.
              Drives the scrubber cursor and the views' DOM updates
              when in history mode.
            labels: [web, scrubber, js]
            ref: p8-replay
            blockedBy: [p3-replay]
          - title: Scrubber pill UI (collapsed + expanded)
            desc: |
              Collapsed: "● Live" pill in the footer. Expanded:
              horizontal bar with play/pause, ±1 event arrows,
              draggable cursor, event-density minimap. Dim the
              viewport to 0.85 and show "Viewing history — return to
              live" banner when active.
            labels: [web, scrubber, js]
            ref: p8-pill
            blockedBy: [p8-replay]
          - title: Timeline strip for Actors view
            desc: |
              Swimlane visualization under the actor columns. One row
              per actor, bars for claim→done spans. Shares the
              replay buffer so scrubber drag moves the timeline
              cursor and actor columns together.
            labels: [web, actors, scrubber]
            ref: p8-timeline
            blockedBy: [p8-replay, p6-columns]
          - title: Tests — ?at composition with filters, replay edge cases
            desc: Golden-file tests for ?at rendering; unit tests for replay buffer edge behavior.
            labels: [web, scrubber, tests]
            ref: p8-tests
            blockedBy: [p8-at-server, p8-replay]

      - title: Phase 9 — Global search, keyboard, favicon
        ref: phase-9
        desc: |
          Cross-cutting affordances: global search from the header
          ("/" to focus), keyboard shortcuts on every view, dynamic
          favicon reflecting overall state.
        labels: [web, polish]
        blockedBy: [phase-4, phase-5, phase-6]
        children:
          - title: Search handler and index
            desc: |
              Server-backed search across task titles, notes, labels,
              actor names. Returns results as a header-dropdown
              fragment. Each view's GET handler respects ?q=<query>
              so search acts as a lens on the current view.
            labels: [web, search]
            ref: p9-search
          - title: Header search UI + "/" shortcut
            desc: Input in the header, keyboard shortcut delegator, result dropdown.
            labels: [web, search, js]
            ref: p9-search-ui
            blockedBy: [p9-search]
          - title: Keyboard shortcuts global module
            desc: |
              Single delegator for j/k nav, space/enter, ?, g-then-
              letter view jumps. Page-specific handlers register with
              it.
            labels: [web, keyboard, js]
            ref: p9-keyboard
          - title: Dynamic favicon
            desc: |
              Small canvas-rendered favicon reflecting state: idle
              dot, active primary pulse, stuck signal-warn tint.
              Updates via live.js.
            labels: [web, favicon, js]
            ref: p9-favicon
            blockedBy: [p3-live-js]

      - title: Phase 10 — Polish, empty states, errors, accessibility
        ref: phase-10
        desc: |
          Final sweep. Real empty states across every view, graceful
          error recovery (connection lost, DB locked, unknown task
          id), accessibility audit (keyboard-only, screen-reader
          landmarks, status-pill ARIA), mobile single-column
          fallback.
        labels: [web, polish]
        blockedBy: [phase-7, phase-8, phase-9]
        children:
          - title: Empty-state polish across every view
            desc: Subtle one-line empty states everywhere, matching the prototype's tone. No blank rectangles.
            labels: [web, polish]
            ref: p10-empty
          - title: Connection recovery UX
            desc: |
              When SSE disconnects, show "reconnecting…" in the
              footer without alarming chrome. Auto-retry with
              exponential backoff up to a cap. Resume from last
              event id.
            labels: [web, polish]
            ref: p10-reconnect
          - title: Error pages for unknown tasks, DB locked, etc.
            desc: Quiet styled error pages that tell the user what happened without panic.
            labels: [web, polish, errors]
            ref: p10-errors
          - title: Accessibility sweep
            desc: |
              Keyboard-only walkthrough of every view. ARIA labels on
              status pills. Landmarks for header/main/footer.
              Screen-reader announcements for live-region updates
              (polite, not assertive).
            labels: [web, a11y]
            ref: p10-a11y
          - title: Mobile single-column fallback
            desc: |
              Home view degrades to a single column on narrow
              viewports showing only the signal cards and active
              claims. Task detail and peek work via full navigation.
              Not a primary target — just "is my agent stuck" from
              a phone.
            labels: [web, mobile]
            ref: p10-mobile
          - title: Documentation sweep
            desc: |
              README reference to `job serve`. docs/ entry for the
              dashboard with screenshots. Update AGENTS.md if agent-
              relevant patterns changed.
            labels: [web, docs]
            ref: p10-docs
          - title: Full pre-release verification
            desc: Run the whole test suite, walk every view at 5 k+ events, confirm performance targets (cold load <200ms, live update <100ms on localhost).
            labels: [web, tests]
            ref: p10-verify
            blockedBy: [p10-empty, p10-reconnect, p10-errors, p10-a11y, p10-mobile]
```

---

## Notes on parallelism

- Phase 1 (prototype) blocks everything; agents gate on the design review.
- After the server skeleton (Phase 2), Phases 3–7 have real parallelism: live updates (3), Plan (4), Home (5), Actors (6), Peek (7) each need the skeleton but can proceed on independent parts of the codebase. Blockers above reflect *data* dependencies, not serial ordering.
- Phase 8 (scrubber) genuinely depends on both live updates and actors.
- Phase 9 (cross-cutting) depends on having the main views in place.
- Phase 10 (polish) gates the release.

## Notes on TDD discipline

Per CLAUDE.md, follow strict red/green TDD for each handler, helper, and signal computation. Browser-level interaction testing is out of scope for v1 (no Playwright); rely on Go `httptest` for handlers and manual verification for progressive-enhancement behavior. Document the manual walkthrough for each visual phase in its PR description.
