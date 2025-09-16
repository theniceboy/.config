package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/david/agent-tracker/internal/ipc"
)

type position string

const (
	posTopRight    position = "top-right"
	posTopLeft     position = "top-left"
	posBottomLeft  position = "bottom-left"
	posBottomRight position = "bottom-right"
	posCenter      position = "center"
)

const (
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

type taskRecord struct {
	SessionID      string
	WindowID       string
	Pane           string
	Summary        string
	CompletionNote string
	StartedAt      time.Time
	CompletedAt    *time.Time
	Status         string
	Acknowledged   bool
}

type tmuxTarget struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
}

type uiSubscriber struct {
	enc *json.Encoder
}

type server struct {
	mu          sync.Mutex
	socketPath  string
	visible     bool
	pos         position
	width       int
	height      int
	tasks       map[string]*taskRecord
	subscribers map[*uiSubscriber]struct{}
}

func newServer() *server {
	return &server{
		socketPath:  socketPath(),
		pos:         posTopRight,
		width:       84,
		height:      24,
		tasks:       make(map[string]*taskRecord),
		subscribers: make(map[*uiSubscriber]struct{}),
	}
}

func main() {
	srv := newServer()
	if err := srv.run(); err != nil {
		log.Fatal(err)
	}
}

func (s *server) run() error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(s.socketPath); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		return err
	}
	defer ln.Close()
	defer os.Remove(s.socketPath)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				errCh <- err
				return
			}
			go s.handleConn(conn)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		return fmt.Errorf("tracker-server stopped: %s", sig)
	}
}

func (s *server) handleConn(conn net.Conn) {
	defer conn.Close()

	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)

	var sub *uiSubscriber
	defer func() {
		if sub != nil {
			s.removeSubscriber(sub)
		}
	}()

	for {
		var env ipc.Envelope
		if err := dec.Decode(&env); err != nil {
			return
		}
		switch env.Kind {
		case "command":
			if err := s.handleCommand(env); err != nil {
				log.Printf("command error: %v", err)
			}
			reply := ipc.Envelope{Kind: "ack"}
			if err := enc.Encode(&reply); err != nil {
				return
			}
		case "ui-register":
			if sub == nil {
				sub = &uiSubscriber{enc: enc}
				s.addSubscriber(sub)
			}
			if err := s.sendStateTo(sub); err != nil {
				return
			}
		default:
			log.Printf("unknown message: %+v", env)
		}
	}
}

func (s *server) handleCommand(env ipc.Envelope) error {
	switch env.Command {
	case "toggle":
		return s.toggle()
	case "show":
		return s.show()
	case "hide":
		return s.hide()
	case "refresh":
		return s.refresh()
	case "move_left":
		return s.moveLeft()
	case "move_right":
		return s.moveRight()
	case "move_up":
		return s.moveUp()
	case "move_down":
		return s.moveDown()
	case "center":
		return s.center()
	case "start_task":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		summary := firstNonEmpty(env.Summary, env.Message)
		if summary == "" {
			return fmt.Errorf("start_task requires summary")
		}
		if err := s.startTask(target, summary); err != nil {
			return err
		}
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "finish_task":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		note := firstNonEmpty(env.Summary, env.Message)
		if err := s.finishTask(target, note); err != nil {
			return err
		}
		s.notifyResponded(target)
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "acknowledge":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		if err := s.acknowledgeTask(target.SessionID, target.WindowID, target.PaneID); err != nil {
			return err
		}
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "delete_task":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		if err := s.deleteTask(target.SessionID, target.WindowID, target.PaneID); err != nil {
			return err
		}
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "focus_task":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		if err := s.focusTask(env.Client, target); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", env.Command)
	}
}

func (s *server) startTask(target tmuxTarget, summary string) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskKey(target.SessionID, target.WindowID, target.PaneID)] = &taskRecord{
		SessionID:    target.SessionID,
		WindowID:     target.WindowID,
		Pane:         target.PaneID,
		Summary:      summary,
		StartedAt:    now,
		Status:       statusInProgress,
		Acknowledged: true,
	}
	return nil
}

