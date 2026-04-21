# Implement Opus Feedback: Leaf-Frontier Semantics and UX Polish

Plan to act on the 2026-04-21 Opus feedback doc. Work is grouped into five
phases, implemented top-down, each phase following strict red/green TDD.

## Source of motivation

See [`2026-04-21-opus-feedback.md`](2026-04-21-opus-feedback.md). The seven
actionable items collapse to five here:

- **P1.** Leaf-frontier semantics — a task is claimable iff it has no open
  children. Cascades into `next` filtering, `claim` refusal on parents,
  parent auto-close on last-child-done, and parent auto-release on
  first-child-add.
- **P2.** `done --claim-next` — collapses the close-then-advance chain.
- **P3.** `-m @file` / `-m -` — multi-line completion notes via file or
  stdin.
- **P4.** Better error on `done <id> "prose"` — detect positional prose
  that should have been `-m`.
- **P5.** Documentation sweep.

The env-var-identity item from the feedback doc was dropped: a static
identity source (env var or file) can't disambiguate parallel Claudes,
and the single-agent case is already adequately served by shell aliases.

## Plan

```yaml
tasks:
  - title: Implement Opus feedback refactor
    ref: root
    desc: |
      Act on the 2026-04-21 Opus feedback doc. Five phases, TDD throughout.
      Each implementation task is gated by its red tests.
    labels: [refactor, dx]
    children:

      # ---------- Phase 1: Leaf-frontier semantics ----------
      - title: "P1: Leaf-frontier claim/next semantics"
        ref: p1
        desc: |
          A task is claimable iff it has no open children.
          Four sub-behaviors, each with its own red/green pair:
          (a) next / next all / claim-next default to the leaf frontier,
              with --include-parents to restore old behavior.
          (b) claim <parent-with-open-children> refuses with a message
              naming the alternative.
          (c) Parents auto-close when their last open child closes
              (cascade upward through ancestors).
          (d) Parent claim auto-releases when its first open child is
              added (decomposition flow).
        children:

          - title: "P1a red: leaf-frontier filtering tests"
            ref: p1a-red
            desc: |
              Failing tests for next, next all, and claim-next:
              - root with unblocked children is NOT surfaced
              - only leaves (no open children) surfaced
              - --include-parents flag restores current behavior
              - canceled children do not count as "open" for frontier purposes
          - title: "P1a green: filter next/claim-next to leaves"
            ref: p1a-green
            blockedBy: [p1a-red]

          - title: "P1b red: claim-parent refusal tests"
            ref: p1b-red
            blockedBy: [p1a-green]
            desc: |
              Failing tests:
              - claim <parent-with-open-children> errors with explicit message
                naming the alternative (e.g. "claim a leaf instead")
              - claim <leaf> still works
              - claim <parent-with-no-open-children> still works (leaf-shaped)
          - title: "P1b green: implement claim refusal"
            ref: p1b-green
            blockedBy: [p1b-red]

          - title: "P1c red: parent auto-close cascade tests"
            ref: p1c-red
            blockedBy: [p1b-green]
            desc: |
              Failing tests:
              - done on last open child closes parent automatically
              - cascade continues up the tree (grandparent closes too)
              - canceled siblings don't block cascade (treated as closed)
              - done output surfaces the cascade (parent ack lines appended)
              - event log records the auto-close with attribution
                of the closer (the agent who closed the last child)
          - title: "P1c green: implement parent auto-close"
            ref: p1c-green
            blockedBy: [p1c-red]

          - title: "P1d red: parent auto-release on child add tests"
            ref: p1d-red
            blockedBy: [p1c-green]
            desc: |
              Failing tests:
              - add child to claimed parent auto-releases the claim
              - add output surfaces the release ack
              - parent with no claim is unaffected
              - release is idempotent if re-triggered
          - title: "P1d green: implement parent auto-release"
            ref: p1d-green
            blockedBy: [p1d-red]

          - title: "P1 docs: document leaf-frontier model"
            ref: p1-docs
            blockedBy: [p1d-green]
            desc: |
              Update docs/ and --help to describe:
              - leaf-frontier as the default for next/claim-next
              - --include-parents for old behavior
              - claim refusal on parents
              - parent auto-close and auto-release

      # ---------- Phase 2: done --claim-next ----------
      - title: "P2: done --claim-next flag"
        ref: p2
        blockedBy: [p1]
        desc: |
          Atomically close a task and claim the next leaf.
          Race: if next leaf taken between done and auto-claim, status line
          reports "Next leaf (X) taken by Y; no claim made." rather than
          erroring (done is irreversible, claim is opportunistic).
        children:
          - title: "P2 red: done --claim-next tests"
            ref: p2-red
            desc: |
              Failing tests:
              - flag closes the task AND claims the next leaf
              - output shape matches bare claim ack (greppable ^Claimed:)
              - race: when next leaf claimed between done and claim, status
                line reported, no error, done still succeeds
              - no next leaf available: status line reported, done succeeds
              - -m still works in combination
          - title: "P2 green: implement done --claim-next"
            ref: p2-green
            blockedBy: [p2-red]
          - title: "P2 docs: document done --claim-next"
            ref: p2-docs
            blockedBy: [p2-green]

      # ---------- Phase 3: -m @file / -m - ----------
      - title: "P3: -m @file and -m - stdin support"
        ref: p3
        desc: |
          Independent of P1/P2. Multi-line notes via file or stdin to dodge
          shell-quoting hell. Applies to done -m, note -m, and anywhere else
          that accepts -m.
        children:
          - title: "P3 red: -m @file / -m - tests"
            ref: p3-red
            desc: |
              Failing tests for done and note:
              - -m @path/to/file.txt reads file contents as the note
              - -m - reads stdin as the note
              - -m "literal string" still works
              - -m @nonexistent errors with a clear message
              - -m @file preserves newlines
          - title: "P3 green: implement @file and stdin support"
            ref: p3-green
            blockedBy: [p3-red]
          - title: "P3 docs: document -m @file and -m -"
            ref: p3-docs
            blockedBy: [p3-green]

      # ---------- Phase 4: better done-prose error ----------
      - title: "P4: Detect positional prose in done/claim"
        ref: p4
        desc: |
          When a second positional arg to done/claim looks like prose
          (contains whitespace or is longer than a short-ID), suggest -m.
        children:
          - title: "P4 red: positional-prose detection tests"
            ref: p4-red
            desc: |
              Failing tests:
              - done <id> "multi word note" errors with "did you mean -m?"
              - done <id> <valid-second-id> still works (multi-done)
              - claim <id> "prose" errors with a helpful suggestion
              - single-word non-id second positional: heuristic handles it
                (decide: err on the side of suggesting -m)
          - title: "P4 green: implement prose detection"
            ref: p4-green
            blockedBy: [p4-red]

      # ---------- Phase 5: final docs + README sweep ----------
      - title: "P5: Final documentation sweep"
        ref: p5
        blockedBy: [p1-docs, p2-docs, p3-docs, p4-green]
        desc: |
          Ensure docs/content/docs/ reflects all semantic changes, update
          README.md only if a new doc file was added or users NEED to
          know. Verify the examples in the quickstart still work under
          leaf-frontier semantics.
```

## Notes on ordering

- P3 and P4 are independent of the P1/P2 bundle. We'll do them serially
  after P2 to keep focus, but they could slot in parallel if needed.
- P1's sub-phases are serialized (a → b → c → d) because each one's
  tests exercise the previous one's behavior; parallel red tests would
  become green prematurely.
- Commit cadence: each phase is a commit (or a small stack if a phase is
  large). `go fix` + `go fmt` + tests must pass before each commit, per
  project Go conventions.

## Known caveat

We're implementing the leaf-frontier change using a version of `job` that
doesn't have it yet, so during this work `job next` will surface the root
task. Workaround: claim leaves by ID from `job list all`. Part of the
dogfooding.
