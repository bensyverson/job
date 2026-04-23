# `job` Papercuts from the R0–R7 Implementation Session

*Author: Claude Opus 4.7, after dogfooding `job` to drive the 10-task umbrella `njUQV` (R0–R7 + short-flag audit + docs sweep).*
*Date: 2026-04-22 (PM session, after the morning's Web-Dashboard retrospective).*

This is a smaller, sharper sibling to `2026-04-22-opus-feedback.md`. That doc covered structural friction discovered while *using* `job` for design work; this one is the residue from one focused implementation session — papercuts I would have shaved off before starting if I'd known to.

Each finding was verified against current code or session output before drafting (per the lesson from the morning's retrospective). The heartbeat question gets its own subsection because it's a bigger design call than the others.

---

## Findings

### P1 — Default claim TTL is 15m, which is short

`DefaultClaimTTLSeconds = 900` in `internal/job/claims.go:9`. For most agent tasks (write tests, implement, run tests, fix, commit), 15 minutes is a tight budget. The practical effect on me this session: I passed an explicit duration on every single `claim` call (2h, 4h, 1h) because 15m felt risky and I didn't want to wire up `heartbeat`. That makes the optional duration arg feel mandatory.

**Fix:** Bump to **30m**. Long enough that most tasks don't need refreshing; short enough that a crashed agent's lock doesn't sit forever.

If we keep heartbeat (see below), bump its extension from 15m to 30m too so the two stay in sync.

### P2 — README quickstart primes the wrong claim habit

The duration arg is documented as optional in `claim --help` (`claim <id> [duration]`, "Duration defaults to 15m"). But the README quickstart's only `claim` example is:

```sh
# Claim a task for 4 hours (default is 15m)
job --as alice claim <id> 4h
```

That's the first time `claim` appears in the docs. The "(default is 15m)" parenthetical is overwhelmed by the explicit `4h`. As an agent reading the docs to learn the tool, I copied the example pattern and never tried the bare form — even though I'd have been better off using it (modulo the short default).

**Fix:** Lead with `job --as alice claim <id>` in the quickstart; move the duration form to a second example labeled "with explicit duration." Punchier `claim --help` Short: `"Claim a task (duration optional, default 30m)"`.

### P3 — `done -m` and `cancel -m` don't echo note bodies

R4 added a preview echo for `note -m`: `Noted: <id> · <N chars> · "<preview>"`. Same logic should apply to `done -m` and `cancel -m`, where the note body is just as load-bearing — but neither was updated in R4.

I noticed this immediately: I shipped R4, then wrote ten `done -m "..."` notes documenting each completed task, and got terse `Done: <id> "Title"` acks for all of them — no preview confirming the long body landed correctly. I built the feature and didn't get to use it.

Verified by inspecting `cmd/job/done.go` (no `notePreview` call) and `cmd/job/cancel.go` (no preview at all on the cancel ack).

**Fix:** Reuse `notePreview` in the done/cancel ack builders. Render as a sub-line under the existing `Done: ...` / `Canceled: ...` line: `  note: <N chars> · "<preview>"`.

### P4 — `claim` ack is asymmetric with `done`

`done` shows: `Done: <id> "Title"` (`cmd/job/commands.go`, `buildDoneAckLines`).
`claim` shows: `Claimed: <id> (expires in 2h)` (`cmd/job/claim.go:54`) — **no title**.

Worse, the asymmetry is internally inconsistent: `done --claim-next` uses `Claimed: <id> "Title" (expires in ...)` (`cmd/job/done.go:130`). So we already know how to render the title-bearing form — just not from the bare `claim` verb.

Cost in this session: ~3 round-trip `job info <id>` calls to confirm I'd grabbed the right task before starting work. Cheap individually, paper-cut nature in aggregate.

**Fix:** Render `claim`'s ack as `Claimed: <id> "Title" (expires in <dur>)`, matching `done --claim-next`.

### P5 — `Next:` follows topology, not import order intent

Every `done` in this session suggested `Next: 7MFho` because that was the topologically-next available task (no `blockedBy` between it and the closing task). My plan, encoded by the import order in the umbrella, was R3 → R1 → R4 → R5 → R6 → R7 → R2 → 7MFho → docs. If I'd followed `Next:` blindly I'd have hit 7MFho before R2.

`Next:` is a hint, not a requirement, so this didn't bite me — I had the plan in conversation context and ignored the hint when needed. But for an autonomous or less-attentive agent, "topologically next" diverges from "next per the author's intent" whenever siblings are unordered by `blockedBy`.

The fix isn't obvious. Two options:

- **Respect import order in `Next:` ties.** Tasks already have a `sort_order` (used by `list`). When the next-claimable query has multiple candidates with no `blockedBy` constraint between them, prefer the lowest `sort_order`. Cheap; matches user intent for plans imported from YAML.
- **Skip.** `Next:` is purposefully simple; making it smarter risks confusion ("why is it suggesting *that*?"). Document the limitation instead.

I lean toward (1) — `sort_order` is already there and `list` already uses it, so there's no new state.

### P6 — `summary` per-child line is information-light when children are leaves

The `summary` command I just shipped renders per-child lines like:

```
R0 — Surface default identity in `job status` (A5tdX): done
R2 — `job summary <parent>` — phase/epic at-a-glance (wjnPM): claimed
Docs sweep — README updates for R0–R7 (qvZ6Y): available
```

Accurate, but the line earns its row only when the child has its own children (where it shines as `0 of 8 done · next dXJPQ`). For flat umbrellas like this one, ten lines of "done" / "claimed" / "available" feel like padding.

**Possible fix:** When all direct children are leaves, fold them into a compact one-liner: `R0: done · R1: done · R2: claimed · ... · 7MFho: available · qvZ6Y: blocked`. Or just skip the per-child block when the rollup line says everything (e.g., target is "10 of 10 done" — no point listing each child).

Lower priority than P1–P5; the current shape is workable, just verbose.

---

## The heartbeat question

Ben asked: is `heartbeat` earning its keep?

**What it does today.** `job heartbeat <id>` extends the caller's claim TTL by 15m (hardcoded in `cmd/job/heartbeat.go:13`). Requires the caller to hold the claim. Records a `heartbeat` event.

**Why it exists.** Multi-agent contention. If agent A claims a task with TTL 15m, work takes 30m, and A doesn't refresh, agent B can grab the task at minute 16. Heartbeat lets A signal "still alive."

**Why it didn't earn its keep this session.**

1. **No prompt to use it.** I never thought to call `heartbeat` because I never *checked* my claim TTL. The agent doesn't see a countdown; only `info <id>` would surface it.
2. **Implicit cadence.** If TTL is 15m and heartbeat extends by 15m, do I call it at minute 10? Minute 14? There's no documented pattern.
3. **The natural intent signal is already there.** When I `note <id>` or `label add <id>` on a claimed task, I'm clearly still working on it. Right now those don't extend the claim.

**Better designs (pick one):**

- **(a) Auto-extend on any write to the claimed task.** `note`, `edit`, `label add`, `label remove` on a task you hold → reset its TTL. Natural; no new verb to remember; signals "still alive" in the same call as the actual work. Keeps `heartbeat` for the rare "I'm thinking, not writing" case but de-emphasizes it from the docs.
- **(b) Drop `heartbeat` entirely; use `claim --extend <id>` instead.** Same cost as `heartbeat` but reuses the verb the agent already knows. Loses the auto-extend-on-write convenience.
- **(c) Status quo, plus surface TTL in `claim` ack and add a periodic warning.** Tells the agent the deadline exists. Doesn't solve the "I'm focused on work" problem.

My recommendation: **(a)**. It maps user intent ("I'm writing about this task → I'm working on this task → don't expire my claim") to the implicit signal we already have. Keeps `heartbeat` available for power users but moves it out of the quickstart.

Implementation sketch for (a):
- In `RunNote`, `RunEdit`, `RunLabelAdd`, `RunLabelRemove`: if the target task is currently claimed by the actor, update `claim_expires_at` to `now + DefaultClaimTTLSeconds`.
- No new event type — the existing `noted` / `edited` / `labeled` events already record the activity.
- Only extend; never shorten. Don't extend if claimed by someone else (that'd be silently overriding their lock).

---

## Plan

Eight follow-up tasks for the umbrella. Ordered roughly by ROI; the heartbeat redesign is its own track since it's the largest design move.

```yaml
tasks:
  - title: CLI papercuts from R0-R7 implementation session
    ref: papercuts
    desc: |
      Eight small-to-medium ergonomics fixes drawn from the
      end-of-session retrospective in
      project/2026-04-22-pm-papercuts.md. P1, P2, P3, P4 are pure
      polish. P5, P6 are smaller behavior changes. P7 is the
      heartbeat redesign — biggest of the bunch and the only one
      with multiple landing options to choose from before
      implementation. P8 wraps the docs.
    labels: [cli, ergonomics, feedback]
    children:

      - title: P1 — Bump default claim TTL from 15m to 30m
        ref: p1
        desc: |
          Change DefaultClaimTTLSeconds in internal/job/claims.go:9
          from 900 to 1800. Most agent tasks (write tests, implement,
          run tests, fix, commit) overrun 15m comfortably; 30m gives
          headroom without leaving crashed-agent locks sitting too
          long.

          If P7's heartbeat redesign keeps `job heartbeat`, bump its
          extension (currently hardcoded "15 minutes" in
          cmd/job/heartbeat.go:13 and the corresponding RunHeartbeat
          extension) to 30m so the two stay in sync.

          Touchpoints:
          - internal/job/claims.go:9 — DefaultClaimTTLSeconds
          - cmd/job/heartbeat.go:13 — Short string
          - internal/job/claims.go (RunHeartbeat) — extension constant
          - Tests that hardcode "15m" in claim/heartbeat assertions
            need updating to "30m"; explain the change to the user
            before flipping each one.
        labels: [cli, claim]

      - title: P2 — README quickstart leads with bare `claim <id>`
        ref: p2
        desc: |
          Today the only `claim` example in the README quickstart is
          `job --as alice claim <id> 4h`, with `(default is 15m)` as a
          parenthetical. As an agent reading the docs to learn the
          tool, the explicit `4h` primes a habit of always passing
          duration. Result: optional arg feels mandatory.

          Shape:
            # Claim a task (default 30m)
            job --as alice claim <id>

            # ... or with an explicit duration
            job --as alice claim <id> 4h

          Plus: punchier `claim --help` Short — change "Claim a task"
          to "Claim a task (duration optional, default 30m)".

          Touchpoints:
          - README.md (quickstart and Claiming section)
          - cmd/job/claim.go — Short string
          - Land after P1 so the "30m" number matches.
        labels: [docs, claim]
        blockedBy: [p1]

      - title: P3 — Echo note body on `done -m` and `cancel -m`
        ref: p3
        desc: |
          R4 added a `Noted: <id> · <N chars> · "<preview>"` echo to
          `note -m`. The same `notePreview` helper (cmd/job/note.go)
          should fire on `done -m` and `cancel -m` so the agent gets
          the same confirmation that a long body landed correctly.

          Shape: under the existing `Done: <id> "Title"` line, append
            note: <N chars> · "<preview>"
          Same for `Canceled: <id> "Title"`.

          Touchpoints:
          - cmd/job/commands.go — buildDoneAckLines (around line 253);
            inject a note line when closed[i].Note != "".
          - internal/job/format.go — RenderCancelAck (line 606); same
            shape under the Canceled line and per-item under the
            multi-cancel form.
          - cmd/job/note.go — notePreview is already exported within
            the package; no change to the helper itself.
          - Tests: done -m short/long/empty; cancel -m short/long;
            multi-target done/cancel where each closed task may carry
            its own note.
        labels: [cli, done, cancel, ack]

      - title: P4 — `claim` ack echoes task title
        ref: p4
        desc: |
          `done` shows `Done: <id> "Title"`; `claim` shows
          `Claimed: <id> (expires in 2h)` with no title. The same
          asymmetry exists internally: `done --claim-next` already
          renders `Claimed: <id> "Title" (expires in ...)`
          (cmd/job/done.go:130). Bring the bare `claim` verb in line
          with that shape.

          Shape: `Claimed: <id> "Title" (expires in <dur>)`.
          For force-overrode case:
            Claimed: <id> "Title" (overrode previous claim by
            <name>, expires in <dur>)

          Touchpoints:
          - cmd/job/claim.go:52,54 — both Fprintf calls; also need
            the title, which means a GetTaskByShortID call (or fetch
            it from RunClaim's return value if we widen RunClaim).
          - Tests for both ack paths.
        labels: [cli, claim, ack]

      - title: P5 — Respect import order in `Next:` ties
        ref: p5
        desc: |
          When picking the next-available task with no blockedBy
          edges among the candidates, prefer the lowest sort_order.
          Tasks already carry sort_order (used by `list`); adding it
          to the next-available query's ORDER BY makes `Next:`
          honour the author's import order for unconstrained
          siblings.

          In this session, every `done` suggested
          `Next: 7MFho` (topologically valid but contrary to the
          plan-encoded order R3 → R1 → R4 → ...). For an autonomous
          agent that follows the hint, this leads to wrong-order
          execution.

          Touchpoints:
          - internal/job/claims.go — queryAvailableTasks; verify the
            ORDER BY clause includes sort_order (or add it).
            findNextSibling already iterates siblings in sort order;
            the gap is in the global frontier query.
          - Tests: a freshly imported plan with no blockedBy should
            return its first child as Next: regardless of insertion
            timing.
        labels: [cli, next, planning]

      - title: P6 — `summary` collapses leaf-only direct-child block
        ref: p6
        desc: |
          When every direct child is a leaf (or status-only —
          done/canceled/available/claimed with no descendants), the
          per-child block is information-light. Fold into a compact
          one-liner under the headline rollup, or skip it entirely
          when the rollup already conveys "X of X done".

          Shape options to decide between:
          - One-liner: "Children: R0 done · R1 done · R2 claimed · ..."
          - Skip block when target.Open == 0
          - Status-grouped tally: "8 done · 1 claimed · 1 available"
          Stop and ask the user before implementing — this is a UX
          choice, not a mechanical fix.

          Lower priority than P1-P5 — current output is workable.

          Touchpoints:
          - internal/job/summary.go — RenderSummary; add a leaf-only
            detection pass and switch to the compact form.
          - Tests: pure-leaf umbrella (this one); deep tree (existing
            shape preserved); mixed (decide whether to switch per
            child or per umbrella).
        labels: [cli, summary, ergonomics]

      - title: P7 — Auto-extend claim TTL on writes to the claimed task
        ref: p7
        desc: |
          The bigger design move from the heartbeat retrospective.
          When an agent writes to a task they currently hold
          (`note`, `edit`, `label add`, `label remove`), the write
          itself is an "I'm still working on this" signal — extend
          the claim TTL by DefaultClaimTTLSeconds (P1's 30m).

          Rules:
          - Only extend; never shorten. If the existing
            claim_expires_at is further in the future than now+TTL,
            leave it alone.
          - Only extend if the writing actor IS the current
            claim_holder. A `note` from a different actor must not
            silently override another agent's lock.
          - Don't add a new event type. The existing `noted` /
            `edited` / `labeled` events already record the activity;
            the TTL bump is a side effect, not a notable event.

          Decide before implementation: keep `job heartbeat` as a
          low-level escape hatch for "I'm working but not writing"
          (recommendation), or remove it entirely once auto-extend
          covers the common case. If kept, demote it from the
          quickstart and orchestration sections of README.

          Touchpoints:
          - internal/job/tasks.go — RunNote, RunEdit (auto-extend
            block at the start of the transaction).
          - internal/job/labels.go — RunLabelAdd, RunLabelRemove
            (same).
          - New helper: maybeExtendClaim(tx, taskID, actor) used by
            all four call sites.
          - Tests: extend on note as holder; no-op on note as
            non-holder; no-op when current expiry > now+TTL; combined
            with the P1 TTL bump.
        labels: [cli, claim, heartbeat, design]
        blockedBy: [p1]

      - title: P8 — Docs sweep for P1-P7
        ref: p8
        desc: |
          Single consolidated docs update once the behavior changes
          have landed.

          Sections to touch:
          - README claim row + quickstart (P1, P2, P4 ack shape).
          - README done/cancel rows (P3 ack echo).
          - README next entry (P5: mention import-order tiebreak).
          - README summary entry (P6: new shape, if shipped).
          - README claiming section (P7: auto-extend on writes,
            heartbeat demoted or removed).
          - AGENTS.md / CLAUDE.md "CLI conventions" — surface auto-
            extend behavior so future agent sessions know they don't
            need to call `heartbeat`.
        labels: [docs]
        blockedBy: [p1, p2, p3, p4, p5, p6, p7]
```
