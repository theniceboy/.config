co() {
  local -a codex_cmd
  codex_cmd=(codex --sandbox danger-full-access -m gpt-5-codex -c 'model_reasoning_summary_format=experimental' -c 'model_reasoning_effort=medium' --search)
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

  local entry name
  for entry in "$base_home"/* "$base_home"/.[!.]* "$base_home"/..?*; do
    if [ ! -e "$entry" ]; then
      continue
    fi
    name="${entry##*/}"
    case "$name" in
      '.'|'..'|'config.toml'|'AGENTS.md')
        continue
        ;;
    esac
    if ! ln -s "$entry" "$tmp_home/$name"; then
      trap - EXIT INT TERM
      eval "$cleanup_cmd"
      print -u2 "co: failed to symlink $entry"
      return 1
    fi
  done
  print -u2 "co: prepared $tmp_home with copies of config.toml and AGENTS.md; symlinked remaining entries"

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
