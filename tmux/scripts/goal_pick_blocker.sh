#!/usr/bin/env bash
# Fuzzy blocker picker for the focused window's thread.
# Bound to tmux prefix-b.
set -euo pipefail

AGENT_BIN="$HOME/.config/agent-tracker/bin/agent"
WINDOW="$(tmux display-message -p '#{window_id}' 2>/dev/null || true)"
CURRENT="$("$AGENT_BIN" goal current-thread --window="$WINDOW" 2>/dev/null || true)"

LINE="$("$AGENT_BIN" goal pick threads | fzf --prompt='blocked by> ' --height=40% --reverse --ansi 2>/dev/null || true)"
[ -z "$LINE" ] && exit 0
ID="${LINE%%	*}"
[ -n "$CURRENT" ] && [ "$ID" = "$CURRENT" ] && exit 0

exec "$AGENT_BIN" goal set-blocker-current --by="$ID" --window="$WINDOW"
