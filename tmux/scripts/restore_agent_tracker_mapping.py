#!/usr/bin/env python3

import json
import os
import subprocess
import sys
from pathlib import Path


HOME = Path.home()
AGENTS_PATH = HOME / ".config/agent-tracker/run/agents.json"
TODOS_PATH = HOME / ".cache/agent/todos.json"
SNAPSHOT_PATH = Path(os.environ.get("XDG_STATE_HOME", str(HOME / ".local/state"))) / "agent-tracker" / "window-snapshot.json"


def tmux_output(*args: str) -> str:
    return subprocess.check_output(["tmux", *args], text=True).strip()


def tmux_run(*args: str) -> None:
    subprocess.run(["tmux", *args], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def load_json(path: Path):
    with path.open() as handle:
        return json.load(handle)


def save_json(path: Path, payload) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w") as handle:
        json.dump(payload, handle, indent=2)
        handle.write("\n")


def list_windows():
    out = tmux_output("list-windows", "-a", "-F", "#{session_name}\t#{session_id}\t#{window_name}\t#{window_id}\t#{window_index}")
    windows = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) != 5:
            continue
        windows.append(
            {
                "session_name": parts[0].strip(),
                "session_id": parts[1].strip(),
                "window_name": parts[2].lstrip(":").strip(),
                "window_id": parts[3].strip(),
                "window_index": parts[4].strip(),
            }
        )
    return windows


def list_panes():
    out = tmux_output("list-panes", "-a", "-F", "#{window_id}\t#{pane_index}\t#{pane_id}\t#{pane_current_path}")
    panes = []
    for line in out.splitlines():
        parts = line.split("\t", 3)
        if len(parts) != 4:
            continue
        try:
            pane_index = int(parts[1].strip())
        except ValueError:
            continue
        panes.append(
            {
                "window_id": parts[0].strip(),
                "pane_index": pane_index,
                "pane_id": parts[2].strip(),
                "path": parts[3].strip(),
            }
        )
    return panes


def detect_agent_id_from_path(path: str) -> str:
    clean = os.path.normpath(path.strip())
    needle = f"{os.sep}.agents{os.sep}"
    if needle not in clean:
        return ""
    rest = clean.split(needle, 1)[1]
    if not rest:
        return ""
    return rest.split(os.sep, 1)[0].strip()


def merge_items(existing, incoming):
    merged = list(existing)
    merged.extend(incoming)
    return merged


