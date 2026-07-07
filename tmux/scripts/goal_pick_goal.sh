#!/usr/bin/env bash
# Fuzzy goal picker for the focused window's thread.
# Bound to tmux prefix-g.
set -euo pipefail

AGENT_BIN="$HOME/.config/agent-tracker/bin/agent"
WINDOW="$(tmux display-message -p '#{window_id}' 2>/dev/null || true)"

LINE="$("$AGENT_BIN" goal pick goals | fzf --prompt='goal> ' --height=40% --reverse --ansi 2>/dev/null || true)"
[ -z "$LINE" ] && exit 0
ID="${LINE%%	*}"

if [ "$ID" = "__new__" ]; then
  exec tmux command-prompt -p "New goal title:" \
    "run-shell \"NEWID=\$($AGENT_BIN goal add-goal --title '%%'); $AGENT_BIN goal set-goal-current --goal=\$NEWID --window=$WINDOW\""
fi

exec "$AGENT_BIN" goal set-goal-current --goal="$ID" --window="$WINDOW"
