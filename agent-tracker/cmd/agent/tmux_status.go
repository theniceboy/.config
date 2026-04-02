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
)

const (
	statusRightModuleCPU          = "cpu"
	statusRightModuleNetwork      = "network"
	statusRightModuleMemory       = "memory"
	statusRightModuleMemoryTotals = "memory_totals"
	statusRightModuleAgent        = "agent"
	statusRightModuleNotes        = "notes"
	statusRightModuleFlashMoe     = "flash_moe"
	statusRightModuleHost         = "host"
)

const (
	statusIconCPU      = ""
	statusIconNetwork  = "󰖩"
	statusIconMemory   = ""
	statusIconWindow   = "󰖲"
	statusIconSession  = ""
	statusIconTotal    = "󰍛"
	statusIconAgent    = "󰚩"
	statusIconNotes    = "󰎚"
	statusIconFlashMoe = "󱙺"
)

func statusRightModules() []string {
	return []string{
		statusRightModuleCPU,
		statusRightModuleNetwork,
		statusRightModuleMemory,
		statusRightModuleMemoryTotals,
		statusRightModuleAgent,
		statusRightModuleNotes,
		statusRightModuleFlashMoe,
		statusRightModuleHost,
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
	PaneID      string
	WindowID    string
}

type statusSegment struct {
	FG   string
	BG   string
	Text string
	Bold bool
}

type statusMemoryCache struct {
	Pane    map[string]string `json:"pane"`
	Window  map[string]string `json:"window"`
	Session map[string]string `json:"session"`
	Total   string            `json:"total"`
}

type statusTodoCache struct {
	Windows map[string][]statusTodoItem `json:"windows"`
}

type statusTodoItem struct {
	Done bool `json:"done"`
}

type statusNetworkCounter struct {
	InBytes  uint64
	OutBytes uint64
}

type statusNetworkRateCache struct {
	Interface string `json:"interface"`
	InBytes   uint64 `json:"in_bytes"`
	OutBytes  uint64 `json:"out_bytes"`
	SampledAt int64  `json:"sampled_at_unix_ms"`
}

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
	if statusRightModuleEnabled(statusRightModuleMemory) {
		if label := loadMemoryStatusLabel(args.PaneID); label != "" {
			segments = append(segments, statusSegment{FG: "#eceff4", BG: "#5e81ac", Text: label})
		}
	}
	if statusRightModuleEnabled(statusRightModuleMemoryTotals) {
		segments = append(segments, loadMemoryTotalsStatusSegments(args)...)
	}
	if statusRightModuleEnabled(statusRightModuleAgent) {
		if label := loadAgentStatusLabel(args.WindowID); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: "#81a1c1", Text: label, Bold: true})
		}
	}
	if statusRightModuleEnabled(statusRightModuleNotes) {
		if label := loadNotesStatusLabel(args.WindowID); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: "#cc6666", Text: label, Bold: true})
		}
	}
	if statusRightModuleEnabled(statusRightModuleFlashMoe) {
		if segment, ok := loadFlashMoeStatusSegment(); ok {
			segments = append(segments, segment)
		}
	}
	if statusRightModuleEnabled(statusRightModuleHost) {
		if label := loadHostStatusLabel(); label != "" {
			segments = append(segments, statusSegment{FG: "#1d1f21", BG: statusThemeColor(), Text: label})
		}
	}
	return formatRightStatusSegments(args.StatusBG, segments)
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
	rate := " ↓-- ↑-- "
	if previous.Interface == iface && previous.SampledAt > 0 && now > previous.SampledAt && current.InBytes >= previous.InBytes && current.OutBytes >= previous.OutBytes {
		seconds := float64(now-previous.SampledAt) / 1000
		if seconds >= 0.25 {
			down := float64(current.InBytes-previous.InBytes) / seconds
			up := float64(current.OutBytes-previous.OutBytes) / seconds
			rate = fmt.Sprintf(" %s ↓%s ↑%s ", statusIconNetwork, formatByteRate(down), formatByteRate(up))
		}
	}
	_ = saveNetworkRateCache(statusNetworkRateCache{
		Interface: iface,
		InBytes:   current.InBytes,
		OutBytes:  current.OutBytes,
		SampledAt: now,
	})
	if rate == " ↓-- ↑-- " {
		return fmt.Sprintf(" %s ↓-- ↑-- ", statusIconNetwork)
	}
	return rate
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
	return fmt.Sprintf(" %s %s ", statusIconMemory, value)
}

