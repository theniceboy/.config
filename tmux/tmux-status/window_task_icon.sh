#!/usr/bin/env bash
set -euo pipefail

window_id="${1:-}"
unread="${2:-0}"
watching="${3:-0}"
watch_failed="${4:-0}"
questions="${5:-}"
[[ -z "$window_id" ]] && exit 0

if [[ "$questions" == *1* ]]; then
  printf '❓'
elif [[ "$unread" == 1 && "$watch_failed" == 1 ]]; then
  printf '❌'
elif [[ "$unread" == 1 ]]; then
  printf '🔔'
else
  if [[ -s /tmp/tmux-tracker-cache.json ]]; then
    jq -jr --arg wid "$window_id" --arg watching "$watching" '
      [.tasks[]? | select(.window_id == $wid)]
      | if any(.status == "completed" and .acknowledged != true) then "🔔"
        elif $watching == "1" or any(.status == "in_progress") then "⏳"
        else "" end
    ' /tmp/tmux-tracker-cache.json 2>/dev/null && exit 0
  fi
  [[ "$watching" != 1 ]] || printf '⏳'
fi
