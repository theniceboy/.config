#!/bin/bash

LOCK="/tmp/tmux-new-session.lock"
if [ -f "$LOCK" ]; then
  exit 0
fi

python3 "$HOME/.config/tmux/scripts/session_manager.py" created
