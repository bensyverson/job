#!/usr/bin/env bash
# backfill-sort-order.sh
#
# One-shot repair for databases populated by the pre-2026-04-23 `job import`,
# which wrote sort_order = 0 on every imported task. With all siblings
# sharing sort_order 0, `findNextSibling`'s strict-greater filter returns
# nothing and `Next:` hints silently degrade.
#
# Safe to rerun. Renumbers siblings (by rowid) only within parent scopes
# that currently contain an actual collision; scopes that already have
# distinct sort_order values — including any the user reorganised with
# `job move` — are left untouched.
#
# Usage:
#   scripts/backfill-sort-order.sh [path/to/.jobs.db]
#
# Default path: .jobs.db in the current directory.

set -euo pipefail

DB="${1:-.jobs.db}"
if [[ ! -f "$DB" ]]; then
    echo "error: database not found: $DB" >&2
    exit 1
fi

sqlite3 "$DB" <<'SQL'
.timeout 2000

BEGIN IMMEDIATE;

-- Count colliding scopes before.
CREATE TEMP TABLE _before AS
SELECT COALESCE(parent_id, -1) AS scope
FROM tasks
WHERE deleted_at IS NULL
GROUP BY parent_id, sort_order
HAVING COUNT(*) > 1;

SELECT 'before: ' || COUNT(DISTINCT scope) || ' parent scope(s) with collisions' FROM _before;

-- Identify parent scopes that need renumbering.
CREATE TEMP TABLE _bad_scopes AS
SELECT DISTINCT scope FROM _before;

-- Produce new sort_order (0..N-1, rowid-ordered) for every live task
-- under a bad scope, leaving other scopes alone.
CREATE TEMP TABLE _ranked AS
SELECT id, ROW_NUMBER() OVER (
    PARTITION BY COALESCE(parent_id, -1)
    ORDER BY id
) - 1 AS new_order
FROM tasks
WHERE deleted_at IS NULL
  AND COALESCE(parent_id, -1) IN (SELECT scope FROM _bad_scopes);

UPDATE tasks
SET sort_order = (SELECT new_order FROM _ranked WHERE _ranked.id = tasks.id)
WHERE id IN (SELECT id FROM _ranked);

-- Count colliding scopes after; should be zero.
SELECT 'after:  ' || COUNT(*) || ' parent scope(s) with collisions' FROM (
    SELECT 1
    FROM tasks
    WHERE deleted_at IS NULL
    GROUP BY parent_id, sort_order
    HAVING COUNT(*) > 1
);

SELECT 'renumbered ' || COUNT(*) || ' task row(s)' FROM _ranked;

DROP TABLE _before;
DROP TABLE _bad_scopes;
DROP TABLE _ranked;

COMMIT;
SQL
