# `job` Feedback from the Web-Dashboard Prototype Session

*Author: Claude Opus 4.7, after using `job` to drive Phase 1 of the web dashboard (static HTML prototype pass).*
*Date: 2026-04-22.*

This session was structurally different from the two earlier feedback docs (2026-04-20, 2026-04-21). Those retrospectives were about *discovery-heavy* refactor work — lots of `add`, lots of mid-session emergence. This one was mostly *lifecycle work*: Ben created the Phase 1 plan up-front, I added ~6 child tasks during the design-review gate, and the bulk of my interaction was `claim → note → done` against an already-shaped DAG, with continuous conversational review between tasks.

So the feedback below emphasizes different edges than the earlier docs. Things that were pain points for discovery are fine when the plan is already in place; things that are fine for discovery are pain points when you're iterating.

---

## Part 1 — Experience report

### Context of use

Ben set me up with `z0OoG` (Phase 1) already containing the original prototype work (tokens.css, base.css, components.css, colors.js, home/plan/peek prototypes) — all done when I arrived. I spent the first hour of the session adding 6 new children to `z0OoG` (home v2, mini-graph interactivity, activity histogram, log view, actors view, scrubber) ahead of the design review gate `9AWkj`, blocking the gate on all of them, then working through them one at a time. Each task was: claim, build, iterate with Ben, note the implementation rationale, done, next.