func loadMemoryTotalsStatusSegments(args tmuxRightStatusArgs) []statusSegment {
	cache, ok := loadMemoryStatusCache()
	if !ok {
		return nil
	}
	segments := make([]statusSegment, 0, 3)
	windowKey := strings.TrimSpace(args.SessionName)
	if windowKey != "" && strings.TrimSpace(args.WindowIndex) != "" {
		windowKey = windowKey + ":" + strings.TrimSpace(args.WindowIndex)
	}
	if value := strings.TrimSpace(cache.Window[windowKey]); value != "" {
		segments = append(segments, statusSegment{FG: "#eceff4", BG: "#4c566a", Text: fmt.Sprintf(" %s %s ", statusIconWindow, value)})
	}
	if value := strings.TrimSpace(cache.Session[strings.TrimSpace(args.SessionName)]); value != "" {
		segments = append(segments, statusSegment{FG: "#eceff4", BG: "#434c5e", Text: fmt.Sprintf(" %s %s ", statusIconSession, value)})
	}
	if value := strings.TrimSpace(cache.Total); value != "" {
		segments = append(segments, statusSegment{FG: "#eceff4", BG: "#3b4252", Text: fmt.Sprintf(" %s %s ", statusIconTotal, value)})
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

func refreshMemoryUsageCache() {
	script := statusMemoryCacheRefreshScript()
	if strings.TrimSpace(script) == "" || !fileExists(script) {
		return
	}
	_ = statusCommandStart("python3", script)
}

func loadNotesStatusLabel(windowID string) string {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return ""
	}
	data, err := os.ReadFile(statusTodoFilePath())
	if err != nil {
		return ""
	}
	var cache statusTodoCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return ""
	}
	items := cache.Windows[windowID]
	count := 0
	for _, item := range items {
		if !item.Done {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	return fmt.Sprintf(" %s %d ", statusIconNotes, count)
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
	case statusRightModuleCPU, statusRightModuleNetwork, statusRightModuleMemory, statusRightModuleAgent, statusRightModuleNotes, statusRightModuleFlashMoe, statusRightModuleHost:
		return true
	case statusRightModuleMemoryTotals:
		return true
	default:
		return false
	}
}

func isValidStatusRightModule(module string) bool {
	switch module {
	case statusRightModuleCPU, statusRightModuleNetwork, statusRightModuleMemory, statusRightModuleMemoryTotals, statusRightModuleAgent, statusRightModuleNotes, statusRightModuleFlashMoe, statusRightModuleHost:
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
		return derefBool(cfg.CPU, defaultStatusRightModuleEnabled(module))
	case statusRightModuleNetwork:
		return derefBool(cfg.Network, defaultStatusRightModuleEnabled(module))
	case statusRightModuleMemory:
		return derefBool(cfg.Memory, defaultStatusRightModuleEnabled(module))
	case statusRightModuleMemoryTotals:
		return derefBool(cfg.MemoryTotals, defaultStatusRightModuleEnabled(module))
	case statusRightModuleAgent:
		return derefBool(cfg.Agent, defaultStatusRightModuleEnabled(module))
	case statusRightModuleNotes:
		return derefBool(cfg.Notes, defaultStatusRightModuleEnabled(module))
	case statusRightModuleFlashMoe:
		return derefBool(cfg.FlashMoe, defaultStatusRightModuleEnabled(module))
	case statusRightModuleHost:
		return derefBool(cfg.Host, defaultStatusRightModuleEnabled(module))
	default:
		return false
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
	if enabled == defaultStatusRightModuleEnabled(module) {
		value = nil
	}
	switch module {
	case statusRightModuleCPU:
		cfg.CPU = value
	case statusRightModuleNetwork:
		cfg.Network = value
	case statusRightModuleMemory:
		cfg.Memory = value
	case statusRightModuleMemoryTotals:
		cfg.MemoryTotals = value
	case statusRightModuleAgent:
		cfg.Agent = value
	case statusRightModuleNotes:
		cfg.Notes = value
	case statusRightModuleFlashMoe:
		cfg.FlashMoe = value
	case statusRightModuleHost:
		cfg.Host = value
	}
}

func (cfg *statusRightConfig) isDefault() bool {
	if cfg == nil {
		return true
	}
	return cfg.CPU == nil && cfg.Network == nil && cfg.Memory == nil && cfg.MemoryTotals == nil && cfg.Agent == nil && cfg.Notes == nil && cfg.FlashMoe == nil && cfg.Host == nil
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
