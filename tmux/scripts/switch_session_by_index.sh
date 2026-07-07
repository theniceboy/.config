#!/bin/bash

index="$1"

if [[ -z "$index" || ! "$index" =~ ^[0-9]+$ ]]; then
  exit 0
fi

current_session=$(tmux display-message -p '#{session_name}' 2>/dev/null || true)
if [[ "$current_session" =~ ^([0-9]+-)?[sS]cratch$ ]]; then
  exit 0
fi

target=$(tmux list-sessions -F '#{session_id} #{session_name}' 2>/dev/null | awk -v idx="$index" '$2 !~ "^([0-9]+-)?[sS]cratch$" && $2 ~ "^"idx"-" {print $1; exit}')

if [[ -n "$target" ]]; then
  tmux switch-client -t "$target"
  tmux refresh-client -S
fi
