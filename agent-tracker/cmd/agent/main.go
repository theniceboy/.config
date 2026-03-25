package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type registry struct {
	Agents         map[string]*agentRecord `json:"agents"`
	FocusedAgentID string                  `json:"focused_agent_id,omitempty"`
}

type agentRecord struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	RepoRoot        string       `json:"repo_root"`
	WorkspaceRoot   string       `json:"workspace_root"`
	RepoCopyPath    string       `json:"repo_copy_path"`
	Branch          string       `json:"branch"`
	SourceBranch    string       `json:"source_branch,omitempty"`
	KeepWorktree    bool         `json:"keep_worktree,omitempty"`
	Runtime         string       `json:"runtime,omitempty"`
	Device          string       `json:"device,omitempty"`
	FeatureConfig   string       `json:"feature_config,omitempty"`
	RunLogPath      string       `json:"run_log_path,omitempty"`
	Port            int          `json:"port,omitempty"`
	URL             string       `json:"url,omitempty"`
	BrowserEnabled  bool         `json:"browser_enabled,omitempty"`
	TmuxSessionName string       `json:"tmux_session_name,omitempty"`
	TmuxSessionID   string       `json:"tmux_session_id,omitempty"`
	TmuxWindowID    string       `json:"tmux_window_id,omitempty"`
	Panes           agentPanes   `json:"panes"`
	Dashboard       dashboardDoc `json:"dashboard"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	LastFocusedAt   *time.Time   `json:"last_focused_at,omitempty"`
	LaunchWindowID  string       `json:"-"`
}

type agentPanes struct {
	AI        string `json:"ai,omitempty"`
	Git       string `json:"git,omitempty"`
	Run       string `json:"run,omitempty"`
	Dashboard string `json:"dashboard,omitempty"`
}

type agentStartOptions struct {
	SourceBranch string
	KeepWorktree bool
}

type dashboardDoc struct {
	Todos       []todoItem `json:"todos"`
	Notes       string     `json:"notes"`
	CurrentTask string     `json:"current_task"`
}

type todoItem struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type appConfig struct {
	Keys    keyConfig `json:"keys"`
	Devices []string  `json:"devices,omitempty"`
}

type keyConfig struct {
	MoveLeft   string `json:"move_left"`
	MoveRight  string `json:"move_right"`
	MoveUp     string `json:"move_up"`
	MoveDown   string `json:"move_down"`
	Edit       string `json:"edit"`
	Cancel     string `json:"cancel"`
	AddTodo    string `json:"add_todo"`
	ToggleTodo string `json:"toggle_todo"`
	Destroy    string `json:"destroy"`
	Confirm    string `json:"confirm"`
	Back       string `json:"back"`
	DeleteTodo string `json:"delete_todo"`
	Help       string `json:"help"`
	FocusAI    string `json:"focus_ai"`
	FocusGit   string `json:"focus_git"`
	FocusDash  string `json:"focus_dashboard"`
	FocusRun   string `json:"focus_run"`
}

type repoConfig struct {
	BaseBranch    string   `yaml:"base_branch,omitempty"`
	CopyIgnore    []string `yaml:"copy_ignore,omitempty"`
	AgentKeyPaths []string `yaml:"agent_key_paths,omitempty"`
}

type featureConfig struct {
	Feature       string `json:"feature"`
	Port          int    `json:"port,omitempty"`
	URL           string `json:"url,omitempty"`
	Device        string `json:"device"`
	IsFlutter     bool   `json:"is_flutter,omitempty"`
	Ready         bool   `json:"ready,omitempty"`
	ShouldOpenTab bool   `json:"should_open_tab,omitempty"`
	ChromeWindow  int    `json:"chrome_window_index,omitempty"`
	ChromeTab     int    `json:"chrome_tab_index,omitempty"`
}

var featureNamePattern = regexp.MustCompile(`[^a-z0-9._-]+`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent <start|resume|list|destroy|init|config|setup|dashboard|tmux|tracker|browser|feature>")
	}
	switch args[0] {
	case "start":
		return runStart(args[1:])
	case "resume":
		return runResume(args[1:])
	case "list":
		return runList(args[1:]...)
	case "destroy":
		return runDestroy(args[1:])
	case "init":
		return runInit(args[1:])
	case "config":
		return runConfig(args[1:])
	case "setup":
		return runSetup(args[1:])
	case "dashboard":
		return runDashboard(args[1:])
	case "palette":
		return runPalette(args[1:])
	case "tmux":
		return runTmuxCommand(args[1:])
	case "tracker":
		return runTracker(args[1:])
	case "browser":
		return runBrowserCommand(args[1:])
	case "feature":
		return runFeatureCommand(args[1:])
	case "bootstrap":
		return runBootstrap(args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func runStart(args []string) error {
	fs := flag.NewFlagSet("agent start", flag.ContinueOnError)
	var feature string
	var device string
	var noDevice bool
	var keepWorktree bool
	fs.StringVar(&feature, "name", "", "feature name")
	fs.StringVar(&device, "d", "", "flutter device")
	fs.BoolVar(&noDevice, "no-device", false, "leave the run pane idle until a device is chosen")
	fs.BoolVar(&keepWorktree, "keep-worktree", false, "copy the current repo worktree into the new agent")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return fmt.Errorf("%w; run `agent init` in your repo to set up agent config", err)
	}
	isFlutter := fileExists(filepath.Join(repoRoot, "pubspec.yaml"))
	if err := ensureGitExcludeEntries(repoRoot, []string{".agents"}); err != nil {
		return err
	}
	repoCfg, err := loadRepoConfig(repoRoot)
	if err != nil {
		return err
	}
	if feature == "" && fs.NArg() > 0 {
		feature = fs.Arg(0)
	}
	feature = sanitizeFeatureName(feature)
	if feature == "" {
		value, err := promptInput("Feature name: ")
		if err != nil {
			return err
		}
		feature = sanitizeFeatureName(value)
	}
	if feature == "" {
		return fmt.Errorf("feature name is required")
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	if _, exists := reg.Agents[feature]; exists {
		return fmt.Errorf("agent %q already exists", feature)
	}

	workspaceRoot := filepath.Join(repoRoot, ".agents", feature)
	repoCopyPath := filepath.Join(workspaceRoot, "repo")
	featureConfigPath := filepath.Join(workspaceRoot, "agent.json")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "logs"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(repoCopyPath, 0o755); err != nil {
		return err
	}
	port := 0
	url := ""
	runtime := ""
	browserEnabled := false
	device = strings.TrimSpace(device)
	if isFlutter {
		runtime = "flutter"
		if noDevice {
			device = ""
		} else if device == "" {
			device = "web-server"
		}
		browserEnabled = device == "web-server"
		port, err = allocatePort(repoRoot, 9100)
		if err != nil {
			return err
		}
		url = fmt.Sprintf("http://localhost:%d", port)
		if err := saveFeatureConfig(featureConfigPath, featureConfig{
			Feature:       feature,
			Port:          port,
			URL:           url,
			Device:        device,
			IsFlutter:     true,
			Ready:         false,
			ShouldOpenTab: false,
		}); err != nil {
			return err
		}
		if device == "web-server" {
			if err := ensureChromeAppleEventsEnabled(); err != nil {
				return err
			}
		}
	}
	sourceBranch := resolveStartSourceBranch(repoRoot, repoCfg)
	if err := prepareAgentContext(repoRoot, repoCopyPath, repoCfg.AgentKeyPaths, false); err != nil {
		return err
	}
	if isFlutter {
		if err := writeFlutterHelperScripts(workspaceRoot, repoCopyPath, url, device); err != nil {
			return err
		}
	}

	record := &agentRecord{
		ID:             feature,
		Name:           feature,
		RepoRoot:       repoRoot,
		WorkspaceRoot:  workspaceRoot,
		RepoCopyPath:   repoCopyPath,
		Branch:         feature,
		SourceBranch:   sourceBranch,
		KeepWorktree:   keepWorktree,
		Runtime:        runtime,
		Device:         device,
		FeatureConfig:  featureConfigPath,
		RunLogPath:     filepath.Join(workspaceRoot, "logs", "run.log"),
		Port:           port,
		URL:            url,
		BrowserEnabled: browserEnabled,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		LaunchWindowID: strings.TrimSpace(os.Getenv("AGENT_TMUX_TARGET_WINDOW")),
	}
	reg.Agents[record.ID] = record
	if err := saveRegistry(reg); err != nil {
		return err
	}
	bootstrapPID, err := spawnWorkspaceBootstrap(workspaceRoot)
	if err != nil {
		delete(reg.Agents, record.ID)
		_ = saveRegistry(reg)
		_ = os.RemoveAll(workspaceRoot)
		return err
	}

	if err := launchAgentLayout(record); err != nil {
		_ = killProcessGroup(bootstrapPID)
		delete(reg.Agents, record.ID)
		_ = saveRegistry(reg)
		_ = os.RemoveAll(workspaceRoot)
		return err
	}
	_ = primeAgentAIPane(record.Panes.AI)
	return nil
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("agent init", flag.ContinueOnError)
	var force bool
	fs.BoolVar(&force, "force", false, "overwrite an existing .agent.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}
	if err := ensureFlutterWebRepo(repoRoot); err != nil {
		return err
	}
	path := repoConfigPath(repoRoot)
	configExisted := fileExists(path)
	if configExisted && !force {
		fmt.Printf("Keeping existing %s\n", path)
		return nil
	}
	cfg := defaultRepoConfig()
	cfg.BaseBranch = detectDefaultBaseBranch(repoRoot)
	if err := saveRepoConfig(repoRoot, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", path)
	return nil
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent config <show|set-base-branch|add-ignore|remove-ignore>")
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}
	cfg, err := loadRepoConfig(repoRoot)
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n%s", repoConfigPath(repoRoot), string(data))
		return nil
	case "set-base-branch":
		branch := ""
		if len(args) > 1 {
			branch = strings.TrimSpace(args[1])
		}
		if branch == "" {
			branch, err = promptInputWithDefault("Base branch", cfg.BaseBranch)
			if err != nil {
				return err
			}
			branch = strings.TrimSpace(branch)
		}
		if branch == "" {
			return fmt.Errorf("base branch is required")
		}
		cfg.BaseBranch = branch
	case "add-ignore":
		values := normalizeIgnoreValues(args[1:])
		if len(values) == 0 {
			value, err := promptInput("Ignore path to add: ")
			if err != nil {
				return err
			}
			values = normalizeIgnoreValues([]string{value})
		}
		if len(values) == 0 {
			return fmt.Errorf("at least one ignore path is required")
		}
		for _, value := range values {
			if !containsString(cfg.CopyIgnore, value) {
				cfg.CopyIgnore = append(cfg.CopyIgnore, value)
			}
		}
	case "remove-ignore":
		values := normalizeIgnoreValues(args[1:])
		if len(values) == 0 {
			value, err := promptInput("Ignore path to remove: ")
			if err != nil {
				return err
			}
			values = normalizeIgnoreValues([]string{value})
		}
		if len(values) == 0 {
			return fmt.Errorf("at least one ignore path is required")
		}
		filtered := cfg.CopyIgnore[:0]
		for _, existing := range cfg.CopyIgnore {
			if !containsString(values, existing) {
				filtered = append(filtered, existing)
			}
		}
		cfg.CopyIgnore = filtered
	default:
		return fmt.Errorf("unknown config subcommand: %s", args[0])
	}
	if err := saveRepoConfig(repoRoot, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", repoConfigPath(repoRoot))
	return nil
}

func runSetup(args []string) error {
	fs := flag.NewFlagSet("agent setup", flag.ContinueOnError)
	var baseBranch string
	fs.StringVar(&baseBranch, "base-branch", "", "base branch used for copied repos")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	forwarded := []string{"set-base-branch"}
	if strings.TrimSpace(baseBranch) != "" {
		forwarded = append(forwarded, strings.TrimSpace(baseBranch))
	}
	return runConfig(forwarded)
}

func spawnWorkspaceBootstrap(workspaceRoot string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "logs"), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(bootstrapLogPath(workspaceRoot), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(exe, "bootstrap", "--workspace", workspaceRoot)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = logFile.Close()
	return pid, nil
}

func spawnDetachedAgentCommand(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	if !processRunning(pid) {
		return nil
	}
	err = syscall.Kill(-pid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func stopWorkspaceBootstrap(workspaceRoot string) error {
	data, err := os.ReadFile(bootstrapPIDPath(workspaceRoot))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	return killProcessGroup(pid)
}

func ensureWorkspaceBootstrap(record *agentRecord, _ *repoConfig) error {
	if fileExists(bootstrapRepoReadyPath(record.WorkspaceRoot)) {
		return nil
	}
	data, err := os.ReadFile(bootstrapPIDPath(record.WorkspaceRoot))
	if err == nil {
		pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if convErr == nil && processRunning(pid) {
			return nil
		}
	}
	_, err = spawnWorkspaceBootstrap(record.WorkspaceRoot)
	return err
}

func resetBootstrapState(workspaceRoot string) error {
	stateDir := bootstrapStateDirPath(workspaceRoot)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	for _, path := range []string{
		bootstrapGitReadyPath(workspaceRoot),
		bootstrapRepoReadyPath(workspaceRoot),
		bootstrapFailedPath(workspaceRoot),
	} {
		_ = os.Remove(path)
	}
	return nil
}

func markBootstrapReady(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().Format(time.RFC3339Nano)+"\n"), 0o644)
}

func writeBootstrapFailure(workspaceRoot string, err error) {
	if err == nil {
		_ = os.Remove(bootstrapFailedPath(workspaceRoot))
		return
	}
	_ = os.MkdirAll(bootstrapStateDirPath(workspaceRoot), 0o755)
	_ = os.Remove(bootstrapGitReadyPath(workspaceRoot))
	_ = os.Remove(bootstrapRepoReadyPath(workspaceRoot))
	_ = os.WriteFile(bootstrapFailedPath(workspaceRoot), []byte(err.Error()+"\n"), 0o644)
}

func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("agent bootstrap", flag.ContinueOnError)
	var workspaceRoot string
	fs.StringVar(&workspaceRoot, "workspace", "", "workspace root")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	if workspaceRoot == "" {
		return fmt.Errorf("--workspace is required")
	}
	repoRoot := repoRootFromWorkspaceRoot(workspaceRoot)
	if repoRoot == "" {
		return fmt.Errorf("unable to detect repo root for %s", workspaceRoot)
	}
	repoCfg, err := loadRepoConfigOrDefault(repoRoot)
	if err != nil {
		return err
	}
	startOptions := resolveBootstrapStartOptions(repoRoot, repoCfg, loadAgentRecordByWorkspaceRoot(workspaceRoot))
	feature := sanitizeFeatureName(filepath.Base(workspaceRoot))
	featureCfgPath := filepath.Join(workspaceRoot, "agent.json")
	featureCfg, featureErr := loadFeatureConfig(featureCfgPath)
	isFlutter := fileExists(filepath.Join(repoRoot, "pubspec.yaml"))
	port := 0
	if featureErr == nil {
		if sanitized := sanitizeFeatureName(featureCfg.Feature); sanitized != "" {
			feature = sanitized
		}
		isFlutter = featureCfg.IsFlutter || isFlutter
		port = featureCfg.Port
	}
	if feature == "" {
		return fmt.Errorf("feature name is required")
	}
	if err := resetBootstrapState(workspaceRoot); err != nil {
		return err
	}
	if err := os.WriteFile(bootstrapPIDPath(workspaceRoot), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	defer func() { _ = os.Remove(bootstrapPIDPath(workspaceRoot)) }()
	defer writeBootstrapFailure(workspaceRoot, err)

	repoCopyPath := filepath.Join(workspaceRoot, "repo")
	if err = copyGitMetadata(repoRoot, repoCopyPath); err != nil {
		return err
	}
	if err = ensureRepoCopyLocalExcludes(repoCopyPath, isFlutter); err != nil {
		return err
	}
	if _, err = createFeatureBranch(repoCopyPath, feature, startOptions.SourceBranch, repoCfg.AgentKeyPaths); err != nil {
		return err
	}
	if err = applyRepoCopyIgnores(repoRoot, repoCopyPath, repoCfg.CopyIgnore); err != nil {
		return err
	}
	if startOptions.KeepWorktree {
		if err = syncRepoWorktree(repoRoot, repoCopyPath, repoCfg.CopyIgnore); err != nil {
			return err
		}
	}
	if err = markBootstrapReady(bootstrapGitReadyPath(workspaceRoot)); err != nil {
		return err
	}
	if isFlutter {
		if err = removeLegacyRuntimeProject(workspaceRoot); err != nil {
			return err
		}
		if err = configureFlutterWebConfig(repoRoot, repoCopyPath, port); err != nil {
			return err
		}
		if featureErr == nil {
			if err = writeFlutterHelperScripts(workspaceRoot, repoCopyPath, featureCfg.URL, featureCfg.Device); err != nil {
				return err
			}
		}
	}
	if err = markBootstrapReady(bootstrapRepoReadyPath(workspaceRoot)); err != nil {
		return err
	}
	writeBootstrapFailure(workspaceRoot, nil)
	return nil
}

func runResume(args []string) error {
	repoRoot, err := repoRoot()
	if err != nil {
		return fmt.Errorf("agent resume defaults to the current repo; run inside a git repo")
	}
	reg, err := loadRegistry()
	if err != nil {
		reg = &registry{Agents: map[string]*agentRecord{}}
	}
	recordsByID, err := loadWorkspaceAgentRecords(repoRoot, reg)
	if err != nil {
		return err
	}
	if len(recordsByID) == 0 {
		return fmt.Errorf("no agents found in %s", filepath.Join(repoRoot, ".agents"))
	}
	var agentID string
	if len(args) > 0 {
		agentID = sanitizeFeatureName(args[0])
	} else {
		ids := make([]string, 0, len(recordsByID))
		for id := range recordsByID {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		fmt.Println("Select an agent:")
		for idx, id := range ids {
			fmt.Printf(" [%d] %s\n", idx+1, id)
		}
		value, err := promptInput("Choice: ")
		if err != nil {
			return err
		}
		choice, convErr := strconv.Atoi(strings.TrimSpace(value))
		if convErr != nil || choice < 1 || choice > len(ids) {
			return fmt.Errorf("invalid choice")
		}
		agentID = ids[choice-1]
	}
	record := recordsByID[agentID]
	if record == nil {
		return fmt.Errorf("unknown agent: %s", agentID)
	}
	if _, err := os.Stat(record.RepoCopyPath); err != nil {
		return fmt.Errorf("agent repo copy missing: %s", record.RepoCopyPath)
	}
	if windowAlive(record.TmuxSessionID, record.TmuxWindowID) {
		return selectTmuxWindow(record.TmuxWindowID)
	}
	repoCfg, err := loadRepoConfigOrDefault(record.RepoRoot)
	if err != nil {
		return err
	}
	if err := prepareAgentContext(record.RepoRoot, record.RepoCopyPath, repoCfg.AgentKeyPaths, true); err != nil {
		return err
	}
	if err := removeLegacyRuntimeProject(record.WorkspaceRoot); err != nil {
		return err
	}
	if err := ensureWorkspaceBootstrap(record, repoCfg); err != nil {
		return err
	}
	if record.Runtime == "flutter" {
		if strings.TrimSpace(record.Device) == "web-server" {
			if err := ensureChromeAppleEventsEnabled(); err != nil {
				return err
			}
		}
		if err := writeFlutterHelperScripts(record.WorkspaceRoot, record.RepoCopyPath, record.URL, record.Device); err != nil {
			return err
		}
	}
	if err := launchAgentLayout(record); err != nil {
		return err
	}
	return nil
}

func loadWorkspaceAgentRecords(repoRoot string, reg *registry) (map[string]*agentRecord, error) {
	agentsRoot := filepath.Join(repoRoot, ".agents")
	entries, err := os.ReadDir(agentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]*agentRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	recordsByID := make(map[string]*agentRecord)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workspaceRoot := filepath.Join(agentsRoot, entry.Name())
		featurePath := filepath.Join(workspaceRoot, "agent.json")
		repoCopyPath := filepath.Join(workspaceRoot, "repo")
		if !pathExists(featurePath) && !pathExists(repoCopyPath) {
			continue
		}
		record, err := loadWorkspaceAgentRecord(repoRoot, workspaceRoot, reg)
		if err != nil {
			return nil, err
		}
		if record == nil {
			continue
		}
		if existing := recordsByID[record.ID]; existing != nil {
			return nil, fmt.Errorf("duplicate agent %q in %s and %s", record.ID, existing.WorkspaceRoot, workspaceRoot)
		}
		recordsByID[record.ID] = record
	}
	return recordsByID, nil
}

func loadWorkspaceAgentRecord(repoRoot, workspaceRoot string, reg *registry) (*agentRecord, error) {
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	if workspaceRoot == "" {
		return nil, nil
	}
	featurePath := filepath.Join(workspaceRoot, "agent.json")
	repoCopyPath := filepath.Join(workspaceRoot, "repo")
	featureCfg, err := loadFeatureConfig(featurePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load feature config for %s: %w", workspaceRoot, err)
	}
	agentID := sanitizeFeatureName(filepath.Base(workspaceRoot))
	if featureCfg != nil {
		if sanitized := sanitizeFeatureName(featureCfg.Feature); sanitized != "" {
			agentID = sanitized
		}
	}
	if agentID == "" {
		return nil, nil
	}
	var record agentRecord
	if existing := registryRecordForWorkspace(reg, repoRoot, workspaceRoot, featurePath, agentID); existing != nil {
		record = *existing
	}
	record.ID = agentID
	record.Name = agentID
	record.RepoRoot = repoRoot
	record.WorkspaceRoot = workspaceRoot
	record.RepoCopyPath = repoCopyPath
	record.FeatureConfig = featurePath
	if strings.TrimSpace(record.Branch) == "" {
		record.Branch = agentID
	}
	if featureCfg != nil {
		if featureCfg.IsFlutter || (strings.TrimSpace(record.Runtime) == "" && fileExists(filepath.Join(repoRoot, "pubspec.yaml"))) {
			record.Runtime = "flutter"
		}
		record.Device = strings.TrimSpace(featureCfg.Device)
		record.Port = featureCfg.Port
		record.URL = strings.TrimSpace(featureCfg.URL)
		record.BrowserEnabled = record.Device == "web-server"
	} else if strings.TrimSpace(record.Runtime) == "" && fileExists(filepath.Join(repoRoot, "pubspec.yaml")) {
		record.Runtime = "flutter"
	}
	return &record, nil
}

func registryRecordForWorkspace(reg *registry, repoRoot, workspaceRoot, featurePath, agentID string) *agentRecord {
	if reg == nil {
		return nil
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	featurePath = filepath.Clean(strings.TrimSpace(featurePath))
	for _, record := range reg.Agents {
		if record == nil {
			continue
		}
		if filepath.Clean(strings.TrimSpace(record.WorkspaceRoot)) == workspaceRoot {
			return record
		}
		if filepath.Clean(strings.TrimSpace(record.FeatureConfig)) == featurePath {
			return record
		}
	}
	record := reg.Agents[agentID]
	if record == nil {
		return nil
	}
	if filepath.Clean(strings.TrimSpace(record.RepoRoot)) != repoRoot {
		return nil
	}
	return record
}

func runList(args ...string) error {
	fs := flag.NewFlagSet("agent list", flag.ContinueOnError)
	var showAll bool
	fs.BoolVar(&showAll, "all", false, "show agents across all repos")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	repoScope := ""
	if !showAll {
		repoScope, err = repoRoot()
		if err != nil {
			return fmt.Errorf("agent list defaults to the current repo; run inside a git repo or use --all")
		}
	}

	for _, id := range sortedAgentIDs(reg) {
		record := reg.Agents[id]
		if repoScope != "" && filepath.Clean(record.RepoRoot) != filepath.Clean(repoScope) {
			continue
		}
		state := "stopped"
		if windowAlive(record.TmuxSessionID, record.TmuxWindowID) {
			state = "running"
		}
		fmt.Printf("%s\t%s\t%s\n", id, state, record.RepoCopyPath)
	}
	return nil
}

func runDestroy(args []string) error {
	fs := flag.NewFlagSet("agent destroy", flag.ContinueOnError)
	var agentID string
	var confirmText string
	fs.StringVar(&agentID, "id", "", "agent id")
	fs.StringVar(&confirmText, "confirm", "", "required confirmation text for destructive destroy")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if agentID == "" && fs.NArg() > 0 {
		agentID = fs.Arg(0)
	}
	if agentID == "" {
		ctx, err := detectCurrentAgentFromTmux("")
		if err != nil {
			return err
		}
		agentID = ctx.ID
	}
	target, err := loadDestroyTarget(agentID)
	if err != nil {
		return err
	}
	if target.RequiresExplicitConfirm && strings.TrimSpace(confirmText) != "destroy" {
		return fmt.Errorf("agent has uncommitted changes; rerun with --confirm destroy")
	}
	reg := target.Reg
	record := target.Record
	windowID := target.WindowID
	destroyingCurrentWindow := target.DestroyingCurrentWindow
	if record.URL != "" {
		_ = closeChromeTab(record.URL)
	}
	delete(reg.Agents, agentID)
	if reg.FocusedAgentID == agentID {
		reg.FocusedAgentID = ""
	}
	if err := saveRegistry(reg); err != nil {
		return err
	}
	_ = stopWorkspaceBootstrap(record.WorkspaceRoot)
	if err := os.RemoveAll(record.WorkspaceRoot); err != nil {
		return err
	}
	if windowAlive(record.TmuxSessionID, windowID) {
		if destroyingCurrentWindow {
			return runTmux("run-shell", "-b", fmt.Sprintf("sleep 0.2; tmux kill-window -t %s", shellQuote(windowID)))
		}
		_ = runTmux("kill-window", "-t", windowID)
	}
	return nil
}

type destroyTarget struct {
	Reg                     *registry
	Record                  *agentRecord
	WindowID                string
	DestroyingCurrentWindow bool
	RequiresExplicitConfirm bool
}

func loadDestroyTarget(agentID string) (destroyTarget, error) {
	agentID = strings.TrimSpace(agentID)
	reg, err := loadRegistry()
	if err != nil {
		return destroyTarget{}, err
	}
	record := reg.Agents[agentID]
	if record == nil {
		return destroyTarget{}, fmt.Errorf("unknown agent: %s", agentID)
	}
	windowID := activeAgentWindowID(record)
	requiresExplicitConfirm, err := destroyRequiresExplicitConfirm(record)
	if err != nil {
		return destroyTarget{}, err
	}
	if strings.TrimSpace(windowID) != "" {
		openWindowTodos, err := countOpenTmuxTodos(todoScopeWindow, windowID)
		if err != nil {
			return destroyTarget{}, err
		}
		if openWindowTodos > 0 {
			label := "todos"
			if openWindowTodos == 1 {
				label = "todo"
			}
			return destroyTarget{}, fmt.Errorf("refusing to destroy agent with %d open window %s", openWindowTodos, label)
		}
	}
	currentWindowID := currentTmuxWindowID()
	return destroyTarget{
		Reg:                     reg,
		Record:                  record,
		WindowID:                windowID,
		DestroyingCurrentWindow: currentWindowID != "" && strings.TrimSpace(windowID) == currentWindowID,
		RequiresExplicitConfirm: requiresExplicitConfirm,
	}, nil
}

func destroyRequiresExplicitConfirm(record *agentRecord) (bool, error) {
	if record == nil {
		return false, nil
	}
	repoPath := strings.TrimSpace(record.RepoCopyPath)
	if repoPath == "" || !fileExists(repoPath) {
		return false, nil
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return false, err
		}
		return false, fmt.Errorf("check git status: %s", message)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func runTmuxCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent tmux <on-focus|focus|palette>")
	}
	switch args[0] {
	case "on-focus":
		return runTmuxOnFocus(args[1:])
	case "focus":
		return runTmuxFocus(args[1:])
	case "palette":
		return runTmuxPalette(args[1:])
	default:
		return fmt.Errorf("unknown tmux subcommand: %s", args[0])
	}
}

func runBrowserCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent browser <open|refresh>")
	}
	fs := flag.NewFlagSet("agent browser", flag.ContinueOnError)
	var workspace string
	var allowOpen bool
	var preserveFocus bool
	fs.StringVar(&workspace, "workspace", "", "workspace root containing agent.json")
	fs.BoolVar(&allowOpen, "allow-open", false, "open a new tab if missing")
	fs.BoolVar(&preserveFocus, "preserve-focus", false, "restore the previously frontmost app after browser changes")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("workspace is required")
	}
	featurePath := filepath.Join(workspace, "agent.json")
	switch args[0] {
	case "open":
		return syncChromeForFeature(featurePath, allowOpen, preserveFocus)
	case "refresh":
		return refreshChromeForFeature(featurePath, preserveFocus)
	default:
		return fmt.Errorf("unknown browser subcommand: %s", args[0])
	}
}

func runFeatureCommand(args []string) error {
	fs := flag.NewFlagSet("agent feature", flag.ContinueOnError)
	var workspace string
	var device string
	var readyText string
	var shouldOpenTabText string
	var writeScripts bool
	fs.StringVar(&workspace, "workspace", "", "workspace root containing agent.json")
	fs.StringVar(&device, "device", "", "set flutter device")
	fs.StringVar(&readyText, "ready", "", "set ready state (true/false)")
	fs.StringVar(&shouldOpenTabText, "should-open-tab", "", "set browser-open state (true/false)")
	fs.BoolVar(&writeScripts, "write-helper-scripts", false, "rewrite generated helper scripts for the workspace")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("--workspace is required")
	}
	featurePath := filepath.Join(workspace, "agent.json")
	if writeScripts {
		cfg, err := loadFeatureConfig(featurePath)
		if err != nil {
			return err
		}
		return writeFlutterHelperScripts(workspace, filepath.Join(workspace, "repo"), cfg.URL, cfg.Device)
	}
	if err := updateFeatureConfig(featurePath, func(cfg *featureConfig) error {
		if strings.TrimSpace(device) != "" {
			cfg.Device = strings.TrimSpace(device)
		}
		if strings.TrimSpace(readyText) != "" {
			value, err := strconv.ParseBool(strings.TrimSpace(readyText))
			if err != nil {
				return fmt.Errorf("invalid --ready value: %w", err)
			}
			cfg.Ready = value
		}
		if strings.TrimSpace(shouldOpenTabText) != "" {
			value, err := strconv.ParseBool(strings.TrimSpace(shouldOpenTabText))
			if err != nil {
				return fmt.Errorf("invalid --should-open-tab value: %w", err)
			}
			cfg.ShouldOpenTab = value
		}
		return nil
	}); err != nil {
		return err
	}
	if strings.TrimSpace(device) != "" {
		if err := syncFeatureDeviceToRegistry(workspace, featurePath, strings.TrimSpace(device)); err != nil {
			return err
		}
	}
	return nil
}

func syncFeatureDeviceToRegistry(workspaceRoot, featurePath, device string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	featurePath = filepath.Clean(strings.TrimSpace(featurePath))
	device = strings.TrimSpace(device)
	browserEnabled := device == "web-server"
	updated := false
	for _, record := range reg.Agents {
		if record == nil {
			continue
		}
		if filepath.Clean(strings.TrimSpace(record.WorkspaceRoot)) != workspaceRoot && filepath.Clean(strings.TrimSpace(record.FeatureConfig)) != featurePath {
			continue
		}
		record.Device = device
		record.BrowserEnabled = browserEnabled
		record.UpdatedAt = time.Now()
		updated = true
		break
	}
	if !updated {
		return nil
	}
	return saveRegistry(reg)
}

func runTmuxOnFocus(args []string) error {
	fs := flag.NewFlagSet("agent tmux on-focus", flag.ContinueOnError)
	var sessionID, windowID, paneID string
	fs.StringVar(&sessionID, "session", "", "session id")
	fs.StringVar(&windowID, "window", "", "window id")
	fs.StringVar(&paneID, "pane", "", "pane id")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = sessionID
	ctx, err := detectCurrentAgentFromTmux(windowID)
	if err != nil {
		return nil
	}
	_ = paneID
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	record := reg.Agents[ctx.ID]
	if record == nil {
		return nil
	}
	if reg.FocusedAgentID == record.ID {
		return nil
	}
	now := time.Now()
	record.LastFocusedAt = &now
	record.UpdatedAt = now
	reg.FocusedAgentID = record.ID
	if err := saveRegistry(reg); err != nil {
		return err
	}
	if record.BrowserEnabled {
		_ = syncChromeForFeature(record.FeatureConfig, true, true)
	}
	return nil
}

func runTmuxFocus(args []string) error {
	fs := flag.NewFlagSet("agent tmux focus", flag.ContinueOnError)
	var windowID string
	fs.StringVar(&windowID, "window", "", "window id")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: agent tmux focus <ai|git|dashboard|run>")
	}
	role := strings.ToLower(fs.Arg(0))
	ctx, err := detectCurrentAgentFromTmux(windowID)
	if err != nil {
		return err
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	record := reg.Agents[ctx.ID]
	if record == nil {
		return fmt.Errorf("unknown agent: %s", ctx.ID)
	}
	target := ""
	switch role {
	case "ai":
		target = record.Panes.AI
	case "git":
		target = record.Panes.Git
	case "dashboard":
		return openDashboardPopup(record.ID)
	case "run":
		target = record.Panes.Run
	default:
		return fmt.Errorf("unknown pane role: %s", role)
	}
	if target == "" {
		return fmt.Errorf("pane not found for role: %s", role)
	}
	return runTmux("select-pane", "-t", target)
}

func openDashboardPopup(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s dashboard --agent-id %s", shellQuote(exe), shellQuote(agentID))
	return runTmux("display-popup", "-E", "-w", "78%", "-h", "80%", "-T", "dashboard", cmd)
}

func runTmuxPalette(args []string) error {
	fs := flag.NewFlagSet("agent tmux palette", flag.ContinueOnError)
	var windowID string
	fs.StringVar(&windowID, "window", "", "window id")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, err := tmuxPaletteContext(windowID)
	if err != nil {
		return err
	}
	windowID = ctx.WindowID
	if windowID == "" {
		return fmt.Errorf("window id is required")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(
		"%s palette --window=%s --agent-id=%s --path=%s --session-name=%s --window-name=%s",
		shellQuote(exe),
		shellQuote(ctx.WindowID),
		shellQuote(ctx.AgentID),
		shellQuote(ctx.CurrentPath),
		shellQuote(ctx.SessionName),
		shellQuote(ctx.WindowName),
	)
	return runTmux("display-popup", "-E", "-w", "78%", "-h", "80%", "-T", "agent", cmd)
}

type currentAgentRef struct{ ID string }

type tmuxPaletteLaunchContext struct {
	WindowID    string
	AgentID     string
	CurrentPath string
	SessionName string
	WindowName  string
}

func tmuxPaletteContext(windowID string) (tmuxPaletteLaunchContext, error) {
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(windowID) != "" {
		args = append(args, "-t", strings.TrimSpace(windowID))
	}
	args = append(args, "#{window_id}\n#{@agent_id}\n#{pane_current_path}\n#{session_name}\n#{window_name}")
	out, err := runTmuxOutput(args...)
	if err != nil {
		return tmuxPaletteLaunchContext{}, err
	}
	parts := strings.SplitN(strings.TrimRight(out, "\n"), "\n", 5)
	for len(parts) < 5 {
		parts = append(parts, "")
	}
	return tmuxPaletteLaunchContext{
		WindowID:    strings.TrimSpace(parts[0]),
		AgentID:     strings.TrimSpace(parts[1]),
		CurrentPath: strings.TrimSpace(parts[2]),
		SessionName: strings.TrimSpace(parts[3]),
		WindowName:  strings.TrimSpace(parts[4]),
	}, nil
}

func detectCurrentAgentFromTmux(windowID string) (currentAgentRef, error) {
	if windowID == "" {
		out, err := runTmuxOutput("display-message", "-p", "#{window_id}")
		if err != nil {
			return currentAgentRef{}, err
		}
		windowID = strings.TrimSpace(out)
	}
	out, err := runTmuxOutput("show-options", "-wqv", "-t", windowID, "@agent_id")
	if err != nil {
		return currentAgentRef{}, err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return currentAgentRef{}, fmt.Errorf("no agent id for window %s", windowID)
	}
	return currentAgentRef{ID: id}, nil
}

func gatedWorkspaceCommand(workspaceRoot, readyMarker, successCmd string) string {
	failurePath := bootstrapFailedPath(workspaceRoot)
	return fmt.Sprintf(
		"cd %s; while [ ! -f %s ] && [ ! -f %s ]; do sleep 0.2; done; if [ -f %s ]; then cat %s 2>/dev/null; printf '\\nSee %%s\\n' %s; exec ${SHELL:-/bin/zsh}; fi; %s",
		shellQuote(workspaceRoot),
		shellQuote(readyMarker),
		shellQuote(failurePath),
		shellQuote(failurePath),
		shellQuote(failurePath),
		shellQuote(bootstrapLogPath(workspaceRoot)),
		successCmd,
	)
}

func currentTmuxWindowID() string {
	if out, err := runTmuxOutput("display-message", "-p", "#{window_id}"); err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}

func activeAgentWindowID(record *agentRecord) string {
	if record == nil {
		return ""
	}
	windowID := strings.TrimSpace(record.TmuxWindowID)
	if os.Getenv("TMUX") == "" {
		return windowID
	}
	ctx, err := detectCurrentAgentFromTmux("")
	if err != nil || strings.TrimSpace(ctx.ID) != strings.TrimSpace(record.ID) {
		return windowID
	}
	currentWindowID := currentTmuxWindowID()
	if currentWindowID != "" {
		return currentWindowID
	}
	return windowID
}

func launchAgentLayout(record *agentRecord) error {
	windowID, sessionID, sessionName, attachAfter, err := createWindow(record.Name, record.RepoCopyPath, record.LaunchWindowID)
	if err != nil {
		return err
	}

	if err := runTmux("select-window", "-t", windowID); err != nil {
		return err
	}
	topPane, err := currentPane(windowID)
	if err != nil {
		return err
	}
	if err := runTmux("split-window", "-t", topPane, "-h", "-p", "40", "-c", record.WorkspaceRoot); err != nil {
		return err
	}
	rightPane, err := currentPane(windowID)
	if err != nil {
		return err
	}
	if err := runTmux("split-window", "-t", rightPane, "-v", "-p", "35", "-c", record.WorkspaceRoot); err != nil {
		return err
	}
	runPane, err := currentPane(windowID)
	if err != nil {
		return err
	}
	gitPane := rightPane
	aiPane := topPane

	if err := runTmux("set-option", "-w", "-t", windowID, "@agent_id", record.ID); err != nil {
		return err
	}
	_ = runTmux("rename-window", "-t", windowID, record.ID)
	for pane, role := range map[string]string{aiPane: "ai", gitPane: "git", runPane: "run"} {
		_ = runTmux("set-option", "-p", "-t", pane, "@agent_role", role)
	}

	record.TmuxSessionID = sessionID
	record.TmuxSessionName = sessionName
	record.TmuxWindowID = windowID
	record.Panes = agentPanes{AI: aiPane, Git: gitPane, Run: runPane}
	record.UpdatedAt = time.Now()
	reg, err := loadRegistry()
	if err == nil {
		reg.Agents[record.ID] = record
		_ = saveRegistry(reg)
	}

	aiCmd := fmt.Sprintf("cd %s; exec ${SHELL:-/bin/zsh}", shellQuote(record.RepoCopyPath))
	gitCmd := gatedWorkspaceCommand(
		record.WorkspaceRoot,
		bootstrapGitReadyPath(record.WorkspaceRoot),
		fmt.Sprintf("cd %s; if command -v lazygit >/dev/null 2>&1; then lazygit; fi; exec ${SHELL:-/bin/zsh}", shellQuote(record.RepoCopyPath)),
	)
	runCmd := agentRunPaneCommand(record)
	for pane, cmd := range map[string]string{aiPane: aiCmd, gitPane: gitCmd, runPane: runCmd} {
		if err := runTmux("respawn-pane", "-k", "-t", pane, cmd); err != nil {
			return err
		}
	}
	if err := runTmux("select-pane", "-t", aiPane); err != nil {
		return err
	}
	if attachAfter && canAttachTmux() {
		return runTmux("attach-session", "-t", sessionID)
	}
	return nil
}

func primeAgentAIPane(paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return nil
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err := runTmuxOutput("display-message", "-p", "-t", paneID, "#{pane_current_command}")
		if err == nil {
			switch strings.TrimSpace(out) {
			case "zsh", "bash", "sh", "fish":
				deadline = time.Now()
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := runTmux("send-keys", "-t", paneID, "-l", "op"); err != nil {
		return err
	}
	return runTmux("send-keys", "-t", paneID, "Enter")
}

func agentRunPaneCommand(record *agentRecord) string {
	if record == nil {
		return ""
	}
	shellCmd := gatedWorkspaceCommand(
		record.WorkspaceRoot,
		bootstrapRepoReadyPath(record.WorkspaceRoot),
		fmt.Sprintf("cd %s; exec ${SHELL:-/bin/zsh}", shellQuote(record.WorkspaceRoot)),
	)
	if record.Runtime != "flutter" || strings.TrimSpace(record.Device) == "" {
		return shellCmd
	}
	return gatedWorkspaceCommand(
		record.WorkspaceRoot,
		bootstrapRepoReadyPath(record.WorkspaceRoot),
		fmt.Sprintf("cd %s; ./ensure-server.sh %s; exec ${SHELL:-/bin/zsh}", shellQuote(record.WorkspaceRoot), shellQuote(record.Device)),
	)
}

func canAttachTmux() bool {
	if os.Getenv("TMUX") != "" {
		return false
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func createWindow(feature, path string, targetWindowID string) (windowID, sessionID, sessionName string, attachAfter bool, err error) {
	targetWindowID = preferredNewWindowTarget(targetWindowID, os.Getenv("TMUX") != "", currentTmuxWindowID())
	if targetWindowID != "" {
		targetSessionID, targetSessionName, resolveErr := tmuxSessionForWindow(targetWindowID)
		if resolveErr == nil && targetSessionID != "" {
			windowID, err = runTmuxOutput(positionedNewWindowArgs(feature, path, targetWindowID)...)
			if err != nil {
				return "", "", "", false, err
			}
			return strings.TrimSpace(windowID), strings.TrimSpace(targetSessionID), strings.TrimSpace(targetSessionName), false, nil
		}
	}
	if os.Getenv("TMUX") != "" {
		windowID, err = runTmuxOutput(positionedNewWindowArgs(feature, path, "")...)
		if err != nil {
			return "", "", "", false, err
		}
		windowID = strings.TrimSpace(windowID)
		sessionID, _ = runTmuxOutput("display-message", "-p", "-t", windowID, "#{session_id}")
		sessionName, _ = runTmuxOutput("display-message", "-p", "-t", windowID, "#{session_name}")
		return windowID, strings.TrimSpace(sessionID), strings.TrimSpace(sessionName), false, nil
	}
	repoName := filepath.Base(path)
	desiredSessionLabel := sanitizeFeatureName(repoName + "-agents")
	if desiredSessionLabel == "" {
		desiredSessionLabel = "agents"
	}
	if existingSessionID, existingSessionName, ok := findTmuxSessionByLabel(desiredSessionLabel); ok {
		windowID, err = runTmuxOutput("new-window", "-P", "-F", "#{window_id}", "-t", existingSessionID, "-n", feature, "-c", path)
		if err != nil {
			return "", "", "", false, err
		}
		return strings.TrimSpace(windowID), strings.TrimSpace(existingSessionID), strings.TrimSpace(existingSessionName), true, nil
	}
	if err := runTmux("new-session", "-d", "-s", desiredSessionLabel, "-n", feature, "-c", path); err != nil {
		return "", "", "", false, err
	}
	attachAfter = true
	sessionID, _ = runTmuxOutput("display-message", "-p", "-t", desiredSessionLabel+":1", "#{session_id}")
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		if existingSessionID, existingSessionName, ok := findTmuxSessionByLabel(desiredSessionLabel); ok {
			sessionID = strings.TrimSpace(existingSessionID)
			sessionName = strings.TrimSpace(existingSessionName)
		}
	}
	if sessionID == "" {
		return "", "", "", false, fmt.Errorf("unable to resolve tmux session for %s", desiredSessionLabel)
	}
	windowID, _ = runTmuxOutput("list-windows", "-t", sessionID, "-F", "#{window_id}")
	if sessionName == "" {
		sessionName, _ = runTmuxOutput("display-message", "-p", "-t", sessionID, "#{session_name}")
	}
	return strings.TrimSpace(strings.Split(windowID, "\n")[0]), strings.TrimSpace(sessionID), strings.TrimSpace(sessionName), true, nil
}

func preferredNewWindowTarget(targetWindowID string, inTmux bool, currentWindowID string) string {
	targetWindowID = strings.TrimSpace(targetWindowID)
	if targetWindowID != "" {
		return targetWindowID
	}
	if !inTmux {
		return ""
	}
	return strings.TrimSpace(currentWindowID)
}

func positionedNewWindowArgs(feature, path, targetWindowID string) []string {
	args := []string{"new-window", "-P", "-F", "#{window_id}"}
	if targetWindowID = strings.TrimSpace(targetWindowID); targetWindowID != "" {
		args = append(args, "-a", "-t", targetWindowID)
	}
	args = append(args, "-n", feature, "-c", path)
	return args
}

func tmuxSessionForWindow(windowID string) (sessionID, sessionName string, err error) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return "", "", fmt.Errorf("window id is required")
	}
	out, err := runTmuxOutput("display-message", "-p", "-t", windowID, "#{session_id}\n#{session_name}")
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(strings.TrimRight(out, "\n"), "\n", 2)
	for len(parts) < 2 {
		parts = append(parts, "")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func findTmuxSessionByLabel(label string) (sessionID, sessionName string, ok bool) {
	out, err := runTmuxOutput("list-sessions", "-F", "#{session_id}\t#{session_name}")
	if err != nil {
		return "", "", false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if name == label {
			return id, name, true
		}
		if match := regexp.MustCompile(`^\d+-(.+)$`).FindStringSubmatch(name); len(match) == 2 && strings.TrimSpace(match[1]) == label {
			return id, name, true
		}
	}
	return "", "", false
}

func currentPane(windowID string) (string, error) {
	out, err := runTmuxOutput("display-message", "-p", "-t", windowID, "#{pane_id}")
	return strings.TrimSpace(out), err
}

func windowAlive(sessionID, windowID string) bool {
	if strings.TrimSpace(windowID) == "" {
		return false
	}
	out, err := runTmuxOutput("list-windows", "-a", "-F", "#{session_id}\t#{window_id}")
	if err != nil {
		return false
	}
	targetWindow := strings.TrimSpace(windowID)
	targetSession := strings.TrimSpace(sessionID)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[1]) != targetWindow {
			continue
		}
		if targetSession == "" || strings.TrimSpace(parts[0]) == targetSession {
			return true
		}
	}
	return false
}

func selectTmuxWindow(windowID string) error {
	if err := runTmux("select-window", "-t", windowID); err != nil {
		return err
	}
	return nil
}

func repoRoot() (string, error) {
	out, err := runGitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not in a git repo")
	}
	return strings.TrimSpace(out), nil
}

func ensureFlutterWebRepo(repoRoot string) error {
	if !fileExists(filepath.Join(repoRoot, "pubspec.yaml")) || !dirExists(filepath.Join(repoRoot, "web")) {
		return fmt.Errorf("agent init only works for Flutter web repos right now; expected both pubspec.yaml and a web/ directory")
	}
	return nil
}

func requiredAgentExcludeEntries(repoRoot string, isFlutter bool) []string {
	entries := []string{".agent.yaml"}
	entries = append(entries, ".agents")
	if isFlutter {
		entries = append(entries, "web_dev_config.yaml")
		entries = append(entries, "hot-reload.sh")
	}
	return entries
}

func ensureRepoCopyLocalExcludes(repoCopyPath string, isFlutter bool) error {
	return ensureGitExcludeEntries(repoCopyPath, requiredAgentExcludeEntries(repoCopyPath, isFlutter))
}

func gitInfoExcludePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".git", "info", "exclude")
}

func ensureGitExcludeEntries(repoRoot string, entries []string) error {
	path := gitInfoExcludePath(repoRoot)
	existing := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				existing[trimmed] = true
			}
		}
	}
	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" || existing[entry] {
			continue
		}
		missing = append(missing, entry)
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	if len(data) > 0 {
		builder.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			builder.WriteByte('\n')
		}
	}
	for _, entry := range missing {
		builder.WriteString(entry)
		builder.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func gitPathIgnored(repoRoot, relPath string) (bool, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return false, fmt.Errorf("path is required")
	}
	cmd := exec.Command("git", "check-ignore", "-q", "--", relPath)
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func runGitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func defaultAgentKeyPaths() []string {
	return []string{"AGENTS.md", ".agent-prompts", "opencode.json"}
}

func bootstrapStateDirPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".bootstrap")
}

func bootstrapGitReadyPath(workspaceRoot string) string {
	return filepath.Join(bootstrapStateDirPath(workspaceRoot), "git-ready")
}

func bootstrapRepoReadyPath(workspaceRoot string) string {
	return filepath.Join(bootstrapStateDirPath(workspaceRoot), "repo-ready")
}

func bootstrapFailedPath(workspaceRoot string) string {
	return filepath.Join(bootstrapStateDirPath(workspaceRoot), "failed")
}

func bootstrapPIDPath(workspaceRoot string) string {
	return filepath.Join(bootstrapStateDirPath(workspaceRoot), "bootstrap.pid")
}

func bootstrapLogPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, "logs", "bootstrap.log")
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func repoRootFromWorkspaceRoot(workspaceRoot string) string {
	clean := filepath.Clean(strings.TrimSpace(workspaceRoot))
	needle := string(filepath.Separator) + ".agents" + string(filepath.Separator)
	if idx := strings.Index(clean, needle); idx >= 0 {
		return clean[:idx]
	}
	return ""
}

func loadAgentRecordByWorkspaceRoot(workspaceRoot string) *agentRecord {
	reg, err := loadRegistry()
	if err != nil {
		return nil
	}
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	for _, record := range reg.Agents {
		if record == nil {
			continue
		}
		if filepath.Clean(strings.TrimSpace(record.WorkspaceRoot)) == workspaceRoot {
			return record
		}
	}
	return nil
}

func resolveStartSourceBranch(repoRoot string, repoCfg *repoConfig) string {
	if branch := currentLocalBranch(repoRoot); branch != "" {
		return branch
	}
	if repoCfg != nil && strings.TrimSpace(repoCfg.BaseBranch) != "" {
		return strings.TrimSpace(repoCfg.BaseBranch)
	}
	return detectDefaultBaseBranch(repoRoot)
}

func resolveBootstrapStartOptions(repoRoot string, repoCfg *repoConfig, record *agentRecord) agentStartOptions {
	options := agentStartOptions{}
	if record != nil {
		options.SourceBranch = strings.TrimSpace(record.SourceBranch)
		options.KeepWorktree = record.KeepWorktree
	}
	if options.SourceBranch == "" {
		if repoCfg != nil && strings.TrimSpace(repoCfg.BaseBranch) != "" {
			options.SourceBranch = strings.TrimSpace(repoCfg.BaseBranch)
		} else {
			options.SourceBranch = detectDefaultBaseBranch(repoRoot)
		}
	}
	return options
}

func prepareAgentContext(repoRoot, repoCopyPath string, keyPaths []string, ignoreExisting bool) error {
	if err := os.MkdirAll(repoCopyPath, 0o755); err != nil {
		return err
	}
	return copySelectedRepoPaths(repoRoot, repoCopyPath, keyPaths, ignoreExisting)
}

func copySelectedRepoPaths(srcRoot, destRoot string, paths []string, ignoreExisting bool) error {
	for _, relPath := range normalizeIgnoreValues(paths) {
		relPath = filepath.Clean(filepath.FromSlash(relPath))
		if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			continue
		}
		if !pathExists(filepath.Join(srcRoot, relPath)) {
			continue
		}
		args := []string{"-a", "--relative"}
		if ignoreExisting {
			args = append(args, "--ignore-existing")
		}
		args = append(args, filepath.ToSlash("."+string(filepath.Separator)+relPath), destRoot+"/")
		cmd := exec.Command("rsync", args...)
		cmd.Dir = srcRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				return err
			}
			return fmt.Errorf("copy path %s: %w: %s", relPath, err, message)
		}
	}
	return nil
}

func pathDepth(path string) int {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return 0
	}
	return len(strings.Split(clean, string(filepath.Separator)))
}

func copyRepoExcludeValues(extraIgnores []string) []string {
	return append(defaultCopyIgnoreExcludes(), extraIgnores...)
}

func copyGitMetadata(srcRoot, repoCopyPath string) error {
	gitPath := filepath.Join(repoCopyPath, ".git")
	if err := os.RemoveAll(gitPath); err != nil {
		return err
	}
	if err := os.MkdirAll(repoCopyPath, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("rsync", "-a", filepath.Join(srcRoot, ".git")+"/", gitPath+"/")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("copy git metadata: %w: %s", err, message)
	}
	return nil
}

func syncRepoWorktree(srcRoot, repoCopyPath string, extraIgnores []string) error {
	if err := os.MkdirAll(repoCopyPath, 0o755); err != nil {
		return err
	}
	args := []string{"-a", "--delete", "--filter", ":- .gitignore", "--exclude", ".git", "--exclude", ".git/**"}
	for _, value := range copyRepoExcludeValues(extraIgnores) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		args = append(args, "--exclude", value)
	}
	args = append(args, srcRoot+"/", repoCopyPath+"/")
	cmd := exec.Command("rsync", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("sync repo worktree: %w: %s", err, message)
	}
	return nil
}

func applyRepoCopyIgnores(sourceRepoRoot, repoCopyPath string, extraIgnores []string) error {
	relPaths, err := repoCopyIgnoredPaths(sourceRepoRoot, repoCopyPath, extraIgnores)
	if err != nil {
		return err
	}
	if len(relPaths) == 0 {
		return nil
	}
	trackedPaths, err := trackedPathsForRepoCopy(repoCopyPath, relPaths)
	if err != nil {
		return err
	}
	if err := markGitPathsSkipWorktree(repoCopyPath, trackedPaths); err != nil {
		return err
	}
	for _, relPath := range relPaths {
		if err := os.RemoveAll(filepath.Join(repoCopyPath, relPath)); err != nil {
			return err
		}
	}
	return nil
}

func repoCopyIgnoredPaths(sourceRepoRoot, repoCopyPath string, extraIgnores []string) ([]string, error) {
	args := []string{"-a", "-n", "--delete", "--delete-excluded", "--itemize-changes", "--exclude", ".git"}
	for _, value := range copyRepoExcludeValues(extraIgnores) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		args = append(args, "--exclude", value)
	}
	args = append(args, sourceRepoRoot+"/", repoCopyPath+"/")
	cmd := exec.Command("rsync", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("compute repo copy ignores: %w: %s", err, message)
	}
	seen := map[string]bool{}
	relPaths := make([]string, 0)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "*deleting ") {
			continue
		}
		relPath := strings.TrimSpace(strings.TrimPrefix(line, "*deleting "))
		relPath = strings.TrimSuffix(relPath, "/")
		if relPath == "" {
			continue
		}
		relPath = filepath.Clean(filepath.FromSlash(relPath))
		if relPath == "." || relPath == ".git" || strings.HasPrefix(relPath, ".git"+string(filepath.Separator)) || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			continue
		}
		if seen[relPath] {
			continue
		}
		seen[relPath] = true
		relPaths = append(relPaths, relPath)
	}
	sort.Slice(relPaths, func(i, j int) bool {
		return pathDepth(relPaths[i]) > pathDepth(relPaths[j])
	})
	return relPaths, nil
}

func trackedPathsForRepoCopy(repoCopyPath string, relPaths []string) ([]string, error) {
	seen := map[string]bool{}
	trackedPaths := make([]string, 0)
	for _, relPath := range relPaths {
		cmd := exec.Command("git", "ls-files", "-z", "--", relPath)
		cmd.Dir = repoCopyPath
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		for _, trackedPath := range strings.Split(string(output), "\x00") {
			trackedPath = strings.TrimSpace(trackedPath)
			if trackedPath == "" || seen[trackedPath] {
				continue
			}
			seen[trackedPath] = true
			trackedPaths = append(trackedPaths, trackedPath)
		}
	}
	sort.Strings(trackedPaths)
	return trackedPaths, nil
}

func markGitPathsSkipWorktree(repoCopyPath string, trackedPaths []string) error {
	if len(trackedPaths) == 0 {
		return nil
	}
	input := strings.Join(trackedPaths, "\x00") + "\x00"
	cmd := exec.Command("git", "update-index", "--skip-worktree", "-z", "--stdin")
	cmd.Dir = repoCopyPath
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("mark skip-worktree: %w: %s", err, message)
	}
	return nil
}

func removeLegacyRuntimeProject(workspaceRoot string) error {
	return os.RemoveAll(filepath.Join(workspaceRoot, "runtime"))
}

func createFeatureBranch(repoCopyPath, branch, sourceBranch string, preservePaths []string) (string, error) {
	sourceBranch = strings.TrimSpace(sourceBranch)
	if sourceBranch == "" {
		sourceBranch = detectDefaultBaseBranch(repoCopyPath)
	}
	resetHeadCmd := exec.Command("git", "reset", "--hard", "HEAD")
	resetHeadCmd.Dir = repoCopyPath
	resetHeadCmd.Stdout = io.Discard
	resetHeadCmd.Stderr = io.Discard
	if err := resetHeadCmd.Run(); err != nil {
		return "", err
	}
	cleanArgs := []string{"clean", "-fdx"}
	for _, preservePath := range normalizeIgnoreValues(preservePaths) {
		preservePath = filepath.Clean(filepath.FromSlash(preservePath))
		if preservePath == "." || strings.HasPrefix(preservePath, ".."+string(filepath.Separator)) {
			continue
		}
		cleanArgs = append(cleanArgs, "-e", preservePath)
		if dirExists(filepath.Join(repoCopyPath, preservePath)) {
			cleanArgs = append(cleanArgs, "-e", filepath.ToSlash(preservePath)+"/**")
		}
	}
	cleanCmd := exec.Command("git", cleanArgs...)
	cleanCmd.Dir = repoCopyPath
	cleanCmd.Stdout = io.Discard
	cleanCmd.Stderr = io.Discard
	if err := cleanCmd.Run(); err != nil {
		return "", err
	}
	fetchCmd := exec.Command("git", "remote", "update", "-p")
	fetchCmd.Dir = repoCopyPath
	_ = fetchCmd.Run()
	if remoteExists(repoCopyPath, "origin/"+sourceBranch) {
		cmd := exec.Command("git", "checkout", "-B", sourceBranch, "origin/"+sourceBranch)
		cmd.Dir = repoCopyPath
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err == nil {
			resetCmd := exec.Command("git", "reset", "--hard", "origin/"+sourceBranch)
			resetCmd.Dir = repoCopyPath
			resetCmd.Stdout = io.Discard
			resetCmd.Stderr = io.Discard
			if err := resetCmd.Run(); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	} else if localExists(repoCopyPath, sourceBranch) {
		cmd := exec.Command("git", "checkout", "-B", sourceBranch)
		cmd.Dir = repoCopyPath
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return "", err
		}
	} else {
		cmd := exec.Command("git", "checkout", "-B", sourceBranch)
		cmd.Dir = repoCopyPath
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}
	cmd := exec.Command("git", "checkout", "-B", branch, sourceBranch)
	cmd.Dir = repoCopyPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return branch, nil
}

func loadRepoConfig(repoRoot string) (*repoConfig, error) {
	path := repoConfigPath(repoRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("missing %s; run `agent init` first", path)
	}
	if err != nil {
		return nil, err
	}
	cfg := defaultRepoConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	normalizeRepoConfig(cfg)
	return cfg, nil
}

func loadRepoConfigOrDefault(repoRoot string) (*repoConfig, error) {
	path := repoConfigPath(repoRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultRepoConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := defaultRepoConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	normalizeRepoConfig(cfg)
	return cfg, nil
}

func repoConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".agent.yaml")
}

func defaultRepoConfig() *repoConfig {
	return &repoConfig{
		CopyIgnore:    []string{"build", ".dart_tool"},
		AgentKeyPaths: defaultAgentKeyPaths(),
	}
}

func defaultCopyIgnoreExcludes() []string {
	return []string{".agents", ".DS_Store"}
}

func normalizeRepoConfig(cfg *repoConfig) {
	if cfg == nil {
		return
	}
	cfg.BaseBranch = strings.TrimSpace(cfg.BaseBranch)
	cfg.CopyIgnore = normalizeIgnoreValues(cfg.CopyIgnore)
	cfg.AgentKeyPaths = normalizeIgnoreValues(cfg.AgentKeyPaths)
}

func normalizeIgnoreValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		value = strings.Trim(value, ",")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func saveRepoConfig(repoRoot string, cfg *repoConfig) error {
	normalizeRepoConfig(cfg)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := repoConfigPath(repoRoot)
	return os.WriteFile(path, data, 0o644)
}

func loadFeatureConfig(path string) (*featureConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var cfg featureConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.IsFlutter {
		if _, ok := raw["device"]; !ok {
			cfg.Device = defaultManagedDeviceID
		}
	}
	return &cfg, nil
}

func saveFeatureConfig(path string, cfg featureConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmpPath := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func updateFeatureConfig(path string, update func(*featureConfig) error) error {
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	cfg, err := loadFeatureConfig(path)
	if err != nil {
		return err
	}
	if err := update(cfg); err != nil {
		return err
	}
	return saveFeatureConfig(path, *cfg)
}

func configureFlutterWebConfig(repoRoot, repoCopyPath string, port int) error {
	if port <= 0 {
		return nil
	}
	templatePath := filepath.Join(repoRoot, "agents", "web_dev_config.template.yaml")
	configPath := filepath.Join(repoCopyPath, "web_dev_config.yaml")
	if fileExists(templatePath) && !fileExists(configPath) {
		data, err := os.ReadFile(templatePath)
		if err == nil {
			if err := os.WriteFile(configPath, data, 0o644); err != nil {
				return err
			}
		}
	}
	content := fmt.Sprintf("server:\n  host: \"localhost\"\n  port: %d\n  headers:\n    - name: \"Cache-Control\"\n      value: \"no-cache, no-store, must-revalidate\"\n", port)
	return os.WriteFile(configPath, []byte(content), 0o644)
}

func ensureGeneratedRepoPathIgnored(repoCopyPath, relPath string) error {
	relPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if relPath == "" || relPath == "." {
		return fmt.Errorf("relative path is required")
	}
	trackedPaths, err := trackedPathsForRepoCopy(repoCopyPath, []string{relPath})
	if err != nil {
		return err
	}
	if len(trackedPaths) > 0 {
		return markGitPathsSkipWorktree(repoCopyPath, trackedPaths)
	}
	return ensureGitExcludeEntries(repoCopyPath, []string{relPath})
}

func writeFlutterHelperScripts(workspaceRoot, repoCopyPath, url, device string) error {
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "logs"), 0o755); err != nil {
		return err
	}
	ensureServer := `#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
