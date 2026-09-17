ppr() {
  if [ -z "$TMUX" ]; then
    print -u2 "ppr: not in tmux"
    return 1
  fi

  local pane_target="${TMUX_PANE:-}"
  if [ -z "$pane_target" ]; then
    pane_target=$(tmux display-message -p '#{pane_id}')
  fi
  if [ -z "$pane_target" ]; then
    print -u2 "ppr: unable to determine tmux pane id"
    return 1
  fi

  local capture
  capture=$(tmux capture-pane -p -t "$pane_target" -S -10000 2>/dev/null)
  if [ -z "$capture" ]; then
    print -u2 "ppr: unable to capture pane history"
    return 1
  fi

  local session_id
  session_id=$(print -rn -- "$capture" \
    | grep -oE -- 'pi --session[[:space:]]+[0-9A-Fa-f-]{36}' \
    | tail -n 1 \
    | grep -oE -- '[0-9A-Fa-f-]{36}')

  if [ -z "$session_id" ]; then
    print -u2 "ppr: no previous Pi session found in this pane's history"
    return 1
  fi

  print -u2 "ppr: resuming session $session_id"

  command pi --session "$session_id" "$@"
}
