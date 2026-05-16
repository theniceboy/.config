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

  if command -v jq >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then
    local rapid_models_json rapid_model_id rapid_model_limit tmp_config_json tmp_context tmp_output tmp_name
    rapid_models_json=$(curl -fsS "http://127.0.0.1:8000/v1/models" 2>/dev/null || true)
    rapid_model_id=$(printf '%s' "$rapid_models_json" | jq -r '.data[0].id // empty' 2>/dev/null || true)

    if [ -n "$rapid_model_id" ] && [ -f "$rapid_model_id/config.json" ]; then
      tmp_context=$(jq -r '
        .text_config.max_position_embeddings //
        .language_model.max_position_embeddings //
        .language_config.max_position_embeddings //
        .max_position_embeddings //
        empty
      ' "$rapid_model_id/config.json" 2>/dev/null || true)

      if [ -n "$tmp_context" ] && [ "$tmp_context" != "null" ]; then
        tmp_output=8192
        tmp_name=${rapid_model_id:t}
        tmp_config_json="$tmp_home/opencode.json.tmp"

        if jq \
          --arg name "$tmp_name" \
          --argjson context "$tmp_context" \
          --argjson output "$tmp_output" \
          '
            .provider.rapidmlx.models.default.name = $name |
            .provider.rapidmlx.models.default.limit.context = $context |
            .provider.rapidmlx.models.default.limit.output = $output
          ' "$tmp_home/opencode.json" > "$tmp_config_json" 2>/dev/null; then
          mv "$tmp_config_json" "$tmp_home/opencode.json"
          print -u2 "$tag: synced rapidmlx/default to $tmp_name (context $tmp_context, output $tmp_output)"
        fi
      fi
    fi
  fi

  local agent_workspace=""
  local agent_feature=""
  local agent_browser_url=""
  local agent_search_dir="$PWD"
  while [ -n "$agent_search_dir" ] && [ "$agent_search_dir" != "/" ]; do
    if [ -f "$agent_search_dir/agent.json" ]; then
      agent_workspace="$agent_search_dir"
      break
    fi
    if [ "${agent_search_dir:t}" = "repo" ] && [ -f "${agent_search_dir:h}/agent.json" ]; then
      agent_workspace="${agent_search_dir:h}"
      break
    fi
    agent_search_dir="${agent_search_dir:h}"
  done

  if [ -n "$agent_workspace" ] && command -v jq >/dev/null 2>&1; then
    local agent_json="$agent_workspace/agent.json"
    local agent_device
    agent_device=$(jq -r '.device // empty' "$agent_json" 2>/dev/null || true)
    agent_browser_url=$(jq -r '.url // empty' "$agent_json" 2>/dev/null || true)
    agent_feature=$(jq -r '.feature // empty' "$agent_json" 2>/dev/null || true)
    if [ "$agent_device" = "web-server" ] && [ -n "$agent_browser_url" ]; then
      local agent_bin="${AGENT_BIN:-$HOME/.config/agent-tracker/bin/agent}"
      local tmp_config_json="$tmp_home/opencode.json.tmp"
      if jq \
        --arg agent_bin "$agent_bin" \
        --arg workspace "$agent_workspace" \
        --arg feature "$agent_feature" \
        --arg url "$agent_browser_url" \
        '
          .mcp = (.mcp // {}) |
          .mcp.agent_browser = {
            type: "local",
            command: [$agent_bin, "browser", "mcp", "--workspace", $workspace],
            environment: {
              AGENT_WORKSPACE: $workspace,
              AGENT_FEATURE: $feature,
              AGENT_BROWSER_URL: $url
            },
            enabled: true,
            timeout: 10000
          }
        ' "$tmp_home/opencode.json" > "$tmp_config_json" 2>/dev/null; then
        mv "$tmp_config_json" "$tmp_home/opencode.json"
        print -u2 "$tag: added agent_browser MCP for ${agent_feature:-$agent_workspace}"
      else
        rm -f "$tmp_config_json"
        print -u2 "$tag: failed to add agent_browser MCP for $agent_workspace"
      fi
    fi
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

  mkdir -p "$tmp_home/plugins"
  local f
  for f in "$base_home/plugins"/* "$base_home/tui-plugins"/*; do
    [ -e "$f" ] || continue
    ln -s "$f" "$tmp_home/plugins/" 2>/dev/null
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

  OPENCODE_CONFIG_DIR="$tmp_home" \
    AGENT_WORKSPACE="${agent_workspace:-${AGENT_WORKSPACE:-}}" \
    AGENT_FEATURE="${agent_feature:-${AGENT_FEATURE:-}}" \
    AGENT_BROWSER_URL="${agent_browser_url:-${AGENT_BROWSER_URL:-}}" \
    OP_TRACKER_NOTIFY="${OP_TRACKER_NOTIFY:-0}" \
    RIPGREP_CONFIG_PATH="${RIPGREP_CONFIG_PATH:-$HOME/.ripgreprc}" \
    "${opencode_cmd[@]}"
  local exit_code=$?

  trap - EXIT INT TERM
  eval "$cleanup_cmd"
  return $exit_code
}