INFO="$DIR/agent.json"
AGENT_BIN="${AGENT_BIN:-$HOME/.config/agent-tracker/bin/agent}"

new_device="${1-}"
if [[ -n "$new_device" ]]; then
  "$AGENT_BIN" feature --workspace "$DIR" --device "$new_device"
fi

port=$(python3 - "$INFO" <<'PY'
import json, pathlib, sys
data = json.loads(pathlib.Path(sys.argv[1]).read_text())
print(data.get('port', ''))
PY
)
device=$(python3 - "$INFO" <<'PY'
import json, pathlib, sys
data = json.loads(pathlib.Path(sys.argv[1]).read_text())
value = data.get('device')
if value is None:
    value = 'web-server'
print(value)
PY
)
logfile="$DIR/logs/flutter-$port.log"
: > "$logfile" 2>/dev/null || true

flutter_ready_seen() {
  if [[ -n "${TMUX_PANE-}" ]]; then
    if tmux capture-pane -p -S -200 -t "$TMUX_PANE" 2>/dev/null | grep -qi 'Flutter run key commands\.'; then
      return 0
    fi
  fi
  if [[ -f "$logfile" ]] && grep -qi 'Flutter run key commands\.' "$logfile" 2>/dev/null; then
    return 0
  fi
  return 1
}

