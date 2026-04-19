# Job

Job is a command-line tool that keeps track of your tasks in a single file so you can organize work from the terminal.

[MIT License](LICENSE)

## Install

```sh
go install github.com/bensyverson/job@latest
```

This drops the `job` binary into `$HOME/go/bin`. Make sure that's on your `PATH`.
```

## Get started

```sh
# Create a task database in the current directory
job init

# Add tasks
job add "Write documentation"
job add "Ship v1"

# Add subtasks
job add "Ship v1" "Write tests"
job add "Ship v1" "Fix CI"

# See what needs doing
job list

# Complete a task
job done <id>

# Mark a task in-progress
job claim <id> 4h by Alice

# Done with all subtasks at once
job done <id> --force

# See the full history
job log <id>
```

## Commands

### Database

| Command | Description |
|---------|-------------|
| `job init [--force]` | Create a `.jobs.db` in the current directory. `--force` overwrites an existing one. |

Every command accepts `--db <path>` to use a different database file. You can also set `JOBS_DB`.

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
| `job next [parent]` | Show the next available task. |

All three support `--format=json`.

### Completing tasks

| Command | Description |
|---------|-------------|
| `job done <id> [note]` | Mark done. Requires subtasks to be done, or use `--force`. Idempotent. |
| `job reopen <id>` | Reopen a completed task. Reopens force-closed descendants too. |

### Editing tasks

| Command | Description |
|---------|-------------|
| `job edit <id> <title>` | Rename a task. |
| `job note <id> <text>` | Append a timestamped note to a task's description. |
| `job move <id> before\|after <sibling>` | Reorder a task among its siblings. |
| `job remove <id> [all]` | Soft-delete a task. Use `all` to include descendants, `--force` to skip confirmation. |

### Claiming

| Command | Description |
|---------|-------------|
| `job claim <id> [duration] [by <who>]` | Claim a task. Duration defaults to `1h`. Units: `s`, `m`, `h`, `d`. |
| `job release <id>` | Release a claim. |
| `job claim-next [parent] [duration] [by <who>]` | Find and claim the next available task in one step. |

Claims expire automatically. `--force` overrides an existing claim.

### Blocking

| Command | Description |
|---------|-------------|
| `job block <blocked> by <blocker>` | Block a task until another is done. Detects circular dependencies. |
| `job unblock <blocked> from <blocker>` | Remove a block manually. Blocks also auto-remove when the blocker is done. |

### Event history

| Command | Description |
|---------|-------------|
| `job log <id>` | Show full event history for a task and its descendants. |
| `job tail <id>` | Stream events in real-time. Polls every second until Ctrl+C. |

Both support `--format=json`.

## Task IDs

IDs are 5-character, case-sensitive, alphanumeric strings (e.g. `aM8eT`). A mismatch is an error, not a fuzzy match.
