# Implementation plan — Opus 4.7 feedback (2026-04-20)

Source: [`2026-04-20-opus-feedback.md`](./2026-04-20-opus-feedback.md). This plan turns that feedback into seven phases, sized so each fits comfortably inside a single ~200k-token working session (not counting subagents).

## Principles applied to this plan

- **Faithful to the spec, with named deviations.** The feedback is treated as a draft spec. Deviations are called out in §Deviations.
- **Red/green TDD per CLAUDE.md.** Every phase begins by writing failing tests for the new behavior and confirming they all fail, then implements.
- **Build mode.** No deprecation windows, no backward-compat shims. Breaking changes land cleanly.
- **Keep docs synced.** README and any help-text output are updated in the same phase as the code change, per project CLAUDE.md.
- **One phase ≈ one PR.** Commits within a phase are fine-grained (per CLAUDE.md: commit after significant accepted work); the phase boundary is the natural PR boundary.

## Deviations from the spec

1. **Default TTL: 15 minutes, not 5** (§8.5). Agreed per clarification — 5m is too aggressive for humans and slow-thinking agents. Heartbeat and per-claim `--ttl` still available.
2. **Checkbox rendering: `[ ]` and `[x]` only** (§3). The spec proposes `[-]` for in-progress, but GFM renders only `[ ]`/`[x]`; `[-]` would appear as literal text in GitHub/Notion/docs viewers, defeating the paste-as-rendered-checklist win that motivates §3. Claim state moves into the trailing parenthetical alongside the other non-binary signals:
    ```
    - [ ] `87TNz` Phase 1 — Data model
      - [x] `s1Ut5` Write red tests for Lesson
      - [ ] `9aedB` Implement Lesson struct (claimed by claude, 12m left)
      - [ ] `2F1C1` Collapse AnalysisResult (blocked on 9aedB)
    ```
    Terminal output retains color on claimed lines for at-a-glance state; the data is just canonically carried in the parenthetical so Markdown rendering stays honest.

## Settled decisions

- **No migration system** (user confirmed 2026-04-20: "pre-launch"). Schema evolves via idempotent `CREATE TABLE IF NOT EXISTS` / additive column defaults in `initSchema`. The stale reference in CLAUDE.md to `internal/migrations/` should be removed in Phase 1's README/docs sweep (note: user CLAUDE.md is separate and outside this repo).

## Open questions

1. **`ref` persistence.** The YAML import's `ref` field (§2) is used only for `blockedBy` resolution within a single import call. Per spec it's an author-chosen handle, not a persistent identifier — the real 5-char short ID replaces it at ingest. Plan assumes refs are **not stored** on tasks after import; flag if you want them persisted (e.g., for re-import-as-update workflows).
2. **`result` storage.** Per §5, `done --result '<json>'` is orthogonal to notes and surfaces in `log --format=json`. Plan stores the JSON blob **in the `done` event's `detail` field**, not as a column on `tasks`. Rollup views (a parent composing children's results) can query events rather than tasks. If you prefer a `result` column for O(1) lookup, flag it.

## Session sizing assumptions

- A phase touches 3–6 files, writes 400–900 lines of code + tests, runs the full test suite ≥2 times.
- Context budget per session: ~200k tokens. System prompt, tool defs, CLAUDE.md, and memory load consume 25–35k before any work. A phase should leave ≥80k free at peak for iterative edits + test output.
- If a phase approaches its budget mid-session, commit the green-tested slice and continue in a follow-on session under the same phase header.

---

## Phase 1 — Identity overhaul (§1) — **Critical, Breaking**

**Goal.** Replace `login`/`logout`/`JOBS_USER`/`JOBS_KEY`/key-in-db with a `--as <name>` global flag. No silent defaults, no env fallback, no keys.

**Why first.** This is the single biggest friction in the feedback. It also unblocks clean attribution in every subsequent phase — every new write verb we add in Phases 2–7 reads identity the same way.

**Scope.**
- Remove `newLoginCmd`, `newLogoutCmd`, `JOBS_USER`/`JOBS_KEY` env lookups, and the `users.key` column.
- Keep the `users` table (identity is still a string); populate it lazily on first write by any new name.
- Add persistent root flag `--as <name>` (position before the verb, like `git -C`).
- Gate writes (`add`, `edit`, `move`, `note`, `done`, `reopen`, `remove`, `claim`, `claim-next`, `release`, `block`, `unblock`) on presence of `--as`. Reads (`list`, `info`, `log`, `tail`, `next`) require no identity.
- `init` is unattributed.
- Stolen-claim error surfacing: when write's caller's prior claim expired and another holder took it, direct the caller to `claim <id>` to reclaim. (See §1 for exact wording.)
- Claim-conflict error directs the caller to either `release` (if they hold it) or to wait.

**Tests (red first).**
- Every write command errors with `Error: identity required. Pass --as <name> before the verb.` when `--as` is absent.
- Every read command works without `--as`.
- `--as alice add "x"` attributes the `add` event to `alice` and auto-creates user `alice` if missing.
- Stolen-claim surfacing: alice claims X, claim expires, bob claims X; alice's next `done X --as alice` errors with the reclaim hint.
- Claim-conflict: bob's `done X --as bob` while alice holds it errors with the release/wait hint.
- No env-var read of `JOBS_USER`/`JOBS_KEY`; setting them has no effect.

**Docs.**
- Rewrite README identity section: drop `eval $(job login)`, show `--as` usage and shell-alias pattern.
- Remove "Identity" command table rows for `login`/`logout`.

**Files touched.** `commands.go`, `users.go`, `users_test.go`, `database.go` (drop `key` column from schema), `README.md`, relevant tests in `database_test.go`.

### Phase 1 implementation notes (landed 2026-04-20)

Deltas from the pre-flight plan; read these before Phase 2 to avoid re-litigating settled calls.

- **`checkClaimOwnership(tx, shortID, caller)` signature.** Plan wrote it as `(tx, taskID, caller)`; callers have the short ID at hand, so the implementation takes `shortID` and looks up the task once inside. Lives in `claims.go` alongside `expireStaleClaimsInTx`.
- **Every guarded write also calls `expireStaleClaimsInTx(tx, actor)` immediately before `checkClaimOwnership`.** Without the pre-expire pass, a caller whose own claim just aged out would see a "claimed by you / conflict" state that's not reflective of reality. Reuse this pattern for any new write verb (heartbeat, cancel, etc.) added later.
- **`runRelease` does its own ownership check, not `checkClaimOwnership`.** The release-wrong-holder wording is unique to `release` and couldn't be produced by the shared helper without adding a verb parameter. Kept the helper generic; release has a ~5-line inline block.
- **`runClaim`'s conflict message** uses the same wording as `checkClaimOwnership`'s non-stolen branch (`claimed by %s (expires in %s). Wait for expiry, or ask %s to release.`), for a single UX voice. Self-claim on an existing hold returns `task %s is already claimed by you`, which the plan didn't specify but is the obvious right answer.
- **Reads pass the literal string `""` for actor.** `runList`, `runNext`, `runLog`, `runTail` kept their `actor string` parameter — not renamed, not dropped — but every command handler passes `""`. `runClaimNext` is a write; it passes the real actor. Any future read side effect recording an event should keep passing `""`.
- **`TestRunClaim_AlreadyClaimed` was updated.** The old assertion checked for `"already claimed"`; new wording is `"claimed by"`. This is the only pre-existing test whose assertion had to change — everything else in `database_test.go` operates on run-functions directly with a literal `testActor` and is identity-agnostic.
- **CLI-level tests live in a new `commands_test.go`.** Uses helpers `setupCLI`/`runCLI`/`resetFlags` to reset the package-level `dbPath` / `asFlag` globals between invocations. Future phases should extend this file for their verb-level tests rather than adding more globals — the pattern is already load-bearing for 11 tests.
- **Sandbox note for the next agent.** `go build`/`go test` need `dangerouslyDisableSandbox` in the current harness because the Go toolchain writes to `/tmp/claude-501/go-build*` which falls outside the sandbox allowlist. Tests themselves are fine once the toolchain has run.
- **Adjectives/animals tables deleted.** They only existed to seed random login names. If any future verb needs a friendly-name generator, regenerate from a fresh source rather than restoring them.
- **`users` table now has columns `(id, name, created_at)` only.** `users.name` carries a `UNIQUE` constraint; `ensureUser` relies on it via a SELECT-then-INSERT pattern (not `INSERT ... ON CONFLICT`). Concurrent first-writes by the same new name could race on this; we accepted that because the observable outcome either way is "one row for that name."

---

## Phase 2 — Markdown+YAML import, `schema`, project-root detection (§2 + §12) — **Critical, Additive** + **Nice-to-have, Additive**

**Goal.** `job import <file.md>` ingests a YAML subtree atomically. `job schema` emits the JSON Schema for the import format. `job` walks up the directory tree to find `.jobs.db` like `git` does for `.git`.

**Why bundled.** All three are new top-level features that don't overlap with Phase 1's surface. Root detection is tiny; schema emission is a ~50-line helper; the bulk of the phase is import.

**Scope.**
- New file `import.go`.
- Dependency: YAML parser. Propose `gopkg.in/yaml.v3` (single dep; de facto standard; already allowed in Go projects — ask user before adding per CLAUDE.md).
- Parse Markdown, locate first fenced `yaml` (or unlabeled) block whose root key is `tasks:`. Ignore other YAML blocks.
- Validate YAML:
  - `title` required per task.
  - `desc`, `labels`, `children`, `ref`, `blockedBy` optional.
  - `ref` namespace is flat across the import (cross-subtree refs work).
  - `blockedBy` resolves first as ref, then as verbatim title. Unresolved or ambiguous → atomic error citing the YAML path (e.g., `tasks[2].children[0].blockedBy[1]`).