if [[ -z "$device" ]]; then
  echo "No launch device selected. Run ./ensure-server.sh <device-id> to start Flutter."
  exit 0
fi

if [[ "$device" == "web-server" ]]; then
  (
    deadline=$((SECONDS+300))
    while [ $SECONDS -lt $deadline ]; do
      if flutter_ready_seen; then
        "$AGENT_BIN" feature --workspace "$DIR" --ready true
        sleep 2
        "$AGENT_BIN" browser refresh --workspace "$DIR" --preserve-focus >/dev/null 2>&1 || true
        exit 0
      fi
      sleep 0.1
    done
  ) &
fi

cd "$DIR"
exec script -q "$logfile" bash -lc "cd \"$DIR/repo\" && exec flutter run -d \"$device\""
`
	ensurePath := filepath.Join(workspaceRoot, "ensure-server.sh")
	if err := os.WriteFile(ensurePath, []byte(ensureServer), 0o755); err != nil {
		return err
	}
	hotReload := `#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_DIR="$(dirname "$REPO_DIR")"
INFO="$WORKSPACE_DIR/agent.json"
AGENT_BIN="${AGENT_BIN:-$HOME/.config/agent-tracker/bin/agent}"

port=$(python3 - "$INFO" <<'PY'
import json, pathlib, sys
data = json.loads(pathlib.Path(sys.argv[1]).read_text())
print(data.get('port', ''))
PY
)
device=$(python3 - "$INFO" <<'PY'
import json, pathlib, sys
data = json.loads(pathlib.Path(sys.argv[1]).read_text())
value = data.get('device')
if value is None:
    value = 'web-server'
print(value)
PY
)
logfile="$WORKSPACE_DIR/logs/flutter-$port.log"

