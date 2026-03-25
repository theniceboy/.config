#!/usr/bin/env python3

import os
import re
import shlex
import subprocess
import sys
from pathlib import Path


HOME = Path.home()


def resurrect_dir() -> Path:
    legacy = HOME / ".tmux" / "resurrect"
    if legacy.is_dir():
        return legacy
    data_home = Path(os.environ.get("XDG_DATA_HOME", str(HOME / ".local" / "share")))
    return data_home / "tmux" / "resurrect"


LAST_FILE = Path(os.environ.get("TMUX_RESURRECT_LAST_FILE", str(resurrect_dir() / "last")))


def tmux_output(*args: str) -> str:
    return subprocess.check_output(["tmux", *args], text=True).strip()


def tmux_run(*args: str) -> None:
    subprocess.run(["tmux", *args], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def parse_device(command: str) -> str:
    match = re.search(r"flutter run -d (?:\"([^\"]+)\"|'([^']+)'|(\S+))", command)
    if not match:
        return ""
    for group in match.groups():
        if group:
            return group.strip()
    return ""


def iter_targets():
    if not LAST_FILE.exists():
        return

    with LAST_FILE.open() as handle:
        for raw_line in handle:
            parts = raw_line.rstrip("\n").split("\t")
            if len(parts) < 11 or parts[0] != "pane":
                continue
            if parts[9].strip() != "script":
                continue

            workspace = parts[7].lstrip(":").strip()
            if "/.agents/" not in workspace:
                continue

            device = parse_device(parts[10].lstrip(":").strip())
            if not device:
                continue

            ensure_server = Path(workspace) / "ensure-server.sh"
            if not ensure_server.is_file():
                continue

            locator = f"{parts[1]}:{parts[2]}.{parts[5]}"
            yield locator, workspace, device


def main() -> int:
    restored = 0

    try:
        targets = list(iter_targets())
    except Exception as exc:
        print(f"restore-agent-run-panes: {exc}", file=sys.stderr)
        return 1

    seen = set()
    for locator, workspace, device in targets:
        if locator in seen:
            continue
        seen.add(locator)

        try:
            current_command = tmux_output("display-message", "-p", "-t", locator, "#{pane_current_command}")
        except Exception:
            continue
        if current_command.strip() != "script":
            continue

        command = f"cd {shlex.quote(workspace)} && ./ensure-server.sh {shlex.quote(device)}"
        tmux_run("respawn-pane", "-k", "-t", locator, command)
        restored += 1

    if restored:
        print(f"restored {restored} agent run pane(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
