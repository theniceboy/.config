package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	statusRightModuleCPU           = "cpu"
	statusRightModuleNetwork       = "network"
	statusRightModuleMemory        = "memory"
	statusRightModuleWindowMemory  = "window_memory"
	statusRightModuleSessionMemory = "session_memory"
	statusRightModuleTotalMemory   = "total_memory"
	statusRightModuleScratch       = "scratch"
	statusRightModuleFlashMoe      = "flash_moe"
	statusRightModuleHost          = "host"
	statusRightModuleJobs          = "jobs"
)

const (
	statusIconCPU      = ""
	statusIconNetwork  = "󰖩"
	statusIconMemory   = ""
	statusIconWindow   = "󰖲"
	statusIconSession  = ""
	statusIconTotal    = "󰍛"
	statusIconAgent    = "󰚩"
	statusIconScratch  = "✏️"
	statusIconTodos    = "󰎚"
	statusIconFlashMoe = "󱙺"
	statusIconGoal     = "⌖"
	statusIconNext     = "󰁔"
	statusIconLastMsg  = "󰅻"
)

func statusRightModules() []string {
	return []string{
		statusRightModuleCPU,
		statusRightModuleNetwork,
		statusRightModuleMemory,
		statusRightModuleWindowMemory,
		statusRightModuleSessionMemory,
		statusRightModuleTotalMemory,
		statusRightModuleScratch,
		statusRightModuleFlashMoe,
		statusRightModuleHost,
		statusRightModuleJobs,
	}
}

var cpuUsagePattern = regexp.MustCompile(`CPU usage:\s*([0-9.]+)% user,\s*([0-9.]+)% sys,`)

var statusCommandOutput = func(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.Output()
}

var statusCommandStart = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

var statusNow = time.Now
var statusHostname = os.Hostname
var statusDetectCurrentAgentFromTmux = detectCurrentAgentFromTmux
var statusLoadRegistry = loadRegistry

// memoryCacheFreshFor mirrors STALE_SECONDS in mem_usage_cache.py; skipping
// the python spawn while fresh avoids a cold start on every status refresh.
const memoryCacheFreshFor = 15 * time.Second

var statusMemoryCachePath = func() string { return "/tmp/tmux-mem-usage.json" }
var statusMemoryCacheRefreshScript = func() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "tmux", "tmux-status", "mem_usage_cache.py")
}
var statusTodoFilePath = func() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "agent", "todos.json")
}
var statusFlashMoeMetricsPath = func() string {
	return filepath.Join(os.Getenv("HOME"), ".flash-moe", "tmux_metrics")
}
var statusNetworkRateCachePath = func() string {
	return "/tmp/agent-tmux-network-rate.json"
}

type tmuxRightStatusArgs struct {
	Width       int
	StatusBG    string
	SessionName string
	WindowIndex string
	WindowName  string
	PaneID      string
	WindowID    string
}

type statusSegment struct {
	FG             string
	BG             string
	Text           string
	Bold           bool
	NoRightPadding bool
}

type statusMemoryCache struct {
	Pane    map[string]string `json:"pane"`
	Window  map[string]string `json:"window"`
	Session map[string]string `json:"session"`
	Total   string            `json:"total"`
}

type statusTodoCache struct {
	Global   []statusTodoItem            `json:"global"`
	Sessions map[string][]statusTodoItem `json:"sessions"`
	Windows  map[string][]statusTodoItem `json:"windows"`
}

type windowSnapshotEntry struct {
	SessionName string `json:"session_name"`
	WindowIndex string `json:"window_index"`
	WindowName  string `json:"window_name"`
}

func windowSnapshotPath() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "agent-tracker", "window-snapshot.json")
}

func saveWindowSnapshot(args tmuxRightStatusArgs) {
	if args.WindowID == "" {
		return
	}
	path := windowSnapshotPath()
	data, err := os.ReadFile(path)
	var snapshot map[string]windowSnapshotEntry
	if err == nil {
		_ = json.Unmarshal(data, &snapshot)
	}
	if snapshot == nil {
		snapshot = make(map[string]windowSnapshotEntry)
	}
	out, err := runTmuxOutput("list-windows", "-a", "-F", "#{window_id}\t#{session_name}\t#{window_index}\t#{window_name}")
	if err != nil {
		entry := windowSnapshotEntry{
			SessionName: args.SessionName,
			WindowIndex: args.WindowIndex,
			WindowName:  args.WindowName,
		}
		if existing, ok := snapshot[args.WindowID]; ok && existing == entry {
			return
		}
		snapshot[args.WindowID] = entry
	} else {
		liveIDs := make(map[string]bool)
		for _, line := range strings.Split(out, "\n") {
			fields := strings.SplitN(line, "\t", 4)
			if len(fields) != 4 {
				continue
			}
			wid := strings.TrimSpace(fields[0])
			if wid == "" {
				continue
			}
			liveIDs[wid] = true
			snapshot[wid] = windowSnapshotEntry{
				SessionName: strings.TrimSpace(fields[1]),
				WindowIndex: strings.TrimSpace(fields[2]),
				WindowName:  strings.TrimSpace(fields[3]),
			}
		}
		pruneOrphanedLastMessageFiles(liveIDs)
	}
	updated, _ := json.Marshal(snapshot)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	tmp := path + ".tmp"
	_ = os.WriteFile(tmp, updated, 0644)
	_ = os.Rename(tmp, path)
}

