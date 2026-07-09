#!/usr/bin/env python3
"""Alt-U / Alt-E sibling reordering in the goals panel.

Covers the confirmed scenarios:
  1. Alt-E swaps a goal down among siblings; cursor follows.
  2. Boundary: Alt-E on the last sibling is a no-op.
  3. Alt-U swaps up (does NOT nest, unlike m+u).
  4. Threads do not cross from one goal into another goal's threads.
  5. Mixed siblings: a thread swaps past a sub-goal under the same parent.
"""
import sys, os, subprocess, time
sys.path.insert(0, os.path.dirname(__file__))
from visual_lib import *

TARGET = "test:0.0"
AGENT = os.environ.get("AGENT_BIN", "/home/testuser/bin/agent")
TRACKER_SERVER = "/home/testuser/bin/tracker-server"
ROWS, COLS = 50, 110


def capture_rows():
    return capture(TARGET, ROWS, COLS)


def dump(label):
    capture_rows()
    print(f"\n--- {label} ---")
    for r in range(ROWS):
        rt = row_text(r)
        if rt.strip():
            print(f"  {r:2d}: |{rt}|")


def selected_row():
    capture_rows()
    for r in range(ROWS):
        if row_has_bg(r, "238") > 5:
            return r
    return -1


def selected_text():
    r = selected_row()
    return row_text(r) if r >= 0 else ""


def navigate_to(text):
    for _ in range(80):
        capture_rows()
        for r in range(ROWS):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "e", 0.06)
    for _ in range(80):
        capture_rows()
        for r in range(ROWS):
            if row_has_bg(r, "238") > 5:
                if text in row_text(r):
                    return True
                break
        send_keys(TARGET, "u", 0.06)
    return False


def row_of(text):
    capture_rows()
    for r in range(ROWS):
        if text in row_text(r):
            return r
    return -1


def open_panel():
    subprocess.run(
        ["tmux", "send-keys", "-t", TARGET,
         f"{AGENT} palette --mode goals --window=@0 --path=/tmp --session-name=test --window-name=test0",
         "Enter"],
        capture_output=True)
    time.sleep(1.5)
    capture_rows()


def reseed(scenario):
    """Wipe store and seed the given scenario's tree."""
    subprocess.run(["pkill", "-f", "palette"], capture_output=True)
    time.sleep(0.4)
    cache = os.path.expanduser("~/.cache/agent")
    subprocess.run(["rm", "-rf", cache], capture_output=True)
    subprocess.run(["mkdir", "-p", cache], capture_output=True)
    if scenario == "goals":
        a = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal A"], capture_output=True, text=True).stdout.strip()
        b = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal B"], capture_output=True, text=True).stdout.strip()
        subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal C"], capture_output=True)
        return a, b
    if scenario == "threads":
        a = subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal A"], capture_output=True, text=True).stdout.strip()
        subprocess.run([AGENT, "goal", "add-goal", "--title", "Goal B"], capture_output=True)
        subprocess.run([AGENT, "goal", "add-thread", "--name", "T1", "--goal", a], capture_output=True)
        subprocess.run([AGENT, "goal", "add-thread", "--name", "T2", "--goal", a], capture_output=True)
        subprocess.run([AGENT, "goal", "add-thread", "--name", "T3", "--goal", "goal-b"], capture_output=True)
        return a
    if scenario == "mixed":
        a = subprocess.run([AGENT, "goal", "add-goal", "--title", "Parent"], capture_output=True, text=True).stdout.strip()
        subprocess.run([AGENT, "goal", "add-goal", "--title", "Sub X", "--parent", a], capture_output=True)
        subprocess.run([AGENT, "goal", "add-thread", "--name", "Thread T", "--goal", a], capture_output=True)
        subprocess.run([AGENT, "goal", "add-goal", "--title", "Sub Y", "--parent", a], capture_output=True)
        return a


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


def main():
    setup()

    # ── 1 & 3: goal swap down + up (no nest) ──
    print("=== Scenario: root goals ===")
    reseed("goals")
    open_panel()
    dump("initial: Goal A / Goal B / Goal C")
    if not navigate_to("Goal B"):
        check("nav to Goal B", False); return 1

    send_keys(TARGET, "alt+e", 0.4)
    dump("Alt-E on Goal B (expect A C B)")
    check("Alt-E: Goal B moved below Goal C", row_of("Goal C") < row_of("Goal B"))
    check("Alt-E: cursor follows Goal B", "Goal B" in selected_text())

    # ── 2: boundary no-op (Goal B is now last) ──
    before_last = row_of("Goal B")
    send_keys(TARGET, "alt+e", 0.4)
    dump("Alt-E on Goal B again (boundary — expect no change)")
    check("Alt-E boundary: no-op (Goal B still last)", row_of("Goal B") == before_last)

    # ── 3: Alt-U swaps up, does NOT nest ──
    if not navigate_to("Goal B"):
        check("nav to Goal B (2)", False); return 1
    send_keys(TARGET, "alt+u", 0.4)
    dump("Alt-U on Goal B (expect swap up, NOT nest under Goal A)")
    check("Alt-U: Goal B swapped above a goal (no nesting)", row_of("Goal B") < row_of("Goal C"))
    # nesting would indent Goal B; verify Goal B row has root indent (1 leading space = panel padding)
    gbb = [row_text(r) for r in range(ROWS) if "Goal B" in row_text(r)]
    if gbb:
        lead = len(gbb[0]) - len(gbb[0].lstrip())
        check("Alt-U: Goal B stayed at root depth (no indent)", lead == 1, f"lead={lead}: |{gbb[0]}|")

    # ── 4: threads don't cross goal boundary ──
    print("\n=== Scenario: threads under two goals ===")
    reseed("threads")
    open_panel()
    dump("initial: Goal A{T1,T2} Goal B{T3}")
    if not navigate_to("T2"):
        check("nav to T2", False); return 1
    t2_before = row_of("T2")
    send_keys(TARGET, "alt+e", 0.4)
    dump("Alt-E on T2 (last under Goal A — expect no-op, must NOT join Goal B)")
    check("Alt-E on last thread: no-op", row_of("T2") == t2_before)
    check("Alt-E: T2 did not move below T3 / Goal B", row_of("T2") < row_of("T3"))

    # swap T2 up within Goal A
    send_keys(TARGET, "alt+u", 0.4)
    dump("Alt-U on T2 (expect T2 above T1 under Goal A)")
    check("Alt-U: T2 swapped above T1", row_of("T2") < row_of("T1"))

    # ── 5: mixed siblings (thread swaps past sub-goal) ──
    print("\n=== Scenario: mixed children under Parent ===")
    reseed("mixed")
    open_panel()
    dump("initial: Parent { Sub X, Thread T, Sub Y }")
    if not navigate_to("Thread T"):
        check("nav to Thread T", False); return 1
    send_keys(TARGET, "alt+e", 0.4)
    dump("Alt-E on Thread T (expect swap past Sub Y)")
    check("Alt-E mixed: Thread T moved below Sub Y", row_of("Sub Y") < row_of("Thread T"))

    print(summary())
    return 0 if FAIL == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
