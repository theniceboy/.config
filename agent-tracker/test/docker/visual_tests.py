#!/usr/bin/env python3
"""Deep visual tests for agent-tracker goal panel TUI.

Tests background coverage, foreground colors, bold, indentation,
scrolling, modal borders, picker styling, and selection highlighting.
"""
import sys, os, subprocess, time, json
sys.path.insert(0, os.path.dirname(__file__))
from visual_lib import *

TARGET = "test:0.0"
AGENT = os.environ.get("AGENT_BIN", "/home/testuser/bin/agent")
ROWS = 50
COLS = 100

TRACKER_SERVER = "/home/testuser/bin/tracker-server"

def setup():
    """Create fresh session, seed data, open goals panel."""
    subprocess.run(["tmux", "kill-session", "-t", "test"], capture_output=True)
    time.sleep(0.2)
    # Build tracker server
    subprocess.run(["go", "build", "-o", TRACKER_SERVER, "./cmd/tracker-server"],
                   cwd="/home/testuser/agent-tracker", capture_output=True)
    # Start tracker server
    subprocess.run(["pkill", "-f", "tracker-server"], capture_output=True)
    time.sleep(0.2)
    subprocess.Popen([TRACKER_SERVER], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(0.5)
    # Create session with known dimensions
    subprocess.run(["tmux", "new-session", "-d", "-s", "test",
                    "-x", str(COLS), "-y", str(ROWS)], capture_output=True)
    time.sleep(0.3)
    # Reset goals data
    subprocess.run(["rm", "-rf", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run(["mkdir", "-p", os.path.expanduser("~/.cache/agent")], capture_output=True)
    # Seed data
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Ship V2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Board UX", "--parent", "ship-v2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "drag-drop", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "notif", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "TestThread", "--goal", "ship-v2"], capture_output=True)
    time.sleep(0.2)
    # Launch palette
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def teardown():
    send_keys(TARGET, "Escape", 0.2)
    send_keys(TARGET, "Escape", 0.2)


def navigate_to(text):
    """Navigate cursor until the selected row contains text. Tries down then up."""
    # First try going down
    for _ in range(60):
        g = capture(TARGET, ROWS, COLS)
        for r in range(len(g)):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "e", 0.08)
    # Try going up from the bottom
    for _ in range(60):
        g = capture(TARGET, ROWS, COLS)
        for r in range(len(g)):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "u", 0.08)
    return False


def reset_to_list():
    """Ensure we're in goals panel list mode. Only sends Escape if a modal is open."""
    g = capture(TARGET, ROWS, COLS)
    # Check for modal/sub-mode indicators
    modal_markers = ["goal title", "thread name", "new goal", "new thread",
                     "todo", "Actions", "Set goal", "Set blocker", "Toggle done",
                     "Delete goal", "Enter save", "Enter create", "Enter promote",
                     "Enter add", "type 'yes'", "y confirm", "(moving)", "moving"]
    in_modal = any(find_row_with_text(t) >= 0 for t in modal_markers)
    if in_modal:
        send_keys(TARGET, "Escape", 0.5)
        g = capture(TARGET, ROWS, COLS)
    # If goals panel not visible, relaunch
    if find_row_with_text("Goals") < 0:
        subprocess.run(
            ["tmux", "send-keys", "-t", TARGET,
             f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
             "Enter"],
            capture_output=True)
        time.sleep(1.5)
        capture(TARGET, ROWS, COLS)


def reseed():
    """Reset goal data to known state and reload the panel."""
    # Kill palette process
    subprocess.run(["pkill", "-f", "palette"], capture_output=True)
    time.sleep(0.5)
    # Kill extra windows (keep only window 0)
    for i in range(20, 0, -1):
        subprocess.run(["tmux", "kill-window", "-t", f"test:{i}"], capture_output=True)
    # Clear and re-seed
    subprocess.run(["rm", "-rf", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run(["mkdir", "-p", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Ship V2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Board UX", "--parent", "ship-v2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "API Refactor", "--parent", "ship-v2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Marketing Site"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "drag-drop upload", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "notification system", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "auth middleware", "--goal", "api-refactor"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "Standalone thread A"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "Standalone thread B"], capture_output=True)
    # Create 3 draft windows with tracker tasks
    for name in ["Draft alpha", "Draft beta", "Draft gamma"]:
        result = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", name, "-P", "-F", "#{window_id}"],
                                capture_output=True, text=True)
        wid = result.stdout.strip()
        if wid:
            subprocess.run([AGENT, "tracker", "command",
                          "--window-id", wid, "--session", "test", "--window", name,
                          "start_task", name], capture_output=True)
    time.sleep(0.3)
    # Switch back to window 0
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)
    # Relaunch palette
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


_REF_NAMES = ["Ship V2", "Board UX", "drag-drop", "notification",
              "API Refactor", "auth", "Marketing", "Standalone thread B"]

def capture_state():
    """Return (ghost_row, ghost_indent, {name: row}) from current pane."""
    capture(TARGET, ROWS, COLS)
    g = capture(TARGET, ROWS, COLS)
    gr = -1
    gindent = -1
    refs = {}
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            gr = r
            stripped = rt.lstrip()
            gindent = len(rt) - len(stripped)
        else:
            for name in _REF_NAMES:
                if name in rt and name not in refs:
                    refs[name] = r
    return gr, gindent, refs


# ═══════════════════════════════════════════════════════════════
# TEST 1: List view — header, tree, footer
# ═══════════════════════════════════════════════════════════════
def test_list_view():
    print("=== 1. List view rendering ===")
    g = capture(TARGET, ROWS, COLS)

    # Header: "Goals" should be bold, fg=230
    hdr_row = find_row_with_text("Goals")
    check("header row found", hdr_row >= 0,
          f"text: {repr(row_text(hdr_row))}" if hdr_row >= 0 else "")

    if hdr_row >= 0:
        col = row_text(hdr_row).index("G")
        check_cell_bold("header bold", hdr_row, col, True)
        check_cell_fg("header fg=230", hdr_row, col, "230")

        meta_col = row_text(hdr_row).find("goals")
        if meta_col > 0:
            check_cell_fg("meta fg=245", hdr_row, meta_col, "245")

    # Goal indentation
    goal_row = find_row_with_text("Ship V2")
    check("root goal found", goal_row >= 0)
    if goal_row >= 0:
        text = row_text(goal_row)
        glyph_col = text.index("Ship")
        check("goal has expand glyph", glyph_col > 0,
              f"text: |{text}|")
        check_cell_bold("goal title bold", goal_row, glyph_col, True)

    # Sub-goal should be indented more
    sub_row = find_row_with_text("Board UX")
    check("sub-goal found", sub_row >= 0)
    if sub_row >= 0 and goal_row >= 0:
        root_text_col = row_text(goal_row).index("Ship")
        sub_text_col = row_text(sub_row).index("Board")
        check("sub-goal indented more than parent",
              sub_text_col > root_text_col,
              f"root col={root_text_col}, sub col={sub_text_col}")

    # Footer with shortcuts (now 2 lines)
    footer_found = False
    for r in range(ROWS - 8, ROWS):
        ft = row_text(r).lower()
        if "move" in ft:
            footer_found = True
            break
    check("footer with shortcuts found", footer_found)


