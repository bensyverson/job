# `job` CLI feedback — Phase 4 web-dashboard session

**Date:** 2026-04-23
**Author:** Opus 4.7 (1M context)
**Context:** Heavy `job`-driven work across one long session — claimed
~12 Phase 4 tasks (Rd92K subtree), filed several follow-on
enhancements as I went, used `job status` as the session-opener every
time I picked the thread back up. This is what stood out *as a user*
of the CLI, separate from the dashboard work itself.

---

## What worked

**`job status` as the session opener is the most-right design call in
the whole tool.** Identity check, landscape briefing, per-root rollup,
`Next:`, `Stale:` — all in one command. I never had to think about
what to look at first. It's the load-bearing primitive of the agent
loop; every other ergonomic falls out of having this anchor.

**`Next: <id>` after `done` keeps the loop tight.** I closed ~12
tasks across the session and never had to manually figure out what to
do next. The hierarchical-walk picked the right thing every time.
Combined with `job status`'s top-line `Next:`, I never lost the
thread when I context-switched.

**`Auto-closed: <parent>` callout in the `done` ack saved me from a
real footgun.** I'd added `kTfXu` as a child of `E2ffo` (a parent
that still had its own real work), then closed `kTfXu`. The
auto-close cascade fired and would have silently swallowed E2ffo's
remaining work — I caught it only because the ack printed
`Auto-closed: E2ffo` on the next line, loud and inline. Same pattern
(ack the cascade) for any other implicit state change is worth
preserving.

**Auto-extending claim TTL on `note` / `edit` / `label` worked
invisibly.** The proof: I literally never invoked `heartbeat` once
across the session. The cadence "check in by writing something and
your claim survives" matches how I actually work.

**`-m` for free-text body across `note` / `done` / `cancel` is good
consistency.** The ack shape (`note: N chars · "preview"`) is also
right — immediate confirmation that the right text landed.

---

## Frictions

**`job add` doesn't honor `-p` as `--parent`.** It uses positional
`[parent]`. `CLAUDE.md` says `-p` is the parent shortflag and the
rest of the toolchain follows it, so `add` is the odd one out — `job
add -p E2ffo "title"` failed for me with `unknown shorthand flag:
'p'`. The fix is either accepting `-p` on `add` (and treating it as
equivalent to the positional), or documenting the exception in `add
--help`. The first is much more in keeping with the tool's "every
short flag is consistent" feel.

**`done` could warn before triggering an auto-close cascade.** The
`Auto-closed:` callout caught it after the fact, which is enough.
But a cheaper preventive: when `done <child>` would close the last
open sibling of a parent, the ack could prepend a single line
showing how many open siblings remain (`(0 siblings remaining in
parent E2ffo)`). Same telemetry, surfaced one step earlier in the
ack flow. No prompt, no confirmation — just a heads-up that lets
the human or agent stop the cascade by re-opening before moving on.

---

## A retraction

In an earlier verbal pass I floated a `job work <id>` shortcut that
would combine `claim` + `note start` + `heartbeat` into one verb.
On reflection the premise was wrong: I almost never called `note`
mid-implementation in this session. The actual loop was just
`claim → work → done -m "..."`. Two verbs, not three. The friction
of an explicit `claim` before starting is real but mild — and at
the task sizes I was running, the `claim → done` window was
minutes, not hours. `note` mid-work would matter on longer tasks
where you want to leave a breadcrumb if you have to step away or
coordinate with parallel agents; none of that came up here, so I
shouldn't generalize. The verb count is fine as is.

---

## Actionable items

```yaml
tasks:
  - title: "`job add` should honor `-p` as `--parent`"
    ref: cli-add-p-flag
    desc: |
      `job add` uses `[parent]` as a positional argument and rejects
      `-p` with `unknown shorthand flag: 'p'`. The rest of the
      toolchain treats `-p` as `--parent` (CLAUDE.md spells out the
      shortflag conventions). Make `add` consistent: accept either
      the positional form (`job add E2ffo "title"`) or the flag form
      (`job add -p E2ffo "title"`), and route both through the same
      parent-lookup path. If both are supplied, prefer `-p` and warn
      that the positional was ignored.
    labels: [cli, dx, papercut]

  - title: "`done` ack should surface remaining open siblings"
    ref: cli-done-cascade-warning
    desc: |
      When `job done <child>` would leave its parent with zero open
      children — the trigger for the auto-close cascade — surface a
      single-line hint in the ack one step before the
      `Auto-closed:` line:

          (closing this leaves parent <ID> with 0 open siblings)

      Same data the cascade already computes, surfaced earlier so a
      human or agent can spot the impending close before it lands.
      No prompt, no confirmation — just a heads-up. Pairs naturally
      with the existing `Auto-closed:` callout, which stays as the
      after-the-fact ack.
    labels: [cli, dx, papercut]
```

---

## Net

The CLI feels designed *for* the loop I'm actually in. The rough
edges are all small and live at the seams between commands (one
flag inconsistency, one earlier-warning opportunity) rather than in
the core verbs. The core verbs are right.
