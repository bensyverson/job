# `job` CLI feedback — Phase 10 session

**Date:** 2026-04-28
**Author:** claude (default identity)
**Context:** Closed YsWiZ in one session — six children (mdYup, l225e,
o6Gtb, ywscG, 666VP, A5vdi) plus RcbTK canceled. Work bundled into
commit `bb201ce`. This is a working-from-the-inside feedback pass on
the parts of the CLI that helped or got in the way during that
session.

## What worked well

- **`job status` as session-opener.** One call, full briefing — open
  count by root, the single "next" suggestion, and identity confirm
  in one block. I never reached for `job ls` to orient. The Phase 10
  rollup ("Web dashboard v1 (bYr6R): 141 of 149 done · next mdYup")
  was exactly the framing I needed to scope the session.

- **Auto-cascade close.** When `job done A5vdi` flipped to *Done*,
  both YsWiZ and the parent bYr6R auto-closed. Reading "Auto-closed:
  YsWiZ … Auto-closed: bYr6R" in the same response made it feel like
  the project had a real terminal moment, not "now go close the
  parents too." This is a feature that pays off late and quietly,
  exactly when the work is done and you'd otherwise lose track.

- **The `--claim-next` chord on `job done`.** I didn't use it this
  session because I was working through Phase 10 children explicitly,
  but the `claim → note → done -m "…" --claim-next` shape is the
  right primitive for a long autonomous run.

- **`job show <id>` rolling up children with checkboxes.** Reading
  YsWiZ once gave me both the description and the leaf punch list
  with `[x]` / `[ ]` markers; I didn't need to issue per-child
  `show` calls until I picked one to claim. The "blocked on …"
  annotation on A5vdi was the single piece of metadata that told me
  the verification gate sequenced *after* the leaves — without it I
  might have tried to claim it first.

- **`job claim` lease + 30m TTL** kept me honest. I didn't see a
  re-claim conflict, but the displayed expiry meant I could reason
  about "if I get stuck, this lease will release on its own" rather
  than worrying about leaving a stale claim.

- **`job cancel <id> -m "…"`** worked exactly the way `done` works,
  including the auto-cascade math. Canceling RcbTK didn't feel like
  a different tool — same shape, same feedback.

## What got in the way

### 1. The 30-minute claim TTL is too short for a focused session

I was working YsWiZ for ~2 hours of wall clock in a single session.
Each individual claim was on a child (mdYup, l225e, …) and each took
10–30 min, so no single child expired. But if I'd been working a
single big child end-to-end I'd have had to either re-claim it
mid-flight or risk an auto-release.

Either:
- bump the default TTL to 60–90 min, or
- expose `--ttl 90m` on `claim` so a long task can ask for more
  headroom up front, or
