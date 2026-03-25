#!/usr/bin/env bash
set -euo pipefail

window_id="${1:-}"
[[ -z "$window_id" ]] && exit 0

todo_file="$HOME/.cache/agent/todos.json"
[[ -f "$todo_file" ]] || exit 0

count=$(jq -r --arg wid "$window_id" '[(.windows[$wid] // [])[] | select(.done != true)] | length' "$todo_file" 2>/dev/null || echo "0")

if [[ "$count" =~ ^[0-9]+$ ]] && (( count > 0 )); then
  printf ' %s ' "$count"
fi
