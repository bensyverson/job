# Jobs: A Lightweight CLI Task Manager

**Date:** 2026-04-18
**Status:** Draft

---

## 1. Vision

Jobs is a command-line task manager designed for **LLM agents** first, humans second. It provides a hierarchical task list backed by SQLite, with short alphanumeric IDs, time-based claiming, dependency blocking, and a full event stream.

### Design Principles

1. **Agent-native.** Token-efficient output, machine-parseable formats (JSON), predictable exit codes, and clear error messages that tell an agent exactly what to do next.
2. **Simple.** One binary, one SQLite file, no daemon, no network. Tasks are trees. Status is a small enum. Blocking is a relationship, not a status.
3. **Portable.** The database lives in the project directory (like `.git`). No global state. Works in any directory with `job init`.
4. **Observable.** Every state change is recorded as an event. `job log` and `job tail` give full visibility into what happened and when.

### Why SQLite?

- Zero config, zero daemon, single-file database.
- WAL mode provides adequate concurrent-read performance for multiple agents.
- Schema migrations are trivial for a single-file DB.
- Ubiquitous — every Go installation can target it via `modernc.org/sqlite` (pure Go, no CGO required).

### Why not `TODO.md` or JSON files?

- No concurrent access safety.
- No event history.
- No query capability.
- Trees in flat files require custom parsers.
- Merge conflicts in git.

---

## 2. Command Reference

### Conventions

- **Positional arguments** are shown in `<angle_brackets>`.
- **Flags** are shown in `[square_brackets]`.
- Commands follow a **verb-noun** pattern: `job <verb> [args]`.
- Task IDs are **5-character, mixed-case, base62 alphanumeric** strings (e.g., `aM8eT`, `dAp83`). Case-sensitive — a mismatch is an error, not a fuzzy match.
- All output goes to **stdout**. All errors and diagnostics go to **stderr**.
- Exit codes: `0` = success, `1` = application error, `2` = usage error.

### Database & Init

#### `job init`

Initialize a new `.jobs.db` in the current directory. Errors if one already exists (use `--force` to overwrite, which destroys all data).

```
$ job init
Initialized .jobs.db

$ job init
Error: .jobs.db already exists in this directory. Use --force to overwrite.

$ job init --force
Initialized .jobs.db (overwrote existing database)
```

If no database exists when any other command is run:

```
$ job list
Error: No Jobs database found in /current/directory. Run `job init` or specify a database with --db.
```

The `--db` flag is available on every command to specify an alternate database path:

```
$ job --db /path/to/jobs.db list
```

The `JOBS_DB` environment variable is also supported. Precedence: `--db` flag > `JOBS_DB` env > `.jobs.db` in CWD.

---

### Task Creation

#### `job add [parent] <title> [--desc=<description>] [--before=<id>]`

Add a new task. If `parent` is provided, the task is added as a child of that task. Otherwise, it's added as a root-level task.

The `title` is the task name (required). An optional `--desc` provides a longer description.

`--before` inserts the new task before the specified sibling. Without it, the task is appended at the end of its siblings.

On success, prints the new task's ID to stdout.

```
$ job add "Database migration"
aM8eT

$ job add aM8eT "Write schema migrations"
dAp83

$ job add aM8eT "Update connection pooling" --desc="See architecture doc for details"
5aIxl

$ job add aM8eT "Add indexes" --before=dAp83
c4871
```

---

### Task Listing

#### `job list [parent] [all] [--format=md|json]`

List tasks. By default, shows only **actionable** tasks: those that are `available` (not `claimed`, not `done`) and not blocked by any incomplete task.

If `parent` is provided, lists subtasks of that task. Otherwise, lists root-level tasks.

`all` includes tasks of all statuses (claimed, done, blocked).

`--format` controls output format:
- `md` (default): Markdown unordered list with status annotations.
- `json`: JSON array of task objects.

