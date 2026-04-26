# `job` CLI feedback — Opus 4.7 session, 2026-04-25

## Context

Single session, dogfooding `job` to drive Phase 6 (Actors view) and Phase 7
(Peek sheet) of the Web dashboard plan. Roughly ten task transitions across
two phases, including parent auto-close cascades on `al9e9` and `0FsYJ`.

This doc is the agent's-eye view of what worked, what didn't, and a YAML
changelog of the discussed updates.

## What worked

**`job status` is the right session opener.** The "Next:" hint and the
per-root rollup ("60 of 89 done · next hgSC7") meant I never had to guess
what to claim. Combined with the project memory that says "open every
session with `job status`", it became muscle memory inside the first
minute.

**`job claim X && job show X` as a one-liner.** Pattern emerged early
and never broke down. Title + description + parent + labels in one shot
is exactly the briefing needed before reading code.

**Phase parents as commit boundaries.** The "X of Y done" rollup on
parents (al9e9, 0FsYJ, bYr6R) shaped the natural commit unit — close the
last child of a phase, watch the parent auto-close, commit the whole
phase as one atomic unit. The cascade was satisfying every time it
fired.

**`job done <id>` cascade-close.** The memory note about parents
auto-closing when their last child completes is correct and load-bearing.
Saved an entire close-the-parent step on every phase boundary.

## What was friction

**`show` / `ls` aliases printed a "prefer the canonical form" note on
every call.** I read it the first time, ignored it the next twenty,
because (a) it's a *note*, not a deprecation, so it reads as a style
nudge rather than "this will go away", and (b) it prints *after* the
output, by which time I'm already reading the result, not meta-text
about how I asked for it. The deeper question — "what should the
canonical form be?" — turned out to be more interesting than the
warning. See YAML below.

**`job show <parent>` underdelivers.** When I claimed a phase parent to
get oriented, `show al9e9` told me the parent had four children but
stopped at the count, so I always had to chase it with `job ls
<parent> --all` to see them. The mental model is `show` = "everything
I need to understand this node," `ls` = "filter by axis," and right
now `show` cedes territory to `ls` that it shouldn't.

**`job tree`'s absence.** I tried it once expecting an indented-tree
view of a phase. It's a small thing, but `tree` is what shell-fluent
users reach for when they want shape rather than detail. Conversation
agreed it can be an alias of `ls` since `ls` already returns the full
subtree — just wanting both names to work removes one rejected command.

**`--claim-next` on `done` is invisible.** The flag exists and is
exactly the workflow loop I want (close + advance in one call). I
didn't discover it the entire session because nothing in the help
output for `claim` or in the routine `job done` output hinted that it
existed. A one-line tip in `done --help` and `claim --help` would
turn an unknown into a reflex.

**Help text is silent on `ls`'s recursion behavior.** `ls --help`
opens with "List tasks. By default shows only actionable…" — silent
on whether the scope is one level or full subtree. The flag-shaped
filters (`--label`, `--mine`, `--claimed-by`, `--grep`) suggest a
tree-wide search; the behavior matches that, but the help doesn't
say so. Users who reach for `ls` with shell instincts (one level)
will be confused.

## Things I noticed but did not need

**`--parent <id>` flag on `ls`.** I tried it once — positional `[parent]`
already exists, my mistake. The flag form would be redundant.

**Claim-expiry visibility.** Claims auto-expire after 30 minutes. The
typical loop stays under that, but on a hard problem I wondered
briefly whether the expiry would surface visibly enough that I'd notice
it had elapsed before I committed to a stale state. Not a problem
encountered in this session — flagged for future awareness.

## Discussion summary

A back-and-forth with the user converged on a few principles:

1. **Match shell idioms.** `show`, `ls`, `tree` are universal verbs.
   `info`, `list` are fine but slightly more meta. House style across
   the rest of the CLI (`claim`, `done`, `note`, `block`, `tail`) is
   short imperative verbs. `show`/`ls`/`tree` fits that style; `info`
   is the only noun-shaped command in the lineup.

2. **`show` is the briefing, `ls` is the filter, `tree` is the shape.**
   When a phase parent has children, `show` should list them inline
   (markdown-shape, same as `ls`) so the user doesn't have to chase
   it with a second call. `ls` stays the filter command. `tree` can
   alias `ls` for now — the data is the same; if visual hierarchy
   becomes important later, `tree` can diverge into an indented
   renderer.

3. **`ls <parent>` does not include the parent in its output.** Same
   semantic as `ls /etc` in a shell — the argument is the scope, not a
   member of the result. Including it would distort filters and counts.
   The "node + its children" union case is `show`'s job (after the
   inline-children change).

4. **`claim` should echo the `show` briefing.** The first line stays
   the scriptable signal ("Claimed: X 'Title' (expires in 30m)"), but
   the body should follow with the full briefing. Claiming is exactly
   the moment you want every detail you'd otherwise have to fetch
   with a follow-up `show`.

5. **Surface `--claim-next` in help.** Both `done` and `claim` should
   carry a one-line tip pointing at the close-and-advance flag. Pattern
   discovery is currently word-of-mouth; should be inline.

