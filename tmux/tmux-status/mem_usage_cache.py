#!/usr/bin/env python3
"""
Runs top once, maps all process memory to tmux panes/windows, writes JSON cache.
Intended to be called periodically (e.g. every status refresh); self-throttles to 10s.
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time

CACHE_FILE = "/tmp/tmux-mem-usage.json"
LOCK_FILE = "/tmp/tmux-mem-usage.lock"
STALE_SECONDS = 5


def is_fresh() -> bool:
    try:
        return (time.time() - os.path.getmtime(CACHE_FILE)) < STALE_SECONDS
    except OSError:
        return False


def run(cmd: str) -> str:
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    return r.stdout


def parse_mem(s: str) -> float:
    s = s.strip()
    if not s or s == "-":
        return 0.0
    m = re.match(r"([\d.]+)([KMGB]?)", s, re.IGNORECASE)
    if not m:
        return 0.0
    v = float(m.group(1))
    u = m.group(2).upper()
    if u == "K":
        return v / 1024
    if u == "G":
        return v * 1024
    return v


def fmt(mb: float) -> str:
    if mb >= 1024:
        return f"{mb / 1024:.1f}G"
    return f"{mb:.0f}M"


def get_top_mem() -> dict[int, float]:
    out = run("top -l 1 -o mem -n 9999 -stats pid,mem,cmprs 2>/dev/null")
    result: dict[int, float] = {}
    for line in out.strip().split("\n"):
        parts = line.split()
        if len(parts) >= 3 and parts[0].isdigit():
            pid = int(parts[0])
            result[pid] = parse_mem(parts[1]) + parse_mem(parts[2])
    return result


def get_ppid_map() -> dict[int, int]:
    out = run("ps -eo pid,ppid")
    m: dict[int, int] = {}
    for line in out.strip().split("\n"):
        parts = line.split()
        if len(parts) >= 2 and parts[0].isdigit() and parts[1].isdigit():
            m[int(parts[0])] = int(parts[1])
    return m


def get_tmux_panes() -> list[dict]:
    out = run(
        "tmux list-panes -a -F '#{session_name}\t#{window_index}\t#{window_id}\t#{pane_id}\t#{pane_pid}' 2>/dev/null"
    )
    panes = []
    for line in out.strip().split("\n"):
        if not line:
            continue
        parts = line.split("\t")
        if len(parts) >= 5 and parts[4].isdigit():
            panes.append(
                {
                    "session": parts[0],
                    "window_idx": parts[1],
                    "window_id": parts[2],
                    "pane_id": parts[3],
                    "pane_pid": int(parts[4]),
                }
            )
    return panes


def build_children_map(ppid_map: dict[int, int]) -> dict[int, list[int]]:
    children: dict[int, list[int]] = {}
    for pid, ppid in ppid_map.items():
        children.setdefault(ppid, []).append(pid)
    return children


def get_descendants(pid: int, children_map: dict[int, list[int]]) -> set[int]:
    result = set()
    stack = [pid]
    while stack:
        p = stack.pop()
        for c in children_map.get(p, []):
            if c not in result:
                result.add(c)
                stack.append(c)
    return result


def main():
    if is_fresh():
        return

    fd = None
    try:
        fd = os.open(LOCK_FILE, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
    except FileExistsError:
        try:
            if (time.time() - os.path.getmtime(LOCK_FILE)) > 30:
                os.unlink(LOCK_FILE)
        except OSError:
            pass
        return
    try:
        _generate()
    finally:
        os.close(fd)
        try:
            os.unlink(LOCK_FILE)
        except OSError:
            pass


def _generate():
    top_mem = get_top_mem()
    ppid_map = get_ppid_map()
    children_map = build_children_map(ppid_map)
    panes = get_tmux_panes()

    pane_mem: dict[str, float] = {}
    window_mem: dict[str, float] = {}

    for pane in panes:
        desc = get_descendants(pane["pane_pid"], children_map)
        desc.add(pane["pane_pid"])
        total = sum(top_mem.get(p, 0) for p in desc)
        pane_mem[pane["pane_id"]] = total

        wkey = f"{pane['session']}:{pane['window_idx']}"
        window_mem[wkey] = window_mem.get(wkey, 0) + total

    total_tmux = sum(pane_mem.values())

    pane_fmt = {k: fmt(v) for k, v in pane_mem.items()}
    window_fmt = {k: fmt(v) for k, v in window_mem.items()}

    data = {"ts": time.time(), "pane": pane_fmt, "window": window_fmt, "total": fmt(total_tmux)}
    tmp = CACHE_FILE + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f)
    os.replace(tmp, CACHE_FILE)


if __name__ == "__main__":
    main()
