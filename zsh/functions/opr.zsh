opr() {
  if [ -z "$TMUX" ]; then
    print -u2 "opr: not in tmux"
    return 1
  fi

  local db="${OPENCODE_DB:-$HOME/.local/share/opencode-v2/opencode.db}"

  local pane_target="${TMUX_PANE:-}"
  if [ -z "$pane_target" ]; then
    pane_target=$(tmux display-message -p '#{pane_id}')
  fi
  if [ -z "$pane_target" ]; then
    print -u2 "opr: unable to determine tmux pane id"
    return 1
  fi

  local capture
  capture=$(tmux capture-pane -p -t "$pane_target" -S -10000 2>/dev/null)
  if [ -z "$capture" ]; then
    print -u2 "opr: unable to capture pane history"
    return 1
  fi

  local session_id
  session_id=$(print -rn -- "$capture" \
    | grep -oE -- 'ses_[A-Za-z0-9]+' \
    | tail -n 1)

  if [ -z "$session_id" ]; then
    local dir
    dir=$(tmux display-message -p -t "$pane_target" '#{pane_current_path}')
    session_id=$(sqlite3 "$db" \
      "SELECT id FROM session_v2 WHERE directory='${dir//\'/\'\'}' AND parent_id IS NULL ORDER BY time_updated DESC LIMIT 1;" 2>/dev/null)
    [ -z "$session_id" ] && session_id=$(sqlite3 "$db" \
      "SELECT id FROM op2_tombstone WHERE directory='${dir//\'/\'\'}' AND parent_id IS NULL ORDER BY time_updated DESC LIMIT 1;" 2>/dev/null)
  fi

  if [ -z "$session_id" ]; then
    print -u2 "opr: no previous op session found in this pane's history or for this dir"
    return 1
  fi

  if ! sqlite3 "$db" "SELECT 1 FROM session_v2 WHERE id='${session_id//\'/\'\'}';" 2>/dev/null | grep -q 1; then
    command op-restore "$session_id" >/dev/null 2>&1 || true
  fi

  print -u2 "opr: resuming session $session_id"

  source "${${(%):-%x}:h}/op.zsh"
  if (( $# )); then
    op run -s "$session_id" "$@"
  else
    op -s "$session_id"
  fi
}
