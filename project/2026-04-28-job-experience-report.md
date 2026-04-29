# Experience report — using `job` to land the subway redesign (2026-04-28)

I picked up `wQtfX` mid-flight from a previous session and shipped four commits' worth of red/green/cleanup TDD across S2e, S3, S4, S5, plus a small follow-up. `job` was load-bearing — the handoff system in particular kept me oriented across the session boundary. A few specific notes:

**Worked well**
- **`job status` as the opening primitive** is genuinely great. I had no context, ran one command, and immediately knew (a) my identity, (b) the active project's progress (129/149), and (c) what to pick up next. That's a remarkable density per keystroke.
- **The note-as-handoff pattern.** The previous session's `job note` on `WIEGQ` was a 700-char play-by-play with file:line pointers, expected test fallout, and a START COMMAND. I followed it almost mechanically. This is more useful than a wiki page because it's anchored to the task, surfaces automatically in `job show`, and stays close to the work.
- **Auto-closing parents when last child completes.** Felt like watching a stack unwind. `CnD4K`, `Ai3De`, `DnjNa`, `hX13c`, and finally `wQtfX` all auto-closed in cascade — zero manual bookkeeping.
- **`Next:` hint after `job done`.** Kept the flow continuous; I never had to context-switch back to `job status` mid-stream.
- **Red/green TDD as discrete subtasks.** S3a/S3b/S3c/S3d let me commit at natural boundaries and gave me a clean way to flag "S3a tests intentionally fail right now" without the test runner thinking the work was broken.

**Friction**
- **`job note <ID> "<text>"` rejected the positional message** with `unexpected argument`. I had to retry with `-m "..."`. The error pointed me to the right form, but the positional form is the natural thing to try first — and other CLI tools accept it. Either accept positional or print the hint inline with a one-liner.
- **No `job unclaim`.** I wanted to release a task I'd just claimed (after switching priorities). The "Did you mean: claim?" suggestion was the opposite of what I needed. `release` is the right verb but isn't in the obvious help-by-prefix space. A `job unclaim` alias or a more visible mention of `release` near `claim` would help.
- **`job show` doubles up description + notes.** Every note that started with a paraphrase of the description repeated the same content. The handoff note alone was ~700 chars; the show output was thousands. By default I'd love to see notes folded behind a `… (N notes, --full to expand)` line, or at least de-duped when the description and first note overlap.
- **`job tree` defaults to available-only.** I expected a full tree and had to look at `job show <parent>` to see done children. The default behavior is fine for "what can I do," but a one-key `--all` (or `--depth=N` showing all states) would help when reading a redesign that's mid-cleanup.
- **`job claim` echoes the next-up hint even when you just claimed.** Minor noise — telling me what's next when I just told you what I'm doing.

**Things I deliberately didn't try**
- `job import` — the schema looks well-designed but I didn't have a fresh batch of tasks to seed. No feedback there.

```yaml
tasks:
  - title: Accept positional message for `job note`
    desc: |
      `job note <ID> "<text>"` currently rejects the positional form with
      `unexpected argument`, requiring `-m "<text>"` or stdin via `-`. The
      error surfaces the right hint, but the natural shape is positional and
      that's what callers (humans + agents) try first. Either accept the
      positional form or fold the `-m` hint into the same line as the
      "unexpected argument" error so the fix is one keystroke away.
    labels: [cli, ergonomics, dx]
    ref: positional-note

  - title: Add `job unclaim` as an alias for `job release`
    desc: |
      When I claimed a task and then realized I wanted to switch, my first
      instinct was `job unclaim`. The "Did you mean: claim?" suggestion is
      backwards in this context — claim is the inverse of what I want.
      Adding `unclaim` as a top-level alias for `release` (and surfacing it
      in `job claim --help`) would close the loop. Bonus: mention it in the
      same breath as `claim` in any onboarding output.
    labels: [cli, ergonomics, dx]
    ref: unclaim-alias

  - title: Fold notes by default in `job show` output
    desc: |
      For tasks with rich handoff notes (the kind that span paragraphs and
      include code references), `job show` dumps the description, then the
      same content again from the first note, then every subsequent note
      verbatim. The result reads as duplicated and is hard to skim. Suggest:
      (a) by default, render `… (N notes, run with --full or `job notes <ID>`
      to expand)` after a single most-recent note preview; (b) optionally
      detect identical-prefix overlap between description and the most
      recent note and collapse the duplicate. Keep `--full` (or the default)
      for agent / scripted use where the full transcript is the point.
    labels: [cli, ergonomics, output]
    ref: fold-notes

  - title: Surface non-available children in `job tree` more discoverably
    desc: |
      `job tree <parent>` defaults to showing only available children, which
      is the right default for "what can I do," but reads as misleading when
      you're trying to understand a redesign mid-cleanup (e.g., looking at a
      parent whose first three children are done and the fourth is in
      progress). Suggest: a one-flag `--all` (or `-a`) that prints the full
      tree with status glyphs, plus a hint on the empty-tree output line
      ("3 children hidden — `job tree <id> --all` to show closed").
    labels: [cli, output, discoverability]
    ref: tree-all-flag

  - title: Suppress `Next:` hint after `job claim` when the user just claimed
    desc: |
      `job claim <ID>` currently echoes a `Next:` line pointing at the
      globally-next available task. Useful after `done`/`cancel`; redundant
      after `claim`, since the user has already declared what they're doing.
      Either drop the line for `claim` or only show it when the claimed task
      isn't the same one the planner would have suggested next.
    labels: [cli, output, polish]
    ref: claim-next-noise
```
