package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/david/agent-tracker/internal/ipc"
	"github.com/gdamore/tcell/v2"
)

const (
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

const spinnerInterval = 120 * time.Millisecond

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		if err := runUI(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "ui":
		if err := runUI(args); err != nil {
			log.Fatal(err)
		}
	case "command":
		if err := runCommand(args); err != nil {
			log.Fatal(err)
		}
	case "state":
		if err := runState(args); err != nil {
			log.Fatal(err)
		}
	default:
		if err := runUI(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
	}
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("tracker-client command", flag.ExitOnError)
	var client, session, sessionID, window, windowID, pane, summary string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.StringVar(&session, "session", "", "tmux session name")
	fs.StringVar(&sessionID, "session-id", "", "tmux session id")
	fs.StringVar(&window, "window", "", "tmux window name")
	fs.StringVar(&windowID, "window-id", "", "tmux window id")
	fs.StringVar(&pane, "pane", "", "tmux pane id")
	fs.StringVar(&summary, "summary", "", "summary or note payload")
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
		Kind:      "command",
		Command:   rest[0],
		Client:    client,
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

	switch env.Command {
	case "start_task", "finish_task", "acknowledge":
		ctx, err := resolveContext(env.Session, env.SessionID, env.Window, env.WindowID, env.Pane)
		if err != nil {
			return err
		}
		env.Session = ctx.SessionName
		env.SessionID = ctx.SessionID
		env.Window = ctx.WindowName
		env.WindowID = ctx.WindowID
		env.Pane = ctx.PaneID
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(&env); err != nil {
		return err
	}

	dec := json.NewDecoder(conn)
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

func runState(args []string) error {
	fs := flag.NewFlagSet("tracker-client state", flag.ExitOnError)
	var client string
	fs.StringVar(&client, "client", "", "tmux client tty")
	if err := fs.Parse(args); err != nil {
		return err
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	if err := enc.Encode(&ipc.Envelope{Kind: "ui-register", Client: client}); err != nil {
		return err
	}

	for {
		var env ipc.Envelope
		if err := dec.Decode(&env); err != nil {
			return err
		}
		if env.Kind == "state" {
			out := json.NewEncoder(os.Stdout)
			out.SetEscapeHTML(false)
			if err := out.Encode(&env); err != nil {
				return err
			}
			return nil
		}
	}
}

type tmuxContext struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
}

func resolveContext(sessionName, sessionID, windowName, windowID, paneID string) (tmuxContext, error) {
	ctx := tmuxContext{
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
		info, err := detectTmuxContext(target)
		if err != nil {
			if target == "" {
				return tmuxContext{}, err
			}
			continue
		}
		ctx = ctx.merge(info)
	}

	if ctx.SessionID == "" || ctx.WindowID == "" {
		return tmuxContext{}, fmt.Errorf("session and window identifiers required")
	}

	if ctx.SessionName == "" || ctx.WindowName == "" {
		if info, err := detectTmuxContext(ctx.WindowID); err == nil {
			ctx = ctx.merge(info)
		}
	}

	if ctx.SessionName == "" {
		ctx.SessionName = ctx.SessionID
	}
	if ctx.WindowName == "" {
		ctx.WindowName = ctx.WindowID
	}

	return ctx, nil
}

func (c tmuxContext) complete() bool {
	return c.SessionName != "" && c.SessionID != "" && c.WindowName != "" && c.WindowID != "" && c.PaneID != ""
}

func (c tmuxContext) merge(other tmuxContext) tmuxContext {
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

func detectTmuxContext(target string) (tmuxContext, error) {
	format := "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}"
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", strings.TrimSpace(target))
	}
	args = append(args, format)
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tmuxContext{}, fmt.Errorf("tmux display-message: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	parts := strings.Split(strings.TrimSpace(string(output)), ":::")
	if len(parts) != 5 {
		return tmuxContext{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(string(output)))
	}
	return tmuxContext{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
	}, nil
}

func runUI(args []string) error {
	fs := flag.NewFlagSet("tracker-client ui", flag.ExitOnError)
	var client string
	fs.StringVar(&client, "client", "", "tmux client tty")
	if err := fs.Parse(args); err != nil {
		return err
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.Clear()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	if err := enc.Encode(&ipc.Envelope{Kind: "ui-register", Client: client}); err != nil {
		return err
	}

	type state struct {
		message string
		tasks   []ipc.Task
	}
	st := state{message: "Connecting to tracker…"}

	incoming := make(chan ipc.Envelope)
	errCh := make(chan error, 1)
	go func() {
		for {
			var env ipc.Envelope
			if err := dec.Decode(&env); err != nil {
				errCh <- err
				close(incoming)
				return
			}
			incoming <- env
		}
	}()

	events := make(chan tcell.Event)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				close(events)
				return
			}
			events <- ev
		}
	}()

	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	var encMu sync.Mutex
	sendCommand := func(name string, opts ...func(*ipc.Envelope)) error {
		encMu.Lock()
		defer encMu.Unlock()
		env := ipc.Envelope{Kind: "command", Command: name, Client: client}
		for _, opt := range opts {
			opt(&env)
		}
		return enc.Encode(&env)
	}

	selected := 0
	offset := 0
	var tasks []ipc.Task

	draw := func(now time.Time) {
		screen.Clear()
		width, height := screen.Size()

		headerStyle := tcell.StyleDefault.Foreground(tcell.ColorLightCyan).Bold(true)
		subtleStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
		infoStyle := tcell.StyleDefault.Foreground(tcell.ColorSilver)

		writeStyledLine(screen, 0, 0, truncate("▌ Tracker", width), headerStyle)
		writeStyledLine(screen, 0, 1, truncate(st.message, width), subtleStyle)
		if width > 0 {
			writeStyledLine(screen, 0, 2, strings.Repeat("─", width), infoStyle)
		}

		if len(tasks) == 0 {
			offset = 0
			if height > 3 {
				writeStyledLine(screen, 0, 3, truncate("No active work. Enjoy the calm.", width), infoStyle)
			}
			screen.Show()
			return
		}

		if selected >= len(tasks) {
			selected = len(tasks) - 1
		}
		if selected < 0 {
			selected = 0
		}

		visibleRows := height - 3
		if visibleRows < 0 {
			visibleRows = 0
		}
		capacity := visibleRows / 4
		if capacity < 1 {
			capacity = 1
		}
		maxOffset := len(tasks) - capacity
		if maxOffset < 0 {
			maxOffset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		if selected < offset {
			offset = selected
		}
		if selected >= offset+capacity {
			offset = selected - capacity + 1
		}
		if offset < 0 {
			offset = 0
		}

		row := 3
		for idx := offset; idx < len(tasks); idx++ {
			t := tasks[idx]
			if row >= height {
				break
			}

			indicator := taskIndicator(t, now)
			summary := t.Summary
			if summary == "" {
				summary = "(no summary)"
			}
			line := fmt.Sprintf("%s %s", indicator, summary)

			mainStyle := tcell.StyleDefault
			switch t.Status {
			case statusInProgress:
				mainStyle = mainStyle.Foreground(tcell.ColorLightGoldenrodYellow).Bold(true)
			case statusCompleted:
				if t.Acknowledged {
					mainStyle = mainStyle.Foreground(tcell.ColorLightGreen).Bold(true)
				} else {
					mainStyle = mainStyle.Foreground(tcell.ColorFuchsia).Bold(true)
				}
			}

			if idx == selected {
				mainStyle = mainStyle.Background(tcell.ColorDarkSlateGray)
			}

			writeStyledLine(screen, 0, row, truncate(line, width), mainStyle)
			row++
			if row >= height {
				break
			}

			metaStyle := subtleStyle
			if idx == selected {
				metaStyle = metaStyle.Background(tcell.ColorDarkSlateGray)
			}

			meta := fmt.Sprintf("   %s · %s · %s", t.Session, t.Window, liveDuration(t, now))
			if t.Status == statusCompleted {
				if !t.Acknowledged {
					meta += " • awaiting review"
				} else if t.CompletedAt != "" {
					if completed, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
						meta += fmt.Sprintf(" • finished %s", completed.Format("15:04"))
					}
				}
			}
			writeStyledLine(screen, 0, row, truncate(meta, width), metaStyle)
			row++

			if t.CompletionNote != "" && row < height {
				noteStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSteelBlue)
				if idx == selected {
					noteStyle = noteStyle.Background(tcell.ColorDarkSlateGray)
				}
				note := fmt.Sprintf("   ↳ %s", t.CompletionNote)
				writeStyledLine(screen, 0, row, truncate(note, width), noteStyle)
				row++
			}

			if row < height {
				row++
			}
		}

		screen.Show()
	}

	draw(time.Now())

	for {
		select {
		case env, ok := <-incoming:
			if !ok {
				return <-errCh
			}
			switch env.Kind {
			case "state":
				st.message = env.Message
				tasks = make([]ipc.Task, len(env.Tasks))
				copy(tasks, env.Tasks)
				sortTasks(tasks)
				if len(tasks) == 0 {
					selected = 0
				} else if selected >= len(tasks) {
					selected = len(tasks) - 1
				}
				draw(time.Now())
			case "ack":
			default:
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			switch tev := ev.(type) {
			case *tcell.EventKey:
				if tev.Key() == tcell.KeyEnter {
					if len(tasks) > 0 && selected < len(tasks) {
						task := tasks[selected]
						if err := sendCommand("focus_task", func(env *ipc.Envelope) {
							env.SessionID = task.SessionID
							env.WindowID = task.WindowID
							env.Pane = task.Pane
						}); err != nil {
							return err
						}
					}
					if err := sendCommand("hide"); err != nil {
						return err
					}
					return nil
				}
				if tev.Key() == tcell.KeyCtrlC {
					return sendCommand("hide")
				}
				if tev.Modifiers()&tcell.ModAlt != 0 {
					r := unicode.ToLower(tev.Rune())
					switch r {
					case 't':
						if err := sendCommand("hide"); err != nil {
							return err
						}
					case 'n':
						if err := sendCommand("move_left"); err != nil {
							return err
						}
					case 'i':
						if err := sendCommand("move_right"); err != nil {
							return err
						}
					case 'u':
						if selected > 0 {
							selected--
							draw(time.Now())
						}
					case 'e':
						if selected < len(tasks)-1 {
							selected++
							draw(time.Now())
						}
					}
				} else if tev.Key() == tcell.KeyRune {
					r := tev.Rune()
					if unicode.ToLower(r) == 'd' && (tev.Modifiers()&tcell.ModShift != 0 || unicode.IsUpper(r)) {
						if len(tasks) > 0 && selected < len(tasks) {
							task := tasks[selected]
							if err := sendCommand("delete_task", func(env *ipc.Envelope) {
								env.SessionID = task.SessionID
								env.WindowID = task.WindowID
								env.Pane = task.Pane
								env.Session = task.Session
								env.Window = task.Window
							}); err != nil {
								return err
							}
						}
						continue
					}
					r = unicode.ToLower(r)
					if r == 'u' {
						if selected > 0 {
							selected--
							draw(time.Now())
						}
					} else if r == 'e' {
						if selected < len(tasks)-1 {
							selected++
							draw(time.Now())
						}
					}
				}
				if tev.Key() == tcell.KeyEscape {
					if err := sendCommand("hide"); err != nil {
						return err
					}
				}
			case *tcell.EventResize:
				draw(time.Now())
			}
		case now := <-ticker.C:
			draw(now)
		case err := <-errCh:
			if err == nil || errors.Is(err, io.EOF) || strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			return err
		}
	}
}

func sortTasks(tasks []ipc.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		ti, tj := tasks[i], tasks[j]
		ranki := taskStatusRank(ti.Status)
		rankj := taskStatusRank(tj.Status)
		if ranki != rankj {
			return ranki < rankj
		}
		switch ti.Status {
		case statusInProgress:
			return ti.StartedAt < tj.StartedAt
		case statusCompleted:
			if ti.Acknowledged != tj.Acknowledged {
				return !ti.Acknowledged && tj.Acknowledged
			}
			if ci, hasCi := parseTimestamp(ti.CompletedAt); hasCi {
				if cj, hasCj := parseTimestamp(tj.CompletedAt); hasCj {
					if !ci.Equal(cj) {
						return ci.After(cj)
					}
				} else {
					return true
				}
			} else if _, hasCj := parseTimestamp(tj.CompletedAt); hasCj {
				return false
			}
			si, hasSi := parseTimestamp(ti.StartedAt)
			sj, hasSj := parseTimestamp(tj.StartedAt)
			if hasSi && hasSj && !si.Equal(sj) {
				return si.After(sj)
			}
			if hasSi != hasSj {
				return hasSi
			}
			return ti.StartedAt > tj.StartedAt
		default:
			return ti.StartedAt < tj.StartedAt
		}
	})
}

