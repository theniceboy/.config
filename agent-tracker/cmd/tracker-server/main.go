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

const (
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

type taskRecord struct {
	SessionID      string
	SessionName    string
	WindowID       string
	WindowName     string
	Pane           string
	Summary        string
	CompletionNote string
	StartedAt      time.Time
	CompletedAt    *time.Time
	Status         string
	Acknowledged   bool
}

type storedSettings struct {
	NotificationsEnabled *bool `json:"notifications_enabled,omitempty"`
}

type tmuxTarget struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
	WindowIndex string
	PaneIndex   string
}

type uiSubscriber struct {
	enc *json.Encoder
}

type server struct {
	mu                   sync.Mutex
	socketPath           string
	notificationsEnabled bool
	tasks                map[string]*taskRecord
	subscribers          map[*uiSubscriber]struct{}
	settingsPath         string
}

func newServer() *server {
	return &server{
		socketPath:           socketPath(),
		notificationsEnabled: true,
		tasks:                make(map[string]*taskRecord),
		subscribers:          make(map[*uiSubscriber]struct{}),
		settingsPath:         settingsStorePath(),
	}
}

func main() {
	srv := newServer()
	if err := srv.run(); err != nil {
		log.Fatal(err)
	}
}

func (s *server) run() error {
	if err := s.loadSettings(); err != nil {
		return err
	}
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
		notify, err := s.finishTask(target, note)
		if err != nil {
			return err
		}
		if notify && s.notificationsAreEnabled() {
			go s.notifyResponded(target)
		}
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "notify":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		message := firstNonEmpty(env.Summary, env.Message)
		if message == "" {
			return fmt.Errorf("notify requires summary")
		}
		if s.notificationsAreEnabled() {
			if err := sendSystemNotification(notificationTitleForTarget(target), message, notificationActionForTarget(target)); err != nil {
				return err
			}
		}
		return nil
	case "update_task":
		target, err := requireSessionWindow(env)
		if err != nil {
			return err
		}
		summary := firstNonEmpty(env.Summary, env.Message)
		if summary == "" {
			return fmt.Errorf("update_task requires summary")
		}
		if err := s.updateTaskSummary(target, summary); err != nil {
			return err
		}
		s.broadcastStateAsync()
		s.statusRefreshAsync()
		return nil
	case "notifications_toggle":
		enabled, err := s.toggleNotifications()
		if err != nil {
			return err
		}
		if client := strings.TrimSpace(env.Client); client != "" {
			status := "OFF"
			if enabled {
				status = "ON"
			}
			if err := runTmux("display-message", "-c", client, "push notifications: "+status); err != nil {
				log.Printf("notification toggle message error: %v", err)
			}
		}
		s.broadcastStateAsync()
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
	default:
		return fmt.Errorf("unknown command %q", env.Command)
	}
}

func (s *server) startTask(target tmuxTarget, summary string) error {
	if target.SessionID == "" || target.WindowID == "" {
		return fmt.Errorf("cannot create task: missing session or window ID")
	}
	target = normalizeTargetNames(target)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(target.SessionID, target.WindowID, target.PaneID)
	t, ok := s.tasks[key]
	if !ok {
		s.tasks[key] = &taskRecord{
			SessionID:    target.SessionID,
			SessionName:  strings.TrimSpace(target.SessionName),
			WindowID:     target.WindowID,
			WindowName:   strings.TrimSpace(target.WindowName),
			Pane:         target.PaneID,
			Summary:      summary,
			StartedAt:    now,
			Status:       statusInProgress,
			Acknowledged: true,
		}
		return nil
	}
	mergeTaskNamesFromTarget(t, target)
	if !(t.Status == statusInProgress && strings.TrimSpace(t.Summary) != "") {
		t.Summary = summary
	}
	t.StartedAt = now
	t.Status = statusInProgress
	t.CompletedAt = nil
	t.CompletionNote = ""
	t.Acknowledged = true
	return nil
}

func (s *server) updateTaskSummary(target tmuxTarget, summary string) error {
	if target.SessionID == "" || target.WindowID == "" {
		return fmt.Errorf("cannot update task: missing session or window ID")
	}
	target = normalizeTargetNames(target)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(target.SessionID, target.WindowID, target.PaneID)
	t, ok := s.tasks[key]
	if !ok {
		t = &taskRecord{
			SessionID:    target.SessionID,
			SessionName:  strings.TrimSpace(target.SessionName),
			WindowID:     target.WindowID,
			WindowName:   strings.TrimSpace(target.WindowName),
			Pane:         target.PaneID,
			StartedAt:    now,
			Status:       statusInProgress,
			Acknowledged: true,
		}
		s.tasks[key] = t
	}
	mergeTaskNamesFromTarget(t, target)
	t.Summary = summary
	if t.Status == "" {
		t.Status = statusInProgress
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = now
	}
	return nil
}

