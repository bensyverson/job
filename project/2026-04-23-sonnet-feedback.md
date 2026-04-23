# `job` Feedback from the NpWZj Papercuts Session

*Author: Claude Sonnet 4.6, after using `job` to drive the six-task "CLI papercuts from the 2026-04-23 session" sprint.*
*Date: 2026-04-23.*

This session was structurally different from the two earlier feedback docs. Those sessions were creative work: designing, prototyping, iterating. This one was almost entirely *implementation work against a pre-shaped plan*: six children of NpWZj already defined, each a concrete feature with a clear spec. My role was claim → implement → test → done, six times in sequence.

That makes this feedback unusually focused on the tight loop. The frictions I hit were the frictions you feel when you're in flow and the tool briefly interrupts that flow. Small costs, felt many times.

---

## Part 1 — Experience report

### Context of use

I started by running `job status` and `job info NpWZj` to orient. NpWZj had six children (F1–F6), none claimed. I worked through them in order, claiming each before starting and closing with `done -m` on completion. Along the way I surfaced one design question (F5 — the note echo format), which required a back-and-forth with Ben before I could implement. After all six were done I followed a code review, fixed a mutation bug, and committed.

Total `job` calls: roughly 35. Most were claim, note (progress updates), and done. A handful were info, reopen, and status.

### What worked

**`done -m` as atomic commit + closure.** Closing a task with a rationale note in a single call is the right shape. No juggling — finish the work, write the note, done. The note and the status change happen together.

**Task descriptions with explicit code paths.** Ben had authored F1–F6 with touchpoint lines like `internal/job/tasks.go:854 — RunAdd`. Those line numbers were directly actionable; I could jump straight to the relevant site without a codebase-wide search. Not a `job` feature per se, but the description field earned its keep here in a way it might not in a higher-level planning session.

**`Next:` in `done` output.** After each `done`, the next available task appeared in the confirmation. The cost of figuring out "what do I do next?" was zero across all six tasks. This is quietly excellent UX. I never once ran a separate lookup to find my next task.

**Auto-extend on write.** I made several `note` calls mid-task to record intermediate progress. Each one silently extended my claim TTL to now + 30m. I never needed to call `heartbeat`. The convention — keep writing, stay claimed — matched the natural work rhythm perfectly.

**Auto-close parent on last child.** Finishing F6 closed NpWZj. No separate bookkeeping. I'd read that this existed but still felt a small satisfaction when it happened in context.

### Frictions

**`reopen` requires a separate `claim`.** When Ben changed direction on F5 after I'd closed it with option (c), I ran `job reopen ORw3g`. The task was open again — but unclaimed. I then had to run `job claim ORw3g` as a second step before I could start working on it. The conceptual operation here is "I'm going to work on this again"; `reopen` and `claim` are a single intent split across two calls. An agent in flow pattern — finish something, immediately restart it — shouldn't need to remember the second step. `reopen` should auto-claim.

**Scoped view requires two commands.** To get a full picture of NpWZj mid-session, I'd run `job status NpWZj` (preamble: counts, Next, Stale) and `job list NpWZj` (task tree). Two calls for what feels like one question: "what's the state of this subtree?" The difference between `status` and `list` is real — `status` knows about claims, stale tasks, and who's holding what; `list` is the filterable structural view — but for a scoped query both pieces are always wanted together. `status <id>` should append the task tree inline, so one call answers the full question.

**No batch `job info`.** Several times during implementation I wanted context on two or three tasks at once — checking how F3 related to F6, verifying what F4's dry-run output spec said. I had to run `job info F3_id`, read it, then `job info F6_id`. The IDs are in my conversation context; the individual calls are cheap; but one `job info <id> [id ...]` that returns multiple task views would save the round-trips when I already know what I need.

**No structural representation for "pending decision" tasks.** F5 was a design question with three options: (a) two-line ack, (b) JSON flag, (c) document in AGENTS.md. There's no way to model this in `job`. I couldn't add three child tasks representing the options and mark the parent as "blocked on decision." I couldn't attach the option list to the task in a way that would surface in `info`. I managed it in conversation context, which works, but felt like a missed opportunity. A lightweight convention — even just a standard label like `decision` that `status` would call out specially — would help. See R4 below.

**`RunListFiltered` signature sprawl as developer friction.** This isn't a user-facing issue, but: adding `--grep` to `list` required me to add a 7th positional parameter to `RunListFiltered`, then mechanically update every caller (about 20 sites across five files). Same thing happened with `RunAdd` gaining a `labels` parameter. The function signatures are growing by accretion; each new feature costs a codebase-wide touch. A filter struct for `RunListFiltered` would cap that cost at one definition change. See R5 below.

