#!/usr/bin/env bash
set -euo pipefail

# Determine theme color from tmux environments with fallback
# Prefer session env, then global env, else default.
theme_line=$(tmux show-environment TMUX_THEME_COLOR 2>/dev/null || true)
if [[ "$theme_line" == TMUX_THEME_COLOR=* ]]; then
  theme="${theme_line#TMUX_THEME_COLOR=}"
else
  theme_line=$(tmux show-environment -g TMUX_THEME_COLOR 2>/dev/null || true)
  if [[ "$theme_line" == TMUX_THEME_COLOR=* ]]; then
    theme="${theme_line#TMUX_THEME_COLOR=}"
  else
    theme="#b294bb"
  fi
fi

# Cache as a user option and apply to border style
tmux set -g @theme_color "$theme"
tmux set -g pane-active-border-style "fg=$theme"

exit 0
