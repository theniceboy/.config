_cl_run() {
  local tag="$1"
  shift

  local base_home="${XDG_CONFIG_HOME:-$HOME/.config}/claude"
  local opencode_home="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"

  setopt local_options null_glob

  local -a extra_args=(--dangerously-skip-permissions --model opus)

  # Build system prompt from AGENTS.md files
  local system_prompt=""
  if [ -f "$opencode_home/AGENTS.md" ]; then
    system_prompt=$(cat "$opencode_home/AGENTS.md")
  fi
  if [ -f "$PWD/AGENTS.md" ]; then
    system_prompt="${system_prompt}
$(cat "$PWD/AGENTS.md")"
    print -u2 "$tag: appended project AGENTS.md"
  fi
  if [ -n "$system_prompt" ]; then
    extra_args+=(--append-system-prompt "$system_prompt")
  fi

  # Build MCP config
  local mcp_config=""
  if [ -f "$base_home/mcpServers.json" ] && command -v jq >/dev/null 2>&1; then
    mcp_config=$(jq -c '.mcpServers // {}' "$base_home/mcpServers.json" 2>/dev/null)
    if [ "$mcp_config" = "{}" ]; then
      mcp_config=""
    else
      print -u2 "$tag: loaded MCP servers from mcpServers.json"
    fi
  fi

  # Merge MCP from project opencode.json (transform format)
  local opencode_project_config="$PWD/opencode.json"
  if [ -f "$opencode_project_config" ] && command -v jq >/dev/null 2>&1; then
    local project_mcp=$(jq -c '.mcp // {}' "$opencode_project_config" 2>/dev/null)
    if [ -n "$project_mcp" ] && [ "$project_mcp" != "{}" ]; then
      local transformed_mcp=$(echo "$project_mcp" | jq -c '
        to_entries | map({
          key: .key,
          value: (
            if .value.command | type == "array" then
              {
                type: "stdio",
                command: .value.command[0],
                args: .value.command[1:],
                env: (.value.environment // .value.env // {})
              }
            else
              .value
            end
          )
        }) | from_entries
      ')
      if [ -n "$mcp_config" ]; then
        mcp_config=$(echo "$mcp_config" "$transformed_mcp" | jq -sc '.[0] + .[1]')
      else
        mcp_config="$transformed_mcp"
      fi
      print -u2 "$tag: merged MCP servers from project opencode.json"
    fi
  fi

  if [ -n "$mcp_config" ]; then
    local mcp_file="${TMPDIR:-/tmp}/claude-mcp-$$.json"
    printf '%s\n' "{\"mcpServers\": $mcp_config}" > "$mcp_file"
    extra_args+=(--mcp-config "$mcp_file")
    trap "rm -f '$mcp_file'" EXIT INT TERM
  fi

  # Create temp symlink for project commands if .opencode/command exists
  local created_claude_dir=0
  local created_commands_link=0
  if [ -d "$PWD/.opencode/command" ]; then
    if [ ! -d "$PWD/.claude" ]; then
      mkdir -p "$PWD/.claude"
      created_claude_dir=1
    fi
    if [ ! -e "$PWD/.claude/commands" ]; then
      ln -s "$PWD/.opencode/command" "$PWD/.claude/commands"
      created_commands_link=1
      print -u2 "$tag: linked .opencode/command -> .claude/commands"
    fi
  fi

  claude "$@" "${extra_args[@]}"
  local exit_code=$?

  # Cleanup temp symlink
  if [ "$created_commands_link" = 1 ]; then
    rm -f "$PWD/.claude/commands"
  fi
  if [ "$created_claude_dir" = 1 ]; then
    rmdir "$PWD/.claude" 2>/dev/null
  fi

  return $exit_code
}
