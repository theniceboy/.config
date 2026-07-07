# Goals & Threads — Requirements

The unified goal/thread/todo management system that replaces the flat tracker panel (Alt-R) and integrates with the existing tmux + agent-tracker + opencode workflow.

## 1. Concept & Naming

| Term | Meaning |
|---|---|
| **Goal** | Nestable container (branch). E.g. "Ship v2", "Board UX". Can hold sub-goals and threads. |
| **Thread** | Leaf node = a window's current focused work unit. Has a name, status, blockers, and a todo checklist. One thread per window. |
| **Todo** | Checklist item under a thread ("what I'll do next"). The existing window-scoped todos. |
| **Turn** | The tracker's per-opencode-cycle record (renamed from "task" to avoid collision with the thread concept). |

### Core invariants
- Every opened tmux window that has started an opencode conversation **must** have a thread created and **not** deleted.
- The **only** way to delete a thread is to **close that tmux window** (not rename it).
- A thread is **keyed by a stable id**; `window_id` is an optional pointer (survives tmux restarts / resurrect).
- A thread is **born as a draft** (`◇ unnamed`, unassigned) the moment an opencode turn starts in a fresh window.
- A thread can be **window-less** (planned) — created manually before any work, or after unbinding.

## 2. Data Model

Store: `~/.cache/agent/goals.json` (file-based, like todos.json; the `agent` binary reads/writes it directly — no server changes).

```json
{
  "version": 1,
  "goals": {
    "ship-v2":  { "title": "Ship v2", "parent": null, "order": 0 },
    "board-ux": { "title": "Board UX", "parent": "ship-v2", "order": 0 }
  },
  "threads": {
    "th_01": { "name": "drag-drop", "goal_id": "board-ux", "blocked_by": [], "window_id": "@35", "done": false },
    "th_02": { "name": "release notes", "goal_id": "ship-v2", "blocked_by": ["th_01"], "window_id": null, "done": false }
  }
}
```

- **Threads are synthesized at view-time** by merging `goals.json` + live tracker state + tmux window list + `todos.json`. A window with tracker activity but no thread entry = an implicit draft.
- **Thread status is computed, not stored** (except `done`):
  - `running ◷` — opencode turn active right now (from tracker)
  - `ready` — not done, not blocked, not running
  - `blocked ⛔` — ≥1 blocker thread not done → shows `blocked ← name`
  - `done ✓` — manual (turns auto-finish, but a thread is done only when you say so)
  - `planned` — window-less, not done, not blocked
- A thread's todos = `todos.json.windows[window_id]` — already there, zero migration.

## 3. The Alt-R View (replaces the tracker panel)

Alt-S opens the palette; **Alt-R** switches to the goals/threads view inside it. (Alt-T stays the todo editor.) No top-level tmux Alt-R/Alt-T binds — they're palette-internal.

### Layout
- 2-line threads:
  - **Line 1:** name (bold) ········· status tag (right-aligned)
  - **Line 2:** `session / window · context` — session name + window name pulled **live from tmux** (auto-updating on rename), plus todo progress or blocker note or "p to promote"
- Goals: 1 line — `▾/▸ title`
- No telemetry: no window id, no turns count, no progress counts (no `4/7 done`).
- Todos **collapsed by default**; `→` expands a thread to show its todos inline (todos become navigable rows in the flat list).

### Status tags
`◷ running` (amber) · `ready` (green) · `⛔ blocked` (red) · `✓ done` (green, name dimmed) · `planned` (muted)

### Keys (Alt-R view)
| Key | Action |
|---|---|
| `u` / `e` | flat cursor up/down (through all rows; indentation is visual only) |
| `n` | jump to parent goal (convenience) |
| `→` / `t` | expand/collapse a thread's todos |
| `Enter` | goto the thread's tmux window (or **bind to current window** if planned/window-less) |
| `r` | rename thread (inline input) |
| `g` | set goal (fuzzy picker: existing goals + "create new") |
| `b` | set blocker (picks the thread that is **blocking** this one → adds to `blocked_by`) |
| `m` | enter move mode |
| `p` | promote a draft into a named thread (form) |
| `l` | toggle assigned ↔ unassigned view |
| `o` | more options (fuzzy searchable list of **all** actions) |
| `D` (shift) | **contextual**: on a goal → delete (cascade); on a thread → mark done |
| `a` | add a todo to the selected thread |
| `c` | toggle a todo (when cursor on a todo) |
| `y` | copy thread/todo title to clipboard |
| `Esc` | back to palette list |

### Move mode (`m`)
- `m` to enter; the moved item collapses (children hidden), replaced by a **ghost** at its current position.
- A dashed ghost row shows where the item will land under the current target.
- `u` / `e` move the ghost; `Enter` commits; `Esc` cancels.
- **No `n`/`i` promote/demote keys, no `Ctrl-u`/`Ctrl-e`.** Reordering is done only in move mode.

