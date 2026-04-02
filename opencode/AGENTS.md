# CRITICAL WORKFLOW REQUIREMENT
- You MUST NOT add comments that describe the change they just made (e.g., “removed”, “legacy”, “cleanup”, “hotfix”, “flag removed”, “temporary workaround”).
- Only add comments for genuinely non‑obvious, persistent logic or external invariants. Keep such comments short (max 2 lines).
- When migrating or refactoring code, do not leave legacy code. Remove all deprecated or unused code.
- Put change reasoning in your plan/final message — not in code.

## Work Summary

- `set_work_summary` is mandatory protocol, not a suggestion.
- Call `set_work_summary` at least once at the start of every busy turn before any substantive tool call, code change, research step, or substantive user-facing response.
- Prefer calling it with both fields: `set_work_summary({ theme: "...", now: "..." })`.
- `theme` answers: what is this pane about overall? Keep it stable across many turns.
- `now` answers: what are you about to do next? Update it whenever the next concrete step changes.
- Keep both labels concrete and under 48 characters.
- Since the summary line has dedicated space, prefer richer phrases that help a forgetful human re-orient instantly.
- Good `theme` examples: `Tmux status summary workflow`, `Agent tracker integration`, `Flutter auth onboarding`.
- Good `now` examples: `Patch summary enforcement`, `Read restore path handling`, `Wait for user reply`.
- Bad labels: `Working`, `Coding`, `Debugging`, `Researching`, `Task`, `Fixing stuff`.
- Bad `now` phrasing: `Debugging summary enforcement`, `Reading restore path handling`, `Waiting on user reply`.
- If you are blocked or waiting, keep the `theme` and change `now`, for example `Wait for user reply` or `Wait for tests`.
- If the labels are missing or stale, stop and update them first.
- Repeating the same `theme` across turns is acceptable when the overall mission has not changed.

---------

## Adaptive Burst Workflow

### How to Burst

- Trigger bursts only when needed; otherwise continue normal execution.
- Choose burst size by complexity:
  - low: 2 subagents
  - medium: 3 subagents
  - high/risky: 4-5 subagents
- Use one burst round by default.
- Run a second round only if confidence is still low.
- Assign non-overlapping scopes to reduce duplicate findings.

### What to Burst

- `discover-locator`: locate relevant files, symbols, and entry points.
- `discover-xref`: map defs/usages/callers/callees.
- `discover-flow`: trace execution or data flow paths.
- `discover-blast`: map direct and indirect impact surface.

### When to Burst

- Unfamiliar code area.
- Multiple plausible implementation paths.
- Unclear failure/root cause after initial inspection.
- Cross-cutting change touching multiple modules.
- High-impact change with regression risk.

### When Not to Burst

- Straightforward single-file changes.
- Clear path with high confidence.
- Small, low-risk, reversible changes.

### Burst Output Contract

Each discovery subagent returns compact, evidence-based output:

- `scope`: what was inspected
- `findings`: claim + `path:line` evidence + confidence
- `unknowns`: unresolved gaps

Limit each subagent to maximum 5 findings.
