# Consolidated CLI/UX feedback synthesis

**Date:** 2026-04-28
**Source documents:**
- `project/2026-04-26-1120-opus-cli-feedback.md`
- `project/2026-04-27-dirac-cli-feedback.md`
- `project/2026-04-27-jobs-feedback-search-build.md`
- `project/2026-04-28-job-cli-experience-report.md`
- `project/2026-04-28-job-cli-phase10-feedback.md`
- `project/2026-04-28-job-experience-report.md`

**Goal:** Collapse ~30 distinct feedback points (with duplication across sessions) into a single coherent plan. Big items live as top-level children; related papercuts are grouped into containers.

---

## Themes across all sessions

1. **Arg-parsing consistency burns repeatedly.** `note` requires `-m` while `add` takes positional title; `--parent` is rejected on `add`; `release` lacks `-m` that `done` has. Agents make the same wrong guesses every session.
2. **`show` output fights itself when descriptions are long.** Handoff notes are a genuinely useful pattern, but they push structural metadata (children, parent, labels) off the first screen.
3. **`ls`/`tree` discoverability is the #2 friction source after arg-parsing.** First-time users think the project is nearly done when they see 3 actionable tasks out of 250. The `--all` escape hatch exists but isn't visible enough.
4. **Auto-close cascade is great when scope is clean, dangerous when it isn't.** No session hit a disaster, but two sessions noted the missing guard-rails.
5. **The CLI surface is missing structural verbs.** Reparenting, splitting a leaf into children, and editing descriptions in-flight all require multi-step workarounds today.
6. **Claim lifecycle has rough edges.** Parent claims expire pointlessly (parents shouldn't be claimable at all); output is noisy; identity isn't echoed on mutations.

---

## Proposed changes (consolidated)

```yaml
tasks:
  - title: Consolidated CLI/UX feedback round — six-session synthesis
    desc: |
      Roll-up of feedback from Opus, Dirac, and three in-session agents
      across 2026-04-26 to 2026-04-28. Big items are top-level children;
      small, related papercuts are grouped into containers.
    labels: [cli, ux, feedback]
    children:
      - title: Extend move with cross-parent reparenting
        ref: move-reparent
        labels: [cli, structure]
        desc: |
          `job move` is currently sibling-only. Add cross-parent reparenting
          so long-running plans can be restructured without cancel/create
          cascades that destroy IDs and pollute the event log.

          Proposed grammar:
            job move <id> under <new-parent>          # reparent to end
            job move <id> under <new-parent> before|after <sibling>
            job move <id> before|after <sibling>      # current shape, unchanged

          Implementation: one new event type (`reparented`) with prior
          parent_id stored in detail for invertibility. Update replay/scrubber
          to handle the new event.

          Why it matters: Phase 10 polish pass surfaced this organically.
          The workaround was 22 events (cancel 10 children, watch auto-close,
          recreate). That breaks ID stability and contradicts "events are the
          source of truth."

      - title: Add split command for inline task subdivision
        ref: split-command
        labels: [cli, structure]
        desc: |
          `job split <id>` takes an existing leaf, opens N children under it
          from an inline list of titles, and re-blocks the parent on them.
          Equivalent to `job add` x N + `job block` today, but condensing it
          makes "I should track this as smaller steps" the path of least
          resistance.

          Proposed grammar:
            job split <id> "Child A" "Child B" "Child C"

          The parent is automatically blocked on the new children. The parent
          remains claimed by the current actor.

          Why it matters: Mid-session, an agent considered breaking a large
          child into sub-children but skipped it because the ritual was too
          heavy. The work stayed mental instead of tracked.

      - title: Add structured acceptance criteria to tasks
        ref: criteria-field
        labels: [cli, schema]
        desc: |
          Add an optional `criteria:` array to the import schema and to
          `job add`/`job edit`. Each entry has a short label and a state
          (`pending` / `passed` / `skipped` / `failed`).

          `job show` renders criteria as a checklist. `job done` can optionally
          accept `--criterion key=passed` to update state inline, but **never
          blocks on pending criteria** — the auto-close cascade remains
          untouched. A soft warning may be printed if criteria are left
          pending, but the close proceeds regardless.

          Why it matters: Verification gates currently carry their acceptance
          criteria as free-text paragraphs. A structured checklist is more
          likely to be filled in by agents, more scannable for the next reader,
          and enables future audit queries (e.g. "which gates had deferred
          performance checks?"). This is lighter than making exit tasks
          independent, which would lose the semantic parent/child relationship.

          Rationale for no enforcement: we are in BUILD mode; ship the field
          as an invitation to structure and observe whether agents voluntarily
          fill it in. If trust gets abused, we can add a `--force` gate later.

      - title: Arg-parsing consistency pass
        ref: arg-consistency
        labels: [cli, dx]
        desc: |
          Four related papercuts where agents make the same wrong guesses
          every session because the CLI surface is inconsistent.
        children:
          - title: Accept positional message text on job note
            ref: note-positional
            labels: [cli, note]
            desc: |
              `job note <id> "text"` should work without `-m`. Today it fails
              with `unexpected argument`. Keep `-m` and stdin (`-`) as
              alternatives so existing scripts don't break.

              Why: `job add <parent> <title>` takes its body positionally.
              The inconsistency means agents guess wrong every session.

          - title: Accept --parent as alias on job add
            ref: add-parent-flag
            labels: [cli, add]
            desc: |
              `job add --parent K8iGR "title"` should be equivalent to
              `job add K8iGR "title"`. The positional form stays canonical;
              this absorbs the wrong guess at zero cost.

          - title: Accept -m on job release (parity with done)
            ref: release-message
            labels: [cli, release]
            desc: |
              `job done <id> -m "..."` works; `job release <id> -m "..."`
              fails with `unknown shorthand flag: 'm'`. Lifecycle verbs `done`
              and `release` should share a flag surface. Append the note before
              releasing the claim.

          - title: Add unclaim as alias for release
            ref: unclaim-alias
            labels: [cli, release]
            desc: |
              Agents instinctively reach for `job unclaim` when they want to
              drop a claim. The "Did you mean: claim?" suggestion is backwards.
              Add `unclaim` as a top-level alias and mention it near `claim` in
              help text.

      - title: show output layout pass
        ref: show-layout
        labels: [cli, show]
        desc: |
          Three related changes to how `job show` renders tasks.
        children:
          - title: Reorder show fields — structural metadata first, description below
            ref: show-field-order
            labels: [cli, show]
            desc: |
              Move ID / Title / Status / Parent / Labels / Children / Created
              to the top; description and notes follow. Mirrors `gh pr view` /
              `gh issue view`: meta header, then body. An Opus session
              mis-reported "show doesn't include children" because the
              Children: block was below a 40+ line description.

          - title: Separate notes from description in show output
            ref: separate-notes
            labels: [cli, show, schema]
            desc: |
              `RunNote` currently appends note text to `tasks.description`,
              causing duplication in `show` output (description already
              contains all notes, then notes are printed again from the event
              log). Stop the dual-write: `tasks.description` stays as the
              original description only. Notes remain as first-class `noted`
              events with actor + timestamp.

              `show` output becomes: Description (original only), then Notes
              (from events, with timestamps/actors). No more duplication.

              This also fixes the search double-count: currently a note-only
              match surfaces as a description match because the text lives in
              both places. After separation, search must query the events
              table for `noted` events too.

          - title: Symmetric dependency display + rename Blocking to Blocked by
            ref: show-blocks-symmetry
            labels: [cli, show]
            desc: |
              Two changes:
              1. Rename `Blocking:` to `Blocked by:` on gate tasks. The current
                 label reads as "this task is blocking those" when it means the
                 opposite.
              2. Add a `Blocks:` (outbound) field on leaf `show` output so an
                 agent working a leaf can see what it unblocks downstream.
                 Today only the gate side shows the relationship.

      - title: ls and tree discoverability pass
        ref: ls-discoverability
        labels: [cli, ls]
        desc: |
          Four related changes to how `ls`, `tree`, and `list` behave and
          communicate their filters.
        children:
          - title: Add sparse-results hint to unscoped ls
            ref: ls-sparse-hint
            labels: [cli, ls]
            desc: |
              When `job ls` (no arg) returns fewer than, say, 5 results, append
              a one-liner: "Showing actionable tasks only. Use --all to include
              blocked / done / canceled tasks." This orients first-time users without
              changing the default filter behavior.

          - title: Add --status or --open filter flag to ls
            ref: ls-status-flag
            labels: [cli, ls]
            desc: |
              Accept `--status <status>` as a filter flag, or at minimum add
              `--open` as a convenience. Absorbs the wrong guess agents make
              when looking for open tasks.

          - title: Fix grep empty state when matches are status-filtered out
            ref: ls-grep-empty-state
            labels: [cli, ls]
            desc: |
              When `--grep` matches zero visible tasks, print a clear empty
              state ("No tasks match `<pat>`. Try --all to include blocked /
              done / canceled.") instead of bare ancestor frames. Ancestors
              should only render when they actually contain a matched node.

          - title: Make truncation signal more prominent in ls/tree
            ref: ls-truncation-signal
            labels: [cli, ls]
            desc: |
              When `ls --all` or `tree` truncates output, place the truncation
              message at the end with a clear visual marker so it's harder to
              miss.

      - title: Lifecycle and claim polish pass
        ref: lifecycle-polish
        labels: [cli, claim, done]
        desc: |
          Seven related papercuts around claiming, completing, and releasing
          tasks.
        children:
          - title: Surface auto-close cascade at the moments that matter
            ref: auto-close-hints
            labels: [cli, ux, parent]
            desc: |
              Two contextual one-liners:
              (a) On `job add <parent> <title>` when the parent already has
                  children: "<parent> now has N children; complete them all
                  to auto-close the parent."
              (b) On `job claim <id>` when the claimed task is the parent's
                  last not-yet-done child: "Closing this task will auto-close
                  parent <parent>. Verify scope first."
              Cheap supplement: one-line mention in `job schema` and in the
              `add` / `done` help text.

          - title: Add long-claim example to job help text
            ref: claim-ttl-example
            labels: [cli, claim, docs]
            desc: |
              The `job claim` command already accepts an optional duration
              (`job claim <id> 2h`), but agents don't discover it mid-session.
              Add one example to the main `job` help text showing a longer
              claim: e.g. `job claim abc123 2h` so the pattern is visible at
              session start without reading `claim --help`.

          - title: Add --quiet to claim, done, and cancel
            ref: quiet-flag
            labels: [cli, output]
            desc: |
              Suppress the redundant full `show` block on success. Keep the
              one-line confirm + the `Next:` / `Parent: N of M complete`
              summary lines — those are the actually-useful parts.

          - title: Echo identity in claim/done/cancel summary lines
            ref: identity-echo
            labels: [cli, identity]
            desc: |
              Add `as=<identity>` to the summary line so misattributed writes
              surface at the moment of action, not on the next `status`.

          - title: Consolidate `claim-next` into `claim --next`
            ref: claim-next
            labels: [cli, claim]
            desc: |
              `job claim-next` already exists as a standalone command. Fold it
              into `job claim --next` for a single cohesive surface. `job
              claim --next` claims the same leaf `job status` surfaces as Next.
              Optionally accept a parent arg (`job claim --next <parent>`) to
              scope the pick to a subtree.

              In BUILD mode with zero users, drop the standalone `claim-next`
              command rather than maintaining both. Update help text and any
              internal references.

          - title: Suppress Next hint after job claim when redundant
            ref: claim-next-noise
            labels: [cli, output]
            desc: |
              `job claim <id>` echoes a `Next:` line pointing at the globally
              next available task. Drop it when the claimed task *is* the
              next task the planner would have suggested; keep it after
              `done` / `cancel` where it genuinely guides flow.

          - title: Make parents with children unclaimable
            ref: unclaimable-parents
            labels: [cli, claim]
            desc: |
              Parents with children should not be claimable. A claim means
              "I'm actively working on this" but a parent is a container that
              auto-closes when its children finish. These concepts fight each
              other and produce expired claims on phases spanning hours or days.

              `claim`, `next`, `status`, and `claim --next` should all filter
              out parents with children from their claimable task lists. A
              parent with no children is a leaf-in-waiting and can still be
              claimed normally.

          - title: List open leaves in claim <parent> error message
            ref: claim-leaves-in-error
            labels: [cli, claim]
            desc: |
              When `job claim` is run against a non-leaf, the error currently
              says "claim a leaf instead, or run 'next <id> all' to see them."
              Inline a one-line summary of the available leaves so the user can
              claim correctly on the next command rather than the one after.
              Cap the inline list to keep the error compact.

      - title: Audit log improvements
        ref: audit-log
        labels: [cli, log]
        desc: |
          Add filters to `job log` for common audit queries.
        children:
          - title: Add time and actor filters to job log
            ref: log-filters
            labels: [cli, log]
            desc: |
              `job log` shows the full event stream including `noted` events,
              but lacks filters for common audit queries. Add `--as <actor>`
              and `--since <duration>` so an agent can ask "what did I just
              close in the last 5 minutes?" without scrolling through the
              entire history.

      - title: Docs and conventions pass
        ref: docs-conventions
        labels: [cli, docs]
        desc: |
          Two small documentation/ambient-signal fixes.
        children:
          - title: Remove hard-wrapping from help text and long-form descriptions
            ref: unwrap-help-text
            labels: [cli, docs]
            desc: |
              Stop hard-wrapping cobra `Long:` fields, command help, and
              `job schema` output. Write each paragraph as one logical line;
              let the terminal renderer handle wrapping at the user's actual
              width. The convention propagates by example — agents reading
              hard-wrapped help reproduce the same wrapping in their own
              notes and YAML desc blocks.

          - title: Update schema doc to reflect separate notes
            ref: schema-note-doc
            labels: [cli, docs, schema]
            desc: |
              Update the schema doc and any migration comments to state that
              `tasks.description` is the original description only, and notes
              are stored as `noted` events in the event log with actor +
              timestamp. Remove any references to the old append-to-description
              behavior.
```