### On the `status` vs `list` question

Ben asked directly: is there too much overlap between `status` and `list`? My answer after using both throughout this session:

They are different tools that happen to look similar for flat subtrees. `status` is a *situational awareness* command — it knows who has what claimed, what's stale, what's next globally. `list` is a *structural* command — it knows the tree, can filter by label or grep, shows blocked/claimed state per node. The overlap is real but the purposes diverge the moment anything interesting is happening (concurrent claims, stale tasks, multi-level trees).

The right fix is not to merge them, but to make `status <id>` complete: currently it gives the rollup summary for a subtree but not the tree itself, so you still need `list <id>` to see the structure. If `status <id>` appended the task list, one call would answer "what's happening in this subtree?" and the apparent overlap with `list` would shrink to a specialization question ("I want the full output" vs "I want to filter by grep/label"), not a functional overlap.

---

## Part 2 — Targeted recommendations

Ranked by friction-per-session rather than implementation size.

### R1. `status <id>` should inline the task list

**Why:** The most common scoped query during this session was "where am I in NpWZj?" I always needed both the rollup (from `status`) and the tree (from `list`). Two calls, one intent.

**Shape:** When `status` is given an `<id>` argument, append the task list for that subtree below the rollup — identical to what `list <id>` would produce, but without requiring a second call. The list section should respect the same depth/filter defaults that `list` uses today. If the user wants to filter, they should still call `list` explicitly; `status <id>` just gives the unfiltered view.

```
job status NpWZj
NpWZj — CLI papercuts from 2026-04-23 session
  4 of 6 done · 0 blocked · 2 available
  Next: F5_id · Stale: none

Tasks:
  - [x] F1_id  Add --label flag to `job add`
  - [x] F2_id  Propagate YAML parse errors from import
  - [x] F3_id  --grep filter on `list`
  - [x] F4_id  blockedBy in dry-run import output
  - [ ] F5_id  Two-line note ack on success
  - [ ] F6_id  --all flag on `list`
```

Touchpoints:
- `cmd/job/status.go` — when `args[0]` is set, run `RunListFiltered` for the subtree and append output below the summary block.
- `internal/job/status.go` — no changes needed; the summary already knows how to scope.
- Tests: `status <id>` on a flat subtree shows both rollup and tree; `status` with no args does not change.

### R2. `reopen` should auto-claim

**Why:** Reopening a task is almost always followed immediately by claiming it. Requiring two calls interrupts the "I'm going to work on this again" intent. The `--no-claim` flag is a sufficient escape hatch for the rare case where someone else will work the reopened task.

**Shape:** `job reopen <id>` reopens and claims atomically. Confirmation output changes from:
```
Reopened: F5_id "Two-line note ack"
```
to:
```
Reopened: F5_id "Two-line note ack"
  claimed by alice (until 2026-04-23 15:42)
```
Add `--no-claim` flag for the case where reopening for someone else.

Touchpoints:
- `cmd/job/reopen.go` — call `RunClaim` after `RunReopen` succeeds (or fold into a new `RunReopenAndClaim`).
- `internal/job/tasks.go` — `RunReopen` either gains a `claim bool` param or a new wrapper calls both in sequence in one transaction.
- Tests: reopen auto-claims by default; `--no-claim` skips the claim; reopen already-open task still fails; claimed-by output in confirmation.

### R3. Batch `job info <id> [id ...]`

**Why:** When context on multiple related tasks is needed at once, N sequential `info` calls are N unnecessary round-trips. The IDs are already known; the per-call overhead is pure friction.

**Shape:** `job info <id> [id ...]` accepts a variadic list. Multiple tasks are rendered sequentially, separated by a blank line, using the same format as today's single-task `info`. No new flags needed; `--format json` returns a JSON array.

Touchpoints:
- `cmd/job/info.go` — change `ExactArgs(1)` to `MinimumNArgs(1)`; loop over args.
- `internal/job/tasks.go` — `RunInfo` already takes a single ID; the loop is at the command layer.
- Tests: single ID (unchanged); two IDs returns both; one valid + one invalid ID returns an error (fail-fast, or all-errors, document the choice); JSON format wraps in an array.

### R4. Convention for "decision pending" design tasks

**Why:** F5 was a design question — three options, one of which would be chosen before implementation could start. There was no way to represent this in `job`'s structure. The task appeared as a normal open task until Ben weighed in; its special status (blocked on a human decision, not on another task) lived only in conversation context.

This is a pattern that will recur: tasks that can't proceed until a question is answered, but where the question itself isn't another task.

**Shape:** The lightest-weight solution is a reserved `decision` label that `status` calls out in its output — like how `Stale:` calls out abandoned claims. A task labeled `decision` shows up as `Decision:` in the `status` preamble, signaling "someone needs to answer a question before work can proceed." The description field carries the options; the task stays open until a choice is made.

