#!/usr/bin/env bash
set -euo pipefail

window_id="$1"
[[ -z "$window_id" ]] && exit 0

CACHE_FILE="/tmp/tmux-tracker-cache.json"
[[ ! -f "$CACHE_FILE" ]] && exit 0

state=$(cat "$CACHE_FILE" 2>/dev/null || true)
[[ -z "$state" ]] && exit 0

result=$(echo "$state" | jq -r --arg wid "$window_id" '
  .tasks // [] | .[] | select(.window_id == $wid) |
  if .status == "in_progress" then "in_progress"
  elif .status == "completed" and .acknowledged != true then "waiting"
  else empty end
' 2>/dev/null | head -1 || true)

case "$result" in
  in_progress) printf '⏳' ;;
  waiting) printf '🔔' ;;
esac
