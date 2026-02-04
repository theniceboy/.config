opr() {
  if [ -z "$TMUX" ]; then
    print -u2 "opr: not in tmux"
    return 1
  fi

  local win_key
  win_key=$(tmux display-message -p '#{session_name}:#{window_name}')
  local state_file="${XDG_STATE_HOME:-$HOME/.local/state}/op/window_${win_key//[^a-zA-Z0-9_]/_}"

  if [ ! -f "$state_file" ]; then
    print -u2 "opr: no previous session for this tmux window"
    return 1
  fi

  local session_id tag
  { read -r session_id; read -r tag; } < "$state_file"

  if [ -z "$session_id" ]; then
    print -u2 "opr: no previous session to resume"
    return 1
  fi

  tag="${tag:-op}"
  print -u2 "opr: resuming session $session_id"

  if ! typeset -f _op_run >/dev/null; then
    local src=${funcfiletrace[1]%:*}
    local dir=${src:h}
    local common="$dir/_op_common.zsh"
    if [ ! -f "$common" ]; then
      common="${XDG_CONFIG_HOME:-$HOME/.config}/zsh/functions/_op_common.zsh"
    fi
    if [ ! -f "$common" ]; then
      print -u2 "opr: missing _op_common.zsh at $common"
      return 1
    fi
    source "$common" || return 1
  fi

  _op_run "$tag" --session "$session_id" "$@"
}
