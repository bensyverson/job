# Job

Job is a command-line tool that keeps track of your tasks in a single file so you can organize work from the terminal.

[MIT License](LICENSE)

## Install

```sh
go install github.com/bensyverson/job@latest
```

This drops the `job` binary into `$HOME/go/bin`. Make sure that's on your `PATH`.

## Get started

```sh
# Create a task database in the current directory
job init

# Add tasks (writes require --as <name>)
job --as alice add "Write documentation"
job --as alice add "Ship v1"

# Add subtasks
job --as alice add "Ship v1" "Write tests"
job --as alice add "Ship v1" "Fix CI"

# See what needs doing (reads need no identity)
job list

# Complete a task
job --as alice done <id>

# Claim a task for 4 hours (default is 15m)
job --as alice claim <id> 4h

# Close a task and all of its open subtasks in one call
job --as alice done <id> --cascade

# Close multiple tasks atomically
job --as alice done <id1> <id2> <id3>

# See the full history
job log <id>
```

## Identity

Every write is attributed to the name passed via the global `--as` flag. Reads (`list`, `info`, `next`, `log`, `tail`) work without it.

```sh
# A write: who did it is required
job --as alice add "Write docs"

# A read: no identity needed
job list
```

Users are created lazily — the first time a new name writes, its row is added to the `users` table.

If you want "set once, forget" behavior in an interactive shell, a shell alias does the job:

```sh
alias job='job --as alice'
```

After that, `job add "x"` is attributed to alice. Different shells and different terminals can pick different names. The flag always wins over the alias, so `job --as bob add "x"` still attributes to bob.

Multiple agents can work in the same directory simultaneously — each passes its own `--as`. There is no password or key; the name is an attribution label, not a security boundary.

## Commands

### Database

| Command | Description |
|---------|-------------|
| `job init [--force] [--gitignore]` | Create a `.jobs.db` in the current directory. `--force` overwrites an existing one. `init` always creates the database in the current directory even if an ancestor already has one — there is no silent no-op. `--gitignore` appends recommended entries (`.jobs.db-shm`, `.jobs.db-wal`) to `./.gitignore`. |
| `job schema` | Print the JSON Schema for the `job import` grammar. Useful for feeding an agent the exact shape it should produce. |

Every command accepts `--db <path>` to use a different database file. You can also set `JOBS_DB`.

When neither is set, `job` walks up from the current directory looking for a `.jobs.db` in an ancestor directory — the same way `git` finds `.git`. That means you can run `job list` from anywhere inside your project. If no ancestor has one, `job` falls back to the literal `.jobs.db` in the current directory (so `job init` keeps working unchanged).

