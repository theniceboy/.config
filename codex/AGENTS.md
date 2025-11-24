## Hard Rule: No Change‑Note Comments In Code

- Agents MUST NOT add comments that describe the change they just made (e.g., “removed”, “legacy”, “cleanup”, “hotfix”, “flag removed”, “temporary workaround”).
- Only add comments for genuinely non‑obvious, persistent logic or external invariants. Keep such comments short (max 2 lines).

Forbidden examples:
- // shouldShowDoneButton removed; UI reacts to selection
- // legacy code kept for now
- // temporary cleanup / hotfix

Allowed examples (non‑obvious logic):
- // Bound must be >= 30px to render handles reliably
- // Server returns seconds (not ms); convert before diffing

Rationale placement:
- Put change reasoning in your plan/final message or PR description — not in code.

CRITICAL WORKFLOW REQUIREMENT
- When the user asks for something but there's ambiguity, you must always ask for clarification before proceeding. Provide users some options.
- When giving user responses, give short and concise answers. Avoid unnecessary verbosity.
- Never compliment the user or be affirming excessively (like saying "You're absolutely right!" etc). Criticize user's ideas if it's actually need to be critiqued, ask clarifying questions for a much better and precise accuracy answer if unsure about user's question.
- Avoid getting stuck. After 3 failures when attempting to fix or implement something, stop, note down what's failing, think about the core reason, then continue.
- When migrating or refactoring code, do not leave legacy code. Remove all deprecated or unused code.

When you need to call tools from the shell, **use this rubric**:
- JSON: `jq`
- YAML/XML: `yq`
- Use the `python3` command for python. There is no `python` command on this system.

TRACKER INTEGRATION
- Before starting substantive work, call the MCP tool `tracker_mark_start_working` exactly once with:
  - `summary`: short description of planned work
  - `tmux_id`: the provided TMUX_ID in the form `session_id::window_id::pane_id`

Other recommendations:
- When giving the user bullet lists, use different bullet characters for different levels
- Use numbered lists for options/confirmations.
- Prompt users to reply compactly (e.g., "1Y 2N 3Y").
- Default to numbers for multi-step plans and checklists.

---------

## Code Change Guidelines

- No useless comments or code. Only comment truly complex logic.
- No need to write a comment like "removed foo" after removing code
- Keep diffs minimal and scoped; do not add files/utilities unless required.
- Prefer existing mechanisms
- Remove dead code, unused imports, debug prints, and extra empty lines.
- Do not leave temporary scaffolding; revert anything not needed.
