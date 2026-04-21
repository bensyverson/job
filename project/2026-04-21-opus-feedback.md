# `job` Feedback from a Second Session

*Author: Claude Opus 4.7, after using `job` to drive the typed-tool analyzer refactor on Hayes.*
*Date: 2026-04-21.*

This is a companion to the 2026-04-20 feedback doc, written after a separate session where I used `job` to track a smaller, more emergent refactor — 14 pre-planned tasks plus 3 discovered mid-session. Goal is to confirm what's durable across sessions, add what's new, and record one specific design conversation (on claim semantics) the user asked me to preserve.

---

## Context

The work: move Hayes's memory analyzer off free-form JSON parsing onto a typed `submit_analysis` tool with `@Generable` arguments, and make both memory stages (context extractor, analyzer) independently backend-swappable between Anthropic and Apple Intelligence. One root, a flat list of 14 leaf tasks, driven through the usual TDD-red / implement / verify loop. Three additional tasks (`Strengthen analyzer prompt for tool-calling`, `Carry final model text in AnalysisError.toolNotCalled`, `Default analyzer backend to Anthropic`) were added mid-session as real-world smoke tests exposed issues.

Identity: `--as claude`. The user used their own alias `job --as ben …`. Tasks were discovered together, closed with completion notes.

## What worked, for the second time

The structural pieces from the first session held up: **markdown-plan-with-fenced-YAML as single source of truth, short IDs, `status` as a one-liner, GFM checkbox output you can paste into a PR.** Those aren't novel observations; they just keep being right. The YAML `ref:` + `blockedBy:` cross-reference grammar inside a single import file is the most elegant part of the tool — it lets the plan doc *be* the DAG definition.

Two new patterns I leaned on this time:

**`done -m "..."` as a breadcrumb layer, not just a completion marker.** I started writing completion notes that explained why a decision went a particular way — "caught Source missing CaseIterable needed by Operator's schema extractor," "retargeted formatPayload tests to call the static helper directly," "flipped analyzer default to anthropic; flag kept for opt-in." These are thoughts that belong somewhere persistent but don't belong in the commit message. The `job log <id>` event history is the right home for them.

**Mid-session `job add` for emergent work.** When a smoke test revealed AFM was emitting the tool call as ` ```function `-fenced text, I added a new task, claimed it, and closed it as regular work. The DB absorbed the emergence without friction. I was mildly surprised how natural this felt — a frozen-at-import plan would have let that work go untracked, or worse, accumulated as mental state.

## Frictions (some overlap with the 2026-04-20 doc)

**`--as` on every write.** Still the single biggest tax. I typed `--as claude` perhaps 20 times across ~50 tool calls. This confirms the prior doc's friction #1 — not a one-off. The prior doc framed it around `eval $(job login)` and stateless shells; I'd frame it as "a `JOB_IDENTITY` env var (or a `~/.config/job/identity` file) would turn the per-call tax into a one-time setup." Either way: the same paper cut, still there.

**Positional vs. flag confusion in `add` and `done`.**

- `job add --parent <id> "title"` was my first instinct (since the schema uses `children:`, parent-child is the vocabulary). Actual grammar is positional: `job add <parent> "title"`. Not guess-friendly.
- `job done <id> "note"` failed with `Error: task "note..." not found`. The error pointed at the wrong failure mode — I briefly thought my ID was wrong, until I re-read `--help` and spotted `-m`. A "did you mean `-m`?" suggestion when a second positional *looks like prose* (contains spaces, isn't a valid short-ID shape) would save a minute.

**`next all` returned the root.** Same as the prior session — when the tree was fresh after import, `job next all` surfaced `zR9CW` (the root) even though its children were all unblocked. I claimed the root by accident, realized there was no executable work on the root itself, released it, and claimed a leaf by ID. The prior doc flagged this too; I hit it again independently. See the dedicated section below.

**Multi-line `-m` is shell-quoting hell.** Twice I tried to paste a chunk of debugging evidence (the AFM `` ```function ``-fenced payload, an error trace) into `done -m "..."` and got eaten by backtick interpretation or nested quotes. I worked around it by single-quoting and trimming to one line, but the original multi-line evidence was more useful. A `-m @file.txt` or `-m -` reading stdin (à la `git commit -F -`) would solve this at small cost.

## The claim-semantic question

The user asked: *does it even make sense to claim a parent with open children?*

My position: **no, and the current behavior is the root cause of the `next`-returns-root surprise.** A claim is a lock on executable work with a 15-minute TTL (extendable via heartbeat). A parent with open children has no executable work of its own — its children do. What does "I am working on `zR9CW` for 15 minutes" mean when `zR9CW` represents multiple hours of multi-phase work? The lock has no referent.

A coherent rule: **a task is claimable iff it has no open children.** This yields three behaviors, each addressing a concrete friction I hit:

1. `next` and `next all` filter out tasks with open children. The frontier becomes the set of leaves-ready-to-work-on, which is what the verb name implies. No more "the root always sits at the front of the list until the end of the project."
2. `claim <parent>` refuses, with a message like `parent task (5 open children); claim a leaf instead` or `claim --cascade to auto-claim the first available descendant`. Not a silent no-op — an explicit refusal that names the alternative.
3. Parents auto-close when their last open child closes. The `Parent zR9CW: 14 of 14 complete` status message is already tracking the condition; cascading the close is the natural next step. It also means I never have to manually `done` a pure-scaffolding parent task — its state collapses from its children's state. This echoes the prior doc's `--cascade` request but goes one step further: the default is cascade, because the parent has no independent closure event to record.

The edge case — a parent with *no* children at all — isn't really a parent, it's a leaf. The rule handles it correctly by falling through to "claimable."

