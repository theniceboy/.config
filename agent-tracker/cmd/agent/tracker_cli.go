package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/david/agent-tracker/internal/ipc"
)

type trackerTmuxContext struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
}

func runTracker(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent tracker <command|state>")
	}
	switch args[0] {
	case "command":
		return runTrackerCommand(args[1:])
	case "state":
		return runTrackerState(args[1:])
	default:
		return fmt.Errorf("unknown tracker subcommand: %s", args[0])
	}
}

func runTrackerCommand(args []string) error {
	fs := flag.NewFlagSet("agent tracker command", flag.ExitOnError)
	var client, session, sessionID, window, windowID, pane, summary string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.StringVar(&session, "session", "", "tmux session name")
	fs.StringVar(&sessionID, "session-id", "", "tmux session id")
	fs.StringVar(&window, "window", "", "tmux window name")
	fs.StringVar(&windowID, "window-id", "", "tmux window id")
	fs.StringVar(&pane, "pane", "", "tmux pane id")
	fs.StringVar(&summary, "summary", "", "summary or completion note")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("command name required")
	}
	if len(rest) > 1 {
		summary = strings.Join(rest[1:], " ")
	}
	env := ipc.Envelope{
		Client:    strings.TrimSpace(client),
		Session:   strings.TrimSpace(session),
		SessionID: strings.TrimSpace(sessionID),
		Window:    strings.TrimSpace(window),
		WindowID:  strings.TrimSpace(windowID),
		Pane:      strings.TrimSpace(pane),
		Summary:   strings.TrimSpace(summary),
	}
	if env.Summary != "" {
		env.Message = env.Summary
	}
	command := strings.TrimSpace(rest[0])
	switch command {
	case "start_task", "finish_task", "update_task", "acknowledge", "delete_task", "notify":
		ctx, err := resolveTrackerContext(env.Session, env.SessionID, env.Window, env.WindowID, env.Pane)
		if err != nil {
			return err
		}
		env.Session = ctx.SessionName
		env.SessionID = ctx.SessionID
		env.Window = ctx.WindowName
		env.WindowID = ctx.WindowID
		env.Pane = ctx.PaneID
	}
	return sendTrackerCommand(command, &env)
}

func runTrackerState(args []string) error {
	fs := flag.NewFlagSet("agent tracker state", flag.ExitOnError)
	var client string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	env, err := trackerLoadState(client)
	if err != nil {
		return err
	}
	out := json.NewEncoder(os.Stdout)
	out.SetEscapeHTML(false)
	return out.Encode(env)
}

func (c trackerTmuxContext) complete() bool {
	return strings.TrimSpace(c.SessionID) != "" && strings.TrimSpace(c.WindowID) != "" && strings.TrimSpace(c.PaneID) != ""
}

func resolveTrackerContext(sessionName, sessionID, windowName, windowID, paneID string) (trackerTmuxContext, error) {
	ctx := trackerTmuxContext{
		SessionName: strings.TrimSpace(sessionName),
		SessionID:   strings.TrimSpace(sessionID),
		WindowName:  strings.TrimSpace(windowName),
		WindowID:    strings.TrimSpace(windowID),
		PaneID:      strings.TrimSpace(paneID),
	}
	fetchOrder := []string{}
	if ctx.PaneID != "" {
		fetchOrder = append(fetchOrder, ctx.PaneID)
	}
	fetchOrder = append(fetchOrder, "")
	for _, target := range fetchOrder {
		if ctx.complete() {
			break
		}
		info, err := detectTrackerTmuxContext(target)
		if err != nil {
			if target == "" {
				return trackerTmuxContext{}, err
			}
			continue
		}
		ctx = ctx.merge(info)
	}
	if ctx.SessionID == "" || ctx.WindowID == "" {
		return trackerTmuxContext{}, fmt.Errorf("session and window identifiers required")
	}
	return ctx, nil
}

func (c trackerTmuxContext) merge(other trackerTmuxContext) trackerTmuxContext {
	if c.SessionName == "" {
		c.SessionName = other.SessionName
	}
	if c.SessionID == "" {
		c.SessionID = other.SessionID
	}
	if c.WindowName == "" {
		c.WindowName = other.WindowName
	}
	if c.WindowID == "" {
		c.WindowID = other.WindowID
	}
	if c.PaneID == "" {
		c.PaneID = other.PaneID
	}
	return c
}

func detectTrackerTmuxContext(target string) (trackerTmuxContext, error) {
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", strings.TrimSpace(target))
	}
	args = append(args, "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}")
	out, err := runTmuxOutput(args...)
	if err != nil {
		return trackerTmuxContext{}, err
	}
	parts := strings.Split(strings.TrimSpace(out), ":::")
	if len(parts) != 5 {
		return trackerTmuxContext{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(out))
	}
	return trackerTmuxContext{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
	}, nil
}