func (s *server) finishTask(target tmuxTarget, note string) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(target.SessionID, target.WindowID, target.PaneID)
	t, ok := s.tasks[key]
	if !ok {
		t = &taskRecord{SessionID: target.SessionID, WindowID: target.WindowID, Pane: target.PaneID, StartedAt: now}
		s.tasks[key] = t
	}
	if t.Summary == "" {
		t.Summary = note
	}
	t.Status = statusCompleted
	t.CompletedAt = &now
	if note != "" {
		t.CompletionNote = note
	}
	t.Acknowledged = false
	return nil
}

func (s *server) acknowledgeTask(sessionID, windowID, paneID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[taskKey(sessionID, windowID, paneID)]; ok {
		t.Acknowledged = true
	}
	return nil
}

func (s *server) deleteTask(sessionID, windowID, paneID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, taskKey(sessionID, windowID, paneID))
	return nil
}

func (s *server) notifyResponded(target tmuxTarget) {
	summary := strings.TrimSpace(s.summaryForTask(target.SessionID, target.WindowID, target.PaneID))
	if summary == "" {
		summary = "Task marked complete"
	}
	action := notificationActionForTarget(target)
	if err := sendSystemNotification("Tracker", summary, action); err != nil {
		log.Printf("notification error: %v", err)
	}
}

func (s *server) summaryForTask(sessionID, windowID, paneID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[taskKey(sessionID, windowID, paneID)]; ok {
		if note := strings.TrimSpace(t.CompletionNote); note != "" {
			return note
		}
		if summary := strings.TrimSpace(t.Summary); summary != "" {
			return summary
		}
	}
	return ""
}

func (s *server) focusTask(client string, target tmuxTarget) error {
	client = strings.TrimSpace(client)
	if client == "" {
		return fmt.Errorf("client required for focus_task")
	}
	if target.SessionID == "" || target.WindowID == "" {
		return fmt.Errorf("session and window required for focus_task")
	}
	if err := runTmux("switch-client", "-c", client, "-t", target.SessionID); err != nil {
		return err
	}
	if err := runTmux("select-window", "-t", target.WindowID); err != nil {
		return err
	}
	if err := runTmux("select-pane", "-t", target.PaneID); err != nil {
		return err
	}
	return s.hide()
}

func (s *server) toggle() error {
	s.mu.Lock()
	visible := s.visible
	s.mu.Unlock()
	if visible {
		return s.hide()
	}
	return s.show()
}

func (s *server) show() error {
	s.mu.Lock()
	s.visible = true
	s.mu.Unlock()
	return s.openOnClients(false)
}

func (s *server) hide() error {
	s.mu.Lock()
	s.visible = false
	s.mu.Unlock()
	return s.closeOnClients()
}

func (s *server) refresh() error {
	s.mu.Lock()
	visible := s.visible
	s.mu.Unlock()
	if !visible {
		return nil
	}
	return s.openOnClients(true)
}

func (s *server) moveLeft() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.pos {
	case posTopRight:
		s.pos = posTopLeft
	case posBottomRight:
		s.pos = posBottomLeft
	case posCenter:
		s.pos = posTopLeft
	default:
		return nil
	}
	s.broadcastStateAsync()
	s.asyncRefresh()
	return nil
}

func (s *server) moveRight() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.pos {
	case posTopLeft:
		s.pos = posTopRight
	case posBottomLeft:
		s.pos = posBottomRight
	case posCenter:
		s.pos = posTopRight
	default:
		return nil
	}
	s.broadcastStateAsync()
	s.asyncRefresh()
	return nil
}

func (s *server) moveUp() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.pos {
	case posBottomLeft:
		s.pos = posTopLeft
	case posBottomRight:
		s.pos = posTopRight
	case posCenter:
		s.pos = posTopRight
	default:
		return nil
	}
	s.broadcastStateAsync()
	s.asyncRefresh()
	return nil
}

func (s *server) moveDown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.pos {
	case posTopLeft:
		s.pos = posBottomLeft
	case posTopRight:
		s.pos = posBottomRight
	case posCenter:
		s.pos = posBottomRight
	default:
		return nil
	}
	s.broadcastStateAsync()
	s.asyncRefresh()
	return nil
}

func (s *server) center() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pos == posCenter {
		return nil
	}
	s.pos = posCenter
	s.broadcastStateAsync()
	s.asyncRefresh()
	return nil
}

func (s *server) asyncRefresh() {
	go func() {
		if err := s.refresh(); err != nil {
			log.Printf("refresh error: %v", err)
		}
	}()
}

