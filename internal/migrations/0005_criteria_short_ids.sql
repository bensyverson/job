-- Adds a server-generated short_id to each criterion so labels can be
-- edited later without orphaning the event log, so per-criterion CLI
-- references survive shell-quoting hell, and so cross-surface references
-- (commit messages, blockedBy, DOM ids) become stable.
--
-- The column is nullable here because SQLite's ALTER TABLE cannot stamp
-- random per-row values inline; the Go-side OpenDB step backfills any
-- NULL rows immediately after migrations run, and insertCriteria mints
-- short_ids for every new row going forward. The unique index is partial
-- (WHERE short_id IS NOT NULL) so it tolerates the brief NULL window
-- between migration and backfill.
ALTER TABLE task_criteria ADD COLUMN short_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_criteria_short_id
    ON task_criteria(short_id) WHERE short_id IS NOT NULL;
