#!/usr/bin/env bash
set -euo pipefail

window_id="${1:-}"
unread="${2:-0}"
watching="${3:-0}"
[[ -z "$window_id" ]] && exit 0

has_bell=0
has_watch=0
has_question=0

[[ "$unread" == "1" ]] && has_bell=1

[[ "$watching" == "1" ]] && has_watch=1

question_pane=$(tmux list-panes -t "$window_id" -F '#{@op_question_pending}' 2>/dev/null | grep -F -m1 -x '1' || true)
[[ -n "$question_pane" ]] && has_question=1

CACHE_FILE="/tmp/tmux-tracker-cache.json"
if [[ -f "$CACHE_FILE" ]]; then
  state=$(cat "$CACHE_FILE" 2>/dev/null || true)
  if [[ -n "$state" ]]; then
    result=$(echo "$state" | jq -r --arg wid "$window_id" '
      .tasks // [] | .[] | select(.window_id == $wid) |
      if .status == "completed" and .acknowledged != true then "waiting"
      elif .status == "in_progress" then "in_progress"
      else empty end
    ' 2>/dev/null | head -1 || true)
    case "$result" in
      waiting) has_bell=1 ;;
      in_progress) has_watch=1 ;;
    esac
  fi
fi

if (( has_question )); then
  printf '❓'
elif (( has_bell )); then
  printf '🔔'
elif (( has_watch )); then
  printf '⏳'
fi