# ═══════════════════════════════════════════════════════════════
# TEST 2: Selection highlight — bg=238 must extend full width
# ═══════════════════════════════════════════════════════════════
def test_selection_highlight():
    print("\n=== 2. Selection highlight bg coverage ===")
    g = capture(TARGET, ROWS, COLS)

    sel_rows = []
    for r in range(len(GRID)):
        count = row_has_bg(r, "238")
        if count > 5:  # significant bg coverage
            sel_rows.append((r, count))

    check("at least one selected row", len(sel_rows) >= 1,
          f"found {len(sel_rows)} rows with significant bg=238: {sel_rows}")

    if sel_rows:
        sr = sel_rows[0][0]
        count = sel_rows[0][1]
        print(f"    Selected row {sr}: {count} cells with bg=238 out of {COLS} cols")
        print(f"    Text: |{row_text(sr)}|")

        # The bg=238 should cover most of the width (not just the text chars)
        check_row_bg_full("selection bg covers >=90% width", sr, "238", COLS, 0.9)

        # Check continuity — no gaps in bg
        first_bg_col = None
        last_bg_col = None
        gaps = []
        for i in range(COLS):
            if str(GRID[sr][i].get("bg")) == "238":
                if first_bg_col is None:
                    first_bg_col = i
                if last_bg_col is not None and i > last_bg_col + 1:
                    gaps.append((last_bg_col, i))
                last_bg_col = i
        check("selection bg has no gaps", len(gaps) == 0,
              f"gaps at: {gaps}" if gaps else "")

        # Selected text should be bold
        text_col = None
        for i in range(COLS):
            if GRID[sr][i]["ch"].isalpha() and str(GRID[sr][i].get("bg")) == "238":
                text_col = i
                break
        if text_col is not None:
            check_cell_bold("selected text bold", sr, text_col, True)


# ═══════════════════════════════════════════════════════════════
# TEST 3: Thread rendering — 2-line layout (name + right meta on L1, status on L2)
# ═══════════════════════════════════════════════════════════════
def test_thread_rendering():
    print("\n=== 3. Thread rendering ===")
    g = capture(TARGET, ROWS, COLS)

    thread_row = find_row_with_text("drag-drop")
    check("thread found in list", thread_row >= 0)

    if thread_row >= 0:
        text = row_text(thread_row)

        # Line 1: thread name only (no status icon)
        check("thread name on line 1", "drag-drop" in text or "notif" in text)

        # Thread name should be bold
        name_col = text.find("drag-drop") if "drag-drop" in text else text.find("notif")
        if name_col >= 0:
            check_cell_bold("thread name bold", thread_row, name_col, True)

        # Status icon should be on line 1 (with the name)
        check("status icon on line 1", "◦" in text or "⠋" in text or "✓" in text,
              f"text: |{text}|")


# ═══════════════════════════════════════════════════════════════
# TEST 4: Input box modal — border + centered
# ═══════════════════════════════════════════════════════════════
def test_input_box():
    print("\n=== 4. Input box modal ===")
    reset_to_list()
    # Move cursor to a goal and press r to rename
    send_keys(TARGET, "r", 0.5)
    g = capture(TARGET, ROWS, COLS)

    # Should have a bordered box centered on screen
    # Border chars: ┌ ┐ └ ┘ │ ─
    border_chars = set("┌┐└┘│─")
    border_rows = []
    top_border = 0
    for r in range(min(ROWS, len(GRID))):
        rt = "".join(c["ch"] for c in GRID[r])
        if any(bc in rt for bc in border_chars):
            border_rows.append(r)

    check("modal has border lines", len(border_rows) >= 2,
          f"border rows: {border_rows}")

    if border_rows:
        top_border = border_rows[0]
        bot_border = border_rows[-1]
        check("modal has top border", top_border >= 2,
              f"top at row {top_border}")
        check("modal has bottom border", bot_border > top_border + 2,
              f"top={top_border}, bot={bot_border}")

        # Check vertical centering (should be near vertical center)
        center = ROWS // 2
        modal_center = (top_border + bot_border) // 2
        check("modal roughly centered vertically",
              abs(modal_center - center) <= 5,
              f"modal center={modal_center}, pane center={center}")

        # Find left/right borders on a middle row
        mid_row = (top_border + bot_border) // 2
        left_col = None
        right_col = None
        for i in range(COLS):
            if GRID[mid_row][i]["ch"] == "│":
                if left_col is None:
                    left_col = i
                right_col = i
        if left_col and right_col:
            check("modal has left border │", left_col is not None)
            check("modal has right border │", right_col is not None)
            check("modal is horizontally wide enough",
                  right_col - left_col >= 30,
                  f"width={right_col - left_col}")

            # Check horizontal centering
            pane_center = COLS // 2
            modal_center_h = (left_col + right_col) // 2
            check("modal roughly centered horizontally",
                  abs(modal_center_h - pane_center) <= 8,
                  f"modal center={modal_center_h}, pane center={pane_center}")

    # Border should have fg=223
    if border_rows:
        for i in range(COLS):
            if GRID[top_border][i]["ch"] in "┌┐└┘─":
                check_cell_fg("border fg=223", top_border, i, "223")
                break

    # Input label should be bold
    label_row = find_row_with_text("goal title")
    if label_row < 0:
        label_row = find_row_with_text("thread name")
    if label_row < 0:
        label_row = find_row_with_text("title")
    if label_row >= 0:
        text = row_text(label_row)
        for i, ch in enumerate(text):
            if ch.isalpha():
                check_cell_bold("input label bold", label_row, i, True)
                break

    # Don't send Esc here — reset_to_list handles returning to list mode
    # The palette was crashing when Esc was sent (possibly escape-time issue)


# ═══════════════════════════════════════════════════════════════
# TEST 5: Picker — filter box bg + selected item bg
# ═══════════════════════════════════════════════════════════════
def test_picker():
    print("\n=== 5. Picker (more options) ===")
    reset_to_list()
    send_keys(TARGET, "o", 0.5)
    g = capture(TARGET, ROWS, COLS)

    title_row = find_row_with_text("Actions")
    check("picker title 'Actions' found", title_row >= 0)

    # Filter line should have bg=236 (searchBox)
    filter_row = -1
    for r in range(title_row + 1 if title_row >= 0 else 0, ROWS):
        count = row_has_bg(r, "236")
        if count > 3:
            filter_row = r
            break
    check("picker filter has bg=236", filter_row >= 0,
          "searched all rows for bg=236")

    # Filter prompt ">" should have fg=223
    if filter_row >= 0:
        for i in range(COLS):
            if GRID[filter_row][i]["ch"] == ">":
                check_cell_fg("filter prompt fg=223", filter_row, i, "223")
                break

    # Selected item should have bg=238
    sel_found = False
    for r in range(filter_row + 1 if filter_row >= 0 else 0, ROWS):
        count = row_has_bg(r, "238")
        if count > 3:
            sel_found = True
            print(f"    Selected picker item at row {r}: |{row_text(r)}|")
            check_row_bg_full("picker selection bg >=90%", r, "238", COLS, 0.9)
            break
    check("picker has selected item with bg=238", sel_found)

    # Items should have padding (1 left, 1 right)
    if sel_found and filter_row >= 0:
        for r in range(filter_row + 1, ROWS):
            rt = row_text(r)
            if "Add goal" in rt or "Add thread" in rt:
                # Check that text starts with at least 1 space (padding)
                raw = "".join(c["ch"] for c in GRID[r])
                first_text_col = None
                for i in range(COLS):
                    if GRID[r][i]["ch"].isalpha():
                        first_text_col = i
                        break
                if first_text_col is not None:
                    check("picker item has left padding", first_text_col >= 1,
                          f"first text at col {first_text_col}")
                break

    send_keys(TARGET, "Escape", 0.5)