```
$ job list
- aM8eT  Database migration
  - 5aIxl  Update connection pooling
- T2ziR  Frontend refactor

$ job list all
- aM8eT  Database migration
  - dAp83  Write schema migrations  [claimed by Jesse, 47m left]
  - 5aIxl  Update connection pooling
  - kB3nR  Design schema v2  [done, abc1234]
- T2ziR  Frontend refactor
  - c4871  Component library update  [blocked by aM8eT]
```

Status annotations (only shown when non-default):
| Status | Annotation |
|--------|-----------|
| Available | *(none)* |
| Claimed | `[claimed by <who>, <time> left]` or `[claimed, <time> left]` |
| Done | `[done, <note>]` or `[done]` |
| Blocked | `[blocked by <id>]` |

When multiple blockers exist: `[blocked by aM8eT, T2ziR]`.

#### JSON Format

```json
[
  {
    "id": "aM8eT",
    "title": "Database migration",
    "status": "available",
    "description": "",
    "children": [
      {
        "id": "dAp83",
        "title": "Write schema migrations",
        "status": "claimed",
        "claimed_by": "Jesse",
        "claim_expires_at": 1745000000,
        "children": []
      }
    ]
  }
]
```

---

### Task Completion

#### `job done <id> [--force] [note]`

Mark a task as done. Requires all subtasks to be done; otherwise, prints an error listing the incomplete children.

`--force` marks the task and all incomplete descendants as done.

An optional positional `note` is recorded (convention: a git commit hash).

```
$ job done dAp83
Done: dAp83

$ job done aM8eT
Error: aM8eT has incomplete subtasks: 5aIxl, kB3nR
Hint: Complete the subtasks first, or use --force to mark them all done.

$ job done aM8eT --force
Done: aM8eT (and 2 subtasks)

$ job done c4871 abc1234
Done: c4871 (note: abc1234)
```

#### `job reopen <id>`

Reopen a completed task. Sets the task back to `available`. If the task was closed with `--force`, also reopens all descendants that were force-closed.

```
$ job reopen aM8eT
Reopened: aM8eT (and 2 subtasks)
```

Reopen restores blockers — if the task was blocking others, those blocking relationships are not re-created (they were removed on `done`). This is intentional: blocking relationships express a dependency that was satisfied. If the dependency is re-introduced, use `job block` again.

---

### Task Info

#### `job info <id> [--format=md|json]`

Show full details of a single task: ID, title, description, status, claim info, blockers, children summary.

```
$ job info aM8eT
ID:           aM8eT
Title:        Database migration
Description:  Move to SQLite as described in project.md
Status:       available
Parent:       (root)
Children:     3 (1 done, 1 claimed, 1 available)
Blocking:     c4871
Created:      2026-04-18 14:00
```

---

### Task Editing

#### `job edit <id> <title>`

Change a task's title.

```
$ job edit aM8eT "Database migration v2"
Edited: aM8eT
```

#### `job note <id> <text>`

Append text to a task's description, prefixed with a timestamp. The description becomes an append-only scratchpad for progress notes and context.

```
$ job note aM8eT "Switched to connection pooling approach"
Noted: aM8eT

# Resulting description:
# "Move to SQLite as described in project.md
#
# [2026-04-18 15:01] Switched to connection pooling approach"
```

If the description is empty, the note is added without a leading newline. Each note is preceded by a timestamp in `[YYYY-MM-DD HH:MM]` format. Two newlines separate notes from each other and from the original description.

---

### Claiming

#### `job claim <id> [duration] [by <who>]`

Claim a task, marking it as in-progress. Duration defaults to `1h`. Supported units: `s` (seconds), `m` (minutes), `h` (hours), `d` (days).

`by <who>` attaches an owner name (plain text, free-form).

Claims expire automatically. Expiry is checked lazily on read — no background process. If a claim has expired, the task is treated as `available` and can be claimed by someone else.

```
$ job claim dAp83
Claimed: dAp83 (expires in 1h)

$ job claim dAp83 4h
Claimed: dAp83 (expires in 4h)

$ job claim dAp83 by Jesse
Claimed: dAp83 by Jesse (expires in 1h)

$ job claim dAp83 4h by Jesse
Claimed: dAp83 by Jesse (expires in 4h)
```