// work-status is invoked once per tmux status refresh, so prune cadence is bounded
// via a stamp file rather than running on every call.
const lastMessagePruneInterval = 2 * time.Minute

func pruneOrphanedLastMessageFiles(liveWindowIDs map[string]bool) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(stateDir, "op")
	stamp := filepath.Join(dir, ".lastmsg-prune-stamp")
	if info, err := os.Stat(stamp); err == nil && statusNow().Sub(info.ModTime()) < lastMessagePruneInterval {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if err := os.WriteFile(stamp, []byte{}, 0o644); err != nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	live := make(map[string]bool, len(liveWindowIDs))
	for id := range liveWindowIDs {
		live[sanitizeStateKey(id)] = true
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "lastmsg_") {
			continue
		}
		if live[strings.TrimPrefix(name, "lastmsg_")] {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}

type statusTodoItem struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type statusNetworkCounter struct {
	InBytes  uint64
	OutBytes uint64
}

type statusNetworkRateCache struct {
	Interface  string  `json:"interface"`
	InBytes    uint64  `json:"in_bytes"`
	OutBytes   uint64  `json:"out_bytes"`
	SampledAt  int64   `json:"sampled_at_unix_ms"`
	LastDown   float64 `json:"last_down_bps"`
	LastUp     float64 `json:"last_up_bps"`
	LastRateAt int64   `json:"last_rate_at_unix_ms"`
}

// Forced status redraws (refresh-client -S hooks) can invoke right-status many
// times per second, so the baseline must not be advanced until it spans a
// usable window; otherwise rates collapse to "--".
const (
	networkRateMinWindowMS = 250
	networkRateKeepMS      = 500
	networkRateReuseMS     = 5000
)

func runTmuxRightStatus(args []string) error {
	fs := flag.NewFlagSet("agent tmux right-status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	values := fs.Args()
	parsed := tmuxRightStatusArgs{}
	if len(values) > 0 {
		parsed.Width, _ = strconv.Atoi(strings.TrimSpace(values[0]))
	}
	if len(values) > 1 {
		parsed.StatusBG = strings.TrimSpace(values[1])
	}
	if len(values) > 2 {
		parsed.SessionName = strings.TrimSpace(values[2])
	}
	if len(values) > 3 {
		parsed.WindowIndex = strings.TrimSpace(values[3])
	}
	if len(values) > 4 {
		parsed.PaneID = strings.TrimSpace(values[4])
	}
	if len(values) > 5 {
		parsed.WindowID = strings.TrimSpace(values[5])
	}
	if parsed.StatusBG == "" || parsed.StatusBG == "default" {
		parsed.StatusBG = "black"
	}
	if parsed.Width > 0 && parsed.Width < statusRightMinimumWidth() {
		return nil
	}
	fmt.Print(renderTmuxRightStatus(parsed))
	return nil
}

func runTmuxWorkStatus(args []string) error {
	fs := flag.NewFlagSet("agent tmux work-status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	values := fs.Args()
	parsed := tmuxRightStatusArgs{}
	if len(values) > 0 {
		parsed.Width, _ = strconv.Atoi(strings.TrimSpace(values[0]))
	}
	if len(values) > 1 {
		parsed.WindowID = strings.TrimSpace(values[1])
	}
	if len(values) > 2 {
		parsed.SessionName = strings.TrimSpace(values[2])
	}
	if len(values) > 3 {
		parsed.WindowIndex = strings.TrimSpace(values[3])
	}
	if len(values) > 4 {
		parsed.WindowName = strings.TrimSpace(values[4])
	}
	saveWindowSnapshot(parsed)
	fmt.Print(renderTmuxWorkStatus(parsed))
	return nil
}

func renderTmuxRightStatus(args tmuxRightStatusArgs) string {
	segments := make([]statusSegment, 0, 6)
	if statusRightModuleEnabled(statusRightModuleCPU) {
		if label := loadCPUStatusLabel(); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: "#d08770", Text: label, Bold: true})
		}
	}
	if statusRightModuleEnabled(statusRightModuleNetwork) {
		if label := loadNetworkStatusLabel(); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: "#8fbcbb", Text: label, Bold: true})
		}
	}
	memoryStart := len(segments)
	if statusRightModuleEnabled(statusRightModuleMemory) {
		if label := loadMemoryStatusLabel(args.PaneID); label != "" {
			segments = append(segments, statusSegment{FG: "#eceff4", BG: "#5e81ac", Text: label, NoRightPadding: true})
		}
	}
	segments = append(segments, loadSplitMemoryStatusSegments(args)...)
	if len(segments) > memoryStart {
		segments[len(segments)-1].Text += " "
	}
	if statusRightModuleEnabled(statusRightModuleFlashMoe) {
		if segment, ok := loadFlashMoeStatusSegment(); ok {
			segments = append(segments, segment)
		}
	}
	if segment, ok := loadJobsStatusSegment(); ok {
		segments = append(segments, segment)
	}
	if statusRightModuleEnabled(statusRightModuleHost) {
		if label := loadHostStatusLabel(); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: statusThemeColor(), Text: label})
		}
	}
	if statusRightModuleEnabled(statusRightModuleScratch) {
		if label := loadScratchStatusLabel(args.SessionName); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: "#d75f5f", Text: label, Bold: true, NoRightPadding: true})
		}
	}
	return formatRightStatusSegments(args.StatusBG, segments)
}