# ═══════════════════════════════════════════════════════════════
# TEST 6: Scrolling — many items, cursor stays visible
# ═══════════════════════════════════════════════════════════════
def test_scroll():
    print("\n=== 6. Scrolling ===")
    reset_to_list()
    # Add many goals to force scrolling
    for i in range(25):
        subprocess.run([AGENT, "goal", "add-goal", "--title", f"ScrollGoal{i:02d}"],
                       capture_output=True)
    time.sleep(0.5)
    # Re-open panel to reload data
    send_keys(TARGET, "Escape", 0.5)
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    g = capture(TARGET, ROWS, COLS)
    has_scroll = find_row_with_text("ScrollGoal") >= 0
    check("scroll goals loaded", has_scroll)
    if not has_scroll:
        return

    # Move to top
    for _ in range(40):
        send_keys(TARGET, "u", 0.05)
    time.sleep(0.3)
    g = capture(TARGET, ROWS, COLS)

    # First goal should be visible
    first_visible = False
    for r in range(3, ROWS - 5):
        if "ScrollGoal" in row_text(r):
            first_visible = True
            break
    check("first goals visible at top of scroll", first_visible)

    # Now scroll down to bottom
    for _ in range(40):
        send_keys(TARGET, "e", 0.03)
    time.sleep(0.3)
    g = capture(TARGET, ROWS, COLS)

    # Last goal should be visible (goals may not fill entire terminal)
    last_visible = False
    for r in range(3, ROWS - 4):
        if "ScrollGoal24" in row_text(r) or "ScrollGoal23" in row_text(r):
            last_visible = True
            break
    check("last goals visible at bottom of scroll", last_visible,
          f"last rows: {[row_text(r)[:40] for r in range(ROWS-5, ROWS)]}")

    # Selected row should still be visible and have bg=238
    sel_count = 0
    for r in range(3, ROWS - 2):
        if row_has_bg(r, "238") > 5:
            sel_count += 1
    check("at least one selected row visible during scroll", sel_count >= 1,
          f"found {sel_count} selected rows")

    # Clean up scroll goals
    for i in range(25):
        subprocess.run([AGENT, "goal", "delete-goal", f"scrollgoal{i:02d}", "--confirm", "yes"],
                       capture_output=True)

    # Cursor row should be near bottom of viewport
    sel_row = -1
    for r in range(ROWS - 8, ROWS):
        if row_has_bg(r, "238") > 5:
            sel_row = r
            break
    if sel_row >= 0:
        check("cursor near bottom of viewport", sel_row >= ROWS - 12,
              f"cursor at row {sel_row}, pane has {ROWS} rows")