Optionally, a new task state `pending-decision` would be cleaner but is a heavier change. Start with the label convention; if it's not expressive enough, promote to first-class state later.

Touchpoints (label convention approach):
- `internal/job/status.go` — in `RunStatus`, scan open tasks for the `decision` label and populate a `DecisionTasks` slice in `StatusSummary`.
- `internal/job/format.go` — render `Decision: <id> "<title>"` lines in the status preamble alongside `Next:` and `Stale:`.
- Tests: `decision`-labeled task surfaces in `status`; non-decision open tasks do not; closed `decision` tasks do not.

### R5. Filter struct for `RunListFiltered`

**Why:** `RunListFiltered` now has 7 positional parameters: `db`, `parentID`, `depth`, `showAll`, `claimedByActor`, `filterLabel`, `grepPattern`. Each new filter feature requires a signature change and updates to ~20 callers. This is pure developer friction, not user friction — but it compounded significantly in this session (adding `--grep` and `--all` each required a codebase-wide update).

**Shape:** Replace the parameter list with a `ListFilter` struct:

```go
type ListFilter struct {
    ParentID       string
    Depth          int
    ShowAll        bool
    ClaimedByActor string
    Label          string
    GrepPattern    string
}
```

`RunListFiltered(db *sql.DB, f ListFilter) ([]*TaskNode, error)`

This is a breaking change to the internal API — but we're in build mode, zero users. Future filter additions are one-line struct field additions; callers only set what they need.

Touchpoints:
- `internal/job/tasks.go` — replace the 7-arg signature; update the single implementation.
- All callers of `RunListFiltered` (currently ~20 sites in 5 files) — replace positional args with struct literals. The mechanical update is the same size as today's parameter additions, but it's the last one.
- Tests: no behavioral change; run the full suite after the refactor to confirm.

---

## Appendix: session at a glance

