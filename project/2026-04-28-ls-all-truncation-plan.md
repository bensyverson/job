# Truncate `job ls --all` to open + N closed by default

```yaml
tasks:
  - title: Truncate `job ls --all` to open + N closed by default
    desc: |
      Bound the default `job ls --all` output to "everything currently open + the N most recent closed tasks" so agents can use --all for surrounding context without dragging in unbounded archive history. Same default applies when --all composes with --status. JSON format stays unbounded (scripts handle their own pagination). The positional `all` and the `--all` flag remain full aliases.

      Wrinkle resolved: closed-tail rows render as a flat "Recently closed" footer section below the open tree, with a parent breadcrumb on every row when the query is unscoped and breadcrumb omitted when the query is subtree-scoped. We do not refold closed tasks back into the tree, since that would force zombie ancestors and break the "tree is live work" mental model.
    children:
      - title: Add closed-tail collection to RunListFiltered
        desc: |
          Extend the list pipeline so a single call returns both the open set (today's behavior) and a closed-tail set. Closed tail is sorted by closed-at (the done/canceled event timestamp) descending, capped at N=10 by default. Tail is subtree-scoped when a parent is given; otherwise it spans the whole tree. The tail respects the same filters as the open set (--label, --mine, --claimed-by, --grep, --status).
        criteria:
          - Tail respects --label / --mine / --claimed-by / --grep / --status
          - Tail capped at 10 rows by default
          - Tail sorted by closed-at desc
          - Tail scoped to subtree when a parent positional is given
          - --format=json returns the full closed set, bypassing truncation
      - title: Render the closed tail as a "Recently closed" footer section
        desc: |
          The open tree renders first, unchanged. Below it, a flat "Recently closed (N of M)" section lists each closed-tail row on its own line. Each row carries a parent breadcrumb (parent short-id + title) when the query is unscoped; the breadcrumb is omitted when the query is subtree-scoped (parent is implicit). The section is omitted entirely when zero closed tasks are in scope.
        criteria:
          - Section header reads "Recently closed (N of M)" where M is the unbounded total in scope
          - Breadcrumb shows parent short-id + title for unscoped queries
          - Breadcrumb omitted for `ls <parent> all`
          - Section omitted when scope has zero closed tasks
          - Open tree rendering unchanged
      - title: Add --since and --no-truncate flags to ls
        desc: |
          Two escape hatches on top of the default truncation. --since accepts either a duration (5m, 2h, 7d) or a count (e.g. 50) and replaces the default 10-row tail cap. --no-truncate removes the cap entirely. Mutually exclusive — passing both is an error.
        criteria:
          - --since 7d returns closed events from the last 7 days only
          - --since 50 returns the most recent 50 closed events
          - --no-truncate returns the full closed history
          - --since and --no-truncate are mutually exclusive (error if both passed)
          - Both flags compose with --label / --mine / --claimed-by / --grep / --status
      - title: Emit a one-line footer when the tail truncates
        desc: |
          When the closed tail in scope is larger than the active cap, emit one line at the end of the markdown output naming the escape hatches. Loud and unconditional when truncation fired; suppressed when no truncation occurred or when --format=json is in use.
        criteria:
          - Footer fires only when tail truncated > 0 rows
          - Footer names --since and --no-truncate explicitly
          - Footer reports shown / total counts (e.g. "10 of 47 recent closures")
          - Footer suppressed for --format=json
          - Footer suppressed when nothing was truncated
      - title: Update ls help text and root quickstart
        desc: |
          Update the long help on `job ls` to describe the new default ("open + 10 most recent closed; --since for windowing, --no-truncate for full history") and reframe `--all` as the context-gathering verb rather than a forensic firehose. If the root quickstart in commands.go references `ls --all`, mirror the change there. Sweep docs/ for stale references.
        criteria:
          - ls --help long description names the open + 10 default
          - ls --help mentions --since and --no-truncate by name
          - Root quickstart, if it mentions ls --all, is consistent with the new default
          - docs/ has no stale references to unbounded ls --all
```
