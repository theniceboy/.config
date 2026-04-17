package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const paletteNoDeviceOption = "__no_device__"

var paletteModalBorder = lipgloss.Border{
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	TopLeft:     "┌",
	TopRight:    "┐",
	BottomLeft:  "└",
	BottomRight: "┘",
}

var paletteDestroyLauncher = launchPaletteDestroyWithConfirm
var paletteTmuxRunner = runTmux
var paletteTmuxOutput = runTmuxOutput

type paletteRuntime struct {
	windowID           string
	agentID            string
	reg                *registry
	record             *agentRecord
	startupMessage     string
	currentPath        string
	currentSessionName string
	currentWindowName  string
	mainRepoRoot       string
}

type paletteModel struct {
	runtime                 *paletteRuntime
	state                   paletteUIState
	actions                 []paletteAction
	openedAt                time.Time
	quickSecondaryEscCloses bool
	width                   int
	height                  int
	result                  paletteResult
	todo                    *todoPanelModel
	activity                *activityMonitorBT
	devices                 *devicePanelModel
	status                  *statusRightPanelModel
	tracker                 *trackerPanelModel
}

type paletteStyles struct {
	title          lipgloss.Style
	meta           lipgloss.Style
	searchBox      lipgloss.Style
	searchPrompt   lipgloss.Style
	input          lipgloss.Style
	inputCursor    lipgloss.Style
	item           lipgloss.Style
	selectedItem   lipgloss.Style
	sectionLabel   lipgloss.Style
	selectedLabel  lipgloss.Style
	itemTitle      lipgloss.Style
	itemSubtitle   lipgloss.Style
	selectedSubtle lipgloss.Style
	panelTitle     lipgloss.Style
	panelText      lipgloss.Style
	muted          lipgloss.Style
	footer         lipgloss.Style
	keyword        lipgloss.Style
	modal          lipgloss.Style
	modalTitle     lipgloss.Style
	modalBody      lipgloss.Style
	modalHint      lipgloss.Style
	statusBad      lipgloss.Style
	statLabel      lipgloss.Style
	statValue      lipgloss.Style
	todoCheck      lipgloss.Style
	todoCheckDone  lipgloss.Style
	panelTextDone  lipgloss.Style
	shortcutKey    lipgloss.Style
	shortcutText   lipgloss.Style
}

type paletteTodoPreviewItem struct {
	Title string
	Done  bool
}

type paletteTodoPreviewSection struct {
	Title string
	Lead  string
	Items []paletteTodoPreviewItem
	Empty string
}

func runBubbleTeaPalette(args []string) error {
	runtime, err := loadPaletteRuntime(args)
	if err != nil {
		return err
	}
	state := paletteUIState{Mode: paletteModeList, Message: runtime.startupMessage}
	for {
		model := newPaletteModel(runtime, state)
		finalModel, err := tea.NewProgram(model).Run()
		if err != nil {
			return err
		}
		final, ok := finalModel.(*paletteModel)
		if !ok {
			return fmt.Errorf("unexpected palette model type")
		}
		state = final.result.State
		switch final.result.Kind {
		case paletteResultClose:
			return nil
		case paletteResultOpenActivityMonitor:
			err := runtime.runActivityMonitor()
			if errors.Is(err, errClosePalette) {
				return nil
			}
			state.Mode = paletteModeList
			state.Message = paletteMessageForError(err)
			continue
		case paletteResultOpenSnippets:
			state.Mode = paletteModeSnippets
			state.Filter = nil
			state.FilterCursor = 0
			state.Selected = 0
			state.Message = ""
			continue
		case paletteResultRunAction:
			reopen, message, err := runtime.execute(final.result)
			if err != nil {
				if reopen {
					state.Mode = paletteModeList
					state.Message = err.Error()
					continue
				}
				return err
			}
			if !reopen {
				return nil
			}
			state.Mode = paletteModeList
			state.Message = message
			continue
		default:
			return nil
		}
	}
}

