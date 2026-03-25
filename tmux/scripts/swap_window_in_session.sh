#!/usr/bin/env bash
set -euo pipefail

direction="${1:-}"
case "$direction" in
  left|right) ;;
  *) exit 0 ;;
esac

session_id=$(tmux display-message -p '#{session_id}' 2>/dev/null || true)
window_id=$(tmux display-message -p '#{window_id}' 2>/dev/null || true)
[[ -n "$session_id" && -n "$window_id" ]] || exit 0

windows=()
while IFS= read -r window; do
  [[ -n "$window" ]] && windows+=("$window")
done < <(tmux list-windows -t "$session_id" -F '#{window_id}' 2>/dev/null || true)
count=${#windows[@]}
(( count >= 2 )) || exit 0

current=-1
for i in "${!windows[@]}"; do
  if [[ "${windows[$i]}" == "$window_id" ]]; then
    current=$i
    break
  fi
done
(( current >= 0 )) || exit 0

if [[ "$direction" == "left" ]]; then
  target_index=$(( current == 0 ? count - 1 : current - 1 ))
else
  target_index=$(( current == count - 1 ? 0 : current + 1 ))
fi

target_window_id="${windows[$target_index]}"
[[ -n "$target_window_id" && "$target_window_id" != "$window_id" ]] || exit 0

tmux swap-window -d -s "$window_id" -t "$target_window_id"
