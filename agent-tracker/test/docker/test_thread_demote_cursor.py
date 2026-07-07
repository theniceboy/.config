#!/usr/bin/env python3
"""Scenario: cursor on a thread, press m, move ghost into the draft section,
press Enter to demote the thread to a draft. The cursor should land on the
new draft after Enter.

Setup:
  Goal A
    Thread T   (bound to window "AlphaWin")
  -----------------------
  Draft 1
  Draft 2
"""
import sys, os, subprocess, time
sys.path.insert(0, os.path.dirname(__file__))
from visual_lib import *

TARGET = "test:0.0"
AGENT = os.environ.get("AGENT_BIN", "/home/testuser/bin/agent")
TRACKER_SERVER = "/home/testuser/bin/tracker-server"
ROWS, COLS = 50, 100
DRAFT_WINDOW = "AlphaWin"


def navigate_to(text):
    for _ in range(60):
        capture(TARGET, ROWS, COLS)
        for r in range(ROWS):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "e", 0.08)
    for _ in range(60):
        capture(TARGET, ROWS, COLS)
        for r in range(ROWS):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "u", 0.08)
    return False


def find_row(substring):
    capture(TARGET, ROWS, COLS)
    for r in range(ROWS):
        if substring in row_text(r):
            return r
    return -1


def ghost_below_separator():
    """Return True if the (moving) ghost row is below the separator row."""
    capture(TARGET, ROWS, COLS)
    sep_row = -1
    ghost_row = -1
    for r in range(ROWS):
        rt = row_text(r)
        if sep_row < 0 and "───" in rt:
            sep_row = r
        if "(moving)" in rt:
            ghost_row = r
    return ghost_row > sep_row >= 0


def setup():
    subprocess.run(["tmux", "kill-session", "-t", "test"], capture_output=True)
    time.sleep(0.2)
    subprocess.run(["go", "build", "-o", TRACKER_SERVER, "./cmd/tracker-server"],
                   cwd="/home/testuser/agent-tracker", capture_output=True)
    subprocess.run(["pkill", "-f", "tracker-server"], capture_output=True)
    time.sleep(0.2)
    subprocess.Popen([TRACKER_SERVER], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(0.5)
    subprocess.run(["tmux", "new-session", "-d", "-s", "test", "-x", str(COLS), "-y", str(ROWS)],
                   capture_output=True)
    time.sleep(0.3)
    cache = os.path.expanduser("~/.cache/agent")
    subprocess.run(["rm", "-rf", cache], capture_output=True)
    subprocess.run(["mkdir", "-p", cache], capture_output=True)

    ga = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal A"],
                        capture_output=True, text=True).stdout.strip()

    # Create the thread's window + tracker task, then promote it to a thread under Goal A
    res = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", DRAFT_WINDOW,
                          "-P", "-F", "#{window_id}"], capture_output=True, text=True)
    wid = res.stdout.strip()
    subprocess.run([AGENT, "tracker", "command", "--window-id", wid, "--session", "test",
                    "--window", DRAFT_WINDOW, "start_task", DRAFT_WINDOW], capture_output=True)
    subprocess.run([AGENT, "goal", "promote", "--window", wid, "--name", "Thread T",
                    "--goal", ga], capture_output=True)

    # Two existing drafts (so there is a draft section to move into)
    for name in ["Draft 1", "Draft 2"]:
        r = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", name,
                            "-P", "-F", "#{window_id}"], capture_output=True, text=True)
        w = r.stdout.strip()
        if w:
            subprocess.run([AGENT, "tracker", "command", "--window-id", w, "--session", "test",
                            "--window", name, "start_task", name], capture_output=True)
    time.sleep(0.3)
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)

    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def dump(label):
    g = capture(TARGET, ROWS, COLS)
    print(f"\n--- {label} ---")
    for r in range(ROWS):
        rt = row_text(r)
        if rt.strip():
            print(f"  {r:2d}: |{rt}|")


def main():
    setup()
    dump("initial")

    if not navigate_to("Thread T"):
        print("FAIL: could not navigate to Thread T")
        return 1
    dump("cursor on Thread T")

    send_keys(TARGET, "m", 0.5)
    dump("after m")

    # Move ghost down until it is in the draft section
    moved = False
    for _ in range(12):
        if ghost_below_separator():
            moved = True
            break
        send_keys(TARGET, "e", 0.3)
    dump("ghost in draft section" if moved else "ghost NEVER reached draft section")
    if not moved:
        print("FAIL: ghost did not reach the draft section")
        return 1

    # Commit demotion
    send_keys(TARGET, "Enter", 0.6)
    dump("after Enter (demote)")

    # Assertions: cursor (selected row) should be on the new draft (DRAFT_WINDOW)
    g = capture(TARGET, ROWS, COLS)
    sel_row = -1
    for r in range(ROWS):
        if row_has_bg(r, "238") > 5:
            sel_row = r
            break
    check("a row is selected after Enter", sel_row >= 0)
    if sel_row >= 0:
        sel_text = row_text(sel_row)
        check("cursor on the demoted draft", DRAFT_WINDOW in sel_text,
              f"selected row {sel_row}: |{sel_text}|")
        check("Thread T demoted (no longer a thread row)", find_row("Thread T") < 0)

    print(summary())
    return 0 if FAIL == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
