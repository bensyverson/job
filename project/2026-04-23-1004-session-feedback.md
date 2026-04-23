# `job` ergonomics — 2026-04-23 session feedback

*Author: Claude Opus 4.7 (1M context), after a multi-hour session driving the `JxyAc` umbrella end-to-end: the P1–P8 papercuts, the `status`/`summary` unification (XLgcl, eight subtasks), the hierarchical `Next:` walk rewrite, the `job import` sort_order fix + backfill, and the root-help rewrite.*

Each finding was re-verified against the current binary before writing this doc — per the memory I carry from the prior retrospective, where two claims were factually wrong and only caught on review.

---

## Session shape

Everything was done through `job`: claiming, closing, noting, importing, cancelling, blocking, labelling, summary/status. I used the tool heavily enough to notice both the real frictions and the places where it genuinely reduced cognitive load.

This doc is what I'd throw on the pile.

---

## Findings

Each finding has the same shape: what I hit (symptom), why it matters (why), and a concrete direction (fix).

### F1 — `job add` has no `--label`

**Symptom.** Tagging a new task is always two calls:
```
$ job add JxyAc "Fix sort_order assignment..."
OlBSV
$ job label add OlBSV cli import bug
Labeled: OlBSV (+cli, +import, +bug)
```
I did this exact two-step for both tasks I added by hand this session.

**Why.** `add` is already a canonical verb used constantly. Labels are the common metadata axis. Pairing them is idiomatic in every comparable tool (`gh issue create --label foo`, `kubectl run --labels=key=val`, etc.). The current split also means the `created` and `labeled` events land in separate transactions with separate timestamps — a minor event-store cleanliness issue.

**Fix.** `-l/--label <name>` as a repeatable `StringSlice` flag on `add`. Thread labels into the existing `RunAdd` transaction so the `created` and `labeled` events land atomically. The `insertLabels` helper is already tx-safe; this is a thin wrapper.

### F2 — `job import` swallows real YAML parse errors

**Symptom.** When I wrote an import plan with an unquoted colon in a title (`title: Surface Next: in status output`), the import errored with:
```
Error: no YAML `tasks:` block found in project/2026-04-23-status-summary-unify.md
```
…which is misleading. The fence exists; the YAML inside it failed to parse with `mapping values are not allowed here, line N col M`. I only found the real cause by extracting the fence manually and running it through `python3 -c "import yaml, sys; yaml.safe_load(sys.stdin)"`.

**Why.** The probe loop in `extractTasksYAML` tries each fenced block, and if none yields a valid `tasks:` key, it returns the generic message. When a plan has *one* fenced YAML block and it fails validation, the user almost certainly wants the parse error — not the probe summary. This is a ~2-minute debugging delay that compounds across iterations.

**Fix.** Have `extractTasksYAML` capture the most-recent `yaml.Unmarshal` error encountered in a candidate fence. If no block yielded a `tasks:` key *and* at least one candidate errored, surface that parse error with its line/column info in place of the generic message. Leave the generic message as the fallback for the "no fences at all" case.

### F3 — No task search

**Symptom.** Several times I knew a title fragment but not the short ID. My fallback was `job list all | grep foo`, which flattens the tree into a search-first view that loses the parent context.

**Why.** Search by title fragment is one of the two or three most common task-manager operations. The current `list` filters (`--label`, `--mine`, `--claimed-by`) cover attribute queries but not text. An agent working in a >20-task database hits this constantly.

**Fix.** `--grep <pattern>` on `list` (leans case-insensitive; regex optional). Reuses `list`'s filters, rendering, and `--format=json`. Alternative: a new `find` verb. `--grep` is simpler and composes with existing list flags (`list --grep foo --mine`); that wins by symmetry.

### F4 — `job import --dry-run` doesn't preview `blockedBy` edges

**Symptom.** Dry-run output for a plan with a block graph is:
```
<new-1>  A
<new-2>  B
<new-3>  C
```
— three tasks, no hint of the edges. If the plan has `B blockedBy [A]` and `C blockedBy [B]`, that's invisible. Cycle detection still catches bad plans, but the graph itself is un-previewable.

