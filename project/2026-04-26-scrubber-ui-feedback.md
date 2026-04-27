# Scrubber UI feedback — 2026-04-26

Manual session against the dashboard scrubber. Seven items; two are real bugs, one is already wired, the rest are missing features.

## Findings

1. **History banner & strip ignore the `hidden` attribute.** `.c-history-banner` and `.c-scrubber-strip` both set `display: flex` (components.css:1258, :1446). Class selector beats the UA `[hidden] { display: none }`, so both elements show in live mode despite the `hidden` attribute on the markup (layout.html.tmpl:17, footer.html.tmpl:4). Fix with a global `[hidden] { display: none !important; }` in base.css.
2. **Timeline always visible.** Same root cause as #1, plus the design change below: the strip should be gated behind a "Time Travel" button rather than always co-visible with the pill.
3. **Cursor dot at the bottom.** components.css:1366-1377 — `.c-scrubber-strip__cursor::after` uses `bottom: -4px`. Flip to `top: -4px`.
4. **Arrow-key step nav not wired.** footer.html.tmpl:8 advertises "←/→ to step" but there's no handler. plan-keyboard.js owns its own arrow keys for the Plan tree, which would conflict if we don't stop propagation.
5. **Pan not implemented.** scrubber-cursor.mjs hardcodes `windowMs = ONE_DAY_MS` with no viewport offset.
6. **Zoom not implemented.** Same constraint.
7. **Esc to cancel** — already wired at scrubber-pill.mjs:250-252.

## Decisions

- Strip stays in the layout; transition with `transform`/`opacity` so screen readers see it only when expanded — toggle `aria-hidden` and `inert` (or the `hidden` attribute) alongside the visual transition so AT users get the same gate.
- Pan and zoom extend `scrubber-cursor.mjs` with explicit `windowStartMs` and `windowMs` parameters (default to current behavior). Math stays pure; UI state lives in scrubber-pill.mjs.

## Plan (`job` YAML)

