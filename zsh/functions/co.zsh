co() {
  local -a codex_cmd
  codex_cmd=(codex)
  local search_dir=$PWD
  local overlay_file=""
  while :; do
    if [ -f "$search_dir/codex-mcp.toml" ]; then
      overlay_file="$search_dir/codex-mcp.toml"
      break
    fi
    if [ "$search_dir" = "/" ]; then
      break
    fi
    search_dir="$(dirname "$search_dir")"
  done

  local base_home="${CODEX_HOME:-$HOME/.codex}"
  local base_config="$base_home/config.toml"
  if [ ! -f "$base_config" ]; then
    print -u2 "co: missing base config at $base_config"
    return 1
  fi

  setopt local_options null_glob

  local tmp_home
  tmp_home=$(mktemp -d "${TMPDIR:-/tmp}/codex-home.XXXXXX") || return 1
  print -u2 "co: using temporary CODEX_HOME at $tmp_home"

  local cleanup_cmd="print -u2 \"co: removing temporary CODEX_HOME $tmp_home\"; rm -rf '$tmp_home'"
  trap "$cleanup_cmd" EXIT INT TERM

  if ! cp "$base_config" "$tmp_home/config.toml" >/dev/null 2>&1; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "co: failed to copy $base_config"
    return 1
  fi

  local base_agents="$base_home/AGENTS.md"
  if [ -f "$base_agents" ]; then
    if ! cp "$base_agents" "$tmp_home/AGENTS.md" >/dev/null 2>&1; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to copy $base_agents"
      return 1
    fi
  fi

  if [ ! -e "$tmp_home/AGENTS.md" ]; then
    if ! : > "$tmp_home/AGENTS.md"; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to create $tmp_home/AGENTS.md"
      return 1
    fi
  fi

  # Symlink only selected persistent items into the temporary home
  local -a to_link
  to_link=(
    log
    sessions
    auth.json
    history.jsonl
    internal_storage.json
    notify.py
    version.json
  )

  local name
  for name in "${to_link[@]}"; do
    if [ -e "$base_home/$name" ]; then
      if ! ln -s "$base_home/$name" "$tmp_home/$name" 2>/dev/null; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "co: failed to symlink $base_home/$name"
        return 1
      fi
    else
      print -u2 "co: note: $base_home/$name not found; skipping symlink"
    fi
  done
  print -u2 "co: prepared $tmp_home with copies of config.toml and AGENTS.md; symlinked selected persistent items"

  if [ ! -d "$base_home/prompts" ]; then
    if ! mkdir -p "$base_home/prompts"; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to create $base_home/prompts"
      return 1
    fi
    print -u2 "co: created $base_home/prompts"
  fi

  # Prepare prompts directory and merge base + project prompts (project overrides)
  if ! mkdir -p "$tmp_home/prompts"; then
    trap - EXIT INT TERM
    eval "$cleanup_cmd"
    print -u2 "co: failed to create $tmp_home/prompts"
    return 1
  fi

  local f
  for f in "$base_home/prompts"/*.md; do
    [ -f "$f" ] || continue
    if [ ! -e "$tmp_home/prompts/${f:t}" ]; then
      if ! cp "$f" "$tmp_home/prompts/" >/dev/null 2>&1; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "co: failed to copy base prompt $f"
        return 1
      fi
    fi
  done
  local project_prompts_dir=""
  if [ -d "$PWD/.agent-prompts" ]; then
    project_prompts_dir="$PWD/.agent-prompts"
  elif [ -d "$PWD/codex-prompts" ]; then
    project_prompts_dir="$PWD/codex-prompts"
  fi

  if [ -n "$project_prompts_dir" ]; then
    local copied_any=0
    for f in "$project_prompts_dir"/*.md; do
      [ -f "$f" ] || continue
      copied_any=1
      if ! cp -f "$f" "$tmp_home/prompts/" >/dev/null 2>&1; then
        trap - EXIT INT TERM
        eval "$cleanup_cmd"
        print -u2 "co: failed to copy project prompt $f"
        return 1
      fi
    done
    if (( copied_any )); then
      print -u2 "co: added project prompts from $project_prompts_dir (overriding base on conflicts)"
    fi
  fi

  local tmux_id
  if tmux_id=$(tmux display-message -p '#{session_id}::#{window_id}::#{pane_id}' 2>/dev/null); then
    if ! printf 'The TMUX_ID for this session will be "%s". Pass this id to the tracker mcp\n' "$tmux_id" >> "$tmp_home/AGENTS.md"; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to append tmux id to AGENTS.md"
      return 1
    fi
    print -u2 "co: recorded tmux id $tmux_id in AGENTS.md"
  else
    print -u2 "co: warning: unable to determine tmux id"
  fi

  if [ -n "$overlay_file" ]; then
    if ! printf '\n' >> "$tmp_home/config.toml" || ! cat "$overlay_file" >> "$tmp_home/config.toml"; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to append $overlay_file to temporary config"
      return 1
    fi
    print -u2 "co: appended MCP overlay from $overlay_file"
  fi

  codex_cmd+=("$@")
  CODEX_HOME="$tmp_home" "${codex_cmd[@]}"
  local exit_code=$?

  trap - EXIT INT TERM
  eval "$cleanup_cmd"
  return $exit_code
}