if [[ -z "$device" ]]; then
  echo "No launch device selected"
  exit 1
fi

set +e
analyze_output=$(cd "$REPO_DIR" && flutter analyze lib --no-fatal-infos --no-fatal-warnings 2>&1)
analyze_exit=$?
set -e

filtered=$(printf "%s\n" "$analyze_output" | awk '/^Analyzing/ {found=1} found {print}')
[[ -n "$filtered" ]] && printf "%s\n" "$filtered"
if [[ $analyze_exit -ne 0 ]]; then
  echo "Analysis failed."
  [[ -z "$filtered" ]] && printf "%s\n" "$analyze_output"
  exit 1
fi

if [[ ! -f "$logfile" ]] || ! grep -qiE 'Flutter run key commands\.|is being served at|serving at|lib/main\.dart is being served' "$logfile" 2>/dev/null; then
  echo "Flutter server not ready"
  exit 1
fi

find_flutter_pane() {
  [[ -z "${TMUX-}" ]] && return 1

  has_flutter_run() {
    local pid=$1 depth=${2:-0}
    [[ $depth -gt 12 ]] && return 1
    local child
    while IFS= read -r child; do
      [[ -z "$child" ]] && continue
      if ps -p "$child" -o command= 2>/dev/null | grep -q 'flutter_tools\.snapshot.*run'; then
        return 0
      fi
      if has_flutter_run "$child" $((depth + 1)); then
        return 0
      fi
    done < <(pgrep -P "$pid" 2>/dev/null || true)
    return 1
  }

  local pane_id pane_pid pane_path
  while read -r pane_id pane_pid pane_path; do
    [[ -z "$pane_id" || -z "$pane_pid" ]] && continue
    [[ "$pane_path" != "$WORKSPACE_DIR" && "$pane_path" != "$REPO_DIR" ]] && continue
    if has_flutter_run "$pane_pid"; then
      printf "%s\n" "$pane_id"
      return 0
    fi
  done < <(tmux list-panes -a -F '#{pane_id} #{pane_pid} #{pane_current_path}' 2>/dev/null)

  return 1
}

