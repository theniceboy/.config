op() {
  local -a opencode_cmd
  opencode_cmd=(opencode)
  
  local base_home="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
  local base_config="$base_home/opencode.json"

  if [ ! -f "$base_config" ]; then
    print -u2 "op: missing base config at $base_config"
    return 1
  fi

  setopt local_options null_glob

  local tmp_home
  tmp_home=$(mktemp -d "${TMPDIR:-/tmp}/opencode-home.XXXXXX") || return 1
  print -u2 "op: using temporary OPENCODE_CONFIG_DIR at $tmp_home"

  local cleanup_cmd="print -u2 \"op: removing temporary OPENCODE_CONFIG_DIR $tmp_home\"; rm -rf '$tmp_home'"
  trap "$cleanup_cmd" EXIT INT TERM

  if ! cp "$base_config" "$tmp_home/opencode.json" >/dev/null 2>&1; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "op: failed to copy $base_config"
    return 1
  fi

  local base_agents="$base_home/AGENTS.md"
  if [ -f "$base_agents" ]; then
    if ! cp "$base_agents" "$tmp_home/AGENTS.md" >/dev/null 2>&1; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "op: failed to copy $base_agents"
      return 1
    fi
  fi

  # Symlink specific persistent items
  local -a to_link
  to_link=(
    plugin
    history
    sessions
    logs
  )

  local name
  for name in "${to_link[@]}"; do
    if [ -e "$base_home/$name" ]; then
      if ! ln -s "$base_home/$name" "$tmp_home/$name" 2>/dev/null; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "op: failed to symlink $base_home/$name"
        return 1
      fi
    fi
  done

  # Prepare command directory for prompts (mapping .agent-prompts to command/)
  if ! mkdir -p "$tmp_home/command"; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "op: failed to create $tmp_home/command"
    return 1
  fi

  # Copy base commands if they exist
  if [ -d "$base_home/command" ]; then
    local f
    for f in "$base_home/command"/*.md; do
      [ -f "$f" ] || continue
      cp "$f" "$tmp_home/command/"
    done
  fi

  # Copy project prompts
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
        print -u2 "op: failed to copy project prompt $f"
        return 1
      fi
    done
    if (( copied_any )); then
      print -u2 "op: added project prompts from $project_prompts_dir to commands"
    fi
  fi

  opencode_cmd+=("$@")
  OPENCODE_CONFIG_DIR="$tmp_home" "${opencode_cmd[@]}"
  local exit_code=$?

  trap - EXIT INT TERM
  eval "$cleanup_cmd"
  return $exit_code
}