func (s *server) finishTask(target tmuxTarget, note string) (bool, error) {
	if target.SessionID == "" || target.WindowID == "" {
		return false, nil // silently ignore - pane likely died
	}
	target = normalizeTargetNames(target)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(target.SessionID, target.WindowID, target.PaneID)
	t, ok := s.tasks[key]
	wasCompleted := false
	if !ok {
		t = &taskRecord{
			SessionID:   target.SessionID,
			SessionName: strings.TrimSpace(target.SessionName),
			WindowID:    target.WindowID,
			WindowName:  strings.TrimSpace(target.WindowName),
			Pane:        target.PaneID,
			StartedAt:   now,
		}
		s.tasks[key] = t
	} else {
		wasCompleted = t.Status == statusCompleted
	}
	if t.Summary == "" {
		t.Summary = note
	}
	mergeTaskNamesFromTarget(t, target)
	t.Status = statusCompleted
	t.CompletedAt = &now
	if note != "" {
		t.CompletionNote = note
	}
	// Auto-acknowledge if user is currently in this pane
	t.Acknowledged = isActivePane(target.PaneID)
	return !wasCompleted, nil
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

func normalizeTargetNames(target tmuxTarget) tmuxTarget {
	if strings.TrimSpace(target.SessionName) == strings.TrimSpace(target.SessionID) {
		target.SessionName = ""
	}
	if strings.TrimSpace(target.WindowName) == strings.TrimSpace(target.WindowID) {
		target.WindowName = ""
	}
	return target
}

func mergeTaskNamesFromTarget(task *taskRecord, target tmuxTarget) {
	if task == nil {
		return
	}
	if sessionName := strings.TrimSpace(target.SessionName); sessionName != "" {
		task.SessionName = sessionName
	}
	if windowName := strings.TrimSpace(target.WindowName); windowName != "" {
		task.WindowName = windowName
	}
}

func (s *server) loadSettings() error {
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored storedSettings
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.NotificationsEnabled != nil {
		s.mu.Lock()
		s.notificationsEnabled = *stored.NotificationsEnabled
		s.mu.Unlock()
	}
	return nil
}

func (s *server) saveSettingsLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.settingsPath), 0o755); err != nil {
		return err
	}
	enabled := s.notificationsEnabled
	data, err := json.MarshalIndent(storedSettings{NotificationsEnabled: &enabled}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.settingsPath)
}

func (s *server) notificationsAreEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notificationsEnabled
}

func (s *server) toggleNotifications() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notificationsEnabled = !s.notificationsEnabled
	if err := s.saveSettingsLocked(); err != nil {
		return false, err
	}
	return s.notificationsEnabled, nil
}

func (s *server) notifyResponded(target tmuxTarget) {
	target = s.fillTargetNamesFromTask(target)
	summary := strings.TrimSpace(s.summaryForTask(target.SessionID, target.WindowID, target.PaneID))
	if summary == "" {
		summary = "Task marked complete"
	}
	title := notificationTitleForTarget(target)
	action := notificationActionForTarget(target)
	if err := sendSystemNotification(title, summary, action); err != nil {
		log.Printf("notification error: %v", err)
	}
}

func (s *server) fillTargetNamesFromTask(target tmuxTarget) tmuxTarget {
	target = normalizeTargetNames(target)
	s.mu.Lock()
	defer s.mu.Unlock()
	if task, ok := s.tasks[taskKey(target.SessionID, target.WindowID, target.PaneID)]; ok {
		if strings.TrimSpace(target.SessionName) == "" {
			target.SessionName = strings.TrimSpace(task.SessionName)
		}
		if strings.TrimSpace(target.WindowName) == "" {
			target.WindowName = strings.TrimSpace(task.WindowName)
		}
	}
	return target
}

func notificationTitleForTarget(target tmuxTarget) string {
	target = normalizeTargetNames(target)
	session := strings.TrimSpace(target.SessionName)
	if session != "" {
		session = stripSessionIndexPrefix(session)
	}
	if session == "" {
		session = strings.TrimSpace(target.SessionID)
	}
	window := strings.TrimSpace(target.WindowName)
	if window == "" {
		window = strings.TrimSpace(target.WindowID)
	}

	if session != "" && window != "" {
		return session + " - " + window
	}
	if session != "" {
		return session
	}
	if window != "" {
		return window
	}
	return "Tracker"
}

func stripSessionIndexPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return name
	}

	j := i
	for j < len(name) && name[j] == ' ' {
		j++
	}
	if j >= len(name) || name[j] != '-' {
		return name
	}

	j++
	for j < len(name) && name[j] == ' ' {
		j++
	}

	trimmed := strings.TrimSpace(name[j:])
	if trimmed == "" {
		return name
	}
	return trimmed
}

func (s *server) summaryForTask(sessionID, windowID, paneID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[taskKey(sessionID, windowID, paneID)]; ok {
		note := strings.TrimSpace(t.CompletionNote)
		summary := strings.TrimSpace(t.Summary)
		if note != "" && !isGenericCompletionNote(note) {
			return note
		}
		if summary != "" {
			return summary
		}
		if note != "" {
			return note
		}
	}
	return ""
}

func isGenericCompletionNote(note string) bool {
	normalized := strings.ToLower(strings.TrimSpace(note))
	normalized = strings.Trim(normalized, ".!?,;:-_()[]{}\"'` ")
	if normalized == "" {
		return true
	}
	switch normalized {
	case "done", "complete", "completed", "finished", "fixed", "resolved", "ok", "okay", "success", "successful", "all set", "all good", "implemented", "updated", "shipped":
		return true
	default:
		return false
	}
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
		sessionName := strings.TrimSpace(t.SessionName)
		windowName := strings.TrimSpace(t.WindowName)
		if sessionName == strings.TrimSpace(t.SessionID) {
			sessionName = ""
		}
		if windowName == strings.TrimSpace(t.WindowID) {
			windowName = ""
		}
		if sessionName == "" || windowName == "" {
			if cached, ok := nameCache[t.WindowID]; ok {
				if sessionName == "" {
					sessionName = cached[0]
				}
				if windowName == "" {
					windowName = cached[1]
				}
			} else {
				sessName, winName, err := tmuxNamesForWindow(t.WindowID)
				if err == nil {
					nameCache[t.WindowID] = [2]string{sessName, winName}
					if sessionName == "" {
						sessionName = sessName
					}
					if windowName == "" {
						windowName = winName
					}
				}
			}
		}
		if sessionName == "" {
			sessionName = t.SessionID
		}
		if windowName == "" {
			windowName = t.WindowID
		}

		tasks = append(tasks, ipc.Task{
			SessionID:       t.SessionID,
			Session:         sessionName,
			WindowID:        t.WindowID,
			Window:          windowName,
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
	return &ipc.Envelope{
		Kind:    "state",
		Message: msg,
		Tasks:   tasks,
	}
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
		shellQuote(session), shellQuote(window), shellQuote(pane))
	return &notificationAction{
		Command:     "sh -lc " + strconv.Quote(cmd),
		ActivateApp: "com.googlecode.iterm2",
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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

func isActivePane(paneID string) bool {
	clients, err := listClients()
	if err != nil {
		return false
	}
	for _, client := range clients {
		output, err := tmuxDisplay(client, "#{pane_id}")
		if err != nil {
			continue
		}
		if strings.TrimSpace(output) == paneID {
			return true
		}
	}
	return false
}

func tmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
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

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}

func settingsStorePath() string {
	base := filepath.Join(os.Getenv("HOME"), ".config", "agent-tracker", "run")
	return filepath.Join(base, "settings.json")
}

func taskKey(sessionID, windowID, paneID string) string {
	return strings.Join([]string{sessionID, windowID, paneID}, "|")
}

func requireSessionWindow(env ipc.Envelope) (tmuxTarget, error) {
	ctx := normalizeTargetNames(tmuxTarget{
		SessionName: strings.TrimSpace(env.Session),
		SessionID:   strings.TrimSpace(env.SessionID),
		WindowName:  strings.TrimSpace(env.Window),
		WindowID:    strings.TrimSpace(env.WindowID),
		PaneID:      strings.TrimSpace(env.Pane),
	})

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
			ctx = ctx.merge(normalizeTargetNames(info))
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
	if t.WindowIndex == "" {
		t.WindowIndex = other.WindowIndex
	}
	if t.PaneIndex == "" {
		t.PaneIndex = other.PaneIndex
	}
	return t
}

func detectTmuxTarget(target string) (tmuxTarget, error) {
	format := "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}:::#{window_index}:::#{pane_index}"
	output, err := tmuxQuery(strings.TrimSpace(target), format)
	if err != nil {
		return tmuxTarget{}, err
	}
	parts := strings.Split(strings.TrimSpace(output), ":::")
	if len(parts) != 7 {
		return tmuxTarget{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(output))
	}
	return tmuxTarget{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
		WindowIndex: strings.TrimSpace(parts[5]),
		PaneIndex:   strings.TrimSpace(parts[6]),
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