func renderTmuxWorkStatus(args tmuxRightStatusArgs) string {
	baseBG := "#232530"
	leftBG := "#272535"
	available := args.Width
	if available <= 0 {
		available = 120
	}
	leftParts := make([]string, 0, 3)
	if label := loadGoalWorkStatusLabel(args.WindowID); label != "" {
		leftParts = append(leftParts, fmt.Sprintf("#[fg=#f8f8f2,bg=#343746] %s %s #[fg=#343746,bg=%s]", statusIconGoal, label, leftBG))
	}
	if label := loadTodoCountWorkStatusLabel(args.WindowID); label != "" {
		leftParts = append(leftParts, fmt.Sprintf("#[fg=#ff79c6,bg=%s] %s %s ", leftBG, statusIconTodos, label))
	}
	if label := loadTodoPreviewWorkStatusLabel(args.WindowID); label != "" {
		leftParts = append(leftParts, fmt.Sprintf("#[fg=#ff79c6,bg=%s]%s #[fg=#ff79c6,bg=%s]%s", leftBG, statusIconNext, leftBG, label))
	}

	hasLeft := len(leftParts) > 0
	msgLabel := loadLastUserMessageLabel(args.WindowID)

	var agentPart string
	if label := loadAgentWorkStatusLabel(args.WindowID); label != "" {
		agentPart = fmt.Sprintf("#[fg=#8be9fd,bg=#233a45] %s %s ", statusIconAgent, label)
	}

	if len(leftParts) == 0 && msgLabel == "" && agentPart == "" {
		return ""
	}

	left := ""
	if hasLeft {
		left = strings.Join(leftParts, fmt.Sprintf("#[fg=#c5c8c6,bg=%s] ", leftBG))
	}
	leftPlain := stripTmuxStyles(left)
	agentPlain := stripTmuxStyles(agentPart)
	leftMaxWidth := maxInt(1, available*60/100)
	if len([]rune(leftPlain)) > leftMaxWidth {
		left = truncateStyledWorkStatus(left, leftMaxWidth)
		leftPlain = stripTmuxStyles(left)
	}

	var msgPart string
	if msgLabel != "" {
		msgSpace := available - len([]rune(leftPlain)) - len([]rune(agentPlain)) - 1
		msgPrefixWidth := len([]rune(statusIconLastMsg)) + 1
		if hasLeft {
			msgPrefixWidth += len([]rune("  │  "))
		}
		msgMaxWidth := msgSpace - msgPrefixWidth
		if msgMaxWidth > 0 {
			msg := truncate(msgLabel, msgMaxWidth)
			if hasLeft {
				msgPart = fmt.Sprintf("#[fg=#3d4050,bg=%s]  │  #[fg=#6c7086,bg=%s]%s #[fg=#c5c8c6,bg=%s]%s", baseBG, baseBG, statusIconLastMsg, baseBG, msg)
			} else {
				msgPart = fmt.Sprintf("#[fg=#6c7086,bg=%s]%s #[fg=#c5c8c6,bg=%s]%s", baseBG, statusIconLastMsg, baseBG, msg)
			}
		}
	}
	msgPlain := stripTmuxStyles(msgPart)

	middle := left + msgPart
	middlePlain := leftPlain + msgPlain
	gapWidth := available - len([]rune(middlePlain)) - len([]rune(agentPlain))
	if gapWidth < 1 {
		gapWidth = 1
	}
	return fmt.Sprintf("#[fg=#c5c8c6,bg=%s,fill=%s]%s%s%s#[default]", baseBG, baseBG, middle, strings.Repeat(" ", gapWidth), agentPart)
}

var tmuxStylePattern = regexp.MustCompile(`#\[[^]]*\]`)

func stripTmuxStyles(text string) string {
	return tmuxStylePattern.ReplaceAllString(text, "")
}

