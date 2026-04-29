# `job` CLI feedback — Dirac session, 2026-04-27

## Context

First-time user of the `job` CLI implementing XeTlP (Search handler and index). Used `job` for landscape briefing, task navigation, and completion tracking across a ~250-task project.

## What worked

1. **`job status` as the session opener.** The one-line summary ("14 open, 231 done, 3 canceled · next XeTlP") oriented me instantly. I never had to guess what to work on.
2. **`job done <id>` with auto-next surfacing.** Closing XeTlP immediately showed 1SYqo as next — no mental bookkeeping required.
3. **`job ls bYr6R --all` for subtree inspection.** Being able to scope the list to a specific parent and see the full tree (including done tasks) was essential for understanding the dashboard project's phase structure.

## What was friction

1. **`job ls` (no arg) defaults to actionable-only, which hides most of the tree.** My first run showed only 3 tasks; I initially thought the project was nearly complete. I had to re-run with `--all` to see the 14 open tasks. The actionable filter is correct for the "what do I do next" use case, but as a first-time user I expected a full inventory.
2. **No `job list --status open` or similar filter.** I tried `--status open` instinctively and got "unknown flag". I then had to read `--help` to discover `--all`, `--mine`, `--claimed-by`, etc. A `--status` filter (or at least `--open`) would have absorbed the wrong guess.
3. **`job ls` output truncates without a clear signal.** When I first ran `job ls --all`, the output was truncated at 10KB with a message about context flooding. I had to run a more specific command (`job ls bYr6R --all`) to see the full tree. The truncation message is accurate but easy to miss in the output stream.

## Recommendations

1. **First-run hint for `job ls`:** When `job ls` (no arg) returns fewer than N results, append a one-liner: "Showing actionable tasks only. Use --all for the full tree." This orients first-time users without changing the default behavior.
2. **Add `--status` filter:** Accept `--status <status>` as a synonym for the existing filter behavior, or at least add `--open` as a convenience flag. This absorbs the wrong guess that agents will make.
3. **Surface truncation more prominently:** When `job ls --all` truncates output, prefix the truncation message with a visual marker (e.g., "---") or place it at the end of the output so it's harder to miss.

## Proposed changes

```yaml
tasks:
  - title: Apply CLI feedback from 2026-04-27 Dirac session
    desc: |
      Small UX improvements to the `job ls` command based on a first-time
      user experience implementing XeTlP.
    labels: [cli, ux, feedback]
    children:
      - title: "`job ls` actionable-only hint when results are sparse"
        ref: ls-actionable-hint
        labels: [cli, ls, ux]
        desc: |
          When `job ls` (no arg) returns fewer than, say, 5 results, append
          a one-liner: "Showing actionable tasks only. Use --all for the
          full tree." This orients first-time users without changing the
          default filter behavior.
      - title: "Add `--status` or `--open` filter flag to `job ls`"
        ref: ls-status-flag
        labels: [cli, ls, ux]
        desc: |
          Accept `--status <status>` as a filter flag, or at minimum add
          `--open` as a convenience. Absorbs the wrong guess that agents
          make when looking for open tasks.
      - title: "Make `job ls` truncation signal more prominent"
        ref: ls-truncation-signal
        labels: [cli, ls, ux]
        desc: |
          When output is truncated, place the truncation message at the
          end of the output with a clear visual marker so it's harder to
          miss.
```