target_pane=$(find_flutter_pane) || {
  echo "Flutter pane not found"
  exit 1
}
lines_before=$(wc -l < "$logfile")
tmux send-keys -t "$target_pane" r 2>/dev/null

(
  restart_server() {
    tmux send-keys -t "$target_pane" C-c 2>/dev/null || true
    sleep 0.5
    tmux send-keys -t "$target_pane" "cd '$WORKSPACE_DIR' && ./ensure-server.sh" Enter 2>/dev/null || true
  }

  for _ in $(seq 1 100); do
    newlines=$(python3 - "$logfile" "$lines_before" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
start = int(sys.argv[2])
if not path.exists():
    sys.exit(0)
with path.open('r', errors='ignore') as handle:
    lines = handle.readlines()
sys.stdout.write(''.join(lines[start:]))
PY
)
    if printf "%s\n" "$newlines" | grep -qiE 'Reloaded [0-9]+ libraries|Reloaded 1 of [0-9]+ libraries|Restarted application in'; then
      exit 0
    fi
    if printf "%s\n" "$newlines" | grep -qi 'Page requires refresh'; then
      "$AGENT_BIN" browser open --workspace "$WORKSPACE_DIR" --allow-open --preserve-focus >/dev/null 2>&1 || true
      "$AGENT_BIN" browser refresh --workspace "$WORKSPACE_DIR" --preserve-focus >/dev/null 2>&1 || true
      exit 0
    fi
    if printf "%s\n" "$newlines" | grep -qiE 'no client connected|no connected devices|Hot reload rejected'; then
      if [[ "$device" == "web-server" ]]; then
        "$AGENT_BIN" browser open --workspace "$WORKSPACE_DIR" --allow-open --preserve-focus >/dev/null 2>&1 || true
        "$AGENT_BIN" browser refresh --workspace "$WORKSPACE_DIR" --preserve-focus >/dev/null 2>&1 || true
        restart_server
      fi
      exit 0
    fi
    sleep 0.3
  done
exit 0
) >/dev/null 2>&1 &

