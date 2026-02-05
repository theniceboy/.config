#!/usr/bin/env bash
set -euo pipefail

pane_id="$1"
window_id="$2"

[[ -z "$pane_id" || -z "$window_id" ]] && exit 1

shells="bash zsh fish sh dash ksh tcsh csh"

is_shell() {
  local cmd="$1"
  for s in $shells; do
    [[ "$cmd" == "$s" ]] && return 0
  done
  return 1
}

current_cmd=$(tmux display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)
[[ -z "$current_cmd" ]] && exit 0

if is_shell "$current_cmd"; then
  exit 0
fi

tmux set -w -t "$window_id" @watching 1 2>/dev/null || true
tmux refresh-client -S

while true; do
  sleep 1
  watching=$(tmux show -wv -t "$window_id" @watching 2>/dev/null || true)
  [[ "$watching" != "1" ]] && exit 0
  cmd=$(tmux display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)
  if [[ -z "$cmd" ]] || is_shell "$cmd"; then
    break
  fi
done

tmux set -wu -t "$window_id" @watching 2>/dev/null || true
tmux set -w -t "$window_id" @unread 1 2>/dev/null || true
tmux refresh-client -S
