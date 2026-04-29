# `job` CLI experience report — 2026-04-28

Date: 2026-04-28
Author: Claude (in-session, mid-implementation of the subway row-merging redesign `wQtfX`)
Scope: ergonomics observed while driving a multi-step TDD implementation through the `job` CLI as my primary task tracker. Not a comprehensive review — just what I bumped into.

## What worked

The model fits how I actually plan and execute. A few things stood out as wins I'd keep untouched.

- **`job status` as the session opener earns its keep.** "31 open, 255 done" + per-root rollup + `Next:` + `Stale:` lands in one short paragraph. It's fast, dense, no scrolling, and it answers "what should I be doing?" without me having to compose anything. Don't let it grow into a dashboard.
- **Parent auto-close on the last child completing was a delightful surprise.** Closing `lqB30` and seeing `Auto-closed: wu1oF "S1: Single-focal preorder window mode"` come back in the same output is exactly the right amount of signal — I didn't have to remember to walk back up the tree, and the cascade was visible without being noisy.
- **The cluster rollup line (`Web dashboard v1 (bYr6R): 119 of 149 done · next mdYup`) is a great compression.** It's the smallest unit of "where the project is," and surfacing it on every `job status` keeps cross-session continuity tight without me having to ask.
- **Notes as append-only timeline matches how real handoffs accumulate.** When I left a note for the next agent on `WIEGQ`, the timestamp + author header was enough context that a fresh agent could read just that one task and know where to start. The append-only-ness is a feature for audit, not a bug.
- **Claim expiry (30m) is a reasonable default.** I never had to think about it during active work, and it nudges me to actually finish or release rather than abandoning a claim. I'd notice if it disappeared.

## Friction I hit

These are small papercuts, but I hit several of them more than once in a single session.

- **`-m` is required even when a positional `<text>` argument would be the obvious shape.** `job note WIEGQ "long text"` failed with `unexpected argument`; had to re-issue as `job note WIEGQ -m "..."`. `git commit`, `gh issue comment`, and similar peer CLIs accept the bare positional. I tripped on this twice in one session — first with `note`, then again later when I instinctively reached for the same shape elsewhere.
- **`job done` accepts `-m`, but `job release` doesn't.** I tried `job release WIEGQ -m "..."` first and got `unknown shorthand flag: 'm'`. Forced me into two commands (`job note` then `job release`) when I actually wanted "release this and capture why." `done` and `release` are sibling lifecycle verbs; the flag surface should match.
- **`job claim <parent>` rejects with the right reason but doesn't list the leaves.** The error says `run 'next wQtfX all' to see them`, which is the right pointer, but it forces a round-trip. A one-line `available leaves: dNY9e, ...` baked into the error would let me claim the right thing on the next command instead of the one after.
- **`job log <id>` doesn't include notes; `job show <id>` does.** I went looking for a note I'd written via `log` (assumed it was the full audit trail) and got only the event stream. Either fold notes into `log`, or make the split documented and obvious — right now you discover it by trial.
- **Task descriptions are stable but not editable in-flight.** When I realized `WIEGQ`'s description was lighter than what a fresh agent would need to hand off, I had no `job edit <id> --desc` to upgrade it. I appended a note instead, which works but is timestamp-noisy and the "stable handoff context" lives in two places (description + latest note). For a long-lived task whose description should evolve as understanding deepens, an editable description would be cleaner than the note workaround.
- **The error wording on `job claim` for a parent is technically correct but slightly unhelpful in tone.** "task wQtfX has 3 open children; claim a leaf instead" reads as a refusal rather than a redirect. Pairing the refusal with a positive next-step ("here are the leaves you can claim") would land softer and save a step.

## Things I want but didn't try to build around

These weren't blockers, just gaps where I noticed I would have used a feature if it existed.