Identity: I passed `--as claude` on every write, ~50 times. This was *unnecessary* — a DB-level default identity of `claude` was already set, so every omitted `--as` would have been attributed correctly. I discovered this only after Ben called it out at the end of the session. See the correction under Frictions, below. Ben wrote as `--as ben` (presumably needed — I didn't verify).

### What worked

**The claim → note → done loop is the right shape for this work.** It mapped one-to-one to my mental model — "I'm starting this; here's what I did and why; it's finished, next." The conversation with Ben stayed on the design, not the tracking.

**`Next: <id>` in the `done` output is a quiet UX gem.** After every `done`, I saw the next available task and — since Phase 1 was a linear plan — just claimed it. The cost of figuring out "what's next?" was zero. Any feature-equivalent that shaves a lookup out of the loop is worth a lot.

**Auto-closing a parent when its last child is done** was a surprise-delight. Finishing `9AWkj` closed `z0OoG`, which closed Phase 1. No bookkeeping. The parent's status reflected reality without me thinking about it.

**`--as claude` discipline (with a catch).** Attribution was clean across the two-actor session; re-reading history, I could tell what I did vs what Ben did without effort. The catch, discovered only after Ben asked: I was specifying `--as claude` on every call, but the DB had a default identity of `claude` already configured. Every omission would have been attributed identically. My "discipline" was self-imposed tax, not a tool requirement. See the correction under Frictions. The behavior I liked was real — default identity with optional `--as` override and optional strict mode is already the right design. I just wasn't using it.

**Notes as first-class.** I used `note -m` heavily to capture rationale at the moment of the decision — "stretched-link z-index bug was at z:0, should be z:1 over default content; internal links at z:2", "per-status hover overrides needed because base `a:hover` sets border-bottom-color at specificity (0,1,1)." These are thoughts that deserve durability but don't belong in commit messages or in the task description. The event history is the right home.

**The `done` note as a commit-message ghostwriter.** When I went to write the final commit for Phase 1, I pulled most of the "key decisions baked in during the review gate" bullets directly from the notes I'd left on each task. The notes *were* the commit message draft. That workflow wasn't intentional but I'd design for it.

### Frictions

**Flag/subcommand grammar is inconsistent, and that cost me round-trips.**
My first attempt at labeling was `job label --add prototype web` — wrong, `label` has subcommands, not flags (`label add <id> …`). First attempt at notes was `job note <id> "text"` — wrong, needs `-m`. First attempt at blocking was `job block X --on Y --on Z` — wrong, it's `block X by Y`, and only one blocker per call. First attempt at cancel-with-reason was `job cancel <id> -r "…"` — wrong, there's no `-r` short form, only `--reason`. Each guess wasted one round-trip.

The underlying inconsistency: some verbs take subcommands (`label add`, `label remove`), some take flags (`claim --before`, `add --desc`), some take positional keywords (`block X by Y`). There's no grammar I can predict from one to the next. I'd pick a single shape per verb-category and stick with it — see Recommendation #3.

**Can't batch `block`.** Declaring one task blocked on six others meant six sequential calls. For a design-review-gate pattern (gate blocked on N parallel contributions) this is common enough to deserve native support.

**`note` is silent.** `Noted: X` — that's the whole response. No echo of what landed, no character count, no confirmation that my `-m "…"` wasn't accidentally truncated by shell quoting. A longer note where I wasn't sure if an embedded backtick survived escaping had me run `info` afterward to check. A one-line echo — `Noted: X · 213 chars · "Implemented: (1) Sticky layout…"` — would close the loop.

**No lightweight "phase summary" view.** I wanted to answer "where am I in Phase 1?" a dozen times. My options were `job status` (too broad — whole DB) or `job list bYr6R all` (too wide — full subtree with every leaf). I want a middle tier: `job summary bYr6R` → `9 of 15 done · 0 blocked · 3 available · next: NTG9Q`. The `Parent z0OoG: 9 of 15 complete` line that flashes briefly in `done` output is exactly the right compression — just expose it as its own command.

**`info` hard-wraps at 80 cols and breaks prose mid-word** on a wider terminal. Better: don't hard-wrap prose at all; emit raw lines and let the terminal reflow. See R6.

**`list` parentheticals are dominated by note bodies.** Done tasks render as `(note: <hundreds of chars>, labels: X, Y)` on a single wrapped line, putting labels — the most useful secondary scan target — at the end of a wall of text. The fix isn't adornment, it's deletion: drop `note: …` from the list output entirely and let `info` carry notes. See R7.

**`info` doesn't include notes.** This is the single biggest conceptual gap. `info` is the "everything about this task" view — title, description, status, labels, parent, children, dates — but notes are absent. To see them, I had to run `log <id>` and mentally filter for `note` events amidst the claim/heartbeat/done stream. Notes are a *living part of the task's context* (rationale, follow-up caveats, links to related decisions) and belong in `info` by default. My earlier off-the-cuff suggestion was a separate `job notes <id>` command; the better fix — which Ben proposed — is just to include notes in `info`.

**Correction: I missed the default-identity feature entirely.** The DB was configured with a default identity of `claude`; I nevertheless passed `--as claude` on every write for the entire session. The 2026-04-20 feedback doc asked for exactly this feature; it exists; I didn't use it. Reading the quickstart more carefully (or running `job identity` once at session start to check config) would have caught this. My earlier "discipline" framing above was wrong — the tool had the right answer, I just typed over it. This is an *agent onboarding* friction, not a tool friction: there's no signal on first use that a default is configured. A one-line "Default identity: claude (from DB config)" in `job status` output would have surfaced this immediately. That becomes a new recommendation (R0 below, elevated for triviality of implementation + severity of the foot-gun).

### Meta-observation: agent vs human

My friction map is skewed. I can't tab-complete, I can't hit `↑` for history, I can't eyeball a tree; I'm paying the cost of *every character I guess wrong*. So grammar consistency and batch forms matter to me more than they likely do to a human at a real terminal. Conversely: tab completion of task IDs is a huge win for a human and actively useless for me — I already have the IDs in conversation context when I call `job`. I'd skip implementing ID tab-completion on my account, though I can see it being worthwhile for human users of the CLI itself. (Ben's stated preference to have humans use the dashboard rather than the CLI makes this even more skippable.)

---

## Part 2 — Targeted recommendations

