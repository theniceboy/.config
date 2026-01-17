#!/usr/bin/env bash
set -euo pipefail

window_id=$(tmux display-message -p '#{window_id}' 2>/dev/null || true)
[[ -z "$window_id" ]] && exit 0

CACHE_FILE="/tmp/tmux-tracker-cache.json"
[[ ! -f "$CACHE_FILE" ]] && exit 0

state=$(cat "$CACHE_FILE" 2>/dev/null || true)
[[ -z "$state" ]] && exit 0

count=$(echo "$state" | jq -r --arg wid "$window_id" '
  [.notes // [] | .[] | select(
    .archived != true and
    .completed != true and
    .scope == "window" and
    .window_id == $wid
  )] | length
' 2>/dev/null || echo "0")

if [[ "$count" =~ ^[0-9]+$ ]] && (( count > 0 )); then
  printf ' %s ' "$count"
fi
