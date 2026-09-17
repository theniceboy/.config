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
	sessionID          string
	windowID           string
	paneID             string
	agentID            string
	reg                *registry
	record             *agentRecord
	startupMessage     string
	currentPath        string
	currentSessionName string
	currentWindowName  string
	currentWindowIndex string
	currentPaneIndex   string
	mainRepoRoot       string
	startMode          paletteMode
}

type paletteModel struct {
	runtime                 *paletteRuntime
	state                   paletteUIState
	actions                 []paletteAction
	openedAt                time.Time
	quickSecondaryEscCloses bool
	singlePanelMode         bool
	width                   int
	height                  int
	result                  paletteResult
	todo                    *todoPanelModel
	activity                *activityMonitorBT
	devices                 *devicePanelModel
	status                  *statusRightPanelModel
	tracker                 *trackerPanelModel
	goals                   *goalPanelModel
	agentList               *actionListPanel
	opencodeForkList        *actionListPanel
	deviceSwitch            *deviceSwitchPanelModel
	restoreAgent            *restoreAgentPanelModel
	quotas                  *llmQuotaPanelModel
	memory                  *memoryPanelModel
	board                   *boardPanelModel
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
	state := paletteUIState{Mode: runtime.startMode, Message: runtime.startupMessage}
	for {
		model := newPaletteModel(runtime, state)
		finalModel, err := tea.NewProgram(model, tea.WithoutBracketedPaste()).Run()
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
	var sessionID string
	var paneID string
	var agentID string
	var currentPath string
	var currentSessionName string
	var currentWindowName string
	var currentWindowIndex string
	var currentPaneIndex string
	var modeFlag string
	fs.StringVar(&windowID, "window", "", "window id")
	fs.StringVar(&sessionID, "session-id", "", "session id")
	fs.StringVar(&paneID, "pane-id", "", "pane id")
	fs.StringVar(&agentID, "agent-id", "", "agent id")
	fs.StringVar(&currentPath, "path", "", "current pane path")
	fs.StringVar(&currentSessionName, "session-name", "", "current session name")
	fs.StringVar(&currentWindowName, "window-name", "", "current window name")
	fs.StringVar(&currentWindowIndex, "window-index", "", "current window index")
	fs.StringVar(&currentPaneIndex, "pane-index", "", "current pane index")
	fs.StringVar(&modeFlag, "mode", "", "initial panel mode (goals, tracker, todos, activity, status, quotas, memory, board)")
	fs.SetOutput(nil)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	runtime := &paletteRuntime{
		sessionID:          firstNonEmpty(sessionID, os.Getenv("AGENT_PALETTE_SESSION_ID")),
		windowID:           firstNonEmpty(windowID, os.Getenv("AGENT_PALETTE_WINDOW_ID")),
		paneID:             firstNonEmpty(paneID, os.Getenv("AGENT_PALETTE_PANE_ID")),
		agentID:            firstNonEmpty(agentID, os.Getenv("AGENT_PALETTE_AGENT_ID")),
		currentPath:        firstNonEmpty(currentPath, os.Getenv("AGENT_PALETTE_PATH")),
		currentSessionName: firstNonEmpty(currentSessionName, os.Getenv("AGENT_PALETTE_SESSION_NAME")),
		currentWindowName:  firstNonEmpty(currentWindowName, os.Getenv("AGENT_PALETTE_WINDOW_NAME")),
		currentWindowIndex: firstNonEmpty(currentWindowIndex, os.Getenv("AGENT_PALETTE_WINDOW_INDEX")),
		currentPaneIndex:   firstNonEmpty(currentPaneIndex, os.Getenv("AGENT_PALETTE_PANE_INDEX")),
	}
	switch strings.ToLower(modeFlag) {
	case "goals":
		runtime.startMode = paletteModeGoals
	case "tracker":
		runtime.startMode = paletteModeTracker
	case "todos":
		runtime.startMode = paletteModeTodos
	case "activity":
		runtime.startMode = paletteModeActivity
	case "status", "status-right", "bottom-right":
		runtime.startMode = paletteModeStatusRight
	case "quotas", "llm-quotas":
		runtime.startMode = paletteModeLLMQuotas
	case "memory", "base-memory":
		runtime.startMode = paletteModeMemory
	case "board":
		runtime.startMode = paletteModeBoard
	default:
		runtime.startMode = paletteModeList
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
		runtime.sessionID,
		runtime.windowID,
		runtime.paneID,
		runtime.agentID,
		runtime.currentPath,
		runtime.currentSessionName,
		runtime.currentWindowName,
		runtime.currentWindowIndex,
		runtime.currentPaneIndex,
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
	if strings.TrimSpace(r.sessionID) == "" {
		r.sessionID = tmuxValue(r.windowID, "#{session_id}")
	}
	if strings.TrimSpace(r.paneID) == "" {
		r.paneID = tmuxValue(r.windowID, "#{pane_id}")
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
	if strings.TrimSpace(r.currentWindowIndex) == "" {
		r.currentWindowIndex = tmuxValue(r.windowID, "#{window_index}")
	}
	if strings.TrimSpace(r.currentPaneIndex) == "" {
		r.currentPaneIndex = tmuxValue(r.windowID, "#{pane_index}")
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

func (r *paletteRuntime) workspaceForBrowser() string {
	if r.record != nil && r.record.BrowserEnabled {
		if ws := strings.TrimSpace(r.record.WorkspaceRoot); ws != "" {
			return ws
		}
	}
	return ""
}

func (r *paletteRuntime) opencodePaneLocator() string {
	sessionName := strings.TrimSpace(r.currentSessionName)
	windowIndex := strings.TrimSpace(r.currentWindowIndex)
	paneIndex := strings.TrimSpace(r.currentPaneIndex)
	if sessionName == "" || windowIndex == "" || paneIndex == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s.%s", sessionName, windowIndex, paneIndex)
}

func (r *paletteRuntime) currentOpenCodeSessionID() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state")
	}
	paths := []string{}
	if paneID := strings.TrimSpace(r.paneID); paneID != "" {
		paths = append(paths, filepath.Join(stateDir, "op", "pane_"+sanitizeStateKey(paneID)))
	}
	if locator := r.opencodePaneLocator(); locator != "" {
		paths = append(paths, filepath.Join(stateDir, "op", "loc_"+sanitizeStateKey(locator)))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fields := strings.Fields(string(data))
		if len(fields) > 0 && strings.TrimSpace(fields[0]) != "" {
			return strings.TrimSpace(fields[0])
		}
	}
	return ""
}

func (r *paletteRuntime) canForkCurrentOpenCode() bool {
	return r.currentOpenCodeSessionID() != ""
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

func (r *paletteRuntime) applyDeviceSwitch(deviceID string) error {
	if r.record == nil {
		return fmt.Errorf("no agent record loaded")
	}
	deviceID = normalizeManagedDeviceID(deviceID)
	if deviceID == "" {
		return fmt.Errorf("invalid device id")
	}
	workspace := strings.TrimSpace(r.record.WorkspaceRoot)
	paneID := strings.TrimSpace(r.record.Panes.Run)
	if workspace == "" || paneID == "" {
		return fmt.Errorf("agent workspace or run pane missing")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "feature", "--workspace", workspace, "--device", deviceID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(string(out)), err.Error()))
	}
	if err := r.reload(); err != nil {
		return err
	}
	runCmd := gatedWorkspaceCommand(
		workspace,
		bootstrapRepoReadyPath(workspace),
		fmt.Sprintf("cd %s; ./ensure-server.sh %s; exec ${SHELL:-/bin/zsh}", shellQuote(workspace), shellQuote(deviceID)),
	)
	if err := runTmux("respawn-pane", "-k", "-t", paneID, runCmd); err != nil {
		return fmt.Errorf("failed to restart server: %w", err)
	}
	return nil
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
		}, paletteAction{
			Section:  "Agent",
			Title:    "Close agent",
			Subtitle: "Close tmux window + stop processes (workspace kept; resume restores)",
			Keywords: []string{"agent", "close", "stop", "shutdown"},
			Kind:     paletteActionCloseAgent,
		})
	}
	if len(restorableAgentItems(r.mainRepoRoot)) > 0 {
		actions = append(actions, paletteAction{
			Section:  "Restore",
			Title:    "Restore agent",
			Subtitle: "Reopen a closed agent + resume its opencode session",
			Keywords: []string{"agent", "restore", "resume", "reopen"},
			Kind:     paletteActionRestoreAgent,
			RepoRoot: r.mainRepoRoot,
		})
	}
	if r.canForkCurrentOpenCode() {
		actions = append(actions, paletteAction{
			Section:  "Opencode",
			Title:    "Fork current opencode",
			Subtitle: "Open this session in another tmux pane or window",
			Keywords: []string{"opencode", "op", "fork", "session", "pane", "window"},
			Kind:     paletteActionOpenOpencodeFork,
		})
	}
	actions = append(actions,
		paletteAction{
			Section:  "System",
			Title:    "Scratch terminal",
			Subtitle: "Open the persistent scratch shell",
			Keywords: []string{"scratch", "terminal", "shell", "popup", "tmux"},
			Kind:     paletteActionOpenScratch,
		},
		paletteAction{
			Section:  "System",
			Title:    "Goals",
			Subtitle: "Goals, threads and todos",
			Keywords: []string{"goal", "goals", "thread", "threads", "tracker", "tasks", "status"},
			Kind:     paletteActionOpenGoals,
		},
		paletteAction{
			Section:  "System",
			Title:    "Tracker",
			Subtitle: "Live tasks and completion status",
			Keywords: []string{"tracker", "tasks", "activity", "status"},
			Kind:     paletteActionOpenTracker,
		},
		paletteAction{
			Section:  "System",
			Title:    "Memory — agent context toggles",
			Subtitle: "Loaded memory files, token estimates and global memory toggle",
			Keywords: []string{"memory", "tokens", "files", "base", "context", "toggle", "enable", "disable", "opencode"},
			Kind:     paletteActionOpenMemory,
		},
		paletteAction{
			Section:  "System",
			Title:    "Board",
			Subtitle: "Browse ~/base work items (read-only, via board export)",
			Keywords: []string{"board", "items", "todo", "doing", "work", "projectone", "hq", "projecttwo"},
			Kind:     paletteActionOpenBoard,
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
			Title:    "LLM quotas",
			Subtitle: "Live Z.AI and Codex subscription usage",
			Keywords: []string{"llm", "quota", "quotas", "usage", "zai", "codex", "cliproxy", "limits"},
			Kind:     paletteActionOpenLLMQuotas,
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
	if r.record.BrowserEnabled {
		actions = append(actions,
			paletteAction{
				Section:  "Browser",
				Title:    "Copy console logs",
				Subtitle: "Copy browser dev console to clipboard",
				Keywords: []string{"browser", "console", "logs", "copy", "clipboard", "devtools"},
				Kind:     paletteActionBrowserCopyLogs,
			},
			paletteAction{
				Section:  "Browser",
				Title:    "Paste console logs",
				Subtitle: "Read browser dev console buffer and paste into pane",
				Keywords: []string{"browser", "console", "logs", "paste", "devtools", "chrome"},
				Kind:     paletteActionBrowserLogs,
			},
			paletteAction{
				Section:  "Browser",
				Title:    "Clear console logs",
				Subtitle: "Clear the browser dev console buffer",
				Keywords: []string{"browser", "console", "logs", "clear", "devtools"},
				Kind:     paletteActionBrowserClearLogs,
			},
			paletteAction{
				Section:  "Browser",
				Title:    "Hot reload",
				Subtitle: "Analyze + flutter reload",
				Keywords: []string{"browser", "reload", "hot", "flutter", "refresh"},
				Kind:     paletteActionBrowserReload,
			},
		)
	}
	return actions
}

func (r *paletteRuntime) runAgentStart(repoRoot, feature, device string, keepWorktree, pull bool) error {
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
	agentBin := filepath.Join(os.Getenv("HOME"), ".config", "agent-tracker", "bin", "agent")
	args := buildAgentStartArgs(feature, device, keepWorktree, pull)
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

func launchPaletteClose(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("no agent found for this tmux window")
	}
	if _, err := loadDestroyTarget(agentID); err != nil {
		return err
	}
	if os.Getenv("TMUX") != "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return runTmux("run-shell", "-b", fmt.Sprintf("%s close --id %s", shellQuote(exe), shellQuote(agentID)))
	}
	return spawnDetachedAgentCommand("close", "--id", agentID)
}

