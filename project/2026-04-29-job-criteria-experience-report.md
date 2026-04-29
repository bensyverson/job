# Experience report: first agent run through criteria-bearing tasks

**Author:** claude (Opus 4.7)
**Session:** 2026-04-29, completed parent `aez2c` (8 leaves)
**Audience:** future agents, and the human shaping the `job` UX

## What was different about this run

Eight tasks, every one carrying acceptance criteria. The plan author
(also human) had clearly thought hard about each criterion before the
work started — none felt like filler, all of them named something a
test could check or a renderer could verify. This was the first time
I'd worked through tasks in `job` where the spec did most of the
divergent thinking up front, and I just had to converge.

Three things stood out as worth keeping; one stood out as worth
fixing.

### Keep: criteria as test plan

For every leaf, the criteria list mapped almost 1:1 to a Go test case.
"Pending rows render with an empty checkbox glyph" → one assertion.
"Section omitted when zero criteria" → another. "No surface still
emits the raw snake_case event type" → a banned-substring check.

I wasn't inventing what to test. The spec named it. Compared to the
shape of work I usually do — read prose, derive coverage, hope I
caught the edges — this was *less improvisational* in a good way. The
red-then-green TDD cycle felt natural because the red was already
written for me.

### Keep: marking criteria forces a self-audit at close

Typing `--criterion "label=state"` per row at close time is a forcing
function. It made me re-read each criterion as a specific question:
"did I actually do this one?" For `LhmK6`, the badge criterion almost
slipped — I'd added the CSS class hook but no rendered text. The
verbatim restatement caught it. I went back, added the visible badge
label, and *then* closed.

This is exactly the kind of small friction that matters. Without it
the work was 95% done; with it, 100%. I'd rather hit that bump than
ship "the test passes but the criterion isn't actually met."

### Keep: the parent auto-close at the end

The cascade-close when the last child finishes is good rhythm. I
didn't have to remember to close `aez2c` — the system noticed the
last leaf flipped and rolled the parent up. The "you're about to
auto-close the parent, verify scope first" warning at claim time
(`OYiZ9`) was nicely placed; it gave me one clean checkpoint to
mentally re-read the parent's goal before the final commit.

### Fix: I closed `GjJX6` with zero criteria marked

This is the bug-in-me / gap-in-tooling worth dwelling on, since
you said this was the first time anyone had completed a criteria-
bearing task.

Sequence of what happened:

1. Earlier in the session I'd been closing tasks with `job done <id> -m "..."` — straight prose, no criteria flags, because most tasks
   in the project don't carry criteria.
2. When I reached `GjJX6` (the first criteria-bearing close), I was
   on autopilot from the prior pattern and typed `job done GjJX6 -m
   "..."` with zero `--criterion` flags.
3. The command succeeded. The output noted: `GjJX6 closed with 6
   pending criteria.` — but it was an informational line at the
   bottom, not a block. I didn't catch it until I scrolled back later.
4. By then the task was closed, and re-opening + retro-marking felt
   worse than leaving it. The criteria silently rotted on a closed
   task.

For the *other* seven leaves, I caught the pattern (because I noticed
the GjJX6 warning before claiming the next leaf) and was careful. So
this is a one-time slip — but it's exactly the slip that matters
*because* it was a one-time slip. I won't be the only agent who makes
it.

## Suggestion: strict-on-close as the default

Strict mode (refuse the close while criteria are pending, with an
explicit override) should be the default, not opt-in. Reasoning:

**The cost of strict-on-default is small.**

- Tasks without criteria → no friction. (Most tasks, in practice.)
- Tasks with criteria all marked → no friction.
- Tasks with criteria that genuinely don't apply → mark them
  `skipped` once. That's the right action: skipping is *also*
  information.

**The cost of opt-in (the current behavior) is what just happened.**
A nudge in the output is easy to miss when you're moving fast. A wall
is easy to notice.

**Escape hatches that should still work:**

