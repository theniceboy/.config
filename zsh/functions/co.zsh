# co() - Start jcode (replaces op).
#
# Usage:
#   co              # start new jcode session in TUI
#   co -r           # resume last session for this tmux pane
#   co --resume foo # resume by session name/id
#   co --resume     # list sessions to pick from
#   co -- any jcode args
#
# Mirrors the structure of op()/opr() but adapted for jcode:
# - Sets up config (source of truth: ~/.config/jcode/config.toml)
# - Detects agent.json for browser MCP integration (same as op)
# - Passes tmux context to hooks via env vars

co() {
  local resume_session=""

  # Parse -r / --resume-last shortcut
  local -a jcode_args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      -r|--resume-last)
        resume_session="last"
        shift
        ;;
      *)
        jcode_args+=("$1")
        shift
        ;;
    esac
  done

  # Resolve resume-last from tmux pane locator
  if [ "$resume_session" = "last" ]; then
    local pane_target="${TMUX_PANE:-}"
    if [ -n "$pane_target" ]; then
      local locator
      locator=$(tmux display-message -p -t "$pane_target" \
        '#{session_name}:#{window_index}.#{pane_index}' 2>/dev/null) || true
      if [ -n "$locator" ]; then
        local sanitized="${locator//[^a-zA-Z0-9_]/_}"
        local state_file="${XDG_STATE_HOME:-$HOME/.local/state}/co/loc_${sanitized}"
        if [ -f "$state_file" ]; then
          resume_session=$(cat "$state_file" 2>/dev/null || true)
          [ -n "$resume_session" ] && print -u2 "co: resuming session $resume_session"
        else
          print -u2 "co: no previous session for this tmux pane"
          return 1
        fi
      fi
    else
      print -u2 "co: -r requires tmux"
      return 1
    fi
    jcode_args=(--resume "$resume_session")
  fi

  # ── Provider/model (explicit, no auto mode) ─────────────
  # Pass -p/-m unless the user already specified them
  local has_provider=0 has_model=0
  local a
  for a in "${jcode_args[@]:-}"; do
    case "$a" in
      -p|--provider|--provider=*) has_provider=1 ;;
      -m|--model|--model=*) has_model=1 ;;
    esac
  done
  [ "$has_provider" = 0 ] && jcode_args=(-p zai "${jcode_args[@]}")
  [ "$has_model" = 0 ] && jcode_args=(-m glm-5.2 "${jcode_args[@]}")

  # ── Detect agent.json (same logic as op) ────────────────
  local agent_workspace=""
  local agent_feature=""
  local agent_browser_url=""
  local search_dir="$PWD"
  while [ -n "$search_dir" ] && [ "$search_dir" != "/" ]; do
    if [ -f "$search_dir/agent.json" ]; then
      agent_workspace="$search_dir"
      break
    fi
    if [ "${search_dir:t}" = "repo" ] && [ -f "${search_dir:h}/agent.json" ]; then
      agent_workspace="${search_dir:h}"
      break
    fi
    search_dir="${search_dir:h}"
  done

  if [ -n "$agent_workspace" ] && command -v jq >/dev/null 2>&1; then
    local agent_json="$agent_workspace/agent.json"
    agent_browser_url=$(jq -r '.url // empty' "$agent_json" 2>/dev/null || true)
    agent_feature=$(jq -r '.feature // empty' "$agent_json" 2>/dev/null || true)
  fi

  # ── Set up CO_* env for hooks ────────────────────────────
  # Hooks read these to resolve tmux context without guessing
  local co_tmux_pane="${TMUX_PANE:-}"
  local co_tmux_session_id=""
  local co_tmux_window_id=""

  if [ -n "$co_tmux_pane" ]; then
    local tmux_ctx
    tmux_ctx=$(tmux display-message -p -t "$co_tmux_pane" \
      '#{session_id}:::#{window_id}' 2>/dev/null) || true
    if [ -n "$tmux_ctx" ]; then
      co_tmux_session_id="${tmux_ctx%%:::*}"
      co_tmux_window_id="${tmux_ctx##*:::}"
    fi
  fi

  local co_state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/co"
  mkdir -p "$co_state_dir" 2>/dev/null || true
  if [ -n "$co_tmux_pane" ]; then
    printf '%s\t%s\t%s\t%s\t%s\n' \
      "$co_tmux_pane" \
      "$co_tmux_session_id" \
      "$co_tmux_window_id" \
      "$PWD" \
      "$(date +%s)" >| "$co_state_dir/pending_pane"
  fi

  # ── Run jcode ────────────────────────────────────────────
  CO_TMUX_PANE="$co_tmux_pane" \
    CO_TMUX_SESSION_ID="$co_tmux_session_id" \
    CO_TMUX_WINDOW_ID="$co_tmux_window_id" \
    AGENT_WORKSPACE="${agent_workspace:-${AGENT_WORKSPACE:-}}" \
    AGENT_FEATURE="${agent_feature:-${AGENT_FEATURE:-}}" \
    AGENT_BROWSER_URL="${agent_browser_url:-${AGENT_BROWSER_URL:-}}" \
    RIPGREP_CONFIG_PATH="${RIPGREP_CONFIG_PATH:-$HOME/.ripgreprc}" \
    jcode --no-update "${jcode_args[@]}"
}
