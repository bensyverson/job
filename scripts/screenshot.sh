#!/usr/bin/env bash
# Screenshot a prototype HTML file via Playwright's bundled Chromium.
#
# Usage:
#   scripts/screenshot.sh <input.html> <output.png> [--width=1440] [--height=900] [--full]
#
# First run will install node_modules/ under scripts/screenshot/ and the
# Chromium browser into Playwright's cache (~300MB one-time). Subsequent
# runs are fast.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SS_DIR="$DIR/screenshot"

if [ ! -d "$SS_DIR/node_modules" ]; then
  echo "→ Installing Playwright..." >&2
  (cd "$SS_DIR" && npm install --silent)
fi

# Chromium is in a shared Playwright cache; install once.
if ! "$SS_DIR/node_modules/.bin/playwright" install --dry-run chromium >/dev/null 2>&1; then
  echo "→ Installing Chromium..." >&2
  (cd "$SS_DIR" && npx --no-install playwright install chromium)
fi

node "$SS_DIR/index.mjs" "$@"