func (s *server) broadcastStateAsync() {
	go s.broadcastState()
}

func (s *server) broadcastState() {
	env := s.buildStateEnvelope()
	if env == nil {
		return
	}
	s.mu.Lock()
	subs := make([]*uiSubscriber, 0, len(s.subscribers))
	for sub := range s.subscribers {
		subs = append(subs, sub)
	}
	s.mu.Unlock()

	for _, sub := range subs {
		if err := sub.enc.Encode(env); err != nil {
			s.removeSubscriber(sub)
		}
	}
}

func (s *server) statusRefreshAsync() {
	go func() {
		if err := runTmux("refresh-client", "-S"); err != nil {
			log.Printf("status refresh error: %v", err)
		}
	}()
}

func (s *server) openOnClients(refresh bool) error {
	clients, err := listClients()
	if err != nil {
		return err
	}
	for _, client := range clients {
		client := client
		go func() {
			if refresh {
				if err := runTmux("display-popup", "-C", "-c", client); err != nil {
					log.Printf("tracker: close popup %s failed: %v", client, err)
				}
			}
			if err := s.openPopup(client); err != nil {
				log.Printf("tracker: open popup %s failed: %v", client, err)
			}
		}()
	}
	return nil
}

func (s *server) closeOnClients() error {
	clients, err := listClients()
	if err != nil {
		return err
	}
	for _, client := range clients {
		if err := runTmux("display-popup", "-C", "-c", client); err != nil {
			log.Printf("close popup %s: %v", client, err)
		}
	}
	return nil
}

func (s *server) openPopup(client string) error {
	width, height := s.popupSize()
	x, y, err := s.popupPosition(client, width, height)
	if err != nil {
		return err
	}
	args := []string{
		"display-popup",
		"-E",
		"-c", client,
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
		"-x", strconv.Itoa(x),
		"-y", strconv.Itoa(y),
	}
	bin, err := trackerClientBinary()
	if err != nil {
		return err
	}
	args = append(args, bin, "ui", "--client", client)
	return runTmux(args...)
}

func (s *server) popupSize() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.width, s.height
}