echo "Hot reload triggered"
`
	if err := ensureGeneratedRepoPathIgnored(repoCopyPath, "hot-reload.sh"); err != nil {
		return err
	}
	hotReloadPath := filepath.Join(repoCopyPath, "hot-reload.sh")
	if err := os.WriteFile(hotReloadPath, []byte(hotReload), 0o755); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(workspaceRoot, "hot-reload.sh"))
	for _, obsolete := range []string{"open-tab.sh", "refresh-tab.sh", "on-tmux-window-activate.sh"} {
		_ = os.Remove(filepath.Join(workspaceRoot, obsolete))
	}
	_ = url
	_ = device
	return nil
}

func detectDefaultBaseBranch(repoRoot string) string {
	for _, candidate := range []string{"develop", "main", "master"} {
		if remoteExists(repoRoot, "origin/"+candidate) || localExists(repoRoot, candidate) {
			return candidate
		}
	}
	return "main"
}

func currentLocalBranch(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

func remoteExists(repoRoot, ref string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/"+ref)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

func localExists(repoRoot, ref string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+ref)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

func allocatePort(repoRoot string, start int) (int, error) {
	for port := start; port < start+500; port++ {
		if !portClaimedByRegistry(port) && !portClaimedByFeatureConfigs(repoRoot, port) && portFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("failed to allocate port")
}

func portClaimedByFeatureConfigs(repoRoot string, port int) bool {
	paths, err := filepath.Glob(filepath.Join(repoRoot, ".agents", "*", "agent.json"))
	if err != nil {
		return false
	}
	for _, path := range paths {
		cfg, err := loadFeatureConfig(path)
		if err == nil && cfg.Port == port {
			return true
		}
	}
	return false
}

func portClaimedByRegistry(port int) bool {
	reg, err := loadRegistry()
	if err != nil {
		return false
	}
	for _, record := range reg.Agents {
		if record.Port == port {
			return true
		}
	}
	return false
}

func portFree(port int) bool {
	cmd := exec.Command("lsof", "-PiTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-n")
	if err := cmd.Run(); err == nil {
		return false
	}
	return true
}

func loadRegistry() (*registry, error) {
	path := registryPath()
	reg := &registry{Agents: map[string]*agentRecord{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return reg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, reg); err != nil {
		return nil, err
	}
	if reg.Agents == nil {
		reg.Agents = map[string]*agentRecord{}
	}
	return reg, nil
}

func saveRegistry(reg *registry) error {
	path := registryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func registryPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "agent-tracker", "run", "agents.json")
}

func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "agent-tracker", "agent-config.json")
}

func loadAppConfig() appConfig {
	cfg := appConfig{Keys: keyConfig{
		MoveLeft:   "n",
		MoveRight:  "i",
		MoveUp:     "u",
		MoveDown:   "e",
		Edit:       "Enter",
		Cancel:     "Escape",
		AddTodo:    "a",
		ToggleTodo: "x",
		Destroy:    "D",
		Confirm:    "y",
		Back:       "Escape",
		DeleteTodo: "d",
		Help:       "?",
		FocusAI:    "M-a",
		FocusGit:   "M-g",
		FocusDash:  "M-s",
		FocusRun:   "M-r",
	}}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func sortedAgentIDs(reg *registry) []string {
	ids := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func promptInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func promptInputWithDefault(label, defaultValue string) (string, error) {
	if strings.TrimSpace(defaultValue) == "" {
		return promptInput(label + ": ")
	}
	value, err := promptInput(fmt.Sprintf("%s [%s]: ", label, defaultValue))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	return value, nil
}

func sanitizeFeatureName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = featureNamePattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runTmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func syncChromeForFeature(featurePath string, allowOpen bool, preserveFocus bool) error {
	cfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.URL) == "" || strings.TrimSpace(cfg.Device) != "web-server" {
		return nil
	}
	if !allowOpen && !preserveFocus {
		script := `on run argv
