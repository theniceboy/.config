#!/usr/bin/env bash
set -euo pipefail
# Reads mem cache and prints pane_mem and window_mem as raw values.
# Usage: mem_usage.sh <session_name> <window_index> <pane_id>
# Output: two lines - pane mem, window mem (e.g. "120M\n450M")

CACHE_FILE="/tmp/tmux-mem-usage.json"

# Trigger cache refresh in background
python3 "$HOME/.config/tmux/tmux-status/mem_usage_cache.py" &>/dev/null &
disown 2>/dev/null

session="$1"
window_idx="$2"
pane_id="$3"

if [[ ! -f "$CACHE_FILE" ]]; then
  printf '\n'
  exit 0
fi

wkey="${session}:${window_idx}"
read -r pane_val win_val total_val < <(
  jq -r --arg pid "$pane_id" --arg wk "$wkey" \
    '[(.pane[$pid] // ""), (.window[$wk] // ""), (.total // "")] | join(" ")' \
    "$CACHE_FILE" 2>/dev/null || echo ""
)

printf '%s\n%s\n%s\n' "$pane_val" "$win_val" "$total_val"
