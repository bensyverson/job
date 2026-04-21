# Feedback on `job` from an LLM Agent's Perspective

*Author: Claude Opus 4.7, after using `job` to drive a day-long refactor of the Hayes memory system.*\
*Date: 2026-04-20.*\

This document captures my experience using `job` as the task-management layer for a 26-subtask, 6-phase refactor, and the concrete changes I'd recommend based on what worked and what didn't. It is organized in two parts: reflections on the experience, and a specification of recommended changes (what / why / how).

---

## Part I — Reflections

### Context

I used `job` end-to-end to drive a refactor of Hayes's memory pipeline: rewriting the analyzer, collapsing a three-field result shape into one, deleting the `Act`/`ActStatus` model and its SQL table, rewriting the middleware, and updating docs. The task tree had one root, six phases, and 26 leaf subtasks. Sole operator: me, with one human (ben) reviewing and steering. The database lived at the project root.

The salient runtime constraint: my `Bash` tool spawns a fresh shell for every invocation. Environment variables set in one call do not persist to the next. This shaped every interaction with `job` that depended on shell state.

### What worked well

**The hierarchy-plus-notes combination was the standout.** Mapping "phases → subtasks" to `job`'s parent-child structure was natural for a refactor with six roughly-sequential phases. Writing a short note on each `done` (what got accomplished, files touched, gotchas) produced an audit trail that was more useful than either `git log` (commit-scoped, usually broader) or the PR description (feature-scoped, usually narrower). The notes were valuable not just to a future reader but to in-session me — when I asked myself "wait, did I already finish the AnalysisResult collapse or just the tests?", `job list all` with notes answered faster than re-reading the diff.

**Addressable IDs.** The 5-character slugs were terse enough to reference inline in prose ("completed as part of `086vp`") and stable enough to embed in the plan doc at `project/2026-04-20-feedback-driven-memory-plan.md`. That gave me a two-way link between plan and tracker.

**Persistence across sessions and shell invocations.** Unlike a per-session todo list, the `job` tree survives tool-call boundaries, shell restarts, and conversation compaction. Coming back to the refactor mid-session, `job list all` re-oriented me without forcing me to reread chat history.

**Claim-with-expiry.** I appreciated the 1h TTL on claims without having to ask for it. It meant a crashed session didn't poison the tree — claims would eventually auto-release. Good default.

### What was frustrating

**The `eval $(job login)` handoff was the biggest single friction.** The login model assumes a persistent shell: `job login` prints `export` statements for the caller to `eval`. My runtime spawns fresh shells per `Bash` tool call, so env vars set by `eval` in one call evaporate before the next. The practical consequence: I prefixed `JOBS_USER=claude JOBS_KEY=0n7c5bv2` on literally every `job` invocation. Thirty-plus `note`/`done`/`claim` calls each paid that tax.

Compounding this, `job login <name>` emitted a "Created ..." status line to stdout that `eval` tried to execute as a command, producing `command not found: Created`. It was non-fatal (the env-setting exports ran anyway), but it was noise in every login attempt.

**No bulk creation.** Drafting the task tree required 28 sequential `job add` calls. I batched them into a single `Bash` invocation via a shell script, but that's me hand-rolling what should be a primitive. The underlying pattern — "LLM has structured plan, wants to import it in one operation" — showed up again at the end of the refactor when I needed to close six parent tasks in dependency order (I used a `for` loop).

**Every `for` loop I wrote against `job` was a diagnostic that the tool was missing a primitive.** Bulk create, cascade close, variadic delete, multi-close-with-notes. LLMs think in sets and trees; each `for` loop was me translating set/tree operations into scalar ones.

**`claim-next` claimed the root, not a leaf.** When I first ran it on a freshly-imported tree, I expected "give me the next actionable leaf." I got the root. I had to claim individual leaves by ID thereafter, losing the convenience of `claim-next` entirely.

**`done` on a parent with open children errored.** This is correct safety, but at the end of the refactor I had to close seven parent IDs in sequence with individual commands. A cascade option would have collapsed this.

**`note` and `done` were two separate commands.** I almost always wanted to close a task *with* a completion note. Two calls where I wanted one; small per-call but added up over 26 subtasks.

**`list` hiding done tasks by default** was right for "show me actionable work" but wrong for "show the human progress." I discovered `list all` late and used it repeatedly once I did.

