_op_run() {
  local tag="$1"
  shift

  local -a opencode_cmd
  opencode_cmd=(opencode "$@")

  local base_home="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
  local base_config="$base_home/opencode.json"

  if [ ! -f "$base_config" ]; then
    print -u2 "$tag: missing base config at $base_config"
    return 1
  fi

  setopt local_options null_glob

  local tmp_home
  tmp_home=$(mktemp -d "${TMPDIR:-/tmp}/opencode-home.XXXXXX") || return 1
  print -u2 "$tag: using temporary OPENCODE_CONFIG_DIR at $tmp_home"

  local cleanup_cmd="print -u2 \"$tag: removing temporary OPENCODE_CONFIG_DIR $tmp_home\"; rm -rf '$tmp_home'"
  trap "$cleanup_cmd" EXIT INT TERM

  if ! cp "$base_config" "$tmp_home/opencode.json" >/dev/null 2>&1; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "$tag: failed to copy $base_config"
    return 1
  fi

  local base_agents="$base_home/AGENTS.md"
  if [ "$tag" = "se" ]; then
    local search_agents="$base_home/agent/search/AGENTS.md"
    if [ -f "$search_agents" ]; then
      base_agents="$search_agents"
    fi
  fi

  if [ -f "$base_agents" ]; then
    if ! cp "$base_agents" "$tmp_home/AGENTS.md" >/dev/null 2>&1; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "$tag: failed to copy $base_agents"
      return 1
    fi
  fi

  local -a to_link=(
    plugin
    history
    sessions
    logs
    skill
  )

  local name
  for name in "${to_link[@]}"; do
    if [ -e "$base_home/$name" ]; then
      if ! ln -s "$base_home/$name" "$tmp_home/$name" 2>/dev/null; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "$tag: failed to symlink $base_home/$name"
        return 1
      fi
    fi
  done

  if ! mkdir -p "$tmp_home/command"; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "$tag: failed to create $tmp_home/command"
    return 1
  fi

  if [ -d "$base_home/command" ]; then
    local f
    for f in "$base_home/command"/*.md; do
      [ -f "$f" ] || continue
      cp "$f" "$tmp_home/command/"
    done
  fi

  local project_prompts_dir=""
  if [ -d "$PWD/.agent-prompts" ]; then
    project_prompts_dir="$PWD/.agent-prompts"
  fi

  if [ -n "$project_prompts_dir" ]; then
    local copied_any=0
    local f
    for f in "$project_prompts_dir"/*.md; do
      [ -f "$f" ] || continue
      copied_any=1
      local filename="${f:t}"
      if ! cp -f "$f" "$tmp_home/command/prompt_$filename" >/dev/null 2>&1; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "$tag: failed to copy project prompt $f"
        return 1
      fi
    done
    if (( copied_any )); then
      print -u2 "$tag: added project prompts from $project_prompts_dir to commands"
    fi
  fi

  local explicit_session=""
  local i
  for (( i=1; i<=${#opencode_cmd[@]}; i++ )); do
    case "${opencode_cmd[$i]}" in
      -s|--session) explicit_session="${opencode_cmd[$((i+1))]}"; break ;;
      -s=*|--session=*) explicit_session="${opencode_cmd[$i]#*=}"; break ;;
    esac
  done

  OPENCODE_CONFIG_DIR="$tmp_home" \
    RIPGREP_CONFIG_PATH="${RIPGREP_CONFIG_PATH:-$HOME/.ripgreprc}" \
    "${opencode_cmd[@]}"
  local exit_code=$?

  local session_to_save=""
  if [ -n "$explicit_session" ]; then
    session_to_save="$explicit_session"
  else
    session_to_save=$(OPENCODE_CONFIG_DIR="$tmp_home" opencode session list 2>/dev/null | awk 'NR==3{print $1}')
  fi

  if [ -n "$session_to_save" ] && [ -n "$TMUX" ]; then
    local win_key
    win_key=$(tmux display-message -p '#{session_name}:#{window_name}')
    local state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/op"
    mkdir -p "$state_dir"
    printf '%s\n%s\n' "$session_to_save" "$tag" > "$state_dir/window_${win_key//[^a-zA-Z0-9_]/_}"
  fi

  trap - EXIT INT TERM
  eval "$cleanup_cmd"
  return $exit_code
}
