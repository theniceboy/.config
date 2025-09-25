#!/usr/bin/env python3

import json
import os
import re
import shlex
import shutil
import subprocess
import sys


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: notify.py <NOTIFICATION_JSON>")
        return 1

    try:
        notification = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        return 1

    if notification.get("type") == "agent-turn-complete":
        assistant_message = notification.get("last-assistant-message")
        title = "Codex"
        subtitle = assistant_message if assistant_message else "Turn Complete!"
        input_messages = notification.get("input_messages", [])
        # Build body and strip empty/whitespace-only lines for a tighter banner
        raw_body = "\n".join(input_messages)
        non_empty_lines = [ln for ln in raw_body.splitlines() if ln.strip()]
        message = "\n".join(non_empty_lines).strip()
    else:
        print(f"not sending a push notification for: {notification}")
        return 0

    # Try to discover tmux target of the caller (session/window/pane)
    # Prefer TMUX_PANE from env; fall back to parent process TTY.
    tmux_path = shutil.which("tmux") or "/opt/homebrew/bin/tmux"
    tmux_ids = None
    title_names = None  # (session_name, window_name)
    if os.path.exists(tmux_path):
        pane = os.environ.get("TMUX_PANE", "").strip()
        try:
            if pane:
                out = subprocess.check_output(
                    [
                        tmux_path,
                        "display-message",
                        "-p",
                        "-t",
                        pane,
                        "#{session_id}:::#{window_id}:::#{pane_id}",
                    ],
                    text=True,
                ).strip()
                parts = out.split(":::")
                if len(parts) == 3:
                    tmux_ids = tuple(parts)
                # Also fetch human-readable names for the banner title
                try:
                    name_out = subprocess.check_output(
                        [
                            tmux_path,
                            "display-message",
                            "-p",
                            "-t",
                            pane,
                            "#{session_name}:::#{window_name}",
                        ],
                        text=True,
                    ).strip()
                    name_parts = name_out.split(":::")
                    if len(name_parts) == 2:
                        title_names = (name_parts[0].strip(), name_parts[1].strip())
                except Exception:
                    pass
        except Exception:
            tmux_ids = None

        if tmux_ids is None:
            try:
                ppid = os.getppid()
                tty = (
                    subprocess.check_output(
                        ["ps", "-o", "tty=", "-p", str(ppid)], text=True
                    )
                    .strip()
                    .lstrip("?")
                )
                if tty:
                    out = subprocess.check_output(
                        [
                            tmux_path,
                            "display-message",
                            "-p",
                            "-c",
                            f"/dev/{tty}",
                            "#{session_id}:::#{window_id}:::#{pane_id}",
                        ],
                        text=True,
                    ).strip()
                    parts = out.split(":::")
                    if len(parts) == 3:
                        tmux_ids = tuple(parts)
                    try:
                        name_out = subprocess.check_output(
                            [
                                tmux_path,
                                "display-message",
                                "-p",
                                "-c",
                                f"/dev/{tty}",
                                "#{session_name}:::#{window_name}",
                            ],
                            text=True,
                        ).strip()
                        name_parts = name_out.split(":::")
                        if len(name_parts) == 2:
                            title_names = (name_parts[0].strip(), name_parts[1].strip())
                    except Exception:
                        pass
            except Exception:
                tmux_ids = None

    # Prefer title = "<session_name> - <window_name>" if available
    if title_names is not None:
        sn, wn = title_names
        sn = re.sub(r'^\d+\s*-\s*', '', sn).strip()
        combined = " - ".join([p for p in (sn, wn) if p])
        if combined:
            title = combined

    # Persist latest target so tmux hotkey can jump to it later
    if tmux_ids is not None:
        try:
            run_dir = os.path.join(os.path.expanduser("~"), ".config", "agent-tracker", "run")
            os.makedirs(run_dir, exist_ok=True)
            tmp_path = os.path.join(run_dir, ".latest_notified.tmp")
            final_path = os.path.join(run_dir, "latest_notified.txt")
            with open(tmp_path, "w", encoding="utf-8") as f:
                f.write("%s:::%s:::%s\n" % tmux_ids)
            os.replace(tmp_path, final_path)
        except Exception:
            pass

    args = [
        "terminal-notifier",
        "-title",
        title,
        "-subtitle",
        subtitle,
        "-message",
        message,
        "-sound",
        "Blow",
        "-group",
        "codex",
        "-ignoreDnD",
        "-activate",
        "com.googlecode.iterm2",
    ]

    # Before showing the banner: tell the tracker server we responded,
    # attaching the assistant's response text as the completion note.
    if tmux_ids is not None and assistant_message:
        try:
            tracker_bin = shutil.which("tracker-client")
            if not tracker_bin:
                tracker_bin = os.path.join(os.path.expanduser("~"), ".config", "agent-tracker", "bin", "tracker-client")
            if os.path.exists(tracker_bin):
                sid, wid, pid = [s.strip() for s in tmux_ids]
                subprocess.check_output(
                    [
                        tracker_bin,
                        "command",
                        "-session-id",
                        sid,
                        "-window-id",
                        wid,
                        "-pane",
                        pid,
                        "-summary",
                        assistant_message,
                        "finish_task",
                    ],
                    text=True,
                )
        except Exception:
            pass

    # On click: focus iTerm2 and switch tmux to the originating pane
    if tmux_ids is not None:
        sid, wid, pid = [s.strip() for s in tmux_ids]
        switch_cmd = (
            f"{shlex.quote(tmux_path)} switch-client -t {shlex.quote(sid)}"
            f" && {shlex.quote(tmux_path)} select-window -t {shlex.quote(wid)}"
            f" && {shlex.quote(tmux_path)} select-pane -t {shlex.quote(pid)}"
        )
        args += ["-execute", "sh -lc " + shlex.quote(switch_cmd)]

    subprocess.check_output(args)

    return 0


if __name__ == "__main__":
    sys.exit(main())