func launchPaletteRestore(agentID, repoRoot string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("no agent selected")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if os.Getenv("TMUX") != "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := fmt.Sprintf("%s restore --id %s", shellQuote(exe), shellQuote(agentID))
		if repoRoot != "" {
			cmd = fmt.Sprintf("cd %s && %s", shellQuote(repoRoot), cmd)
		}
		return runTmux("run-shell", "-b", cmd)
	}
	return spawnDetachedAgentCommand("restore", "--id", agentID)
}

func buildAgentStartArgs(feature, device string, keepWorktree, pull bool) []string {
	args := []string{"start"}
	if keepWorktree {
		args = append(args, "--keep-worktree")
	}
	if !pull {
		args = append(args, "--no-pull")
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
	isFlutter := repoRoot != "" && fileExists(filepath.Join(repoRoot, "pubspec.yaml"))
	if isFlutter {
		devices = append([]string{paletteNoDeviceOption}, devices...)
	}
	repoCfg, _ := loadRepoConfigOrDefault(repoRoot)
	preferred := resolveDefaultDevice(repoCfg)
	if isPaletteNoDeviceOption(preferred) || preferred == "" {
		preferred = paletteNoDeviceOption
	}
	for i, d := range devices {
		if d == preferred {
			return devices, i
		}
	}
	if isFlutter {
		return devices, 1
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
		if err := r.runAgentStart(action.RepoRoot, text, result.Device, result.KeepWorktree, result.Pull); err != nil {
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
	case paletteActionCloseAgent:
		agentID := r.effectiveAgentID()
		if agentID == "" {
			return true, "", fmt.Errorf("no agent found for this tmux window")
		}
		if err := launchPaletteClose(agentID); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionReloadTmuxConfig:
		return false, "", paletteTmuxRunner("source-file", os.Getenv("HOME")+"/.config/.tmux.conf")
	case paletteActionOpenScratch:
		return false, "", launchScratchTerminalFromPalette(r.currentPath)
	case paletteActionForkOpencodeHorizontal:
		if err := r.launchOpenCodeFork("horizontal"); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionForkOpencodeVertical:
		if err := r.launchOpenCodeFork("vertical"); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionForkOpencodeWindow:
		if err := r.launchOpenCodeFork("window"); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionBrowserClearLogs:
		workspace := r.workspaceForBrowser()
		if workspace == "" {
			return true, "", fmt.Errorf("no browser-enabled agent in this window")
		}
		exe, err := os.Executable()
		if err != nil {
			return false, "", err
		}
		cmd := exec.Command(exe, "browser", "clear-logs", "--workspace", workspace)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return true, "", fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(string(output)), err.Error()))
		}
		return false, "Console logs cleared", nil
	case paletteActionBrowserReload:
		workspace := r.workspaceForBrowser()
		if workspace == "" {
			return true, "", fmt.Errorf("no browser-enabled agent in this window")
		}
		exe, err := os.Executable()
		if err != nil {
			return false, "", err
		}
		cmd := exec.Command(exe, "hot-reload", "--workspace", workspace)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return true, "", err
		}
		return false, "", nil
	case paletteActionRestartAgentServer:
		if r.record == nil || r.record.Runtime != "flutter" || strings.TrimSpace(r.record.Device) == "" || strings.TrimSpace(r.record.Panes.Run) == "" || strings.TrimSpace(r.record.WorkspaceRoot) == "" {
			return true, "", fmt.Errorf("no restartable Flutter server for this agent")
		}
		runCmd := gatedWorkspaceCommand(
			r.record.WorkspaceRoot,
			bootstrapRepoReadyPath(r.record.WorkspaceRoot),
			fmt.Sprintf("cd %s; ./ensure-server.sh %s; exec ${SHELL:-/bin/zsh}", shellQuote(r.record.WorkspaceRoot), shellQuote(r.record.Device)),
		)
		if err := runTmux("respawn-pane", "-k", "-t", r.record.Panes.Run, runCmd); err != nil {
			return true, "", fmt.Errorf("failed to restart server: %w", err)
		}
		return false, "Restarting agent server", nil
	default:
		return false, "", nil
	}
}