func loadPaletteRuntime(args []string) (*paletteRuntime, error) {
	fs := flag.NewFlagSet("agent palette", flag.ContinueOnError)
	var windowID string
	var agentID string
	var currentPath string
	var currentSessionName string
	var currentWindowName string
	fs.StringVar(&windowID, "window", "", "window id")
	fs.StringVar(&agentID, "agent-id", "", "agent id")
	fs.StringVar(&currentPath, "path", "", "current pane path")
	fs.StringVar(&currentSessionName, "session-name", "", "current session name")
	fs.StringVar(&currentWindowName, "window-name", "", "current window name")
	fs.SetOutput(nil)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	runtime := &paletteRuntime{
		windowID:           firstNonEmpty(windowID, os.Getenv("AGENT_PALETTE_WINDOW_ID")),
		agentID:            firstNonEmpty(agentID, os.Getenv("AGENT_PALETTE_AGENT_ID")),
		currentPath:        firstNonEmpty(currentPath, os.Getenv("AGENT_PALETTE_PATH")),
		currentSessionName: firstNonEmpty(currentSessionName, os.Getenv("AGENT_PALETTE_SESSION_NAME")),
		currentWindowName:  firstNonEmpty(currentWindowName, os.Getenv("AGENT_PALETTE_WINDOW_NAME")),
	}
	logPaletteLaunchIfMalformed(runtime)
	if looksLikeTmuxFormatLiteral(runtime.agentID) {
		runtime.agentID = ""
	}
	if runtime.agentID == "" && runtime.windowID != "" {
		if ctx, err := detectCurrentAgentFromTmux(runtime.windowID); err == nil {
			runtime.agentID = ctx.ID
		}
	} else if runtime.agentID == "" {
		if ctx, err := detectCurrentAgentFromTmux(""); err == nil {
			runtime.agentID = ctx.ID
		}
	}
	if err := runtime.reload(); err != nil {
		return nil, err
	}
	return runtime, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func logPaletteLaunchIfMalformed(runtime *paletteRuntime) {
	if runtime == nil {
		return
	}
	values := []string{
		runtime.windowID,
		runtime.agentID,
		runtime.currentPath,
		runtime.currentSessionName,
		runtime.currentWindowName,
	}
	for _, value := range values {
		if strings.Contains(value, "#{") {
			file, err := os.OpenFile("/tmp/agent-palette-launch.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return
			}
			defer file.Close()
			_, _ = fmt.Fprintf(file, "%s window=%q agent=%q path=%q session=%q window_name=%q args=%q\n",
				time.Now().Format(time.RFC3339Nano),
				runtime.windowID,
				runtime.agentID,
				runtime.currentPath,
				runtime.currentSessionName,
				runtime.currentWindowName,
				os.Args,
			)
			return
		}
	}
}

func (r *paletteRuntime) reload() error {
	reg, err := loadRegistry()
	if err != nil {
		r.startupMessage = fmt.Sprintf("Ignoring malformed registry: %v", err)
		reg = &registry{Agents: map[string]*agentRecord{}}
	} else {
		r.startupMessage = ""
	}
	r.reg = reg
	r.record = nil
	if looksLikeTmuxFormatLiteral(r.agentID) {
		r.agentID = ""
	}
	if r.agentID != "" {
		r.record = reg.Agents[r.agentID]
	}
	tmuxValue := func(target string, format string) string {
		args := []string{"display-message", "-p"}
		if strings.TrimSpace(target) != "" {
			args = append(args, "-t", strings.TrimSpace(target))
		}
		args = append(args, format)
		out, err := runTmuxOutput(args...)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	if strings.TrimSpace(r.windowID) == "" {
		r.windowID = tmuxValue("", "#{window_id}")
	}
	if strings.TrimSpace(r.currentPath) == "" {
		r.currentPath = tmuxValue(r.windowID, "#{pane_current_path}")
	}
	if strings.TrimSpace(r.currentSessionName) == "" {
		r.currentSessionName = tmuxValue(r.windowID, "#{session_name}")
	}
	if strings.TrimSpace(r.currentWindowName) == "" {
		r.currentWindowName = tmuxValue(r.windowID, "#{window_name}")
	}
	if inferredAgentID := detectPaletteAgentIDFromPath(r.currentPath); inferredAgentID != "" {
		if strings.TrimSpace(r.agentID) == "" || r.record == nil {
			r.agentID = inferredAgentID
		}
		if r.record == nil {
			r.record = reg.Agents[inferredAgentID]
		}
	}
	if r.record != nil {
		r.agentID = r.record.ID
	}
	r.mainRepoRoot = detectPaletteMainRepoRoot(r.currentPath, r.record)
	return nil
}

func (r *paletteRuntime) effectiveAgentID() string {
	if r.record != nil && strings.TrimSpace(r.record.ID) != "" {
		return strings.TrimSpace(r.record.ID)
	}
	if inferred := detectPaletteAgentIDFromPath(r.currentPath); inferred != "" {
		return inferred
	}
	if looksLikeTmuxFormatLiteral(r.agentID) {
		return ""
	}
	return sanitizeFeatureName(r.agentID)
}

func (r *paletteRuntime) persistRecord(update func(*agentRecord) error) error {
	if r.record == nil {
		return fmt.Errorf("no agent found for this tmux window")
	}
	if err := update(r.record); err != nil {
		return err
	}
	r.record.UpdatedAt = time.Now()
	r.reg.Agents[r.record.ID] = r.record
	if err := saveRegistry(r.reg); err != nil {
		return err
	}
	return r.reload()
}

func (r *paletteRuntime) buildActions() []paletteAction {
	actions := []paletteAction{
		{
			Section:  "Agent",
			Title:    "Start agent",
			Subtitle: startAgentSubtitle(r.mainRepoRoot, r.currentPath),
			Keywords: []string{"agent", "start", "new", "feature", "repo"},
			Kind:     paletteActionPromptStartAgent,
			RepoRoot: r.mainRepoRoot,
		},
	}
	if strings.TrimSpace(r.agentID) != "" {
		actions = append(actions, paletteAction{
			Section:  "Agent",
			Title:    "Destroy agent",
			Subtitle: "Delete the workspace and close its tmux window",
			Keywords: []string{"agent", "destroy", "remove", "delete"},
			Kind:     paletteActionConfirmDestroy,
		})
	}
	actions = append(actions,
		paletteAction{
			Section:  "System",
			Title:    "Tracker",
			Subtitle: "Live tasks and completion status",
			Keywords: []string{"tracker", "tasks", "activity", "status"},
			Kind:     paletteActionOpenTracker,
		},
		paletteAction{
			Section:  "System",
			Title:    "Activity Monitor",
			Subtitle: "View CPU, memory and process usage",
			Keywords: []string{"activity", "monitor", "cpu", "memory", "processes", "top", "ps"},
			Kind:     paletteActionOpenActivityMonitor,
		},
		paletteAction{
			Section:  "System",
			Title:    "Paste snippet",
			Subtitle: "Search and paste a snippet into the current pane",
			Keywords: []string{"snippet", "paste", "template", "text", "insert"},
			Kind:     paletteActionOpenSnippets,
		},
		paletteAction{
			Section:  "System",
			Title:    "Todos",
			Subtitle: "Manage window/global todos",
			Keywords: []string{"todo", "task", "checklist", "manage"},
			Kind:     paletteActionOpenTodos,
		},
		paletteAction{
			Section:  "System",
			Title:    "Edit devices",
			Subtitle: "Add or remove global launch devices",
			Keywords: []string{"devices", "device", "edit", "manage", "web-server"},
			Kind:     paletteActionOpenDevices,
		},
		paletteAction{
			Section:  "System",
			Title:    "Reload tmux config",
			Subtitle: "Source ~/.config/.tmux.conf",
			Keywords: []string{"tmux", "reload", "config", "source", "refresh"},
			Kind:     paletteActionReloadTmuxConfig,
		},
		paletteAction{
			Section:  "System",
			Title:    "Bottom-right status",
			Subtitle: "Open control center for tmux right-side status modules",
			Keywords: []string{"tmux", "status", "status-right", "bottom-right", "control", "center", "istat", "cpu", "network", "memory", "todos", "host", "flash"},
			Kind:     paletteActionOpenStatusRight,
		},
	)
	if strings.TrimSpace(r.agentID) == "" {
		return actions
	}
	if r.record == nil {
		return actions
	}
	return actions
}

func (r *paletteRuntime) runAgentStart(repoRoot, feature, device string, keepWorktree bool) error {
	repoRoot = r.resolveStartRepoRoot(repoRoot)
	feature = sanitizeFeatureName(feature)
	if !isPaletteNoDeviceOption(device) {
		device = normalizeManagedDeviceID(device)
	}
	if repoRoot == "" {
		return fmt.Errorf("main repo not found")
	}
	if feature == "" {
		return fmt.Errorf("feature name is required")
	}
	agentBin := filepath.Join(os.Getenv("HOME"), ".config", "bin", "agent")
	args := buildAgentStartArgs(feature, device, keepWorktree)
	cmd := exec.Command(agentBin, args...)
	cmd.Dir = repoRoot
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if strings.TrimSpace(r.windowID) != "" {
		cmd.Env = append(cmd.Env, "AGENT_TMUX_TARGET_WINDOW="+strings.TrimSpace(r.windowID))
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message != "" {
		return fmt.Errorf("%s", message)
	}
	return err
}

func launchPaletteDestroy(agentID string) error {
	return launchPaletteDestroyWithConfirm(agentID, "")
}

func launchPaletteDestroyWithConfirm(agentID string, confirmText string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("no agent found for this tmux window")
	}
	if _, err := loadDestroyTarget(agentID); err != nil {
		return err
	}
	extraArgs := ""
	if strings.TrimSpace(confirmText) != "" {
		extraArgs = fmt.Sprintf(" --confirm %s", shellQuote(strings.TrimSpace(confirmText)))
	}
	if os.Getenv("TMUX") != "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return runTmux("run-shell", "-b", fmt.Sprintf("%s destroy --id %s%s", shellQuote(exe), shellQuote(agentID), extraArgs))
	}
	args := []string{"destroy", "--id", agentID}
	if strings.TrimSpace(confirmText) != "" {
		args = append(args, "--confirm", strings.TrimSpace(confirmText))
	}
	return spawnDetachedAgentCommand(args...)
}

func buildAgentStartArgs(feature, device string, keepWorktree bool) []string {
	args := []string{"start"}
	if keepWorktree {
		args = append(args, "--keep-worktree")
	}
	if isPaletteNoDeviceOption(device) {
		args = append(args, "--no-device")
	} else if device != "" {
		args = append(args, "-d", device)
	}
	return append(args, feature)
}

func isPaletteNoDeviceOption(device string) bool {
	return strings.TrimSpace(device) == paletteNoDeviceOption
}

func startAgentPromptDevices(repoRoot string) ([]string, int) {
	repoRoot = strings.TrimSpace(repoRoot)
	devices := loadManagedDevices()
	if len(devices) == 0 {
		devices = []string{defaultManagedDeviceID}
	}
	if repoRoot != "" && fileExists(filepath.Join(repoRoot, "pubspec.yaml")) {
		return append([]string{paletteNoDeviceOption}, devices...), 1
	}
	return devices, 0
}

func (r *paletteRuntime) resolveStartRepoRoot(repoRoot string) string {
	tryPaths := []string{
		strings.TrimSpace(repoRoot),
		strings.TrimSpace(r.mainRepoRoot),
		strings.TrimSpace(r.currentPath),
	}
	if strings.TrimSpace(r.windowID) != "" {
		if out, err := runTmuxOutput("display-message", "-p", "-t", strings.TrimSpace(r.windowID), "#{pane_current_path}"); err == nil {
			tryPaths = append(tryPaths, strings.TrimSpace(out))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		tryPaths = append(tryPaths, strings.TrimSpace(cwd))
	}
	for _, path := range tryPaths {
		if resolved := detectPaletteMainRepoRoot(path, r.record); strings.TrimSpace(resolved) != "" {
			return strings.TrimSpace(resolved)
		}
	}
	return ""
}

func (r *paletteRuntime) startSourceBranch(repoRoot string) string {
	repoRoot = r.resolveStartRepoRoot(repoRoot)
	if repoRoot == "" {
		return ""
	}
	repoCfg, err := loadRepoConfigOrDefault(repoRoot)
	if err != nil {
		return detectDefaultBaseBranch(repoRoot)
	}
	return resolveStartSourceBranch(repoRoot, repoCfg)
}

func (r *paletteRuntime) canStartAgent(repoRoot string) bool {
	return strings.TrimSpace(r.resolveStartRepoRoot(repoRoot)) != ""
}

func (r *paletteRuntime) runActivityMonitor() error {
	return runBubbleTeaActivityMonitor(r.windowID)
}

func (r *paletteRuntime) execute(result paletteResult) (bool, string, error) {
	action := result.Action
	text := strings.TrimSpace(result.Input)
	switch action.Kind {
	case paletteActionPromptStartAgent:
		if err := r.runAgentStart(action.RepoRoot, text, result.Device, result.KeepWorktree); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionConfirmDestroy:
		agentID := r.effectiveAgentID()
		if agentID == "" {
			return true, "", fmt.Errorf("no agent found for this tmux window")
		}
		confirmText := ""
		if result.State.ConfirmRequiresText {
			confirmText = strings.TrimSpace(result.Input)
		}
		err := paletteDestroyLauncher(agentID, confirmText)
		if err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionReloadTmuxConfig:
		return false, "", paletteTmuxRunner("source-file", os.Getenv("HOME")+"/.config/.tmux.conf")
	default:
		return false, "", nil
	}
}

func statusRightModuleLabel(module string) string {
	switch module {
	case statusRightModuleCPU:
		return "CPU"
	case statusRightModuleNetwork:
		return "Network"
	case statusRightModuleMemory:
		return "Memory"
	case statusRightModuleMemoryTotals:
		return "Tmux Memory"
	case statusRightModuleAgent:
		return "Agent"
	case statusRightModuleTodoPreview:
		return "Todo Preview"
	case statusRightModuleTodos:
		return "Todos"
	case statusRightModuleFlashMoe:
		return "Flash-MoE"
	case statusRightModuleHost:
		return "Host"
	default:
		return module
	}
}

func statusRightModuleDescription(module string) string {
	switch module {
	case statusRightModuleCPU:
		return "CPU usage"
	case statusRightModuleNetwork:
		return "network throughput"
	case statusRightModuleMemory:
		return "pane memory stats"
	case statusRightModuleMemoryTotals:
		return "window, session, and total tmux memory"
	case statusRightModuleAgent:
		return "active agent device"
	case statusRightModuleTodoPreview:
		return "append the first open window todo to Todos"
	case statusRightModuleTodos:
		return "todo count"
	case statusRightModuleFlashMoe:
		return "Flash-MoE status"
	case statusRightModuleHost:
		return "hostname"
	default:
		return module
	}
}

func togglePaletteStatusRightModule(module string) error {
	if err := toggleStatusRightModule(module); err != nil {
		return err
	}
	return paletteTmuxRunner("refresh-client", "-S")
}

func newPaletteModel(runtime *paletteRuntime, state paletteUIState) *paletteModel {
	if state.Mode == 0 {
		state.Mode = paletteModeList
	}
	state.FilterCursor = clampInt(state.FilterCursor, 0, len(state.Filter))
	state.PromptCursor = clampInt(state.PromptCursor, 0, len(state.PromptText))
	if len(state.PromptDevices) > 0 {
		state.PromptDeviceIndex = clampInt(state.PromptDeviceIndex, 0, len(state.PromptDevices)-1)
	}
	model := &paletteModel{runtime: runtime, state: state, actions: runtime.buildActions(), openedAt: time.Now()}
	if state.Mode == paletteModeTodos {
		_ = model.openTodosPanel()
	}
	if state.Mode == paletteModeActivity {
		_, _ = model.openActivityPanel()
	}
	if state.Mode == paletteModeDevices {
		model.openDevicesPanel()
	}
	if state.Mode == paletteModeStatusRight {
		model.openStatusRightPanel()
	}
	if state.Mode == paletteModeTracker {
		_, _ = model.openTrackerPanel()
	}
	return model
}

func (m *paletteModel) Init() tea.Cmd {
	return nil
}

func (m *paletteModel) noteSecondaryPageOpen() {
	m.quickSecondaryEscCloses = time.Since(m.openedAt) <= 800*time.Millisecond
}

func (m *paletteModel) closePalette() (tea.Model, tea.Cmd) {
	m.result = paletteResult{Kind: paletteResultClose, State: m.state}
	return m, tea.Quit
}

func (m *paletteModel) openTodosPanel() error {
	m.noteSecondaryPageOpen()
	sessionID, windowID := getCurrentTmuxScopeInfo()
	if m.todo == nil {
		panel, err := newTodoPanelModel(sessionID, windowID)
		if err != nil {
			return err
		}
		m.todo = panel
	} else {
		m.todo.sessionID = strings.TrimSpace(sessionID)
		m.todo.windowID = strings.TrimSpace(windowID)
		m.todo.reloadEntries()
		m.todo.clampSelections()
		m.todo.setFocusedPane(todoPanelPaneWindow)
		m.todo.mode = todoPanelModeList
	}
	m.todo.showAltHints = false
	m.state.Mode = paletteModeTodos
	m.state.Message = ""
	m.state.ShowAltHints = false
	return nil
}

func (m *paletteModel) openSnippetsPanel() {
	m.noteSecondaryPageOpen()
	m.state.Mode = paletteModeSnippets
	m.state.Filter = nil
	m.state.FilterCursor = 0
	m.state.Selected = 0
	m.state.SnippetOffset = 0
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openActivityPanel() (tea.Cmd, error) {
	m.noteSecondaryPageOpen()
	if m.activity == nil {
		m.activity = newActivityMonitorModel(m.runtime.windowID, true)
	} else {
		m.activity.windowID = strings.TrimSpace(m.runtime.windowID)
		m.activity.requestBack = false
		m.activity.requestClose = false
	}
	m.activity.width = m.width
	m.activity.height = m.height
	m.activity.showAltHints = false
	m.state.Mode = paletteModeActivity
	m.state.Message = ""
	m.state.ShowAltHints = false
	if !m.activity.refreshInFlight {
		return tea.Batch(
			activityRequestRefreshBT(true, m.activity.refreshedAt.IsZero(), m.activity),
			activityTickCmd(),
		), nil
	}
	return nil, nil
}

func (m *paletteModel) openDevicesPanel() {
	m.noteSecondaryPageOpen()
	if m.devices == nil {
		m.devices = newDevicePanelModel()
	} else {
		m.devices.reload()
		m.devices.mode = devicePanelModeList
		m.devices.requestBack = false
	}
	m.devices.showAltHints = false
	m.state.Mode = paletteModeDevices
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openStatusRightPanel() {
	m.noteSecondaryPageOpen()
	if m.status == nil {
		m.status = newStatusRightPanelModel()
	} else {
		m.status.reload()
		m.status.requestBack = false
	}
	m.status.showAltHints = false
	m.state.Mode = paletteModeStatusRight
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openTrackerPanel() (tea.Cmd, error) {
	m.noteSecondaryPageOpen()
	if m.tracker == nil {
		m.tracker = newTrackerPanelModel(m.runtime)
	} else {
		m.tracker.runtime = m.runtime
		m.tracker.requestBack = false
		m.tracker.requestClose = false
	}
	m.tracker.width = m.width
	m.tracker.height = m.height
	m.tracker.showAltHints = false
	m.state.Mode = paletteModeTracker
	m.state.Message = ""
	m.state.ShowAltHints = false
	return m.tracker.activate(), nil
}

func (m *paletteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.todo != nil {
			m.todo.width = msg.Width
			m.todo.height = msg.Height
		}
		if m.activity != nil {
			m.activity.width = msg.Width
			m.activity.height = msg.Height
		}
		if m.tracker != nil {
			m.tracker.width = msg.Width
			m.tracker.height = msg.Height
		}
		if m.status != nil {
			m.status.width = msg.Width
			m.status.height = msg.Height
		}
	case tea.KeyMsg:
		if m.state.Mode != paletteModeActivity && m.state.Mode != paletteModeTodos && m.state.Mode != paletteModeDevices && m.state.Mode != paletteModeStatusRight && m.state.Mode != paletteModeTracker {
			if isAltFooterToggleKey(msg) {
				m.state.ShowAltHints = !m.state.ShowAltHints
				return m, nil
			}
			m.state.ShowAltHints = false
		}
		key := msg.String()
		if key == "alt+s" {
			if time.Since(m.openedAt) < 250*time.Millisecond {
				return m, nil
			}
			return m.closePalette()
		}
		if key == "esc" && m.quickSecondaryEscCloses {
			switch m.state.Mode {
			case paletteModeTodos:
				if m.todo != nil && m.todo.mode == todoPanelModeList {
					return m.closePalette()
				}
			case paletteModeActivity:
				return m.closePalette()
			case paletteModeDevices:
				if m.devices != nil && m.devices.mode == devicePanelModeList {
					return m.closePalette()
				}
			case paletteModeStatusRight:
				return m.closePalette()
			case paletteModeTracker:
				return m.closePalette()
			case paletteModeSnippets:
				return m.closePalette()
			}
		}
		if m.state.Mode == paletteModeActivity {
			if m.activity == nil {
				cmd, err := m.openActivityPanel()
				if err != nil {
					m.state.Mode = paletteModeList
					m.state.Message = err.Error()
					return m, nil
				}
				return m, cmd
			}
			model, cmd := m.activity.Update(msg)
			if updated, ok := model.(*activityMonitorBT); ok {
				m.activity = updated
			}
			if m.activity.requestClose {
				m.result = paletteResult{Kind: paletteResultClose, State: m.state}
				return m, tea.Quit
			}
			if m.activity.requestBack {
				m.activity.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.activity.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeTodos {
			if key == "esc" && m.todo != nil && m.todo.mode == todoPanelModeList {
				m.state.Mode = paletteModeList
				m.state.Message = m.todo.currentStatus()
				return m, nil
			}
			if m.todo == nil {
				if err := m.openTodosPanel(); err != nil {
					m.state.Mode = paletteModeList
					m.state.Message = err.Error()
					return m, nil
				}
			}
			model, cmd := m.todo.Update(msg)
			if updated, ok := model.(*todoPanelModel); ok {
				m.todo = updated
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeDevices {
			if m.devices == nil {
				m.openDevicesPanel()
			}
			model, cmd := m.devices.Update(msg)
			if updated, ok := model.(*devicePanelModel); ok {
				m.devices = updated
			}
			if m.devices.requestBack {
				m.devices.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.devices.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeStatusRight {
			if m.status == nil {
				m.openStatusRightPanel()
			}
			model, cmd := m.status.Update(msg)
			if updated, ok := model.(*statusRightPanelModel); ok {
				m.status = updated
			}
			if m.status.requestBack {
				m.status.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.status.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeTracker {
			if m.tracker == nil {
				cmd, err := m.openTrackerPanel()
				if err != nil {
					m.state.Mode = paletteModeList
					m.state.Message = err.Error()
					return m, nil
				}
				return m, cmd
			}
			model, cmd := m.tracker.Update(msg)
			if updated, ok := model.(*trackerPanelModel); ok {
				m.tracker = updated
			}
			if m.tracker.requestClose {
				m.result = paletteResult{Kind: paletteResultClose, State: m.state}
				return m, tea.Quit
			}
			if m.tracker.requestBack {
				m.tracker.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.tracker.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		switch m.state.Mode {
		case paletteModePrompt:
			return m.updatePrompt(key)
		case paletteModeConfirmDestroy:
			return m.updateConfirm(key)
		case paletteModeSnippets:
			return m.updateSnippets(key)
		case paletteModeSnippetVars:
			return m.updateSnippetVars(key)
		default:
			return m.updateList(key)
		}
	}
	if m.state.Mode == paletteModeActivity && m.activity != nil {
		model, cmd := m.activity.Update(msg)
		if updated, ok := model.(*activityMonitorBT); ok {
			m.activity = updated
		}
		if m.activity.requestClose {
			m.result = paletteResult{Kind: paletteResultClose, State: m.state}
			return m, tea.Quit
		}
		if m.activity.requestBack {
			m.activity.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.activity.currentStatus()
			return m, nil
		}
		return m, cmd
	}
	if m.state.Mode == paletteModeTodos && m.todo != nil {
		model, cmd := m.todo.Update(msg)
		if updated, ok := model.(*todoPanelModel); ok {
			m.todo = updated
		}
		return m, cmd
	}
	if m.state.Mode == paletteModeDevices && m.devices != nil {
		model, cmd := m.devices.Update(msg)
		if updated, ok := model.(*devicePanelModel); ok {
			m.devices = updated
		}
		if m.devices.requestBack {
			m.devices.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.devices.currentStatus()
			return m, nil
		}
		return m, cmd
	}
	if m.state.Mode == paletteModeStatusRight && m.status != nil {
		model, cmd := m.status.Update(msg)
		if updated, ok := model.(*statusRightPanelModel); ok {
			m.status = updated
		}
		if m.status.requestBack {
			m.status.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.status.currentStatus()
			return m, nil
		}
		return m, cmd
	}
	if m.state.Mode == paletteModeTracker && m.tracker != nil {
		model, cmd := m.tracker.Update(msg)
		if updated, ok := model.(*trackerPanelModel); ok {
			m.tracker = updated
		}
		if m.tracker.requestClose {
			m.result = paletteResult{Kind: paletteResultClose, State: m.state}
			return m, tea.Quit
		}
		if m.tracker.requestBack {
			m.tracker.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.tracker.currentStatus()
			return m, nil
		}
		return m, cmd
	}
	return m, nil
}

func (m *paletteModel) updateList(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "ctrl+c" || key == "alt+n" {
		m.result = paletteResult{Kind: paletteResultClose, State: m.state}
		return m, tea.Quit
	}
	if key == "alt+a" {
		cmd, err := m.openActivityPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	}
	if key == "alt+p" {
		m.openSnippetsPanel()
		return m, nil
	}
	if key == "alt+r" {
		cmd, err := m.openTrackerPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	}
	if key == "alt+t" {
		if err := m.openTodosPanel(); err != nil {
			m.state.Message = err.Error()
		}
		return m, nil
	}
	if key == "alt+c" {
		m.openPrompt(palettePromptStartAgent, "", m.runtime.mainRepoRoot)
		return m, nil
	}
	actions := m.filteredActions()
	navigate := func(delta int) {
		if len(actions) == 0 {
			m.state.Selected = 0
			return
		}
		next := clampInt(m.state.Selected, 0, len(actions)-1) + delta
		if next < 0 {
			next = len(actions) - 1
		} else if next >= len(actions) {
			next = 0
		}
		m.state.Selected = next
	}
	switch key {
	case "ctrl+u", "alt+u", "up":
		navigate(-1)
		return m, nil
	case "ctrl+e", "alt+e", "down":
		navigate(1)
		return m, nil
	case "ctrl+n", "left":
		m.state.FilterCursor = clampInt(m.state.FilterCursor-1, 0, len(m.state.Filter))
		return m, nil
	case "ctrl+i", "tab", "right":
		m.state.FilterCursor = clampInt(m.state.FilterCursor+1, 0, len(m.state.Filter))
		return m, nil
	case "enter", "alt+i":
		if len(actions) == 0 || m.state.Selected < 0 || m.state.Selected >= len(actions) {
			return m, nil
		}
		return m.selectAction(actions[m.state.Selected])
	}
	if applyPaletteInputKey(key, &m.state.Filter, &m.state.FilterCursor, false) {
		m.state.Selected = 0
		m.state.ActionOffset = 0
		m.state.Message = ""
	}
	return m, nil
}

func (m *paletteModel) selectAction(action paletteAction) (tea.Model, tea.Cmd) {
	switch action.Kind {
	case paletteActionPromptStartAgent:
		m.openPrompt(palettePromptStartAgent, "", action.RepoRoot)
		return m, nil
	case paletteActionConfirmDestroy:
		target, err := loadDestroyTarget(m.runtime.effectiveAgentID())
		if err != nil {
			m.state.Message = err.Error()
			m.state.Mode = paletteModeList
			return m, nil
		}
		m.state.Mode = paletteModeConfirmDestroy
		m.state.Message = ""
		m.state.ShowAltHints = false
		m.state.ConfirmRequiresText = target.RequiresExplicitConfirm
		m.state.PromptText = nil
		m.state.PromptCursor = 0
		return m, nil
	case paletteActionOpenActivityMonitor:
		cmd, err := m.openActivityPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	case paletteActionOpenSnippets:
		m.openSnippetsPanel()
		return m, nil
	case paletteActionOpenTracker:
		cmd, err := m.openTrackerPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	case paletteActionOpenTodos:
		if err := m.openTodosPanel(); err != nil {
			m.state.Message = err.Error()
		}
		return m, nil
	case paletteActionOpenDevices:
		m.openDevicesPanel()
		return m, nil
	case paletteActionOpenStatusRight:
		m.openStatusRightPanel()
		return m, nil
	default:
		m.state.Mode = paletteModeList
		m.result = paletteResult{Kind: paletteResultRunAction, Action: action, State: m.state}
		return m, tea.Quit
	}
}

func (m *paletteModel) openPrompt(kind palettePromptKind, initial string, repoRoot string) {
	devices := []string(nil)
	deviceIndex := 0
	if kind == palettePromptStartAgent {
		resolvedRepoRoot := strings.TrimSpace(m.runtime.resolveStartRepoRoot(repoRoot))
		devices, deviceIndex = startAgentPromptDevices(resolvedRepoRoot)
	}
	m.state.Mode = paletteModePrompt
	m.state.PromptKind = kind
	m.state.PromptField = palettePromptFieldName
	m.state.PromptText = []rune(initial)
	m.state.PromptCursor = len(m.state.PromptText)
	m.state.PromptRepoRoot = strings.TrimSpace(repoRoot)
	m.state.PromptDevices = devices
	m.state.PromptDeviceIndex = deviceIndex
	m.state.PromptKeepWorktree = false
	m.state.ShowAltHints = false
	m.state.Message = ""
}

func (m *paletteModel) updatePrompt(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.state.Mode = paletteModeList
		m.state.Message = ""
		return m, nil
	}
	if m.state.PromptKind == palettePromptStartAgent {
		if !m.runtime.canStartAgent(m.state.PromptRepoRoot) {
			return m, nil
		}
		if key == "alt+d" || key == "alt+D" || key == "alt+shift+d" {
			deviceCount := len(m.state.PromptDevices)
			if deviceCount == 0 {
				resolvedRepoRoot := strings.TrimSpace(m.runtime.resolveStartRepoRoot(m.state.PromptRepoRoot))
				m.state.PromptDevices, m.state.PromptDeviceIndex = startAgentPromptDevices(resolvedRepoRoot)
				deviceCount = len(m.state.PromptDevices)
			}
			if deviceCount > 0 {
				selected := clampInt(m.state.PromptDeviceIndex, 0, deviceCount-1)
				if key == "alt+d" {
					m.state.PromptDeviceIndex = (selected + 1) % deviceCount
				} else {
					m.state.PromptDeviceIndex = (selected - 1 + deviceCount) % deviceCount
				}
			}
			return m, nil
		}
		switch key {
		case "tab", "ctrl+i":
			switch m.state.PromptField {
			case palettePromptFieldName:
				m.state.PromptField = palettePromptFieldDevice
			case palettePromptFieldDevice:
				m.state.PromptField = palettePromptFieldWorktree
			default:
				m.state.PromptField = palettePromptFieldName
			}
			return m, nil
		case "shift+tab":
			switch m.state.PromptField {
			case palettePromptFieldWorktree:
				m.state.PromptField = palettePromptFieldDevice
			case palettePromptFieldDevice:
				m.state.PromptField = palettePromptFieldName
			default:
				m.state.PromptField = palettePromptFieldWorktree
			}
			return m, nil
		}
		if m.state.PromptField == palettePromptFieldDevice {
			deviceCount := len(m.state.PromptDevices)
			if deviceCount == 0 {
				m.state.PromptDevices = []string{defaultManagedDeviceID}
				deviceCount = 1
			}
			switch key {
			case "ctrl+n", "left", "n":
				m.state.PromptDeviceIndex = clampInt(m.state.PromptDeviceIndex-1, 0, deviceCount-1)
				return m, nil
			case "right", "i":
				m.state.PromptDeviceIndex = clampInt(m.state.PromptDeviceIndex+1, 0, deviceCount-1)
				return m, nil
			}
		}
		if m.state.PromptField == palettePromptFieldWorktree {
			switch key {
			case "ctrl+n", "left", "n":
				m.state.PromptKeepWorktree = false
				return m, nil
			case "right", "i":
				m.state.PromptKeepWorktree = true
				return m, nil
			case " ", "space":
				m.state.PromptKeepWorktree = !m.state.PromptKeepWorktree
				return m, nil
			}
		}
	}
	if key == "enter" {
		text := strings.TrimSpace(string(m.state.PromptText))
		if m.state.PromptKind == palettePromptStartAgent && text == "" {
			m.state.Message = "Feature name is required"
			m.state.Mode = paletteModeList
			return m, nil
		}
		action := paletteAction{}
		switch m.state.PromptKind {
		case palettePromptStartAgent:
			action = paletteAction{Kind: paletteActionPromptStartAgent, RepoRoot: m.state.PromptRepoRoot}
		}
		device := ""
		if m.state.PromptKind == palettePromptStartAgent && m.state.PromptDeviceIndex >= 0 && m.state.PromptDeviceIndex < len(m.state.PromptDevices) {
			device = m.state.PromptDevices[m.state.PromptDeviceIndex]
		}
		m.state.Mode = paletteModeList
		m.result = paletteResult{Kind: paletteResultRunAction, Action: action, Input: text, Device: device, KeepWorktree: m.state.PromptKeepWorktree, State: m.state}
		return m, tea.Quit
	}
	if m.state.PromptKind == palettePromptStartAgent && m.state.PromptField != palettePromptFieldName {
		return m, nil
	}
	applyPaletteInputKey(key, &m.state.PromptText, &m.state.PromptCursor, true)
	return m, nil
}

func (m *paletteModel) updateConfirm(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.state.Mode = paletteModeList
		m.state.ConfirmRequiresText = false
		m.state.PromptText = nil
		m.state.PromptCursor = 0
		return m, nil
	}
	if m.state.ConfirmRequiresText {
		if key == "enter" {
			if strings.TrimSpace(string(m.state.PromptText)) != "destroy" {
				m.state.Message = "Type destroy to confirm"
				return m, nil
			}
			m.state.Mode = paletteModeList
			m.result = paletteResult{Kind: paletteResultRunAction, Action: paletteAction{Kind: paletteActionConfirmDestroy}, Input: strings.TrimSpace(string(m.state.PromptText)), State: m.state}
			return m, tea.Quit
		}
		applyPaletteInputKey(key, &m.state.PromptText, &m.state.PromptCursor, true)
		return m, nil
	}
	if key == "y" || key == "Y" {
		m.state.Mode = paletteModeList
		m.state.ConfirmRequiresText = false
		m.result = paletteResult{Kind: paletteResultRunAction, Action: paletteAction{Kind: paletteActionConfirmDestroy}, State: m.state}
		return m, tea.Quit
	}
	m.state.Mode = paletteModeList
	m.state.ConfirmRequiresText = false
	return m, nil
}

func (m *paletteModel) updateSnippets(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "ctrl+c" {
		m.state.Mode = paletteModeList
		m.state.Message = ""
		return m, nil
	}
	snippets := m.filteredSnippets()
	navigate := func(delta int) {
		if len(snippets) == 0 {
			m.state.Selected = 0
			return
		}
		m.state.Selected = clampInt(m.state.Selected+delta, 0, len(snippets)-1)
	}
	switch key {
	case "ctrl+u", "up":
		navigate(-1)
		return m, nil
	case "ctrl+e", "down":
		navigate(1)
		return m, nil
	case "ctrl+n", "left":
		m.state.FilterCursor = clampInt(m.state.FilterCursor-1, 0, len(m.state.Filter))
		return m, nil
	case "ctrl+i", "tab", "right":
		m.state.FilterCursor = clampInt(m.state.FilterCursor+1, 0, len(m.state.Filter))
		return m, nil
	case "enter":
		if len(snippets) == 0 || m.state.Selected < 0 || m.state.Selected >= len(snippets) {
			return m, nil
		}
		snippet := snippets[m.state.Selected]
		if len(snippet.Vars) > 0 {
			m.state.SnippetName = snippet.Name
			m.state.SnippetContent = snippet.Content
			m.state.SnippetVars = snippet.Vars
			m.state.SnippetVarIndex = 0
			m.state.SnippetVarValues = make(map[string]string)
			m.state.PromptText = nil
			m.state.PromptCursor = 0
			m.state.Mode = paletteModeSnippetVars
			return m, nil
		}
		if err := pasteToTmuxPane(snippet.Content); err != nil {
			m.state.Mode = paletteModeList
			m.state.Message = err.Error()
			return m, nil
		}
		m.result = paletteResult{Kind: paletteResultClose, State: m.state}
		return m, tea.Quit
	}
	if applyPaletteInputKey(key, &m.state.Filter, &m.state.FilterCursor, false) {
		m.state.Selected = 0
		m.state.SnippetOffset = 0
		m.state.Message = ""
	}
	return m, nil
}

func (m *paletteModel) updateSnippetVars(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.state.Mode = paletteModeSnippets
		m.state.Message = ""
		return m, nil
	}
	if key == "enter" {
		varName := m.state.SnippetVars[m.state.SnippetVarIndex]
		m.state.SnippetVarValues[varName] = string(m.state.PromptText)
		m.state.SnippetVarIndex++
		if m.state.SnippetVarIndex >= len(m.state.SnippetVars) {
			rendered := renderSnippet(m.state.SnippetContent, m.state.SnippetVarValues)
			if err := pasteToTmuxPane(rendered); err != nil {
				m.state.Mode = paletteModeList
				m.state.Message = err.Error()
				return m, nil
			}
			m.result = paletteResult{Kind: paletteResultClose, State: m.state}
			return m, tea.Quit
		}
		m.state.PromptText = nil
		m.state.PromptCursor = 0
		return m, nil
	}
	applyPaletteInputKey(key, &m.state.PromptText, &m.state.PromptCursor, true)
	return m, nil
}

func (m *paletteModel) filteredSnippets() []snippet {
	snippets := loadSnippets()
	query := strings.ToLower(strings.TrimSpace(string(m.state.Filter)))
	if query == "" {
		return snippets
	}
	parts := strings.Fields(query)
	filtered := make([]snippet, 0, len(snippets))
	for _, s := range snippets {
		haystack := strings.ToLower(s.Name + " " + s.Description + " " + s.Content)
		matched := true
		for _, part := range parts {
			if !strings.Contains(haystack, part) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func (m *paletteModel) View() string {
	width := m.width
	height := m.height
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	if width < 48 || height < 14 {
		return "Window too small for command palette"
	}
	styles := newPaletteStyles()
	if m.state.Mode == paletteModePrompt {
		return m.renderPrompt(styles, width, height)
	}
	if m.state.Mode == paletteModeConfirmDestroy {
		return m.renderConfirm(styles, width, height)
	}
	if m.state.Mode == paletteModeActivity {
		if m.activity != nil {
			m.activity.width = width
			m.activity.height = height
			return m.activity.View()
		}
		return styles.muted.Render("Activity monitor unavailable")
	}
	if m.state.Mode == paletteModeSnippets {
		return m.renderSnippets(styles, width, height)
	}
	if m.state.Mode == paletteModeSnippetVars {
		return m.renderSnippetVars(styles, width, height)
	}
	if m.state.Mode == paletteModeTodos {
		if m.todo != nil {
			m.todo.width = width
			m.todo.height = height
			return m.todo.View()
		}
		return styles.muted.Render("Todo panel unavailable")
	}
	if m.state.Mode == paletteModeDevices {
		if m.devices != nil {
			m.devices.width = width
			m.devices.height = height
			return m.devices.render(styles, width, height)
		}
		return styles.muted.Render("Device panel unavailable")
	}
	if m.state.Mode == paletteModeStatusRight {
		if m.status != nil {
			m.status.width = width
			m.status.height = height
			return m.status.render(styles, width, height)
		}
		return styles.muted.Render("Status panel unavailable")
	}
	if m.state.Mode == paletteModeTracker {
		if m.tracker != nil {
			m.tracker.width = width
			m.tracker.height = height
			return m.tracker.render(styles, width, height)
		}
		return styles.muted.Render("Tracker unavailable")
	}
	return m.renderListView(styles, width, height)
}

func (m *paletteModel) renderListView(styles paletteStyles, width, height int) string {
	actions := m.filteredActions()
	if len(actions) == 0 {
		m.state.Selected = 0
	} else {
		m.state.Selected = clampInt(m.state.Selected, 0, len(actions)-1)
	}
	title := "Command Palette"
	if m.runtime.record != nil {
		title = title + "  " + styles.keyword.Render(m.runtime.record.ID)
	}
	metaParts := []string{}
	if m.runtime.currentSessionName != "" {
		metaParts = append(metaParts, m.runtime.currentSessionName)
	}
	if m.runtime.currentWindowName != "" {
		metaParts = append(metaParts, m.runtime.currentWindowName)
	}
	if m.runtime.mainRepoRoot != "" {
		metaParts = append(metaParts, filepathBaseOrFull(m.runtime.mainRepoRoot))
	}
	header := styles.title.Render(title)
	if len(metaParts) > 0 {
		header = lipgloss.JoinVertical(lipgloss.Left, header, styles.meta.Render(strings.Join(metaParts, "  ·  ")))
	}
	filterLine := styles.searchBox.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.state.Filter, m.state.FilterCursor, styles)),
		),
	)
	contentHeight := maxInt(8, height-7)
	listWidth := maxInt(34, width*48/100)
	sidebarWidth := maxInt(28, width-listWidth-3)
	list := m.renderActions(styles, actions, listWidth, contentHeight)
	sidebar := m.renderSidebar(styles, sidebarWidth, contentHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, strings.Repeat(" ", 3), sidebar)
	footer := renderPaletteFooter(styles, width, m.state.Message, m.state.ShowAltHints)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *paletteModel) renderActions(styles paletteStyles, actions []paletteAction, width, height int) string {
	entriesPerPage := maxInt(1, (height-2)/3)
	selected := clampInt(m.state.Selected, 0, maxInt(0, len(actions)-1))
	offset := stableListOffset(m.state.ActionOffset, selected, entriesPerPage, len(actions))
	m.state.ActionOffset = offset
	blocks := []string{styles.meta.Render(fmt.Sprintf("%d commands", len(actions))), ""}
	if len(actions) == 0 {
		blocks = append(blocks, styles.muted.Width(width).Render("No matching commands"))
	} else {
		for row := 0; row < entriesPerPage; row++ {
			idx := offset + row
			if idx >= len(actions) {
				break
			}
			action := actions[idx]
			sectionLabel := styles.sectionLabel
			subtle := styles.itemSubtitle
			titleStyle := styles.itemTitle
			box := styles.item
			markerText := "  "
			markerStyle := styles.muted
			rowStyle := lipgloss.NewStyle().Width(maxInt(16, width-2))
			fillStyle := lipgloss.NewStyle()
			if idx == selected {
				selectedBG := lipgloss.Color("238")
				sectionLabel = styles.selectedLabel.Background(selectedBG)
				subtle = styles.selectedSubtle.Background(selectedBG)
				titleStyle = styles.itemTitle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				box = styles.selectedItem
				markerText = "› "
				markerStyle = styles.selectedLabel.Background(selectedBG)
				rowStyle = rowStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				fillStyle = fillStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
			}
			innerWidth := maxInt(16, width-2)
			labelText := strings.ToUpper(action.Section)
			labelWidth := lipgloss.Width(labelText)
			markerWidth := lipgloss.Width(markerText)
			titleWidth := maxInt(10, innerWidth-markerWidth-labelWidth-1)
			titleText := truncate(action.Title, titleWidth)
			gapWidth := maxInt(1, innerWidth-markerWidth-lipgloss.Width(titleText)-labelWidth)
			titleRow := rowStyle.Render(
				markerStyle.Render(markerText) +
					titleStyle.Render(titleText) +
					fillStyle.Render(strings.Repeat(" ", gapWidth)) +
					sectionLabel.Render(labelText),
			)
			subtitleRow := rowStyle.Render(fillStyle.Render(strings.Repeat(" ", markerWidth)) + subtle.Render(truncate(action.Subtitle, maxInt(0, innerWidth-markerWidth))))
			block := lipgloss.JoinVertical(lipgloss.Left, titleRow, subtitleRow)
			blocks = append(blocks, box.Width(width).Render(block))
		}
	}
	content := strings.Join(blocks, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (m *paletteModel) renderSidebar(styles paletteStyles, width, height int) string {
	lines := []string{}
	trackerContext, trackerAgent, trackerBootstrap := m.runtime.sidebarTrackerStatus()
	lines = append(lines, styles.panelTitle.Render("Tracker Status"))
	lines = append(lines, renderPaletteStat(styles, "Context", trackerContext, width, 9))
	lines = append(lines, renderPaletteStat(styles, "Agent", trackerAgent, width, 9))
	lines = append(lines, renderPaletteStat(styles, "Bootstrap", trackerBootstrap, width, 9))
	lines = append(lines, "")
	lines = append(lines, styles.panelTitle.Render("Todo Preview"))
	previewLimit := clampInt((height-6)/4, 1, 3)
	sections := m.runtime.sidebarTodoPreviewSections()
	for idx, section := range sections {
		if idx > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderPaletteTodoPreviewSection(styles, section, width, previewLimit)...)
	}
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (r *paletteRuntime) sidebarTrackerStatus() (contextSummary, agentSummary, bootstrapSummary string) {
	contextParts := []string{}
	if r.currentSessionName != "" {
		contextParts = append(contextParts, r.currentSessionName)
	}
	if r.currentWindowName != "" {
		contextParts = append(contextParts, r.currentWindowName)
	}
	if r.mainRepoRoot != "" {
		contextParts = append(contextParts, filepathBaseOrFull(r.mainRepoRoot))
	} else if r.currentPath != "" {
		contextParts = append(contextParts, filepathBaseOrFull(r.currentPath))
	}
	if len(contextParts) == 0 {
		contextSummary = "No tmux context detected"
	} else {
		contextSummary = strings.Join(contextParts, "  ·  ")
	}
	if r.record == nil {
		agentID := r.effectiveAgentID()
		if agentID == "" {
			return contextSummary, "No active agent", "No active agent"
		}
		return contextSummary, fmt.Sprintf("%s not loaded", agentID), "No active agent"
	}
	agentSummary = r.record.ID
	if r.record.Branch != "" {
		agentSummary = agentSummary + " on " + r.record.Branch
	}
	bootstrapSummary = paletteBootstrapStatus(r.record)
	return contextSummary, agentSummary, bootstrapSummary
}

func (r *paletteRuntime) sidebarTodoPreviewSections() []paletteTodoPreviewSection {
	sections := []paletteTodoPreviewSection{}
	store, err := loadTmuxTodoStore()
	windowID := strings.TrimSpace(r.windowID)
	if err != nil {
		sections = append(sections, paletteTodoPreviewSection{Title: "Window", Empty: "Todo store unavailable"})
		sections = append(sections, paletteTodoPreviewSection{Title: "Global", Empty: "Todo store unavailable"})
	} else {
		windowSection := paletteTodoPreviewSection{Title: "Window", Empty: "No window todos"}
		if windowID == "" {
			windowSection.Empty = "No window context"
		} else {
			windowSection.Items = paletteTmuxTodoPreviewItems(todoItemsForScope(store, todoScopeWindow, windowID))
		}
		sections = append(sections, windowSection)
		sections = append(sections, paletteTodoPreviewSection{
			Title: "Global",
			Items: paletteTmuxTodoPreviewItems(todoItemsForScope(store, todoScopeGlobal, "")),
			Empty: "No global todos",
		})
	}
	return sections
}

func paletteBootstrapStatus(record *agentRecord) string {
	if record == nil {
		return "No active agent"
	}
	workspaceRoot := strings.TrimSpace(record.WorkspaceRoot)
	if workspaceRoot == "" {
		return "No workspace"
	}
	if fileExists(bootstrapRepoReadyPath(workspaceRoot)) {
		return paletteBootstrapLabel("ready", paletteBootstrapPID(workspaceRoot))
	}
	if fileExists(bootstrapFailedPath(workspaceRoot)) {
		message := firstPaletteLine(readPaletteBootstrapFailure(workspaceRoot))
		if message == "" {
			return "failed"
		}
		return "failed: " + message
	}
	pid := paletteBootstrapPID(workspaceRoot)
	if fileExists(bootstrapGitReadyPath(workspaceRoot)) {
		return paletteBootstrapLabel("copying repo", pid)
	}
	if pid > 0 {
		return paletteBootstrapLabel("preparing git", pid)
	}
	return "preparing git"
}

func paletteBootstrapPID(workspaceRoot string) int {
	data, err := os.ReadFile(bootstrapPIDPath(workspaceRoot))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !processRunning(pid) {
		return 0
	}
	return pid
}

func paletteBootstrapLabel(status string, pid int) string {
	if pid <= 0 {
		return status
	}
	return fmt.Sprintf("%s (pid %d)", status, pid)
}

func readPaletteBootstrapFailure(workspaceRoot string) string {
	data, err := os.ReadFile(bootstrapFailedPath(workspaceRoot))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (m *paletteModel) renderPrompt(styles paletteStyles, width, height int) string {
	title := "Input"
	detail := "Enter a value"
	if m.state.PromptKind == palettePromptStartAgent {
		title = "Start agent"
		repoRoot := blankIfEmpty(m.runtime.resolveStartRepoRoot(m.state.PromptRepoRoot), "Main repo not found")
		if repoRoot == "Main repo not found" {
			body := lipgloss.JoinVertical(lipgloss.Left,
				styles.modalTitle.Render(title),
				styles.statusBad.Render(repoRoot),
				"",
				styles.modalHint.Render(renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
					[][][2]string{{{"Esc", "back"}, {footerHintToggleKey, "more"}}},
					[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
				)),
			)
			box := styles.modal.Width(minInt(72, maxInt(36, width-10))).Render(body)
			return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
		}
		sourceBranch := blankIfEmpty(m.runtime.startSourceBranch(m.state.PromptRepoRoot), "Unavailable")
		devices := m.state.PromptDevices
		if len(devices) == 0 {
			devices = []string{defaultManagedDeviceID}
		}
		nameLabel := styles.modalHint.Render("NAME")
		deviceLabel := styles.modalHint.Render("DEVICE")
		worktreeLabel := styles.modalHint.Render("WORKTREE")
		if m.state.PromptField == palettePromptFieldName {
			nameLabel = styles.selectedLabel.Render("NAME")
		} else if m.state.PromptField == palettePromptFieldDevice {
			deviceLabel = styles.selectedLabel.Render("DEVICE")
		} else {
			worktreeLabel = styles.selectedLabel.Render("WORKTREE")
		}
		deviceChips := make([]string, 0, len(devices))
		for idx, deviceID := range devices {
			deviceChips = append(deviceChips, renderPaletteDeviceChip(styles, deviceID, idx == clampInt(m.state.PromptDeviceIndex, 0, len(devices)-1)))
		}
		worktreeChips := []string{
			renderPaletteDeviceChip(styles, "CLEAR", !m.state.PromptKeepWorktree),
			renderPaletteDeviceChip(styles, "KEEP", m.state.PromptKeepWorktree),
		}
		body := lipgloss.JoinVertical(lipgloss.Left,
			styles.modalTitle.Render(title),
			styles.modalBody.Render(repoRoot),
			"",
			styles.modalHint.Render("BRANCH"),
			styles.modalBody.Render(sourceBranch),
			"",
			nameLabel,
			styles.input.Render(renderInputValue(m.state.PromptText, m.state.PromptCursor, styles)),
			"",
			deviceLabel,
			styles.modalBody.Render(strings.Join(deviceChips, " ")),
			"",
			worktreeLabel,
			styles.modalBody.Render(strings.Join(worktreeChips, " ")),
			"",
			styles.modalHint.Render(renderPaletteHintLine(styles, minInt(64, maxInt(28, width-18)), m.state.ShowAltHints,
				[][][2]string{
					{{"Enter", "create"}, {"Tab", "focus"}, {"n/i", "choose"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
					{{"Enter", "create"}, {"n/i", "choose"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
					{{"Esc", "back"}, {footerHintToggleKey, "more"}},
				},
				[][][2]string{
					{{"Alt-D", "next"}, {"Alt-Shift-D", "prev"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
					{{"Alt-D", "next"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
					{{"Alt-S", "close"}},
				},
			)),
		)
		box := styles.modal.Width(minInt(84, maxInt(40, width-10))).Render(body)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render(title),
		styles.modalBody.Render(detail),
		"",
		styles.input.Render(renderInputValue(m.state.PromptText, m.state.PromptCursor, styles)),
		"",
		styles.modalHint.Render(renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
			[][][2]string{{{"Enter", "save"}, {"Esc", "back"}, {footerHintToggleKey, "more"}}, {{"Esc", "back"}, {footerHintToggleKey, "more"}}},
			[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
		)),
	)
	box := styles.modal.Width(minInt(72, maxInt(34, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *paletteModel) renderConfirm(styles paletteStyles, width, height int) string {
	agentID := "this agent"
	detail := "Remove " + agentID + " and close its tmux window?"
	hint := renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
		[][][2]string{{{"y", "confirm"}, {"Esc", "cancel"}, {footerHintToggleKey, "more"}}, {{"Esc", "cancel"}, {footerHintToggleKey, "more"}}},
		[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
	)
	if m.runtime.record != nil {
		agentID = m.runtime.record.ID
		detail = "Remove " + agentID + " and close its tmux window?"
		if windowID := activeAgentWindowID(m.runtime.record); windowID != "" {
			if openTodos, err := countOpenTmuxTodos(todoScopeWindow, windowID); err == nil && openTodos > 0 {
				label := "todos"
				if openTodos == 1 {
					label = "todo"
				}
				detail = fmt.Sprintf("Close %d open window %s before destroying %s.", openTodos, label, agentID)
				hint = renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
					[][][2]string{{{"Esc", "cancel"}, {footerHintToggleKey, "more"}}},
					[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
				)
			}
		}
	}
	if m.state.ConfirmRequiresText {
		detail = detail + " Uncommitted changes detected; type destroy to continue."
		hint = renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
			[][][2]string{{{"Enter", "confirm"}, {"Esc", "cancel"}, {footerHintToggleKey, "more"}}, {{"Esc", "cancel"}, {footerHintToggleKey, "more"}}},
			[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
		)
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render("Destroy agent"),
		styles.modalBody.Render(detail),
		func() string {
			if !m.state.ConfirmRequiresText {
				return ""
			}
			return styles.input.Render(renderInputValue(m.state.PromptText, m.state.PromptCursor, styles))
		}(),
		"",
		styles.modalHint.Render(hint),
	)
	box := styles.modal.Width(minInt(72, maxInt(36, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *paletteModel) renderSnippets(styles paletteStyles, width, height int) string {
	snippets := m.filteredSnippets()
	if len(snippets) == 0 {
		m.state.Selected = 0
	} else {
		m.state.Selected = clampInt(m.state.Selected, 0, len(snippets)-1)
	}

	title := "Paste Snippet"
	header := styles.title.Render(title)

	filterLine := styles.searchBox.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.state.Filter, m.state.FilterCursor, styles)),
		),
	)

	contentHeight := maxInt(8, height-7)
	listWidth := maxInt(34, width*52/100)
	previewWidth := maxInt(28, width-listWidth-3)

	list := m.renderSnippetList(styles, snippets, listWidth, contentHeight)
	preview := m.renderSnippetPreview(styles, snippets, previewWidth, contentHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, strings.Repeat(" ", 3), preview)

	footer := renderPaletteModeFooter(styles, width, m.state.Message, m.state.ShowAltHints,
		[][][2]string{
			{{"Ctrl-U/E", "move"}, {"Ctrl-N/I", "filter"}, {"Enter", "paste"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			{{"Ctrl-U/E", "move"}, {"Enter", "paste"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			{{"Enter", "paste"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
		},
		[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
	)

	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *paletteModel) renderSnippetList(styles paletteStyles, snippets []snippet, width, height int) string {
	entriesPerPage := maxInt(1, (height-2)/2)
	selected := clampInt(m.state.Selected, 0, maxInt(0, len(snippets)-1))
	offset := stableListOffset(m.state.SnippetOffset, selected, entriesPerPage, len(snippets))
	m.state.SnippetOffset = offset

	blocks := []string{styles.meta.Render(fmt.Sprintf("%d snippets", len(snippets))), ""}
	if len(snippets) == 0 {
		blocks = append(blocks, styles.muted.Width(width).Render("No matching snippets"))
	} else {
		for row := 0; row < entriesPerPage; row++ {
			idx := offset + row
			if idx >= len(snippets) {
				break
			}
			snippet := snippets[idx]
			selectedBG := lipgloss.Color("238")
			titleStyle := styles.itemTitle
			subtitleStyle := styles.itemSubtitle
			rowStyle := lipgloss.NewStyle().Width(maxInt(16, width-2))
			fillStyle := lipgloss.NewStyle()
			varLabelStyle := styles.sectionLabel
			if idx == selected {
				titleStyle = styles.itemTitle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				subtitleStyle = styles.selectedSubtle.Background(selectedBG)
				rowStyle = rowStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				fillStyle = fillStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				varLabelStyle = styles.selectedLabel.Background(selectedBG)
			}

			varLabel := ""
			if len(snippet.Vars) > 0 {
				varLabel = varLabelStyle.Render(fmt.Sprintf(" %d vars", len(snippet.Vars)))
			}

			innerWidth := maxInt(16, width-2)
			titleText := truncate(snippet.Name, innerWidth-lipgloss.Width(varLabel)-1)
			gapWidth := maxInt(1, innerWidth-lipgloss.Width(titleText)-lipgloss.Width(varLabel))

			titleRow := rowStyle.Render(
				titleStyle.Render(titleText) +
					fillStyle.Render(strings.Repeat(" ", gapWidth)) +
					varLabel,
			)
			desc := snippet.Description
			if desc == "" {
				desc = truncate(snippet.Content, 40)
			}
			subtitleRow := rowStyle.Render(fillStyle.Render("  ") + subtitleStyle.Render(truncate(desc, innerWidth-2)))
			block := lipgloss.JoinVertical(lipgloss.Left, titleRow, subtitleRow)
			blocks = append(blocks, styles.item.Width(width).Render(block))
		}
	}

	content := strings.Join(blocks, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (m *paletteModel) renderSnippetPreview(styles paletteStyles, snippets []snippet, width, height int) string {
	lines := []string{}
	if len(snippets) > 0 && m.state.Selected >= 0 && m.state.Selected < len(snippets) {
		snippet := snippets[m.state.Selected]
		lines = append(lines, styles.panelTitle.Render("Preview"))
		lines = append(lines, styles.title.Render(snippet.Name))
		if snippet.Description != "" {
			lines = append(lines, styles.muted.Render(snippet.Description))
		}
		lines = append(lines, "")
		if len(snippet.Vars) > 0 {
			chips := []string{}
			for _, v := range snippet.Vars {
				chips = append(chips, styles.keyword.Render("{{"+v+"}}"))
			}
			lines = append(lines, "Variables: "+strings.Join(chips, " "))
			lines = append(lines, "")
		}
		lines = append(lines, styles.panelTitle.Render("Content"))
		for _, l := range wrapText(snippet.Content, maxInt(10, width-2)) {
			lines = append(lines, styles.panelText.Render(truncate(l, width)))
		}
	}
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (m *paletteModel) renderSnippetVars(styles paletteStyles, width, height int) string {
	varName := m.state.SnippetVars[m.state.SnippetVarIndex]
	progress := fmt.Sprintf("(%d/%d)", m.state.SnippetVarIndex+1, len(m.state.SnippetVars))
	title := fmt.Sprintf("Enter %s %s", varName, progress)

	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render(title),
		styles.modalBody.Render("Value for {{"+varName+"}}"),
		"",
		styles.input.Render(renderInputValue(m.state.PromptText, m.state.PromptCursor, styles)),
		"",
		styles.modalHint.Render(renderPaletteHintLine(styles, minInt(52, maxInt(20, width-18)), m.state.ShowAltHints,
			[][][2]string{{{"Enter", "continue"}, {"Esc", "back"}, {footerHintToggleKey, "more"}}, {{"Esc", "back"}, {footerHintToggleKey, "more"}}},
			[][][2]string{{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}, {{"Alt-S", "close"}}},
		)),
	)
	box := styles.modal.Width(minInt(72, maxInt(34, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *paletteModel) filteredActions() []paletteAction {
	query := strings.ToLower(strings.TrimSpace(string(m.state.Filter)))
	if query == "" {
		return m.actions
	}
	parts := strings.Fields(query)
	filtered := make([]paletteAction, 0, len(m.actions))
	for _, action := range m.actions {
		haystack := strings.ToLower(action.Title)
		matched := true
		for _, part := range parts {
			if !strings.Contains(haystack, part) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func newPaletteStyles() paletteStyles {
	return paletteStyles{
		title:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		meta:           lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		searchBox:      lipgloss.NewStyle().Background(lipgloss.Color("236")).Padding(0, 1),
		searchPrompt:   lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Bold(true),
		input:          lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		inputCursor:    lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("223")).Bold(true),
		item:           lipgloss.NewStyle().Padding(0, 1).MarginBottom(1),
		selectedItem:   lipgloss.NewStyle().Padding(0, 1).MarginBottom(1).Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")),
		sectionLabel:   lipgloss.NewStyle().Foreground(lipgloss.Color("180")).Bold(true),
		selectedLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Bold(true),
		itemTitle:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		itemSubtitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		selectedSubtle: lipgloss.NewStyle().Foreground(lipgloss.Color("251")),
		panelTitle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("223")),
		panelText:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		muted:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		footer:         lipgloss.NewStyle().Foreground(lipgloss.Color("216")),
		keyword:        lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("237")).Padding(0, 1),
		modal:          lipgloss.NewStyle().Border(paletteModalBorder).BorderForeground(lipgloss.Color("223")).Padding(1, 2),
		modalTitle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		modalBody:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		modalHint:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		statusBad:      lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		statLabel:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		statValue:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		todoCheck:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		todoCheckDone:  lipgloss.NewStyle().Foreground(lipgloss.Color("150")),
		panelTextDone:  lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
		shortcutKey:    lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("223")).Padding(0, 1).Bold(true),
		shortcutText:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
}

func renderInputValue(text []rune, cursor int, styles paletteStyles) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}
	left := string(text[:cursor])
	right := string(text[cursor:])
	cursorChar := " "
	if cursor < len(text) {
		cursorChar = string(text[cursor])
		right = string(text[cursor+1:])
	}
	if len(text) == 0 && cursor == 0 {
		cursorChar = " "
	}
	return left + styles.inputCursor.Render(cursorChar) + right
}

func applyPaletteInputKey(key string, text *[]rune, cursor *int, allowEnter bool) bool {
	if text == nil || cursor == nil {
		return false
	}
	switch key {
	case "left":
		*cursor = clampInt(*cursor-1, 0, len(*text))
		return true
	case "right":
		*cursor = clampInt(*cursor+1, 0, len(*text))
		return true
	case "backspace", "ctrl+h":
		if *cursor > 0 {
			*text = append((*text)[:*cursor-1], (*text)[*cursor:]...)
			*cursor--
		}
		return true
	case "delete":
		if *cursor < len(*text) {
			*text = append((*text)[:*cursor], (*text)[*cursor+1:]...)
		}
		return true
	case "ctrl+a", "home":
		*cursor = 0
		return true
	case "ctrl+e", "end":
		*cursor = len(*text)
		return true
	case "ctrl+u":
		*text = (*text)[*cursor:]
		*cursor = 0
		return true
	case "ctrl+w":
		start := previousWordBoundary(*text, *cursor)
		*text = append((*text)[:start], (*text)[*cursor:]...)
		*cursor = start
		return true
	case "enter":
		return allowEnter
	}
	r, ok := paletteRuneFromKey(key)
	if !ok {
		return false
	}
	*text = append((*text)[:*cursor], append([]rune{r}, (*text)[*cursor:]...)...)
	*cursor++
	return true
}

func paletteRuneFromKey(key string) (rune, bool) {
	if key == "space" {
		return ' ', true
	}
	runes := []rune(key)
	if len(runes) == 1 {
		return runes[0], true
	}
	return 0, false
}

func renderVerticalDivider(height int) string {
	lines := make([]string, maxInt(1, height))
	for i := range lines {
		lines[i] = "│"
	}
	return strings.Join(lines, "\n")
}

func renderPaletteStat(styles paletteStyles, label, value string, width int, labelWidth int) string {
	parts := wrapText(value, maxInt(10, width-labelWidth-3))
	if len(parts) == 0 {
		parts = []string{"-"}
	}
	lines := []string{styles.statLabel.Width(labelWidth).Render(label+":") + " " + styles.statValue.Render(parts[0])}
	for _, part := range parts[1:] {
		lines = append(lines, strings.Repeat(" ", labelWidth+1)+styles.statValue.Render(part))
	}
	return strings.Join(lines, "\n")
}

func renderPaletteModeFooter(styles paletteStyles, width int, message string, showAltHints bool, normalCandidates [][][2]string, altCandidates [][][2]string) string {
	message = strings.TrimSpace(message)
	if message != "" {
		style := styles.footer
		lower := strings.ToLower(message)
		if strings.Contains(lower, "error") || strings.Contains(lower, "required") || strings.Contains(lower, "unknown") {
			style = styles.statusBad
		}
		return style.Width(width).Render(truncate(message, width))
	}
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	candidates := normalCandidates
	if showAltHints {
		candidates = altCandidates
	}
	footer := pickRenderedShortcutFooter(width, renderSegments, candidates...)
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func renderPaletteFooter(styles paletteStyles, width int, message string, showAltHints bool) string {
	return renderPaletteModeFooter(styles, width, message, showAltHints,
		[][][2]string{
			{{"Ctrl-U/E", "move"}, {"Ctrl-N/I", "filter"}, {"Enter", "run"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			{{"Ctrl-U/E", "move"}, {"Enter", "run"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			{{"Enter", "run"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
		},
		[][][2]string{
			{{"Alt-U/E", "move"}, {"Alt-I", "run"}, {"Alt-C", "create"}, {"Alt-R", "tracker"}, {"Alt-A", "activity"}, {"Alt-P", "snippets"}, {"Alt-T", "todos"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			{{"Alt-C", "create"}, {"Alt-R", "tracker"}, {"Alt-A", "activity"}, {"Alt-T", "todos"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			{{"Alt-C", "create"}, {"Alt-R", "tracker"}, {"Alt-S", "close"}},
		},
	)
}

func renderPaletteHintLine(styles paletteStyles, width int, showAltHints bool, normalCandidates [][][2]string, altCandidates [][][2]string) string {
	return pickRenderedShortcutFooter(width, func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "  ", pairs)
	}, func() [][][2]string {
		if showAltHints {
			return altCandidates
		}
		return normalCandidates
	}()...)
}

func paletteTmuxTodoPreviewItems(items []tmuxTodoItem) []paletteTodoPreviewItem {
	rows := make([]paletteTodoPreviewItem, 0, len(items))
	for _, item := range items {
		title := firstPaletteLine(item.Title)
		if title == "" || item.Done {
			continue
		}
		rows = append(rows, paletteTodoPreviewItem{Title: title, Done: item.Done})
	}
	return rows
}

func renderPaletteTodoPreviewSection(styles paletteStyles, section paletteTodoPreviewSection, width int, previewLimit int) []string {
	lines := []string{styles.statLabel.Render(section.Title)}
	if section.Lead != "" {
		lines = append(lines, renderPalettePreviewValue(styles, section.Lead, width, 2)...)
	}
	if len(section.Items) == 0 {
		if section.Lead == "" {
			lines = append(lines, styles.muted.Render("  "+section.Empty))
		}
		return lines
	}
	limit := clampInt(previewLimit, 1, len(section.Items))
	for _, item := range section.Items[:limit] {
		lines = append(lines, renderPaletteTodoPreviewItem(styles, item, width, 2)...)
	}
	hidden := len(section.Items) - limit
	if hidden > 0 {
		lines = append(lines, styles.muted.Render(fmt.Sprintf("  +%d more", hidden)))
	}
	return lines
}

func renderPaletteTodoPreviewItem(styles paletteStyles, item paletteTodoPreviewItem, width int, indent int) []string {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return nil
	}
	check := "○"
	checkStyle := styles.todoCheck
	textStyle := styles.panelText
	if item.Done {
		check = "●"
		checkStyle = styles.todoCheckDone
		textStyle = styles.panelTextDone
	}
	indentPrefix := strings.Repeat(" ", maxInt(0, indent))
	textPrefix := indentPrefix + check + " "
	available := maxInt(10, width-lipgloss.Width(textPrefix))
	parts := wrapText(title, available)
	if len(parts) == 0 {
		parts = []string{title}
	}
	lines := []string{indentPrefix + checkStyle.Render(check) + " " + textStyle.Render(truncate(parts[0], available))}
	continuationPrefix := strings.Repeat(" ", lipgloss.Width(textPrefix))
	for _, part := range parts[1:] {
		lines = append(lines, continuationPrefix+textStyle.Render(truncate(part, available)))
	}
	return lines
}

func renderPalettePreviewValue(styles paletteStyles, value string, width int, indent int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	prefix := strings.Repeat(" ", maxInt(0, indent))
	available := maxInt(10, width-len([]rune(prefix)))
	parts := wrapText(value, available)
	if len(parts) == 0 {
		parts = []string{value}
	}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, prefix+styles.panelText.Render(truncate(part, available)))
	}
	return lines
}

func renderPaletteDeviceChip(styles paletteStyles, deviceID string, active bool) string {
	chipStyle := styles.keyword
	if active {
		chipStyle = styles.keyword.Copy().Foreground(lipgloss.Color("223")).Background(lipgloss.Color("238")).Bold(true)
	}
	label := deviceID
	if isPaletteNoDeviceOption(deviceID) {
		label = "NONE"
	}
	return chipStyle.Render(label)
}

func firstPaletteLine(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "\n")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func paletteMessageForError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func paletteSuccessMessage(err error, success string) string {
	if err != nil {
		return ""
	}
	return success
}

func filepathBaseOrFull(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}

func clampInt(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