func taskStatusRank(status string) int {
	switch status {
	case statusInProgress:
		return 0
	case statusCompleted:
		return 1
	default:
		return 2
	}
}

func parseTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func taskIndicator(t ipc.Task, now time.Time) string {
	switch t.Status {
	case statusInProgress:
		idx := int(now.UnixNano()/int64(spinnerInterval)) % len(spinnerFrames)
		return string(spinnerFrames[idx])
	case statusCompleted:
		if t.Acknowledged {
			return "✓"
		}
		return "⚑"
	default:
		return "?"
	}
}

func liveDuration(t ipc.Task, now time.Time) string {
	if t.StartedAt == "" {
		return "0s"
	}
	start, err := time.Parse(time.RFC3339, t.StartedAt)
	if err != nil {
		return formatDuration(t.DurationSeconds)
	}
	if t.Status == statusCompleted {
		if t.CompletedAt != "" {
			if end, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
				return formatDuration(end.Sub(start).Seconds())
			}
		}
		return formatDuration(t.DurationSeconds)
	}
	return formatDuration(time.Since(start).Seconds())
}

func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	if d >= 99*time.Hour {
		return ">=99h"
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	secondsPart := (d % time.Minute) / time.Second
	if hours > 0 {
		return fmt.Sprintf("%02dh%02dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%02dm%02ds", minutes, secondsPart)
	}
	return fmt.Sprintf("%02ds", secondsPart)
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func writeStyledLine(s tcell.Screen, x, y int, text string, style tcell.Style) {
	width, _ := s.Size()
	if x >= width {
		return
	}
	runes := []rune(text)
	limit := width - x
	for i := 0; i < limit; i++ {
		r := rune(' ')
		if i < len(runes) {
			r = runes[i]
		}
		s.SetContent(x+i, y, r, nil, style)
	}
}

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}
