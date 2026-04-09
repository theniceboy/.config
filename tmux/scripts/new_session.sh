#!/bin/bash

LOCK="/tmp/tmux-new-session.lock"
current_session_id="${1:-}"
current_path="${2:-}"

touch "$LOCK"
tmux_args=(new-session -d -P -s 'session' -F '#{session_id}')
if [ -n "$current_path" ]; then
  tmux_args+=( -c "$current_path" )
  printf -v start_cmd 'cd %q && exec ${SHELL:-/bin/zsh} -l' "$current_path"
  tmux_args+=( "$start_cmd" )
fi
session_id=$(tmux "${tmux_args[@]}" 2>/dev/null)

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
