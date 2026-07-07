package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/david/agent-tracker/internal/ipc"
)

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tracker-client <command|state> [flags] [args]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "command":
		runCommand(os.Args[2:])
	case "state":
		runState(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runCommand(args []string) {
	fs := flag.NewFlagSet("command", flag.ExitOnError)
	var client, session, sessionID, window, windowID, pane, summary, phase string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.StringVar(&session, "session", "", "tmux session name")
	fs.StringVar(&sessionID, "session-id", "", "tmux session id")
	fs.StringVar(&window, "window", "", "tmux window name")
	fs.StringVar(&windowID, "window-id", "", "tmux window id")
	fs.StringVar(&pane, "pane", "", "tmux pane id")
	fs.StringVar(&summary, "summary", "", "summary or completion note")
	fs.StringVar(&phase, "phase", "", "task phase")
	fs.Parse(args)
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "command name required")
		os.Exit(1)
	}
	if len(rest) > 1 {
		summary = strings.Join(rest[1:], " ")
	}
	env := ipc.Envelope{
		Kind:      "command",
		Command:   strings.TrimSpace(rest[0]),
		Client:    strings.TrimSpace(client),
		Session:   strings.TrimSpace(session),
		SessionID: strings.TrimSpace(sessionID),
		Window:    strings.TrimSpace(window),
		WindowID:  strings.TrimSpace(windowID),
		Pane:      strings.TrimSpace(pane),
		Summary:   strings.TrimSpace(summary),
		Phase:     strings.TrimSpace(phase),
	}
	if env.Summary != "" && env.Message == "" {
		env.Message = env.Summary
	}
	if err := send(&env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runState(args []string) {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	var client string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.Parse(args)
	env := ipc.Envelope{
		Kind:   "ui-register",
		Client: strings.TrimSpace(client),
	}
	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	if err := enc.Encode(&env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	var reply ipc.Envelope
	if err := dec.Decode(&reply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := json.NewEncoder(os.Stdout)
	out.SetEscapeHTML(false)
	out.Encode(&reply)
}

func send(env *ipc.Envelope) error {
	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	if err := enc.Encode(env); err != nil {
		return err
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	for {
		var reply ipc.Envelope
		if err := dec.Decode(&reply); err != nil {
			return err
		}
		if reply.Kind == "ack" {
			return nil
		}
	}
}