- Flags: `--dry-run` (preview with `<new-1>`, `<new-2>` placeholders), `--parent <id>` (insert under existing task), `--format=json`.
- Echo the created subtree in input order. Atomic: on any validation failure, no tasks created, no events fired.
- `job schema` emits JSON Schema for the import format to stdout. No identity required.
- Project-root detection: `resolveDBPath` walks up from cwd looking for `.jobs.db` (symmetric to `.git` discovery). `--db` and `JOBS_DB` still override. **Note:** in Phase 1 we kept `JOBS_DB`; identity is the only env we removed. Document clearly.

**Tests (red first).**
- Import valid YAML tree with `desc`, `labels`, nested `children` → creates expected tasks with expected attribution (via `--as`).
- `ref` + `blockedBy` forward and backward across subtrees resolve correctly.
- `blockedBy` by verbatim title resolves.
- Ambiguous title in `blockedBy` → error with YAML-path locator, no tasks created.
- Unresolved ref → error with YAML-path locator, no tasks created.
- Missing `title` → error.
- `--dry-run` emits placeholders, creates nothing; follow-up real run generates distinct IDs.
- `--parent <id>` inserts under existing task.
- Multiple YAML blocks in doc: only the `tasks:`-rooted one is used.
- `job schema` output parses as valid JSON and matches our import grammar.
- Project-root: running from a subdirectory finds the db above; `--db` overrides.

**Docs.**
- README: new "Planning" section with a worked example showing a Markdown doc with a trailing YAML block + `job import` invocation.
- Top-level help references `job import` in the Quickstart (which will be fully rewritten in Phase 3).

**Files touched.** New `import.go`, `schema.go`; `commands.go` (register verbs, add `--db`/root-walk), `database.go` (`resolveDBPath`), `README.md`, new tests.

### Phase 2 implementation notes (landed 2026-04-20)

Deltas from the pre-flight plan. Read before Phase 3 so you don't "fix" deliberate calls.