func launchScratchTerminalFromPalette(currentPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("sleep 0.1; %s tmux scratch --path %s", shellQuote(exe), shellQuote(currentPath))
	return runTmux("run-shell", "-b", cmd)
}

func (r *paletteRuntime) launchOpenCodeFork(kind string) error {
	sessionID := r.currentOpenCodeSessionID()
	if sessionID == "" {
		return fmt.Errorf("no opencode session found for the current pane")
	}
	path := strings.TrimSpace(r.currentPath)
	target := firstNonEmpty(r.paneID, r.windowID)
	var args []string
	switch kind {
	case "horizontal":
		if target == "" {
			return fmt.Errorf("current tmux pane is unknown")
		}
		args = []string{"split-window", "-h", "-P", "-F", "#{pane_id}", "-t", target}
	case "vertical":
		if target == "" {
			return fmt.Errorf("current tmux pane is unknown")
		}
		args = []string{"split-window", "-v", "-P", "-F", "#{pane_id}", "-t", target}
	case "window":
		forkName := nextForkWindowName(r.currentWindowName, r.sessionID)
		args = []string{"new-window", "-a", "-P", "-F", "#{pane_id}", "-n", forkName}
		if strings.TrimSpace(r.windowID) != "" {
			args = append(args, "-t", strings.TrimSpace(r.windowID))
		} else if strings.TrimSpace(r.sessionID) != "" {
			args = append(args, "-t", strings.TrimSpace(r.sessionID))
		}
	default:
		return fmt.Errorf("unknown fork target")
	}
	if path != "" {
		args = append(args, "-c", path)
	}
	out, err := paletteTmuxOutput(args...)
	if err != nil {
		return err
	}
	paneID := strings.TrimSpace(out)
	if paneID == "" {
		return fmt.Errorf("tmux did not return a new pane")
	}
	if err := waitForShellPane(paneID, 2*time.Second); err != nil {
		return err
	}
	launcher := "op"
	if err := paletteTmuxRunner("send-keys", "-t", paneID, "-l", launcher+" -s "+sessionID); err != nil {
		return err
	}
	return paletteTmuxRunner("send-keys", "-t", paneID, "Enter")
}

