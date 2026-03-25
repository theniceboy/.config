#!/usr/bin/env bash
set -euo pipefail

AGENT_BIN="$HOME/.config/agent-tracker/bin/agent"
[[ -x "$AGENT_BIN" ]] || exit 0
command -v jq >/dev/null 2>&1 || exit 0

state=$("$AGENT_BIN" tracker state 2>/dev/null || true)
[[ -n "$state" ]] || exit 0

target=$(echo "$state" | jq -r '
  (.tasks // [])
  | map(
      select(
        .status == "completed" and
        (.acknowledged != true) and
        (.session_id // "") != "" and
        (.window_id // "") != ""
      )
      | . + {__ts: ((.completed_at // .started_at // "") | (fromdateiso8601? // 0))}
    )
  | if length == 0 then empty else max_by(.__ts) end
  | [.session_id, .window_id, (.pane // "")] | @tsv
' 2>/dev/null || true)

[[ -n "$target" ]] || exit 0
IFS=$'\t' read -r sid wid pid <<< "$target"
[[ -n "${sid:-}" && -n "${wid:-}" ]] || exit 0

task_pid="$pid"

pane_exists() {
  local pane="$1"
  [[ -n "$pane" ]] || return 1
  tmux list-panes -a -F "#{pane_id}" 2>/dev/null | awk -v p="$pane" '$1==p {found=1} END{exit(found?0:1)}'
}

resolve_first_pane_for_window() {
  local window="$1"
  tmux list-panes -t "$window" -F "#{pane_id}" 2>/dev/null | awk 'NF {print $1; exit}'
}

if pane_exists "$pid"; then
  resolved=$(tmux display-message -p -t "$pid" '#{session_id}:::#{window_id}:::#{pane_id}' 2>/dev/null | tr -d '\r\n')
  rsid=$(printf '%s' "$resolved" | awk -F ':::' '{print $1}')
  rwid=$(printf '%s' "$resolved" | awk -F ':::' '{print $2}')
  rpid=$(printf '%s' "$resolved" | awk -F ':::' '{print $3}')
  if [[ -n "$rsid" && -n "$rwid" && -n "$rpid" ]]; then
    sid="$rsid"
    wid="$rwid"
    pid="$rpid"
  fi
else
  pid=$(resolve_first_pane_for_window "$wid")
fi

[[ -n "${pid:-}" ]] || exit 0

RUN_DIR="$HOME/.config/agent-tracker/run"
mkdir -p "$RUN_DIR"

current=$(tmux display-message -p "#{session_id}:::#{window_id}:::#{pane_id}:::#{session_name}:::#{window_index}:::#{pane_index}" | tr -d '\r\n')
if [[ -n "$current" ]]; then
  printf '%s\n' "$current" > "$RUN_DIR/jump_back.txt"
fi

if [[ -x "$AGENT_BIN" ]]; then
  ack_pid="$pid"
  [[ -n "${task_pid:-}" ]] && ack_pid="$task_pid"
  "$AGENT_BIN" tracker command acknowledge -session-id "$sid" -window-id "$wid" -pane "$ack_pid" >/dev/null 2>&1 || true
fi

tmux switch-client -t "$sid" \; select-window -t "$wid" \; select-pane -t "$pid"