**Claim-next-picks-root and done-requires-children-closed were both surprising.** They stemmed from me not reading the mental model carefully enough, but an LLM's pattern is often "try the obvious verb and react to the error." Error-then-recover is fine if the errors are informative enough to self-correct; these ones were.

### Design principles that emerged

These crystallized over the course of using `job`, and shaped the recommendations in Part II:

1. **Every tool call is expensive.** A permission roundtrip, latency, context pollution. Every response is free real estate for context the caller would otherwise need a second call to retrieve. Enrich success acks. Tell me not just what happened but what I can do next.

2. **LLMs think in sets and trees; CLIs often expose scalars.** Every `for` loop I write is a missing primitive. Variadic verbs, cascade operators, tree-aware filters. Each of these collapses N tool calls into one.

3. **Agent runtimes vary wildly in their ability to persist shell state.** Some have env hooks, some don't. A CLI that relies on shell-session state (env vars set via `eval`, cwd context) will feel janky to any agent without a persistent shell. Flag-based state is strictly more portable.

4. **No silent defaults for identity.** In a multi-agent setting, fallback rules that "helpfully" pick a default can cause accidental impersonation. Require identity explicitly for writes; refuse to guess.

5. **Success acks should answer "what's next?"** That's always the caller's immediate next thought after any write. A `Done: 87TNz` that also names the next sibling is twice as useful for the same response size.

6. **Token-efficient default; structured opt-in.** Dense Markdown-like output is both human-readable and LLM-parseable at 2-3× the token efficiency of JSON. `--format=json` exists for deterministic parsers (`jq`, subscriber agents on live streams where partial-line ambiguity matters, programmatic chaining) — not as a "machine-readable" alternative, because LLMs parse the dense default fluently. The implication compounds: because the default is the preferred format for both humans and LLM readers, enriching it further (checkboxes, code-span IDs, parenthetical state annotations) has multiplying value — every character carries more signal-per-token than JSON does.

7. **`job` is closer to a multi-agent orchestration substrate than a tracker.** The existing primitives (claim, note, done, tail, TTL) are already 80% of what multi-agent coordination needs. The remaining 20% is event semantics — cancellation, structured events, ready-sets, heartbeats. Additive, not a rewrite.

---

## Part II — Recommended Changes

Each recommendation is labeled **Critical**, **Valuable**, or **Nice-to-have** and flagged as **Breaking** or **Additive**. Within each section: *what* (the change), *why* (the motivation), *how* (the concrete interface).

### 1. Attribution: drop keys, require `--as` for writes **[Critical, Breaking]**

**What.** Remove `job login`, `job logout`, `JOBS_USER`, `JOBS_KEY`. Identity is resolved from `--as <name>` (global flag, before the verb). No env-var fallback, no `$USER` fallback, no identity file. Writes require `--as`; reads do not.

**Why.** The key-based model adds no real security over a local SQLite file (any writer can bypass it by editing the db directly) but costs every caller the ceremony of `job login` and the `eval` handoff. The silent-default models we considered — falling back to `$USER`, or reading a shared identity file, or an env-var fallback — all create an impersonation path in multi-agent scenarios or add hidden state that makes "who ran this?" ambiguous. Forcing `--as` on every write is the simplest model that's safe: the identity is visible in the command itself, auditable from shell history, and impossible to accidentally inherit. Humans who want "set once, forget" use a shell alias (`alias job='job --as alice'`); agents wrap or template the prefix at their integration layer. Aliases put the identity in the user's dotfiles — explicit — rather than hidden in env.

**How.**