**Why.** The whole point of `--dry-run` is to let the author eyeball a plan before persisting. For plans with any serialized work (essentially every multi-task umbrella), the edges are half the plan. Hiding them defeats the preview.

**Fix.** Extend each dry-run row with the resolved `blockedBy` list. Shape:
```
<new-1>  A
<new-2>  B (blocked on <new-1>)
<new-3>  C (blocked on <new-2>)
```
For edges that resolve to existing DB rows (via ref or short-ID), show the real ID instead of a `<new-N>` placeholder.

---

## Polish — lower ROI, worth noting

### F5 — `-m` ack shapes aren't uniform across verbs

`note` puts the preview on the verb line:
```
Noted: <id> · N chars · "preview"
```
`done` and `cancel` use a verb-line-plus-sub-line shape:
```
Done: <id> "Title"
  note: N chars · "preview"
```
Both make sense given the per-verb arg shape (note has no title; done does). But a programmatic reader needs two regexes, and the visual gap between the two forms is larger than the underlying semantic gap. Low-ROI to change; worth a design conversation before touching.

### F6 — `list [parent] [all]` uses a magic positional

`all` is a literal string argument, not a flag. It composes awkwardly with `--label`, `--mine`, and `--claimed-by`:
```
job list JxyAc all --mine       # reads like a shell artifact
job list JxyAc --all --mine     # reads cleanly
```
`--all` is a breaking change. In `BUILD` mode per `CLAUDE.md` that's fine, but it's a user-facing migration. Could keep the positional undocumented for one release to smooth the transition.

---

## What worked really well — preserve these

Not proposals; just recording the decisions that paid off.

- **Default identity set at `init` + auto-extend on writes.** After `init` recorded a default, I almost never reached for `--as`. Combined with the auto-extend-on-write semantics that landed in P7, the claim machinery became invisible in the best way. This is the best ergonomic decision in the codebase for agents.
- **The hierarchical `Next:` walk we just shipped.** When I closed a task and saw a `Next:` that pointed at the next logical task within my current plan — rather than hopping to a different root tree — it noticeably reduced the "what now" cognitive tax.
- **Pre-commit hooks running the full test suite.** Caught two things I would have pushed broken this session.
- **`status` as the session-start briefing.** The unify umbrella's payoff was real: running `job status` at the top of every session gives the landscape in one call.

---

## Not `job`'s problem, but worth naming once

Long `-m "..."` notes with backticks in the body are painful to quote in shells. `-m @path` and `-m -` (stdin) exist but aren't discoverable at the right moment. One option worth considering: mention the `@file` / `-` forms in the `note`/`done`/`cancel` `--help` examples, so agents see the escape hatch when they first hit the shell-quoting wall.

---

## Plan