- have `note` extend the lease (it doesn't appear to, today).

### 2. Output noise on `claim` and `done` is a little chatty

Every `job claim X` printed the full `show` block again. Useful the
first time you claim something; redundant when you're already
holding the description in conversation context. A `--quiet` flag
on `claim`/`done`/`cancel` that prints just the one-line "Claimed
X / expires in 30m" + "Next: Y / Parent: N of M complete" would
keep the receipts tight. (The `Next:` and `Parent:` lines are the
actually-useful parts and they're already laid out well.)

### 3. There's no "what did I just close?" recall

After auto-cascade, I had no easy way from the CLI to ask "show me
the ids of everything that closed in the last 5 minutes attributed
to me." I read the cascade summary lines off the `job done` output
and that was enough — but if I'd been multi-tasking I'd have lost
that thread. A `job log --as me --since 5m` style filter (or
extending `job log` if it already exists) would close the loop.

### 4. The `--as` flag default isn't always obvious in error states

`Identity: claude (default) · strict mode off` was clear at session
start, but if I'd been switching identities a lot I'd have benefited
from `claim`/`done` echoing `as=claude` somewhere in their summary
line so an "oops, I closed that as the wrong actor" mistake would be
visible at the moment of action, not on the next `status`.

### 5. The verification-gate child carried no machine-readable scope

A5vdi's description listed three concrete checks ("test suite",
"5k+ events walk", "cold load <200ms / live update <100ms"). I had
to read it as prose, decide which were automatable, and write a
free-text note explaining which I deferred. Some way to attach
acceptance criteria as structured items (a `criteria:` array, each
with done/skip/fail status) would let a verification job tell the
next reader "automated parts: green; manual parts: deferred" without
relying on me to spell that out in the note.

### 6. No way to add a child without re-importing

Mid-session I considered splitting one of the bigger children into
sub-children to track granularly. I would have used `job add` for
that — and it works — but the YAML-import-then-`job done`-the-rest
ergonomic isn't there for an in-flight tree. A `job split <id>`
that claims the parent, opens children from a small inline list, and
re-blocks the parent on them would have made me reach for it. As it
stood, I just kept the work mental rather than tracking it in the
graph.

### 7. The "blocking" relationship is invisible from the leaf

`job show A5vdi` showed `Blocking: mdYup, o6Gtb, l225e, ywscG, 666VP`
which is correct (A5vdi blocks *on* those — though the label reads
ambiguously, see below). But none of the leaf `show` outputs
mentioned "blocks A5vdi." So when I was working mdYup I couldn't
tell from its detail page that closing it would unblock something
downstream. A `Blocks:` (outbound) field on the children's `show`
block, alongside `Blocked by:` on the gate, would make the dataflow
symmetric.

### 8. "Blocking" vs "Blocked by" wording

`Blocking: mdYup, o6Gtb, l225e, ywscG, 666VP` on A5vdi reads
naturally as "A5vdi is blocking those" — the opposite of the truth
(A5vdi is *blocked by* them). Standard convention is `Blocked by:`
for the inbound edge. Right now you have to read the parent context
to know which way the arrows point.

## Themes

- **The CLI handles a 6-leaf phase + verification gate without
  ceremony.** No friction at session start, clear cascade closure
  at end, the right level of detail in between. The structural bones
  are good.
- **The remaining wins are about *durations* (longer leases),
  *precision* (criteria, blocks symmetry), and *quiet* (less noise
  on claim/done).** None require schema changes; most are flag
  additions, output tweaks, or one new subcommand.
- **The auto-memory feedback loop felt cleaner than the CLI did at
  expressing acceptance criteria.** I wrote a 532-char note on
  A5vdi to capture which verification dimensions passed and which I
  deferred. Structured criteria would replace that paragraph with
  rows.

## Recommended changes (importable plan)

The following YAML follows the `job schema` grammar and can be
imported with `job import` once reviewed.

```yaml
tasks:
  - title: CLI ergonomics — Phase 10 feedback
    desc: |
      Working-from-the-inside feedback after closing the Web Dashboard
      v1 (bYr6R) phase in one session. Source:
      project/2026-04-28-job-cli-phase10-feedback.md.
    labels: [cli, dx, feedback]
    children:
      - title: Extend the default claim TTL and let `note` renew it
        ref: cli-claim-ttl
        labels: [cli, claim]
        desc: |
          The 30-minute default is too tight for a focused multi-hour
          leaf. Either bump the default to 60 or 90 minutes, expose
          `--ttl 90m` on `claim`, or have `note` extend the existing
          lease. Pick one and document the precedence.

      - title: Add `--quiet` to `claim`, `done`, `cancel`
        ref: cli-quiet-flag
        labels: [cli, output]
        desc: |
          Suppress the redundant full `show` block on success. Keep
          the one-line confirm + the `Next:` / `Parent: N of M
          complete` summary lines — those are the parts a working
          agent uses.

      - title: Echo identity in claim/done/cancel summary lines
        ref: cli-identity-echo
        labels: [cli, identity]
        desc: |
          Add `as=<identity>` to the summary line so misattributed
          writes surface at the moment of action, not on the next
          `status`. Trivial change, big leverage when juggling
          --as flags.

      - title: `job log --as <id> --since <duration>` filter
        ref: cli-log-since
        labels: [cli, log]
        desc: |
          Recall window for "what did I just close?" so a multitasking
          agent can recover the cascade-close summary after the fact.
          If `job log` already exists, extend its filters; otherwise
          add the subcommand. Pair with `--cascade` to see auto-closed
          ancestors inline.

      - title: Structured acceptance criteria on tasks
        ref: cli-criteria
        labels: [cli, schema]
        desc: |
          Add an optional `criteria:` array to tasks (import schema +
          add/edit). Each entry: a short label and a state
          (pending/passed/skipped/failed). `job done` can prompt for
          per-criterion state, or accept `--criterion key=passed`.
          Replaces the free-text "what got verified" paragraph with
          rows the next reader can scan.
        blockedBy: [cli-quiet-flag]

      - title: Symmetric dependency display on `job show`
        ref: cli-blocks-symmetry
        labels: [cli, show]
        desc: |
          Mirror the `Blocked by:` line on the child side as `Blocks:`
          (outbound) so working a leaf surfaces what it unblocks.
          Today only the gate side shows the relationship and the
          wording reads ambiguously.

      - title: Rename `Blocking:` → `Blocked by:` in `job show`
        ref: cli-blocked-by-wording
        labels: [cli, show, copy]
        desc: |
          The current `Blocking: a, b, c` line on a gate task reads
          as "this task is blocking those" when it actually means
          "this task is blocked by those." Standard convention is
          `Blocked by:` for the inbound edge. Pair with
          cli-blocks-symmetry so both directions read naturally.

      - title: `job split <id>` — open children from inline list
        ref: cli-split
        labels: [cli, structure]
        desc: |
          Mid-session refactor: take an existing leaf, open N children
          under it from an inline list of titles, and re-block the
          parent on them. Equivalent to `job add` × N + `job block`
          today; condensing it makes "I should track this as smaller
          steps" the path of least resistance instead of a small ritual.
```