- **Global flag only**: `job --as <name> <verb> ...`. Position: before the verb, matching conventions like `git -C <path>`.
- **No env-var fallback. No `$USER` fallback. No identity file. No keys.**
- **Required for writes**: `add`, `import`, `edit`, `label`, `block`, `unblock`, `move`, `claim`, `claim-next`, `heartbeat`, `release`, `note`, `done`, `reopen`, `cancel`. Missing `--as` on these errors with `Error: identity required. Pass --as <name> before the verb.`
- **Not required for reads**: `list`, `info`, `log`, `status`, `next`, `schema`, `tail`. These work identity-less.
- **`init` is unattributed**. It creates the db without writing any event attributed to a user.
- **Claim-conflict errors** on writes: if task is claimed by alice and bob tries to `done` it, error with `Task 87TNz is claimed by alice (expires in 12m). Wait for expiry, or ask alice to release.` (`release` operates only on the caller's own claims, so the error must direct the caller to the correct remedy.)
- **Stolen-claim surfacing**: if A's claim on X expired and B has since claimed it, A's next write against X errors with `Your claim on 87TNz expired; it is now held by bob (claimed 3m ago). Run 'claim 87TNz' to take it back.` This gives A a chance to investigate whether their in-flight changes should be abandoned.

**Migration.** Existing dbs with attributed events keep those attributions (they're just strings; no schema change required). Existing users drop `eval $(job login)` and add one line to `.zshrc`: `alias job='job --as <their-name>'`. Agents: the integration layer that used to set `JOBS_USER`/`JOBS_KEY` now prefixes `--as <name>` on every write-verb invocation (or wraps `job` in a script).

### 2. Markdown-embedded YAML import **[Critical, Additive]**

**What.** `job import <file>` reads a Markdown document, finds the first fenced YAML block whose root key is `tasks:`, and ingests it as a subtree. Supports `--dry-run`, `--parent <id>`, `--format=json`.

**Why.** A plan doc naturally ends with a declarative task list. Making the doc the source of the tracker eliminates the gap between planning and execution — no 28 sequential `job add` calls, and the plan in the doc is the thing that got imported. The YAML block functions as an initial-plan snapshot; once ingested, the tracker is the live state (the block is not kept in sync).

**How.**

- **Input format**: a YAML block in a fenced Markdown code fence (`` ```yaml ... ``` ``), with root key `tasks:`.

    ```yaml
    tasks:
      - title: Refactor Hayes memory to feedback-driven edges
        desc: |
          Multi-line description...
        labels: [refactor, memory]
        children:
          - title: Phase 1 — Data model
            children:
              - title: Write red tests for Lesson
                desc: Covers Codable, snake_case source, unknown-value rejection.
    ```

- **Discriminator**: `tasks:` root. Any other YAML blocks in the document are ignored. This means YAML examples elsewhere in the plan don't accidentally get interpreted.
- **Required vs. optional fields.** `title` is required. `desc` and `labels` are strongly encouraged — they carry context that makes later `log` readouts, retrospection, and dispatch filtering work well, but neither is required. `children`, `ref`, and `blockedBy` are optional (see below).
- **Blocks**: tasks can declare block relationships via two optional fields — `ref` (author-chosen identifier for tasks that are *block targets*) and `blockedBy` (list of refs or verbatim titles pointing to prerequisites). Refs are sparse: declare them only on tasks that are actually block targets, so most tasks carry no ref at all and no duplication tax.

    ```yaml
    tasks:
      - title: Write red tests for Lesson
        ref: red-tests
      - title: Implement Lesson struct
        blockedBy: [red-tests]
      - title: Run coverage
        blockedBy: ["Write red tests for Lesson"]   # verbatim title also resolves
    ```

  - **Resolution order**: each `blockedBy` entry is looked up first as a ref, then as a verbatim title. The ref namespace is **flat across the whole import**, so cross-subtree block references work (Phase 3's task can block on a ref declared in Phase 1).
  - **Ambiguity errors**: if a `blockedBy` entry matches no ref and no title, or matches multiple tasks by title, the import fails atomically with a YAML-path error naming the unresolved or ambiguous entry.
- **Response**: subtree echo, in input order, with IDs filled in. Default Markdown format, `--format=json` opt-in. Only the newly-created subtree is returned — not the whole tree. Order within each parent is preserved, which is how callers correlate echoed IDs back to their input when two tasks happen to share a title at different tree levels (the path is stable, and the echo follows the path).
- **`--dry-run`**: preview what *would* be created, with placeholder refs (`<new-1>`, `<new-2>`, ...) rather than real IDs. The placeholders do not persist; running without `--dry-run` generates fresh real IDs.
- **`--parent <id>`**: insert the subtree under an existing task instead of at the root.
- **Atomic semantics**: on any validation error, no tasks are created. The import is transactional.
- **Error reporting**: cite YAML path, not line number. `tasks[2].children[0].title: required field missing`. Paths are directly actionable for LLM callers editing structurally.
- **`job schema`** emits a JSON Schema for the import format to stdout. Useful for YAML-aware editors giving real-time validation, and for LLMs constructing imports with the schema in context.

### 3. Checkbox rendering in list output **[Valuable, Additive]**

**What.** Replace the trailing `[done]` tag with GitHub-flavored task-list checkbox notation.

**Why.** Three compounding wins: (1) checkboxes render as actual checkboxes in any Markdown viewer (GitHub PR descriptions, issue trackers, docs), so pasting `job list all` into a PR becomes a rendered status list; (2) the state marker is at the *front* of each line, where both human eyes and LLM readers land first; (3) each character is carrying more information than JSON would at the same token cost — the universal idiom (`[x]` / `[ ]`) is immediately recognized by both audiences, so no vocabulary has to be invented or learned. This is the principle from Part I §6 in action: the dense default is doing the work for humans and machines alike.

**How.** Format per task:

- `- [ ] ` — open
- `- [x] ` — done
- `- [-] ` — in progress (claimed)

Non-binary state (claimed-by, expires-in, blocked-on) goes in a trailing parenthetical:

    - [x] 87TNz Phase 1 — Data model
      - [x] s1Ut5 Write red tests for Lesson
      - [-] 9aedB Implement Lesson struct (claimed by claude, 25m left)
      - [ ] 2F1C1 Collapse AnalysisResult (blocked on 9aedB)

IDs render as Markdown inline code (`` `87TNz` ``) — monospace in Markdown viewers, visually distinct from titles, and robust to copy-paste.

### 4. Enriched success acknowledgments **[Critical, Additive]**

**What.** Every write verb's success output includes not just what happened but what's next. Concretely, `done` surfaces the next claimable sibling (if any), the parent's completion progress, and escalation when a parent becomes closeable.

**Why.** The caller's immediate next thought after `done` is always "what do I work on next?" Answering in the same response saves a follow-up `list` or `next` call. For LLM callers paying per-call context cost, this is nearly pure upside.

**How.** Mid-phase (siblings remain):

    Done: s1Ut5 "Write red tests for Lesson"
      Next: 9aedB "Implement Lesson struct"
      Phase 87TNz: 1 of 3 complete

Last child (phase complete):

    Done: 2F1C1 "Collapse AnalysisResult"
      Phase 87TNz complete — run 'job done 87TNz' to close it.
      Then: moZ5d "Phase 2 — Analyzer: prompt + parser + tests"

Whole tree:

    Done: wqCIz "Commit"
      All tasks in f3PDy complete. (26 done, 0 open)

Skip-blocked case:

    Done: s1Ut5
      Next sibling 9aedB is blocked on y7P4k. Skipping to 2F1C1.

**Principle**: every success ack should carry enough context to inform the next action without a follow-up read.

### 5. Atomic close-with-note and structured results **[Valuable, Additive]**

**What.** `done` accepts optional `-m "<note>"` and `--result '<json>'` flags, either or both.

**Why.** Nearly every close in my session was preceded by a `note` call. Collapsing them into one atomic operation halves the tool-call cost. The separate `--result` field supports orchestration workflows where parent agents need structured outputs (commit SHAs, test counts, artifact paths) from subagents, distinct from prose notes.

**How.**

    job done 87TNz                                      # neither
    job done 87TNz -m "all tests green"                 # note only
    job done 87TNz --result '{"tests": 105}'            # result only
    job done 87TNz -m "all green" --result '{...}'      # both

Same pattern on `note`: `job note 87TNz -m "..." --result '{...}'` writes an intermediate checkpoint without closing.

Fields are orthogonal: `log` renders notes as prose; `--format=json` surfaces both as structured fields. A parent task's result can be composed from children's results in rollup views.

**Stdin support for notes**: `swift test 2>&1 | job note 87TNz -` reads the note body from stdin. Useful for capturing full command output without manual summarization.

### 6. Variadic and cascade operations **[Valuable, Additive]**

**What.** Verbs accept multiple IDs where it makes semantic sense, plus `--cascade` for tree-shaped operations.

**Why.** LLMs do set/tree operations all the time (close a phase's subtasks together, reopen an entire branch). Hand-rolling these with shell loops costs tool calls and makes error handling fragile.

**How.**

- `job done id1 id2 id3` — close multiple tasks in one call; all-or-nothing atomic (on error, no state change).
- `job done --cascade <id>` — close the target plus all open descendants.
- `job reopen --cascade <id>` — symmetric.
- `job cancel --cascade <id>` — cancel a subtree.
- `--format=json` returns a structured per-ID result array so partial-failure scenarios (if ever needed) stay diagnosable.

### 7. Mutation: `edit --desc` **[Valuable, Additive]**

**What.** `job edit <id> --desc "..."` replaces the task description.

**Why.** Today `edit` only changes titles. When scope or understanding evolves mid-refactor (which happened multiple times this session), there's no way to revise a description — `note` appends rather than replaces. Locking the description at creation time forces drift between what a task means now and what its description says.

**How.** `edit` accepts `--title <new-title>` and `--desc <new-desc>` independently or together. Both are replace operations.

### 8. Orchestration primitives for multi-agent workflows **[Critical, Additive]**

This set of changes is what makes `job` a viable substrate for multi-agent coordination, not just a solo tracker. The existing primitives (claim, note, done, TTL, tail) are already most of what's needed; these additions close the remaining gaps.

#### 8.1 Cancellation (consolidating with `remove`)

**What.** `job --as <name> cancel <id> --reason "..."` marks the task as canceled, emits a `cancel` event on `tail`, preserves the task in the audit trail. A `--purge` flag promotes cancellation to destructive deletion for the rare cases (sensitive data in a title, bulk-import mistake) that genuinely require erasure. The existing `remove` verb is retired; `cancel --purge --reason "..."` replaces it.

**Why.** Orchestrators revise plans mid-execution. Without cancellation, revisions happen out-of-band (kill the subagent process) and leave zombie claims on the tree. Subagents subscribed via `tail` see the cancel event and exit cleanly. Keeping both `cancel` and `remove` as separate verbs invites every caller to ask "which one do I want?" on every cleanup operation — when the honest answer is "cancel, almost always." Collapsing into a single verb with a destructive flag keeps the safety distinction (you cannot accidentally `--purge`) while simplifying the mental model to "`cancel` when you want to stop work."

**How.**

- **Default (non-destructive)**: `job --as alice cancel 87TNz --reason "priority shifted"`. The task stays in the db with state `canceled`. A `cancel` event fires on `tail` with `{id, by, at, reason}`. Canceled tasks appear in `list all` but are hidden from default `list`.
- **Cascade**: `cancel --cascade <id>` cancels the task and all open descendants in one operation. Each cancellation emits its own event so subscribers see the full propagation.
- **Destructive**: `cancel --purge <id> --reason "..."` erases the task and its history from the db. No events fire (nothing left to observe). `--reason` is mandatory with `--purge` to force deliberate use. `--purge --cascade` on a subtree requires `--yes` as a terminal confirmation: `Will purge <id> and N descendants. Confirm with --yes.`
- **Subagent contract**: subagents tailing their assigned task or subtree are expected to handle `cancel` events by releasing any claims and exiting. The orchestrator's responsibility is to emit the cancellation; subagents' responsibility is to heed it. `--purge`d tasks cannot be observed via `tail` at all — if a subagent's task is purged, the subagent sees its claim vanish on next heartbeat and should exit.
- **Migration**: existing `remove` callers rewrite as `cancel --purge --reason "<why>"`. Mechanical, one-line change; documented in release notes.

#### 8.2 Structured events on `tail --format=json`

**What.** `tail` streams one JSON object per line when `--format=json` is set. Covers all state transitions: `claim`, `release`, `close`, `reopen`, `note`, `cancel`, `expire`, `heartbeat`.

**Why.** Parsing terminal-formatted output for orchestration is brittle. Structured events are robust to output-format changes and directly consumable by subscriber agents. Event ordering is total and monotonic (serialized by the underlying SQLite transaction log).

**How.** Each event has at minimum: `{"event": "<type>", "id": "<task-id>", "by": "<user>", "at": "<iso8601>", ...}`. Type-specific fields: `close` includes `result` if set; `note` includes the note body; `cancel` includes the reason; `expire` includes the previous holder.

Supporting flags: `--events <types>` (filter by event type), `--users <names>` (filter by actor), `--tree <id>` (scope to a subtree).

**Heartbeat noise**: heartbeat events fire often (potentially every couple of minutes per claimed task) and would dominate a live stream. Default `tail` filters them out. Subscribers that want to see them opt in explicitly with `--events heartbeat` (alone or combined with other event types).

#### 8.3 `job next all`

**What.** `job next` returns the next claimable task (leaf-preferring, unblocked, unclaimed). `job next all` returns all currently-claimable tasks — the parallel frontier.

**Why.** An orchestrator dispatching subagents needs to know "what can be worked on right now." Without this, the orchestrator walks the tree manually to compute the ready set. With it, the dispatch primitive is one call.

**How.** Parallels the existing `list` / `list all` grammar: bare verb for the minimal useful output, `all` for the full view. Supports `--format=json`, `--label <name>` (filter for label-specialized dispatch), `--tree <id>` (scope to subtree). `next` is read-only by definition — preview only, never mutates — so there's no `--peek` flag to add; `claim-next` is the mutating counterpart.

**JSON shape.** `next --format=json` returns a single object (or `null` if nothing is available). `next all --format=json` returns an array. This matches the natural "one vs. many" distinction between the two forms.

#### 8.4 `tail --until-close <id>` (blocking semantics)

**What.** `tail --until-close <id>` blocks until the target task closes, then exits. Multiple `--until-close` flags wait for all (conjunction). Timeout via `--timeout <duration>`.

**Why.** The orchestration primitive for "await this milestone before proceeding." Without a blocking form, orchestrators poll or maintain their own state machines.

**How.** Returns exit code 0 on close, nonzero on timeout or cancel. `--until-any-close <id1,id2,...>` could later add disjunction if needed; not required initially.

No separate `wait-for` verb — `tail --until-close` composes cleanly and doesn't grow the surface.

#### 8.5 Heartbeats and shorter default TTL

**What.** Lower the default claim TTL to five minutes. Add `job heartbeat <id>` that refreshes the claim without other state change. Subagents call it periodically while working.

**Why.** 1h TTL is too long for orchestration — a crashed subagent blocks its task for an hour before auto-release. Five minutes with explicit heartbeat lets the orchestrator detect stuck agents quickly (no heartbeat for 5m → claim expires → tail emits `expire` → orchestrator re-dispatches).

**How.** Heartbeat emits a distinct event (subscribed callers can choose whether to surface it). TTL is per-claim; `job claim <id> --ttl <duration>` allows per-call override for long-running tasks that shouldn't need heartbeating.

#### 8.6 Resumability: `log --since <timestamp>`

**What.** `job log --since <iso8601>` returns events since the given timestamp.

**Why.** If an orchestrator crashes and restarts, it needs to catch up on events it missed. Full re-reading of the tree is wasteful; replay from a checkpoint is efficient.

**How.** `log` with `--since` filters by event timestamp. Combined with `--format=json`, this is the catch-up primitive. Orchestrators persist their last-seen timestamp somewhere outside `.jobs.db` (their own state file) and resume from it.

### 9. Labels **[Nice-to-have, Additive]**

**What.** Free-form tags per task. Settable on creation (in YAML import), mutable post-hoc, queryable.

**Why.** When tasks are heterogeneous (different areas of the code, different skill requirements), labels let subagents dispatch to the slice they're built for. Not urgent for phase-based workflows, but natural once agents specialize.

**How.**

- In YAML: `labels: [tdd, red-tests]`
- Post-hoc: `job label <id> --add <name>` / `job label <id> --remove <name>`
- Filter: `job next all --label red-tests`, `job list --label red-tests`
- **Labels are local to each task.** No inheritance from parent; set explicitly on children if the label should apply there. Simpler mental model than inheritance rules.

### 10. Error and observability quality **[Valuable, Additive]**

**What.** Error messages include suggested next commands where applicable.

**Why.** An informative error is half of self-correction for LLM callers. "X is wrong" is much less useful than "X is wrong because Y; try Z."

**How.**

- `done` on a parent with open children: already lists the open children; add a trailing `(run 'job done --cascade <id>' to close all)`.
- `claim` on an already-claimed task: `Claimed by alice (expires in 12m). Run 'release' if held by you, or wait.`
- `import` validation errors: YAML-path locators as described in §2.
- `edit` on a nonexistent ID: suggest close matches by Levenshtein if any.

### 11. Orientation helpers **[Valuable, Additive]**

**What.**

- **`job status`** — one-line summary: `3 open, 1 claimed by you, 24 done (last session: 4h ago)`. Suitable for showing users progress without dumping the whole tree.
- **`job list` empty state** — when no actionable tasks remain, emit `Nothing actionable. 26 tasks done. Run 'list all' to see the full tree.` rather than silent empty output.

**Why.** Silent empty output leaves the caller uncertain whether the tool worked, the db is missing, or work is genuinely complete. Informative empty states resolve the ambiguity in one call.

### 12. Project-root detection **[Nice-to-have, Additive]**

**What.** `job` commands walk up the directory tree looking for `.jobs.db`, like `git` does for `.git`.

**Why.** Running `job` from a subdirectory of a project currently requires `cd`'ing back or passing `--db <path>`. LLMs working in nested source trees (`Sources/HayesCore/Memory/`) would invoke `job` from wherever they happen to be; the tool should find the db automatically.

**How.** Same pattern as git: start at cwd, walk up until a `.jobs.db` is found or the filesystem root is hit. Explicit `--db <path>` still overrides.

### 13. `job init` gitignore hints **[Nice-to-have, Additive]**

**What.** `job init` emits recommended `.gitignore` entries at the end of initialization. Optional `--gitignore` flag writes the entries to `.gitignore` directly.

**Why.** SQLite WAL-mode creates `.jobs.db-shm` and `.jobs.db-wal` sidecar files that are always local and shouldn't be committed. Users discover this the hard way otherwise.

**How.**

    $ job init
    Initialized .jobs.db.

    Recommended .gitignore entries:
      .jobs.db-shm       # SQLite WAL index (always local)
      .jobs.db-wal       # SQLite WAL journal (always local)

    To also keep the tracker local (don't check in the tree):
      .jobs.db

    Or run: job init --gitignore  to write these for you.

### 14. Revised verb list and help text **[Valuable, Additive]**

**What.** Rework the default help output (`job`, `job help`, `job --help`) so that a caller with only `"Use job to manage all tasks"` as preamble can operate the tool without additional prompting. Group verbs by role rather than alphabetically, teach the workflow loop inline, and surface identity semantics upfront.

**Why.** The current help output is a flat alphabetical verb list with one-line descriptions. It answers "what verbs exist?" but not "how do I use this?" — and an LLM encountering the tool for the first time won't compose the verbs correctly without additional guidance. Making the help text self-contained eliminates a class of prompting burden on tool integrators.

**How.**

#### 14.1 Full verb list as envisioned

Grouped by role. **(new)** / **(changed)** / **(removed)** / unmarked = unchanged semantics.

**Setup (unattributed):**
- `init` — Initialize a new job database
- `schema` — Print the JSON Schema for the import YAML format **(new)**

**Planning (writes):**
- `add` — Add a new task
- `import` — Ingest a YAML task tree from a Markdown plan doc **(new)**
- `edit` — Change a task's title and/or description **(changed: adds `--desc`)**
- `label` — Add or remove labels on a task **(new)**
- `block` — Mark a task as blocked on another
- `unblock` — Remove a blocking relationship
- `move` — Re-parent or reorder a task among its siblings

**Execution (writes):**
- `claim` — Claim a task for yourself
- `claim-next` — Find and claim the next available leaf **(changed: leaf-preferring)**
- `heartbeat` — Refresh a claim's TTL without other state change **(new)**
- `release` — Release a claimed task
- `note` — Append a note to a task; optional structured `--result` **(changed: adds `--result`, stdin)**
- `done` — Close a task; optional inline `-m "<note>"` and `--result '<json>'` **(changed: adds `-m`, `--result`, variadic, `--cascade`)**
- `reopen` — Reopen a closed task **(changed: adds `--cascade`)**
- `cancel` — Mark a task as canceled without destroying history; propagates via `tail`. `--purge` erases destructively (replaces the old `remove`). `--cascade` propagates to subtree. **(new)**

**Observation (no identity required):**
- `list` — List tasks (actionable by default; `list all` for everything)
- `info` — Show full details of a task
- `log` — Show event history for a task and its descendants **(changed: adds `--since`)**
- `status` — One-line summary of tree state **(new)**
- `next` — Preview the next available leaf (read-only); `next all` for the parallel frontier **(changed: adds `all`)**
- `tail` — Stream events in real-time; `tail --until-close <id>` blocks until target closes **(changed: `--format=json`, `--until-close`, event filters, heartbeats filtered by default)**

**Utility:**
- `help` — Help about any command
- `completion` — Generate shell autocompletion script

**Removed:**
- `login` — gone (identity resolved per-command via `--as`; see §1)
- `logout` — gone (no session state to clear)
- `remove` — gone (replaced by `cancel --purge --reason "..."`; see §8.1)

#### 14.2 Proposed help text for `job` / `job help`

Replacement for the current default output. The goal: an LLM reading this alone can write correct imports, claim and close tasks, and observe state without additional prompting.

```
job — a lightweight task tracker for multi-phase, multi-agent work.

Use job for any task with more than a few steps, work that benefits from
a durable audit trail, or work that may involve multiple agents
coordinating. For ad-hoc one-off todos, built-in session notes are fine;
use job when persistence, attribution, or coordination matter.

QUICKSTART

  1. Plan in a Markdown doc ending with a YAML code fence:

       ```yaml
       tasks:
         - title: Root task
           children:
             - title: First subtask
             - title: Second subtask
       ```

  2. Import:  job import plan.md
     (Use --dry-run first if you want to preview without creating.)

  3. Work:    job --as claude claim-next
              job --as claude done <id> -m "notes on what was done"

  4. Observe: job list         (actionable tasks)
              job status       (one-line summary)
              job log <id>     (history of a task and its subtree)

IDENTITY

  Writes require --as <name>. Reads (list, info, log, status, next,
  tail, schema) work without it.

    job --as alice claim 87TNz     # explicit identity per write

  For "set once, forget" ergonomics, shell-alias it:
    alias job='job --as alice'     # in .zshrc, .bashrc, etc.

  Identity is free-form. Pick a stable name per agent or user; if two
  agents use the same name they share attribution, so choose unique
  names in multi-agent workflows.

VERBS (grouped by role)

  Setup:        init, schema
  Planning:     add, import, edit, label, block, unblock, move
  Execution:    claim, claim-next, heartbeat, release, note, done,
                reopen, cancel
  Observation:  list, info, log, status, next, tail

  For full options on any verb:  job <verb> --help

OUTPUT

  Dense Markdown by default, token-efficient for both human and LLM
  readers. `--format=json` on any read verb for deterministic parsers
  or subscriber agents on live streams.

ORCHESTRATION

  For multi-agent workflows, see:
    job next all                       # parallel frontier (what can be
                                       #   dispatched right now)
    job tail --until-close <id>        # block until <id> closes
    job --as <name> cancel <id>        # non-destructively stop work
    job --as <name> cancel <id> --purge --reason "..."
                                       # destructive erase (rare)
    job --as <name> heartbeat <id>     # refresh a long-running claim

  Default claim TTL is 5 minutes; call heartbeat periodically on tasks
  that take longer.
```

This output fits in ~70 lines. It teaches the workflow in the first two sections (Quickstart, Identity), references the verb groups, surfaces the output format, and gates orchestration primitives behind an explicit "for multi-agent workflows" section so solo callers aren't distracted by them.

**Why this much detail in `job` alone?** Because the stated goal is for a tool integrator to drop a single line of preamble (`"Use \`job\` to manage all tasks"`) into a system prompt and have the LLM figure out the rest from the tool's self-description. Every line of context the help text carries is a line the integrator doesn't have to write. For an LLM-facing CLI, the help text is the tutorial.

**What `job help <verb>` provides.** The subcommand help stays short and specific — full flag list, one example. The top-level help is the workflow teacher; the subcommand help is the reference.

---

## Implementation priority suggestion

If ordering the work: §1 (identity) unblocks the single biggest friction. §2 (Markdown import) makes planning cheap. §4 (enriched acks) and §5 (atomic close-with-note) are near-free wins in the success-path of common verbs. §8 (orchestration) is where the tool's strategic ceiling lifts — that's the work that transitions `job` from "solo tracker" to "multi-agent coordination substrate." Everything else is polish.

A realistic first release could ship §1, §2, §4, §5, §6, §11, §14 without the orchestration set and already feel dramatically better — §14 (revised help text) is particularly cheap relative to the onboarding value it delivers. The orchestration set (§8) is a second, thematic release with its own story.

---

## A closing observation

Most of the friction I experienced wasn't `job` being broken — it was `job` being a good human-facing CLI that happened to be used by a machine operator with a different cost function. The recurring pattern: humans absorb small per-call ceremonies trivially (type a prefix, eval a login, parse a tag at the end of a line). Machines don't; every small cost gets multiplied by the number of calls and the number of sessions.

The changes I'm proposing mostly don't subtract from human ergonomics — they add opt-in paths (structured output, bulk operations, Markdown ingest, enriched acks) while preserving the existing single-op-at-a-time shell-friendly surface. The one exception is the identity model, which I'd argue the keys were over-engineered to solve for anyone.

The largest latent value in `job` is its shape: a tree, with claims, with expiry, with event streams. That shape is already an orchestration substrate. A handful of event-semantics additions would let multi-agent systems use it as such. For LLM infrastructure specifically, that's the big unlock — and the thing I'd be most excited to see in a subsequent release.
