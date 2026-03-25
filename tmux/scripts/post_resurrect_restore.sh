#!/usr/bin/env bash
set -euo pipefail

RESTART_OP_SCRIPT="${TMUX_RESTART_OP_SCRIPT:-${XDG_CONFIG_HOME:-$HOME/.config}/tmux/scripts/restart_opencode_pane.sh}"
RESTORE_AGENT_RUN_PANES_SCRIPT="${TMUX_RESTORE_AGENT_RUN_PANES_SCRIPT:-${XDG_CONFIG_HOME:-$HOME/.config}/tmux/scripts/restore_agent_run_panes.py}"
RESTORE_AGENT_TRACKER_SCRIPT="${TMUX_RESTORE_AGENT_TRACKER_SCRIPT:-${XDG_CONFIG_HOME:-$HOME/.config}/tmux/scripts/restore_agent_tracker_mapping.py}"

resurrect_dir() {
  if [[ -d "$HOME/.tmux/resurrect" ]]; then
    printf '%s\n' "$HOME/.tmux/resurrect"
  else
    printf '%s\n' "${XDG_DATA_HOME:-$HOME/.local/share}/tmux/resurrect"
  fi
}

last_file="${TMUX_RESURRECT_LAST_FILE:-$(resurrect_dir)/last}"

op_pane_locators() {
  [[ -e "$last_file" ]] || return 0
  awk -F '\t' '
    $1 == "pane" && (index($7, "OpenCode") > 0 || index($11, "opencode") > 0) {
      print $2 ":" $3 "." $6
    }
  ' "$last_file" | awk '!seen[$0]++'
}

resume_opencode_panes() {
  [[ -x "$RESTART_OP_SCRIPT" ]] || return 0

  local locator pane_id
  while IFS= read -r locator; do
    [[ -n "$locator" ]] || continue
    pane_id="$(tmux display-message -p -t "$locator" '#{pane_id}' 2>/dev/null || true)"
    [[ -n "$pane_id" ]] || continue
    "$RESTART_OP_SCRIPT" "$pane_id" >/dev/null 2>&1 || true
  done < <(op_pane_locators)
}

restore_agent_tracker_mappings() {
  [[ -x "$RESTORE_AGENT_TRACKER_SCRIPT" ]] || return 0
  "$RESTORE_AGENT_TRACKER_SCRIPT" >/dev/null 2>&1 || true
}

restore_agent_run_panes() {
  [[ -x "$RESTORE_AGENT_RUN_PANES_SCRIPT" ]] || return 0
  "$RESTORE_AGENT_RUN_PANES_SCRIPT" >/dev/null 2>&1 || true
}

sleep 1
resume_opencode_panes
restore_agent_run_panes
restore_agent_tracker_mappings
