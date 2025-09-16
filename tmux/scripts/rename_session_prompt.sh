#!/bin/bash

label="$1"

if [ -z "$label" ]; then
  exit 0
fi

python3 "$HOME/.config/tmux/scripts/session_manager.py" rename "$label"
