package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const memoryStaleAfter = 5 * time.Minute

func baseMemoryStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "base", "state", "memory.json")
}

func opSessionStateDir() string {
	if stateDir := os.Getenv("XDG_STATE_HOME"); stateDir != "" {
		return filepath.Join(stateDir, "op")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "op")
}

type opPaneCtx struct {
	PaneID      string
	WindowID    string
	SessionName string
	WindowIndex string
	PaneIndex   string
}

type opSessionRegistration struct {
	SessionID string `json:"sessionID"`
	Directory string `json:"directory"`
	Pid       int    `json:"pid"`
	Heartbeat int64  `json:"heartbeat"`
	Pane      struct {
		PaneID string `json:"paneId"`
	} `json:"pane"`
	live    opPaneCtx
	modTime time.Time
}

func (r opSessionRegistration) lastSeen() time.Time {
	if r.Heartbeat > 0 {
		return time.UnixMilli(r.Heartbeat)
	}
	return r.modTime
}

func tmuxPaneCtxByPaneID() map[string]opPaneCtx {
	ctxs := map[string]opPaneCtx{}
	out, err := runTmuxOutput("list-panes", "-a", "-F",
		"#{pane_id}\t#{window_id}\t#{session_name}\t#{window_index}\t#{pane_index}")
	if err != nil {
		return ctxs
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) == 5 && parts[0] != "" {
			ctxs[parts[0]] = opPaneCtx{
				PaneID:      parts[0],
				WindowID:    parts[1],
				SessionName: parts[2],
				WindowIndex: parts[3],
				PaneIndex:   parts[4],
			}
		}
	}
	return ctxs
}

// Injectable so tests can supply pane context without a live tmux server.
var tmuxPaneCtxLoader = tmuxPaneCtxByPaneID

func loadOpSessionRegistrations() []opSessionRegistration {
	dir := opSessionStateDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	// Registration files carry only the pane id; session/window context is
	// resolved live so renames and window moves are always current.
	live := tmuxPaneCtxLoader()
	regs := []opSessionRegistration{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "ses_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var reg opSessionRegistration
		if err := json.Unmarshal(data, &reg); err != nil {
			continue
		}
		if strings.TrimSpace(reg.Directory) == "" {
			continue
		}
		reg.live = live[strings.TrimSpace(reg.Pane.PaneID)]
		if info, err := entry.Info(); err == nil {
			reg.modTime = info.ModTime()
		}
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(i, j int) bool {
		a, b := regs[i], regs[j]
		if a.live.SessionName != b.live.SessionName {
			return a.live.SessionName < b.live.SessionName
		}
		aWindow, _ := strconv.Atoi(a.live.WindowIndex)
		bWindow, _ := strconv.Atoi(b.live.WindowIndex)
		if aWindow != bWindow {
			return aWindow < bWindow
		}
		if a.Directory != b.Directory {
			return a.Directory < b.Directory
		}
		return a.SessionID < b.SessionID
	})
	return regs
}

func memoryExpandKey(key string) string {
	if key == "~" || strings.HasPrefix(key, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(key, "~"))
		}
	}
	return key
}

func memoryShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// Mirrors the opencode bridge glob: * and ? wildcards anchored to the whole
// path, with an optional /<subtree> suffix so a glob key also covers
// everything below what it matches.
func memoryGlobMatch(pattern, dir string) bool {
	var expr strings.Builder
	expr.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			expr.WriteString(".*")
		case '?':
			expr.WriteString(".")
		default:
			expr.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	expr.WriteString("(/.*)?$")
	matched, err := regexp.MatchString(expr.String(), dir)
	return err == nil && matched
}

func memoryKeyMatchesDir(key, dir string) bool {
	pattern := memoryExpandKey(key)
	if pattern == "" {
		return false
	}
	return dir == pattern || strings.HasPrefix(dir, pattern+"/") || memoryGlobMatch(pattern, dir)
}

