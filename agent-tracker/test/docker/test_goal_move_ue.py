#!/usr/bin/env python3
"""Scenario: cursor on Goal B, press m, u, e.
Setup:
  Goal A
  Goal B
    Thread C
  -----------------------
  Draft 1
  Draft 2
  Draft 3
Prints the panel after each keypress so we can observe the ghost position.
"""
import sys, os, subprocess, time
sys.path.insert(0, os.path.dirname(__file__))
from visual_lib import *

TARGET = "test:0.0"
AGENT = os.environ.get("AGENT_BIN", "/home/testuser/bin/agent")
TRACKER_SERVER = "/home/testuser/bin/tracker-server"
ROWS, COLS = 50, 100


def dump(label):
    g = capture(TARGET, ROWS, COLS)
    print(f"\n===== {label} =====")
    for r in range(ROWS):
        rt = row_text(r)
        if rt.strip():
            print(f"  {r:2d}: |{rt}|")
    # locate ghost
    for r in range(ROWS):
        if "(moving)" in row_text(r):
            print(f"  >> ghost at row {r}: |{row_text(r)}|")
            break


def navigate_to(text):
    """Move cursor until the selected (bg 238) row contains text. Down then up."""
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


def setup():
    subprocess.run(["tmux", "kill-session", "-t", "test"], capture_output=True)
    time.sleep(0.2)
    subprocess.run(["go", "build", "-o", TRACKER_SERVER, "./cmd/tracker-server"],
                   cwd="/home/testuser/agent-tracker", capture_output=True)
    subprocess.run(["pkill", "-f", "tracker-server"], capture_output=True)
    time.sleep(0.2)
    subprocess.Popen([TRACKER_SERVER], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    time.sleep(0.5)
    subprocess.run(["tmux", "new-session", "-d", "-s", "test",
                    "-x", str(COLS), "-y", str(ROWS)], capture_output=True)
    time.sleep(0.3)
    cache = os.path.expanduser("~/.cache/agent")
    subprocess.run(["rm", "-rf", cache], capture_output=True)
    subprocess.run(["mkdir", "-p", cache], capture_output=True)

    ga = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal A"],
                        capture_output=True, text=True).stdout.strip()
    gb = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal B"],
                        capture_output=True, text=True).stdout.strip()
    print(f"goal ids: A={ga} B={gb}")
    subprocess.run([AGENT, "goal", "add-thread", "--name", "Thread C", "--goal", gb],
                   capture_output=True)

    for name in ["Draft 1", "Draft 2", "Draft 3"]:
        result = subprocess.run(["tmux", "new-window", "-d", "-t", "test", "-n", name,
                                 "-P", "-F", "#{window_id}"], capture_output=True, text=True)
        wid = result.stdout.strip()
        if wid:
            subprocess.run([AGENT, "tracker", "command", "--window-id", wid,
                            "--session", "test", "--window", name, "start_task", name],
                           capture_output=True)
    time.sleep(0.3)
    subprocess.run(["tmux", "select-window", "-t", "test:0"], capture_output=True)

    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture(TARGET, ROWS, COLS)


def main():
    setup()
    dump("initial")

    if not navigate_to("Goal B"):
        print("FAIL: could not navigate cursor to Goal B")
        return 1
    dump("cursor on Goal B")

    send_keys(TARGET, "m", 0.5)
    dump("after m  (enter move mode)")

    send_keys(TARGET, "u", 0.4)
    dump("after u  (expect: nest under Goal A)")

    send_keys(TARGET, "e", 0.4)
    dump("after e  (expect: pop back to root; NOT in draft area)")

    # Assertions
    g = capture(TARGET, ROWS, COLS)
    ghost_row = -1
    first_draft_row = -1
    for r in range(ROWS):
        rt = row_text(r)
        if "(moving)" in rt and ghost_row < 0:
            ghost_row = r
        if "Draft" in rt and first_draft_row < 0:
            first_draft_row = r
    print(f"\nghost_row={ghost_row}  first_draft_row={first_draft_row}")
    check("ghost still visible after e", ghost_row >= 0)
    if ghost_row >= 0 and first_draft_row >= 0:
        check("ghost above draft section (not in draft area)",
              ghost_row < first_draft_row,
              f"ghost row {ghost_row} >= first draft row {first_draft_row}")
    if ghost_row >= 0:
        gt = row_text(ghost_row)
        ghost_lead = len(gt) - len(gt.lstrip())
        # root rows carry 1 leading space (panel padding); find Goal A's indent as the root reference
        root_lead = 1
        for r in range(ROWS):
            if "Goal A" in row_text(r):
                root_lead = len(row_text(r)) - len(row_text(r).lstrip())
                break
        check("ghost at root depth (matches Goal A indent)", ghost_lead == root_lead,
              f"ghost leading={ghost_lead}, root leading={root_lead}: |{gt}|")

    print(summary())
    return 0 if FAIL == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
