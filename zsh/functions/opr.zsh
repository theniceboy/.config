opr() {
  if [ -z "$TMUX" ]; then
    print -u2 "opr: not in tmux"
    return 1
  fi

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
    | grep -oE -- '-s[[:space:]]+ses_[A-Za-z0-9]+' \
    | tail -n 1 \
    | grep -oE -- 'ses_[A-Za-z0-9]+')
  if [ -z "$session_id" ]; then
    session_id=$(print -rn -- "$capture" \
      | grep -oE -- 'ses_[A-Za-z0-9]+' \
      | tail -n 1)
  fi

  if [ -z "$session_id" ]; then
    print -u2 "opr: no previous op session found in this pane's history"
    return 1
  fi

  print -u2 "opr: resuming session $session_id"

  op -s "$session_id" "$@"
}
