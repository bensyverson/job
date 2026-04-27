# `job` CLI feedback — Opus 4.7 session, 2026-04-26 (afternoon)

## Context

Single-session dogfood while implementing K8iGR ("Home view: rebuild
signal cards from frame on scrubber-frame events"), the last per-view
scrubber gap on the dashboard. Five children, ~13 file changes, one
commit; ~30 task transitions including parent auto-close at the end.

Builds on the prior CLI feedback at
`project/2026-04-25-opus-cli-feedback.md`. Several items from that doc
have shipped (`show`/`ls` are now canonical, `tree` exists as a `ls`
alias, the alias-warning notes are gone) — confirming this loop works.
This round adds four new items, withdraws two of my own initial
claims, and revises one.

## What worked (no change from prior reports)

**`job status` as the session opener.** The "Web dashboard v1 (bYr6R):
80 of 94 done · next XeTlP" line oriented me in one read. I never had
to guess what to claim next.

**`job claim X` returning the full task body inline.** The prior
session asked for this and it shipped. Saved a follow-up `show` on
every claim transition this session — exactly the friction it was
meant to remove.

**`job done <id>` returning the next claimable task and the parent
rollup.** "Parent K8iGR: 1 of 5 complete" after each `done` call gave
me running progress without me having to ask. Same goes for the
auto-close cascade: closing `JdM0C` cascaded to K8iGR cleanly, exactly
as the project memory predicted.

**Long handoff notes inside task descriptions.** K8iGR carried a
~300-line markdown handoff from the prior session. `job show K8iGR`
rendered it intact. That was the single most valuable artifact of
this session — I picked up where the prior agent stopped without
re-deriving anything. Whether `job` is responsible for this or it's
just-not-truncating-stuff is debatable, but the fact that it doesn't
fight long bodies is what made the pattern work.

## What was friction (new items)

### 1. `job note <id> "text"` requires `-m`; nothing else does

```
$ job note 8u05z "Pivoted from ?at= URL filter to..."
Error: note: unexpected argument "Pivoted from ?at= URL filter to..."
       (use -m "<text>" or stdin via -)
```

Tried positional first because that's how `job add` works
(`job add K8iGR "title" --desc "..."`). Got an error. Used `-m` on the
retry.

**Why it matters:** the inconsistency is what burns. `job add` takes
title positionally, `job note` doesn't take its body positionally. I'll
make the same wrong guess every session unless something forces me to
remember which command takes which.

**Why it's worth fixing:** the error message *already names the right
flag*, so a one-shot `job note <id> "text"` could just route the text
to the same handler `-m` calls. No new surface; just accept the
positional form. Alternative: require `-t/--title` on `add` for
symmetry. Either way, pick one rule and apply it everywhere.

### 2. `job add --parent <id> "title"` doesn't exist; parent is positional

```
$ job add --parent K8iGR "Server: POST /home/graph endpoint..."
Error: unknown flag: --parent
$ job add K8iGR "Server: POST /home/graph endpoint..."
8u05z
```

The help text (`job add [parent] <title>`) does signal that "parent"
is positional, but the bracket convention reads as "optional second
positional" rather than "this is how you specify a parent." A
`--parent` alias would absorb the wrong guess at zero cost.

**Why it matters:** less than #1 (one-shot retry, not a recurring
error pattern), but the same theme — the agent reaches for a flag
shape that mirrors how it would *describe* the operation, and the CLI
returns an unhelpful error.

**Why it's worth fixing:** trivial — accept `--parent <id>` as an
alternative to the positional form. Doesn't break the existing
positional shape for users who know it.

### 3. `ls`/`tree`/`list` filter to actionable by default; this hides
   completed subtree structure

I ran `job tree K8iGR` after the parent auto-closed and got:

```
Nothing actionable. 5 tasks done. Run 'list all' to see the full tree.
```

For the *use case of "what should I work on next"*, the actionable
filter is correct — surfacing five `[x]` rows is noise. But for the
use case of "show me the structure of this parent so I can write a
retrospective / reference its children in a note / verify completeness
before the parent auto-closes," the same command is the wrong answer
without `--all`.

