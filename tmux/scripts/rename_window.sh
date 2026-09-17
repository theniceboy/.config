#!/bin/bash

name="$1"
target="$2"

if [ -z "$name" ]; then
  exit 0
fi

tmux rename-window -t "$target" "${name// /-}"