Claiming a task that is already claimed by someone else (and not yet expired):

```
$ job claim dAp83
Error: dAp83 is already claimed by Jesse (expires in 23m)
Hint: Wait for the claim to expire, or use --force to override.
```

`--force` overrides an existing claim:

```
$ job claim dAp83 --force
Claimed: dAp83 (overrode previous claim by Jesse, expires in 1h)
```

#### `job release <id>`

Release a claim, returning the task to `available`.

```
$ job release dAp83
Released: dAp83
```

#### `job claim-next <parent> [duration] [by <who>] [--force]`

Find the next available (unblocked, unclaimed, not done) task within `parent` and claim it. Equivalent to finding the next task and claiming it in one step.

Prints the claimed task's ID, title, and description.

```
$ job claim-next aM8eT
Claimed: dAp83 "Write schema migrations" (expires in 1h)

  Write schema migrations for the new SQLite backend.

$ job claim-next aM8eT 4h by Jesse
Claimed: 5aIxl "Update connection pooling" by Jesse (expires in 4h)
```

If no tasks are available:

```
$ job claim-next aM8eT
Error: No available tasks found under aM8eT
```

#### `job next <parent>`

Show the next available task without claiming it. Prints ID, title, and description.

```
$ job next aM8eT
dAp83  Write schema migrations

  Write schema migrations for the new SQLite backend.
```

If no tasks are available, exits with code 1 and prints an error.

---

### Blocking

#### `job block <blocked> by <blocker>`

Declare that `blocked` cannot proceed until `blocker` is complete. The `blocked` task is excluded from `list` output (but visible in `list all`).

```
$ job block c4871 by aM8eT
Blocked: c4871 (blocked by aM8eT)
```

When `aM8eT` is marked done, the blocking relationship is automatically removed and `c4871` becomes actionable again.

Circular dependency check:

```
$ job block aM8eT by c4871
Error: Cannot block aM8eT by c4871: would create a circular dependency (aM8eT → c4871 → aM8eT)
```

#### `job unblock <blocked> from <blocker>`

Remove a blocking relationship manually.

```
$ job unblock c4871 from aM8eT
Unblocked: c4871 (was blocked by aM8eT)
```

---

### Removal

#### `job remove <id> [--force]`

Remove a task. Requires confirmation (interactive `y/N` prompt). `--force` skips confirmation.

If the task has children, errors unless `all` is specified:

```
$ job remove aM8eT
Error: aM8eT has 3 subtasks. Use `job remove aM8eT all` to remove them all, or remove subtasks first.

$ job remove aM8eT all
Remove aM8eT and 3 subtasks? [y/N] y
Removed: aM8eT (and 3 subtasks)
```

Removing a task also removes:
- All blocking relationships involving the task (both as blocker and blocked).
- All events for the task and its descendants.
- All descendants (if `all` is specified).

---

### Reordering

#### `job move <id> before|after <sibling>`

Move a task relative to a sibling. Both tasks must share the same parent.

```
$ job move 5aIxl before dAp83
Moved: 5aIxl before dAp83

$ job move 5aIxl after dAp83
Moved: 5aIxl after dAp83
```

If tasks are not siblings:

```
$ job move 5aIxl before c4871
Error: 5aIxl and c4871 are not siblings (different parents)
```

---

### Event Stream

#### `job log <id>`

Show the full event history for a task and its descendants, from creation to now.

```
$ job log aM8eT
[2026-04-18 14:00] aM8eT created: "Database migration"
[2026-04-18 14:01] dAp83 created: "Write schema migrations"
[2026-04-18 14:02] 5aIxl created: "Update connection pooling"
[2026-04-18 14:05] dAp83 claimed by Jesse (1h)
[2026-04-18 14:30] aM8eT noted: "Switched to connection pooling approach"
[2026-04-18 15:05] dAp83 claim expired
[2026-04-18 15:06] dAp83 claimed by Agent-1 (2h)
[2026-04-18 15:30] dAp83 done (note: abc1234)
[2026-04-18 15:31] aM8eT done --force (and 1 subtask)
```