// Gate precedence shared with the opencode bridge: an exact-path key (raw or
// ~/-expanded) decides on its own, so an explicit false key overrides a
// matching true glob; otherwise any true glob/subtree key enables; default
// disabled.
func memoryStateFor(dir string, enabled map[string]bool) (on bool, viaKey string) {
	dir = memoryExpandKey(strings.TrimSpace(dir))
	if dir == "" {
		return false, ""
	}
	if value, ok := enabled[dir]; ok {
		return value, dir
	}
	keys := make([]string, 0, len(enabled))
	for key := range enabled {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if memoryExpandKey(key) == dir {
			return enabled[key], key
		}
	}
	for _, key := range keys {
		if !enabled[key] {
			continue
		}
		if memoryKeyMatchesDir(key, dir) {
			return true, key
		}
	}
	return false, ""
}

func loadBaseMemoryEnabled(path string) (map[string]bool, error) {
	enabled := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return enabled, nil
		}
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if entry, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(entry, &enabled); err != nil {
			return nil, err
		}
	}
	return enabled, nil
}

func setBaseMemoryEnabled(path, dir string, value bool) error {
	dir = memoryExpandKey(strings.TrimSpace(dir))
	if dir == "" {
		return fmt.Errorf("no directory to toggle")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("base memory state path unknown")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("malformed %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	enabled := map[string]bool{}
	if entry, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(entry, &enabled); err != nil {
			return fmt.Errorf("malformed enabled map in %s: %w", path, err)
		}
	}
	enabled[dir] = value
	encoded, err := json.MarshalIndent(enabled, "", "  ")
	if err != nil {
		return err
	}
	raw["enabled"] = encoded
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".memory-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func loadAllMemoryModes(path string) (map[string][]string, error) {
	modes := map[string][]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return modes, nil
		}
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	entries := map[string]json.RawMessage{}
	if entry, ok := raw["allMode"]; ok {
		if err := json.Unmarshal(entry, &entries); err != nil {
			return nil, err
		}
	}
	for session, value := range entries {
		var stores []string
		if err := json.Unmarshal(value, &stores); err == nil {
			modes[session] = stores
			continue
		}
		var legacy bool
		if err := json.Unmarshal(value, &legacy); err == nil && legacy {
			modes[session] = []string{"global"}
		}
	}
	return modes, nil
}