```yaml
tasks:
  - title: Scrubber UI fixes — visibility, controls, navigation
    desc: |
      Bundle of UI fixes on the time-travel scrubber identified on 2026-04-26. See project/2026-04-26-scrubber-ui-feedback.md for findings.

      Ship in any order; #1 (visibility) and #3 (cursor dot) are one-line fixes worth doing first because they make manual testing of the rest sane.
    labels: [web, scrubber, ux]
    children:
      - title: "Fix: history banner & strip ignore the `hidden` attribute"
        ref: scrubber-hidden-bug
        labels: [web, scrubber, css, bug]
        desc: |
          `.c-history-banner` and `.c-scrubber-strip` both set `display: flex`, which overrides the UA stylesheet's `[hidden] { display: none }` because the class selector has higher specificity. Result: both elements show in live mode despite the `hidden` attribute.

          Fix: add a global utility rule `[hidden] { display: none !important; }` in base.css. Protects every future component too.

          Regression test: a JSDOM unit test that loads layout + components.css and asserts `.c-history-banner[hidden]` computes `display: none`.

      - title: "Move cursor dot from the bottom of the time cursor to the top"
        ref: scrubber-cursor-dot-top
        labels: [web, scrubber, css]
        desc: |
          components.css:1366 — `.c-scrubber-strip__cursor::after` uses `bottom: -4px`. Flip to `top: -4px` (and remove the `bottom` property) so the grip sits at the top of the cursor line.

          Manual verify: drag the cursor; the dot should be flush with the top of the strip's track area.

      - title: "Rename Live pill to Time Travel; gate the strip behind its toggle"
        ref: scrubber-time-travel-toggle
        labels: [web, scrubber, ui]
        desc: |
          Today the footer pill reads "Live" with a green dot. It should read "Time Travel" with a back-arrow icon when in live mode; clicking it reveals the strip + cursor with a CSS transition (slide up from the bottom). When in scrubbing mode, the pill becomes "Return to live" with the pulsing green dot (i.e. the current "Live" pill styling).

          Remove the in-strip "Return to live" button (footer.html.tmpl:10-13) — the pill carries that state now.

          Strip stays in the layout always; the enter/exit transition is `transform: translateY(...)` + `opacity`, ~180ms, matching `--motion-duration-base`. Toggle `aria-hidden`/`inert` (or the `hidden` attribute) in lockstep with the transition so screen readers see the strip only while scrubbing.

          Wiring: scrubber-pill.mjs already toggles `page--scrubbing` on `.page` and sets `[data-scrubber-pill-label]`. Extend it to swap a back-arrow ↔ green-dot icon and update the button label between "Time travel" and "Return to live".

      - title: "History banner appears only while scrubbing"
        ref: scrubber-banner-only-when-scrubbing
        labels: [web, scrubber, ui]
        desc: |
          Once #scrubber-hidden-bug lands, the existing enterScrubbing/exitScrubbing logic should already produce the right behavior. Add a JSDOM test that toggles each and asserts the banner's `hidden` attribute flips correctly.

      - title: "Wire ←/→ arrow keys to step the cursor by one event"
        ref: scrubber-arrow-keys
        labels: [web, scrubber, keyboard]
        desc: |
          footer.html.tmpl:8 advertises "Drag the cursor or ←/→ to step." but no handler exists. Add to scrubber-pill.mjs:

          On keydown when `isScrubbing(doc)`:
            - ArrowLeft  → seek to events[currentIndex - 1]
            - ArrowRight → seek to events[currentIndex + 1]
            - Stop propagation so plan-keyboard.js doesn't also act on the same key while the Plan view is scrubbing.

          The current cursor's eventId is derivable from the banner state or the URL `?at=`. Cache the index in module state next to `inFlightX`.

          Tests: extend the scrubber-pill tests with synthesized KeyboardEvents and assert the cursor advances/retreats.

      - title: "Pan: hold Space (or Alt/Option) and drag to move the visible window"
        ref: scrubber-pan
        labels: [web, scrubber, interaction]
        desc: |
          Today the strip is a fixed 24h window. Add a pan offset so users can shift the visible range earlier or later.

          UX:
            - Hold Space or Alt/Option → cursor turns to a grab hand on the track.
            - Drag → translates the window by N ms; the cursor stays where it was in event-space (not screen-space).
            - Release → window stays panned until reset.

          Implementation: add `panOffsetMs` to scrubber state. `xToEventId` / `eventIdToX` / `computeDensityBars` take an optional `windowStartMs` (default `nowMs - windowMs`); panning shifts that start. Math module stays pure.

          Out-of-scope for this child: kinetic / inertial scrolling. Linear pan only.

          Tests in scrubber-cursor.test for the new `windowStartMs` plumbing; manual verify pointer interaction.

      - title: "Zoom: scrollwheel over track (and Alt+←/→) to change window length"
        ref: scrubber-zoom
        labels: [web, scrubber, interaction]
        desc: |
          Add a `windowMs` zoom level (currently constant `ONE_DAY_MS` in scrubber-cursor.mjs).

          UX:
            - Scrollwheel over the track → multiply `windowMs` by 0.9 (zoom in) / 1.1 (zoom out) per wheel tick, centered on the cursor (or on the mouse pointer when not on the cursor).
            - Alt/Option + ←/→ → same effect via keyboard, fixed steps (e.g. ×0.5 / ×2).
            - Clamp to `[60_000ms, 30 * ONE_DAY_MS]` so the zoom can't collapse to nothing or run infinitely wide.

          Rebuild density bars and the time-axis labels on each zoom change. The axis currently hard-codes 24h/18h/12h/6h/now in the template — switch to JS-driven labels derived from `windowMs`.

          Tests for the zoom math (clamping, center-anchored zoom preserving the cursor's event-id) in scrubber-cursor.test.

      - title: "Verify Esc cancels scrubbing (already wired); add a test if missing"
        ref: scrubber-esc-test
        labels: [web, scrubber, test]
        desc: |
          scrubber-pill.mjs:250-252 already calls `exitScrubbing` on Escape. If there's no JSDOM test covering this branch, add one. Otherwise close as already-done.
```