func (s *server) popupPosition(client string, width, height int) (int, int, error) {
	cols, rows, err := clientSize(client)
	if err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	pos := s.pos
	s.mu.Unlock()

	if width >= cols {
		width = cols - 2
	}
	if height >= rows {
		height = rows - 1
	}

	var x, y int
	switch pos {
	case posTopRight:
		x = cols - width
		y = 0
	case posTopLeft:
		x = 0
		y = 0
	case posBottomLeft:
		x = 0
		y = rows - height
	case posBottomRight:
		x = cols - width
		y = rows - height
	case posCenter:
		x = (cols - width) / 2
		y = (rows - height) / 2
	default:
		x = cols - width
		y = 0
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, nil
}

func (s *server) sendState(enc *json.Encoder) {
	env := s.buildStateEnvelope()
	if env == nil {
		return
	}
	if err := enc.Encode(env); err != nil {
		log.Printf("state send error: %v", err)
	}
}

func (s *server) sendStateTo(sub *uiSubscriber) error {
	env := s.buildStateEnvelope()
	if env == nil {
		return nil
	}
	if err := sub.enc.Encode(env); err != nil {
		s.removeSubscriber(sub)
		return err
	}
	return nil
}

func (s *server) buildStateEnvelope() *ipc.Envelope {
	s.mu.Lock()
	visible := s.visible
	pos := s.pos
	copies := make([]*taskRecord, 0, len(s.tasks))
	for _, task := range s.tasks {
		copy := *task
		copies = append(copies, &copy)
	}
	s.mu.Unlock()

	now := time.Now()
	tasks := make([]ipc.Task, 0, len(copies))
	nameCache := make(map[string][2]string)
	for _, t := range copies {
		started := ""
		if !t.StartedAt.IsZero() {
			started = t.StartedAt.Format(time.RFC3339)
		}
		completed := ""
		var duration time.Duration
		if t.CompletedAt != nil {
			completed = t.CompletedAt.Format(time.RFC3339)
			duration = t.CompletedAt.Sub(t.StartedAt)
		} else {
			duration = now.Sub(t.StartedAt)
		}
		if duration < 0 {
			duration = 0
		}
		var names [2]string
		if cached, ok := nameCache[t.WindowID]; ok {
			names = cached
		} else {
			sessName, winName, err := tmuxNamesForWindow(t.WindowID)
			if err != nil {
				sessName = t.SessionID
				winName = t.WindowID
			}
			names = [2]string{sessName, winName}
			nameCache[t.WindowID] = names
		}

		tasks = append(tasks, ipc.Task{
			SessionID:       t.SessionID,
			Session:         names[0],
			WindowID:        t.WindowID,
			Window:          names[1],
			Pane:            t.Pane,
			Status:          t.Status,
			Summary:         t.Summary,
			CompletionNote:  t.CompletionNote,
			StartedAt:       started,
			CompletedAt:     completed,
			DurationSeconds: duration.Seconds(),
			Acknowledged:    t.Acknowledged,
		})
	}

	msg := stateSummary(tasks)
	return &ipc.Envelope{Kind: "state", Visible: &visible, Position: string(pos), Message: msg, Tasks: tasks}
}

func (s *server) addSubscriber(sub *uiSubscriber) {
	s.mu.Lock()
	s.subscribers[sub] = struct{}{}
	s.mu.Unlock()
}

func (s *server) removeSubscriber(sub *uiSubscriber) {
	s.mu.Lock()
	delete(s.subscribers, sub)
	s.mu.Unlock()
}

type notificationAction struct {
	Command     string
	ActivateApp string
}

func notificationActionForTarget(target tmuxTarget) *notificationAction {
	session := strings.TrimSpace(target.SessionID)
	window := strings.TrimSpace(target.WindowID)
	pane := strings.TrimSpace(target.PaneID)
	if session == "" || window == "" || pane == "" {
		return nil
	}
	cmd := fmt.Sprintf("tmux switch-client -t %s && tmux select-window -t %s && tmux select-pane -t %s",
		strconv.Quote(session), strconv.Quote(window), strconv.Quote(pane))
	return &notificationAction{
		Command:     "sh -lc " + strconv.Quote(cmd),
		ActivateApp: "com.apple.Terminal",
	}
}

func sendSystemNotification(title, message string, action *notificationAction) error {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Tracker"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = title
	}
	switch runtime.GOOS {
	case "darwin":
		if bin, err := exec.LookPath("terminal-notifier"); err == nil {
			args := []string{"-title", title, "-message", message, "-group", "agent-tracker"}
			if action != nil {
				if strings.TrimSpace(action.Command) != "" {
					args = append(args, "-execute", action.Command)
				}
				if strings.TrimSpace(action.ActivateApp) != "" {
					args = append(args, "-activate", action.ActivateApp)
				}
			}
			cmd := exec.Command(bin, args...)
			if err := cmd.Run(); err != nil {
				return err
			}
			return nil
		}
		scriptLines := []string{fmt.Sprintf("display notification %s with title %s", strconv.Quote(message), strconv.Quote(title))}
		cmd := exec.Command("osascript", "-e", strings.Join(scriptLines, "\n"))
		if err := cmd.Run(); err != nil {
			return err
		}
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return err
		}
		cmd := exec.Command("notify-send", title, message)
		if err := cmd.Run(); err != nil {
			return err
		}
	default:
		return nil
	}
	return nil
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return fmt.Errorf("tmux %s: %v: %s", strings.Join(args, " "), err, trimmed)
		}
		return fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func tmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func clientSize(client string) (int, int, error) {
	colsStr, err := tmuxDisplay(client, "#{client_width}")
	if err != nil {
		return 0, 0, err
	}
	rowsStr, err := tmuxDisplay(client, "#{client_height}")
	if err != nil {
		return 0, 0, err
	}
	cols, err := strconv.Atoi(strings.TrimSpace(colsStr))
	if err != nil {
		return 0, 0, err
	}
	rows, err := strconv.Atoi(strings.TrimSpace(rowsStr))
	if err != nil {
		return 0, 0, err
	}
	return cols, rows, nil
}

func tmuxDisplay(client, format string) (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "-c", client, format)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("display-message %s: %w (%s)", format, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func listClients() ([]string, error) {
	cmd := exec.Command("tmux", "list-clients", "-F", "#{client_tty}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var clients []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			clients = append(clients, trimmed)
		}
	}
	return clients, nil
}

