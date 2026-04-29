CREATE TABLE IF NOT EXISTS task_criteria (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'pending',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_task_criteria_task_id ON task_criteria(task_id);
CREATE INDEX IF NOT EXISTS idx_task_criteria_state ON task_criteria(state);