set targetUrl to item 1 of argv
set windowText to item 2 of argv
set tabText to item 3 of argv
tell application "Google Chrome"
  if windowText is not "" and tabText is not "" then
    try
      set targetWindow to window (windowText as integer)
      set targetTab to tab (tabText as integer) of targetWindow
      if (URL of targetTab starts with targetUrl) then
        set active tab index of targetWindow to (tabText as integer)
        return "reused\n" & windowText & "\n" & tabText
      end if
    end try
  end if
  set windowCounter to 0
  repeat with w in windows
    if mode of w is "normal" then
      set windowCounter to windowCounter + 1
      set i to 1
      repeat with t in tabs of w
        if (URL of t starts with targetUrl) then
          set active tab index of w to i
          return "switched\n" & windowCounter & "\n" & i
        end if
        set i to i + 1
      end repeat
    end if
  end repeat
end tell
return "none"
end run`
		out, err := runAppleScript(script, cfg.URL, strconv.Itoa(cfg.ChromeWindow), strconv.Itoa(cfg.ChromeTab))
		if err != nil {
			return err
		}
		parts := strings.Split(strings.TrimSpace(out), "\n")
		if len(parts) >= 3 && parts[0] != "none" {
			return updateFeatureConfig(featurePath, func(cfg *featureConfig) error {
				if v, convErr := strconv.Atoi(strings.TrimSpace(parts[1])); convErr == nil {
					cfg.ChromeWindow = v
				}
				if v, convErr := strconv.Atoi(strings.TrimSpace(parts[2])); convErr == nil {
					cfg.ChromeTab = v
				}
				return nil
			})
		}
		return nil
	}
	appID, appName := currentFrontApp()
	script := `on run argv