```yaml
tasks:
  - title: CLI papercuts from the 2026-04-23 session
    ref: fb
    desc: |
      Six ergonomics findings from a long-session use of `job` to drive
      the JxyAc umbrella end-to-end. F1–F4 are real frictions hit
      during the session; F5–F6 are polish items to weigh. See
      project/2026-04-23-session-feedback.md for the full experience
      report and the rationale for each.
    labels: [cli, ergonomics, feedback]
    children:

      - title: F1 — `job add` accepts `-l, --label <name>` (repeatable)
        ref: f1
        desc: |
          Adding a task with labels currently requires two calls:
          `job add ...` then `job label add <id> a b c`. Thread labels
          into RunAdd's transaction so the created + labeled events
          land atomically and the common case is one call.

          Touchpoints:
          - cmd/job/add.go — StringSliceVarP on --label/-l
          - internal/job/tasks.go — RunAdd signature gains []string
            labels; insertLabels called inside the existing tx; the
            "labeled" event recorded alongside "created"
          - Tests: add with one label, add with multiple labels, add
            with a label containing a comma (validator rejection),
            add with --label omitted (no regression)
        labels: [cli, add, ergonomics]

      - title: F2 — Surface real YAML parse errors on import
        ref: f2
        desc: |
          When a plan has a single fenced YAML block but the YAML
          fails to parse (e.g., unquoted colon in a title
          `title: Surface Next: in status output`), the current error
          is the probe-loop's generic
          `no YAML tasks: block found in <file>`. The real
          `yaml.Unmarshal` error (mapping values not allowed, line N
          col M) is swallowed.

          Fix: in extractTasksYAML's probe loop, capture the most
          recent yaml.Unmarshal error encountered in a candidate
          fence. If no block yielded a usable tasks key AND at least
          one candidate errored during Unmarshal, surface that error
          (with its line/col) instead of the generic message.

          Touchpoints:
          - internal/job/import.go — extractTasksYAML probe loop
            tracks lastErr; RunImport uses it when no block matched
          - Tests: plan with one yaml fence and a bad-colon title
            asserts the error text names the parse failure (line
            number or "mapping values are not allowed")
        labels: [cli, import, errors]

      - title: F3 — `job list --grep <pattern>` for task search
        ref: f3
        desc: |
          Agents fall back to `job list | grep` when searching by
          title fragment, losing parent context and tree structure.
          Add --grep <pattern> to list — case-insensitive substring
          match by default — that composes with existing filters
          (--label, --mine, --claimed-by) and respects --format=json.

          Decide before implementation: substring-only, or allow
          regex via a separate --grep-regex? Lean substring; it's
          what every agent would try first.

          Touchpoints:
          - cmd/job/list.go — --grep StringVar, plumbed through
          - internal/job/tasks.go — list query adds LIKE filter on
            title (or post-filter, depending on perf on larger dbs)
          - Tests: substring match, case-insensitive, no match,
            composes with --label, respects --format=json shape
        labels: [cli, list, search]

      - title: F4 — Preview `blockedBy` edges in `import --dry-run`
        ref: f4
        desc: |
          Current dry-run lists imported tasks but omits the block
          graph. Any plan with serialized chains can't be reviewed
          end-to-end without the real import. Cycle detection runs
          regardless, but the edges themselves are invisible.

          Shape:
            <new-1>  A
            <new-2>  B (blocked on <new-1>)
            <new-3>  C (blocked on <new-2>)
          For edges resolving to existing DB tasks, render the real
          short-ID in place of the <new-N> placeholder.

          Touchpoints:
          - internal/job/import.go — dry-run branch threads each
            parsed task's resolved blockedBy (local refs + existing
            DB short IDs) into the ImportedTask result
          - cmd/job/import.go — md renderer extends each row with
            the parenthetical when the list is non-empty
          - Tests: dry-run of a plan with blockedBy edges surfaces
            them; dry-run with a blockedBy to an existing task
            renders the real ID, not a placeholder
        labels: [cli, import, preview]

      - title: F5 — Align `-m` verb ack shapes (design)
        ref: f5
        desc: |
          `note` puts the body preview on the verb line
          (`Noted: <id> · N chars · "preview"`) while `done` and
          `cancel` put title on the verb line and preview on a
          sub-line (`Done: <id> "Title"\n  note: N chars · "preview"`).
          Both shapes make sense per-verb but a programmatic reader
          needs two regexes.

          This is a design conversation, not a mechanical fix. Stop
          and ask before implementing. Options:
          (a) Normalize note to the verb-line-plus-sub-line form
              (a dummy sub-line when there's nothing to qualify).
          (b) Normalize done/cancel to the single-line form for the
              cases where there's no title context worth preserving.
          (c) Leave as-is; document the two shapes in AGENTS.md.
        labels: [cli, ack, ergonomics, design]

      - title: F6 — `list --all` flag replaces the `all` positional
        ref: f6
        desc: |
          `list [parent] [all]` uses a literal-string positional for
          the "include closed" toggle. It composes awkwardly with
          the other flags: `list JxyAc all --mine` vs the cleaner
          `list JxyAc --all --mine`.

          Breaking change. In BUILD mode per CLAUDE.md that's fine,
          but it is user-visible. Option: keep the positional
          undocumented for one release as a graceful migration.

          Touchpoints:
          - cmd/job/list.go — BoolVar --all, drop (or undocument) the
            positional
          - internal/job/tasks.go — showAll bool is already plumbed;
            only the cmd layer changes
          - Tests: existing `list <parent> all` call sites migrate
            to --all; keep one asserting the positional still works
            if we go the soft-migration route
        labels: [cli, list, ergonomics]
```