Ranked by value to an agent caller, not by implementation size. All are independent and can be landed in any order.

### R0. Surface the default identity in `job status`

**Why:** I didn't discover that this DB had a default identity of `claude` configured until the very end of the session. I passed `--as claude` ~50 times unnecessarily. No error, no hint — it just silently did the same thing with more typing. This is the single highest-ROI change on the list: one line of output, high chance of preventing the same foot-gun on every future agent's first session.

**Shape:** The one-liner from `job status` currently reads `66 open, 59 done (last activity: 51m ago)`. Extend it to include identity state when meaningful:

```
66 open, 59 done (last activity: 51m ago)
Identity: claude (default) · strict mode off
```

If no default is set, say so: `Identity: none set · --as required on writes`. If strict mode is on, say that too. An agent running `job status` as its first call in any session would see the truth about how identity will resolve and act accordingly. Bonus: it plants the idea that `job identity set <name>` exists.

### R1. Multi-value `block` — `job block A by B [C …]`

**Why:** The design-review-gate pattern is "one gate task blocked on N parallel contributions." It recurs naturally. Six sequential `block` calls where one would do is the most tangible per-session cost in this session.

**Shape:** `job block <blocked> by <blocker> [blocker ...]`. Validation: all blockers must exist and be open; no self-blocks; no cycles introduced. Atomic (all-or-nothing). Output: one line per blocker declared, plus one summary line.

### R2. `job summary <parent>` — phase/epic at-a-glance

**Why:** The between-tasks checkpoint that `status` is too broad for and `list … all` is too wide for. The data is already computed and already leaks through `done` output.

**Shape:**
```
job summary bYr6R
Web dashboard v1 · bYr6R
  14 of 15 done · 0 blocked · 0 available · 1 in flight
  Phase 1 — Static HTML prototype (z0OoG): done
  Phase 2 — Server skeleton + SSR foundation (XAtlm): 0 of 8 done · next dXJPQ
```

Two levels: rollup of the target, plus one line per direct child with its own rollup. Stops there; `list` already covers deep trees. Works against any task, not just the root.

### R3. Consistent verb grammar

**Why:** The one friction that cost me the most round-trips in this session. I can't predict from one command whether to use a subcommand, a flag, or a positional keyword.

**Proposal — one rule:** *Multi-operation verbs use subcommands; single-operation verbs use flags/positionals.*

Concretely:

| Verb | Current | Proposed |
|---|---|---|
| `label add/remove` | `label add X a b` | ✓ keep — multi-op |
| `block / unblock` | `block A by B`, `unblock A B` | unify: `block add A B` / `block remove A B` (or keep `block/unblock` as aliases but document them as single-ops) |
| `note` | `note X -m "..."` | ✓ keep — single-op |
| `add` | `add [parent] title --desc ... --before ...` | ✓ keep — single-op |
| `claim` / `release` / `cancel` / `done` | positional id + flags | ✓ keep — single-ops |

The important part isn't the exact scheme — it's *picking one* and sticking to it, plus a short grammar note in `--help` that tells me which shape to expect before I guess. Two lines at the top of `job --help`:

```
Multi-operation verbs (label, block): job <verb> <add|remove> <args>
Single-operation verbs: job <verb> <id> [--flags]
```

That alone would have saved me three of the round-trips I ate this session.

### R4. Echo note body on success

**Why:** Close the loop. Notes are my primary rationale-capture tool; silent success makes me unsure the text landed correctly under shell quoting.

**Shape:** `Noted: <id> · <N chars> · "<first 60 chars of note…>"`. No new flag, no opt-out needed — it's already more useful than `Noted: <id>` and not meaningfully noisier.

### R5. Include notes in `job info`

**Why:** Notes are living context for the task — rationale, follow-up caveats, decisions captured at the moment. They're authored through `note` and stored as events, but conceptually they belong with the task's description and status. Today `info` shows the description but not the notes, so reading my own reasoning from a prior iteration meant running `log <id>` and mentally filtering out claim/heartbeat/done events. Folding notes into `info` makes the "everything about this task" command actually that.