#### `job tail <id>`

Stream events in real-time for a task and its descendants. Polls the database for new events and prints them as they occur. Blocks until interrupted (Ctrl+C).

```
$ job tail aM8eT
Tailing events for aM8eT (Ctrl+C to stop)...
[2026-04-18 15:45] dAp83 reopened
[2026-04-18 15:46] dAp83 claimed by Jesse (1h)
```

Implementation: polls the events table every 1 second, tracking the last-seen event ID.

---

## 3. Database Schema

```sql
-- Core task table
CREATE TABLE tasks (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    short_id         TEXT UNIQUE NOT NULL,
    parent_id        INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'available',  -- available | claimed | done
    sort_order       INTEGER NOT NULL DEFAULT 0,
    claimed_by       TEXT,
    claim_expires_at INTEGER,  -- unix timestamp, NULL if not claimed
    completion_note  TEXT,
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at       INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX idx_tasks_short_id ON tasks(short_id);
CREATE INDEX idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX idx_tasks_status ON tasks(status);

-- Blocking relationships
CREATE TABLE blocks (
    blocker_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (blocker_id, blocked_id)
);

CREATE INDEX idx_blocks_blocked_id ON blocks(blocked_id);

-- Append-only event log
CREATE TABLE events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    detail      TEXT,  -- JSON payload with context-specific data
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX idx_events_task_id ON events(task_id);
CREATE INDEX idx_events_created_at ON events(created_at);
```

### Design Decisions

- **Event sourcing.** The events table is the authoritative source of truth. The tasks and blocks tables are materialized caches for query performance, updated in the same transaction as event insertion. The full state of the system can be reconstructed by replaying the event log. See §3a for details.
- **Timestamps as Unix epochs.** Simpler than ISO strings in SQLite, trivial to format on output.
- **`sort_order` as integer.** Simple, stable ordering within siblings. Moves require renumbering siblings, but the list is small.
- **`ON DELETE CASCADE`.** Deleting a parent cascades to children, blocks, and events.
- **WAL mode.** Enabled on connection for concurrent-read performance.
- **`updated_at`** is updated on every mutation. Useful for debugging.

### Event Types

Every state change produces an event. Events carry enough detail to fully reconstruct task state from the log alone.

| Event Type | Detail (JSON) | Triggered By |
|-----------|---------------|-------------|
| `created` | `{"parent_id": null, "title": "...", "description": "...", "sort_order": 0}` | `add` |
| `claimed` | `{"by": "Jesse", "duration": "1h", "expires_at": 1745000000}` | `claim` |
| `released` | `{"was_claimed_by": "Jesse"}` | `release` |
| `claim_expired` | `{"was_claimed_by": "Jesse"}` | Lazy detection on read |
| `done` | `{"note": "abc1234", "force": false, "force_closed_children": []}` | `done` |
| `reopened` | `{"reopened_children": ["...", "..."]}` | `reopen` |
| `noted` | `{"text": "...", "description_after": "full description after append"}` | `note` |
| `edited` | `{"old_title": "...", "new_title": "..."}` | `edit` |
| `blocked` | `{"blocked_id": "c4871", "blocker_id": "aM8eT"}` | `block` |
| `unblocked` | `{"blocked_id": "c4871", "blocker_id": "aM8eT", "reason": "manual" or "blocker_done"}` | `unblock`, auto-cleanup on `done` |
| `moved` | `{"direction": "before", "relative_to": "...", "old_sort_order": 2, "new_sort_order": 0}` | `move` |
| `removed` | `{"title": "...", "children_removed": ["...", "..."], "was_status": "available"}` | `remove` |

Each event captures:
- **Who/what changed** (`task_id`)
- **What the change was** (`event_type` + `detail`)
- **When** (`created_at`)

