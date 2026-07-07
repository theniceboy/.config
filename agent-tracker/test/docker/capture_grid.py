#!/usr/bin/env python3
"""Capture tmux pane as a grid of cells with full ANSI attribute info.

Handles 256-color (38;5;N / 48;5;N) and true-color (38;2;R;G;B) sequences.
"""
import sys, json, subprocess, re

def capture(target):
    result = subprocess.run(
        ["tmux", "capture-pane", "-e", "-p", "-t", target],
        capture_output=True, text=True
    )
    return result.stdout

ANSI_RE = re.compile(r'\x1b\[([0-9;]*)m')

def empty_cell():
    return {"ch": " ", "fg": None, "bg": None, "bold": False, "dim": False,
            "underline": False, "reverse": False, "italic": False}

class AnsiState:
    def __init__(self):
        self.fg = None
        self.bg = None
        self.bold = False
        self.dim = False
        self.underline = False
        self.reverse = False
        self.italic = False

    def reset(self):
        self.fg = None
        self.bg = None
        self.bold = False
        self.dim = False
        self.underline = False
        self.reverse = False
        self.italic = False

    def cell(self, ch):
        return {
            "ch": ch,
            "fg": self.fg,
            "bg": self.bg,
            "bold": self.bold,
            "dim": self.dim,
            "underline": self.underline,
            "reverse": self.reverse,
            "italic": self.italic,
        }

def apply_sgr(params, st):
    """Apply an SGR parameter list to the state."""
    if not params or params == [0]:
        st.reset()
        return

    i = 0
    while i < len(params):
        p = params[i]
        if p == 0:
            st.reset()
        elif p == 1:
            st.bold = True
        elif p == 2:
            st.dim = True
        elif p == 3:
            st.italic = True
        elif p == 4:
            st.underline = True
        elif p == 7:
            st.reverse = True
        elif p == 22:
            st.bold = False
            st.dim = False
        elif p == 23:
            st.italic = False
        elif p == 24:
            st.underline = False
        elif p == 27:
            st.reverse = False
        elif p == 39:
            st.fg = None
        elif p == 49:
            st.bg = None
        elif 30 <= p <= 37:
            st.fg = str(p - 30)
        elif 40 <= p <= 47:
            st.bg = str(p - 40)
        elif 90 <= p <= 97:
            st.fg = str(p - 90 + 8)
        elif 100 <= p <= 107:
            st.bg = str(p - 100 + 8)
        elif p == 38 or p == 48:
            # Extended color: 38;5;N (256-color) or 38;2;R;G;B (true-color)
            if i + 1 < len(params):
                if params[i+1] == 5 and i + 2 < len(params):
                    # 256-color
                    color = str(params[i+2])
                    if p == 38:
                        st.fg = color
                    else:
                        st.bg = color
                    i += 2
                elif params[i+1] == 2 and i + 4 < len(params):
                    # True-color: approximate to 256-color palette
                    r, g, b = params[i+2], params[i+3], params[i+4]
                    color = rgb_to_256(r, g, b)
                    if p == 38:
                        st.fg = color
                    else:
                        st.bg = color
                    i += 4
        i += 1

def rgb_to_256(r, g, b):
    """Approximate RGB to nearest 256-color palette index."""
    # Check grayscale first (232-255)
    if r == g == b:
        if r < 8:
            return "16"
        if r > 248:
            return "231"
        gray = int(round((r - 8) / 247 * 24))
        return str(232 + gray)
    # 6x6x6 color cube (16-231)
    def to_cube(v):
        if v < 48:
            return 0
        if v < 115:
            return 1
        return min(5, int(round((v - 115) / 40)) + 2)
    return str(16 + 36 * to_cube(r) + 6 * to_cube(g) + to_cube(b))

def parse_to_grid(raw, rows, cols):
    lines = raw.split('\n')
    grid = []
    st = AnsiState()
    for ri in range(rows):
        if ri >= len(lines):
            grid.append([empty_cell() for _ in range(cols)])
            continue
        line = lines[ri]
        cells = []
        i = 0
        while i < len(line) and len(cells) < cols:
            ch = line[i]
            if ch == '\x1b':
                m = ANSI_RE.match(line, i)
                if m:
                    param_str = m.group(1)
                    if param_str:
                        params = [int(x) for x in param_str.split(';') if x]
                    else:
                        params = [0]
                    apply_sgr(params, st)
                    i = m.end()
                    continue
                else:
                    i += 1
                    continue

            if ch == '\r':
                i += 1
                continue

            cells.append(st.cell(ch))
            i += 1

        while len(cells) < cols:
            c = empty_cell()
            c["bg"] = st.bg
            cells.append(c)

        grid.append(cells[:cols])
    return grid

if __name__ == '__main__':
    target = sys.argv[1]
    rows = int(sys.argv[2]) if len(sys.argv) > 2 else 50
    cols = int(sys.argv[3]) if len(sys.argv) > 3 else 100
    raw = capture(target)
    grid = parse_to_grid(raw, rows, cols)
    print(json.dumps(grid))
