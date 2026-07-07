#!/usr/bin/env python3
"""Debug helper: dump grid cells with attributes for specific rows."""
import sys, os
sys.path.insert(0, os.path.dirname(__file__))
from capture_grid import parse_to_grid
import subprocess, json

target = sys.argv[1]
rows_to_show = [int(x) for x in sys.argv[2:]] if len(sys.argv) > 2 else [3, 4, 5]

raw = subprocess.run(["tmux", "capture-pane", "-e", "-p", "-t", target],
                     capture_output=True, text=True).stdout
grid = parse_to_grid(raw, 50, 100)

for r in rows_to_show:
    if r >= len(grid):
        continue
    row = grid[r]
    text = "".join(c["ch"] for c in row).rstrip()
    bg_count = sum(1 for c in row if str(c.get("bg")) not in ("None", "none") and c.get("bg") is not None)
    bg238 = sum(1 for c in row if str(c.get("bg")) == "238")
    print(f"\nRow {r}: |{text}|")
    print(f"  cells with any bg: {bg_count}/100, with bg=238: {bg238}/100")
    # Show cells with attributes
    shown = 0
    for i, c in enumerate(row):
        bg = c.get("bg")
        fg = c.get("fg")
        bold = c.get("bold", False)
        if (bg and str(bg) not in ("None", "none")) or (fg and str(fg) not in ("None", "none") and c["ch"] != " "):
            ch = c["ch"]
            parts = []
            if bg: parts.append(f"bg={bg}")
            if fg: parts.append(f"fg={fg}")
            if bold: parts.append("bold")
            print(f"  col {i}: {repr(ch)} [{', '.join(parts)}]")
            shown += 1
            if shown > 20:
                print("  ... (truncated)")
                break