func setAllMemoryMode(path, session, store string, value bool) error {
	session = strings.TrimSpace(session)
	if session == "" {
		return fmt.Errorf("no session to toggle")
	}
	if store != "global" && store != "repo" {
		return fmt.Errorf("unknown memory store: %s", store)
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("base memory state path unknown")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("malformed %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	entries := map[string][]string{}
	if entry, ok := raw["allMode"]; ok {
		if err := json.Unmarshal(entry, &entries); err != nil {
			return fmt.Errorf("malformed allMode map in %s: %w", path, err)
		}
	}
	stores := []string{}
	for _, existing := range entries[session] {
		if (existing == "global" || existing == "repo") && existing != store {
			stores = append(stores, existing)
		}
	}
	if value {
		stores = append(stores, store)
	}
	if len(stores) > 0 {
		entries[session] = stores
	} else {
		delete(entries, session)
	}
	encoded, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	raw["allMode"] = encoded
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".memory-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func tmuxWindowNamesByID() map[string]string {
	names := map[string]string{}
	out, err := runTmuxOutput("list-windows", "-a", "-F", "#{window_id}\t#{window_name}")
	if err != nil {
		return names
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) == 2 && parts[0] != "" {
			names[parts[0]] = parts[1]
		}
	}
	return names
}

type memoryPanelModel struct {
	windowID       string
	paneID         string
	windowName     string
	agentTitle     string
	sessionID      string
	directory      string
	enabled        bool
	viaKey         string
	allGlobal      bool
	allRepo        bool
	hasRepo        bool
	hasAgent       bool
	stale          bool
	contentLines   []string
	contentOffset  int
	contentRows    int
	width          int
	height         int
	status         string
	statusUntil    time.Time
	requestBack    bool
	usage          memoryUsage
	usageInFlight  bool
	pollGeneration uint64
}

func newMemoryPanelModel(windowID string) *memoryPanelModel {
	m := &memoryPanelModel{windowID: strings.TrimSpace(windowID)}
	m.reload()
	return m
}

func (m *memoryPanelModel) reload() {
	m.hasAgent = false
	m.allGlobal = false
	m.allRepo = false
	enabled, _ := loadBaseMemoryEnabled(baseMemoryStatePath())
	allModes, _ := loadAllMemoryModes(baseMemoryStatePath())
	windowNames := tmuxWindowNamesByID()
	if m.windowName == "" {
		m.windowName = windowNames[m.windowID]
	}
	now := time.Now()
	regs := loadOpSessionRegistrations()
	sort.SliceStable(regs, func(i, j int) bool {
		a, b := regs[i].live.PaneID == m.paneID, regs[j].live.PaneID == m.paneID
		if a != b {
			return a
		}
		return regs[i].lastSeen().After(regs[j].lastSeen())
	})
	for _, reg := range regs {
		if m.windowID != "" && reg.live.WindowID != m.windowID {
			continue
		}
		dir := memoryExpandKey(strings.TrimSpace(reg.Directory))
		if dir == "" {
			continue
		}
		parts := []string{}
		if name := strings.TrimSpace(reg.live.SessionName); name != "" {
			parts = append(parts, name)
		}
		if m.windowName != "" {
			parts = append(parts, m.windowName)
		} else if windowIndex := strings.TrimSpace(reg.live.WindowIndex); windowIndex != "" {
			parts = append(parts, "window "+windowIndex)
		}
		if len(parts) == 0 {
			parts = append(parts, firstNonEmpty(reg.SessionID, "opencode session"))
		}
		m.agentTitle = strings.Join(parts, " · ")
		if m.sessionID != reg.SessionID {
			m.usage = memoryUsage{}
			m.contentOffset = 0
			m.hasRepo = false
		}
		m.sessionID = reg.SessionID
		m.directory = dir
		m.enabled, m.viaKey = memoryStateFor(dir, enabled)
		for _, store := range allModes[m.sessionID] {
			if store == "global" {
				m.allGlobal = true
			}
			if store == "repo" {
				m.allRepo = true
			}
		}
		m.stale = now.Sub(reg.lastSeen()) > memoryStaleAfter
		m.hasAgent = true
		return
	}
}

func (m *memoryPanelModel) Init() tea.Cmd {
	m.pollGeneration++
	return tea.Batch(m.requestUsage(), m.tick())
}

func (m *memoryPanelModel) handleKey(key string) {
	if key == "esc" || key == "q" {
		m.requestBack = true
		return
	}
	if key == "r" || key == "R" {
		m.reload()
		m.contentOffset = 0
		m.setStatus("reloaded", 1200*time.Millisecond)
		return
	}
	if key == "u" || key == "up" {
		if m.contentOffset > 0 {
			m.contentOffset--
		}
		return
	}
	if key == "e" || key == "down" {
		if m.contentOffset < m.maxContentOffset() {
			m.contentOffset++
		}
		return
	}
	if key == "," {
		m.contentOffset = maxInt(0, m.contentOffset-5)
		return
	}
	if key == "." {
		m.contentOffset = minInt(m.maxContentOffset(), m.contentOffset+5)
		return
	}
	if key == "t" || key == "T" || key == " " || key == "space" {
		m.toggleCurrent()
		return
	}
	if key == "a" || key == "A" {
		m.toggleAllMemory("global")
		return
	}
	if key == "p" || key == "P" {
		m.toggleAllMemory("repo")
		return
	}
}

func (m *memoryPanelModel) maxContentOffset() int {
	visible := m.contentVisibleRows()
	n := len(m.contentLines) - visible
	if n < 0 {
		return 0
	}
	return n
}

func (m *memoryPanelModel) contentVisibleRows() int {
	if m.contentRows > 0 {
		return m.contentRows
	}
	if m.height <= 0 {
		return 8
	}
	return maxInt(1, m.height-14)
}

func (m *memoryPanelModel) toggleCurrent() {
	if !m.hasAgent || m.directory == "" {
		return
	}
	next := !m.enabled
	if err := setBaseMemoryEnabled(baseMemoryStatePath(), m.directory, next); err != nil {
		m.setStatus(err.Error(), 2500*time.Millisecond)
		return
	}
	m.reload()
	label := "off"
	if m.enabled {
		label = "on"
	}
	m.setStatus(fmt.Sprintf("memory %s", label), 1500*time.Millisecond)
}

func (m *memoryPanelModel) toggleAllMemory(store string) {
	if !m.hasAgent || m.sessionID == "" {
		return
	}
	if store == "repo" && !m.hasRepo {
		return
	}
	if store == "global" && !m.enabled {
		m.setStatus("global memory is off — press t first", 2200*time.Millisecond)
		return
	}
	next := !m.allGlobal
	if store == "repo" {
		next = !m.allRepo
	}
	if err := setAllMemoryMode(baseMemoryStatePath(), m.sessionID, store, next); err != nil {
		m.setStatus(err.Error(), 2500*time.Millisecond)
		return
	}
	m.reload()
	label := "off"
	if next {
		label = "on — every catalog file of that store next turn"
	}
	m.setStatus(fmt.Sprintf("all %s memory %s", store, label), 2200*time.Millisecond)
}

func (m *memoryPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case memoryPanelTickMsg:
		if msg.panel != m || msg.generation != m.pollGeneration {
			return m, nil
		}
		m.reload()
		return m, tea.Batch(m.requestUsage(), m.tick())
	case memoryUsageMsg:
		m.acceptUsage(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		m.handleKey(msg.String())
		return m, m.requestUsage()
	}
	return m, nil
}

func (m *memoryPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *memoryPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	m.width = width
	m.height = height
	width = maxInt(8, width-2)
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Base memory"),
		styles.meta.Render(truncate("Loaded files and context settings", width)),
	)

	lines := []string{}
	if !m.hasAgent {
		lines = append(lines, "",
			styles.muted.Width(width).Render("No opencode session in this window."),
			styles.meta.Render("Open this panel from the agent's tmux window."),
		)
	} else {
		statusBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("150")).Padding(0, 1).Bold(true).Render("ON")
		stateLine := "global memory available · repo memory is independent"
		if !m.enabled {
			statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("240")).Padding(0, 1).Bold(true).Render("OFF")
			stateLine = "global memory off · repo memory is independent"
		}
		badges := statusBadge
		allBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("141")).Padding(0, 1).Bold(true)
		if m.allGlobal && m.enabled {
			badges = lipgloss.JoinHorizontal(lipgloss.Left, badges, " ", allBadge.Render("ALL GLOBAL"))
		}
		if m.allRepo {
			badges = lipgloss.JoinHorizontal(lipgloss.Left, badges, " ", allBadge.Render("ALL PROJECT"))
		}
		if m.enabled && m.viaKey != "" && memoryExpandKey(m.viaKey) != m.directory {
			stateLine += "  · via " + memoryShortenPath(m.viaKey)
		}
		if m.enabled && m.viaKey != "" && memoryExpandKey(m.viaKey) == m.directory {
			stateLine += "  · exact key"
		}
		if m.stale {
			stateLine += "  · stale"
		}
		lines = append(lines, "", styles.itemTitle.Render(truncate(m.agentTitle, maxInt(8, width-2))),
			styles.meta.Render(truncate("session "+firstNonEmpty(m.sessionID, "?"), width)),
			styles.meta.Render(truncate("directory "+memoryShortenPath(m.directory), width)),
			"",
			lipgloss.JoinHorizontal(lipgloss.Left, badges, " ", styles.meta.Render(truncate(stateLine, maxInt(8, width-lipgloss.Width(badges)-2)))),
		)
	}

	footer := m.renderFooter(styles, width)
	bodyHeight := maxInt(1, height-4-lipgloss.Height(footer))
	m.contentRows = maxInt(1, bodyHeight-7)
	m.contentLines = m.renderMemoryContent(styles, width)
	if m.contentOffset > m.maxContentOffset() {
		m.contentOffset = m.maxContentOffset()
	}
	if m.hasAgent {
		visible := m.contentVisibleRows()
		end := m.contentOffset + visible
		if end > len(m.contentLines) {
			end = len(m.contentLines)
		}
		start := m.contentOffset
		if start > len(m.contentLines) {
			start = len(m.contentLines)
		}
		lines = append(lines, "", strings.Join(m.contentLines[start:end], "\n"))
	}
	body := lipgloss.NewStyle().Height(bodyHeight).Render(strings.Join(lines, "\n"))
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width+2).Height(height).Padding(0, 1).Render(view)
}

