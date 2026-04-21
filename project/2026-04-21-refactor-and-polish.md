# Refactor + Polish: Post-P1 Follow-Ups

This is the second plan doc following the 2026-04-21 Opus feedback work.
The [first plan](2026-04-21-implement-opus-feedback.md) landed the
leaf-frontier semantics and UX polish (commits `3f04659`, `c750171`).
This plan covers a package-layout refactor, a real migrations system,
and five targeted polish items surfaced by the dogfooding session.

## Source of motivation

After implementing P1-P5 of the prior plan, we captured a set of
follow-ups in the post-session review:

- **Package refactor.** The repo is currently `package main` with
  ~30 flat `.go` files. We're about to add a `job serve` web view, and
  a shared domain package will make the CLI + HTTP server cleanly
  share types. Doing the refactor first avoids re-plumbing.
- **Migration system.** CLAUDE.md references `internal/migrations/`
  but it doesn't exist. The stale `users.key` schema bump at the start
  of the P1 session cost us five minutes and will recur every time the
  schema changes until a real runner exists.
- **Default identity + `--strict` mode.** The `--as` tax is real but
  "drop attribution" is wrong for multi-agent. Resolution (discussed
  post-session): permissive by default — `init` fills a default
  identity into the DB; `--as` still overrides; multi-agent setups
  opt into strictness via `init --strict` / `identity strict on`.
- **Cancel cascade.** Done cascades upward when the last open child
  closes, but cancel does not. Under "open = not done AND not canceled",
  the two should be symmetric. Destination status depends on sibling
  mix: any sibling done → parent cascades to `done`; zero siblings
  done → parent cascades to `canceled`.
- **`open children` SQL predicate is copy-pasted** across four query
  sites (`countOpenChildren`, `queryAvailableLeafFrontier`,
  `cascadeAutoCloseAncestors`, list filter). A shared helper or
  package const prevents drift.
- **`status` should show the caller's current claim.** During the
  P1 session I lost track of which ID I was holding and had to
  re-scroll. `status` should include a `N claimed, M open, P done`
  count, with `claimed` suppressed when zero.
- **Done ack output has three tightenable redundancies** (latent bugs
  and duplication I spotted during P1). Fixing them cleanly is the
  user-visible reason to also refactor `renderDoneAck` into an
  `[]AckLine` render plan.

The `job serve` web view is **deferred** — design/layout to be discussed
separately once this lands. The refactor reserves `internal/server/` as
a stub so the server lands on a clean structure.

## Plan

