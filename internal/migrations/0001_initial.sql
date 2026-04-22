CREATE TABLE IF NOT EXISTS tasks (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    short_id         TEXT UNIQUE NOT NULL,
    parent_id        INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'available',
    sort_order       INTEGER NOT NULL DEFAULT 0,
    claimed_by       TEXT,
    claim_expires_at INTEGER,
    completion_note  TEXT,
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at       INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    deleted_at       INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tasks_short_id ON tasks(short_id);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS blocks (
    blocker_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    blocked_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX IF NOT EXISTS idx_blocks_blocked_id ON blocks(blocked_id);

CREATE TABLE IF NOT EXISTS task_labels (
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (task_id, name)
);
CREATE INDEX IF NOT EXISTS idx_task_labels_name ON task_labels(name);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER REFERENCES tasks(id),
    event_type  TEXT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    detail      TEXT,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);

CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT UNIQUE NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
