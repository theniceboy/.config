op() {
  if ! typeset -f _op_run >/dev/null; then
    local src=${funcfiletrace[1]%:*}
    local dir=${src:h}
    local common="$dir/_op_common.zsh"
    if [ ! -f "$common" ]; then
      common="${XDG_CONFIG_HOME:-$HOME/.config}/zsh/functions/_op_common.zsh"
    fi
    if [ ! -f "$common" ]; then
      print -u2 "op: missing _op_common.zsh at $common"
      return 1
    fi
    source "$common" || return 1
  fi

  OP_TRACKER_NOTIFY=1 _op_run op "$@"
}