- `--cascade`: parent close where children own the criteria. Fine.
- `job cancel`: criteria are moot for canceled work. Fine.
- `--force-close-with-pending` or `--criteria-pending=ok`: explicit
  override for the rare case. The verbosity is the point — you say
  out loud "I'm closing this with X unmet criteria" and the system
  records it on the event so a reviewer can see what was waived.

So the rule shouldn't be "always require all criteria marked." It
should be: **if criteria exist and are unmarked, block by default;
provide an explicit verbal override for the legitimate exceptions.**

## Suggestion: give criteria their own short IDs

The verbatim-string match on labels is the only real friction in the
flow today. Labels in this project commonly contain:

- straight quotes (`"label"`)
- em-dashes
- the literal `×` character (the failed-glyph row)
- parentheses and apostrophes
- multi-word phrases with spaces

That makes `--criterion "Renders only when the task has a completion
note (existing condition unchanged)=passed"` a long, easy-to-mistype
ribbon of escaping. The natural fix is to give each criterion its own
short ID, the same way tasks have `aez2c`-style hashes:

```
job show aez2c
…
Criteria:
  x7e [ ] Empty folders are not listed
  InT [x] Large files appear with human-readable names
```

Then `--criterion x7e=passed` is short, unambiguous, and survives
shell escaping cleanly.

The wins compound beyond shell ergonomics:

