cl() {
  if ! typeset -f _cl_run >/dev/null; then
    local src=${funcfiletrace[1]%:*}
    local dir=${src:h}
    local common="$dir/_cl_common.zsh"
    if [ ! -f "$common" ]; then
      common="${XDG_CONFIG_HOME:-$HOME/.config}/zsh/functions/_cl_common.zsh"
    fi
    if [ ! -f "$common" ]; then
      print -u2 "cl: missing _cl_common.zsh at $common"
      return 1
    fi
    source "$common" || return 1
  fi

  _cl_run cl "$@"
}