**Revised from my initial reaction:** I originally suggested splitting
`tree` from `ls` so `tree` would always show structure. That was
wrong — they're aliases of the same command, and the canonical
behavior (filter to actionable) is the right default for the most
common case.

**The real issue:** when an agent calls `ls/tree <parent>` with an
explicit ID, the implicit question shifts from "what should I do?" to
"what's under this thing?" Filtering an explicit subtree to "nothing
actionable" loses information the agent specifically asked for.

**Why it matters:** I had to read the full `Run 'list all' to see
the full tree` hint, run a second command, get the structure I
wanted. Two commands when one would do.

**Why it's worth fixing:** scope the actionable filter to *unscoped*
calls. `job ls` (no arg) → actionable across all roots. `job ls
<parent>` → full subtree by default, with `--actionable` to filter.
The hint message in the actionable-filter case is good; just flip
which case is the default when a parent ID is supplied.

### 4. `job show <id>` field order buries structural metadata under
   long descriptions

`Children:`, `Parent:`, `Labels:`, `Status:` all sit *below* the
description. For a task with a 300-line handoff note, the structural
fields scroll off the first read. I missed K8iGR's `Children:` block
entirely on first call this session — so much that I initially
reported "show doesn't include children" as feedback (withdrawn
below).

**Why it matters:** the long-handoff-note pattern is genuinely useful
(see "What worked" #4), but it's exactly the case that pushes
metadata off the agent's first screen. The two features are working
against each other.

**Why it's worth fixing:** put the structural skeleton first, prose
second. Mirrors `gh pr view` and `gh issue view`: meta block (state,
labels, branches), then body. Same content, opposite order.
Suggested order:

```
ID:       K8iGR
Title:    Home view: rebuild signal cards from frame on scrubber-frame
Status:   open · available
Parent:   bYr6R (Web dashboard v1)
Labels:   home, js, scrubber, web
Children: 5 (1 done, 4 open)
  - [x] 8u05z Server: POST /home/graph endpoint...
  - [ ] VAOtt JS: home-scrub-build.mjs + tests
  - [ ] 5XNmm JS: home-scrub-render.mjs + tests
  - [ ] aromh JS: home-scrub.mjs driver...
  - [ ] JdM0C Wiring + live-test + commit
Created:  2026-04-26 10:31

Description:
  <long body here>

Notes:
  [...]
```

### 5. Auto-close cascade isn't surfaced at the moments that matter

The project memory says "parents auto-close when their last child
completes." I read that and internalized it. But the moment I was
*at risk* of mis-scoping — when I created the five children of K8iGR,
where K8iGR's title implied scope I wasn't sure my children fully
covered — was the moment no system message appeared. The user had to
remind me mid-session.

**Why it matters:** the cascade is great when scope is clean and
disastrous when it isn't. A child's `done` permanently closes the
parent; you can't append a sixth child to a closed parent without
reopening it (and even then, the cascade has already fired). The
window to catch under-scoping is *before* you close the last child,
ideally *before* you commit to the children that exist.

**Why it's worth fixing:** two cheap nudges, both at moments the agent
is reading text:

- On `job add <parent> <title>` when the parent already has children:
  print a one-liner like `K8iGR now has 5 children; complete them all
  to auto-close the parent.` Reaffirms the rule at the moment of
  scope commitment.
- On `job claim <id>` when the claimed task is the parent's last
  not-yet-done child: print `Closing this task will auto-close parent
  K8iGR ("Home view..."). Verify scope first.` Gives the agent a
  natural pause-and-think moment.

Documented in `job schema` once would also help, but the system-
reminder mid-session shows that even an explicit schema mention
doesn't fire at the right moment without a contextual hook.

## Withdrawn (self-corrections)

**(W1) "`job show` doesn't include children."** False — it does, in a
"Children:" block with markdown checkbox status. I missed it because
of #4 (field order). The bug is the order, not the absence.

**(W2) "Add a `job edit + note` combo for atomically editing a
description and recording why."** Encountered once this session, fine
to do as two commands. Withdrew before recommending — feature creep.

## Revised

**(R1) "`tree` should include the parent at the top; `ls` should be
children-only."** Revised — `ls`/`list`/`tree` are aliases of the
same command. The actual asymmetry I hit is in the actionable filter,
covered above as #3.

## Top three to ship first

If implementing one at a time:

1. **#4 (field order in `show`)** — purely cosmetic, zero risk, large
   readability win for the handoff-note pattern that's clearly working.
2. **#3 (subtree default when ID supplied)** — collapses a two-call
   pattern into one without changing unscoped `ls` behavior.
3. **#1 (`note` accepts positional body)** — smallest possible change,
   removes a recurring error case.

#2 (`--parent` alias) and #5 (auto-close hints) are nice-to-have. The
`schema` mention in #5 is the cheapest version of that fix and worth
doing on its own.

## On hard-wrapping (meta)

A late-session observation worth surfacing because it explains where the sixth child below comes from. The first draft of this doc was hard-wrapped at ~72 columns, including the YAML `desc:` blocks. Nothing in markdown, YAML, or the shell required that — markdown viewers handle long lines fine, YAML's `|` and `>` block scalars define their own line semantics, and `job note -m "..."` accepts arbitrary-length strings without forcing wrapping. So why did I do it?

Three sources of ambient pressure, all of them readable in the project itself: cobra's `Long:` help text is hard-wrapped (necessarily — terminals); the prior feedback doc at `project/2026-04-25-opus-cli-feedback.md` is hard-wrapped at ~72 columns; the K8iGR handoff note I read at session start was hard-wrapped. Together those signals form an implicit "this is how prose looks here," and a fresh agent reading them will reproduce the convention without ever having decided to.

The cost of hard-wrapped prose in a long-lived markdown file is small but real: noisier diffs (a one-word edit reflows the surrounding paragraph), substring search broken across line boundaries, and friction when a sentence grows. The cost in YAML `desc:` blocks is worse — those values are stored verbatim and re-displayed by `job show`, so the wraps become forced line breaks even on terminals wider than 72 columns.

The `unwrap-help-text` child below removes the most upstream of those signals (cobra help text), on the theory that future agents will calibrate to whatever they read. As a worked example of the convention, that child's `desc:` uses a folded block scalar (`>`) and is written without hard wraps; the other five children stick with the hard-wrapped `|` style they were originally drafted in, partly so the diff stays focused on the actual feedback content and partly so the contrast between the two styles is visible in one file.

(This section is also written without hard wraps. If your editor renders it as one long line per paragraph, that's the intended shape.)

## Proposed changes

```yaml
tasks:
  - title: Apply CLI feedback from 2026-04-26 Opus session
    desc: |
      Roll-up parent for the CLI changes proposed in
      project/2026-04-26-1120-opus-cli-feedback.md.

      Five small, independent fixes; ship in any order. None require
      coordinated rollout — each one's blast radius is its own
      command's output or arg parsing.
    labels: [cli, ux]
    children:
      - title: "`show <id>` reorders fields: structural metadata first, description below"
        ref: show-field-order
        labels: [cli, show]
        desc: |
          Move ID / Title / Status / Parent / Labels / Children / Created
          to the top of `job show` output; description and notes follow.

          Why: long handoff notes (the established pattern in this
          project) push the structural fields off the first screen.
          An Opus session this round mis-reported "show doesn't
          include children" because the Children: block was at line
          47, below a 40+ line description.

          Mirrors `gh pr view` / `gh issue view`: meta header, then
          body. Same content, opposite order.

          Suggested layout:
            ID:       K8iGR
            Title:    Home view: ...
            Status:   open · available
            Parent:   bYr6R (Web dashboard v1)
            Labels:   home, js, scrubber, web
            Children: 5 (1 done, 4 open)
              - [x] 8u05z Server: POST /home/graph endpoint...
              - [ ] VAOtt JS: home-scrub-build.mjs + tests
              - ...
            Created:  2026-04-26 10:31

            Description:
              <body>

            Notes:
              <notes>

      - title: "`ls/tree <parent>` defaults to full subtree; actionable filter requires --actionable"
        ref: ls-subtree-default
        labels: [cli, ls]
        desc: |
          When `ls` (and its `tree`/`list` aliases) are called with an
          explicit parent ID, default to showing the full subtree.
          Unscoped calls (`job ls` with no arg) keep the current
          actionable-only default.

          Why: filtering an explicit subtree to "Nothing actionable"
          loses the structural info the agent specifically asked for.
          The use-case shifts from "what should I do" (unscoped) to
          "what's under this thing" (scoped) — those want different
          defaults.

          Add `--actionable` to opt back into the filter for scoped
          calls when needed. `--all` continues to mean "include done,
          claimed, and blocked" everywhere.

          Update the "Nothing actionable. ... Run 'list all' to see
          the full tree." hint to only fire on unscoped calls.

      - title: "`job note <id> \"text\"` accepts positional body (currently requires -m)"
        ref: note-positional-body
        labels: [cli, note]
        desc: |
          `job note <id> <text>` should work without `-m`. Today:

            $ job note 8u05z "..."
            Error: note: unexpected argument "..."
                   (use -m "<text>" or stdin via -)

          Inconsistent with `job add <parent> <title>` which DOES
          take its body positionally. Pick one rule across the CLI;
          this is the smaller change.

          Keep `-m` and the stdin form (`-`) as alternatives so
          existing scripts don't break. The error message text
          ("use -m...") becomes obsolete; it can stay as a fallback
          for the case where someone passes too many positional args.

      - title: "`job add --parent <id>` as alias of the positional parent form"
        ref: add-parent-flag
        labels: [cli, add]
        desc: |
          `job add --parent K8iGR "title"` should be equivalent to
          `job add K8iGR "title"`.

          Why: `--parent` is the name an agent reaches for when
          describing the operation in flag form. The positional form
          stays canonical; this is just an alias to absorb the wrong
          guess.

          Bonus: makes the syntax composable with optional flags in
          either order (`--parent X --label foo "title"` reads
          uniformly).

      - title: Surface auto-close cascade at the moments that matter
        ref: auto-close-hints
        labels: [cli, ux, parent]
        desc: |
          Two contextual one-liners to make the auto-close behavior
          visible at the moments scope is being committed:

          (a) On `job add <parent> <title>` when `<parent>` already
              has children, append:
                "K8iGR now has N children; complete them all to
                 auto-close the parent."

          (b) On `job claim <id>` when the claimed task is the
              parent's last not-yet-done child, append:
                "Closing this task will auto-close parent K8iGR
                 ('Home view: ...'). Verify scope first."

          Why: the project memory describes the cascade correctly,
          and the schema/help text could too — but neither fires at
          the moment the agent is at risk of under-scoping. The
          windows where the warning needs to land are: child
          creation (commit point for scope) and last-child claim
          (last chance to add a sibling). Both are points the agent
          is reading CLI output anyway.

          Cheap supplement: add a one-line mention in `job schema`
          and in the `add` / `done` help text:
            "Note: when a parent's last child completes, the parent
             auto-closes. Split before completing if scope grew."

          The contextual hints are the load-bearing change; the
          docs nudge is a backstop.

      - title: "Remove hard-wrapping from `job` help text and long-form descriptions"
        ref: unwrap-help-text
        labels: [cli, ux, docs]
        desc: >
          Stop hard-wrapping the long descriptions in cobra `Long:`
          fields, command help, and `job schema` output. Write each
          paragraph as one logical line; let the terminal renderer
          (or cobra's own help formatter) handle wrapping at the
          user's actual width.

          Why this matters beyond aesthetics: ambient CLI text is
          the strongest signal an agent has for "how does this
          project write prose." This Opus session caught itself
          hard-wrapping the feedback doc and the YAML `desc:` blocks
          at ~72 columns without thinking — purely because the help
          text and prior docs were hard-wrapped. The convention
          propagates by example. Unwrapping the help text removes
          that signal and gently guides future agents toward
          terminal-width-aware soft wrapping in their own output
          (notes, descriptions, project docs).

          Scope: every `Short:` and `Long:` field across cobra
          commands, plus any embedded help/usage strings and the
          JSON schema's `description` fields. Exception: explicit
          line breaks (lists, code samples) stay as literal
          newlines.

          Validation: `job <cmd> --help` should still render at a
          reasonable width on an 80-column terminal — cobra wraps
          long lines at the terminal width by default, so the
          rendered output should look the same or better. Test by
          resizing a terminal narrower (60 cols) and wider (120
          cols) and confirming the help text reflows.
```