- **`resolveDBPathForInit` is a second resolver alongside `resolveDBPath`.** The plan (test #24) required that `job init` never walk up. Rather than gate-walking inside `resolveDBPath`, I added a sibling function with the old precedence (flag > env > literal cwd). `newInitCmd` calls it; all other commands call `resolveDBPath`. Any future init-like verb (e.g., Phase 3's `--gitignore` flow) should reuse `resolveDBPathForInit` if it means to operate on cwd specifically.

- **`JOBS_DB=""` does NOT skip the ancestor walk.** `os.Getenv` returns empty for unset *or* explicitly-empty; both cases fall through to the walk. This matters for tests that want to force cwd-local resolution: set `JOBS_DB` to a specific path, don't clear it. The `TestResolveDBPath_NoAncestor_FallsBackToCwd` / `TestResolveDBPath_WalksUp` tests use `t.Setenv("JOBS_DB", "")` and `t.Chdir(...)` to dodge this; they work because they chdir into a freshly-minted tempdir with no ancestor db.

- **Unknown YAML keys in task entries are silently accepted, not rejected.** The JSON Schema declares `additionalProperties: false` as *documentation* but the parser tolerates unknowns. Rationale: keeps forward-compat painless (e.g., a future `assignee:` field on a plan file doesn't error out on older binaries). If strict-schema enforcement is wanted later, add a JSON Schema validator library and gate it behind a flag. Phase 7 may revisit during its error-polish pass.

- **Import-created blocks emit a `blocked` event per entry.** Plan spec said "insert rows into `blocks`"; I also called `recordEvent(..., "blocked", ...)` to mirror `runBlock`. Consequence: `job log` on imported tasks shows the block relationships as first-class events, and downstream event consumers don't need a special case for "implicit" blocks. The event's `detail` uses the same `{"blocked_id", "blocker_id"}` shape as `runBlock`.

- **`title: ""` is rejected, but so is a missing title.** Both error with `%s: title is required`. Done via a custom `UnmarshalYAML` on `rawTask` that tracks which keys appeared. If Phase 4's `edit --desc` imports analogous code, consider factoring the key-presence pattern into a shared helper.

- **YAML parse errors use `YAML parse error: %s`, not the plan's `YAML parse error at %s: %s`.** yaml.v3's own error strings embed `line N:` already, so the wrapper is the identity transform. No test asserts the full wording; update the format string if a test needs it.

- **Pointer-identity keys in `blockedByResolved map[*parsedTask][]resolved`.** Works because `buildParsedTree` allocates exactly one `*parsedTask` per raw entry. If a future refactor interns or deduplicates parsed tasks (e.g., to detect duplicate subtrees), switch the key type to `flatIndex int` before the change lands.

- **Fence detection is line-based, not a full Markdown parser.** Regex `^(` + "```" + `|~~~)([a-zA-Z0-9_+-]*)\s*$` matches the opener. This means code blocks indented under a list item (`    ` + fence) aren't recognized. Intentional — agents writing plans for `job import` should use top-level fences. If a real user hits this, the fix is to strip a leading, consistent indent before scanning; don't pull in a Markdown lib.

- **Schema uses `$ref: "#/properties/tasks/items"` for the recursive `children` shape.** Inlined rather than `$defs` because `TestSchema_DeclaresRequiredTitle` asserts `properties.tasks.items.required` directly; the `items` shape must be concrete at that path, not a ref. Draft 2020-12 resolves the self-ref fine, but schema-browser UIs may show it as "reference" rather than expanding it inline — acceptable.

- **`resolveDBPath` now walks up for all commands, including writes.** Plan language framed project-root detection as a read-ergonomics feature; implementation doesn't distinguish. A write invoked from a subfolder of a project targets the ancestor db, which matches `git`'s behavior. If a future verb should always operate on cwd (e.g., a hypothetical `job export`), use `resolveDBPathForInit` or add a third resolver.

- **CLI tests for this phase extend `commands_test.go` directly.** The file is now ~300 lines with helpers (`setupCLI`, `runCLI`, `resetFlags`) from Phase 1. Phase 3's help-text snapshot test and Phase 5's tail-streaming tests will both grow this file; if it crosses ~600 lines, split by verb-family (`import_cli_test.go`, `tail_cli_test.go`) rather than introducing new globals.

- **`TestInit_StillUsesCwd_EvenUnderAncestor` builds its own cobra harness** instead of using `runCLI`. It needs `t.Chdir()` into a descendant directory *before* the command runs, which `runCLI`/`setupCLI` don't support. If more tests need this pattern (Phase 3's `job init --gitignore` likely will), factor a `setupCLIInDir(t, dir)` helper.

- **`generateShortID` is called inside the import transaction per-task.** Low collision risk at current volumes; the function already has a retry loop. No pre-allocation pass.

- **Sandbox note carried over from Phase 1.** `go get`, `go build`, `go test` still need `dangerouslyDisableSandbox: true` because the Go toolchain writes outside the allowlist (`/tmp/claude-501/go-build*`, `~/Library/Application Support/go/telemetry/...`). Tests themselves run fine once the toolchain has touched disk.

---

## Phase 3 — Rendering, acks, status, help text, gitignore (§3, §4, §11, §13, §14) — **Valuable, Additive**

**Goal.** Reshape everything the user/agent *reads* when the tool runs: list rendering, success acks, empty states, init output, top-level help.

**Why bundled.** These are all output-layer changes concentrated in `format.go` and a thin slice of `commands.go`. Each is small; together they're a coherent "the tool's voice" pass. A single reviewer can assess them against each other.

**Scope.**
- **§3 Checkbox rendering.** `list`/`list all` output switches to `- [ ]` / `- [x]` with backtick-wrapped IDs. Claim state, expiry, and blocked-on info go in a trailing parenthetical. Terminal color retained for at-a-glance state (claimed lines colorized; no `[-]` marker).
- **§4 Enriched acks.** `done` output includes the next claimable sibling, parent progress, and escalation when a parent becomes closeable. Wording per feedback §4. Skip-blocked case handled.
- **§11 Status + empty state.** New `job status` verb — one-line tree summary. `list` returns a helpful "nothing actionable" message when the actionable set is empty.
- **§13 Gitignore hints.** `job init` prints recommended `.gitignore` entries. `job init --gitignore` writes them for the user (appends if `.gitignore` exists; creates otherwise; idempotent against already-present lines).
- **§14 Help text.** Rewrite `job` / `job help` default output per the feedback's full proposed block. Group verbs by role (Setup / Planning / Execution / Observation / Utility). Teach the workflow in a Quickstart. Call out identity, output format, and orchestration primitives inline. Remove references to `login`/`logout`/`remove`. Add references to `import`, `schema`, `cancel`, `heartbeat`, `status`, `next all`, `tail --until-close` even though several of those land in later phases — the help text is a contract with the LLM integrator, and we ship it once; later phases wire up the verbs the help text already mentions. Gate the help text to reference only verbs that exist in the current build; otherwise split into Phase 3a (ack/render/status/help-stub) and Phase 3b (full help once all verbs exist). **Recommend** keeping the full help text in Phase 3 but adding a brief "(coming in next release)" caveat next to orchestration verbs that land later — the integration-time value of the help text is too high to gate.

**Tests (red first).**
- `list` rendering emits `- [ ]`/`- [x]` with backtick IDs and parenthetical state.
- `done` of a mid-phase leaf prints the next sibling + parent progress.
- `done` of a last child prints the parent-close escalation + next phase.
- `done` of the whole tree's last task prints the all-complete ack.
- `job status` outputs the expected one-liner for a tree with mixed states.
- Empty-tree `list` emits the informative empty-state string.
- `job init` stdout includes the gitignore hints block.
- `job init --gitignore` creates/updates `.gitignore` idempotently.
- Snapshot test on `job help` output (to lock the help text).

**Docs.**
- README: update the "Viewing tasks" section with new checkbox output.
- README: add `status` row.
- README: update `init` section with gitignore note.

**Files touched.** `format.go`, `commands.go` (done, init, list, new status cmd, root Long), `README.md`, tests.

### Phase 3 implementation notes (landed 2026-04-20)

Deltas from the pre-flight plan. Read before Phase 4 so you don't "fix" deliberate calls.

- **No terminal color, ever.** The pre-flight scope line still said "Terminal color retained for at-a-glance state (claimed lines colorized; no `[-]` marker)." but the Phase 3 plan (its own "Explicit non-goals" section) explicitly reversed that decision on 2026-04-20: ANSI codes defeat the paste-as-GFM-checklist win and corrupt LLM tool-capture. Implementation matches the Phase 3 plan, not the earlier scope line. Don't re-add color in Phase 4 without re-litigating.

- **Checkbox output uses one space between `]` and the backtick ID, not two.** The Phase 3 plan's prose said "Two spaces between checkbox and backtick ID (standard GFM)" but the example it gave used one space (`- [ ] \`id\` title`), and standard GFM is one space. Tests lock the one-space form.

- **`remove` is still a registered verb.** The Phase 3 plan claimed "remove is already gone from the registered command set (Phase 5's job, not ours); help text has no entry for it." The first half was wrong — `newRemoveCmd` is still wired in `newRootCmd`. The help text intentionally omits it. `TestHelp_MentionsCurrentVerbs` lists the set it asserts rather than reflecting over registered commands, so the mismatch doesn't fail. Phase 5 will actually delete the verb per its own plan.

- **`DoneContext` lives in `tasks.go`, not a separate `donecontext.go`.** Plan offered both; kept alongside `runDone` for locality. The helpers `getTaskByID`, `getChildren`, `getRootTasks`, `findNextSibling`, `findTopAncestor`, `subtreeCompleteness` are all in `tasks.go` too. If Phase 4's `--cascade` / variadic `done` needs to aggregate contexts per ID, that's the file to extend — the helpers are already DB-level and reusable.

- **`NextAfterParent` must consider root-level siblings too.** My first pass only looked up `parent.ParentID` (grandparent's children), which produced no "Then:" line when the closed task's parent was itself a root. Fixed with `getRootTasks(db)` when `parent.ParentID == nil`. Test `TestDone_EnrichedAck_LastChild_WithParentSibling` locks this.

- **`computeDoneContext` runs AFTER `runDone` commits, as a read-only query.** That's intentional — the context computation is independent of the write transaction, and sees the just-closed task in its final state. It also means if `computeDoneContext` fails, the close already succeeded and we return the error; we do *not* roll back. Downstream (Phase 4 variadic `done`) should decide whether to compute per-ID or aggregate; I didn't paint into either corner.

- **`ParentWasDone` guard, added per the Plan's "Risks" section.** If the closed task's parent was already done (possible via `--force` closing a mixed subtree), we skip the "Parent X: N of M complete" line to avoid lying. Test-wise, this path isn't directly exercised — the WholeTreeComplete branch usually catches it first — but the guard is cheap insurance.

- **Status "open" count means `status = 'available'` only — claimed tasks are counted separately.** The plan's example `3 open, 1 claimed by you, 24 done` is ambiguous about whether "open" includes claimed. I went with *exclusive*: open = available, claimed = claimed, done = done. `ClaimedByYou` is a subset of claimed, rendered only when `--as` is set AND the caller holds at least one claim. `TestStatus_Counts` locks this.

- **`renderStatus` uses `nowUnix()` so tests can drive it via `currentNowFunc`.** Matches the existing Phase 1 pattern (`formatDuration` helpers all route through `currentNowFunc`). `TestStatus_LastActivityPhrase` validates.

- **`writeGitignoreEntries` always writes a `# job` section header before appended entries, even when `.gitignore` was empty/absent.** Makes the file's provenance obvious to a reader. Idempotency is by-line, not by-section: re-running with both entries present produces no change and no duplicate header. Test `TestInit_GitignoreFlag_Idempotent`.

- **The `--gitignore` flag does NOT write `.jobs.db` itself.** Checking in vs. committing the tracker is a user preference. The printed hint mentions the option; the flag only writes the two WAL sidecar entries. Matches the Phase 3 plan verbatim.

- **`TestHelp_Snapshot` is an anchor-phrase test, not a true byte-for-byte snapshot.** The plan mentioned a "golden string" but asserting full exact output against cobra's help template is brittle (cobra adds flag listings, use lines, etc.). I asserted the presence of `QUICKSTART`, `IDENTITY`, `VERBS (grouped by role)`, `OUTPUT`, `ORCHESTRATION`, plus three content anchors (`job import plan.md`, `claim-next`, `--format=json`). Phase 4 should extend this list if it adds new help sections.

- **Phase-gated verb annotations use the string `(in next release)`.** Tests lock that exact phrase for `label`, `heartbeat`, `cancel`, plus the `next all` and `tail --until-close` subcommand forms. Phase 5/6/7 should remove the annotation from their respective verbs when they land — `TestHelp_PhaseGatedVerbsAnnotated` will then need those lines removed too.

- **Empty-list message for `list all` with zero rows uses the fresh-db wording** (`No tasks. Run 'job import plan.md' ...`). The "Nothing actionable. N done." branch only fires when `total > 0 && filtered == 0`. This matches the plan's "list all with zero rows is rare (means db is empty)" note.

- **JSON output is unchanged on empty — still emits `null` (from `formatTaskNodesJSON` when nodes is nil). Empty-state prose applies to Markdown only.** Spec-compliant; machine readers get the same shape they got before.

- **`filepath` import added to `commands.go` for the `--gitignore` flow.** No new file needed; `writeGitignoreEntries` lives in `gitignore.go` but is called from `newInitCmd`.

- **Test file organization.** Added `format_test.go` (~150 lines) and `status_test.go` (~150 lines) as fresh files; the Phase 2 note predicted `commands_test.go` would be split if it crossed ~600 lines, but I extended it to ~700 and left it there because the new tests are all CLI-harness-driven and reuse `setupCLI`/`runCLI`. Phase 4's enriched-done multi-ID tests will push this past 800; that's probably the right moment to split.

- **Sandbox note (carried forward).** `go build`, `go test`, and the `job` binary executing from `/tmp/claude/...` still require `dangerouslyDisableSandbox: true`. Tests themselves run fine once the toolchain has touched disk.

---

## Phase 4 — Mutation ergonomics (§5, §6, §7) — **Valuable, Additive**

**Goal.** `done`/`note` accept inline notes and structured results. `done`/`reopen` accept multiple IDs and a `--cascade` flag. `edit` mutates descriptions.

**Why bundled.** These all touch the mutation verbs (`done`, `note`, `reopen`, `edit`) and share a "common cascade walker" implementation. Bundling avoids two reviewers needing to think about overlapping changes in consecutive PRs.

**Scope.**
- **§5 Close-with-note + result.**
  - `done <id> -m "<note>"` atomically records the note and closes.
  - `done <id> --result '<json>'` records a structured JSON blob in the `done` event's `detail`.
  - `note <id> -m "<text>" --result '<json>'` writes an intermediate checkpoint without closing.
  - Stdin support: `... | job note <id> -` reads the note body from stdin.
  - Remove the old positional `done <id> [note]` / `note <id> <text>` forms in favor of `-m`. (Breaking.)
- **§6 Variadic + cascade.**
  - `done id1 id2 id3` closes all in one call, all-or-nothing atomic.
  - `done --cascade <id>` closes the target plus all open descendants.
  - `reopen --cascade <id>` symmetric.
  - `--format=json` emits a per-ID result array.
  - The old `done --force` flag is retired; `--cascade` supersedes it.
- **§7 Edit desc.**
  - `edit <id> --title <t> --desc <d>` — either or both; replace semantics.
  - Remove the positional title form in favor of `--title`. (Breaking, in line with §6's variadic refactor.)

**Tests (red first).**
- `done <id> -m "n"` writes both the note event and the done event atomically; query confirms both exist.
- `done <id> --result '{"k":1}'` writes JSON to the done event's detail; `log --format=json` surfaces it.
- `note <id> -m "n" --result '{}'` is non-closing.
- `note <id> -` reads from stdin.
- `done id1 id2 id3` — all succeed; if any fails pre-flight validation, none are closed.
- `done --cascade <parent>` closes all open descendants + the parent.
- `reopen --cascade <parent>` reverses it.
- `edit <id> --desc "new"` replaces description (doesn't append like `note`).
- `edit <id> --title a --desc b` updates both.

**Docs.**
- README: update "Completing tasks", "Editing tasks" sections.
- Help text footnotes: remove `--force` references.

**Files touched.** `commands.go` (done, note, reopen, edit, remove), `tasks.go`, `format.go` (JSON shape for multi-ID results), tests.

### Phase 4 implementation notes (landed 2026-04-20)

Deltas from the Phase 4 design doc (`project/2026-04-20-phase-4-plan.md`, if split out, or this block). Read before Phase 5 so the cascade/event-shape decisions aren't re-litigated.

- **Event detail for cascaded descendants uses `cascade_closed_by_parent`, not `force_closed_by_parent`.** The design doc only nailed the explicit-target event shape (`{cascade: bool, cascade_closed: [...]}`) and punted on the descendant shape. I mirrored the legacy `force_closed_by_parent` naming into `cascade_closed_by_parent` for symmetry. Phase 5's cancel walker should use the same `cascade_closed_by_parent` key when it emits cascaded cancel events — one consistent naming across write verbs.

- **Cascaded descendants record `cascade: true` (not false) on their own `done` event.** An alternative reading is "only the explicit target records cascade=true; descendants just record force_closed_by_parent". I went with cascade=true on descendants too because it makes `formatEventDescription` render them as `done --cascade` in the log, which matches how a reader would mentally model what happened. If you care about distinguishing "I was the target" from "I was swept up", read `cascade_closed_by_parent` vs `cascade_closed`.

- **`runDone` no longer passes `note` to cascaded descendants.** Cascaded children never get the `-m`/`--result` values — those are target-only. Their `completion_note` column stays NULL and their event detail has no `note`/`result` key. The design doc's "Shared across all IDs in the call (one note, many closes)" applies only to explicit targets, not to cascade sweeps. Phase 5's cancel should probably also scope `--reason` to explicit targets only, with cascaded children referencing the parent event.

- **Transaction scope: one tx for the whole call, including Phase A validation.** I call `expireStaleClaimsInTx` + `checkClaimOwnership` + `getTaskByShortID` inside the transaction rather than pre-validating outside. This means a claim that expires *during* the variadic call is seen consistently. If Phase 5 adds `cancel --cascade`, reuse this pattern — Phase A validate + Phase B execute, all one `tx`.

- **Claim-ownership check runs per ID, not deduped.** If caller passes `done a a` both checks fire. Harmless (same answer both times) and the validation loop dedupes by task ID via `seenExplicit` *after* the ownership check. If future phases add any per-ID state mutation during validation, move dedup ahead.

- **`runDone` now accepts `json.RawMessage` for the result blob, not `string`.** The cobra layer validates JSON syntactically via `json.Valid` before handing off; `runDone` re-parses via `json.Unmarshal`. Two validations is a little wasteful but the CLI-layer check gives a better error message with the offending input visible, and the internal check survives a non-CLI caller. Same pattern in `runNote`.

- **`alreadyDone` is `[]string`, not `bool`.** Old API was `(forced []string, alreadyDone bool, err)`. New is `(closed []*ClosedResult, alreadyDone []string, err)` — the bool became a list so variadic calls can surface which specific IDs were already done. Single-ID idempotent callers check `len(alreadyDone) == 1 && len(closed) == 0`.

- **Single-ID non-cascade idempotent already-done preserves Phase 3's "Already done: <id>" wording via a special case in `newDoneCmd.RunE`.** When `len(closed)==0 && len(alreadyDone)==1 && len(args)==1` we print that line verbatim and skip the render path. Phase 5's locked-string tests rely on this; if `cancel` wants the same idempotency, copy the pattern.

- **`renderDoneAck` splits by shape, not by arg count.** The three branches are (a) single close, no cascade → Phase 3's `Done: <id> "<title>"` line; (b) single close with cascade → `Done: <id> "<title>" (and N subtasks)`; (c) multi-close → `Closed N tasks:` + per-ID bullet lines. An already-done-only call (closed==0, alreadyDone>0, but not the single-ID idempotent case) emits only the `already done: …` line with no context block. The design doc prescribed the branches but left the already-done-only case implicit; I chose to suppress the context block because there's no meaningful "what's next" to render when we didn't actually close anything.

- **`next` context computed against the *last* input ID, not per-ID.** Reused Phase 3's `computeDoneContext` verbatim. The design doc said "same shape as Phase 3's `renderDoneAck`, minus the `Done:` headline" — I did exactly that. If in Phase 5 a multi-cancel wants per-ID context (e.g., a progress ledger), the aggregation lives in the caller; keep `computeDoneContext` single-ID.

- **`findDoneDescendants` is a new helper.** `runReopen --cascade` needs the *done* descendants (not incomplete ones), so the existing `findIncompleteDescendants` doesn't apply. The two helpers are structurally identical except for the status predicate; I kept them separate for readability rather than parameterizing. Phase 5 may need a third variant for `cancel --cascade` (predicate: `status != 'done' AND status != 'canceled'`). If you add a third, consider extracting a `findDescendantsWhere(predicate)` base and having the three wrappers call into it.

- **`reopen --cascade`'s per-descendant reopened event carries `cascade: false, reopened_children: []`.** Each descendant is its own "I was reopened as part of a cascade from elsewhere" record. The *parent* event carries `cascade: true, reopened_children: [...]`. This matches the pattern the done path uses for `cascade_closed`/`cascade_closed_by_parent` — the aggregated list lives on the explicit target; descendants record their local fact.

- **`edit` with `--title` equal to current title still records the edit event.** The design doc didn't address no-op edits; I went with "record the event even if nothing changed" to give an audit trail (e.g., someone explicitly confirmed the title by passing it unchanged). The UPDATE on title is skipped via an inner guard, but `detail.old_title`/`new_title` still populate. Phase 7's error polish may want to collapse no-op edits into a "no change" message instead — note the existing behavior before changing.

- **`edit --desc ""` produces a recorded edit event with `old_desc` and `new_desc` present.** Clearing is distinct from not-touching; the `cmd.Flags().Changed("desc")` check is the signal. Locked by `TestEdit_ClearDesc`.

- **Stdin plumbing added via new `runCLIWithStdin` helper in `commands_test.go`.** Cobra's `cmd.InOrStdin()` reads from whatever `root.SetIn` was called with. The helper mirrors `runCLI` but takes a `stdin string`. Phase 5/6 verbs that might want stdin (e.g., `cancel --reason -`) can reuse it directly.

- **Note stdin form uses `strings.TrimRight(body, "\n\r")`.** A trailing newline from `echo "body" | job note id -` would otherwise become part of the appended description. Don't trim leading whitespace — might be semantically meaningful (indented code blocks).

- **`formatEventDescription` reads BOTH legacy `force` and new `cascade` keys, plus BOTH `force_closed_children` and `cascade_closed`.** Any DB that saw writes before and after Phase 4 rolls forward without blank log entries. `TestFormatEvent_LegacyForce_Renders` locks this. Phase 5's cancel events have a clean slate (new event type), so no compat burden.

- **`TestWriteRequiresAs` `done` entry passes a single positional id — still works because variadic done accepts `MinimumNArgs(1)`.** The old positional-note form `done <id> <note>` is "gone" only in the sense that the second arg is now interpreted as a second task id, not as a note. Test `TestDone_Note_Positional_Gone` locks the new behavior: `done id "some note"` errors because `"some note"` is not a valid id. If Phase 5 reinterprets positional args again, update this test.

- **`done <id> <bogus-id-that-happens-to-be-5-chars>` currently fails with `task %q not found`, NOT a distinct "looks like you're passing a note" hint.** The design doc called this out as "unless the second arg is itself a valid 5-char short ID" — I didn't special-case 5-char-looking strings because the error message is already clear. Phase 7's Levenshtein-based hint flow is the natural place to add "did you mean...?" here.

- **JSON output for done is always pretty-printed (`MarshalIndent`).** Matches existing `info`/`list --format=json` shape. If a subscriber agent wants JSON-lines, Phase 5's `tail --format=json` is the streaming path.

- **`TestHelp_Snapshot` still passes unchanged.** No new help-section headers were added. Phase 5 adds `cancel` and removes `remove`; that will change the verb listings but not the anchor phrases unless you add new sections.

- **Sandbox note (carried forward).** `go build`, `go test`, `go vet`, and `gofmt` all require `dangerouslyDisableSandbox: true` in this environment. Unchanged from Phase 1–3.

- **Files touched.** `tasks.go`, `commands.go`, `format.go`, `database.go` (added `findDoneDescendants`), `database_test.go` (signature updates), `commands_test.go` (flag updates, new stdin helper), `format_test.go` (signature update). New files: `done_test.go`, `note_test.go`, `edit_test.go`, `reopen_test.go`. README Completing/Editing sections rewritten.

- **Design doc location.** Phase 4's pre-flight expansion lives in `project/2026-04-20-phase-4-plan.md`-style filenames weren't used; the plan passed to the implementing agent via the user's prompt was the source of truth. Phase 5 should generate its own pre-flight doc under `project/` before implementation to keep the audit trail.

---

## Phase 5 — Cancellation + observation primitives (§8.1, §8.2, §8.3, §8.6) — **Critical, Additive**

**Goal.** Replace `remove` with `cancel` (non-destructive by default, `--purge` for true erasure). Add structured events on `tail --format=json`. Add `job next all` for the parallel frontier. Add `log --since`.

**Why bundled.** These four are the read-side of the orchestration story. A parent agent that wants to dispatch subagents and observe them needs: a way to stop work (cancel), a structured event stream (tail --format=json), a view of the dispatch frontier (next all), and replay (log --since). Bundling lets the tests exercise them end-to-end against a realistic scenario.

**Scope.**
- **§8.1 Cancel.**
  - New `cancel <id> --reason "..."` — sets status `canceled`, emits a `cancel` event, preserves history. Hidden from default `list`; visible in `list all`.
  - `cancel --cascade <id>` — cancels target + open descendants; one event per task.
  - `cancel --purge <id> --reason "..."` — destructive; erases task + events. `--reason` mandatory.
  - `cancel --purge --cascade <id>` — requires `--yes` to confirm.
  - Retire `remove` verb. Help text and errors point callers to `cancel [--purge]`.
- **§8.2 Structured events.**
  - `tail --format=json` emits one JSON object per line: `{"event", "id", "by", "at", ...type-specific}`.
  - Event types: `create`, `claim`, `release`, `note`, `done`, `reopen`, `cancel`, `expire`, `heartbeat` (heartbeat lands in Phase 6 but the stream format reserves it).
  - Filter flags: `--events <types>` (comma-separated), `--users <names>`, `--tree <id>` (scope to subtree).
  - **Heartbeat noise:** heartbeat events are filtered from default tail output. `--events heartbeat` opts in. (Phase 6 adds the emission; the filter is wired here.)
- **§8.3 `next all`.**
  - `next` → one claimable leaf (or `null` for JSON).
  - `next all` → array of all currently-claimable leaves.
  - `--label <name>`, `--tree <id>` filters.
- **§8.6 `log --since <iso8601>`.**
  - Filters event output to events at or after the given timestamp.
  - Works with `--format=json` for catch-up consumers.

**Tests (red first).**
- `cancel <id> --reason "..."` → task hidden from `list`, shown in `list all` with canceled marker; `cancel` event recorded.
- `cancel --cascade` propagates; each task emits its own event.
- `cancel --purge <id>` erases; `info <id>` returns not-found; events table no longer contains task's rows.
- `cancel --purge --cascade` without `--yes` errors; with `--yes` succeeds.
- `remove` verb is gone — invocation returns unknown-command error with pointer to `cancel`.
- `tail --format=json` emits parseable JSON-lines for each event type we can generate in Phase 5.
- `tail --events cancel,done` filters correctly.
- `tail --users alice` filters correctly.
- `tail --tree <root>` scopes correctly.
- `next` returns one leaf; `next all` returns all claimable leaves in tree order.
- `log --since <t>` truncates events by timestamp.

**Docs.**
- README: new "Cancellation" and "Orchestration" sections.
- Help text: remove `remove`; add `cancel`, `next all`, `log --since` callouts.

**Files touched.** New `cancel.go`; `commands.go` (tail, next, log, register cancel, remove `newRemoveCmd`), `format.go` (JSON-lines event shape), `events.go` (event type constants), tests.

### Phase 5 implementation notes (landed 2026-04-20)

Deltas from the Phase 5 design doc (`project/2026-04-20-phase-5-plan.md`). Read before Phase 6 so the cancel/tail/event-shape decisions aren't re-litigated.

- **`reopen` now also accepts `canceled` tasks (deviation from plan).** The original Phase 5 plan left "can a canceled task be undone?" as an open question and shipped only the cancel half. User asked mid-implementation to bundle the symmetric reopen path. Result: `runReopen` accepts `status in ("done","canceled")`; the `reopened` event carries `from_status` so log readers can distinguish. `reopen --cascade` uses a new helper `findClosedDescendants` (status in `{done, canceled}`) instead of the old `findDoneDescendants`. The old helper is still in `database.go` but unused — Phase 6 can delete it. Locked by `TestReopen_FromCanceled` and `TestReopen_Cascade_IncludesCanceledDescendants`.

- **Schema relaxation: `events.task_id` is now nullable.** Required for `cancel --purge` on a root task (the audit event survives the row deletion). Existing DBs are not migrated — `CREATE TABLE IF NOT EXISTS` is a no-op when the table already exists. In BUILD mode this is fine; first persisted DB after Phase 5 picks up the new constraint, and there is no users-have-existing-DBs concern. **For Phase 6+:** if you need to add or change another column, plan for a real migration system before shipping to anyone outside the dev loop.

- **New helper `recordOrphanEvent(tx, eventType, actor, detail)` in `database.go`.** Inserts an event with `task_id = NULL`. Only used by purge-on-root today. Existing `recordEvent(tx, taskID, ...)` signature unchanged — every other caller passes a real task id. Going forward, prefer this helper for any "audit event that outlives its subject" use case.

- **`cancel` and `cancel --purge` are one cobra command, one `runCancel` entry point, two internal execution paths.** I split into `executeCancel` (status transition path) and `executePurge` (erasure path) inside `cancel.go` rather than threading both through one giant function. `runCancel` opens the tx, decides which to call, and commits. The `--purge` and `--cascade` validation gates live in `executePurge` so a future `runCancel` caller-without-CLI can rely on them.

- **`cancel --purge` (no cascade) on a parent with children errors with the locked string.** Implementation choice: error rather than silently delete the leaves. The plan called this out as "decide at implementation"; I picked error because `--purge --cascade --yes` is the deliberate, audit-friendly way to erase a subtree. Locked by `TestCancel_Purge_RequiresCascade_WhenChildrenPresent` with the exact wording `task %s has subtasks; add --cascade --yes to purge the subtree`.

- **Three descendant-walker helpers now live in `database.go`** — `findIncompleteDescendants` (status != done; for `done --cascade`), `findClosedDescendants` (status in {done, canceled}; for `reopen --cascade`), `findOpenDescendants` (status not in {done, canceled}; for `cancel --cascade`), plus `findAllDescendants` (no status filter; for `cancel --purge --cascade`). The plan suggested extracting a `findDescendantsWhere(predicate)` base. I left them as four separate ~25-line functions because the predicates are mutually exclusive and inlining the SQL keeps each obvious. If a fifth variant lands, do the extraction then.

- **Cancel auto-unblock uses a parallel helper, not the done one.** `recordBlocksUnblockedOnCancel` in `cancel.go` mirrors `recordBlocksUnblockedOn` from `tasks.go` byte-for-byte except for the `reason` value (`"blocker_canceled"` vs `"blocker_done"`). I duplicated rather than parameterized because the call sites are the only callers and a flag-bool argument would have been awkward to test. Phase 6's heartbeat and Phase 7's labels won't need a third variant; if they do, parameterize then.

- **Purge erases in this order: audit event → events rows → blocks rows → task rows (leaves first).** The audit `purged` event must be persisted *before* the subject rows go away; otherwise the FK to a now-deleted parent breaks (when storing on parent) or the row is never written (when storing as orphan and the tx rolls back partway). Order matters; do not reorder. Blocks are deleted without emitting `unblocked` events for purged tasks (the blocker no longer exists; emitting events about a deleted entity is noise). This is a deliberate divergence from `cancel`-without-purge which DOES emit `unblocked`.

- **Cancel `--purge` event count includes events on cascaded descendants.** `eventsErased` in the result aggregates across the entire subtree. Single-target purge of a parent with N child events reports N+M total. Subscribers of the `purged` event get one event with the aggregate, not per-task counts.

- **`status` "canceled" bucket only appears when count > 0.** This keeps `0 open, 0 done\n` as the empty-DB output (locked by `TestStatus_EmptyDB`) and only adds a third comma-separated phrase when there's something to report. `TestStatus_OmitsCanceled_WhenZero` regression-tests this.

- **`list all` shows canceled tasks with `(canceled)` parenthetical.** Added a `case "canceled":` branch in `listStateParens`. Default `list` continues to filter by `status == "available"` so canceled tasks naturally drop out — no special filter logic needed in `filterTree`.

- **Tail JSON-lines emits `\n`-terminated objects via a NEW helper `formatEventLogJSONLines(w, events)` in `format.go`.** Distinct from `formatEventLogJSON` (which returns a `[...]` byte array, used by `log --format=json`). The tail handler picks one or the other based on `--format`. Don't try to unify them — `log` is batch, `tail` is stream; the surface asymmetry mirrors the use-case asymmetry. Plan called this out; followed it.

- **`tail --format=json` suppresses the "Tailing events for X (Ctrl+C to stop)..." preamble.** The preamble would corrupt JSON-lines output (subscribers parsing each line as JSON would choke). Md mode keeps the preamble. If Phase 6 adds `--until-close`, do the same gate.

- **Default-hidden event types are a hard-coded map `defaultTailHiddenEvents` in `events.go`.** Currently `{"heartbeat": true}`. When `--events` is non-empty, the default map is ignored entirely (the user opted into a specific allowlist). Phase 6 lands the heartbeat emitter; no filter change needed. If Phase 6 adds related noise types (`heartbeat_lost`, etc.), add them here.

- **`--events`/`--users` filters parse via `parseFilterList(s)` returning `map[string]bool` or nil for empty.** Nil means "no filter on this dimension". Both filters intersect (logical AND). Locked by `TestTail_Events_Intersection_Users`.

- **`next all` arg parsing: positional `parent` and `all` literal can come in either order.** Matches `list [parent] [all]` ergonomics. `next all foo` and `next foo all` both scope to parent `foo`. Locked by `TestNextAll_ScopesToParent_Either_Order`.

- **`next all` empty result is exit 0 with `No available tasks.\n` (md) or `[]\n` (json).** Not an error. Subscribers iterating want length-0 to mean "nothing to dispatch", not a failure. Plan noted this; followed it.

- **`runNext` (single) is now built on top of `queryAvailableTasks(db, parent, limit)`.** Old `runNext` did its own SQL; I extracted to share with `runNextAll`. `runNext` calls `queryAvailableTasks(db, parent, 1)` and returns `tasks[0]` or the locked "No available tasks" error. `runNextAll` calls `queryAvailableTasks(db, parent, 0)` (no LIMIT). Subtle: I pass the limit as `LIMIT %d` (string-formatted), not as a `?` parameter, because SQLite doesn't accept LIMIT as a placeholder in all drivers. Limit comes from internal Go ints, not user input — safe.

- **`log --since` parses RFC3339 only, no `1h`/`5m` shorthand.** Locked error string: `--since: invalid RFC3339 timestamp: %s`. Phase 7's polish pass can add relative-duration support; the parsing site is centralized in `newLogCmd`'s RunE.

- **Tests stub timestamps via direct `UPDATE events SET created_at = ?` rather than `currentNowFunc`.** `recordEvent` and `runAdd` use `time.Now()` directly (not `currentNowFunc`), so stubbing the global doesn't shift event timestamps. Updating the rows post-hoc is reliable. If Phase 6's heartbeat tests need fine-grained time control, either route those callers through `currentNowFunc` or use the same UPDATE trick.

- **`runRemove` and `softDeleteDescendants` deleted outright.** No alias, no shim, no transitional behavior. Help text and tests retired in lockstep. The `removed` case in `formatEventDescription` stays for legacy DBs that still hold `removed` events (locked behavior — don't delete it). The `deleted_at` column on `tasks` is dormant but kept; every query still has `AND deleted_at IS NULL`. Phase 6 can audit-and-drop both; Phase 5 just stops writing.

- **`renderCancelAck` is a separate function from `renderDoneAck`.** I copied the structure rather than parameterizing. The "single / single+cascade / multi" branching pattern is identical, but the wording differs ("Done" → "Canceled", added trailing `reason: <text>` line). Cancel intentionally has NO trailing context block (no `Next:`/`Parent X of Y` enrichment) — `computeDoneContext` is not reused. The reason line IS the ack. Plan called this out; followed it.

- **Cancel JSON shape is asymmetric for cancel vs purge.** Cancel: `{canceled: [...], already_canceled: [...], reason, purged: false}`. Purge: `{purged: true, purged_items: [...], already_canceled: [...], reason, erased_events: N}`. The `purged` boolean is the discriminator. Subscribers should switch on it before reading the canceled-only or purged-only fields.

- **Cancel `was_status` field on the event detail captures the pre-cancel state** (`available`, `claimed`, etc.). Useful for replay/audit ("this task was being worked on when it was canceled"). On a claimed task that the caller themselves held, the claim is dropped (claimed_by + claim_expires_at NULLed) as part of the same UPDATE — no separate `released` event. The `canceled` event with `was_status: "claimed"` subsumes it.

- **No `cancel <id> --reason -` stdin form.** Kept the surface narrow per plan. `runCLIWithStdin` still exists from Phase 4 if you want to wire it up later.

- **Help-text annotation `(in next release)` removed from `cancel` and `next all`.** `heartbeat (in next release)` and `tail --until-close (in next release)` stay until Phase 6 lands them. `label (in next release)` stays until Phase 7. `TestHelp_PhaseGatedVerbsAnnotated` updated to drop the cancel assertion.

- **Files touched.** New: `cancel.go`, `cancel_test.go`, `tail_test.go`, `next_all_test.go`, `log_since_test.go`. Modified: `commands.go`, `tasks.go` (deleted runRemove/softDeleteDescendants; reopen accepts canceled), `claims.go` (extracted `queryAvailableTasks`, added `runNextAll`), `database.go` (nullable events.task_id, `recordOrphanEvent`, `findClosedDescendants`, `findOpenDescendants`, `findAllDescendants`, `getEventsForTaskTreeSince`), `events.go` (`runLog` takes `since *int64`; `EventFilter`, `parseFilterList`, `filterEvents`), `format.go` (`renderCancelAck`, `renderPurgeAck`, `renderCancelJSON`, `renderNextAllText`, `renderNextAllJSON`, `formatEventLogJSONLines`, canceled/purged event descriptions), `status.go` (Canceled bucket), `commands_test.go` (drop remove from variadic-as test, drop cancel from phase-gated, add cancel to wantVerbs), `database_test.go` (delete TestRunRemove_*, TestRunLog_ExcludesSoftDeletedDescendants, mustRemove; runLog signature update via sed), `status_test.go`, `reopen_test.go`, `README.md`.

- **Sandbox note (carried forward).** `go build`, `go test`, `go vet`, `gofmt` all require `dangerouslyDisableSandbox: true`. Unchanged from Phases 1–4.

- **Design doc location.** Phase 5's design doc is `project/2026-04-20-phase-5-plan.md` (per the user's instruction to keep design docs alongside notes). Phase 6 should follow the same convention.

---

## Phase 6 — Heartbeat, TTL change, blocking tail (§8.4, §8.5) — **Critical, Additive**

**Goal.** Default claim TTL drops to 15 minutes (deviation from spec's 5m). `job heartbeat <id>` refreshes claims. `tail --until-close <id>` blocks until the target closes.

**Why last of the orchestration group.** These are the "active work" primitives that require the stream infra from Phase 5 to shine. Separated so the TTL change can be tested without confounding with cancel/tail semantics.

**Scope.**
- **§8.5 TTL + heartbeat.**
  - Change default claim TTL from 1h → 15m in `claims.go`.
  - New `heartbeat <id>` verb — extends claim expiry without other state change. Emits `heartbeat` event.
  - Per-claim `claim <id> --ttl <duration>` override retained for long-running tasks.
- **§8.4 Blocking tail.**
  - `tail --until-close <id>` blocks until task reaches `done` or `canceled` state; exits 0 on close, nonzero on timeout.
  - `--timeout <duration>` — optional deadline; on expiry, exit code 2.
  - Repeated `--until-close <id>` flags wait for all (conjunction).
  - Integrates with event filters from Phase 5 — `--until-close` is orthogonal to `--events`.

**Tests (red first).**
- Default `claim <id>` without `--ttl` produces a claim expiring 15 minutes out.
- `heartbeat <id>` extends a held claim and emits an event of type `heartbeat`.
- `heartbeat <id>` on a task the caller doesn't hold errors with a clear message.
- `tail --until-close <id>` returns exit 0 when another session closes the task (use a goroutine in-test or driver).
- `tail --until-close <id> --timeout 100ms` returns nonzero on expiry with no close event.
- Default `tail` output continues to hide heartbeat events; `tail --events heartbeat` shows them.

**Docs.**
- README: update "Claiming" section (new TTL default, heartbeat).
- Help text: update TTL wording.

**Files touched.** `claims.go`, `commands.go` (new heartbeat cmd, tail `--until-close`/`--timeout`), `events.go`, `format.go`, tests.

### Phase 6 implementation notes (landed 2026-04-20)

Shipped per spec with a few forced improvisations. Phase 7 agent should read this block before starting.

**TTL constant.** `defaultClaimTTLSeconds int64 = 900` at top of `claims.go:9`. `parseDuration("")` and the two `durStr := "1h"` literals in `commands.go` now reference it (via `formatDuration(defaultClaimTTLSeconds)` for the string form). The `"claimed"` event-description fallback in `format.go` also reads the constant instead of the hardcoded "1h". Anywhere you need the default-TTL string at render time, use `formatDuration(defaultClaimTTLSeconds)` — do not re-hardcode "15m" or it'll drift.

**Heartbeat error disambiguation — the one subtle bit.** The plan says "claimed-by-other: reuse release's wording" and "expired-was-mine-now-held-by-other: reuse `checkClaimOwnership`'s wording." Problem: `checkClaimOwnership` emits the *wrong* message for the "never held it" case (it returns the "wait for expiry, or ask X to release" string, which is about write-conflict, not about heartbeat). So in `heartbeat.go` I query `events` directly for `callerOnceHeld` and branch:
- `callerOnceHeld == true` → call `checkClaimOwnership` (it emits the "your claim expired; now held by" message).
- `callerOnceHeld == false` → emit the custom "claimed by X, not you. 'heartbeat' refreshes only your own claims." message.

The `priorHolder` map is built *before* `expireStaleClaimsInTx` runs — it's how we tell "my claim just got expireStaled to `available`" from "never claimed". If you rearrange the validation loop, preserve this ordering or the expired-was-mine-now-unclaimed error wording will collapse into the generic "not claimed" path.

**`--until-close` bare-flag UX — cobra/pflag compromise.** The plan wanted `tail X --until-close` (no value) to default to the positional id, AND `tail X --until-close Y --until-close Z` (space-separated values) to accumulate Y and Z. These two behaviors are mutually exclusive in pflag: `NoOptDefVal` on a StringSlice makes the flag *never* consume a following token, so space-separated values get parsed as positional args and cobra's `ExactArgs(1)` rejects them. I landed `NoOptDefVal = "_"` as a sentinel — this means:
- `tail X --until-close` → watches X (the "_" sentinel is replaced by positional id).
- `tail X --until-close=Y` → watches Y (equals-form works).
- `tail X --until-close=Y --until-close=Z` → watches Y and Z.
- `tail X --until-close Y` → Y parsed as positional arg → rejected.

Flag help text says `use --until-close=_ to default to the positional id` to surface this. If the next agent wants true space-separated support, the only clean path I see is dropping `NoOptDefVal` and making bare `--until-close` error out with a clear "provide an id or use `--until-close=_`" message. Re-litigate if the UX bites in practice.

**Poll interval parameterized, not overridden.** Original plan implied `runTailUntilClose` would inherit `runTail`'s hardcoded 1s poll. With `--timeout 100ms` + 1s poll, the timeout test resolution is terrible; worse, the Multi_Conjunction test with three sequential closes butts up against realistic test timeouts. Solution: added an explicit `pollInterval time.Duration` parameter to `runTailUntilClose`. The cobra layer passes `defaultTailUntilClosePollInterval = 1 * time.Second`; tests pass 20ms. No package-level override state — cleaner than the "test hook" alternative. If Phase 7 adds more time-sensitive tail behavior, follow this pattern (parameter, not global override).

**`timeoutCtx` instead of a bool.** First cut had a goroutine setting a `timedOut` bool when the deadline fired, which the main goroutine then read after `runTail` returned. Classic data race (the race detector caught it on the first `-race` run). Fixed by keeping the `timeoutCtx` handle around and checking `errors.Is(timeoutCtx.Err(), context.DeadlineExceeded)` *after* `runTail` returns — `context` already handles the cross-goroutine synchronization. No goroutine, no bool. Pattern worth repeating anywhere else you need "did the deadline fire" after a blocking call.

**Pre-existing flaky test fixed in-flight.** `TestRunTail_PicksUpNewEvents` (database_test.go) was already flaky before Phase 6 — it races the test goroutine against `mustClaim`, so the tail's first poll can see the `created` event, hit its `len(collected) >= 1` gate, cancel the ctx, and miss `claimed`. Fix: added an `initialDrained` channel that the callback closes on first invocation, and the test waits on it before firing `mustClaim`. Also added a mutex around `collected` — the race detector flagged it once I was stress-running. If you see similar patterns elsewhere (goroutine + shared slice + "wait for goroutine to have started"), the sync-on-first-callback idiom is the cleanest move.

**Watched-task terminal detection bypasses the display filter — deliberately.** `--events note --until-close` still exits on `done`/`canceled`. Documented in README under "Synchronous waits" and in the locked test `TestTail_UntilClose_EventsFilter_DoesNotHideTermination`. The plan's risk note on this is preserved wording: filter is for *display*, terminal detection is for *control flow*.

**Exit code 2 wiring.** `errTailTimeout` sentinel lives in `events.go` (with the `errors` import added); `main.go` uses `errors.Is` (not `==`) so the wrapped form `fmt.Errorf("%d ... : %w", ..., errTailTimeout)` still matches. Tests assert `errors.Is(err, errTailTimeout)` directly on `cmd.Execute()`'s return value, not on stderr text.

**Help text cleanup.** The three `(in next release)` annotations for heartbeat and `tail --until-close` are gone. Only `label (in next release)` remains. `TestHelp_PhaseGatedVerbsAnnotated` was rewritten to assert the *absence* of `heartbeat (in next release)` alongside the continued presence of `label (in next release)` — if Phase 7 ships labels, remove that assertion and the `"label (in next release)"` phrase from help.

**Test run numbers.** `go test -race -count=1 ./...` clean across 5 stress runs. `gofmt -l` and `go vet` clean. Total test time ~10s without `-race`, ~20s with `-race`; the tail tests dominate due to the sleep-then-act goroutine pattern.

**Deferred / out-of-scope that held up.**
- No `heartbeat --ttl` flag, no lenient re-acquisition. Locked strict-15m behavior via tests. Re-litigate only if Phase 7 user feedback demands.
- `runTail` itself unchanged. All new behavior is in `runTailUntilClose`, which wraps it.
- Existing in-flight claims in any deployed DB keep their original `claim_expires_at` until expiry — no migration needed since there's no migration system and no deployed users.

---

## Phase 7 — Labels + error polish (§9, §10) — **Nice-to-have + Valuable**

**Goal.** Tasks can carry free-form labels. Errors carry actionable hints.

**Why last.** Labels are genuinely nice-to-have; error polish is valuable but concentrated (touches strings, not logic). Neither blocks anything else. This phase is the natural "polish pass" before calling the release complete.

**Scope.**
- **§9 Labels.**
  - New `labels` table: `(task_id, name)` with `UNIQUE(task_id, name)`.
  - YAML import recognizes `labels: [a, b]` per task.
  - `label <id> --add <name>` / `label <id> --remove <name>` (idempotent).
  - `list --label <name>`, `next all --label <name>` filter.
  - **No inheritance** — labels are local to each task.
- **§10 Error polish.**
  - `done` on a parent with open children: trailing `(run 'job done --cascade <id>' to close all)`.
  - `claim` on an already-claimed task: suggests `release` (if caller holds) or "wait" + expiry.
  - `import` validation errors already carry YAML paths from Phase 2; audit to confirm.
  - `edit`/`info`/any-by-id on nonexistent ID: Levenshtein distance ≤ 1 from a real ID → suggest the close match.

**Tests (red first).**
- `label <id> --add foo` then `info <id>` shows `foo`; re-adding is idempotent.
- `label <id> --remove foo` removes it; removing absent is idempotent.
- Importing YAML with `labels: [a, b]` creates label rows.
- `list --label a` filters to only tasks with label `a`.
- `next all --label a` filters the frontier.
- `done <parent>` with open children includes the `--cascade` hint.
- `claim <id>` already claimed includes the release/wait hint.
- `edit <unknown-id-close-to-real>` suggests the close match.

**Docs.**
- README: new "Labels" section.
- Help text: add `label` to Planning group.

**Files touched.** New `labels.go`; `commands.go` (label cmd, list/next filters, error messages), `database.go` (labels table schema), `import.go` (labels field), `format.go`, tests.

---

## Cross-phase concerns

- **`go fix && go fmt` pre-commit** per project CLAUDE.md — run each phase before every commit.
- **Full test suite** before each phase's final commit per user CLAUDE.md.
- **Keep README in sync** per project CLAUDE.md — update within the same PR as the code change, not in a follow-up.
- **Commit-message convention** — "This commit…" present tense (e.g., "Adds `job import` for YAML plan ingestion").
- **No accidental destructive ops.** Phase 5's `--purge --cascade --yes` flow is the only path that erases history; test it explicitly; make sure nothing else can reach destructive semantics.

## Session ordering

Strict linear order: Phase 1 → 2 → 3 → 4 → 5 → 6 → 7. Each phase's tests depend on earlier phases' surface (e.g., Phase 2 import tests use `--as` from Phase 1; Phase 5's cancel events ride on the structured-event infra landed alongside; Phase 7 label-based filters depend on `next all` from Phase 5). No productive parallelism across phases until the `--as` change is in.

---

## Realistic import YAML for testing

The YAML below is the canonical test fixture for Phase 2's import implementation. It encodes this plan itself — seven phases, each with 3–6 leaves, using `desc` multi-line bodies, `labels`, flat-namespace `ref`s, and `blockedBy` entries that resolve both by ref *and* by verbatim title. Cross-subtree block dependencies are intentional (Phase 3's "Rewrite top-level help" blocks on Phase 2's "Add `job schema` verb" by ref, for example).

```yaml
tasks:
  - title: Implement Opus 4.7 feedback recommendations
    desc: |
      Seven-phase plan landing the recommendations from
      project/2026-04-20-opus-feedback.md. Each phase is sized to one
      working session. See 2026-04-20-opus-feedback-plan.md for rationale
      and per-phase scope.
    labels: [meta, plan, refactor]
    children:

      - title: Phase 1 — Identity overhaul
        desc: |
          Replace login/logout/JOBS_USER/JOBS_KEY/keys with a global
          --as <name> flag. Gate writes on --as; reads remain open.
          Breaking change; no deprecation window per build mode.
        labels: [breaking, identity, phase-1]
        ref: phase-1
        children:
          - title: Write red tests for --as semantics on every write verb
            desc: Each write verb errors without --as, succeeds with it.
            labels: [tdd, red, identity]
            ref: p1-tests
          - title: Remove login/logout commands and key column
            desc: Drop newLoginCmd, newLogoutCmd, users.key. Clean up env reads.
            labels: [identity, breaking]
            blockedBy: [p1-tests]
          - title: Add --as persistent root flag
            desc: cobra PersistentFlag; resolve before every write's RunE.
            labels: [identity]
            blockedBy: [p1-tests]
          - title: Wire stolen-claim + claim-conflict error surfacing
            desc: Caller-directed remedies in error strings per spec §1.
            labels: [identity, errors]
            blockedBy: ["Add --as persistent root flag"]
          - title: Update README identity section
            desc: Drop eval $(job login); show --as + shell alias pattern.
            labels: [docs]
            blockedBy: [p1-tests]

      - title: Phase 2 — YAML import, schema, root detection
        desc: |
          Markdown+YAML import with atomic semantics, ref/blockedBy
          resolution, --dry-run and --parent. `job schema` emits JSON
          Schema. `job` walks up the directory tree for .jobs.db.
        labels: [import, planning, phase-2]
        ref: phase-2
        blockedBy: [phase-1]
        children:
          - title: Confirm yaml.v3 dependency with user
            desc: Project CLAUDE.md says ask before adding deps.
            labels: [deps, gate]
            ref: p2-dep-ok
          - title: Write red tests for import happy path and error paths
            desc: |
              Covers: nested children, multi-line desc, labels, ref and
              blockedBy resolution (both forms), ambiguity errors with
              YAML paths, --dry-run placeholders, --parent insertion,
              multiple YAML blocks ignoring non-tasks root, atomicity.
            labels: [tdd, red, import]
            ref: p2-tests
            blockedBy: [p2-dep-ok]
          - title: Implement Markdown fence + YAML parser
            desc: Locate first ```yaml fence with tasks: root; ignore others.
            labels: [import]
            blockedBy: [p2-tests]
          - title: Implement ref/blockedBy flat-namespace resolver
            desc: |
              First match as ref, then as verbatim title. Emit YAML-path
              errors on unresolved or ambiguous. Atomic validation pass
              before any insert.
            labels: [import, validation]
            blockedBy: ["Implement Markdown fence + YAML parser"]
          - title: Add `job schema` verb
            desc: Emit JSON Schema for the import grammar to stdout.
            labels: [import, docs-tool]
            ref: p2-schema
            blockedBy: [p2-tests]
          - title: Implement project-root detection for .jobs.db
            desc: Walk up from cwd like .git discovery; --db still overrides.
            labels: [ergonomics]
            blockedBy: [p2-tests]
          - title: Update README with Planning section and import example
            desc: Worked example — plan.md ending with a tasks: YAML block.
            labels: [docs]
            blockedBy:
              - "Implement ref/blockedBy flat-namespace resolver"
              - p2-schema

      - title: Phase 3 — Rendering, acks, status, help, gitignore
        desc: |
          Reshape everything the caller reads. Checkbox list rendering
          (GFM-safe: [ ] and [x] only, claim state in parenthetical),
          enriched done acks, `job status`, empty-state messages,
          init --gitignore, rewritten top-level help.
        labels: [ui, phase-3]
        ref: phase-3
        blockedBy: [phase-2]
        children:
          - title: Write red tests for list checkbox rendering
            desc: |
              - [ ]/- [x] with backtick IDs; claim/expiry/blocked in the
              parenthetical. Terminal color retained but not asserted.
            labels: [tdd, red, rendering]
            ref: p3-list-tests
          - title: Write red tests for done enriched acks
            desc: Mid-phase, last-child, whole-tree, skip-blocked cases.
            labels: [tdd, red, rendering]
            ref: p3-ack-tests
          - title: Implement checkbox list rendering
            desc: format.go; replace [done] tag with GFM task-list markers.
            labels: [rendering]
            blockedBy: [p3-list-tests]
          - title: Implement enriched done acks
            desc: Include next sibling, parent progress, escalation.
            labels: [rendering, acks]
            blockedBy: [p3-ack-tests]
          - title: Add `job status` one-line summary verb
            desc: "N open, M claimed by you, K done (last session: t ago)."
            labels: [rendering, new-verb]
          - title: Add empty-state messages for list
            desc: Informative output when nothing actionable remains.
            labels: [rendering]
          - title: Add gitignore hints to `job init` (+ --gitignore flag)
            desc: Print recommended entries; --gitignore appends idempotently.
            labels: [init, ergonomics]
          - title: Rewrite top-level help text
            desc: |
              Replace flat verb list with role-grouped layout, Quickstart,
              Identity, Output, Orchestration sections per spec §14.2.
              Add "(coming in next release)" notes on Phase 5/6 verbs.
            labels: [docs, help-text]
            ref: p3-help
            blockedBy: [p2-schema]

      - title: Phase 4 — Mutation ergonomics
        desc: |
          Atomic close-with-note and --result. Variadic done/reopen.
          --cascade on done/reopen. edit --desc. Retire the positional
          note form and `done --force`.
        labels: [mutation, phase-4]
        ref: phase-4
        blockedBy: [phase-3]
        children:
          - title: Write red tests for done -m / --result / stdin
            desc: Atomicity of note+done; stdin piping for note body.
            labels: [tdd, red, mutation]
            ref: p4-done-tests
          - title: Write red tests for variadic done + --cascade
            desc: Atomic multi-close; cascade closes all open descendants.
            labels: [tdd, red, mutation]
            ref: p4-cascade-tests
          - title: Implement done -m, done --result, note --result, note stdin
            desc: Atomic event pair for note+done; result stored in done detail.
            labels: [mutation]
            blockedBy: [p4-done-tests]
          - title: Implement variadic done + --cascade (and reopen --cascade)
            desc: Shared cascade walker; all-or-nothing transaction.
            labels: [mutation]
            blockedBy: [p4-cascade-tests]
          - title: Add edit --title and edit --desc (replace semantics)
            desc: Drop positional title form.
            labels: [mutation, breaking]
          - title: Update README mutation sections
            desc: done, note, edit, reopen command tables; drop --force.
            labels: [docs]
            blockedBy:
              - "Implement done -m, done --result, note --result, note stdin"
              - "Implement variadic done + --cascade (and reopen --cascade)"
              - "Add edit --title and edit --desc (replace semantics)"

      - title: Phase 5 — Cancellation + observation primitives
        desc: |
          Replace `remove` with `cancel` (non-destructive by default,
          --purge for erasure). Structured JSON events on tail. `next all`
          parallel frontier. `log --since`. Orchestration read-side.
        labels: [orchestration, phase-5]
        ref: phase-5
        blockedBy: [phase-4]
        children:
          - title: Write red tests for cancel (non-destructive + --purge)
            desc: |
              Cancel hides from default list, shows in list all; purge
              erases; cascade propagates; --purge --cascade requires --yes.
            labels: [tdd, red, cancellation]
            ref: p5-cancel-tests
          - title: Write red tests for structured tail JSON events
            desc: JSON-lines per event type; --events, --users, --tree filters.
            labels: [tdd, red, events]
            ref: p5-tail-tests
          - title: Write red tests for next all and log --since
            desc: Parallel frontier in tree order; timestamp filtering on log.
            labels: [tdd, red, observation]
            ref: p5-obs-tests
          - title: Implement cancel verb (+ --reason, --cascade, --purge, --yes)
            desc: Retire `remove`; wire unknown-command hint pointing at cancel.
            labels: [cancellation]
            blockedBy: [p5-cancel-tests]
          - title: Implement structured events on tail --format=json
            desc: JSON-lines shape; filter flags; heartbeat filtered by default.
            labels: [events, tail]
            blockedBy: [p5-tail-tests]
          - title: Implement `next all` (array form of next)
            desc: |
              Bare `next` returns one leaf (or null JSON); `next all`
              returns all claimable leaves. --label and --tree filters.
            labels: [observation]
            blockedBy: [p5-obs-tests]
          - title: Implement log --since <iso8601>
            desc: Timestamp filter on event output.
            labels: [observation]
            blockedBy: [p5-obs-tests]
          - title: Update README with Cancellation + Orchestration sections
            desc: Document cancel, next all, log --since, and tail JSON shape.
            labels: [docs]
            blockedBy:
              - "Implement cancel verb (+ --reason, --cascade, --purge, --yes)"
              - "Implement structured events on tail --format=json"
              - "Implement `next all` (array form of next)"
              - "Implement log --since <iso8601>"

      - title: Phase 6 — Heartbeat, TTL, blocking tail
        desc: |
          Default TTL drops to 15 minutes (deviation from spec's 5m).
          `job heartbeat <id>` refreshes claims. `tail --until-close <id>`
          blocks until target closes (with optional --timeout).
        labels: [orchestration, phase-6]
        ref: phase-6
        blockedBy: [phase-5]
        children:
          - title: Write red tests for 15m default TTL + heartbeat
            desc: Claim without --ttl → expiry +15m; heartbeat extends + emits event.
            labels: [tdd, red, claims]
            ref: p6-hb-tests
          - title: Write red tests for tail --until-close and --timeout
            desc: Exit 0 on close; nonzero on timeout; conjunctive multi-flag.
            labels: [tdd, red, tail]
            ref: p6-until-tests
          - title: Change default claim TTL to 15 minutes
            desc: claims.go default; per-claim --ttl override retained.
            labels: [claims, behavior-change]
            blockedBy: [p6-hb-tests]
          - title: Implement `job heartbeat <id>` verb
            desc: Refresh claim expiry; emit heartbeat event; error if not holder.
            labels: [claims, new-verb]
            blockedBy: [p6-hb-tests]
          - title: Implement tail --until-close and --timeout
            desc: Block loop on event stream; exit-code semantics per spec §8.4.
            labels: [tail]
            blockedBy: [p6-until-tests]
          - title: Update README claiming section for new TTL + heartbeat
            desc: Explain 15m default, heartbeat workflow, --until-close.
            labels: [docs]
            blockedBy:
              - "Change default claim TTL to 15 minutes"
              - "Implement `job heartbeat <id>` verb"
              - "Implement tail --until-close and --timeout"

      - title: Phase 7 — Labels + error polish
        desc: |
          Free-form labels per task (no inheritance). Error message
          polish with Levenshtein suggestions and cascade hints on
          open-children done errors.
        labels: [polish, phase-7]
        ref: phase-7
        blockedBy: [phase-6]
        children:
          - title: Write red tests for label add/remove + filter
            desc: Idempotent add/remove; list --label and next all --label filters.
            labels: [tdd, red, labels]
            ref: p7-label-tests
          - title: Write red tests for error polish
            desc: |
              Cascade hint in done-on-open-parent; release/wait hint on
              claim conflict; Levenshtein suggestion on unknown ID edit.
            labels: [tdd, red, errors]
            ref: p7-err-tests
          - title: Add labels table + `label` verb + YAML import field
            desc: CREATE TABLE IF NOT EXISTS labels; wire YAML labels field.
            labels: [labels, schema]
            blockedBy: [p7-label-tests]
          - title: Add --label filter to list and next all
            desc: Filter by label name.
            labels: [labels]
            blockedBy: ["Add labels table + `label` verb + YAML import field"]
          - title: Implement error-message polish pass
            desc: |
              Cascade hint, claim-conflict remedy, Levenshtein ID suggestion.
              Audit Phase 2's YAML-path errors for consistency.
            labels: [errors]
            blockedBy: [p7-err-tests]
          - title: Update README with Labels section + final pass
            desc: Document labels; re-skim README end-to-end for consistency.
            labels: [docs]
            blockedBy:
              - "Add labels table + `label` verb + YAML import field"
              - "Add --label filter to list and next all"
              - "Implement error-message polish pass"
```

---

## Phase 7 implementation notes (2026-04-20)

- **Schema migration via `openDB`.** `initSchema` is now called from `openDB` as well as `createDB`. Every statement is `CREATE … IF NOT EXISTS`, so reopening a DB that already has the schema is a no-op. This is the closest thing we have to a migration system in BUILD mode and avoids needing `job init --force` after pulling Phase 7.

- **`task_labels(task_id, name)` PK doubles as the idempotency primitive.** `INSERT OR IGNORE` + `RowsAffected()` gives us the partition between `Added` and `Existing` without a second SELECT.

- **Label runners skip the event on pure no-ops.** If `label add` finds every name already attached, no `labeled` event is emitted; same for `unlabeled`. The CLI still surfaces the state through the `Already labeled:` / `Already unlabeled:` lines, so the caller never loses signal — the audit log just stays uncluttered. Locked by `TestLabelAdd_Idempotent` and `TestLabelRemove_Idempotent`.

- **Labels do not require a claim.** Any actor can add or remove labels regardless of who holds the claim. Labels are planning metadata; constraining them to the claim-holder would block a planning role from re-tagging work in flight. Locked by `TestLabelAdd_DoesNotRequireClaim`.

- **`runList` filtered post-tree, not in the SQL.** `queryAvailableTasks` (used by `next` / `next all`) gained a `labelName` parameter that ANDs in an `EXISTS` clause; that path stays SQL-side. But `runList` walks the whole tree (including done/blocked under `all`) and then filters the resulting `[]*TaskNode` via `filterByLabel`, which keeps a node when it's labeled OR when any descendant is. That preserves hierarchical context — you see the parent chain that leads to a labeled task — instead of flattening the result.

- **Validation lives in one place.** `validateLabelName` (in `labels.go`) is the single source of truth for "trim whitespace, reject empty, reject commas". Both the CLI runners and the YAML import path go through it; `validateImportLabels` wraps it to add the YAML-path prefix to the error message. Don't reintroduce separate regexes.

- **`renderMarkdownList` signature changed.** It now takes a `labels map[int64][]string`. `format_test.go`'s test helper was updated to pass `nil`. Any new caller that wants labels in the parens needs to call `collectLabels(db, nodes)` and thread the result.

- **Help-text annotation retired.** `label (in next release)` was the last `(in next release)` annotation. `TestHelp_PhaseGatedVerbsAnnotated` now asserts the *absence* of any such annotation. If a future phase introduces a new gated verb, reintroduce the annotation mechanism deliberately and update the assertion.

- **Done-incomplete-subtasks error wording.** Now ends with `(run 'job done --cascade <id>' to close all).`. The pre-existing `TestDone_WithoutCascade_OpenChildrenErrors` was loosened from `Pass --cascade` to `--cascade <id>` — only the substring match was relaxed, the regression coverage is unchanged.

- **Claim-already-claimed-by-you wording.** Now suggests both `heartbeat` and `release`. Locked by `TestClaim_AlreadyClaimedByYou_SuggestsHeartbeat`.

- **Out of scope (carried).** Levenshtein close-match for unknown IDs is still deferred — short IDs are 5-char random alphanumeric, so the false-positive risk outweighs the typo-rescue value until user feedback says otherwise. `claim --ttl <duration>` flag, multi-label AND/OR filter, label inheritance, label rename, and a dedicated `job labels` enumerator are all parked for follow-up.

- **Sandbox note (carried forward).** `go build`, `go test`, `gofmt`, `go vet` still need `dangerouslyDisableSandbox: true`.