- **A way to attach a stable "handoff brief" to a task that survives multiple notes.** Today the description is the closest thing, but it's not editable. The next agent on `WIEGQ` reads notes (newest-first), and as more notes accumulate, the original handoff brief gets buried.
- **`job claim --next`** that picks the next available leaf under whatever I'm currently working on. I found myself running `job status`, copying a short ID, then `job claim <id>` — three steps where one would do for the common case.
- **An indication of "this task has notes" in `job status` rollups.** Notes are easy to write but easy to miss reading; surfacing a `(2 notes)` marker on Next/Stale lines would make the timeline more discoverable.

## Suggested changes

The grammar below uses the `job import` schema (run `job schema`). It's a YAML block I'd expect `job import` to ingest as a coherent set of CLI improvements; titles complete "This task…" in present tense per the project commit style.

```yaml
tasks:
  - title: Improve job CLI flag-parsing consistency and handoff ergonomics
    desc: |
      Umbrella task gathering CLI papercuts surfaced during the 2026-04-28
      subway row-merging implementation session. See
      project/2026-04-28-job-cli-experience-report.md for the full report.
    labels: [cli, dx]
    ref: cli-experience-2026-04-28
    children:
      - title: Accept positional message text on `job note` (alongside -m)
        desc: |
          `job note WIEGQ "long text"` currently fails with `unexpected argument`,
          forcing the user to re-issue with -m. Match peer CLIs (git commit, gh
          issue comment) by accepting a bare trailing positional as the message
          when -m is absent. Keep -m and stdin support as-is.
        labels: [cli, dx]
      - title: Accept -m on `job release` (parity with `job done`)
        desc: |
          `job done <id> -m "..."` works; `job release <id> -m "..."` fails with
          `unknown shorthand flag: 'm'`. When a user is releasing a claim with
          context worth capturing ("pausing here, here's why"), the natural shape
          is one command. Lifecycle verbs `done` and `release` should share a
          flag surface; add -m to `release` and have it append a note before
          releasing the claim.
        labels: [cli, dx]
      - title: List open leaves in the `job claim <parent>` rejection message
        desc: |
          When the user runs `job claim` against a non-leaf, the current error
          ("task wQtfX has 3 open children; claim a leaf instead, or run 'next
          wQtfX all' to see them") points at the right next step but forces a
          round-trip. Inline a one-line summary of the available leaves so the
          user can claim correctly on their next command rather than the one
          after. Cap the inline list to keep the error compact.
        labels: [cli, dx]
      - title: Fold notes into `job log <id>` output by default
        desc: |
          `job log` currently shows the event stream only; notes live on
          `job show`. A user who reaches for `log` for the audit trail discovers
          this only by trying it. Either inline notes into `log` chronologically
          (timestamped, distinct from events), or rename the existing `log`
          output to `events` and have `log` mean "everything that happened on
          this task." Pick one and document it.
        labels: [cli, dx]
      - title: Add `job edit <id> --desc` to update a task description in flight
        desc: |
          Descriptions are gold for handoff context but currently can't evolve
          as understanding deepens. The workaround — append a note — splits the
          stable handoff brief from the original description and is timestamp-
          noisy. Add `job edit <id> --desc <text>` (or --desc-stdin) that
          replaces the description outright; record the previous description in
          the event log so the change remains auditable.
        labels: [cli, dx]
      - title: Add `job claim --next` to pick the first available leaf
        desc: |
          The common loop is `job status` → eyeball Next → `job claim <shortid>`.
          Collapse it: `job claim --next` claims the same leaf `job status`
          would surface as Next. Optionally accept a parent argument
          (`job claim --next <parent>`) to scope the pick to a subtree.
        labels: [cli, dx]
      - title: Surface "has notes" markers on `job status` rollup lines
        desc: |
          Notes are easy to write but easy to miss reading. On the Next: and
          Stale: lines (and possibly per-cluster rollups), append a small
          `(N notes)` marker when the task carries notes — so a user opening a
          session sees that a prior agent left handoff context worth reading.
          Keep the marker compact; the goal is discoverability, not detail.
        labels: [cli, dx]
```