From the event log alone, you can derive:
- **Task existence:** a `created` event without a corresponding `removed` event means the task exists.
- **Title:** the title from the latest `created` or `edited` event.
- **Description:** the `description` from `created`, plus all `noted` appends applied in order.
- **Status:** the latest of `claimed`, `released`, `done`, `reopened`, or `claim_expired`.
- **Sort order:** the `sort_order` from `created`, updated by any `moved` event.
- **Blocking relationships:** all `blocked` events minus corresponding `unblocked` events.
- **Claim state:** the latest `claimed` event's `expires_at`, superseded by `released`, `claim_expired`, or `done`.

### Short ID Generation

IDs are base62-encoded ( `[a-zA-Z0-9]` ) auto-incrementing integers. 5 characters provides ~916 million unique IDs, which is effectively unlimited for a project task manager.

```go
const base62Chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func encodeID(n int64) string {
    if n == 0 { return "a" }
    result := make([]byte, 0, 5)
    for n > 0 {
        result = append(result, base62Chars[n%62])
        n /= 62
    }
    // Pad to 5 chars
    for len(result) < 5 {
        result = append(result, 'a')
    }
    return string(result)
}
```

### §3a. Event Sourcing Architecture

The event log is the single source of truth. Every mutation follows this pattern:

1. Validate the operation (task exists, preconditions met, etc.).
2. Insert an event into the `events` table.
3. Update the `tasks` (and/or `blocks`) cache tables.
4. Commit as a single SQLite transaction.

Steps 2 and 3 are atomic — the cache and the log are always consistent.

**Why cache?** Replaying thousands of events for every `list` query is unnecessary for a task manager. The cached `tasks` table enables fast, simple SQL queries. The event log enables:

- **`job log`** and **`job tail`** — full history without extra bookkeeping.
- **`job reopen`** — `reopened` events record which children were force-closed, so they can be restored.
- **Debugging** — if something goes wrong, the event log tells you exactly what happened and when.
- **State reconstruction** — in theory, the `tasks` table could be dropped and rebuilt by replaying events. A future `job verify` command could check cache/log consistency.

**Events are append-only.** We never update or delete events (including on `remove` — the removal itself is an event). This preserves a complete audit trail.

**`removed` events capture full state.** When a task is removed, the event records the task's title, status, and children at the time of removal. This provides traceability even after the `tasks` rows are deleted.

---

## 4. Project Structure

```
Jobs/
├── project/
│   └── 2026-04-18-jobs-vision.md   # this document
├── main.go                          # entry point, CLI parsing
├── database.go                      # SQLite connection, schema, migrations
├── models.go                        # Task, Event, Block structs
├── commands.go                      # command implementations
├── format.go                        # output formatting (markdown, JSON)
├── go.mod
└── go.sum
```

Single Go package, no sub-packages. The tool is small enough to keep flat.

### Dependencies

- `modernc.org/sqlite` — pure-Go SQLite driver (no CGO). Chosen over `mattn/go-sqlite3` to avoid CGO compilation requirements, which simplifies cross-compilation and deployment.
- `github.com/spf13/cobra` — CLI framework. Handles subcommands, flags, help text, and tab completion out of the box.

### Binary Name

`job` (single command, singular). The project/repo is "Jobs", the binary is `job`.

---

## 5. Claim Expiry

Claims are checked lazily — no background goroutine or timer. Whenever a task is read (via `list`, `info`, `next`, `claim`, etc.), its `claim_expires_at` is compared to the current time. If expired:

1. A `claim_expired` event is recorded (capturing who the claim was from).
2. The task's cached `status` is set to `available`.
3. `claimed_by` and `claim_expires_at` are set to `NULL`.

This means claim expiry is only observed when someone interacts with the system, which is acceptable for a CLI tool.

---

## 6. Blocking

A task is **blocked** if any row in `blocks` references it as `blocked_id` AND the corresponding `blocker_id` task has `status != 'done'`.

Query for "is task X blocked?":

```sql
SELECT b.blocker_id, t.short_id, t.title
FROM blocks b
JOIN tasks t ON t.id = b.blocker_id
WHERE b.blocked_id = ?
  AND t.status != 'done'
```