def main() -> int:
    if not AGENTS_PATH.exists():
        return 0

    try:
        agents_payload = load_json(AGENTS_PATH)
        windows = list_windows()
        panes = list_panes()
    except Exception as exc:
        print(f"restore-agent-tracker-mapping: {exc}", file=sys.stderr)
        return 1

    windows_by_key = {(w["session_name"], w["window_name"]): w for w in windows}
    panes_by_window = {}
    inferred_window_for_agent = {}
    for pane in panes:
        panes_by_window.setdefault(pane["window_id"], []).append(pane)
        agent_id = detect_agent_id_from_path(pane["path"])
        if agent_id and agent_id not in inferred_window_for_agent:
            inferred_window_for_agent[agent_id] = pane["window_id"]

    windows_by_id = {w["window_id"]: w for w in windows}

    changed_agents = 0
    state_changed = False
    window_migrations = {}
    session_migrations = {}

    agents = agents_payload.get("agents", {})
    for agent_id, record in agents.items():
        session_name = str(record.get("tmux_session_name", "")).strip()
        old_window_id = str(record.get("tmux_window_id", "")).strip()
        old_session_id = str(record.get("tmux_session_id", "")).strip()

        match = windows_by_key.get((session_name, agent_id))
        if match is None:
            inferred_window_id = inferred_window_for_agent.get(agent_id, "")
            if inferred_window_id:
                match = windows_by_id.get(inferred_window_id)
        if match is None:
            continue

        new_window_id = match["window_id"]
        new_session_id = match["session_id"]
        new_session_name = match["session_name"]

        if old_window_id and old_window_id != new_window_id:
            window_migrations[old_window_id] = new_window_id
        if old_session_id and old_session_id != new_session_id:
            session_migrations[old_session_id] = new_session_id

        if (
            old_window_id != new_window_id
            or old_session_id != new_session_id
            or session_name != new_session_name
        ):
            changed_agents += 1
            state_changed = True

        record["tmux_window_id"] = new_window_id
        record["tmux_session_id"] = new_session_id
        record["tmux_session_name"] = new_session_name

        tmux_run("set-option", "-w", "-t", new_window_id, "@agent_id", agent_id)

        panes_for_window = sorted(panes_by_window.get(new_window_id, []), key=lambda pane: pane["pane_index"])
        if panes_for_window:
            roles = ["ai", "git", "run"]
            pane_record = dict(record.get("panes", {}))
            for idx, role in enumerate(roles):
                if idx >= len(panes_for_window):
                    break
                pane_id = panes_for_window[idx]["pane_id"]
                pane_record[role] = pane_id
                tmux_run("set-option", "-p", "-t", pane_id, "@agent_role", role)
            if pane_record != record.get("panes", {}):
                state_changed = True
            record["panes"] = pane_record

    if state_changed:
        save_json(AGENTS_PATH, agents_payload)

    # ── snapshot-based migration for non-agent windows ────────
    if SNAPSHOT_PATH.exists():
        try:
            snapshot = load_json(SNAPSHOT_PATH)
        except Exception:
            snapshot = {}
        current_by_pos = {
            (w["session_name"], w["window_index"]): w["window_id"] for w in windows
        }
        for old_wid, info in snapshot.items():
            old_wid = old_wid.strip()
            sn = str(info.get("session_name", "")).strip()
            wi = str(info.get("window_index", "")).strip()
            if not sn or not wi:
                continue
            new_wid = current_by_pos.get((sn, wi), "")
            if new_wid and old_wid != new_wid and old_wid not in window_migrations:
                window_migrations[old_wid] = new_wid

    migrated_window_todos = 0
    migrated_session_todos = 0
    if TODOS_PATH.exists():
        todos_payload = load_json(TODOS_PATH)

        windows_store = todos_payload.setdefault("windows", {})
        new_windows_store: dict[str, list] = {}
        for wid, todos in windows_store.items():
            target = window_migrations.get(wid, wid)
            if target != wid:
                migrated_window_todos += len(todos)
            existing = new_windows_store.get(target, [])
            new_windows_store[target] = merge_items(existing, todos)
        todos_payload["windows"] = new_windows_store

        sessions_store = todos_payload.setdefault("sessions", {})
        new_sessions_store: dict[str, list] = {}
        for sid, todos in sessions_store.items():
            target = session_migrations.get(sid, sid)
            if target != sid:
                migrated_session_todos += len(todos)
            existing = new_sessions_store.get(target, [])
            new_sessions_store[target] = merge_items(existing, todos)
        todos_payload["sessions"] = new_sessions_store

        if migrated_window_todos or migrated_session_todos or new_windows_store != windows_store or new_sessions_store != sessions_store:
            save_json(TODOS_PATH, todos_payload)

    # ── goals.json: re-point thread window_id to new ids ──────
    GOALS_PATH = HOME / ".cache" / "agent" / "goals.json"
    migrated_threads = 0
    if GOALS_PATH.exists() and (window_migrations or session_migrations):
        goals_payload = load_json(GOALS_PATH)
        threads = goals_payload.get("threads", {})
        for thread in threads.values():
            old_wid = str(thread.get("window_id", "")).strip()
            if old_wid and old_wid in window_migrations:
                thread["window_id"] = window_migrations[old_wid]
                migrated_threads += 1
        if migrated_threads:
            save_json(GOALS_PATH, goals_payload)

    summary = []
    if changed_agents:
        summary.append(f"{changed_agents} agent windows")
    if migrated_window_todos:
        summary.append(f"{migrated_window_todos} window todos")
    if migrated_session_todos:
        summary.append(f"{migrated_session_todos} session todos")
    if migrated_threads:
        summary.append(f"{migrated_threads} goal threads")
    if summary:
        print("restored " + ", ".join(summary))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