All writes additionally require `--as <name>` (see [Identity](#identity)).

### Creating tasks

| Command | Description |
|---------|-------------|
| `job add [parent] <title>` | Add a new task. Optionally under a parent. |
| | `--desc <text>` Set a description. |
| | `--before <id>` Insert before this sibling. |

### Viewing tasks

| Command | Description |
|---------|-------------|
| `job list [parent] [all]` | List actionable tasks. `all` includes done, claimed, and blocked. |
| `job info <id>` | Show full details for one task. |
| `job next [parent]` | Show the next available leaf (a task with no open children). Pass `--include-parents` to surface any available task. |
| `job status` | One-line summary: open / claimed by you / done, plus time since the last event. |

All four support `--format=json` (except `status`, which is always plain text).

List output is GitHub-Flavored Markdown with checkbox items, so pasting `job list all` into a PR or issue renders as a task list:

```
- [ ] `87TNz` Phase 1 — Data model
  - [x] `s1Ut5` Write red tests for Lesson
  - [ ] `9aedB` Implement Lesson struct (claimed by claude, 25m left)
  - [ ] `2F1C1` Collapse AnalysisResult (blocked on 9aedB)
```

### Completing tasks

| Command | Description |
|---------|-------------|
| `job done <id> [<id>...]` | Mark one or more tasks done, atomically. Idempotent: already-done IDs are reported, not re-recorded. |
| | `--cascade` Also close all open descendants. |
| | `-m "<note>"` Record a completion note. |
| | `--result '<json>'` Record structured JSON on the `done` event. |
| | `--format=json` Machine-readable output. |
| `job reopen <id>` | Reopen a completed task. `--cascade` also reopens all done descendants. |

### Editing tasks

| Command | Description |
|---------|-------------|
| `job edit <id> [--title <t>] [--desc <d>]` | Replace title and/or description. `--desc ""` clears the description. |
| `job note <id> -m "<text>"` | Append a timestamped note to a task's description. Use `job note <id> -` to read the note body from stdin. `--result '<json>'` attaches structured JSON to the event without touching the description. |
| `job move <id> before\|after <sibling>` | Reorder a task among its siblings. |

### Cancellation

`cancel` non-destructively stops work on a task. The task transitions to status
`canceled`, history is preserved, and any tasks blocked by it are auto-unblocked.

| Command | Description |
|---------|-------------|
| `job --as <name> cancel <id> [<id>...] --reason "<text>"` | Cancel one or more open tasks atomically. `--reason` is mandatory. |
| | `--cascade` Also cancel every still-open descendant. |
| | `--purge` Erase the task row and its events instead of transitioning state (audit trail kept on the parent task). Requires `--reason`. |
| | `--purge --cascade --yes` Erase an entire subtree. `--yes` is required and there is no interactive prompt. |
| | `--format=json` Machine-readable output. |

`reopen` accepts both `done` and `canceled` tasks, returning them to `available`.

### Claiming

| Command | Description |
|---------|-------------|
| `job claim <id> [duration]` | Claim a task. Duration defaults to `15m`. Units: `s`, `m`, `h`, `d`. |
| `job release <id>` | Release a claim. |
| `job claim-next [parent] [duration]` | Find and claim the next available leaf in one step. Pass `--include-parents` to claim any available task. |
| `job heartbeat <id> [<id>...]` | Extend your live claim(s) by 15 minutes. Errors if the caller doesn't hold the claim. |

Claims are attributed to the `--as` name. Claims expire automatically. `--force` overrides an existing claim. For long-running work, `heartbeat` refreshes a live claim without re-acquiring it.

#### Leaf-frontier semantics

A task is "claimable" iff it has no open children. Parents with open children are descended through, not surfaced, so `next`, `next all`, and `claim-next` return the set of leaves ready to work on.

- `claim <parent-with-open-children>` is refused — the lock has no referent, since the parent's executable work is in its descendants. Claim a leaf instead, or use `--include-parents` on `next` / `claim-next` to fall back to the legacy behavior.
- When the last open child of a parent is `done`, the parent **auto-closes**, cascading upward. The agent who closed the last child is attributed on every auto-close. Canceled siblings don't block the cascade (they're already "not open").
- Adding an open child to a **claimed** parent **auto-releases** the parent's claim. The parent no longer has executable work of its own, so the lock has no referent. The `released` event records the trigger and the prior claimant.

These three behaviors together mean parents are pure scaffolding: you never explicitly claim or close them, and `next` always points at real work.

### Blocking

| Command | Description |
|---------|-------------|
| `job block <blocked> by <blocker>` | Block a task until another is done. Detects circular dependencies. |
| `job unblock <blocked> from <blocker>` | Remove a block manually. Blocks also auto-remove when the blocker is done. |

### Labels

Tasks carry free-form, flat labels. Labels are local to each task — there's no inheritance — and useful for slicing the frontier by area, priority, owner, or any other dimension.

| Command | Description |
|---------|-------------|
| `job label add <id> <name> [<name>...]` | Add one or more labels. Variadic, idempotent, atomic. |
| `job label remove <id> <name> [<name>...]` | Remove one or more labels. Idempotent. |
| `job list --label <name>` | Filter the list to tasks carrying the label. |
| `job next --label <name>` | Pick the next available task carrying the label. |
| `job next all --label <name>` | The whole claimable frontier, scoped to the label. |

Labels show up on `job info <id>` and inline in `job list` parentheses. They can also be set at import time via the YAML `labels: [...]` key.

### Planning

Instead of building up a plan one `job add` at a time, write it as a Markdown document with a fenced YAML block and import the whole tree atomically.

````markdown
# Ship v1

We need to ship a minimal v1 this quarter. Here is the plan.

```yaml
tasks:
  - title: Ship v1
    ref: ship
    labels: [release, p0]
    children:
      - title: Write tests
        desc: cover the happy path and a few edges
        labels: [tests]
      - title: Fix CI
        blockedBy: [Write tests]
  - title: Announce v1
    blockedBy: [ship]
```
````

Then:

```sh
job --as alice import plan.md
```

Every task inside the first fenced YAML block whose top-level key is `tasks:` is created in a single transaction. If anything in the plan is invalid — a missing `title`, a duplicate `ref`, an unresolvable `blockedBy` — nothing is written.

| Command | Description |
|---------|-------------|
| `job import <file.md>` | Import tasks from a Markdown plan. |
| | `--dry-run` Validate without writing. |
| | `--parent <id>` Import under an existing task. |
| | `--format=json` Machine-readable echo of created IDs. |

Per-task keys in the YAML:

- `title` — required. Human-readable title.
- `desc` — optional description; supports YAML block scalars.
- `ref` — optional handle used by other `blockedBy` entries in the same import. Flat namespace across the whole plan. Not persisted.
- `blockedBy` — optional list. Each entry resolves as (1) a `ref` in the plan, (2) a verbatim task title in the plan if unambiguous, or (3) a pre-existing short ID.
- `children` — optional sub-tasks, recursive.
- `labels` — optional list of free-form labels, persisted on the task. Queryable via `list --label <name>`.

Run `job schema` for the full JSON Schema.

### Event history

| Command | Description |
|---------|-------------|
| `job log <id>` | Show full event history for a task and its descendants. |
| | `--since <rfc3339>` Only events at or after the given timestamp. |
| | `--format=json` Pretty-printed JSON array. |
| `job tail <id>` | Stream events in real-time. Polls every second until Ctrl+C. |
| | `--format=json` Emits one JSON object per line (JSON-lines), suitable for `jq -c` or line-based subscriber agents. |
| | `--events <type,type,...>` Only emit events of the listed types. By default `heartbeat` events are hidden; pass `--events heartbeat` to opt in. |
| | `--users <name,...>` Only emit events from the listed actors. |

Every event includes the actor who performed it.

### Orchestration

For multi-agent workflows, two read verbs let a parent agent dispatch and observe work:

```
job next all                 # full claimable frontier (every available, unblocked, unclaimed task)
job tail <id> --format=json  # stream events as JSON-lines, suitable for piping to subscribers
```

`next all` accepts an optional `[parent]` to scope the frontier to a subtree. The same
arg can come before or after `all`. Returns an empty array (json) or a friendly message (md)
when nothing is claimable; not an error.

#### Synchronous waits

`tail` can block until a task closes, so a parent agent can dispatch work and wait for a
child to finish without polling.

| Flag | Description |
|------|-------------|
| `--until-close=<id>` | Block until the named task reaches `done` or `canceled`. Repeatable: all listed tasks must close before exit. Use `--until-close` bare (no value) to watch the positional id. |
| `--timeout <duration>` | Exit with code **2** if the watch set hasn't drained within this duration. Units: `s`, `m`, `h`, `d`. |
| `--quiet` | Suppress the streamed event output while waiting; the close-transition line still prints. |

Exit codes for `tail --until-close`:

- **0** — every watched task closed cleanly.
- **1** — any other error (task not found, invalid duration, db error).
- **2** — `--timeout` expired with at least one watched task still open.

The `--events` / `--users` display filters are orthogonal to `--until-close`: filters hide
events from the stream, but terminal `done` / `canceled` events always trigger exit.

```sh
# Dispatch and wait up to 5 minutes
job --as claude claim-next phaseRoot
job tail phaseRoot --until-close --timeout 5m --quiet

# Multiple watches
job tail root --until-close=aM8eT --until-close=9aedB --timeout 10m
```

## Task IDs

IDs are 5-character, case-sensitive, alphanumeric strings (e.g. `aM8eT`). A mismatch is an error, not a fuzzy match.