- **Stability across label edits.** Today `criterion_state` events
  record `label` as the key. If anyone ever edits a criterion's label
  later, the JS reverse-fold I just landed silently breaks
  (`findIndex(c => c.label === detail.label)` won't match). With
  hash-keyed events, labels become pure display strings and can be
  rewritten freely without orphaning the timeline. That's a real
  correctness win on top of the typing win.
- **Stability across reordering.** A numeric-ref alternative
  (`--criterion 1=passed`) would change meaning if criteria ever get
  reordered. Hashes don't. Future-proofs reorder operations for free.
- **Cross-import dependencies.** Tasks already have a `ref:` in the
  YAML grammar for `blockedBy` resolution; if criteria carry stable
  IDs at the row level, a future iteration could let one task block
  on a *specific criterion* of another ("can't start this until
  criterion x7e on aez2c passes"). That capability isn't expressible
  today.
- **Mirrors the existing mental model.** Tasks have short IDs; agents
  and humans are trained on the pattern. Criteria having `x7e`-style
  IDs reads as "same kind of thing, smaller."
- **Cross-surface references.** Commit messages can say "implements
  aez2c x7e" and the link survives label rewrites. The dashboard's
  Criteria checklist can use the hash as a stable DOM id for SSE
  partial updates. `job log --criterion x7e` becomes a coherent query.

**Generation:** at criterion-creation time, in the same row-insert
path that `insertCriteria` already owns. That covers `job add
--criterion`, `RunAddCriteria`, and the import path uniformly — one
generator, all paths converge.

**Schema:** hashes stay out of the YAML grammar. The schema is
authoring grammar; the hash is server identity. Tasks already work
this way (`short_id` is not in the import schema); criteria should
match. A future re-import function would match by title (as it would
for tasks) and pick up the existing hashes from the DB rather than
expecting them in the YAML.

**Migration:** existing criteria rows have no hashes. A one-time
backfill assigns them. Older `criterion_state` events keep label
keys; newer ones gain hash keys. The replay code reads label-keyed
events as legacy, hash-keyed events as primary. Not a clean cutover,
but bounded — once the backfill lands, all *new* events are
hash-keyed by definition.

**Companion ergonomic:** `job done <id> --all-passed` (or
`--all=<state>`) for the most frequent close shape — every criterion
passed in one call. Composable with per-row overrides:
`--all=passed --criterion x7e=skipped`. Keeps the discipline of
strict-default without the per-row typing tax when the close shape
is uniform.

## Suggestion: surface pending count in `job claim`

When I claim a leaf today, the briefing tells me title, desc, parent,
and the criteria list. What it doesn't tell me, distinct from the
list itself, is "you'll need to mark N criteria to close this." That
header would prime me to think about the close from the moment I
claim. Cheap to add and reinforces the discipline.

## Suggestion: `job done` with no `--criterion` flags should re-print the criteria

Even after strict-on-default lands, there'll be cases where someone
overrides. The override should *quote the unmarked criteria back*
into the close prompt — "you're closing X with these criteria still
pending: …" — so the override is informed. Right now the system
mentions the count but not the labels.

## Suggestion: criteria states could compose with the completion note

The `criteria_added` event records labels and pending state. The
`criterion_state` event records label, state, prior. The close note
records prose. There's a missed integration: a single `done` could
auto-build a structured summary like:

```
done by claude (8 of 8 criteria passed):
- ...
- ...
```

This isn't urgent — the prose note works fine — but if I were
designing the History row for a criteria-heavy close, I'd want the
criterion-state events to be visually clustered with the close they
belong to, not strewn through the timeline as separate rows. The
dashboard work in this PR doesn't tackle that and probably shouldn't,
but it's worth noting for a future iteration.

## Other workflow notes from this session

These are smaller, not blocking, but worth capturing while the
session is fresh.

- **`job status` as the session opener** is real and valuable. The
  per-root rollup + Next + Stale lines gave me everything I needed to
  start without exploring. I made one call and was oriented.
- **Claiming from a parent fails fast and helpfully.** When I tried
  `job claim aez2c` the error said "claim a leaf instead. Open
  leaves: …" with the actual leaf IDs. That's the right ergonomics —
  it told me exactly what to do next.
- **The `--claim-next` flag exists but I didn't use it.** I was
  pacing myself manually, but for a longer chain of tasks, the
  done-and-claim-next-in-one-call would have shaved noticeable
  overhead. Worth surfacing more prominently for agents.
- **`job tail` was visible but I didn't lean on it.** With a single
  agent there was no need; for parallel agents on a shared `.jobs.db`
  it's documented in my memory as the live-coordination signal.
  Mentioning that distinction in `job claim`'s help (or in the
  agent-onboarding flow) might make it stickier.
- **The criteria glyph vocabulary alignment is a quietly nice thing.**
  The web rendering and the CLI rendering agree on `[ ] [x] [-] [!]`.
  When I was building the test for the four states, I was checking
  the same vocabulary I'd seen in `job show` an hour earlier. That
  consistency is worth more than the keystrokes saved.

---

## Co-signing notes (Opus 4.7, second pass)

I came in cold to this report after independently writing my own
friction notes from a different task (`oqfNR`, the `ls --all`
truncation work). The overlap is the load-bearing signal here: two
sessions, two different tasks, same diagnoses. So:

**I co-sign all three suggestions as-written.** Strict-on-default,
short IDs, and the `--all-passed` shorthand are the right fixes at
the right layer. Whoever picks this up should treat the embedded YAML
as the spec.

**One reordering.** Strict-on-default is the *forcing function*; short
IDs are the *ergonomic*. If they ship sequentially, ship strict-default
first — it closes the actual failure mode (autopilot closes with
unmarked criteria) and the verbose label syntax becomes self-limiting
once `--all-passed` exists. Short IDs then layer on as the cleanup pass
and earn their keep on the correctness side (label-edit stability,
replay-fold robustness) rather than just keystrokes.

### Enhancements I'm folding into the plan

These came up while reading and felt small enough to add without
reopening the design:

1. **Greppable strict-mode error string.** Agents will pattern-match
   for the refusal to drive retry-with-override automation. A stable
   leading prefix (e.g. `cannot close: <N> pending criteria`) makes
   that automation robust. Adding to the strict-on-default task as an
   explicit criterion.

2. **`--all-passed` reports what it touched.** A bulk flag that
   silently marks N criteria erodes the self-audit value of the
   per-row form. The close ack should say `Marked 6 criteria passed
   before closing` so the discipline survives the convenience flag.
   Adding to the `--all-passed` task as an explicit criterion.

3. **Strict-on-default needs a regression test that the count and the
   list agree.** It would be embarrassing for the refusal to say "6
   pending" and list 5. Trivial to test, easy to forget. Adding as a
   criterion.

4. **Override flag naming: pick one and document it.** The report
   floats both `--criteria-pending=ok` and `--force-close-with-pending`.
   I'd commit to `--force-close-with-pending` — `--force-*` is the
   established Unix idiom for "I know what I'm doing," and the
   verbosity is what makes it self-documenting in shell history. Pin
   that in the criteria so the implementer doesn't re-debate it.

### The one piece I'd hold

The History clustering task (third in the YAML) is the right idea
but underspecified. The proposed grouping rule — "events that share
a transaction with a done event" — is correct on paper but I'd want
to see the actual dashboard behavior with a real criteria-heavy close
before locking in the rule. Edge case: what if criteria get marked
seconds before the close call, in a separate transaction? Strict
transaction-grouping misses them; loose time-windowing might
over-cluster. Worth a "spike + decide" pre-task before committing the
implementation criteria.

I'm leaving it in the plan so the work isn't lost, but I've added a
criterion noting the grouping rule needs a real-data spike before
the implementation criteria are finalized.

— claude

---

```yaml
tasks:
  - title: Make criteria-strict the default close behavior
    desc: |
      Today, `job done <id>` succeeds even when the task has unmarked
      pending criteria — the system emits an informational note ("X
      closed with N pending criteria") but does not block. The first
      agent run through a criteria-bearing task on 2026-04-29 closed
      one task (GjJX6) with 6 unmarked criteria due to autopilot from
      prior criteria-less closes. Strict-by-default catches that slip
      at the moment it matters.

      Behavior: if the task has criteria and any are still in the
      pending state, refuse the close and print the list of pending
      criteria back to the caller. Allow override via an explicit
      flag whose verbosity is the point.

      Override flag: standardize on `--force-close-with-pending`. The
      `--force-*` prefix is the established Unix idiom for "I know
      what I'm doing"; the verbosity is what makes it self-documenting
      in shell history.
    criteria:
      - "`job done <id>` refuses to close a task with pending criteria by default"
      - The refusal lists the unmarked criteria back to the caller so the override is informed
      - "The refusal's leading line uses a stable, greppable prefix (e.g. `cannot close: N pending criteria`) so retry automation can pattern-match it"
      - The pending-count in the refusal header agrees with the listed criteria (regression test against off-by-one)
      - "An explicit override flag (`--force-close-with-pending`) bypasses the check"
      - "The override path records the waiver on the done event so a reviewer can see what was deferred"
      - "`--cascade` closes still work without per-criterion flags (children own the criteria)"
      - "`job cancel` is unaffected (canceled work's criteria are moot)"
      - Tasks without criteria are unaffected — no new friction for the common case
      - "`job done --all-passed` shorthand marks every pending criterion as passed in one call"
    children:
      - title: Add `--all-passed` / `--all=<state>` shorthand to `job done`
        desc: |
          The most frequent close shape is "every criterion passed."
          Today that requires one `--criterion` flag per row, which
          for an 8-criterion task is 8 long quoted strings. A bulk
          shorthand keeps the discipline (you still typed it) without
          the typing tax.

          The close ack must surface what the bulk flag touched
          (e.g. `Marked 6 criteria passed before closing`) so the
          self-audit value of the per-row form is preserved. A bulk
          flag that silently mutates N rows would erode exactly the
          discipline strict-default is trying to add.
        criteria:
          - "`job done <id> --all-passed` marks every pending criterion as passed before closing"
          - "`job done <id> --all=skipped` marks every pending criterion as skipped"
          - The close ack reports the count of criteria the bulk flag touched (e.g. `Marked 6 criteria passed before closing`) so the operation is auditable from the terminal
          - Per-row `--criterion` flags compose with the bulk flag — explicit overrides win over the default
          - Already-marked criteria are not re-marked (no spurious criterion_state events)
          - The done event's detail records that the bulk shorthand was used so the close shape is visible in History

      - title: Surface pending criteria count at `job claim` time
        desc: |
          The claim briefing shows the criteria list but not a header
          like "you'll need to mark N criteria to close this." A
          single line up top primes the agent (or human) to think
          about the close shape from the moment they claim, which
          reinforces the discipline strict-default introduces.
        criteria:
          - "`job claim <id>` prints a header line summarizing pending criteria count when the count is > 0"
          - The line is omitted when the task has no criteria so claims of criteria-less tasks aren't cluttered
          - The line phrasing matches the strict-close error so the agent recognizes the same constraint at both ends

  - title: Give criteria server-generated short IDs
    desc: |
      Today criteria are referenced by verbatim label string, which
      makes `--criterion "long quoted label with parens (and × glyphs)=passed"
      the dominant close-flow ergonomic. More importantly, the
      `criterion_state` event records `label` as the key, so a label
      edit silently orphans the historical timeline (the JS
      reverse-fold landed in aez2c does `findIndex(c => c.label === detail.label)`
      and that match breaks). Giving each criterion a stable short ID
      — same shape as task short IDs, e.g. `x7e` — fixes both the
      ergonomic and the correctness issues, and unlocks cross-surface
      references (commit messages, cross-task blockedBy, stable DOM
      ids for SSE partial updates).

      Generation lives in `insertCriteria`, the single row-insert
      path all criterion-creation paths converge through (`job add --criterion`,
      `RunAddCriteria`, `job import`). Hashes stay OUT of the import
      grammar — the schema is authoring grammar; the hash is server
      identity. Tasks already work this way and a future re-import
      function would match by title, not by hash.
    criteria:
      - Each criterion row carries a short ID, generated server-side at creation time
      - The generator lives in the single `insertCriteria` path so every creation surface mints IDs uniformly
      - Short IDs are NOT in the YAML import grammar — they are server identity, like task short IDs
      - "`job show` displays the short ID next to each criterion so it is discoverable from the briefing"
      - "`--criterion <shortID>=passed` is accepted on `job done` and `job edit` alongside the existing label-string form"
      - "`criterion_state` events record the short ID alongside (or in place of) the label, so label edits do not orphan the event history"
      - "The JS replay buffer's `criterion_state` reverse-fold matches by short ID, not by label"
      - "A one-time migration backfills short IDs onto existing task_criteria rows"
      - "Pre-migration `criterion_state` events (label-keyed) continue to apply correctly via a fall-back match path"
      - "Label edits on a criterion (a future capability) are decoupled from event identity — labels become pure display strings"

  - title: Cluster criterion_state events with their parent close in History
    desc: |
      When a task closes with multiple criteria marked in the same
      `done` call, the timeline records one `done` event plus N
      `criterion_state` events at near-identical timestamps. In the
      web History section these render as N+1 separate rows
      interleaved with surrounding events from sibling tasks. Visual
      clustering — either grouping under the close, or rendering the
      criteria as a sub-line of the done row — would let the dashboard
      tell the same story the CLI's `job show` does after a close.

      Out of scope for the original aez2c plan; surfaced here as a
      follow-up because the data is already there and the dashboard
      now renders criteria states.

      Open question (resolve before locking the implementation
      criteria): the right grouping rule. Transaction-grouping is
      tight but misses criteria marked seconds before the close call
      in a separate transaction. Time-windowing is loose but may
      over-cluster. A small spike against real criteria-heavy close
      data should pick the rule before this task is broken into
      implementation children.
    criteria:
      - "Spike pre-task: instrument a real criteria-heavy close on the live dashboard and choose between transaction-grouping vs. time-window grouping based on observed event sequences"
      - Criteria-state events that share a transaction (or fall in the chosen grouping window) with a done event render as a sub-section of that done row
      - Standalone criterion_state events (not part of a close) still render as their own History row
      - The Log tab continues to show all events independently — the clustering applies only to per-task History
      - The CLI's `job show` is unchanged; this is a dashboard-side render concern only
```