func (m *memoryPanelModel) renderMemoryContent(styles paletteStyles, width int) []string {
	rows := []string{styles.itemTitle.Render("Loaded memory")}
	appendRows := func(text string) { rows = append(rows, strings.Split(text, "\n")...) }
	switch m.usage.Status {
	case "ready":
		unit := "files"
		if len(m.usage.Files) == 1 {
			unit = "file"
		}
		rows = strings.Split(styles.itemTitle.Width(width).Render(fmt.Sprintf("Loaded memory · %d %s · ≈%s tokens", len(m.usage.Files), unit, formatMemoryTokens(m.usage.TotalTokens))), "\n")
		if len(m.usage.Files) == 0 {
			appendRows(styles.muted.Width(width).Render("No memory files loaded."))
		}
		for _, file := range m.usage.Files {
			count := "≈" + formatMemoryTokens(file.Tokens)
			pathWidth := maxInt(8, width-lipgloss.Width(count)-3)
			path := styles.item.MarginBottom(0).Padding(0).Width(pathWidth).Render(file.Store + ":" + file.Path)
			appendRows(lipgloss.JoinHorizontal(lipgloss.Top, path, "  ", styles.meta.Render(count)))
		}
		appendRows(styles.muted.Width(width).Render("count-tokens estimate · loaded baselines + updates"))
	case "error":
		appendRows(styles.statusBad.Width(width).Render("Memory usage unavailable: " + m.usage.Error))
	default:
		appendRows(styles.muted.Render("Loading memory usage…"))
	}
	return rows
}

func formatMemoryTokens(tokens int) string {
	text := strconv.Itoa(tokens)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}

func (m *memoryPanelModel) renderFooter(styles paletteStyles, width int) string {
	status := strings.TrimSpace(m.currentStatus())
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	shortcuts := [][2]string{{"t", "toggle"}, {"a", "all global"}, {"u/e", "scroll"}, {"r", "reload"}, {"q/Esc", "back"}}
	if m.hasRepo {
		shortcuts = append(shortcuts[:2], append([][2]string{{"p", "all proj"}}, shortcuts[2:]...)...)
	}
	footer := pickRenderedShortcutFooter(width, renderSegments, shortcuts)
	if status != "" {
		statusText := styles.statusBad.Render(truncate(status, maxInt(12, minInt(36, width/3))))
		if lipgloss.Width(footer)+2+lipgloss.Width(statusText) <= width {
			gap := width - lipgloss.Width(footer) - lipgloss.Width(statusText)
			if gap < 2 {
				gap = 2
			}
			return footer + strings.Repeat(" ", gap) + statusText
		}
		return statusText
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func (m *memoryPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *memoryPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