```yaml
tasks:
  - title: Refactor + polish (post-P1)
    ref: root
    desc: |
      Seven phases, top-down. P1 is mechanical (no behavior change,
      existing test suite is the contract). P2-P7 follow strict
      red/green TDD. P8 is the docs sweep.
    labels: [refactor, dx]
    children:

      # ---------- Phase 1: Package refactor ----------
      - title: "P1: Package refactor"
        ref: p1
        desc: |
          Move from flat `package main` to:
            cmd/job/              — main.go + one file per verb (cobra wiring)
            internal/job/         — domain: Task, runs (Add/Done/Claim/Cancel),
                                    queries, renderers, events
            internal/server/      — stub package for upcoming `job serve`
            internal/migrations/  — (empty, populated by P2)

          Purely mechanical: no behavior changes, no new tests.
          Contract: the existing full test suite passes unchanged after
          the move.
        children:
          - title: "P1a: Move domain files to internal/job/"
            ref: p1a
            desc: |
              Move tasks.go, claims.go, blocks.go, cancel.go, events.go,
              models.go, users.go, database.go, format.go, labels.go,
              heartbeat.go, gitignore.go, import.go, schema.go, status.go,
              and their _test.go pairs into internal/job/. Change package
              declaration to `package job`. Export what the cobra layer
              needs (type capitalization).
          - title: "P1b: Move cobra wiring to cmd/job/"
            ref: p1b
            blockedBy: [p1a]
            desc: |
              Move commands.go's newXCmd constructors into cmd/job/, one
              file per verb (add.go, done.go, claim.go, ...). cmd/job/main.go
              holds newRootCmd and main(). All call sites delegate to
              internal/job/.
          - title: "P1c: Stub internal/server/"
            ref: p1c
            blockedBy: [p1b]
            desc: |
              Create internal/server/server.go with `package server` and a
              single `// New serves the job web view — stubbed.` comment.
              Ensures the package exists for P2+ imports and signals intent.
          - title: "P1d: Verify contract"
            ref: p1d
            blockedBy: [p1c]
            desc: |
              go fmt, go vet, go build, full test suite pass. No new or
              changed tests — if the move broke something, we fix the move
              (not the tests).

      # ---------- Phase 2: Migration system ----------
      - title: "P2: Forward-only migration system"
        ref: p2
        blockedBy: [p1]
        desc: |
          Forward-only SQL migrations, numbered files, tracked in a
          schema_migrations table, applied automatically on openDB.
          No down migrations, no auto-generation — just enough to stop
          the stale-schema footgun.

          Runner invariants:
            - schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER)
            - Baseline migration 0001_initial.sql captures the current schema
            - Subsequent migrations NNNN_description.sql applied in filename order
            - Each migration runs in its own tx; failure aborts startup
            - Idempotent: running on an up-to-date DB is a no-op
        children:
          - title: "P2 red: migration runner tests"
            ref: p2-red
            desc: |
              Failing tests:
              - fresh DB has schema_migrations table and 0001 applied
              - running migrations again is a no-op (idempotent)
              - a new migration 0002_x.sql applies on next open
              - syntactically broken migration errors and does NOT mark applied
              - numeric prefix gaps are allowed; lexicographic order wins
          - title: "P2 green: implement runner + baseline migration"
            ref: p2-green
            blockedBy: [p2-red]
            desc: |
              Embed migrations via //go:embed internal/migrations/*.sql.
              Convert current initSchema into 0001_initial.sql. Runner
              lives in internal/job/migrations.go; openDB calls it.

      # ---------- Phase 3: Default identity + --strict ----------
      - title: "P3: Default identity + strict mode"
        ref: p3
        blockedBy: [p2]
        desc: |
          Permissive-by-default attribution. The DB carries an optional
          default identity; strict mode disables the default.

          Precedence (first match wins):
            1. --as <name> flag
            2. DB-level default identity (unless strict mode is on)
            3. Error: "identity required. Pass --as <name> ..."

          Surface area:
            - `job init [--default-identity <name>] [--strict]`
              Defaults to `--default-identity $USER` unless --strict.
              Emits a one-line note: "Default identity: ben (from $USER)".
            - `job identity set <name>` — change the default post-init.
              Requires --as on the call (bootstrap discipline).
            - `job identity strict on|off` — toggle strict mode.
            - Honest: no env var fallback, no OS-user inference at
              write-time. If you want strictness, `--strict` gives it
              to you cleanly.
        children:
          - title: "P3 red: identity resolution tests"
            ref: p3-red
            desc: |
              Failing tests:
              - init without flags sets default = $USER and emits a
                one-line note
              - init --default-identity claude sets default = claude
              - init --strict leaves default unset; writes without --as error
              - writes without --as succeed when default is set
              - --as <name> overrides the default
              - identity set <name> updates the default (requires --as)
              - identity strict on disables default; strict off restores it
              - strict off after a strict init: default is restored to
                what it would have been at init (or $USER if unset?)
                — resolve this: probably "unset until explicitly set"
          - title: "P3 green: implement identity resolution"
            ref: p3-green
            blockedBy: [p3-red]
            desc: |
              Add `config(key TEXT PRIMARY KEY, value TEXT)` table via
              migration 0002. Store keys: default_identity, strict_mode.
              requireAs() checks flag → config default → error. Update
              init, add `identity` verb group.

      # ---------- Phase 4: Cancel cascade ----------
      - title: "P4: Cancel cascade with status-aware destination"
        ref: p4
        blockedBy: [p1]
        desc: |
          Canceling the last open child cascades upward like done does.
          Destination status depends on sibling mix:
            - any sibling closed as `done` → parent cascades to `done`
            - zero siblings done (all canceled) → parent cascades to
              `canceled`

          Attribution: the canceling agent is the actor on the cascade
          event, same pattern as done.

          Event detail: {auto_closed: true, trigger_kind: "cancel",
          triggered_by: <shortID>, cascade_status: "done"|"canceled"}
        children:
          - title: "P4 red: cancel cascade tests"
            ref: p4-red
            desc: |
              Failing tests:
              - cancel last open child (siblings all done) → parent → done
              - cancel last open child (siblings all canceled) → parent → canceled
              - cancel last open child (mixed siblings) → parent → done
                (any-done rule)
              - cascade continues upward through multiple levels with
                correct per-level destination
              - parent with canceled-only tree cascades to canceled all
                the way up
              - existing done-cascade tests still pass (regression)
          - title: "P4 green: extend cascade helper"
            ref: p4-green
            blockedBy: [p4-red]
            desc: |
              Refactor cascadeAutoCloseAncestors to take a trigger kind
              and compute destination per-ancestor based on sibling mix.
              Wire into runCancel alongside existing runDone call site.

      # ---------- Phase 5: Status claimed count ----------
      - title: "P5: Show claimed count in status"
        ref: p5
        blockedBy: [p1]
        desc: |
          When the caller holds one or more claims, the status one-liner
          includes a "N claimed" term. Suppress the term when zero to
          avoid noise for non-claiming callers.

          Format: "1 claimed, 9 open, 14 done (last activity: 0s ago)"
          Fallback: "9 open, 14 done (last activity: 0s ago)"

          The count is scoped to the caller's identity (resolved via the
          P3 precedence chain). With no caller (read-only `status`),
          count all claimed tasks globally.
        children:
          - title: "P5 red: status claimed-count tests"
            ref: p5-red
            desc: |
              Failing tests:
              - caller holding 2 claims sees "2 claimed, N open, P done"
              - caller holding 0 claims sees "N open, P done" (no claimed)
              - --as absent and caller-less read: shows total claimed tasks
                if > 0
              - expired claims don't count (subject to expireStaleClaims)
          - title: "P5 green: implement scoped claim count"
            ref: p5-green
            blockedBy: [p5-red]

      # ---------- Phase 6: DRY the open-children predicate ----------
      - title: "P6: Extract shared `has open children` helper"
        ref: p6
        blockedBy: [p1]
        desc: |
          The predicate `status NOT IN ('done', 'canceled') AND
          deleted_at IS NULL` appears in countOpenChildren,
          queryAvailableLeafFrontier, cascadeAutoCloseAncestors, and
          at least one list filter. Extract into a package const or a
          small helper so the four sites can't drift.

          Not a TDD phase — the existing tests are the safety net for
          "behavior unchanged." Single task.
        children:
          - title: "P6: extract and replace call sites"
            ref: p6-do
            desc: |
              Either a `const openChildFilter = "status NOT IN ('done',
              'canceled') AND deleted_at IS NULL"` in internal/job, or a
              helper `whereOpenChild(alias)` returning the clause.
              Replace the four (or more) inline occurrences. Full suite
              passes; no new tests.

      # ---------- Phase 7: Output tightening + renderer refactor ----------
      - title: "P7: Done ack output improvements + AckLine render plan"
        ref: p7
        blockedBy: [p1]
        desc: |
          Three user-visible output improvements, implemented cleanly
          via a render-plan refactor:

          (1) Suppress "All tasks in X complete." line when X is the
              highest auto-closed ancestor (avoids duplicating the
              Auto-closed line that was just emitted).
          (2) Fix `Next:` to point at a leaf, not a parent-with-open-
              children. Currently NextAfterParent returns the next
              sibling of the top auto-closed ancestor, which may itself
              be a non-leaf; walk into its subtree to find the first
              claimable leaf.
          (3) Suppress `Next:` when --claim-next already produced a
              Claimed: line (don't tell the user what to do after
              telling them you already did it).

          Implementation move: refactor renderDoneAck to build an
          ordered `[]AckLine` plan in the context step, then the
          renderer loops over it. The three improvements become
          append/skip conditions on the plan builder, not nested
          branches.
        children:
          - title: "P7 red: output-improvement tests"
            ref: p7-red
            desc: |
              Failing tests:
              - simple auto-close (one level): output has Auto-closed
                line but NOT a redundant "All tasks in X complete" line
              - nested cascade: "All tasks in X complete" fires only
                when the whole-tree root is ABOVE the highest
                auto-closed ancestor
              - Next: resolves to the leaf under a sibling parent, not
                the sibling parent itself
              - done --claim-next with successful claim: no `Next:` line
              - done --claim-next with race-lost claim: `Next:` still
                fires (useful fallback)
          - title: "P7 green: AckLine refactor + three improvements"
            ref: p7-green
            blockedBy: [p7-red]
            desc: |
              Introduce AckLine type (probably just `type AckLine string`
              to start; widen if HTML renderer wants structured data).
              Build the plan in computeDoneContext (or a new
              buildDoneAckPlan that takes ctx + closed results). Render
              becomes a for-loop. Existing done_test.go assertions stay
              green via the same literal output shape.

      # ---------- Phase 8: Global log / tail views ----------
      - title: "P8: Global log and tail (all top-level nodes)"
        ref: p8
        blockedBy: [p1]
        desc: |
          Currently `job log` and `job tail` require a single <id>
          positional. Add a global scope: `job log` / `job tail` with
          no positional arg streams/shows events across every top-level
          task and its descendants — effectively "the whole DB."

          Surface: follow the `next all` precedent. `log all` / `tail all`
          as the explicit form; no-args can also map to global for
          convenience. Filters (--since, --events, --users) apply
          globally. `tail --until-close=<id>` still names its watch set
          explicitly and is orthogonal to scope.

          Implementation: the recursive-CTE queries in log/tail take an
          anchor shortID; make the anchor optional. When absent, the
          CTE anchor set is all tasks with parent_id IS NULL (and
          walks down from there).
        children:
          - title: "P8 red: global log/tail tests"
            ref: p8-red
            desc: |
              Failing tests:
              - `job log` with no arg returns events from all top-level
                trees, in chronological order
              - `job log all` is a synonym
              - `job tail` with no arg streams events from all trees
              - `job tail all` is a synonym
              - existing `job log <id>` / `job tail <id>` unchanged
              - filters (--since, --events, --users) compose with global
                scope
              - tail --until-close=<id> still watches the named task
                even when streaming globally
          - title: "P8 green: implement optional anchor"
            ref: p8-green
            blockedBy: [p8-red]
            desc: |
              Lift the "anchor required" constraint in getEventsForTaskTree
              and its tail equivalents. When anchor is empty, the CTE
              starts from `parent_id IS NULL` and recurses. Update cobra
              Args from ExactArgs(1) to MaximumNArgs(1); handle the
              optional "all" keyword.

      # ---------- Phase 9: Docs sweep ----------
      - title: "P9: Final docs sweep"
        ref: p9
        blockedBy: [p2-green, p3-green, p4-green, p5-green, p6-do, p7-green, p8-green]
        desc: |
          Update README.md for:
          - default identity + --strict mode (identity section)
          - cancel cascade symmetry (leaf-frontier subsection)
          - migration system (database section, developer-facing note)
          - status claimed count (viewing tasks section)
          - global log/tail (observation section)
          - note on package layout for contributors (new subsection
            or CONTRIBUTING.md — user's call)

          No docs/content/docs/ — that directory still doesn't exist and
          README remains canonical. Verify the quickstart examples still
          work under the refactored layout.
```

## Notes on ordering

- P1 (refactor) blocks everything — the new package layout is the
  surface other phases operate on.
- P2 (migrations) blocks P3 because the default-identity + strict
  config needs a migration to add a `config` table.
- P4, P5, P6, P7, P8 are independent of each other once P1 is done.
  They can be done in any order; the plan serializes them for focus
  but nothing prevents parallel work.
- P9 (docs) waits on everything.
- Commit cadence: one commit per phase is the default. P1 may want to
  be split (P1a/P1b as separate commits) so the move and the wiring
  can be reviewed in isolation.

## Known caveats

- **Binary install after each phase.** Same as last time — the
  installed `job` binary is stale until `go install .` runs. The first
  phase's test suite will catch regressions; runtime dogfooding needs
  the reinstall.
- **P3 uses the config table introduced in P2's migration.** If you
  pause between P2 and P3, the table sits unused. Fine; P3 just reads
  from it when it lands.
- **`job serve` is out of scope** for this plan. The `internal/server/`
  stub is there to land on a clean structure later — no behavior, no
  tests, no server wiring yet.