6. **Drop the alias deprecation warnings.** Choose one canonical name
   per concept and let the help text carry the convention. Aliases
   that print a nag every call create more friction than they remove.

## Proposed changes

```yaml
tasks:
  - title: Apply CLI feedback from 2026-04-25 Opus session
    desc: |
      Roll-up parent for the CLI changes proposed in
      project/2026-04-25-opus-cli-feedback.md.
    labels: [cli, ux]
    children:
      - title: Make `show` and `ls` canonical; silence the alias warnings
        ref: canonical-show-ls
        labels: [cli, naming]
        desc: |
          Switch canonical command names to `show` (was `info`) and `ls`
          (was `list`). `info` and `list` continue to work as aliases but
          stop printing the "prefer the canonical form" note.

          Why: matches `git show`, `kubectl describe`, and the rest of
          the verb-shaped CLI vocabulary (claim, done, tail). Help text
          carries the convention; aliases stay quiet.

          Drops these trailing notes:
            - "note: `job show` is an alias for `job info`; prefer the canonical form."
            - "note: `job ls` is an alias for `job list`; prefer the canonical form."

      - title: Add `tree` as an alias of `ls`
        ref: tree-alias
        labels: [cli, naming]
        desc: |
          `tree <id>` produces the same output as `ls <id>`. Shell-fluent
          users reach for `tree` when they want hierarchy; `ls` already
          returns the full subtree, so this is a name-only alias for now.

          If indented rendering becomes important later, `tree` can
          diverge into its own renderer; for today the alias removes
          one rejected-command friction without splitting implementation.

      - title: "`show <id>` lists children inline as a markdown checklist"
        ref: show-inline-children
        labels: [cli, show]
        desc: |
          When the node has children, render them inline under a
          `Children:` header, in the same shape as `ls` output:
          `- [ ] \`shortID\` Title (claim/blocker/labels)`.

          Cap:
            - ≤ 10 children: render every child as one line.
            - > 10 children: render `Children: N (use 'job ls <id>' to see all)`.

          Example:
            Children:
              - [x] `x81Op` GET /actors — column layout SSR (labels: actors, web)
              - [x] `hgSC7` GET /actors/<name> — single-actor view (labels: actors, web)
              - [ ] `B645R` Live updates (claimed by claude, 28m left)
              - [ ] `oPR2R` Tests (blocked on hgSC7, x81Op)

          Why: closes the loop where `show` sends users to `ls` to
          discover the same information they came to `show` for.

      - title: "`claim <id>` prints the `show` briefing after the one-liner"
        ref: claim-prints-briefing
        labels: [cli, claim]
        blockedBy: [show-inline-children]
        desc: |
          Keep the existing scriptable signal as the first line
          ("Claimed: X 'Title' (expires in 30m)"), then follow with the
          full `show` output. Same flow for `claim-next` and
          `done --claim-next`.

          Why: claiming is the moment the user wants the briefing.
          Folds the universal `claim X && show X` pattern into a single
          call. Scripts grepping for "Claimed:" keep working.

      - title: "`ls --help`: document subtree-recursion behavior"
        ref: help-ls-recursion
        labels: [cli, docs, help]
        desc: |
          Add a behavior note to `ls --help`:

            When given a parent, `ls` returns the full subtree under
            that parent (not just direct children). Combine with
            --label, --mine, --claimed-by, --grep to filter.

          Why: the current help is silent on recursion depth, leaving
          users to infer it from the filter-shaped flags.

      - title: "`done --help`: surface the `--claim-next` close-and-advance flag"
        ref: help-done-claim-next
        labels: [cli, docs, help]
        desc: |
          Add a one-line tip to `done --help`:

            Tip: pass --claim-next to atomically close this task and
            claim the next available leaf, collapsing the work loop
            into one call.

          Why: the flag exists today but is invisible to anyone who
          hasn't read the full --help. Surfacing it turns it into a
          reflex.

      - title: "`claim --help`: point at `claim-next` and `done --claim-next`"
        ref: help-claim-tips
        labels: [cli, docs, help]
        desc: |
          Add a tip to `claim --help`:

            Tip: see `claim-next` for "find and claim the next available
            leaf in one step" — and `done --claim-next` for the
            close-and-advance loop.

          Why: same surfacing problem as `done`. The two-step
          `done X && claim Y` flow is what users discover by default;
          the single-call versions should be one help page away.
```

## Out of scope (mentioned but not changing)

- **Claim auto-expiry visibility.** Not encountered in this session;
  flagged for future. May want a hint in `status` output if any of
  the user's own claims are within 5 min of expiry.

- **`--parent <id>` flag on `ls`.** Already covered by the positional
  argument; flag would be redundant.

- **Per-edge "unblocked" Log copy.** A separate finding from this
  session: the Log shows tasks as "unblocked" via per-edge events
  while the task remains blocked by other open blockers. The detail
  payload already records the specific `blocker_id`, so the Log row
  could read "unblocked (P5JAV done)" — clarifying that this was a
  per-edge unblock, not a state transition. User declined the change
  for now; noted here for future copy work.