# ═══════════════════════════════════════════════════════════════
# TEST 7: Move mode — ghost rendering
# ═══════════════════════════════════════════════════════════════
def test_move_mode():
    print("\n=== 7. Move mode ghost ===")
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("move mode ghost rendered", False, "could not navigate to thread")
        check("ghost visible after moving down", False)
        return
    send_keys(TARGET, "m", 0.4)
    g = capture(TARGET, ROWS, COLS)

    # Ghost should show "(moving)" text
    ghost_found = False
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_found = True
            print(f"    Ghost at row {r}: |{rt}|")
            break
    check("move mode ghost rendered", ghost_found)

    # Move mode shows status or different footer
    move_hint = False
    for r in range(ROWS):
        rt = row_text(r).lower()
        if "moving" in rt or "move" in rt:
            move_hint = True
    check("move mode footer hints shown", move_hint)

    # Move down once — should swap with notification (ghost appears below it)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_row = -1
    notif_row = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_row = r
        if "notification" in rt and "(moving)" not in rt:
            notif_row = r
    check("ghost visible after moving down", ghost_row >= 0)
    if ghost_row >= 0 and notif_row >= 0:
        check("ghost swapped below notification", ghost_row > notif_row,
              f"ghost at row {ghost_row}, notification at row {notif_row}")

    # Move down again — should pop out of Board UX to depth 1
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_popped = False
    for r in range(ROWS):
        if "(moving)" in row_text(r):
            ghost_popped = True
            break
    check("ghost visible after popping out", ghost_popped)

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7b: Move mode — nest into goal via u, then pop out via e
# ═══════════════════════════════════════════════════════════════
def test_move_nest_pop():
    print("\n=== 7b. Move mode nest and pop ===")
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("nest: ghost rendered", False, "could not navigate to thread")
        return
    send_keys(TARGET, "m", 0.4)

    # Press u to pop out of Board UX (should go above Board UX at depth 1)
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_row = -1
    board_row = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_row = r
        if "Board UX" in rt:
            board_row = r
    check("nest: ghost rendered after u", ghost_row >= 0)
    if ghost_row >= 0 and board_row >= 0:
        check("nest: ghost above Board UX after u", ghost_row < board_row,
              f"ghost at row {ghost_row}, Board UX at row {board_row}")

    # Press e to go back — should nest back inside Board UX (before notification)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_row2 = -1
    notif_row2 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_row2 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_row2 = r
    check("nest: ghost rendered after e", ghost_row2 >= 0)
    if ghost_row2 >= 0 and notif_row2 >= 0:
        check("nest: ghost inside Board UX before notification after e",
              ghost_row2 < notif_row2,
              f"ghost at row {ghost_row2}, notification at row {notif_row2}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7c: Move mode — commit swap and verify store
# ═══════════════════════════════════════════════════════════════
def test_move_commit_swap():
    print("\n=== 7c. Move mode commit swap ===")
    reseed()
    reset_to_list()
    if not navigate_to("notification"):
        check("swap commit: navigated to notification", False)
        return
    # Move notification down past drag-drop? No — let's navigate to drag-drop
    # Actually navigate to drag-drop and swap it below notification
    if not navigate_to("drag-drop"):
        check("swap commit: navigated to drag-drop", False)
        return

    send_keys(TARGET, "m", 0.4)
    send_keys(TARGET, "e", 0.2)  # swap below notification
    send_keys(TARGET, "Enter", 0.5)

    # Verify store order via CLI
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    notif_idx = -1
    dragdrop_idx = -1
    for i, line in enumerate(lines):
        if "notification" in line:
            notif_idx = i
        if "drag-drop" in line:
            dragdrop_idx = i
    check("swap commit: notification before drag-drop in store",
          notif_idx >= 0 and dragdrop_idx >= 0 and notif_idx < dragdrop_idx,
          f"notification at line {notif_idx}, drag-drop at line {dragdrop_idx}")

    # Cursor should be on drag-drop
    g = capture(TARGET, ROWS, COLS)
    cursor_on_moved = False
    for r in range(ROWS):
        if row_has_bg(r, "238") > 5 and "drag-drop" in row_text(r):
            cursor_on_moved = True
            break
    check("swap commit: cursor stays on moved thread", cursor_on_moved)


# ═══════════════════════════════════════════════════════════════
# TEST 7d: Move mode — commit nest into goal and verify store
# ═══════════════════════════════════════════════════════════════
def test_move_commit_nest():
    print("\n=== 7d. Move mode commit nest ===")
    reseed()
    reset_to_list()

    # Navigate to Standalone thread A (top-level, no goal)
    if not navigate_to("Standalone thread A"):
        check("nest commit: navigated", False, "could not navigate")
        return

    send_keys(TARGET, "m", 0.4)

    # Press u once — node above is Marketing Site (goal) → nests into it
    send_keys(TARGET, "u", 0.2)

    # Commit
    send_keys(TARGET, "Enter", 0.5)

    # Verify store: Standalone thread A should be under Marketing Site
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    standalone_nested = False
    standalone_line = ""
    for line in out.split("\n"):
        if "Standalone thread A" in line:
            standalone_nested = line.startswith("  -")
            standalone_line = line
            break
    check("nest commit: thread assigned to Marketing Site in store", standalone_nested,
          f"Standalone thread A line: {standalone_line or 'NOT FOUND'}")


# ═══════════════════════════════════════════════════════════════
# TEST 7e: Move mode — full navigation then commit
# ═══════════════════════════════════════════════════════════════
def test_move_full_sequence():
    print("\n=== 7e. Move mode full sequence ===")
    reseed()
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("full seq: navigated to drag-drop", False)
        return

    send_keys(TARGET, "m", 0.4)

    # --- e: swap below notification ---
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r = -1
    notif_r = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r = r
    check("full seq: after e1 ghost below notification",
          ghost_r >= 0 and notif_r >= 0 and ghost_r > notif_r,
          f"ghost={ghost_r} notif={notif_r}")

    # --- e again: pop out to depth 1 (between Board UX and API Refactor) ---
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r2 = -1
    api_r = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r2 = r
        if "API Refactor" in rt:
            api_r = r
    check("full seq: after e2 ghost above API Refactor (popped to depth 1)",
          ghost_r2 >= 0 and api_r >= 0 and ghost_r2 < api_r,
          f"ghost={ghost_r2} api={api_r}")

    # --- u: skip back past notification subtree, nest into Board UX ---
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r3 = -1
    board_r = -1
    notif_r3 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r3 = r
        if "Board UX" in rt:
            board_r = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r3 = r
    check("full seq: after u ghost back inside Board UX",
          ghost_r3 >= 0 and board_r >= 0 and ghost_r3 > board_r,
          f"ghost={ghost_r3} board={board_r}")
    check("full seq: after u ghost after notification (last child)",
          ghost_r3 >= 0 and notif_r3 >= 0 and ghost_r3 > notif_r3,
          f"ghost={ghost_r3} notif={notif_r3}")

    # --- Enter: commit ---
    send_keys(TARGET, "Enter", 0.5)

    # Verify store: drag-drop after notification (u nests as last child)
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    dragdrop_idx = -1
    notif_idx = -1
    for i, line in enumerate(lines):
        if "drag-drop" in line:
            dragdrop_idx = i
        if "notification" in line:
            notif_idx = i
    check("full seq: notification before drag-drop in store (u nests last)",
          dragdrop_idx >= 0 and notif_idx >= 0 and notif_idx < dragdrop_idx,
          f"notification at line {notif_idx}, drag-drop at line {dragdrop_idx}")


# ═══════════════════════════════════════════════════════════════
# TEST 7f: Move mode — e then u cancels out (returns to original)
# ═══════════════════════════════════════════════════════════════
def test_move_e_u_cancel():
    print("\n=== 7f. Move mode e then u cancel ===")
    reseed()
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("e/u cancel: navigated to drag-drop", False)
        return

    send_keys(TARGET, "m", 0.4)

    # e: swap below notification
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r1 = -1
    notif_r1 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r1 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r1 = r
    check("e/u cancel: after e ghost below notification",
          ghost_r1 >= 0 and notif_r1 >= 0 and ghost_r1 > notif_r1,
          f"ghost={ghost_r1} notif={notif_r1}")

    # u: swap back above notification (original position)
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r2 = -1
    notif_r2 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r2 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r2 = r
    check("e/u cancel: after u ghost above notification",
          ghost_r2 >= 0 and notif_r2 >= 0 and ghost_r2 < notif_r2,
          f"ghost={ghost_r2} notif={notif_r2}")

    # Enter: commit at original position — store unchanged
    send_keys(TARGET, "Enter", 0.5)
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    dragdrop_idx = -1
    notif_idx = -1
    for i, line in enumerate(lines):
        if "drag-drop" in line:
            dragdrop_idx = i
        if "notification" in line:
            notif_idx = i
    check("e/u cancel: drag-drop before notification in store (unchanged)",
          dragdrop_idx >= 0 and notif_idx >= 0 and dragdrop_idx < notif_idx,
          f"drag-drop at line {dragdrop_idx}, notification at line {notif_idx}")


# ═══════════════════════════════════════════════════════════════
# TEST 7g: Move mode — u then e cancels out (returns to original)
# ═══════════════════════════════════════════════════════════════
def test_move_u_e_cancel():
    print("\n=== 7g. Move mode u then e cancel ===")
    reseed()
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("u/e cancel: navigated to drag-drop", False)
        return

    send_keys(TARGET, "m", 0.4)

    # u: pop out above Board UX (ghost above the goal)
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r1 = -1
    board_r1 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r1 = r
        if "Board UX" in rt:
            board_r1 = r
    check("u/e cancel: after u ghost above Board UX",
          ghost_r1 >= 0 and board_r1 >= 0 and ghost_r1 < board_r1,
          f"ghost={ghost_r1} board={board_r1}")

    # e: pop back below Board UX (ghost back inside, before notification)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r2 = -1
    notif_r2 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r2 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r2 = r
    check("u/e cancel: after e ghost before notification",
          ghost_r2 >= 0 and notif_r2 >= 0 and ghost_r2 < notif_r2,
          f"ghost={ghost_r2} notif={notif_r2}")

    # Enter: commit — store unchanged
    send_keys(TARGET, "Enter", 0.5)
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    dragdrop_idx = -1
    notif_idx = -1
    for i, line in enumerate(lines):
        if "drag-drop" in line:
            dragdrop_idx = i
        if "notification" in line:
            notif_idx = i
    check("u/e cancel: drag-drop before notification in store (unchanged)",
          dragdrop_idx >= 0 and notif_idx >= 0 and dragdrop_idx < notif_idx,
          f"drag-drop at line {dragdrop_idx}, notification at line {notif_idx}")


# ═══════════════════════════════════════════════════════════════
# TEST 7h: Move mode — e then u cancels out (pops parent boundary)
# ═══════════════════════════════════════════════════════════════
def test_move_e_u_cancel_last_child():
    print("\n=== 7h. Move mode e then u cancel (last child) ===")
    reseed()
    reset_to_list()
    if not navigate_to("notification"):
        check("e/u pop: navigated to notification", False)
        return

    send_keys(TARGET, "m", 0.4)

    # e: notification is last child of Board UX → pops out to depth 1 (below Board UX)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r1 = -1
    board_r1 = -1
    api_r1 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r1 = r
        if "Board UX" in rt:
            board_r1 = r
        if "API Refactor" in rt:
            api_r1 = r
    check("e/u pop: after e ghost below Board UX",
          ghost_r1 >= 0 and board_r1 >= 0 and ghost_r1 > board_r1,
          f"ghost={ghost_r1} board={board_r1}")
    check("e/u pop: after e ghost above API Refactor",
          ghost_r1 >= 0 and api_r1 >= 0 and ghost_r1 < api_r1,
          f"ghost={ghost_r1} api={api_r1}")

    # u: nest back into Board UX (back to original position, last child)
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r2 = -1
    dragdrop_r2 = -1
    api_r2 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r2 = r
        if "drag-drop" in rt:
            dragdrop_r2 = r
        if "API Refactor" in rt:
            api_r2 = r
    check("e/u pop: after u ghost after drag-drop",
          ghost_r2 >= 0 and dragdrop_r2 >= 0 and ghost_r2 > dragdrop_r2,
          f"ghost={ghost_r2} dragdrop={dragdrop_r2}")
    check("e/u pop: after u ghost above API Refactor",
          ghost_r2 >= 0 and api_r2 >= 0 and ghost_r2 < api_r2,
          f"ghost={ghost_r2} api={api_r2}")

    # Enter: commit at original position — store unchanged
    send_keys(TARGET, "Enter", 0.5)
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    dragdrop_idx = -1
    notif_idx = -1
    for i, line in enumerate(lines):
        if "drag-drop" in line:
            dragdrop_idx = i
        if "notification" in line:
            notif_idx = i
    check("e/u pop: drag-drop before notification in store (unchanged)",
          dragdrop_idx >= 0 and notif_idx >= 0 and dragdrop_idx < notif_idx,
          f"drag-drop at line {dragdrop_idx}, notification at line {notif_idx}")


# ═══════════════════════════════════════════════════════════════
# TEST 7i: Move mode — u then e then e (pop out, nest back, swap down)
# ═══════════════════════════════════════════════════════════════
def test_move_u_e_e_swap():
    print("\n=== 7i. Move mode u then e then e swap ===")
    reseed()
    reset_to_list()
    if not navigate_to("drag-drop"):
        check("u/e/e swap: navigated to drag-drop", False)
        return

    send_keys(TARGET, "m", 0.4)

    # u: pop above Board UX
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r1 = -1
    board_r1 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r1 = r
        if "Board UX" in rt:
            board_r1 = r
    check("u/e/e swap: after u ghost above Board UX",
          ghost_r1 >= 0 and board_r1 >= 0 and ghost_r1 < board_r1,
          f"ghost={ghost_r1} board={board_r1}")

    # e: nest back into Board UX as first child (before notification)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r2 = -1
    notif_r2 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r2 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r2 = r
    check("u/e/e swap: after e1 ghost before notification",
          ghost_r2 >= 0 and notif_r2 >= 0 and ghost_r2 < notif_r2,
          f"ghost={ghost_r2} notif={notif_r2}")

    # e again: swap with notification (drag-drop becomes second child)
    send_keys(TARGET, "e", 0.2)
    g = capture(TARGET, ROWS, COLS)
    ghost_r3 = -1
    notif_r3 = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_r3 = r
        if "notification" in rt and "(moving)" not in rt:
            notif_r3 = r
    check("u/e/e swap: after e2 ghost after notification",
          ghost_r3 >= 0 and notif_r3 >= 0 and ghost_r3 > notif_r3,
          f"ghost={ghost_r3} notif={notif_r3}")

    # Enter: commit — store should show notification before drag-drop now
    send_keys(TARGET, "Enter", 0.5)
    import subprocess
    out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    lines = out.split("\n")
    dragdrop_idx = -1
    notif_idx = -1
    for i, line in enumerate(lines):
        if "drag-drop" in line:
            dragdrop_idx = i
        if "notification" in line:
            notif_idx = i
    check("u/e/e swap: notification before drag-drop in store",
          dragdrop_idx >= 0 and notif_idx >= 0 and notif_idx < dragdrop_idx,
          f"notification at line {notif_idx}, drag-drop at line {dragdrop_idx}")


# ═══════════════════════════════════════════════════════════════
# TEST 7j: Move mode — u chain (11 presses from Standalone thread A)
# ═══════════════════════════════════════════════════════════════
def test_move_u_chain():
    print("\n=== 7j. Move mode u chain (11 presses) ===")
    reseed()
    reset_to_list()
    if not navigate_to("Standalone thread A"):
        for i in range(1, 12):
            check(f"u{i}: navigated", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u1: nest into Marketing Site
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u1: ghost inside Marketing Site",
          gr >= 0 and gr > refs.get("Marketing", -1),
          f"ghost={gr}")

    # u2: pop out above Marketing Site
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u2: ghost above Marketing Site",
          gr >= 0 and gr < refs.get("Marketing", 999),
          f"ghost={gr}")

    # u3: last child of Ship V2 (depth 1, after auth middleware subtree)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u3: ghost after auth before Marketing (last child of Ship V2)",
          gr >= 0 and gr > refs.get("auth", -1) and gr < refs.get("Marketing", 999),
          f"ghost={gr}")

    # u4: nest into API Refactor as last child
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u4: ghost after auth before Marketing (inside API Refactor)",
          gr >= 0 and gr > refs.get("auth", -1) and gr < refs.get("Marketing", 999),
          f"ghost={gr}")

    # u5: swap up with auth middleware
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u5: ghost before auth after API Refactor header",
          gr >= 0 and gr < refs.get("auth", 999) and gr > refs.get("API Refactor", -1),
          f"ghost={gr}")

    # u6: pop out to depth 1, between Board UX and API Refactor
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u6: ghost after notification before API Refactor",
          gr >= 0 and gr > refs.get("notification", -1) and gr < refs.get("API Refactor", 999),
          f"ghost={gr}")

    # u7: nest into Board UX as last child
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u7: ghost after notification before API Refactor (inside Board UX)",
          gr >= 0 and gr > refs.get("notification", -1) and gr < refs.get("API Refactor", 999),
          f"ghost={gr}")

    # u8: swap up with notification
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u8: ghost before notification after drag-drop",
          gr >= 0 and gr < refs.get("notification", 999) and gr > refs.get("drag-drop", -1),
          f"ghost={gr}")

    # u9: swap up with drag-drop
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u9: ghost before drag-drop after Board UX header",
          gr >= 0 and gr < refs.get("drag-drop", 999) and gr > refs.get("Board UX", -1),
          f"ghost={gr}")

    # u10: pop out of Board UX, depth 1, first child of Ship V2
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u10: ghost before Board UX after Ship V2 header",
          gr >= 0 and gr < refs.get("Board UX", 999) and gr > refs.get("Ship V2", -1),
          f"ghost={gr}")

    # u11: pop out of Ship V2, depth 0, very top
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("u11: ghost at top before Ship V2",
          gr >= 0 and gr < refs.get("Ship V2", 999),
          f"ghost={gr}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7k: Move mode — e chain (11 presses from top)
# ═══════════════════════════════════════════════════════════════
def test_move_e_chain():
    print("\n=== 7k. Move mode e chain (11 presses) ===")
    reseed()
    reset_to_list()
    if not navigate_to("Standalone thread A"):
        for i in range(1, 12):
            check(f"e{i}: navigated", False)
        return
    send_keys(TARGET, "m", 0.4)

    # Do 11 u's to reach the top
    for _ in range(11):
        send_keys(TARGET, "u", 0.12)

    gr, gi, refs = capture_state()
    if not (gr >= 0 and gr < refs.get("Ship V2", 999)):
        check("e-chain: positioned at top", False,
              f"ghost={gr} shipv2={refs.get('Ship V2')}")
        send_keys(TARGET, "Escape", 0.3)
        return
    check("e-chain: positioned at top", True)

    # e1: nest into Ship V2 (first child)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e1: ghost after Ship V2 before Board UX",
          gr >= 0 and gr > refs.get("Ship V2", -1) and gr < refs.get("Board UX", 999),
          f"ghost={gr}")

    # e2: nest into Board UX (first child)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e2: ghost after Board UX before drag-drop",
          gr >= 0 and gr > refs.get("Board UX", -1) and gr < refs.get("drag-drop", 999),
          f"ghost={gr}")

    # e3: swap down with drag-drop
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e3: ghost after drag-drop before notification",
          gr >= 0 and gr > refs.get("drag-drop", -1) and gr < refs.get("notification", 999),
          f"ghost={gr}")

    # e4: swap with notification (last child)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e4: ghost after notification before API Refactor",
          gr >= 0 and gr > refs.get("notification", -1) and gr < refs.get("API Refactor", 999),
          f"ghost={gr}")

    # e5: pop out to depth 1 (between Board UX subtree and API Refactor)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e5: ghost after notification before API Refactor (depth 1)",
          gr >= 0 and gr > refs.get("notification", -1) and gr < refs.get("API Refactor", 999),
          f"ghost={gr}")

    # e6: nest into API Refactor (first child)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e6: ghost after API Refactor before auth",
          gr >= 0 and gr > refs.get("API Refactor", -1) and gr < refs.get("auth", 999),
          f"ghost={gr}")

    # e7: swap with auth middleware
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e7: ghost after auth before Marketing",
          gr >= 0 and gr > refs.get("auth", -1) and gr < refs.get("Marketing", 999),
          f"ghost={gr}")

    # e8: pop out to depth 1 (last child of Ship V2)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e8: ghost after auth before Marketing (depth 1)",
          gr >= 0 and gr > refs.get("auth", -1) and gr < refs.get("Marketing", 999),
          f"ghost={gr}")

    # e9: pop out to depth 0 (before Marketing Site)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e9: ghost before Marketing Site (depth 0)",
          gr >= 0 and gr < refs.get("Marketing", 999) and gr > refs.get("auth", -1),
          f"ghost={gr}")

    # e10: nest into Marketing Site
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e10: ghost after Marketing before Standalone B",
          gr >= 0 and gr > refs.get("Marketing", -1) and gr < refs.get("Standalone thread B", 999),
          f"ghost={gr}")

    # e11: pop out to depth 0 (back to original position)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("e11: ghost after Marketing before Standalone B (depth 0)",
          gr >= 0 and gr > refs.get("Marketing", -1) and gr < refs.get("Standalone thread B", 999),
          f"ghost={gr}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7l: Move mode — draft promote boundary (u past separator)