func nextForkWindowName(sourceName, sessionID string) string {
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" {
		sourceName = "op"
	}
	base := sourceName
	if idx := strings.LastIndex(sourceName, "-"); idx > 0 {
		suffix := sourceName[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil {
			base = sourceName[:idx]
		}
	}

	maxN := 0
	if session := strings.TrimSpace(sessionID); session != "" {
		out, err := paletteTmuxOutput("list-windows", "-t", session, "-F", "#{window_name}")
		if err == nil {
			prefix := base + "-"
			for _, name := range strings.Split(out, "\n") {
				name = strings.TrimSpace(name)
				if strings.HasPrefix(name, prefix) {
					if n, err := strconv.Atoi(strings.TrimPrefix(name, prefix)); err == nil && n > maxN {
						maxN = n
					}
				}
			}
		}
	}
	return fmt.Sprintf("%s-%d", base, maxN+1)
}

func waitForShellPane(paneID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := paletteTmuxOutput("display-message", "-p", "-t", paneID, "#{pane_current_command}")
		if err == nil {
			switch strings.TrimSpace(out) {
			case "zsh", "bash", "sh", "fish":
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func statusRightModuleLabel(module string) string {
	switch module {
	case statusRightModuleCPU:
		return "CPU"
	case statusRightModuleNetwork:
		return "Network"
	case statusRightModuleMemory:
		return "Tmux Pane Memory"
	case statusRightModuleWindowMemory:
		return "Tmux Window Memory"
	case statusRightModuleSessionMemory:
		return "Tmux Session Memory"
	case statusRightModuleTotalMemory:
		return "Tmux Total Memory"
	case statusRightModuleScratch:
		return "Scratch"
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
		return "tmux pane memory"
	case statusRightModuleWindowMemory:
		return "tmux window memory"
	case statusRightModuleSessionMemory:
		return "tmux session memory"
	case statusRightModuleTotalMemory:
		return "total tmux memory"
	case statusRightModuleScratch:
		return "hidden scratch terminal bell"
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
	model := &paletteModel{runtime: runtime, state: state, actions: runtime.buildActions(), openedAt: time.Now(), singlePanelMode: runtime.startMode != paletteModeList}
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
	if state.Mode == paletteModeGoals {
		_, _ = model.openGoalsPanel()
	}
	if state.Mode == paletteModeLLMQuotas {
		model.quotas = newLLMQuotaPanelModel()
	}
	if state.Mode == paletteModeMemory {
		model.openMemoryPanel()
	}
	if state.Mode == paletteModeBoard {
		model.openBoardPanel()
	}
	return model
}

func (m *paletteModel) Init() tea.Cmd {
	if m.memory != nil {
		return m.memory.Init()
	}
	if m.goals != nil {
		return goalPanelTickCmd()
	}
	if m.tracker != nil {
		return trackerPanelTickCmd()
	}
	if m.quotas != nil {
		return m.quotas.activate()
	}
	if m.activity != nil {
		return activityTickCmd()
	}
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

func (m *paletteModel) openLLMQuotaPanel() tea.Cmd {
	m.noteSecondaryPageOpen()
	if m.quotas == nil {
		m.quotas = newLLMQuotaPanelModel()
	} else {
		m.quotas.requestBack = false
	}
	m.quotas.width = m.width
	m.quotas.height = m.height
	m.quotas.showAltHints = false
	m.state.Mode = paletteModeLLMQuotas
	m.state.Message = ""
	m.state.ShowAltHints = false
	return m.quotas.activate()
}

func (m *paletteModel) openGoalsPanel() (tea.Cmd, error) {
	m.noteSecondaryPageOpen()
	if m.goals == nil {
		m.goals = newGoalPanelModel(m.runtime)
	} else {
		m.goals.runtime = m.runtime
		m.goals.requestBack = false
		m.goals.requestClose = false
	}
	m.goals.width = m.width
	m.goals.height = m.height
	m.goals.showAltHints = false
	m.state.Mode = paletteModeGoals
	m.state.Message = ""
	m.state.ShowAltHints = false
	return m.goals.activate(), nil
}

func agentPanelActions(r *paletteRuntime) []paletteAction {
	actions := []paletteAction{
		{Section: "Browser", Title: "Hot Reload", Subtitle: "Analyze + flutter reload", Keywords: []string{"reload", "hot", "flutter", "analyze"}, Kind: paletteActionBrowserReload},
		{Section: "Browser", Title: "Copy Logs", Subtitle: "Copy browser console to clipboard", Keywords: []string{"copy", "logs", "console", "clipboard"}, Kind: paletteActionBrowserCopyLogs},
		{Section: "Browser", Title: "Paste Logs", Subtitle: "Paste browser console into pane", Keywords: []string{"paste", "logs", "console", "pane"}, Kind: paletteActionBrowserLogs},
		{Section: "Browser", Title: "Clear Logs", Subtitle: "Clear browser console buffer", Keywords: []string{"clear", "logs", "console", "buffer"}, Kind: paletteActionBrowserClearLogs},
	}
	if r != nil && r.record != nil {
		actions = append(actions,
			paletteAction{
				Section:  "Server",
				Title:    "Restart agent server",
				Subtitle: "Re-run ensure-server.sh in the Run pane",
				Keywords: []string{"restart", "server", "flutter", "ensure-server", "run", "pane"},
				Kind:     paletteActionRestartAgentServer,
			},
			paletteAction{
				Section:  "Server",
				Title:    "Switch device",
				Subtitle: "Change the Flutter launch device and restart the server",
				Keywords: []string{"switch", "device", "flutter", "change", "launch"},
				Kind:     paletteActionSwitchAgentDevice,
			},
		)
	}
	return actions
}

func agentPanelHotKeys() map[string]int {
	return map[string]int{
		"alt+r": 0,
		"alt+y": 1,
		"alt+p": 2,
		"alt+c": 3,
		"alt+t": 4,
		"alt+d": 5,
	}
}

func opencodeForkPanelActions() []paletteAction {
	return []paletteAction{
		{Section: "Opencode", Title: "Fork in horizontal pane", Subtitle: "Split right and run op -s session_id", Keywords: []string{"opencode", "fork", "horizontal", "right", "split"}, Kind: paletteActionForkOpencodeHorizontal},
		{Section: "Opencode", Title: "Fork in vertical pane", Subtitle: "Split below and run op -s session_id", Keywords: []string{"opencode", "fork", "vertical", "below", "split"}, Kind: paletteActionForkOpencodeVertical},
		{Section: "Opencode", Title: "Fork in new window", Subtitle: "Create a tmux window and run op -s session_id", Keywords: []string{"opencode", "fork", "window", "new"}, Kind: paletteActionForkOpencodeWindow},
	}
}

func opencodeForkPanelHotKeys() map[string]int {
	return map[string]int{
		"alt+h": 0,
		"alt+v": 1,
		"alt+w": 2,
	}
}

func (m *paletteModel) openAgentPanel() {
	m.noteSecondaryPageOpen()
	m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
	m.state.Mode = paletteModeAgent
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openDeviceSwitchPanel() {
	m.noteSecondaryPageOpen()
	current := ""
	if m.runtime.record != nil {
		current = m.runtime.record.Device
	}
	m.deviceSwitch = newDeviceSwitchPanelModel(current)
	m.state.Mode = paletteModeSwitchDevice
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openRestoreAgentPanel(repoRoot string) {
	m.noteSecondaryPageOpen()
	m.restoreAgent = newRestoreAgentPanelModel(repoRoot)
	m.state.Mode = paletteModeRestoreAgent
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openOpencodeForkPanel() {
	m.noteSecondaryPageOpen()
	m.opencodeForkList = newActionListPanel(opencodeForkPanelActions(), opencodeForkPanelHotKeys())
	m.state.Mode = paletteModeOpencodeFork
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openMemoryPanel() {
	m.noteSecondaryPageOpen()
	if m.memory == nil {
		m.memory = newMemoryPanelModel(m.currentMemoryWindowID())
	} else {
		m.memory.windowID = m.currentMemoryWindowID()
		m.memory.windowName = ""
		m.memory.reload()
		m.memory.contentOffset = 0
		m.memory.requestBack = false
	}
	if m.runtime != nil {
		m.memory.paneID = m.runtime.paneID
		m.memory.reload()
	}
	m.state.Mode = paletteModeMemory
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) openBoardPanel() {
	m.noteSecondaryPageOpen()
	if m.board == nil {
		m.board = newBoardPanelModel()
	} else {
		m.board.reload()
		m.board.cursor = 0
		m.board.offset = 0
		m.board.detailOffset = 0
		m.board.requestBack = false
	}
	m.state.Mode = paletteModeBoard
	m.state.Message = ""
	m.state.ShowAltHints = false
}

func (m *paletteModel) updateBoardPanel(key string) (tea.Model, tea.Cmd) {
	if m.board == nil {
		m.board = newBoardPanelModel()
	}
	m.board.handleKey(key)
	if m.board.requestBack {
		m.board.requestBack = false
		m.state.Mode = paletteModeList
		m.state.Message = m.board.currentStatus()
		return m, nil
	}
	return m, nil
}

func (m *paletteModel) currentMemoryWindowID() string {
	if m.runtime != nil {
		return m.runtime.windowID
	}
	return ""
}

func (m *paletteModel) updateMemoryPanel(key string) (tea.Model, tea.Cmd) {
	if m.memory == nil {
		m.memory = newMemoryPanelModel(m.currentMemoryWindowID())
	}
	m.memory.handleKey(key)
	if m.memory.requestBack {
		m.memory.requestBack = false
		m.state.Mode = paletteModeList
		m.state.Message = m.memory.currentStatus()
		return m, nil
	}
	return m, m.memory.requestUsage()
}

func (m *paletteModel) updateAgentPanel(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "ctrl+c" || key == "alt+n" {
		m.state.Mode = paletteModeList
		m.state.Message = ""
		return m, nil
	}
	if m.agentList == nil {
		m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
	}
	action, consumed := m.agentList.handleKey(key)
	if consumed && action != nil {
		return m.selectAction(*action)
	}
	return m, nil
}

func (m *paletteModel) updateOpencodeForkPanel(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "ctrl+c" || key == "alt+n" {
		m.state.Mode = paletteModeList
		m.state.Message = ""
		return m, nil
	}
	if m.opencodeForkList == nil {
		m.opencodeForkList = newActionListPanel(opencodeForkPanelActions(), opencodeForkPanelHotKeys())
	}
	action, consumed := m.opencodeForkList.handleKey(key)
	if consumed && action != nil {
		return m.selectAction(*action)
	}
	return m, nil
}

func (m *paletteModel) updateDeviceSwitchPanel(key string) (tea.Model, tea.Cmd) {
	if m.deviceSwitch == nil {
		m.deviceSwitch = newDeviceSwitchPanelModel("")
	}
	m.deviceSwitch.handleKey(key)
	if m.deviceSwitch.requestBack {
		m.deviceSwitch.requestBack = false
		m.state.Mode = paletteModeAgent
		m.state.Message = ""
		m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
		return m, nil
	}
	if m.deviceSwitch.requestDone {
		chosen := m.deviceSwitch.chosen
		m.deviceSwitch.requestDone = false
		if chosen == "" {
			return m, nil
		}
		if err := m.runtime.applyDeviceSwitch(chosen); err != nil {
			m.state.Mode = paletteModeAgent
			m.state.Message = err.Error()
			m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
			return m, nil
		}
		m.state.Mode = paletteModeAgent
		m.state.Message = fmt.Sprintf("Switched to %s, restarting server", chosen)
		m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
		return m, nil
	}
	return m, nil
}

func (m *paletteModel) updateRestoreAgentPanel(key string) (tea.Model, tea.Cmd) {
	if m.restoreAgent == nil {
		m.restoreAgent = newRestoreAgentPanelModel(m.runtime.mainRepoRoot)
	}
	m.restoreAgent.handleKey(key)
	if m.restoreAgent.requestBack {
		m.restoreAgent.requestBack = false
		m.state.Mode = paletteModeList
		m.state.Message = ""
		return m, nil
	}
	if m.restoreAgent.requestDone {
		chosen := m.restoreAgent.chosen
		repoRoot := m.restoreAgent.repoRoot
		m.restoreAgent.requestDone = false
		if chosen == "" {
			return m, nil
		}
		if err := launchPaletteRestore(chosen, repoRoot); err != nil {
			m.state.Mode = paletteModeList
			m.state.Message = err.Error()
			return m, nil
		}
		m.result = paletteResult{Kind: paletteResultClose, State: m.state}
		return m, tea.Quit
	}
	return m, nil
}

func (m *paletteModel) agentPanelCopyLogs() (tea.Model, tea.Cmd) {
	workspace := m.runtime.workspaceForBrowser()
	if workspace == "" {
		m.state.Message = "No browser-enabled agent"
		return m, nil
	}
	exe, err := os.Executable()
	if err != nil {
		m.state.Message = err.Error()
		return m, nil
	}
	cmd := exec.Command(exe, "browser", "logs", "--workspace", workspace, "--tail", "50", "--keep")
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.state.Message = firstNonEmpty(strings.TrimSpace(string(output)), err.Error())
		return m, nil
	}
	text := strings.TrimSpace(string(output))
	clip := exec.Command("pbcopy")
	clip.Stdin = strings.NewReader(text)
	if err := clip.Run(); err != nil {
		m.state.Message = err.Error()
		return m, nil
	}
	m.result = paletteResult{Kind: paletteResultClose, State: m.state}
	return m, tea.Quit
}

func (m *paletteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case memoryUsageMsg:
		if m.memory != nil {
			m.memory.acceptUsage(msg)
		}
		return m, nil
	case memoryPanelTickMsg:
		if m.state.Mode == paletteModeMemory && m.memory != nil && msg.panel == m.memory && msg.generation == m.memory.pollGeneration {
			m.memory.reload()
			return m, tea.Batch(m.memory.requestUsage(), m.memory.tick())
		}
		return m, nil
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
		if m.quotas != nil {
			m.quotas.width = msg.Width
			m.quotas.height = msg.Height
		}
		if m.goals != nil {
			m.goals.width = msg.Width
			m.goals.height = msg.Height
		}
		if m.status != nil {
			m.status.width = msg.Width
			m.status.height = msg.Height
		}
	case tea.KeyMsg:
		if msg.Paste {
			if m.state.Mode == paletteModePrompt {
				pasted := strings.ReplaceAll(string(msg.Runes), "\n", " ")
				runes := []rune(pasted)
				m.state.PromptText = append(m.state.PromptText[:m.state.PromptCursor], append(runes, m.state.PromptText[m.state.PromptCursor:]...)...)
				m.state.PromptCursor += len(runes)
			}
			return m, nil
		}
		if m.state.Mode != paletteModeActivity && m.state.Mode != paletteModeTodos && m.state.Mode != paletteModeDevices && m.state.Mode != paletteModeStatusRight && m.state.Mode != paletteModeTracker && m.state.Mode != paletteModeGoals && m.state.Mode != paletteModeLLMQuotas && m.state.Mode != paletteModeAgent && m.state.Mode != paletteModeSwitchDevice && m.state.Mode != paletteModeOpencodeFork && m.state.Mode != paletteModeRestoreAgent && m.state.Mode != paletteModeMemory && m.state.Mode != paletteModeBoard {
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
			case paletteModeLLMQuotas:
				return m.closePalette()
			case paletteModeGoals:
				if m.goals != nil && m.goals.mode == goalModeList {
					return m.closePalette()
				}
			case paletteModeSnippets:
				return m.closePalette()
			case paletteModeOpencodeFork:
				return m.closePalette()
			case paletteModeSwitchDevice:
				return m.closePalette()
			case paletteModeRestoreAgent:
				return m.closePalette()
			case paletteModeMemory:
				return m.closePalette()
			case paletteModeBoard:
				return m.closePalette()
			}
		}
		if m.state.Mode == paletteModeAgent {
			return m.updateAgentPanel(key)
		}
		if m.state.Mode == paletteModeSwitchDevice {
			return m.updateDeviceSwitchPanel(key)
		}
		if m.state.Mode == paletteModeRestoreAgent {
			return m.updateRestoreAgentPanel(key)
		}
		if m.state.Mode == paletteModeOpencodeFork {
			return m.updateOpencodeForkPanel(key)
		}
		if m.state.Mode == paletteModeMemory {
			return m.updateMemoryPanel(key)
		}
		if m.state.Mode == paletteModeBoard {
			return m.updateBoardPanel(key)
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
				if m.singlePanelMode {
					m.result = paletteResult{Kind: paletteResultClose, State: m.state}
					return m, tea.Quit
				}
				m.tracker.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.tracker.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeLLMQuotas {
			if m.quotas == nil {
				return m, m.openLLMQuotaPanel()
			}
			model, cmd := m.quotas.Update(msg)
			if updated, ok := model.(*llmQuotaPanelModel); ok {
				m.quotas = updated
			}
			if m.quotas.requestBack {
				if m.singlePanelMode {
					return m.closePalette()
				}
				m.quotas.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.quotas.currentStatus()
				return m, nil
			}
			return m, cmd
		}
		if m.state.Mode == paletteModeGoals {
			if m.goals == nil {
				cmd, err := m.openGoalsPanel()
				if err != nil {
					m.state.Mode = paletteModeList
					m.state.Message = err.Error()
					return m, nil
				}
				return m, cmd
			}
			model, cmd := m.goals.Update(msg)
			if updated, ok := model.(*goalPanelModel); ok {
				m.goals = updated
			}
			if m.goals.requestClose {
				m.result = paletteResult{Kind: paletteResultClose, State: m.state}
				return m, tea.Quit
			}
			if m.goals.requestBack {
				if m.singlePanelMode {
					m.result = paletteResult{Kind: paletteResultClose, State: m.state}
					return m, tea.Quit
				}
				m.goals.requestBack = false
				m.state.Mode = paletteModeList
				m.state.Message = m.goals.currentStatus()
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
	if m.state.Mode == paletteModeLLMQuotas && m.quotas != nil {
		model, cmd := m.quotas.Update(msg)
		if updated, ok := model.(*llmQuotaPanelModel); ok {
			m.quotas = updated
		}
		if m.quotas.requestBack {
			m.quotas.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.quotas.currentStatus()
			return m, nil
		}
		return m, cmd
	}
	if m.state.Mode == paletteModeGoals && m.goals != nil {
		model, cmd := m.goals.Update(msg)
		if updated, ok := model.(*goalPanelModel); ok {
			m.goals = updated
		}
		if m.goals.requestClose {
			m.result = paletteResult{Kind: paletteResultClose, State: m.state}
			return m, tea.Quit
		}
		if m.goals.requestBack {
			m.goals.requestBack = false
			m.state.Mode = paletteModeList
			m.state.Message = m.goals.currentStatus()
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
		m.openAgentPanel()
		return m, nil
	}
	if key == "alt+f" {
		if !m.runtime.canForkCurrentOpenCode() {
			m.state.Message = "No opencode session in current pane"
			return m, nil
		}
		m.openOpencodeForkPanel()
		return m, nil
	}
	if key == "alt+w" {
		cmd, err := m.openActivityPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	}
	if key == "alt+q" {
		return m, m.openLLMQuotaPanel()
	}
	if key == "alt+p" {
		m.openSnippetsPanel()
		return m, nil
	}
	if key == "alt+r" {
		cmd, err := m.openGoalsPanel()
		if err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
		return m, cmd
	}
	if key == "alt+d" {
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
	if key == "alt+m" {
		m.openMemoryPanel()
		return m, m.memory.Init()
	}
	if key == "alt+b" {
		m.openBoardPanel()
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
	case paletteActionOpenLLMQuotas:
		return m, m.openLLMQuotaPanel()
	case paletteActionOpenGoals:
		cmd, err := m.openGoalsPanel()
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
	case paletteActionOpenOpencodeFork:
		m.openOpencodeForkPanel()
		return m, nil
	case paletteActionSwitchAgentDevice:
		m.openDeviceSwitchPanel()
		return m, nil
	case paletteActionRestoreAgent:
		m.openRestoreAgentPanel(action.RepoRoot)
		return m, nil
	case paletteActionOpenMemory:
		m.openMemoryPanel()
		return m, m.memory.Init()
	case paletteActionOpenBoard:
		m.openBoardPanel()
		return m, nil
	case paletteActionBrowserLogs:
		return m.runBrowserLogsPaste()
	case paletteActionBrowserCopyLogs:
		return m.agentPanelCopyLogs()
	default:
		m.state.Mode = paletteModeList
		m.result = paletteResult{Kind: paletteResultRunAction, Action: action, State: m.state}
		return m, tea.Quit
	}
}

func (m *paletteModel) runBrowserLogsPaste() (tea.Model, tea.Cmd) {
	workspace := m.runtime.workspaceForBrowser()
	if workspace == "" {
		m.state.Message = "No browser-enabled agent in this window"
		return m, nil
	}
	exe, err := os.Executable()
	if err != nil {
		m.state.Message = err.Error()
		return m, nil
	}
	cmd := exec.Command(exe, "browser", "logs", "--workspace", workspace, "--tail", "50")
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.state.Message = firstNonEmpty(strings.TrimSpace(string(output)), err.Error())
		return m, nil
	}
	text := strings.TrimSpace(string(output))
	if text != "" {
		if err := pasteToTmuxPane(text); err != nil {
			m.state.Message = err.Error()
			return m, nil
		}
	} else {
		m.state.Message = "No console output captured"
		return m, nil
	}
	m.result = paletteResult{Kind: paletteResultClose, State: m.state}
	return m, tea.Quit
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
	m.state.PromptPull = true
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
			case palettePromptFieldWorktree:
				m.state.PromptField = palettePromptFieldPull
			default:
				m.state.PromptField = palettePromptFieldName
			}
			return m, nil
		case "shift+tab":
			switch m.state.PromptField {
			case palettePromptFieldPull:
				m.state.PromptField = palettePromptFieldWorktree
			case palettePromptFieldWorktree:
				m.state.PromptField = palettePromptFieldDevice
			case palettePromptFieldDevice:
				m.state.PromptField = palettePromptFieldName
			default:
				m.state.PromptField = palettePromptFieldPull
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
		if m.state.PromptField == palettePromptFieldPull {
			switch key {
			case "ctrl+n", "left", "n":
				m.state.PromptPull = false
				return m, nil
			case "right", "i":
				m.state.PromptPull = true
				return m, nil
			case " ", "space":
				m.state.PromptPull = !m.state.PromptPull
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
		m.result = paletteResult{Kind: paletteResultRunAction, Action: action, Input: text, Device: device, KeepWorktree: m.state.PromptKeepWorktree, Pull: m.state.PromptPull, State: m.state}
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
		haystack := strings.ToLower(s.Name)
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
	if m.state.Mode == paletteModeLLMQuotas {
		if m.quotas != nil {
			m.quotas.width = width
			m.quotas.height = height
			return m.quotas.render(styles, width, height)
		}
		return styles.muted.Render("LLM quotas unavailable")
	}
	if m.state.Mode == paletteModeGoals {
		if m.goals != nil {
			m.goals.width = width
			m.goals.height = height
			return m.goals.View()
		}
		return styles.muted.Render("Goals unavailable")
	}
	if m.state.Mode == paletteModeAgent {
		return m.renderAgentPanel(styles, width, height)
	}
	if m.state.Mode == paletteModeSwitchDevice {
		if m.deviceSwitch == nil {
			m.deviceSwitch = newDeviceSwitchPanelModel("")
		}
		m.deviceSwitch.width = width
		m.deviceSwitch.height = height
		return m.deviceSwitch.View()
	}
	if m.state.Mode == paletteModeOpencodeFork {
		return m.renderOpencodeForkPanel(styles, width, height)
	}
	if m.state.Mode == paletteModeRestoreAgent {
		if m.restoreAgent == nil {
			m.restoreAgent = newRestoreAgentPanelModel(m.runtime.mainRepoRoot)
		}
		m.restoreAgent.width = width
		m.restoreAgent.height = height
		return m.restoreAgent.View()
	}
	if m.state.Mode == paletteModeMemory {
		if m.memory == nil {
			m.memory = newMemoryPanelModel(m.currentMemoryWindowID())
		}
		m.memory.width = width
		m.memory.height = height
		return m.memory.View()
	}
	if m.state.Mode == paletteModeBoard {
		if m.board == nil {
			m.board = newBoardPanelModel()
		}
		m.board.width = width
		m.board.height = height
		return m.board.View()
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
	innerWidth := width - 2
	filterLine := styles.searchBox.Width(innerWidth).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.state.Filter, m.state.FilterCursor, styles)),
		),
	)
	contentHeight := maxInt(8, height-7)
	listWidth := maxInt(34, innerWidth*48/100)
	sidebarWidth := maxInt(28, innerWidth-listWidth-3)
	list := m.renderActions(styles, actions, listWidth, contentHeight)
	sidebar := m.renderSidebar(styles, sidebarWidth, contentHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, strings.Repeat(" ", 3), sidebar)
	footer := renderPaletteFooter(styles, width, m.state.Message, m.state.ShowAltHints)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *paletteModel) renderActions(styles paletteStyles, actions []paletteAction, width, height int) string {
	return renderActionItems(styles, actions, &m.state.Selected, &m.state.ActionOffset, width, height)
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
		pullLabel := styles.modalHint.Render("PULL")
		if m.state.PromptField == palettePromptFieldName {
			nameLabel = styles.selectedLabel.Render("NAME")
		} else if m.state.PromptField == palettePromptFieldDevice {
			deviceLabel = styles.selectedLabel.Render("DEVICE")
		} else if m.state.PromptField == palettePromptFieldWorktree {
			worktreeLabel = styles.selectedLabel.Render("WORKTREE")
		} else {
			pullLabel = styles.selectedLabel.Render("PULL")
		}
		deviceChips := make([]string, 0, len(devices))
		for idx, deviceID := range devices {
			deviceChips = append(deviceChips, renderPaletteDeviceChip(styles, deviceID, idx == clampInt(m.state.PromptDeviceIndex, 0, len(devices)-1)))
		}
		worktreeChips := []string{
			renderPaletteDeviceChip(styles, "CLEAR", !m.state.PromptKeepWorktree),
			renderPaletteDeviceChip(styles, "KEEP", m.state.PromptKeepWorktree),
		}
		pullChips := []string{
			renderPaletteDeviceChip(styles, "SKIP", !m.state.PromptPull),
			renderPaletteDeviceChip(styles, "PULL", m.state.PromptPull),
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
			pullLabel,
			styles.modalBody.Render(strings.Join(pullChips, " ")),
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

func (m *paletteModel) renderAgentPanel(styles paletteStyles, width, height int) string {
	if m.agentList == nil {
		m.agentList = newActionListPanel(agentPanelActions(m.runtime), agentPanelHotKeys())
	}
	m.state.Filter = m.agentList.filter
	m.state.FilterCursor = m.agentList.cursor
	innerWidth := width - 2
	title := "Agent Actions"
	header := styles.title.Render(title)
	metaParts := []string{}
	if m.runtime.currentSessionName != "" {
		metaParts = append(metaParts, m.runtime.currentSessionName)
	}
	if m.runtime.currentWindowName != "" {
		metaParts = append(metaParts, m.runtime.currentWindowName)
	}
	if m.runtime.record != nil && m.runtime.record.ID != "" {
		metaParts = append(metaParts, m.runtime.record.ID)
	}
	if m.runtime.record != nil && m.runtime.record.BrowserEnabled {
		metaParts = append(metaParts, "browser")
	}
	if len(metaParts) > 0 {
		header = lipgloss.JoinVertical(lipgloss.Left, header, styles.meta.Render(strings.Join(metaParts, "  ·  ")))
	}
	filterLine := styles.searchBox.Width(innerWidth).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.state.Filter, m.state.FilterCursor, styles)),
		),
	)
	contentHeight := maxInt(8, height-7)
	list := m.agentList.renderList(styles, innerWidth, contentHeight)
	footer := renderPaletteModeFooter(styles, width, m.state.Message, m.state.ShowAltHints,
		[][][2]string{
			{{"Ctrl-U/E", "move"}, {"Ctrl-N/I", "filter"}, {"Enter", "run"}, {"Alt-R", "reload"}, {"Alt-Y", "copy"}, {"Alt-P", "paste"}, {"Alt-C", "clear"}, {"Esc", "back"}},
		},
		[][][2]string{
			{{"Alt-U/E", "move"}, {"Alt-I", "run"}, {"Alt-R", "reload"}, {"Alt-Y", "copy"}, {"Alt-P", "paste"}, {"Alt-C", "clear"}, {"Esc", "back"}},
		},
	)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", list, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *paletteModel) renderOpencodeForkPanel(styles paletteStyles, width, height int) string {
	if m.opencodeForkList == nil {
		m.opencodeForkList = newActionListPanel(opencodeForkPanelActions(), opencodeForkPanelHotKeys())
	}
	m.state.Filter = m.opencodeForkList.filter
	m.state.FilterCursor = m.opencodeForkList.cursor
	innerWidth := width - 2
	header := styles.title.Render("Fork Opencode")
	metaParts := []string{}
	if m.runtime.currentSessionName != "" {
		metaParts = append(metaParts, m.runtime.currentSessionName)
	}
	if m.runtime.currentWindowName != "" {
		metaParts = append(metaParts, m.runtime.currentWindowName)
	}
	if sessionID := m.runtime.currentOpenCodeSessionID(); sessionID != "" {
		metaParts = append(metaParts, sessionID)
	}
	if len(metaParts) > 0 {
		header = lipgloss.JoinVertical(lipgloss.Left, header, styles.meta.Render(strings.Join(metaParts, "  ·  ")))
	}
	filterLine := styles.searchBox.Width(innerWidth).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.state.Filter, m.state.FilterCursor, styles)),
		),
	)
	contentHeight := maxInt(8, height-7)
	list := m.opencodeForkList.renderList(styles, innerWidth, contentHeight)
	footer := renderPaletteModeFooter(styles, width, m.state.Message, m.state.ShowAltHints,
		[][][2]string{
			{{"Ctrl-U/E", "move"}, {"Ctrl-N/I", "filter"}, {"Enter", "fork"}, {"Esc", "back"}},
		},
		[][][2]string{
			{{"Alt-H", "horizontal"}, {"Alt-V", "vertical"}, {"Alt-W", "window"}, {"Alt-I", "fork"}, {"Esc", "back"}},
		},
	)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", list, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
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
	return filterActionsByQuery(m.actions, m.state.Filter)
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
	runes, ok := paletteRunesFromKey(key)
	if !ok {
		return false
	}
	*text = append((*text)[:*cursor], append(runes, (*text)[*cursor:]...)...)
	*cursor += len(runes)
	return true
}

func readClipboardPaste() string {
	cmd := exec.Command("pbpaste")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func paletteRunesFromKey(key string) ([]rune, bool) {
	if key == "space" {
		return []rune{' '}, true
	}
	runes := []rune(key)
	if len(runes) >= 1 {
		return runes, true
	}
	return nil, false
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
			{{"Alt-C", "create"}, {"Alt-F", "fork"}, {"Alt-R", "goals"}, {"Alt-D", "tracker"}, {"Alt-Q", "quotas"}, {"Alt-A", "agent"}, {"Alt-W", "activity"}, {"Alt-P", "snippets"}, {"Alt-T", "todos"}, {"Alt-M", "memory"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			{{"Alt-C", "create"}, {"Alt-R", "goals"}, {"Alt-D", "tracker"}, {"Alt-Q", "quotas"}, {"Alt-A", "agent"}, {"Alt-W", "activity"}, {"Alt-T", "todos"}, {"Alt-M", "memory"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			{{"Alt-R", "goals"}, {"Alt-D", "tracker"}, {"Alt-Q", "quotas"}, {"Alt-S", "close"}},
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
