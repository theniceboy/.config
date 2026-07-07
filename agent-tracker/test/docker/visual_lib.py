#!/usr/bin/env python3
"""Visual test framework for agent-tracker TUI.

Captures tmux pane cells with full ANSI attribute info and runs assertions
on background colors, foreground colors, bold, and spatial layout.
"""
import sys, json, subprocess, time, os

GRID = []  # type: list

def send_keys(target, keys, delay=0.3):
    subprocess.run(["tmux", "send-keys", "-t", target] + keys.split(),
                   capture_output=True)
    time.sleep(delay)

def send_keys_literal(target, text, delay=0.2):
    """Send text literally, char by char, preserving spaces."""
    for ch in text:
        subprocess.run(["tmux", "send-keys", "-t", target, "-l", ch],
                       capture_output=True)
        time.sleep(0.03)
    time.sleep(delay)

def capture(target, rows=50, cols=100):
    result = subprocess.run(
        ["python3", os.path.join(os.path.dirname(__file__), "capture_grid.py"),
         target, str(rows), str(cols)],
        capture_output=True, text=True
    )
    global GRID
    new_grid = json.loads(result.stdout)
    GRID.clear()
    GRID.extend(new_grid)
    return GRID

def cell(row, col):
    if row < 0 or row >= len(GRID):
        return {"ch": " ", "fg": None, "bg": None, "bold": False, "dim": False}
    r = GRID[row]
    if col < 0 or col >= len(r):
        return {"ch": " ", "fg": None, "bg": None, "bold": False, "dim": False}
    return r[col]

def row_text(row, cols=None):
    if row < 0 or row >= len(GRID):
        return ""
    r = GRID[row]
    if cols:
        r = r[:cols]
    return "".join(c["ch"] for c in r).rstrip()

def row_has_bg(row, bg_val, min_cols=1, max_col=None):
    """Count cells in a row that have a specific background."""
    if row < 0 or row >= len(GRID):
        return 0
    r = GRID[row]
    if max_col is None:
        max_col = len(r)
    count = 0
    for c in r[:max_col]:
        if str(c.get("bg")) == str(bg_val):
            count += 1
    return count

def row_bg_extent(row):
    """Return (start_col, end_col) of contiguous bg on a row, or (0,0) if none."""
    if row < 0 or row >= len(GRID):
        return (0, 0)
    r = GRID[row]
    bgs = set()
    for c in r:
        bg = c.get("bg")
        if bg is not None and str(bg) not in ("None", "none"):
            bgs.add(str(bg))
    if not bgs:
        return (0, 0)
    # find first and last cell with any bg
    first = None
    last = None
    for i, c in enumerate(r):
        bg = c.get("bg")
        if bg is not None and str(bg) not in ("None", "none"):
            if first is None:
                first = i
            last = i
    return (first if first else 0, last if last else 0)

def find_row_with_text(text, start=0, end=None):
    """Find row index containing text."""
    if end is None:
        end = len(GRID)
    for r in range(start, end):
        if text in row_text(r):
            return r
    return -1

PASS = 0
FAIL = 0
FAILURES = []

def check(name, condition, detail=""):
    global PASS, FAIL
    if condition:
        PASS += 1
        print(f"  PASS: {name}")
    else:
        FAIL += 1
        FAILURES.append(name)
        print(f"  FAIL: {name}" + (f" — {detail}" if detail else ""))

def check_cell_bg(name, row, col, expected_bg):
    c = cell(row, col)
    actual = c.get("bg")
    ok = str(actual) == str(expected_bg)
    check(name, ok,
          f"row {row} col {col}: expected bg={expected_bg}, got bg={actual} ch={repr(c['ch'])}")

def check_cell_fg(name, row, col, expected_fg):
    c = cell(row, col)
    actual = c.get("fg")
    ok = str(actual) == str(expected_fg)
    check(name, ok,
          f"row {row} col {col}: expected fg={expected_fg}, got fg={actual} ch={repr(c['ch'])}")

def check_cell_bold(name, row, col, expected=True):
    c = cell(row, col)
    actual = c.get("bold", False)
    ok = actual == expected
    check(name, ok,
          f"row {row} col {col}: expected bold={expected}, got bold={actual} ch={repr(c['ch'])}")

def check_row_bg_full(name, row, expected_bg, width, min_pct=0.9):
    """Check that a row has bg color extending at least min_pct of width."""
    count = row_has_bg(row, expected_bg, max_col=width)
    pct = count / width if width > 0 else 0
    ok = pct >= min_pct
    check(name, ok,
          f"row {row}: expected bg={expected_bg} covering >= {min_pct*100:.0f}% of {width} cols, "
          f"got {count}/{width} ({pct*100:.0f}%)")

def check_row_bg_continuous(name, row, expected_bg, expected_width):
    """Check that bg extends continuously from first cell to at least expected_width."""
    if row >= len(GRID):
        check(name, False, f"row {row} out of bounds")
        return
    r = GRID[row]
    first_bg = None
    for i, c in enumerate(r):
        if str(c.get("bg")) == str(expected_bg):
            first_bg = i
            break
    if first_bg is None:
        check(name, False, f"row {row}: no cell with bg={expected_bg} found")
        return
    # check continuity from first_bg
    continuous = 0
    for i in range(first_bg, len(r)):
        if str(r[i].get("bg")) == str(expected_bg):
            continuous += 1
        else:
            break
    ok = continuous >= expected_width
    check(name, ok,
          f"row {row}: expected continuous bg={expected_bg} for >= {expected_width} cols from col {first_bg}, "
          f"got {continuous}")

def dump_row(row, cols=None):
    """Pretty-print a row with cell attributes."""
    if row < 0 or row >= len(GRID):
        print(f"    [row {row}: out of bounds]")
        return
    r = GRID[row]
    if cols:
        r = r[:cols]
    text = "".join(c["ch"] for c in r)
    print(f"    row {row}: |{text}|")
    for i, c in enumerate(r):
        attrs = []
        if c.get("bg") and str(c["bg"]) not in ("None", "none"):
            attrs.append(f"bg={c['bg']}")
        if c.get("fg") and str(c["fg"]) not in ("None", "none"):
            attrs.append(f"fg={c['fg']}")
        if c.get("bold"):
            attrs.append("bold")
        if attrs and c["ch"] != " ":
            print(f"      col {i}: {repr(c['ch'])} [{', '.join(attrs)}]")
        elif attrs and c["ch"] == " ":
            # only print first few bg-only cells to avoid noise
            pass

def dump_row_bg(row, cols=None):
    """Show only bg coverage for a row."""
    if row < 0 or row >= len(GRID):
        print(f"    [row {row}: out of bounds]")
        return
    r = GRID[row]
    if cols:
        r = r[:cols]
    parts = []
    for i, c in enumerate(r):
        bg = c.get("bg")
        if bg and str(bg) not in ("None", "none"):
            parts.append(str(bg))
        else:
            parts.append("·")
    bg_str = " ".join(parts[:60])
    text = "".join(c["ch"] for c in r).rstrip()
    print(f"    row {row} text: |{text}|")
    print(f"    row {row} bg:   {bg_str}")

def summary():
    print(f"\n{'='*40}")
    print(f"Visual Results: {PASS} passed, {FAIL} failed")
    print(f"{'='*40}")
    if FAILURES:
        print(f"Failed: {', '.join(FAILURES)}")
    return FAIL
