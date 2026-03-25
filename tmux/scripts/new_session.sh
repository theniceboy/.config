#!/bin/bash

LOCK="/tmp/tmux-new-session.lock"
current_session_id="${1:-}"

touch "$LOCK"
session_id=$(tmux new-session -d -P -s 'session' -F '#{session_id}' 2>/dev/null)

if [ -z "$session_id" ]; then
  rm -f "$LOCK"
  exit 0
fi

if [ -n "$current_session_id" ]; then
  python3 "$HOME/.config/tmux/scripts/session_manager.py" insert-right "$current_session_id" "$session_id"
else
  python3 "$HOME/.config/tmux/scripts/session_manager.py" ensure
fi

rm -f "$LOCK"

tmux switch-client -t "$session_id"
