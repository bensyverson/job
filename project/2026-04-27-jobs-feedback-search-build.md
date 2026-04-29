# `job` feedback from the global-search build session

*Author: Claude Opus 4.7, after using `job` to drive task `1SYqo` (Header search UI + "/" shortcut) end-to-end and then trying to slot a new "Phase 10 — Dashboard polish pass" parent under the existing Phase 10 (`YsWiZ`).*
*Date: 2026-04-27.*

The session was mostly smooth: `job status` opened, `job claim 1SYqo` plus implement/test/commit, `job done 1SYqo` cascaded the count up to `qkWEQ`. The friction came afterward, when the user surfaced 11 small UI polish items and we wanted to nest them under the existing Phase 10. That's where `job` ran out of road.

Each finding below was re-run against the live CLI before drafting (per the [verify-retrospectives lesson](2026-04-22-pm-papercuts.md)); evidence inline.

---

## P1 — No cross-parent move

`job move` is sibling-only:

```
$ job move --help
Move a task before or after a sibling task. Both tasks must share the same parent.
Usage:
  job move <id> before|after <sibling> [flags]
```

There's no `reparent`, no `move <id> under <new-parent>`, no `--parent` flag on `move`. Verified by enumerating subcommands (`job` with no args) — only `move`, `edit`, `add`, `block`, and `label` touch tree shape, and none of them re-parent.

This came up organically: I created `zZuPx "Phase 10 — Dashboard polish pass"` at the root (because that's what `job add <title>` does without a positional parent), then the user wanted it nested under the pre-existing `YsWiZ` (also "Phase 10 — …"). The workaround was 22 events — cancel 10 children, watch the parent auto-close on last-child-cancel, then `job add YsWiZ "Dashboard polish pass"` plus 10 fresh `job add` calls plus one `job move` for ordering. The IDs all changed.

That's:
- noisy in the event log (20 cancel/create pairs for what is conceptually one move),
- destructive of any external reference to the old IDs (none here, but in a multi-agent or doc-linked workflow this would hurt),
- inconsistent with the "events are the source of truth" promise — the canceled tasks are *not* what happened, the user's intent was to relocate.

### Suggested shape

```
job move <id> under <new-parent>          # reparent only; preserves sibling order at the end
job move <id> before|after <sibling>      # current sibling-reorder, unchanged
job move <id> under <new-parent> before|after <sibling>   # reparent and place
```

Implementation note: this is one new event type (`reparented`) + an update to `tasks.parent_id`. The replay/scrubber layer would need to know how to invert the event (memory stores prior parent_id in `detail`).

Optional but nice: `--keep-position` flag to land at end vs. start of the new parent.

### Why it matters

Long-running plans accrete structure over time. The Phase 9 → Phase 10 transition surfaced this naturally — work the user originally thought belonged in Phase 9 turned out to be Phase 10 polish, and there's no clean way to relocate it. As soon as Jobs is being used by humans-plus-agents on multi-week roadmaps, this will hit weekly.

---

## P2 — `job add --parent <id>` is rejected; the right form is positional

Verified:

```
$ job add --parent foo "test"
Error: unknown flag: --parent
```

The positional form is documented in `--help`:

> `job add [parent] <title>` — "If parent is provided, the task is added as a child."

So the muscle-memory shape (`--parent <id>`) is wrong, and the required shape is `job add <parent-id> <title>`. The error is clear, but the obvious guess fails. For LLM agents — who reach for named flags before positional args — this costs a turn.

### Suggested shape

Accept `--parent <id>` as an alias of the positional. Both shapes round-trip to the same SQL.

---

## P3 — `job ls --grep <pat>` returns ancestor scaffold even when nothing matches in the visible scope

This bit me when looking for the existing Phase 10 task. Verified:

```
$ job ls --grep "Phase 10"
- [ ] `bYr6R` Web dashboard v1 (labels: dashboard, web)

$ job ls --grep "Phase 10" --all
- [ ] `bYr6R` Web dashboard v1 (labels: dashboard, web)
  - [ ] `YsWiZ` Phase 10 — Polish, empty states, errors, accessibility (blocked on qkWEQ, …)
- [ ] `zZuPx` Phase 10 — Dashboard polish pass (canceled, …)
```

Without `--all`, `YsWiZ` is filtered out (it's `blocked`), but its ancestor (`bYr6R`) still renders, because `ls` evidently keeps ancestors of any node it walked into. The result reads like "I matched `Web dashboard v1`" — which I didn't. The match is invisible.

### Suggested fix

When `--grep` matches *zero* visible tasks, print a clear empty state ("No tasks match `Phase 10`. Try `--all` to include blocked / done / canceled.") instead of bare ancestor frames. Or: only print ancestors of nodes that actually matched the grep.

The hint about `--all` would have shaved a turn here too — I had to guess that the missing match was status-filtered.

---

## P4 — `RunNote` appends note text to `tasks.description`; this isn't called out anywhere

Not a `job` CLI issue per se, but a schema-doc gap I hit while building the search backend. `internal/job/tasks.go:756` (RunNote) does:

```go
newDesc = task.Description + "\n\n[" + timestamp + "] " + text
// then UPDATE tasks SET description = ? …
// then recordEvent(noted, detail.text=text, detail.description_after=newDesc)
```

The implication is that note text is *both* an event payload *and* part of `tasks.description`. I designed the search SQL with a separate "note rank" branch (`EXISTS (… events e WHERE e.event_type='noted' …)`) and a separate `MatchSource = "note"` constant — until a test failed because the description-LIKE branch had already matched first, since the note text was sitting right there in `tasks.description`. The "note" rank was unreachable. I dropped the branch and folded notes into description matches.

### Suggested fix

A single sentence in the schema doc (or migration comment) noting that `tasks.description` is the rolled-up state including all `noted` event text. This saves anyone building search/filter/index features the round-trip I made.

---

## What worked well (worth keeping)

- `job status` as session opener — landed me with `Next: 1SYqo` ready to claim. Zero ceremony.
- `job done <id>` cascading the parent's "X of Y done" rollup, plus a `Next:` hint to the next leaf (`hNTiB`). The hint kept me inside the current plan instead of drifting.
- `job cancel <ids…> -m "<reason>"` taking a flat ID list and a single reason. Multi-cancel was one call.
- Parent auto-close on last-child cancel was a useful side-effect during the reparent (saved an explicit `job cancel zZuPx`). The CLI surfaced it (`Auto-closed (canceled): zZuPx "…"`), so it wasn't silent magic. Good.
- `job add` returning *just* the new short_id on stdout — `NEW=$(job add ... | tail -1)` worked first try.

---

## Recommendation order

1. **P1 (cross-parent move)** — biggest functional gap; the workaround is genuinely costly for any non-toy plan. Worth a `Phase 10` ticket or its own roadmap item.
2. **P3 (grep empty state)** — small fix, prevents real LLM-agent confusion.
3. **P2 (`--parent` alias)** — trivial; shave-a-turn for agents.
4. **P4 (schema doc note)** — one sentence somewhere obvious.