When a task is marked `done`, all rows in `blocks` where `blocker_id = <task>` are deleted. This may unblock multiple tasks.

Circular dependency check (when adding a block): walk the `blocks` graph from `blocked` to see if we can reach `blocker`.

---

## 7. Implementation Phases

### Phase 1: Foundation & Core CRUD

**Goal:** A working task manager you can create, list, and complete tasks with.

**Scope:**
- Project setup: `go.mod`, dependencies (`cobra`, `modernc.org/sqlite`)
- `database.go`: SQLite connection, schema creation, WAL mode, `--db` flag / `JOBS_DB` env
- `models.go`: `Task`, `Event` structs
- `job init`
- `job add [parent] <title> [--desc=...] [--before=...]`
- `job list [parent] [all]` — Markdown output only
- `job done <id> [--force] [note]`
- `job reopen <id>`
- Short ID generation (base62)
- Event recording for: `created`, `done`, `reopened`
- Basic error handling and exit codes

**Not included:** JSON output, claiming, blocking, move, remove, edit, note, info, log, tail.

**Test:** After this phase, you can `init`, `add` a tree of tasks, `list` them, `done` leaf tasks, `done` parents, and `reopen` them.

---

### Phase 2: Task Management & Output

**Goal:** Full task editing, removal, reordering, and JSON output.

**Scope:**
- `format.go`: Markdown and JSON formatters
- `job list [parent] [all] --format=json`
- `job info <id> --format=md|json`
- `job edit <id> <title>`
- `job note <id> <text>` (with timestamp prepending)
- `job remove <id> [all] [--force]` (with interactive confirmation)
- `job move <id> before|after <sibling>`
- Event recording for: `edited`, `noted`, `moved`, `removed`
- Status annotations in `list` output (claimed, blocked, done)

**Test:** After this phase, you can edit/reorder/remove tasks, see rich status annotations, and get JSON output for scripting.

---

### Phase 3: Claims & Blocking

**Goal:** Full claiming and dependency management.

**Scope:**
- `job claim <id> [duration] [by <who>] [--force]`
- `job release <id>`
- `job claim-next <parent> [duration] [by <who>]`
- `job next <parent>`
- `job block <blocked> by <blocker>`
- `job unblock <blocked> from <blocker>`
- Claim expiry (lazy detection on read)
- Circular dependency detection
- Blocking relationship auto-cleanup on `done`
- Event recording for: `claimed`, `released`, `claim_expired`, `blocked`, `unblocked`

**Test:** After this phase, you can claim/release tasks, set up dependencies, and observe claim expiry and auto-unblocking.

---

### Phase 4: Event Stream & Polish

**Goal:** Full observability, help text, edge case handling, testing.

**Scope:**
- `job log <id>` — full event history with formatted output
- `job tail <id>` — real-time event streaming (poll-based)
- `--format=json` on `log` and `tail`
- Comprehensive help text for all commands (`--help`)
- Edge case handling:
  - `done` on already-done task
  - `reopen` on available task
  - `claim` on done task
  - `blocks` with invalid IDs
  - `move` across parents
  - `remove` root task with children
  - `done` on blocked task (should succeed — blocking is a property of the *blocked* task, not the blocker)
- Unit tests for core logic (ID generation, claim parsing, blocking queries)
- Integration tests (end-to-end CLI tests)

**Test:** After this phase, the tool is feature-complete and documented.

---

## 8. Future Considerations

These are explicitly **out of scope** for v1 but worth keeping in mind:

- **`job export` / `job import`** — dump and restore the database as JSON.
- **`job sync`** — merge task lists across databases (for multi-agent workflows).
- **Task tags/labels** — for filtering beyond hierarchy.
- **Task priority** — explicit ordering beyond manual `move`.
- **`job done <id> --amend`** — change the completion note after the fact.
- **Webhook / notification on `tail`** — push events to an HTTP endpoint.
- **Shell completion** — zsh/bash completion for task IDs.
- **Configurable default claim duration** — per-database setting for default claim time.
