#!/usr/bin/env python3

import re
import subprocess
import sys
from typing import List, Dict


def run_tmux(args: List[str], check: bool = True, capture: bool = False) -> str:
    kwargs = {
        "check": check,
    }
    if capture:
        kwargs["stdout"] = subprocess.PIPE
        kwargs["text"] = True
    result = subprocess.run(["tmux", *args], **kwargs)
    if capture:
        return result.stdout.rstrip("\n")
    return ""


def list_sessions() -> List[Dict[str, object]]:
    output = run_tmux([
        "list-sessions",
        "-F",
        "#{session_id}\t#{session_name}\t#{session_created}"
    ], capture=True)
    if not output:
        return []

    sessions = []
    for line in output.splitlines():
        session_id, name, created_str = line.split("\t")
        created = int(created_str)
        match = re.match(r"^(\d+)-(.*)$", name)
        if match:
            index = int(match.group(1))
            label = match.group(2)
        else:
            index = None
            label = name
        sessions.append({
            "id": session_id,
            "name": name,
            "created": created,
            "index": index,
            "label": label,
        })

    def sort_key(entry: Dict[str, object]):
        index = entry["index"]
        return (0, index) if index is not None else (1, entry["created"])

    sessions.sort(key=sort_key)
    return sessions


def sanitize_label(label: str) -> str:
    stripped = label.strip()
    return stripped or "session"


def apply_order(ordered_sessions: List[Dict[str, object]]) -> None:
    for position, session in enumerate(ordered_sessions, start=1):
        label = sanitize_label(str(session["label"]))
        new_name = f"{position}-{label}"
        run_tmux(["rename-session", "-t", session["id"], new_name])


def current_session_id() -> str:
    return run_tmux(["display-message", "-p", "#{session_id}"], capture=True)


def current_window_id() -> str:
    return run_tmux(["display-message", "-p", "#{window_id}"], capture=True)


def command_switch(index_str: str) -> None:
    try:
        index = int(index_str)
    except ValueError:
        return
    if index < 1:
        return
    sessions = list_sessions()
    if index > len(sessions):
        return
    run_tmux(["switch-client", "-t", sessions[index - 1]["id"]], check=False)
    run_tmux(["refresh-client", "-S"], check=False)


def command_rename(label: str) -> None:
    label = sanitize_label(label)
    current_id = current_session_id()
    sessions = list_sessions()
    for session in sessions:
        if session["id"] == current_id:
            session["label"] = label
            break
    else:
        return
    apply_order(sessions)
    # run_tmux(["display-message", f"Renamed tmux session to {label}"] , check=False)


def command_move(direction: str) -> None:
    direction = direction.lower()
    sessions = list_sessions()
    current_id = current_session_id()
    indices = {session["id"]: idx for idx, session in enumerate(sessions)}
    if current_id not in indices:
        return
    pos = indices[current_id]
    if direction == "left" and pos > 0:
        sessions[pos - 1], sessions[pos] = sessions[pos], sessions[pos - 1]
    elif direction == "right" and pos < len(sessions) - 1:
        sessions[pos], sessions[pos + 1] = sessions[pos + 1], sessions[pos]
    else:
        return
    apply_order(sessions)


def command_ensure() -> None:
    sessions = list_sessions()
    if sessions:
        apply_order(sessions)


def command_created() -> None:
    # Called after a session is created; ensure numbering stays contiguous.
    command_ensure()


def command_move_window_to_session(index_str: str) -> None:
    try:
        index = int(index_str)
    except ValueError:
        return
    if index < 1:
        return
    sessions = list_sessions()
    if index > len(sessions):
        return
    target_session_id = sessions[index - 1]["id"]
    source_window_id = current_window_id()
    if not source_window_id:
        return
    current_id = current_session_id()
    if target_session_id != current_id:
        run_tmux(["move-window", "-s", source_window_id, "-t", f"{target_session_id}:"], check=False)
    run_tmux(["switch-client", "-t", target_session_id], check=False)


def main(argv: List[str]) -> None:
    if len(argv) < 2:
        return
    command = argv[1]
    if command == "switch" and len(argv) >= 3:
        command_switch(argv[2])
    elif command == "rename" and len(argv) >= 3:
        command_rename(argv[2])
    elif command == "move" and len(argv) >= 3:
        command_move(argv[2])
    elif command == "ensure":
        command_ensure()
    elif command == "created":
        command_created()
    elif command == "move-window-to" and len(argv) >= 3:
        command_move_window_to_session(argv[2])


if __name__ == "__main__":
    main(sys.argv)