**Shape:** Append a `Notes:` section to `info` output, chronological (oldest-first), one entry per note with actor + relative timestamp + body:
```
Notes:
  [2026-04-22 16:42] @claude (58m ago)
    Implemented: (1) body/page/main sticky layout — body has height:100vh…

  [2026-04-22 17:13] @claude (27m ago)
    Follow-up: bumped canvas height 140 -> 160 (both the div height and…
```

If a task is heavily annotated and output starts to feel long, add a `--no-notes` flag later. Don't add it up-front — the whole point is that notes should be present by default.

(This replaces my earlier off-the-cuff suggestion of a separate `job notes <id>` command. Ben pointed out that `info` is already the right home.)

### R6. Stop hard-wrapping prose in `info`

**Why:** `info` currently wraps description and (once R5 lands) notes at 80 columns, which breaks words mid-line on anything wider and embeds linebreaks that make piping lossy. The modern CLI convention — git, gh, kubectl, cargo — is to emit raw prose and let the terminal soft-wrap. Terminals already do this well; the tool re-doing it worse is strictly harmful.

**Shape:** Drop the hard-wrap in the description and notes sections. Emit each paragraph as a single logical line; keep real newlines (e.g., bullet lists in a note body). Tables and column-aligned output — child lists, label rows — keep their current formatting; the rule applies to prose fields only.

If preserving the 80-col shape is desired for some future pager / printer workflow, gate it behind an opt-in `--wrap <n>` flag rather than making it the default.

### R7. Drop `note: …` from `list` parentheticals