func trackerClientBinary() (string, error) {
	base := filepath.Join(os.Getenv("HOME"), ".config", "agent-tracker", "bin", "tracker-client")
	if info, err := os.Stat(base); err == nil && !info.IsDir() {
		return base, nil
	}
	path, err := exec.LookPath("tracker-client")
	if err != nil {
		return "", fmt.Errorf("tracker-client binary not found")
	}
	return path, nil
}

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}

func taskKey(sessionID, windowID, paneID string) string {
	return strings.Join([]string{sessionID, windowID, paneID}, "|")
}

func requireSessionWindow(env ipc.Envelope) (tmuxTarget, error) {
	ctx := tmuxTarget{
		SessionName: strings.TrimSpace(env.Session),
		SessionID:   strings.TrimSpace(env.SessionID),
		WindowName:  strings.TrimSpace(env.Window),
		WindowID:    strings.TrimSpace(env.WindowID),
		PaneID:      strings.TrimSpace(env.Pane),
	}

	fetchOrder := []string{}
	if ctx.PaneID != "" {
		fetchOrder = append(fetchOrder, ctx.PaneID)
	}
	if ctx.WindowID != "" {
		fetchOrder = append(fetchOrder, ctx.WindowID)
	}
	fetchOrder = append(fetchOrder, "")

	for _, target := range fetchOrder {
		if ctx.complete() {
			break
		}
		info, err := detectTmuxTarget(target)
		if err != nil {
			if target == "" {
				return tmuxTarget{}, err
			}
			continue
		}
		ctx = ctx.merge(info)
	}

	if ctx.SessionID == "" || ctx.WindowID == "" {
		return tmuxTarget{}, fmt.Errorf("session and window required")
	}

	if ctx.SessionName == "" || ctx.WindowName == "" {
		if info, err := detectTmuxTarget(ctx.WindowID); err == nil {
			ctx = ctx.merge(info)
		}
	}

	if ctx.SessionName == "" {
		ctx.SessionName = ctx.SessionID
	}
	if ctx.WindowName == "" {
		ctx.WindowName = ctx.WindowID
	}
	if strings.TrimSpace(ctx.PaneID) == "" {
		return tmuxTarget{}, fmt.Errorf("pane identifier required")
	}

	return ctx, nil
}

func (t tmuxTarget) complete() bool {
	return t.SessionName != "" && t.SessionID != "" && t.WindowName != "" && t.WindowID != "" && t.PaneID != ""
}

func (t tmuxTarget) merge(other tmuxTarget) tmuxTarget {
	if t.SessionName == "" {
		t.SessionName = other.SessionName
	}
	if t.SessionID == "" {
		t.SessionID = other.SessionID
	}
	if t.WindowName == "" {
		t.WindowName = other.WindowName
	}
	if t.WindowID == "" {
		t.WindowID = other.WindowID
	}
	if t.PaneID == "" {
		t.PaneID = other.PaneID
	}
	return t
}

func detectTmuxTarget(target string) (tmuxTarget, error) {
	format := "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}"
	output, err := tmuxQuery(strings.TrimSpace(target), format)
	if err != nil {
		return tmuxTarget{}, err
	}
	parts := strings.Split(strings.TrimSpace(output), ":::")
	if len(parts) != 5 {
		return tmuxTarget{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(output))
	}
	return tmuxTarget{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
	}, nil
}

func tmuxNamesForWindow(windowID string) (string, string, error) {
	if strings.TrimSpace(windowID) == "" {
		return "", "", fmt.Errorf("window id required")
	}
	output, err := tmuxQuery(strings.TrimSpace(windowID), "#{session_name}:::#{window_name}")
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(output), ":::")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(output))
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func tmuxQuery(target, format string) (string, error) {
	args := []string{"display-message", "-p"}
	if target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, format)
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stateSummary(tasks []ipc.Task) string {
	inProgress := 0
	waiting := 0
	for _, t := range tasks {
		switch t.Status {
		case statusInProgress:
			inProgress++
		case statusCompleted:
			if !t.Acknowledged {
				waiting++
			}
		}
	}
	return fmt.Sprintf("Active %d · Waiting %d · %s", inProgress, waiting, time.Now().Format(time.Kitchen))
}
