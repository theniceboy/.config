# v2 launcher (default `op`): shared managed service (config/db pinned via
# `opencode2 service set env`). The TUI registers its pane via
# plugins/tracker-bridge; `run` and `mini` (no TUI plugin in mini) register
# via the op-run-register DB poll below.
op() {
  case "$1" in
    run|mini)
      local sub="$1"
      shift
      __op_client_with_registrar "$sub" "$@"
      return
      ;;
  esac
  OPENCODE_CONFIG_DIR="$HOME/.config/opencode" \
  OPENCODE_DB="$HOME/.local/share/opencode-v2/opencode.db" \
  "$HOME/.opencode/bin/opencode2" "$@"
}

__op_client_with_registrar() {
  local sub="$1"
  shift
  local -a args=("$@")
  local sid="" a i=1
  while (( i <= $# )); do
    a="${args[i]}"
    if [[ ( "$a" == "-s" || "$a" == "--session" ) && (( i < $# )) ]]; then
      sid="${args[i+1]}"
      break
    fi
    if [[ "$a" == --session=* ]]; then
      sid="${a#--session=}"
      break
    fi
    (( i++ ))
  done

  local reg=""
  if [[ -n "${TMUX_PANE:-}" ]]; then
    "$HOME/.local/bin/op-run-register" "$sid" "$HOME/.local/share/opencode-v2/opencode.db" "$PWD" "$TMUX_PANE" & reg=$!
  fi

  OPENCODE_CONFIG_DIR="$HOME/.config/opencode" \
  OPENCODE_DB="$HOME/.local/share/opencode-v2/opencode.db" \
  "$HOME/.opencode/bin/opencode2" "$sub" "${args[@]}"
  local rc=$?

  if [[ -n "$reg" ]]; then
    kill -TERM "$reg" 2>/dev/null
    wait "$reg" 2>/dev/null
  fi
  return $rc
}