func truncateStyledWorkStatus(text string, width int) string {
	plain := stripTmuxStyles(text)
	if len([]rune(plain)) <= width {
		return text
	}
	if width <= 0 {
		return ""
	}
	limit := maxInt(0, width-1)
	visible := 0
	var b strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "#[") {
			end := strings.IndexByte(text[i:], ']')
			if end >= 0 {
				end += i
				b.WriteString(text[i : end+1])
				i = end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if visible >= limit {
			break
		}
		b.WriteRune(r)
		visible++
		i += size
	}
	b.WriteRune('…')
	return b.String()
}

func loadLastUserMessageLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	sanitized := sanitizeStateKey(windowID)
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state")
	}
	path := filepath.Join(stateDir, "op", "lastmsg_"+sanitized)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	text = strings.Join(strings.Fields(text), " ")
	return text
}

func sanitizeStateKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func loadAgentWorkStatusLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	ref, err := statusDetectCurrentAgentFromTmux(windowID)
	if err != nil || strings.TrimSpace(ref.ID) == "" {
		return ""
	}
	reg, err := statusLoadRegistry()
	if err != nil || reg == nil {
		return ""
	}
	record := reg.Agents[strings.TrimSpace(ref.ID)]
	if record == nil {
		return ""
	}
	device := strings.TrimSpace(record.Device)
	if device == "" {
		device = "no device"
	}
	return device
}

func loadGoalWorkStatusLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	store, err := loadGoalStore()
	if err != nil || store == nil {
		return ""
	}
	thread := findThreadByWindow(store, windowID)
	if thread == nil {
		return ""
	}
	path := goalPathTitles(store, thread.GoalID)
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, " › ")
}

