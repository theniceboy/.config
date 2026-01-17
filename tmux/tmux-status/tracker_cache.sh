#!/usr/bin/env bash
set -euo pipefail

CACHE_FILE="/tmp/tmux-tracker-cache.json"
CACHE_MAX_AGE=1

tracker_client="$HOME/.config/agent-tracker/bin/tracker-client"

# Check if cache is fresh enough
if [[ -f "$CACHE_FILE" ]]; then
  if [[ "$OSTYPE" == darwin* ]]; then
    file_age=$(( $(date +%s) - $(stat -f %m "$CACHE_FILE") ))
  else
    file_age=$(( $(date +%s) - $(stat -c %Y "$CACHE_FILE") ))
  fi
  if (( file_age < CACHE_MAX_AGE )); then
    exit 0
  fi
fi

# Simple lock using mkdir (atomic on all systems)
LOCK_DIR="/tmp/tmux-tracker-cache.lock"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  exit 0
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

if [[ -x "$tracker_client" ]]; then
  "$tracker_client" state 2>/dev/null > "$CACHE_FILE.tmp" && mv "$CACHE_FILE.tmp" "$CACHE_FILE"
else
  echo '{}' > "$CACHE_FILE"
fi