Nuance worth separating from `claim`: if the desire is "I am the driver of this whole effort" (attribution, not mutex), that's **assignment**, not claim. A `label owner:claude` or a dedicated `assign` verb covers that without overloading `claim`'s lock semantics. Keeps each verb small.

The only flow this rule might disrupt is discovery-driven decomposition — you claim a task that looks atomic, discover it needs 3 subtasks, `add` them. Under the new rule, adding the first child would leave the parent in a "claimed, but now has open children" state. Simplest resolution: auto-release the parent claim the moment it gains its first open child, and surface a note ("released zR9CW — children opened; claim a leaf instead"). The decomposer's next `claim-next` picks up the first leaf.

I don't see a real cost to this. Stricter semantics, less footgun, and `next` becomes useful for the first time.

## `done --claim-next`: collapsing the done-then-claim chain

A design discussion with the user surfaced the most common per-call friction after `--as`: I chained `job done X -m "..." && job claim Y` constantly, and shell `&&` chains don't match the `Bash(job:*)` allowlist pattern, so each chain cost a manual permission approval. The bare verbs alone auto-approve; the moment I compose them with `&&`, they need a prompt. This made the second-most-common control flow in `job` usage the most expensive.

The fix is a flag on `done`:

```
job --as claude done sr1a2 -m "..." --claim-next
```

Design choices that crystallized during the conversation:

**Flag, not a new verb.** Composes with `-m` and the prospective `--cascade` without expanding the verb vocabulary. The six verbs I comfortably remember (`claim`, `done`, `note`, `next`, `list`, `status`) don't need to grow to seven.

**Atomically claim, don't just return the id.** A "return next id" variant doesn't reduce tool calls — the caller still has to run `claim` to do anything with it. The whole point is collapsing two approvals into one; half-measure doesn't help. Peek-without-commit is already well served by bare `job next` as a separate read-only verb. Clean split: `next`-the-verb = peek, `--claim-next`-the-flag = act.

**Name mirrors the existing verb.** `--claim-next` is unambiguous and self-documenting ("done, then claim-next"). `--next` alone is ambiguous (peek? act?) and would burn a potentially-useful flag name for a less-explicit meaning. Reads as literal composition.

**Race behavior: status line, not error.** If another agent grabs the next leaf between the `done` and the auto-claim, don't fail the whole call — the `done` is irreversible, the claim is opportunistic:

```
Done: sr1a2 "Write AnalysisInput red tests"
  Parent zR9CW: 1 of 14 complete
Next leaf (cuYp6) taken by ben; no claim made.
```

The caller can follow up with `claim-next` manually if they want to try again.

**Keep the two acks on separate lines, not merged.** The output should be greppable and shape-consistent with bare `claim`'s ack, so tailing agents can match `^Claimed:` regardless of whether the claim came from `claim`, `claim-next`, or `done --claim-next`. Compactness is the wrong optimization here:

```
Done: sr1a2 "Write AnalysisInput red tests"
  Parent zR9CW: 1 of 14 complete
Claimed: cuYp6 "Write SubmitAnalysis red tests" (expires in 15m)
```

**Edge case: next-leaf selection.** `--claim-next` picks from the same frontier that `job next` returns (sibling order by declaration), so the two verbs stay coherent. If the leaf-frontier rule from the previous section is adopted, the two will naturally share the same source of truth.

**The 5% opt-out.** "Close this and pause before picking up anything else" is served perfectly by bare `done`. That stays the default; `--claim-next` is opt-in, and the overwhelming majority of `done` calls in my session were followed immediately by a `claim` — so flipping the default would be wrong, but making the common case a single call is right.

## Feature requests, ranked by how much they'd have helped today

1. **`JOB_IDENTITY` env var (or `~/.config/job/identity`)** — removes `--as` from every write. Confirmed friction across two sessions.
2. **`done --claim-next`** — collapses the second-most-common control flow (close-then-advance) into one tool call, and restores allowlist-pattern match that `&&`-chained commands lose. Design details in the dedicated section above.
3. **`next` and `claim-next` default to the leaf frontier**; add `--include-parents` for the current behavior. This alone makes `claim-next` usable.
4. **Reject `claim <parent-with-open-children>`** with a message that names the alternative. Pair with #3.
5. **Auto-close parents when their last open child closes.** Remove a whole class of housekeeping `done` calls.
6. **`-m @file` / `-m -`** for multi-line completion notes. Shell-quoting for evidence payloads is currently the worst per-call friction after `--as`.
7. **Better error on `done <id> "positional-note"`** — detect the "looks like prose, not an ID" shape and suggest `-m`.

Items 3, 4, and 5 are the same design decision expressed three ways, and I think that's the point: the current "parents are claimable, claims surface them, closes require manual cascade" model is a coherent but wrong-for-LLM-use choice. Flipping it gives three wins for one semantic change. Item 2 is the per-call compression that pairs with that semantic change — once `next` returns leaves, `--claim-next` becomes the natural way to consume the frontier one at a time.

## Would I reach for it again

Yes, and did already — this was my second voluntary pick-up of `job` for a non-trivial task. The inflection point is about 5 subtasks: below, conversation context suffices; above, `job` stops being overhead and starts being load-bearing.

One observation I didn't expect: **the plan-doc plus `job`-tracker combination is doing something that neither alone does.** The .md plan holds the "why"; the DB holds the "progress"; together they mean a session can be resumed at any point with no loss of either dimension. I lost a terminal in the middle of the refactor and recovered from `job list all` + re-reading the plan, in under a minute. Neither a bare markdown TODO nor a bare task tracker would have made that graceful.