**Why:** When I reproduced the `list` output while writing this doc, I expected to confirm a different friction (blocked-by-claim vs blocked-by-dep, which I'd mis-remembered). Instead the real problem stood out: done tasks render as `(note: <500–1200 chars>, labels: X, Y)`, so the labels — the single most useful secondary filter when scanning a tree — sit at the end of a wall of note text. The parenthetical is structurally sound; it's the note body dominating it that's the issue.

Notes belong in `info` (see R5). The list view should be structural.

**Shape:** Remove `note: <body>` from the list parenthetical. Keep everything else — `labels: …`, `blocked on …`, `(canceled)`, `(reason: …)` for canceled tasks. Optionally, for done tasks whose completion note is genuinely useful at a glance, show just `(done, labels: …)` — no note body. Users who want the rationale run `job info <id>`.

Before:
```
- [x] `NTG9Q` Home view v2 — sticky chrome, link semantics, ordering (note: Home v2 shipped per spec. Sticky chrome (body+page 100vh, main overflow-y:auto…, labels: prototype, web)
```

After:
```
- [x] `NTG9Q` Home view v2 — sticky chrome, link semantics, ordering (labels: prototype, web)
```

Trees become scannable again. Rationale is one `info` call away.

---

**On tab completion (previously R7 in my off-the-cuff list):** skipping. I don't use it; I have task IDs in conversation context when I call `job`. A human at a terminal would benefit, but per Ben's design principle — humans should reach for the dashboard, not the CLI — this is the lowest-value item. If someone else wants it later, the existing `job completion` subcommand is a good starting point.

---

## Appendix: session at a glance

- ~50 `job` calls across ~3 hours of work
- 6 tasks created (with `add`), 6 blocks declared, 6 tasks claimed + noted + done
- 2 task statuses checked (`status`, `list bYr6R all`)
- 0 tasks released, 1 canceled (the identity-test task), 0 reopened
- 4 commands failed on first attempt due to grammar guesses (R3 hits) — one of them (`cancel -r`) surfaced while drafting this doc, not during the Phase 1 work itself
- 1 silent-note-worry diagnostic (R4 hit)
- 12+ instances of wanting R2 (phase summary)
- ~50 redundant `--as claude` passes (R0 hit) — every write I made

---

## Plan

Concrete `job import` YAML for R0–R7 plus a consolidated docs sweep. Each recommendation is one child of a single umbrella task; tests are bundled per-child (strict red/green per CLAUDE.md). The only cross-child ordering is `r1` waiting on `r3` so multi-value blockers ship on the canonical `block add` form.

```yaml
tasks:
  - title: CLI improvements from 2026-04-22 feedback
    ref: cli-feedback
    desc: |
      Eight agent-facing CLI ergonomics fixes grouped under one
      umbrella, drawn from the R0–R7 recommendations in
      project/2026-04-22-opus-feedback.md. Each child stands alone and
      can be landed in any order except for the explicit blockedBy
      between R1 and R3. Tests are bundled with each change; the docs
      task at the end covers README touch-ups across every R.
    labels: [cli, feedback, ergonomics]
    children:

      - title: R0 — Surface default identity in `job status`
        ref: r0
        desc: |
          Extend the `job status` one-liner to report the configured
          identity state so an agent running `job status` at session
          start can see whether `--as` is needed.

          Shape (default set):
            66 open, 59 done (last activity: 51m ago)
            Identity: claude (default) · strict mode off
          No default:
            Identity: none set · --as required on writes
          When strict mode is on, include that clause.

          Touchpoints:
          - internal/job/status.go — RunStatus / RenderStatus (lines
            21–75). Extend StatusSummary with IdentityDefault and
            Strict fields; render a second line.
          - internal/job/config.go — GetDefaultIdentity (line 41),
            IsStrict (line 52). Both already exist; call from RunStatus.
          - cmd/job/status_test.go — cases for (default set / no
            default / strict on) covering struct + rendered output.
        labels: [cli, status, identity]

      - title: R1 — Multi-value `block add` (N blockers in one call)
        ref: r1
        desc: |
          Accept multiple blockers in a single invocation so the
          design-review-gate pattern (one gate blocked on N parallel
          contributions) does not require N sequential calls.

          Shape: `job block add <blocked> <blocker> [blocker ...]`.
          - All blockers must exist and be open.
          - Atomic: all-or-nothing in a single transaction.
          - Reject self-block and any blocker that would introduce a
            cycle, checked across the full input set.
          - Emit one "blocked" event per edge so the event stream stays
            one-event-per-edge; print one line per edge plus a summary.

          Touchpoints:
          - cmd/job/block.go (line 14) — relax ExactArgs(3); accept
            variadic blocker tail on `block add`.
          - internal/job/blocks.go:9–59 — RunBlock becomes
            RunBlockMany or takes a []string; keep the per-edge
            recordEvent(..., "blocked", ...) call at line 51.
          - Tests: cycle detection across multi-blocker input;
            duplicate blockers collapsed or rejected (pick one and
            document); atomicity on mid-list failure (nothing persists).

          Blocked by R3: the canonical form after this plan is
          `block add`; ship multi-value on the new surface, not the
          legacy `block X by Y`.
        labels: [cli, block]
        blockedBy: [r3]

      - title: R2 — `job summary <parent>` — phase/epic at-a-glance
        ref: r2
        desc: |
          New command filling the gap between `status` (whole DB, too
          broad) and `list … all` (full subtree, too wide).

          Shape:
            job summary bYr6R
            Web dashboard v1 · bYr6R
              14 of 15 done · 0 blocked · 0 available · 1 in flight
              Phase 1 — Static HTML prototype (z0OoG): done
              Phase 2 — Server skeleton + SSR foundation (XAtlm): 0 of 8 done · next dXJPQ

          One rollup line for the target plus one line per direct
          child with its own rollup. Stops there — `list` already
          covers deep trees. Works against any task, not just the root.

          Touchpoints:
          - New cmd/job/summary.go with newSummaryCmd(); register in
            cmd/job/commands.go:62 (newRootCmd).
          - internal/job/tasks.go:854 — ComputeDoneContext already
            computes parent rollup counts; extract/generalize into a
            subtree rollup returning done/blocked/available/in-flight
            counts plus the next-available id.
          - Tests: empty subtree, all-done subtree, mixed, nested
            (summary against an intermediate node), single-leaf parent.
        labels: [cli, summary]

      - title: R3 — Consistent verb grammar + typo aliases with helper warnings
        ref: r3
        desc: |
          Settle the grammar inconsistency that cost the most
          round-trips in the 2026-04-22 session, and catch two common
          typos up front.

          Rule: multi-operation verbs use subcommands; single-operation
          verbs use positional + flags.

          Concrete work:
          1. Introduce `block add` / `block remove` as the canonical
             forms. Keep `block` and `unblock` as aliases; on every
             alias invocation emit a one-line stderr notice, e.g.:
               note: `job block` is an alias for `job block add`; prefer the canonical form.
             Same shape for `unblock` → `block remove`.
          2. Alias `job ls` → `job list` with the same "prefer `list`"
             stderr notice.
          3. Alias `job show <id>` → `job info <id>` with the same
             notice.
          4. Add two lines to the top of `job --help`:
               Multi-operation verbs (label, block): job <verb> <add|remove> <args>
               Single-operation verbs: job <verb> <id> [--flags]

          Touchpoints:
          - cmd/job/block.go, cmd/job/unblock.go — restructure into a
            `block` parent with `add` / `remove` subcommands; keep
            top-level `block` and `unblock` registered, delegating
            after printing the notice.
          - cmd/job/list.go — register `ls` via Cobra's Aliases field
            + PersistentPreRun stderr notice gated on cmd.CalledAs().
          - cmd/job/info.go — register `show` the same way.
          - cmd/job/commands.go — help-text preamble.
          - Tests: each alias emits the warning exactly once per
            invocation, exits 0, and runs the canonical action; help
            text contains both grammar lines; notice goes to stderr
            (not stdout) so pipelines are unaffected.
        labels: [cli, grammar, aliases]

      - title: R4 — Echo note body on success
        ref: r4
        desc: |
          Close the loop on `note`. Silent `Noted: <id>` leaves an
          agent unsure whether shell quoting truncated the body.

          Shape:
            Noted: <id> · <N chars> · "<first 60 chars of note…>"
          - Truncate on a word boundary where possible.
          - Elide with `…` only when the body exceeds 60 chars.
          - Collapse embedded newlines to spaces in the preview (real
            body is stored unchanged).

          Touchpoints:
          - cmd/job/note.go:75 — replace `Noted: <id>` print.
          - internal/job/tasks.go:669–732 — RunNote already has the
            final text; return length + preview alongside the id (or
            compute at the call site from the returned text).
          - Tests: exact preview formatting at <60, =60, >60 chars;
            bodies with newlines/tabs; bodies containing backticks and
            quotes (round-trip the preview safely); empty-body
            rejection behavior preserved.
        labels: [cli, note]

      - title: R5 — Include notes in `job info`
        ref: r5
        desc: |
          `info` is the "everything about this task" view, but notes —
          the primary rationale-capture artifact — are absent. Fold
          them in.

          Shape: append a `Notes:` section, chronological (oldest
          first), one entry per note with actor and relative
          timestamp:
            Notes:
              [2026-04-22 16:42] @claude (58m ago)
                Implemented: (1) body/page/main sticky layout…
              [2026-04-22 17:13] @claude (27m ago)
                Follow-up: bumped canvas height 140 -> 160…

          Notes are events of type "noted" (internal/job/tasks.go,
          RunNote around line 669) — fetch and filter from the event
          stream scoped to the task.

          Touchpoints:
          - internal/job/tasks.go:1147–1206 — RunInfo currently fetches
            task, parent, children, blockers, labels; extend to load
            notes (event history filtered to type="noted").
          - internal/job/format.go:155–199 — RenderInfoMarkdown; new
            section after the existing fields.
          - internal/job/format.go — RenderInfoJSON parallel update so
            the JSON surface gets a `notes` array.
          - Do NOT add `--no-notes` up front; the point is that notes
            are present by default. Revisit only if feedback warrants.
          - Tests: zero notes (section omitted / empty JSON array),
            one note, many notes; ordering; actor + relative-time
            formatting; timestamps rendered deterministically in tests.
        labels: [cli, info, notes]

      - title: R6 — Stop hard-wrapping prose in `job info`
        ref: r6
        desc: |
          `info` today hard-wraps description (and, post-R5, notes) at
          80 columns, breaking words on wider terminals and embedding
          newlines that make piping lossy. Modern CLI convention (git,
          gh, kubectl, cargo) is to emit raw prose and let the
          terminal reflow.

          Shape: drop the hard-wrap on prose fields (description, note
          bodies). Emit each paragraph as a single logical line; keep
          real author-supplied newlines (bullet lists inside a note).
          Tables and column-aligned output — child lists, label rows —
          keep their current formatting; this rule applies to prose
          only.

          If a future pager/printer workflow wants the wrapped shape,
          gate it behind an opt-in `--wrap <n>` flag rather than
          making it the default.

          Touchpoints:
          - internal/job/format.go:155–199 — RenderInfoMarkdown; locate
            the wrap helper and restrict its scope (or remove it for
            prose fields).
          - Tests: long-paragraph description survives intact on both
            narrow and wide pseudo-terminals; embedded newlines
            preserved; tabular rows still column-aligned.

          Independent of R5; if both ship, this rule covers the new
          Notes section too.
        labels: [cli, info, formatting]

      - title: R7 — Drop `note:` body from `list` parentheticals
        ref: r7
        desc: |
          Done tasks currently render as
            (note: <500–1200 chars>, labels: X, Y)
          burying the label scan target behind a wall of note text.
          Notes belong in `info` (see R5); `list` should be
          structural.

          Shape: remove the `note: <body>` clause from the
          parenthetical. Keep every other component — `labels: …`,
          `blocked on …`, `(canceled)`, `(reason: …)` for canceled
          tasks, `claimed by …` for claimed.

          Before:
            - [x] `NTG9Q` Home view v2 (note: Home v2 shipped per spec. Sticky chrome…, labels: prototype, web)
          After:
            - [x] `NTG9Q` Home view v2 (labels: prototype, web)

          Touchpoints:
          - internal/job/format.go:92–128 — listStateParens; drop the
            "note: " + completionNote branch (lines 96–97).
          - Tests: done task with / without labels; canceled task
            preserves `(canceled)` and `(reason: …)`; claimed task
            preserves `claimed by …`; blocked-by markers unchanged.
        labels: [cli, list, formatting]

      - title: Docs sweep — README updates for R0–R7
        ref: cli-feedback-docs
        desc: |
          Single consolidated docs update once the behavior changes
          have landed. README.md is the unified docs location in this
          repo (no separate docs/content/ directory).

          Sections to touch:
          - `status` (~line 135): new identity line in the summary.
          - `block` / `unblock` (~line 155): canonical `block add` /
            `block remove` form; legacy aliases + helper warning;
            multi-blocker form.
          - `note` (~line 148): new echo-on-success format.
          - `info` (~line 133): Notes section; no more hard-wrap.
          - `list` (~line 129): updated parenthetical shape; mention
            `ls` alias.
          - Add a `summary` section alongside `status`.
          - Grammar preamble on the commands table (~line 97).

          Update CLAUDE.md if any agent-visible invariant changed
          (e.g. surface the "prefer canonical form; aliases warn"
          convention for future agent sessions).
        labels: [docs]
        blockedBy: [r0, r1, r2, r3, r4, r5, r6, r7]
```