set targetUrl to item 1 of argv
set shouldOpen to item 2 of argv
set windowText to item 3 of argv
set tabText to item 4 of argv

tell application "Google Chrome"
  if windowText is not "" and tabText is not "" then
    try
      set targetWindow to window (windowText as integer)
      set targetTab to tab (tabText as integer) of targetWindow
      set tabUrl to URL of targetTab
      if (tabUrl starts with targetUrl) then
        set active tab index of targetWindow to (tabText as integer)
        return "reused\n" & windowText & "\n" & tabText
      end if
      if shouldOpen is "true" and tabUrl is "chrome-error://chromewebdata/" then
        set URL of targetTab to targetUrl
        set active tab index of targetWindow to (tabText as integer)
        return "reused\n" & windowText & "\n" & tabText
      end if
    end try
  end if
  set windowCounter to 0
  repeat with w in windows
    if mode of w is "normal" then
      set windowCounter to windowCounter + 1
      set i to 1
      repeat with t in tabs of w
        if (URL of t starts with targetUrl) then
          set active tab index of w to i
          return "switched\n" & windowCounter & "\n" & i
        end if
        set i to i + 1
      end repeat
    end if
  end repeat
  if shouldOpen is "true" then
    set targetWindow to missing value
    set targetWindowIndex to 0
    set windowCounter to 0
    repeat with w in windows
      if mode of w is "normal" then
        set windowCounter to windowCounter + 1
        set targetWindow to w
        set targetWindowIndex to windowCounter
        exit repeat
      end if
    end repeat
    if targetWindow is missing value then
      make new window
      set targetWindow to window 1
      set targetWindowIndex to 1
    end if
    tell targetWindow to make new tab with properties {URL:targetUrl}
    tell targetWindow to set active tab index to (count of tabs)
    return "opened\n" & targetWindowIndex & "\n" & (active tab index of targetWindow)
  end if
end tell
return "none"
end run`
	out, err := runAppleScript(script, cfg.URL, strconv.FormatBool(allowOpen), strconv.Itoa(cfg.ChromeWindow), strconv.Itoa(cfg.ChromeTab))
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimSpace(out), "\n")
	if preserveFocus && len(parts) >= 1 && strings.TrimSpace(parts[0]) == "opened" && strings.TrimSpace(appName) != "" && appName != "Google Chrome" {
		_ = restoreFrontApp(appID, appName)
	}
	if len(parts) >= 3 && parts[0] != "none" {
		return updateFeatureConfig(featurePath, func(cfg *featureConfig) error {
			if v, convErr := strconv.Atoi(strings.TrimSpace(parts[1])); convErr == nil {
				cfg.ChromeWindow = v
			}
			if v, convErr := strconv.Atoi(strings.TrimSpace(parts[2])); convErr == nil {
				cfg.ChromeTab = v
			}
			return nil
		})
	}
	return nil
}

func currentFrontApp() (string, string) {
	nameOut, nameErr := exec.Command("/usr/bin/osascript", "-e", `tell application "System Events" to name of first application process whose frontmost is true`).Output()
	idOut, idErr := exec.Command("/usr/bin/osascript", "-e", `tell application "System Events" to bundle identifier of first application process whose frontmost is true`).Output()
	if nameErr != nil && idErr != nil {
		return "", ""
	}
	return strings.TrimSpace(string(idOut)), strings.TrimSpace(string(nameOut))
}

func restoreFrontApp(appID, appName string) error {
	if strings.TrimSpace(appName) == "" {
		return nil
	}
	script := `on run argv
set appId to item 1 of argv
set appName to item 2 of argv
delay 0.2
if appId is not "" then
  try
    tell application id appId to activate
  end try
  try
    tell application "System Events"
      set frontmost of first application process whose bundle identifier is appId to true
    end tell
    return
  end try
end if
if appName is not "" then
  try
    tell application appName to activate
  end try
  try
    tell application "System Events"
      set frontmost of first application process whose name is appName to true
    end tell
  end try
end if
end run`
	cmd := exec.Command("/usr/bin/osascript", "-e", script, appID, appName)
	return cmd.Run()
}

func refreshChromeForFeature(featurePath string, preserveFocus bool) error {
	cfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.URL) == "" || strings.TrimSpace(cfg.Device) != "web-server" {
		return nil
	}
	script := `on run argv
set targetUrl to item 1 of argv
set portText to item 2 of argv
set windowText to item 3 of argv
set tabText to item 4 of argv
set preserveFocus to item 5 of argv

tell application "Google Chrome"
  if windowText is not "" and tabText is not "" then
    try
		set targetWindow to window (windowText as integer)
		set targetTab to tab (tabText as integer) of targetWindow
		set tabUrl to URL of targetTab
		if preserveFocus is "true" and tabUrl is not "chrome-error://chromewebdata/" then
		  tell targetTab to execute javascript "window.location.reload()"
		else
		  set URL of targetTab to targetUrl
		end if
      if preserveFocus is not "true" then
        try
          set active tab index of targetWindow to (tabText as integer)
        end try
      end if
      return
    end try
  end if
  repeat with w in windows
    if mode of w is "normal" then
      repeat with t in tabs of w
	      set tabUrl to URL of t
	      set tabTitle to title of t
	      if (tabUrl starts with targetUrl) or (portText is not "" and tabTitle contains ("localhost:" & portText)) or (tabUrl is "chrome-error://chromewebdata/" and active tab index of w is (index of t)) then
	        if preserveFocus is "true" and tabUrl is not "chrome-error://chromewebdata/" then
	          tell t to execute javascript "window.location.reload()"
	        else
	          set URL of t to targetUrl
	        end if
          if preserveFocus is not "true" then
            set active tab index of w to (index of t)
          end if
          return
        end if
      end repeat
    end if
  end repeat
end tell
end run`
	cmd := exec.Command("/usr/bin/osascript", "-e", script, cfg.URL, strconv.Itoa(cfg.Port), strconv.Itoa(cfg.ChromeWindow), strconv.Itoa(cfg.ChromeTab), strconv.FormatBool(preserveFocus))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

type chromePreferences struct {
	Browser struct {
		AllowJavaScriptAppleEvents bool `json:"allow_javascript_apple_events"`
	} `json:"browser"`
}

func chromePreferencesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Preferences"), nil
}

func chromeAppleEventsEnabled() (bool, error) {
	if runtime.GOOS != "darwin" {
		return true, nil
	}
	path, err := chromePreferencesPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var prefs chromePreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return false, err
	}
	return prefs.Browser.AllowJavaScriptAppleEvents, nil
}

func ensureChromeAppleEventsEnabled() error {
	enabled, err := chromeAppleEventsEnabled()
	if err != nil {
		return fmt.Errorf("unable to verify Chrome Apple Events JavaScript permission: %w", err)
	}
	if enabled {
		return nil
	}
	return fmt.Errorf("Chrome 'Allow JavaScript from Apple Events' is disabled; enable it in Chrome View > Developer > Allow JavaScript from Apple Events before starting a web-server agent")
}

func runAppleScript(script string, args ...string) (string, error) {
	cmdArgs := append([]string{"-e", script}, args...)
	cmd := exec.Command("/usr/bin/osascript", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return string(out), nil
}

func focusChromeTab(url string, openIfMissing bool) error {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	script := `on run argv
set targetUrl to item 1 of argv
set shouldOpen to item 2 of argv
tell application "System Events"
  set frontAppName to ""
  set frontAppID to ""
  try
    set frontAppName to name of first application process whose frontmost is true
    set frontAppID to bundle identifier of first application process whose frontmost is true
  end try
end tell
tell application "Google Chrome"
  set matchedTab to missing value
  set matchedWindow to missing value
  set matchedIndex to 0
  repeat with w in windows
    if mode of w is "normal" then
    set i to 1
    repeat with t in tabs of w
      if (URL of t starts with targetUrl) then
        set matchedTab to t
        set matchedWindow to w
        set matchedIndex to i
        exit repeat
      end if
      set i to i + 1
    end repeat
    end if
    if matchedTab is not missing value then exit repeat
  end repeat
  if matchedTab is not missing value then
    set active tab index of matchedWindow to matchedIndex
  else if shouldOpen is "true" then
    set targetWindow to missing value
    repeat with w in windows
      if mode of w is "normal" then
        set targetWindow to w
        exit repeat
      end if
    end repeat
    if targetWindow is missing value then
      make new window
      set targetWindow to window 1
    end if
    tell targetWindow
      make new tab with properties {URL:targetUrl}
      set active tab index to (count of tabs)
    end tell
    if frontAppName is not "" and frontAppName is not "Google Chrome" then
      if frontAppID is not "" then
        try
          tell application id frontAppID to activate
          return
        end try
      end if
      try
        tell application frontAppName to activate
      end try
    end if
  end if
end tell
end run`
	cmd := exec.Command("osascript", "-e", script, url, strconv.FormatBool(openIfMissing))
	return cmd.Run()
}

func closeChromeTab(url string) error {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	script := `on run argv
set targetUrl to item 1 of argv
tell application "Google Chrome"
  if not running then return
  repeat with w in windows
    set i to (count of tabs of w)
    repeat while i > 0
      set t to tab i of w
      if (URL of t starts with targetUrl) then close t
      set i to i - 1
    end repeat
  end repeat
end tell
end run`
	cmd := exec.Command("osascript", "-e", script, url)
	return cmd.Run()
}