- ~35 `job` calls across ~2 hours of work
- 6 tasks claimed, noted, and closed (all children of NpWZj)
- 1 task reopened (F5, after a direction change) + 1 separate claim call (R2 hit)
- 2 scoped status + list pairs run (R1 hit, twice each)
- 3 individual `info` calls where a batch would have done it (R3 hit)
- 1 design decision managed in conversation context with no `job` representation (R4 hit)
- 1 codebase-wide signature update for `RunListFiltered` (7th param) + 1 for `RunAdd` (labels param) (R5 hit)
- 0 `heartbeat` calls needed (auto-extend on write worked perfectly)
- 0 grammar-guess failures (R3 from the prior session's doc appears to have landed)

---

## Plan

Concrete `job import` YAML for R1–R5 plus a docs sweep. All items are independent except the docs task, which waits on all five.

```yaml
tasks:
  - title: CLI improvements from 2026-04-23 Sonnet session
    ref: sonnet-feedback
    desc: |
      Five agent-facing ergonomics fixes drawn from the R1–R5
      recommendations in project/2026-04-23-sonnet-feedback.md.
      Each child stands alone and can be landed in any order.
      Tests are bundled with each change; the docs task at the
      end covers any README or CLAUDE.md touches across all five.
    labels: [cli, feedback, ergonomics]
    children:

      - title: R1 — `status <id>` inlines task list when scoped
        ref: s1
        desc: |
          When `status` is given an `<id>` argument, append the task
          list for that subtree below the rollup summary — identical
          to what `list <id>` would produce, without requiring a
          second call. No filtering; for grep/label filtering the user
          still calls `list` explicitly.

          Shape:
            job status NpWZj
            NpWZj — CLI papercuts from 2026-04-23 session
              4 of 6 done · 0 blocked · 2 available
              Next: F5_id · Stale: none

            Tasks:
              - [x] F1_id  Add --label flag to `job add`
              - [ ] F5_id  Two-line note ack on success
              ...

          Touchpoints:
          - cmd/job/status.go — when args[0] is set, call
            RunListFiltered for the subtree and append output below
            the summary block.
          - internal/job/status.go — no changes needed; summary
            already scopes correctly.
          - Tests: status <id> on a flat subtree shows both rollup
            and tree; status with no args is unchanged; status on a
            deep tree shows all levels (same depth default as list).
        labels: [cli, status]

      - title: R2 — `reopen` auto-claims; add `--no-claim` escape hatch
        ref: s2
        desc: |
          Reopening a task is almost always followed immediately by
          claiming it. Make this atomic by default; add --no-claim
          for the rare case where someone else will work the task.

          Shape:
            job reopen F5_id
            Reopened: F5_id "Two-line note ack"
              claimed by alice (until 2026-04-23 15:42)

          Touchpoints:
          - cmd/job/reopen.go — after RunReopen succeeds, call
            RunClaim unless --no-claim is set. Append the claim
            confirmation line to output.
          - cmd/job/reopen.go — add BoolVar for --no-claim flag.
          - internal/job/tasks.go — no internal change required;
            the command layer calls both in sequence.
          - Tests: reopen auto-claims by default; --no-claim skips
            the claim step and omits the claim output line; reopening
            an already-open task still fails (unchanged); output
            format includes the claim line when claimed.
        labels: [cli, reopen]

      - title: R3 — Batch `job info <id> [id ...]`
        ref: s3
        desc: |
          Accept multiple IDs so context on several related tasks
          can be fetched in one call.

          Shape: variadic args, rendered sequentially separated by
          a blank line. --format json returns a JSON array.

          Fail behavior: first invalid ID returns an error and stops
          (fail-fast). This can be relaxed to "all-errors" later if
          needed, but fail-fast is simpler to implement and easier
          to reason about.

          Touchpoints:
          - cmd/job/info.go — change ExactArgs(1) to MinimumNArgs(1);
            loop over args; blank-line separator between tasks.
          - cmd/job/info.go — --format json wraps results in a JSON
            array.
          - internal/job/tasks.go — RunInfo is unchanged; the loop
            is at the command layer.
          - Tests: single ID (unchanged behavior); two IDs returns
            both; valid + invalid ID fails on the invalid one; JSON
            format produces an array; separator line appears between
            tasks in text output, not after the last.
        labels: [cli, info]

      - title: R4 — Surface `decision`-labeled tasks in `job status`
        ref: s4
        desc: |
          Tasks labeled `decision` represent questions that must be
          answered before work can proceed. Today they are invisible
          in status output — indistinguishable from normal open tasks.
          Surfacing them in the status preamble (alongside Next: and
          Stale:) gives agents and humans a reliable signal that a
          human decision is pending.

          Shape — new line in status preamble when any open task
          carries the `decision` label:
            Next:     F5_id "Two-line note ack"
            Stale:    none
            Decision: D3_id "Choose auth strategy"

          Multiple decision tasks: list all on separate lines.
          Scoped status (status <id>): include only decision tasks
          within that subtree.

          Touchpoints:
          - internal/job/status.go — RunStatus: after computing
            Next and Stale, query for open tasks with label=decision
            scoped to the current subtree (or whole DB for unscoped).
            Populate a new DecisionTasks []TaskRef field on
            StatusSummary.
          - internal/job/format.go — render Decision: lines in the
            preamble block. Empty slice → omit the section.
          - Tests: decision-labeled open task surfaces; closed
            decision task does not; non-decision open task does not;
            scoped status only shows in-subtree decisions; multiple
            decision tasks each get a line.
        labels: [cli, status, decision]

      - title: R5 — Replace RunListFiltered positional params with ListFilter struct
        ref: s5
        desc: |
          RunListFiltered now has 7 positional parameters. Each new
          filter feature costs a signature change and ~20 caller
          updates. A filter struct caps that cost permanently.

          Shape:
            type ListFilter struct {
                ParentID       string
                Depth          int
                ShowAll        bool
                ClaimedByActor string
                Label          string
                GrepPattern    string
            }
            func RunListFiltered(db *sql.DB, f ListFilter) ([]*TaskNode, error)

          Breaking change to the internal API — acceptable in build
          mode with zero external users.

          Touchpoints:
          - internal/job/tasks.go — replace the 7-arg signature;
            update the implementation (no behavioral change).
          - All callers (~20 sites across 5 files):
            cmd/job/list.go, cmd/job/labels_test.go,
            cmd/job/short_flags_test.go, cmd/job/database_test.go,
            cmd/job/format_listparens_test.go — replace positional
            args with struct literals.
          - Tests: full suite should pass without behavioral change.
            The test for the refactor is a green suite, not a new
            test case.
        labels: [cli, refactor, internal]

      - title: Docs sweep for R1–R5
        ref: sonnet-docs
        desc: |
          Single consolidated docs update once the behavior changes
          have landed.

          Sections to touch:
          - docs/content/docs/ and README.md (if new doc added):
            status <id> inline list behavior (R1).
          - `reopen` section: auto-claim default + --no-claim flag (R2).
          - `info` section: variadic IDs, JSON array output (R3).
          - `status` section: Decision: preamble line, decision label
            convention (R4).
          - CLAUDE.md if any agent-visible invariant changed — in
            particular, note that `status <id>` now includes the task
            list and that `reopen` auto-claims.
        labels: [docs]
        blockedBy: [s1, s2, s3, s4, s5]
```