func loadTodoCountWorkStatusLabel(windowID string) string {
	items, ok := statusTodoItemsForWindow(windowID)
	if !ok {
		return ""
	}
	count := 0
	for _, item := range items {
		if !item.Done {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%d", count)
}

func loadTodoPreviewWorkStatusLabel(windowID string) string {
	items, ok := statusTodoItemsForWindow(windowID)
	if !ok {
		return ""
	}
	title := firstOpenStatusTodoTitle(items)
	if title == "" {
		return ""
	}
	return title
}

func loadScratchStatusLabel(currentSessionName string) string {
	if scratchSessionName(currentSessionName) {
		return ""
	}
	if scratchTrackerWaiting() {
		return fmt.Sprintf(" %s 🔔", statusIconScratch)
	}
	out, err := statusCommandOutput("tmux", "list-windows", "-t", "Scratch", "-F", "#{window_bell_flag} #{window_activity_flag} #{@unread} #{@watch_failed}")
	if err != nil {
		out, err = statusCommandOutput("tmux", "list-windows", "-t", "scratch", "-F", "#{window_bell_flag} #{window_activity_flag} #{@unread} #{@watch_failed}")
		if err != nil {
			return ""
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, field := range strings.Fields(line) {
			if field == "1" {
				return fmt.Sprintf(" %s 🔔", statusIconScratch)
			}
		}
	}
	return ""
}

func scratchTrackerWaiting() bool {
	data, err := os.ReadFile("/tmp/tmux-tracker-cache.json")
	if err != nil {
		return false
	}
	var state struct {
		Tasks []struct {
			SessionID    string `json:"session_id"`
			Session      string `json:"session"`
			Status       string `json:"status"`
			Acknowledged bool   `json:"acknowledged"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return false
	}
	for _, task := range state.Tasks {
		if task.Status == "completed" && !task.Acknowledged && scratchTrackerTaskSession(task.Session, task.SessionID) {
			return true
		}
	}
	return false
}

func scratchTrackerTaskSession(sessionName, sessionID string) bool {
	if scratchSessionName(sessionName) {
		return true
	}
	for _, target := range []string{"Scratch", "scratch"} {
		out, err := statusCommandOutput("tmux", "display-message", "-p", "-t", target, "#{session_id}")
		if err == nil && strings.TrimSpace(string(out)) == strings.TrimSpace(sessionID) {
			return true
		}
	}
	return false
}

func scratchSessionName(name string) bool {
	name = strings.TrimSpace(name)
	if match := regexp.MustCompile(`^\d+-(.*)$`).FindStringSubmatch(name); len(match) == 2 {
		name = match[1]
	}
	return strings.EqualFold(name, "scratch")
}

func formatRightStatusSegments(statusBG string, segments []statusSegment) string {
	if len(segments) == 0 {
		return ""
	}
	separator := ""
	rightCap := "█"
	prevBG := statusBG
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(fmt.Sprintf("#[fg=%s,bg=%s]%s#[fg=%s,bg=%s", segment.BG, prevBG, separator, segment.FG, segment.BG))
		if segment.Bold {
			builder.WriteString(",bold")
		}
		builder.WriteString("]")
		builder.WriteString(segment.Text)
		prevBG = segment.BG
	}
	last := segments[len(segments)-1]
	if last.NoRightPadding {
		builder.WriteString(fmt.Sprintf("#[bg=%s]", statusBG))
		return builder.String()
	}
	builder.WriteString(fmt.Sprintf(" #[fg=%s,bg=%s]%s", prevBG, statusBG, rightCap))
	return builder.String()
}

func statusRightMinimumWidth() int {
	value := strings.TrimSpace(os.Getenv("TMUX_RIGHT_MIN_WIDTH"))
	if value == "" {
		return 90
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 90
	}
	return parsed
}

func statusThemeColor() string {
	value := strings.TrimSpace(os.Getenv("TMUX_THEME_COLOR"))
	if value == "" {
		return "#b294bb"
	}
	return value
}

func loadCPUStatusLabel() string {
	output, err := statusCommandOutput("top", "-l", "1", "-n", "0")
	if err != nil {
		return ""
	}
	total, ok := parseCPUUsageTotal(string(output))
	if !ok {
		return ""
	}
	return fmt.Sprintf(" %s %s ", statusIconCPU, formatUsagePercent(total))
}

func parseCPUUsageTotal(output string) (float64, bool) {
	matches := cpuUsagePattern.FindStringSubmatch(output)
	if len(matches) != 3 {
		return 0, false
	}
	user, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, false
	}
	system, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return 0, false
	}
	total := math.Max(0, user+system)
	if total > 100 {
		total = 100
	}
	return total, true
}

func formatUsagePercent(value float64) string {
	if value < 10 && math.Abs(value-math.Round(value)) > 0.05 {
		return fmt.Sprintf("%.1f%%", value)
	}
	return fmt.Sprintf("%.0f%%", value)
}

func loadNetworkStatusLabel() string {
	preferred := loadPrimaryNetworkInterface()
	output, err := statusCommandOutput("netstat", "-ibn")
	if err != nil {
		return ""
	}
	counters := parseNetworkCounters(string(output))
	iface, current, ok := pickNetworkCounter(counters, preferred)
	if !ok {
		return ""
	}
	now := statusNow().UnixMilli()
	previous, _ := loadNetworkRateCache()

	updated := statusNetworkRateCache{
		Interface: iface,
		InBytes:   current.InBytes,
		OutBytes:  current.OutBytes,
		SampledAt: now,
	}
	down, up := -1.0, -1.0
	validBaseline := previous.Interface == iface && previous.SampledAt > 0 && now > previous.SampledAt &&
		current.InBytes >= previous.InBytes && current.OutBytes >= previous.OutBytes
	if validBaseline {
		updated.LastDown = previous.LastDown
		updated.LastUp = previous.LastUp
		updated.LastRateAt = previous.LastRateAt
		elapsed := now - previous.SampledAt
		if elapsed >= networkRateMinWindowMS {
			seconds := float64(elapsed) / 1000
			down = float64(current.InBytes-previous.InBytes) / seconds
			up = float64(current.OutBytes-previous.OutBytes) / seconds
			updated.LastDown = down
			updated.LastUp = up
			updated.LastRateAt = now
		} else if previous.LastRateAt > 0 && now-previous.LastRateAt <= networkRateReuseMS {
			down = previous.LastDown
			up = previous.LastUp
		}
		if elapsed < networkRateKeepMS {
			updated.Interface = previous.Interface
			updated.InBytes = previous.InBytes
			updated.OutBytes = previous.OutBytes
			updated.SampledAt = previous.SampledAt
		}
	}
	_ = saveNetworkRateCache(updated)
	if down < 0 {
		return fmt.Sprintf(" %s ↓-- ↑-- ", statusIconNetwork)
	}
	return fmt.Sprintf(" %s ↓%s ↑%s ", statusIconNetwork, formatByteRate(down), formatByteRate(up))
}

func loadPrimaryNetworkInterface() string {
	output, err := statusCommandOutput("route", "-n", "get", "default")
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "interface:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
	}
	return ""
}

func parseNetworkCounters(output string) map[string]statusNetworkCounter {
	counters := make(map[string]statusNetworkCounter)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSpace(fields[0]), "*")
		if name == "" || strings.EqualFold(name, "Name") {
			continue
		}
		if _, exists := counters[name]; exists {
			continue
		}
		inBytes, err := strconv.ParseUint(fields[6], 10, 64)
		if err != nil {
			continue
		}
		outBytes, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		counters[name] = statusNetworkCounter{InBytes: inBytes, OutBytes: outBytes}
	}
	return counters
}

func pickNetworkCounter(counters map[string]statusNetworkCounter, preferred string) (string, statusNetworkCounter, bool) {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		if counter, ok := counters[preferred]; ok {
			return preferred, counter, true
		}
	}
	var bestName string
	var bestCounter statusNetworkCounter
	var bestTotal uint64
	for name, counter := range counters {
		if ignoreNetworkInterface(name) {
			continue
		}
		total := counter.InBytes + counter.OutBytes
		if total <= bestTotal {
			continue
		}
		bestName = name
		bestCounter = counter
		bestTotal = total
	}
	if bestName == "" {
		return "", statusNetworkCounter{}, false
	}
	return bestName, bestCounter, true
}

func ignoreNetworkInterface(name string) bool {
	for _, prefix := range []string{"lo", "awdl", "llw", "gif", "stf", "anpi", "ap", "bridge", "pktap"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func formatByteRate(bytesPerSecond float64) string {
	if bytesPerSecond < 0 {
		bytesPerSecond = 0
	}
	units := []string{"B", "K", "M", "G"}
	value := bytesPerSecond
	unit := units[0]
	for _, candidate := range units[1:] {
		if value < 1024 {
			break
		}
		value /= 1024
		unit = candidate
	}
	if value < 10 && unit != "B" {
		return fmt.Sprintf("%.1f%s", value, unit)
	}
	return fmt.Sprintf("%.0f%s", value, unit)
}

func loadNetworkRateCache() (statusNetworkRateCache, error) {
	data, err := os.ReadFile(statusNetworkRateCachePath())
	if err != nil {
		return statusNetworkRateCache{}, err
	}
	var cache statusNetworkRateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return statusNetworkRateCache{}, err
	}
	return cache, nil
}

func saveNetworkRateCache(cache statusNetworkRateCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	path := statusNetworkRateCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func loadMemoryStatusLabel(paneID string) string {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return ""
	}
	cache, ok := loadMemoryStatusCache()
	if !ok {
		return ""
	}
	value := strings.TrimSpace(cache.Pane[paneID])
	if value == "" {
		return ""
	}
	return fmt.Sprintf(" %s %s", statusIconMemory, value)
}

func loadSplitMemoryStatusSegments(args tmuxRightStatusArgs) []statusSegment {
	cache, ok := loadMemoryStatusCache()
	if !ok {
		return nil
	}
	segments := make([]statusSegment, 0, 3)
	windowKey := strings.TrimSpace(args.SessionName)
	if windowKey != "" && strings.TrimSpace(args.WindowIndex) != "" {
		windowKey = windowKey + ":" + strings.TrimSpace(args.WindowIndex)
	}
	if statusRightModuleEnabled(statusRightModuleWindowMemory) {
		if value := strings.TrimSpace(cache.Window[windowKey]); value != "" {
			segments = append(segments, statusSegment{FG: "#eceff4", BG: "#4c566a", Text: fmt.Sprintf(" %s %s", statusIconWindow, value), NoRightPadding: true})
		}
	}
	if statusRightModuleEnabled(statusRightModuleSessionMemory) {
		if value := strings.TrimSpace(cache.Session[strings.TrimSpace(args.SessionName)]); value != "" {
			segments = append(segments, statusSegment{FG: "#eceff4", BG: "#434c5e", Text: fmt.Sprintf(" %s %s", statusIconSession, value), NoRightPadding: true})
		}
	}
	if statusRightModuleEnabled(statusRightModuleTotalMemory) {
		if value := strings.TrimSpace(cache.Total); value != "" {
			segments = append(segments, statusSegment{FG: "#eceff4", BG: "#3b4252", Text: fmt.Sprintf(" %s %s", statusIconTotal, value), NoRightPadding: true})
		}
	}
	return segments
}

func loadMemoryStatusCache() (statusMemoryCache, bool) {
	refreshMemoryUsageCache()
	data, err := os.ReadFile(statusMemoryCachePath())
	if err != nil {
		return statusMemoryCache{}, false
	}
	var cache statusMemoryCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return statusMemoryCache{}, false
	}
	return cache, true
}

func loadAgentStatusLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	ref, err := statusDetectCurrentAgentFromTmux(windowID)
	if err != nil || strings.TrimSpace(ref.ID) == "" {
		return ""
	}
	reg, err := statusLoadRegistry()
	if err != nil || reg == nil {
		return ""
	}
	record := reg.Agents[strings.TrimSpace(ref.ID)]
	if record == nil {
		return ""
	}
	device := strings.TrimSpace(record.Device)
	if device == "" {
		device = "no device"
	}
	return fmt.Sprintf(" %s %s ", statusIconAgent, device)
}

// loadGoalStatusLabel renders the focused window's goal path + thread name.
func loadGoalStatusLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	store, err := loadGoalStore()
	if err != nil || store == nil {
		return ""
	}
	thread := findThreadByWindow(store, windowID)
	if thread == nil {
		return ""
	}
	path := goalPathTitles(store, thread.GoalID)
	if len(path) == 0 {
		return ""
	}
	label := strings.Join(path, " › ")
	return fmt.Sprintf(" %s %s ", statusIconGoal, truncate(label, 40))
}

// goalPathTitles returns ancestor goal titles from root to the given goal.
func goalPathTitles(store *GoalStore, goalID string) []string {
	goalID = strings.TrimSpace(goalID)
	if goalID == "" || store == nil {
		return nil
	}
	var titles []string
	seen := map[string]bool{}
	for goalID != "" && !seen[goalID] {
		g, ok := store.Goals[goalID]
		if !ok {
			break
		}
		seen[goalID] = true
		titles = append([]string{strings.TrimSpace(g.Title)}, titles...)
		goalID = strings.TrimSpace(g.ParentID)
	}
	return titles
}

func refreshMemoryUsageCache() {
	script := statusMemoryCacheRefreshScript()
	if strings.TrimSpace(script) == "" || !fileExists(script) {
		return
	}
	cache := statusMemoryCachePath()
	if info, err := os.Stat(cache); err == nil && time.Since(info.ModTime()) < memoryCacheFreshFor {
		return
	}
	_ = statusCommandStart("python3", script)
}

func loadTodosStatusLabel(windowID string) string {
	items, ok := statusTodoItemsForWindow(windowID)
	if !ok {
		return ""
	}
	count := 0
	for _, item := range items {
		if !item.Done {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	return fmt.Sprintf(" %s %d ", statusIconTodos, count)
}

func loadStatusTodoCache() (statusTodoCache, bool) {
	data, err := os.ReadFile(statusTodoFilePath())
	if err != nil {
		return statusTodoCache{}, false
	}
	var cache statusTodoCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return statusTodoCache{}, false
	}
	if cache.Global == nil {
		cache.Global = []statusTodoItem{}
	}
	if cache.Sessions == nil {
		cache.Sessions = map[string][]statusTodoItem{}
	}
	if cache.Windows == nil {
		cache.Windows = map[string][]statusTodoItem{}
	}
	return cache, true
}

func statusTodoItemsForWindow(windowID string) ([]statusTodoItem, bool) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return nil, false
	}
	cache, ok := loadStatusTodoCache()
	if !ok {
		return nil, false
	}
	return cache.Windows[windowID], true
}

func firstOpenStatusTodoTitle(items []statusTodoItem) string {
	for _, item := range items {
		if item.Done {
			continue
		}
		title := strings.Join(strings.Fields(strings.TrimSpace(item.Title)), " ")
		if title != "" {
			return title
		}
	}
	return ""
}

func statusTodoMaxChars() int {
	value := strings.TrimSpace(os.Getenv("TMUX_TODO_MAX_CHARS"))
	if value == "" {
		return 32
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 8 {
		return 32
	}
	return parsed
}

func loadFlashMoeStatusSegment() (statusSegment, bool) {
	metricsPath := statusFlashMoeMetricsPath()
	if strings.TrimSpace(metricsPath) == "" {
		return statusSegment{}, false
	}
	data, err := os.ReadFile(metricsPath)
	if err != nil {
		return statusSegment{}, false
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	phase := values["phase"]
	if updated := strings.TrimSpace(values["updated_ms"]); updated != "" {
		if updatedMS, err := strconv.ParseInt(updated, 10, 64); err == nil {
			ageMS := statusNow().UnixMilli() - updatedMS
			if ageMS > 10000 && (phase == "gen" || phase == "prefill") {
				phase = "idle"
			}
		}
	}
	switch phase {
	case "prefill":
		promptTokens := strings.TrimSpace(values["prompt_tokens"])
		label := fmt.Sprintf(" %s prefill ", statusIconFlashMoe)
		if promptTokens != "" && promptTokens != "0" {
			label = fmt.Sprintf(" %s prefill:%s ", statusIconFlashMoe, promptTokens)
		}
		return statusSegment{FG: "#1d1f21", BG: "#ebcb8b", Text: label, Bold: true}, true
	case "gen":
		tokS := strings.TrimSpace(values["tok_s"])
		label := fmt.Sprintf(" %s gen ", statusIconFlashMoe)
		if tokS != "" && tokS != "0.00" {
			label = fmt.Sprintf(" %s %s tok/s ", statusIconFlashMoe, tokS)
		}
		return statusSegment{FG: "#1d1f21", BG: "#a3be8c", Text: label, Bold: true}, true
	default:
		return statusSegment{}, false
	}
}

func loadJobsStatusSegment() (statusSegment, bool) {
	if !statusRightModuleEnabled(statusRightModuleJobs) {
		return statusSegment{}, false
	}
	running := 0
	newest := backgroundJob{}
	for _, job := range listBackgroundJobs() {
		if job.running() {
			running++
			newest = job
		}
	}
	if running == 0 {
		return statusSegment{}, false
	}
	var label string
	if running == 1 {
		label = fmt.Sprintf(" ⏳ %s %s ", newest.Title, newest.elapsedLabel(statusNow()))
	} else {
		label = fmt.Sprintf(" ⏳ %d jobs ", running)
	}
	return statusSegment{FG: "#1d1f21", BG: "#ebcb8b", Text: label, Bold: true}, true
}

func loadHostStatusLabel() string {
	host, err := statusHostname()
	if err != nil {
		return ""
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if short, _, ok := strings.Cut(host, "."); ok {
		host = short
	}
	return fmt.Sprintf(" %s", host)
}

func defaultStatusRightModuleEnabled(module string) bool {
	switch module {
	case statusRightModuleCPU, statusRightModuleNetwork, statusRightModuleMemory, statusRightModuleWindowMemory, statusRightModuleSessionMemory, statusRightModuleTotalMemory, statusRightModuleScratch, statusRightModuleFlashMoe, statusRightModuleHost, statusRightModuleJobs:
		return true
	default:
		return false
	}
}

func isValidStatusRightModule(module string) bool {
	switch module {
	case statusRightModuleCPU, statusRightModuleNetwork, statusRightModuleMemory, statusRightModuleWindowMemory, statusRightModuleSessionMemory, statusRightModuleTotalMemory, statusRightModuleScratch, statusRightModuleFlashMoe, statusRightModuleHost, statusRightModuleJobs:
		return true
	default:
		return false
	}
}

func statusRightModuleEnabled(module string) bool {
	if !isValidStatusRightModule(module) {
		return false
	}
	cfg := loadAppConfig()
	if cfg.StatusRight == nil {
		return defaultStatusRightModuleEnabled(module)
	}
	return cfg.StatusRight.moduleEnabled(module)
}

func (cfg statusRightConfig) moduleEnabled(module string) bool {
	switch module {
	case statusRightModuleCPU:
		return derefBool(cfg.CPU, cfg.moduleFallbackEnabled(module))
	case statusRightModuleNetwork:
		return derefBool(cfg.Network, cfg.moduleFallbackEnabled(module))
	case statusRightModuleMemory:
		return derefBool(cfg.Memory, cfg.moduleFallbackEnabled(module))
	case statusRightModuleWindowMemory:
		return derefBool(cfg.WindowMemory, cfg.moduleFallbackEnabled(module))
	case statusRightModuleSessionMemory:
		return derefBool(cfg.SessionMemory, cfg.moduleFallbackEnabled(module))
	case statusRightModuleTotalMemory:
		return derefBool(cfg.TotalMemory, cfg.moduleFallbackEnabled(module))
	case statusRightModuleScratch:
		return derefBool(cfg.Scratch, cfg.moduleFallbackEnabled(module))
	case statusRightModuleFlashMoe:
		return derefBool(cfg.FlashMoe, cfg.moduleFallbackEnabled(module))
	case statusRightModuleHost:
		return derefBool(cfg.Host, cfg.moduleFallbackEnabled(module))
	case statusRightModuleJobs:
		return derefBool(cfg.Jobs, cfg.moduleFallbackEnabled(module))
	default:
		return false
	}
}

func (cfg statusRightConfig) moduleFallbackEnabled(module string) bool {
	switch module {
	case statusRightModuleWindowMemory, statusRightModuleSessionMemory, statusRightModuleTotalMemory:
		return derefBool(cfg.MemoryTotals, defaultStatusRightModuleEnabled(module))
	default:
		return defaultStatusRightModuleEnabled(module)
	}
}

func toggleStatusRightModule(module string) error {
	if !isValidStatusRightModule(module) {
		return fmt.Errorf("unknown status-right module: %s", module)
	}
	enabled := !statusRightModuleEnabled(module)
	return updateAppConfig(func(cfg *appConfig) {
		if cfg.StatusRight == nil {
			cfg.StatusRight = &statusRightConfig{}
		}
		cfg.StatusRight.setModuleEnabled(module, enabled)
		if cfg.StatusRight.isDefault() {
			cfg.StatusRight = nil
		}
	})
}

func (cfg *statusRightConfig) setModuleEnabled(module string, enabled bool) {
	value := boolPtr(enabled)
	if enabled == cfg.moduleFallbackEnabled(module) {
		value = nil
	}
	switch module {
	case statusRightModuleCPU:
		cfg.CPU = value
	case statusRightModuleNetwork:
		cfg.Network = value
	case statusRightModuleMemory:
		cfg.Memory = value
	case statusRightModuleWindowMemory:
		cfg.WindowMemory = value
	case statusRightModuleSessionMemory:
		cfg.SessionMemory = value
	case statusRightModuleTotalMemory:
		cfg.TotalMemory = value
	case statusRightModuleScratch:
		cfg.Scratch = value
	case statusRightModuleFlashMoe:
		cfg.FlashMoe = value
	case statusRightModuleHost:
		cfg.Host = value
	case statusRightModuleJobs:
		cfg.Jobs = value
	}
}

func (cfg *statusRightConfig) isDefault() bool {
	if cfg == nil {
		return true
	}
	return cfg.CPU == nil && cfg.Network == nil && cfg.Memory == nil && cfg.MemoryTotals == nil && cfg.WindowMemory == nil && cfg.SessionMemory == nil && cfg.TotalMemory == nil && cfg.Scratch == nil && cfg.FlashMoe == nil && cfg.Host == nil && cfg.Jobs == nil
}

func derefBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPtr(value bool) *bool {
	ptr := new(bool)
	*ptr = value
	return ptr
}