#### Move semantics (validated)
- Move toward a **goal** neighbor (same depth) → **nest as its first child** (depth+1).
- Move toward a **thread** neighbor (same depth) → **swap** positions, keep depth.
- At the **edge** of your parent (first child + `u`, or last child + `e`) → **pop out** one level (become your parent's sibling).
- Toward a **shallower** neighbor → pop out (depth-1, stay in slot).
- At **root** + `u` → blocked.
- Nesting into a goal that already has children → ghost becomes the **first** child (above existing children).

### Promote form (`p`, on a draft)
- **name** — prefilled from the first user message (editable).
- **goal** — fuzzy pick existing, or type new (supports `parent › child` to nest).
- **gates / blocked_by** — optional.
- Two tabs: **create new** / **bind to existing** planned thread.

### More options (`o`)
- A fuzzy, searchable list of every action (so you never have to memorize keys).
- Includes: add goal, add sub-goal, add thread, rename, set goal, set blocker, toggle done, bind/unbind window, goto, merge, delete (for planned threads / goals).

## 4. Delete Semantics

| Target | Key | Confirm | Behavior |
|---|---|---|---|
| **Goal** | `D` | type `yes` + Enter | **Cascade**: delete all sub-goals; **unassign** (not delete) all threads in the subtree. |
| **Thread** (window-bound) | — | — | **Cannot be manually deleted.** Auto-deletes only when the tmux window closes. |
| **Thread** (window-less/planned) | `o` → delete | `y`/`n` | Deleted (it has no window to close). |
| **Todo** | `d` / via `o` | none | Removed from the window's todo list. |

## 5. Thread Lifecycle

1. **Birth (automatic):** opencode turn starts in a fresh window → draft thread appears (`◇ unnamed`, unassigned). Tracked but no identity.
2. **Naming (automatic, via hook):** the draft name = the **first user message**, set by a **script + hook** (agent-agnostic — not the opencode plugin, so the agent runtime is swappable). The existing `jcode/hooks/tracker-hook.sh` is the canonical pattern; opencode's plugin dispatches to a shared shell script.
3. **Promotion (manual):** `p` on a draft → form (name + goal + optional blocker). Or bind to an existing planned thread.
4. **Done (manual):** `D` on a thread → `y`/`n` confirm. Thread is hidden from the active view (archived, not deleted).
5. **Revival:** if a done thread's window starts a new opencode turn, the thread **revives** (un-dones) and takes the **new** turn's name.
6. **Death:** the thread is deleted only when its tmux **window closes** (renaming the window does NOT delete the thread).

## 6. Status Bar (bottom-right)

New segment showing the **focused pane's** goal path + thread name:
```
⌖ Ship v2 › Board UX › drag-drop
```
- Empty if the focused window has no thread.
- No "active goal" concept — no `◉` marker, no manual pinning, no global "ready next" hint. The bar just reflects whatever pane you're on.
- Enabled by default as a `status_right` module (`goal`).

## 7. Prefix Keys (from any tmux pane, no palette needed)

| Prefix | Action |
|---|---|
| `C-s t` | rename the focused window's thread (inline command-prompt) |
| `C-s g` | set goal — fzf picker dialog with "create new" option |
| `C-s b` | set blocker — fzf picker dialog |

These call `agent goal rename-current / set-goal-current / set-blocker-current` (or `pick` + fzf). Same vocabulary as the in-view keys.

## 8. Todos

- **Window todos** live under threads (shown in Alt-R when expanded; also editable in Alt-T).
- **Global todos** stay in Alt-T (untouched) — lightweight scratch list.
- **Session todos: REMOVED.** The `sessions` map was dead code (the Alt-T UI never displayed them and had no creation path). Stop reading/writing it.

## 9. Resilience

- **tmux-resurrect:** window ids change on restore. `restore_agent_tracker_mapping.py` re-points `thread.window_id` to the new ids (matched by session+window name). `blocked_by` uses stable thread ids, so it survives unchanged.
- **Orphan reaping:** when the goals panel loads, threads bound to windows that no longer exist are deleted (along with their window todos). Planned (window-less) threads are preserved. This is resurrect-safe because the restore migration runs first.

## 10. What was dropped (explicitly decided against)

- ~~Active goal / focus lens~~ — no pinning, no filtering, no `◉` marker.
- ~~Progress counts~~ — no `4/7 done` on goals.
- ~~Promote/demote keys (`n`/`i`)~~ — `u`/`e` in move mode handle everything.
- ~~`Ctrl-u`/`Ctrl-e` sibling reorder~~ — reorder via move mode only.
- ~~Session todos~~ — removed.
- ~~"task" name for the leaf~~ — renamed to "thread"; tracker record renamed to "turn".
- ~~Telemetry in view~~ — no window id, no turns count.

## 11. Testing Approach

- **No unit tests.**
- Test by **manually running in a Docker container** and verifying the rendered output:
  - Linux image + tmux + zsh + Go + jq + python3 + fzf, agent-tracker source mounted.
  - Build in-container; run `tracker-server` directly.
  - **Fake agent:** a shell script that fires hook events (`turn_start` with a user message, `turn_end`) — no real opencode/API keys.
  - Drive: `tmux send-keys` to simulate keystrokes, `tmux capture-pane` to read what rendered.
  - Walk the exact scenarios: create goal, fire a fake turn → draft thread named from the message, promote it, each move-mode case, set blocker, mark done, cascade-delete a goal, check the status bar segment.