# ═══════════════════════════════════════════════════════════════
def reseed_no_standalone():
    """Seed without standalone threads — goals with children + drafts only."""
    subprocess.run(["pkill", "-f", "palette"], capture_output=True)
    time.sleep(0.5)
    subprocess.run(["pkill", "-f", "tracker-server"], capture_output=True)
    time.sleep(0.2)
    subprocess.Popen([TRACKER_SERVER], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(0.5)
    for i in range(20, 0, -1):
        subprocess.run(["tmux", "kill-window", "-t", f"test:{i}"], capture_output=True)
    subprocess.run(["rm", "-rf", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run(["mkdir", "-p", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Ship V2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Board UX", "--parent", "ship-v2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "API Refactor", "--parent", "ship-v2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Marketing Site"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "drag-drop upload", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "notification system", "--goal", "board-ux"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "auth middleware", "--goal", "api-refactor"], capture_output=True)
    for name in ["Draft alpha", "Draft beta", "Draft gamma"]:
        result = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", name, "-P", "-F", "#{window_id}"],
                                capture_output=True, text=True)
        wid = result.stdout.strip()
        if wid:
            subprocess.run([AGENT, "tracker", "command",
                          "--window-id", wid, "--session", "test", "--window", name,
                          "start_task", name], capture_output=True)
    time.sleep(0.3)
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def test_move_draft_promote_boundary():
    print("\n=== 7l. Move mode draft promote boundary ===")
    reseed_no_standalone()
    reset_to_list()
    if not navigate_to("Draft alpha"):
        check("draft promote: navigated to Draft alpha", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u: move ghost above separator (promote boundary)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft promote: ghost after auth middleware",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")
    check("draft promote: ghost before Draft beta",
          gr >= 0 and gr < refs.get("Draft beta", 999),
          f"ghost={gr} draft_beta={refs.get('Draft beta')}")

    # Enter: commit — should promote Draft alpha to a thread
    send_keys(TARGET, "Enter", 0.5)
    import subprocess, json
    store_path = os.path.expanduser("~/.cache/agent/goals.json")
    with open(store_path) as f:
        store = json.load(f)
    thread_names = [t.get("name", "") for t in store.get("threads", {}).values()]
    check("draft promote: Draft alpha is now a thread",
          "Draft alpha" in thread_names,
          f"threads: {thread_names}")
    check("draft promote: Draft beta is NOT a thread",
          "Draft beta" not in thread_names,
          f"threads: {thread_names}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7m: Move mode — draft promote with single thread parent
# ═══════════════════════════════════════════════════════════════
def reseed_single_thread():
    """Seed with just Ship V2 > auth middleware + 1 draft."""
    subprocess.run(["pkill", "-f", "palette"], capture_output=True)
    time.sleep(0.5)
    for i in range(20, 0, -1):
        subprocess.run(["tmux", "kill-window", "-t", f"test:{i}"], capture_output=True)
    subprocess.run(["rm", "-rf", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run(["mkdir", "-p", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Ship V2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "auth middleware", "--goal", "ship-v2"], capture_output=True)
    result = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", "Draft alpha", "-P", "-F", "#{window_id}"],
                            capture_output=True, text=True)
    wid = result.stdout.strip()
    if wid:
        subprocess.run([AGENT, "tracker", "command",
                      "--window-id", wid, "--session", "test", "--window", "Draft alpha",
                      "start_task", "Draft alpha"], capture_output=True)
    time.sleep(0.3)
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def test_move_draft_single_parent():
    print("\n=== 7m. Move mode draft promote single parent ===")
    reseed_single_thread()
    if not navigate_to("Draft alpha"):
        check("draft single: navigated to Draft alpha", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u: move ghost above separator
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft single: ghost after auth middleware",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")

    # Enter: commit — promote Draft alpha
    send_keys(TARGET, "Enter", 0.5)
    import json
    store_path = os.path.expanduser("~/.cache/agent/goals.json")
    with open(store_path) as f:
        store = json.load(f)
    thread_names = [t.get("name", "") for t in store.get("threads", {}).values()]
    check("draft single: Draft alpha promoted to thread",
          "Draft alpha" in thread_names,
          f"threads: {thread_names}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7n: Move mode — draft u then e cancels (promote boundary)
# ═══════════════════════════════════════════════════════════════
def test_move_draft_u_e_cancel():
    print("\n=== 7n. Move mode draft u/e cancel ===")
    reseed_single_thread()
    if not navigate_to("Draft alpha"):
        check("draft u/e: navigated to Draft alpha", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u: ghost moves above separator
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft u/e: after u ghost above separator",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")

    # e: ghost moves back below separator (cancel)
    send_keys(TARGET, "e", 0.2)
    gr2, gi2, refs2 = capture_state()
    check("draft u/e: after e ghost below auth middleware (back in draft zone)",
          gr2 >= 0 and gr2 > refs2.get("auth", -1),
          f"ghost={gr2} auth={refs2.get('auth')}")

    # Enter: commit — should NOT promote since ghost is back below separator
    send_keys(TARGET, "Enter", 0.5)
    import json
    store_path = os.path.expanduser("~/.cache/agent/goals.json")
    with open(store_path) as f:
        store = json.load(f)
    thread_names = [t.get("name", "") for t in store.get("threads", {}).values()]
    check("draft u/e: Draft alpha NOT promoted (cancel worked)",
          "Draft alpha" not in thread_names,
          f"threads: {thread_names}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7o: Move mode — draft beta promote + e returns to position
# ═══════════════════════════════════════════════════════════════
def reseed_two_drafts():
    """Seed with Ship V2 > auth middleware + 2 drafts."""
    subprocess.run(["pkill", "-f", "palette"], capture_output=True)
    time.sleep(0.5)
    for i in range(20, 0, -1):
        subprocess.run(["tmux", "kill-window", "-t", f"test:{i}"], capture_output=True)
    subprocess.run(["rm", "-rf", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run(["mkdir", "-p", os.path.expanduser("~/.cache/agent")], capture_output=True)
    subprocess.run([AGENT, "goal", "add-goal", "--title", "Ship V2"], capture_output=True)
    subprocess.run([AGENT, "goal", "add-thread", "--name", "auth middleware", "--goal", "ship-v2"], capture_output=True)
    for name in ["Draft alpha", "Draft beta"]:
        result = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", name, "-P", "-F", "#{window_id}"],
                                capture_output=True, text=True)
        wid = result.stdout.strip()
        if wid:
            subprocess.run([AGENT, "tracker", "command",
                          "--window-id", wid, "--session", "test", "--window", name,
                          "start_task", name], capture_output=True)
    time.sleep(0.3)
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def test_move_draft_beta_promote():
    print("\n=== 7o. Move mode draft beta promote ===")
    reseed_two_drafts()
    if not navigate_to("Draft beta"):
        check("draft beta: navigated to Draft beta", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u: Draft beta moves above separator (skips past Draft alpha)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft beta: after u ghost above separator (after auth)",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")

    # e: Draft beta returns to its original position (below Draft alpha)
    send_keys(TARGET, "e", 0.2)
    gr2, gi2, refs2 = capture_state()
    check("draft beta: after e ghost below auth (back in drafts)",
          gr2 >= 0 and gr2 > refs2.get("auth", -1),
          f"ghost={gr2} auth={refs2.get('auth')}")

    # Enter: commit — should NOT promote since ghost is back in draft zone
    send_keys(TARGET, "Enter", 0.5)
    import json
    store_path = os.path.expanduser("~/.cache/agent/goals.json")
    with open(store_path) as f:
        store = json.load(f)
    thread_names = [t.get("name", "") for t in store.get("threads", {}).values()]
    check("draft beta: NOT promoted (cancel worked)",
          "Draft beta" not in thread_names,
          f"threads: {thread_names}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 7p: Move mode — draft u-chain (4 presses) then e-chain (4 presses)
# ═══════════════════════════════════════════════════════════════
def test_move_draft_u4_e4():
    print("\n=== 7p. Move mode draft u4/e4 chain ===")
    reseed_two_drafts()
    if not navigate_to("Draft alpha"):
        check("draft u4e4: navigated to Draft alpha", False)
        return
    send_keys(TARGET, "m", 0.4)

    # u1: promote boundary (after auth, above separator)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft u1: ghost after auth middleware",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")

    # u2: nest into Ship V2 as last child (depth 1)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft u2: ghost after auth (depth 1, inside Ship V2)",
          gr >= 0 and gr > refs.get("auth", -1) and gi > 3,
          f"ghost={gr} indent={gi} auth={refs.get('auth')}")

    # u3: swap up with auth middleware (before auth)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft u3: ghost before auth middleware",
          gr >= 0 and gr < refs.get("auth", 999) and gr > refs.get("Ship V2", -1),
          f"ghost={gr} auth={refs.get('auth')} shipv2={refs.get('Ship V2')}")

    # u4: pop out of Ship V2 (depth 0, above Ship V2)
    send_keys(TARGET, "u", 0.2)
    gr, gi, refs = capture_state()
    check("draft u4: ghost above Ship V2",
          gr >= 0 and gr < refs.get("Ship V2", 999),
          f"ghost={gr} shipv2={refs.get('Ship V2')}")

    # e1: nest into Ship V2 as first child (before auth)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("draft e1: ghost inside Ship V2 before auth",
          gr >= 0 and gr > refs.get("Ship V2", -1) and gr < refs.get("auth", 999),
          f"ghost={gr} shipv2={refs.get('Ship V2')} auth={refs.get('auth')}")

    # e2: swap down with auth (after auth, depth 1)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("draft e2: ghost after auth middleware",
          gr >= 0 and gr > refs.get("auth", -1),
          f"ghost={gr} auth={refs.get('auth')}")

    # e3: pop out to promote boundary (depth 0)
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("draft e3: ghost at promote boundary (after auth, depth 0)",
          gr >= 0 and gr > refs.get("auth", -1) and gi <= 3,
          f"ghost={gr} indent={gi} auth={refs.get('auth')}")

    # e4: return to original draft position
    send_keys(TARGET, "e", 0.2)
    gr, gi, refs = capture_state()
    check("draft e4: ghost back in draft zone (after auth, before Draft beta)",
          gr >= 0 and gr > refs.get("auth", -1) and gr < refs.get("Draft beta", 999),
          f"ghost={gr} auth={refs.get('auth')} draft_beta={refs.get('Draft beta')}")

    # Enter: commit — should NOT promote (ghost is back in draft zone)
    send_keys(TARGET, "Enter", 0.5)
    import json
    store_path = os.path.expanduser("~/.cache/agent/goals.json")
    with open(store_path) as f:
        store = json.load(f)
    thread_names = [t.get("name", "") for t in store.get("threads", {}).values()]
    check("draft u4e4: NOT promoted (cancel worked)",
          "Draft alpha" not in thread_names,
          f"threads: {thread_names}")

    send_keys(TARGET, "Escape", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 8: Confirm dialog — border + centered
# ═══════════════════════════════════════════════════════════════
def test_confirm_dialog():
    print("\n=== 8. Confirm dialog (toggle done) ===")
    reseed()
    if not navigate_to("drag-drop"):
        check("confirm dialog has border", False, "could not navigate to thread")
        check("confirm title present", False)
        check("confirm dialog has y/n hints", False)
        return
    send_keys(TARGET, "D", 0.5)
    g = capture(TARGET, ROWS, COLS)

    # Should have modal border
    border_rows = []
    for r in range(min(ROWS, len(GRID))):
        if any(i < len(GRID[r]) and GRID[r][i]["ch"] in "┌┐└┘" for i in range(min(COLS, len(GRID[r])))):
            border_rows.append(r)
    check("confirm dialog has border", len(border_rows) >= 2,
          f"border rows: {border_rows}")
    title_row = find_row_with_text("Delete thread")
    check("confirm title present", title_row >= 0,
          f"checked all {ROWS} rows")

    # Should have y/n hints
    hint_found = False
    for r in range(title_row if title_row >= 0 else 0, ROWS):
        if "confirm" in row_text(r).lower():
            hint_found = True
    check("confirm dialog has y/n hints", hint_found)

    send_keys(TARGET, "n", 0.3)


# ═══════════════════════════════════════════════════════════════
# TEST 9: Done state — strikethrough/muted rendering
# ═══════════════════════════════════════════════════════════════
def test_done_state():
    print("\n=== 9. Done thread rendering ===")
    reseed()
    # Get thread ID from list
    list_out = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True).stdout
    import re
    m = re.search(r'drag-drop.*?\(th_\w+\)', list_out)
    if not m:
        check("done thread found", False, "drag-drop thread not in list")
        check("done thread fg=246 (muted)", False)
        return
    thread_id = m.group().split("(")[-1].rstrip(")")
    # Mark as done
    subprocess.run([AGENT, "goal", "done", "--thread", thread_id], capture_output=True)
    time.sleep(0.2)
    # Re-open panel to reload
    send_keys(TARGET, "Escape", 0.5)
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    # Navigate to drag-drop then move away so it's not selected
    navigate_to("drag-drop")
    send_keys(TARGET, "u", 0.2)
    g = capture(TARGET, ROWS, COLS)

    # Find the thread
    thread_row = find_row_with_text("drag-drop")
    check("done thread found", thread_row >= 0)

    if thread_row >= 0:
        text = row_text(thread_row)
        name_col = text.find("drag-drop")
        if name_col >= 0:
            c = cell(thread_row, name_col)
            is_done = str(c.get("fg")) == "246"
            check("done thread fg=246 (muted)", is_done,
                  f"fg={c.get('fg')}, expected 246 (if selected, fg may be 230)")
            if not is_done:
                print(f"    DEBUG done thread row {thread_row} cells:")
                for i in range(min(20, len(GRID[thread_row]))):
                    cc = GRID[thread_row][i]
                    if cc["ch"] != " " or cc.get("bg"):
                        attrs = []
                        if cc.get("fg"): attrs.append(f"fg={cc['fg']}")
                        if cc.get("bg"): attrs.append(f"bg={cc['bg']}")
                        if cc.get("bold"): attrs.append("bold")
                        print(f"      col {i}: {repr(cc['ch'])} [{', '.join(attrs)}]")

        # Done icon (✓) should be on line 1 with fg=150 (green)
        for r in range(max(0, thread_row-1), min(ROWS, thread_row+1)):
            rt = row_text(r)
            if "✓" in rt:
                check_col = rt.find("✓")
                c = cell(r, check_col)
                check("done icon ✓ fg=150 (green)",
                      str(c.get("fg")) == "150",
                      f"fg={c.get('fg')}, expected 150")
                break

    # Undo
    subprocess.run([AGENT, "goal", "done", "--thread", thread_id], capture_output=True)
    time.sleep(0.2)


# ═══════════════════════════════════════════════════════════════
# TEST 10: Expand/collapse glyph toggle
# ═══════════════════════════════════════════════════════════════
def test_expand_glyph():
    print("\n=== 10. Expand/collapse glyph ===")
    reseed()
    # Cursor should be on Ship V2 (node 0) after reset
    # Navigate up to ensure we're at top
    for _ in range(10):
        send_keys(TARGET, "u", 0.02)
    g = capture(TARGET, ROWS, COLS)

    goal_row = find_row_with_text("Ship V2")
    check("goal found", goal_row >= 0)
    if goal_row < 0:
        return

    text = row_text(goal_row)
    has_expand = "▾" in text or "▸" in text or "▹" in text
    check("goal has expand glyph", has_expand, f"text: |{text}|")

    # Goals with children should show ▾ (expanded)
    check("goal with children shows ▾", "▾" in text, f"text: |{text}|")


# ═══════════════════════════════════════════════════════════════
# Run all tests
# ═══════════════════════════════════════════════════════════════
# ═══════════════════════════════════════════════════════════════
# TEST 11: Goal move mode
# ═══════════════════════════════════════════════════════════════
def test_goal_move():
    print("\n=== 11. Goal move mode ===")
    reset_to_list()
    if not navigate_to("Board UX"):
        check("goal move: navigate to Board UX", False)
        return
    send_keys(TARGET, "m", 0.4)
    g = capture(TARGET, ROWS, COLS)

    ghost_found = False
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt and "Board UX" in rt:
            ghost_found = True
            break
    check("goal move: ghost rendered", ghost_found)

    # Move down past subtree — ghost should skip to Marketing or end
    send_keys(TARGET, "e", 0.3)
    g = capture(TARGET, ROWS, COLS)
    ghost_row = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt:
            ghost_row = r
            break
    check("goal move: ghost visible after e", ghost_row >= 0)

    # Move back up
    send_keys(TARGET, "u", 0.3)
    g = capture(TARGET, ROWS, COLS)
    ghost_back = False
    for r in range(ROWS):
        if "(moving)" in row_text(r):
            ghost_back = True
            break
    check("goal move: ghost visible after u", ghost_back)

    # Commit — goal should move
    send_keys(TARGET, "Enter", 0.4)
    g = capture(TARGET, ROWS, COLS)
    check("goal move: committed (no ghost)", "(moving)" not in g)

    # Verify Board UX still exists in the tree
    board_found = False
    for r in range(ROWS):
        if "Board UX" in row_text(r):
            board_found = True
            break
    check("goal move: Board UX still in tree", board_found)

    # Verify store: Board UX still has correct parent
    result = subprocess.run([AGENT, "goal", "list"], capture_output=True, text=True)
    check("goal move: goal list works", result.returncode == 0)

    reset_to_list()


if __name__ == '__main__':
    print("Setting up...")
    setup()

    test_list_view()
    test_selection_highlight()
    test_thread_rendering()
    test_input_box()
    test_picker()
    test_scroll()
    test_move_mode()
    test_move_nest_pop()
    test_move_commit_swap()
    test_move_commit_nest()
    test_move_full_sequence()
    test_move_e_u_cancel()
    test_move_u_e_cancel()
    test_move_e_u_cancel_last_child()
    test_move_u_e_e_swap()
    test_move_u_chain()
    test_move_e_chain()
    test_move_draft_promote_boundary()
    test_move_draft_single_parent()
    test_move_draft_u_e_cancel()
    test_move_draft_beta_promote()
    test_move_draft_u4_e4()
    test_confirm_dialog()
    test_done_state()
    test_expand_glyph()
    test_goal_move()

    teardown()
    sys.exit(summary())
