package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runHotReloadCommand(args []string) error {
	fs := flag.NewFlagSet("agent hot-reload", flag.ContinueOnError)
	var workspace string
	var skipAnalyze bool
	fs.StringVar(&workspace, "workspace", "", "workspace root containing agent.json")
	fs.BoolVar(&skipAnalyze, "skip-analyze", false, "")
	fs.SetOutput(nil)
	if err := fs.Parse(args); err != nil {
		return err
	}
	workspaceRoot, featurePath, err := resolveBrowserWorkspace(workspace)
	if err != nil {
		return err
	}
	return hotReload(workspaceRoot, featurePath, skipAnalyze)
}

func hotReload(workspaceRoot, featurePath string, skipAnalyze bool) error {
	cfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		return err
	}
	repoDir := filepath.Join(workspaceRoot, "repo")
	device := strings.TrimSpace(cfg.Device)
	if device == "" {
		return fmt.Errorf("no launch device selected")
	}
	logfile := filepath.Join(workspaceRoot, "logs", fmt.Sprintf("flutter-%d.log", cfg.Port))

	if !skipAnalyze {
		analyzeOutput, err := runFlutterAnalyze(repoDir)
		filtered := filterAnalyzeOutput(analyzeOutput)
		if strings.TrimSpace(filtered) != "" {
			fmt.Println(filtered)
		}
		if err != nil {
			if strings.TrimSpace(filtered) == "" {
				fmt.Print(analyzeOutput)
			}
			return fmt.Errorf("analysis failed")
		}
	}

	if !flutterServerReady(logfile) {
		return fmt.Errorf("flutter server not ready")
	}

	paneID, err := findFlutterPane(repoDir, workspaceRoot)
	if err != nil {
		return err
	}

	linesBefore := countLines(logfile)

	_ = browserClearLogsForFeature(featurePath)

	if err := runTmux("send-keys", "-t", paneID, "r"); err != nil {
		return fmt.Errorf("failed to send hot reload key: %w", err)
	}

	result := monitorHotReload(logfile, linesBefore, device, featurePath)
	fmt.Println(result)
	return nil
}

func runFlutterAnalyze(repoDir string) (string, error) {
	cmd := exec.Command("flutter", "analyze", "lib", "--no-fatal-infos", "--no-fatal-warnings")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func filterAnalyzeOutput(output string) string {
	lines := strings.Split(output, "\n")
	var filtered []string
	found := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Analyzing") {
			found = true
		}
		if found {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func flutterServerReady(logfile string) bool {
	data, err := os.ReadFile(logfile)
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	patterns := []string{
		"flutter run key commands.",
		"is being served at",
		"serving at",
		"lib/main.dart is being served",
	}
	for _, p := range patterns {
		if strings.Contains(content, p) {
			return true
		}
	}
	return false
}

func findFlutterPane(repoDir, workspaceDir string) (string, error) {
	repoDir = strings.TrimSuffix(filepath.Clean(repoDir), string(filepath.Separator))
	workspaceDir = strings.TrimSuffix(filepath.Clean(workspaceDir), string(filepath.Separator))
	output, err := runTmuxOutput("list-panes", "-a", "-F", "#{pane_id} #{pane_pid}")
	if err != nil {
		return "", fmt.Errorf("failed to list tmux panes")
	}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		paneID, panePID := parts[0], parts[1]
		flutterPID := findFlutterRunPID(panePID, 0)
		if flutterPID == "" {
			continue
		}
		cwd := processCWD(flutterPID)
		cwd = strings.TrimSuffix(filepath.Clean(cwd), string(filepath.Separator))
		if cwd == repoDir || cwd == workspaceDir {
			return paneID, nil
		}
	}
	return "", fmt.Errorf("flutter pane not found")
}

func findFlutterRunPID(parentPID string, depth int) string {
	if depth > 12 {
		return ""
	}
	out, err := exec.Command("pgrep", "-P", parentPID).Output()
	if err != nil {
		return ""
	}
	for _, child := range strings.Fields(string(out)) {
		cmd := processCommand(child)
		if strings.Contains(cmd, "flutter_tools.snapshot") && strings.Contains(cmd, "run") {
			return child
		}
		if found := findFlutterRunPID(child, depth+1); found != "" {
			return found
		}
	}
	return ""
}

func processCommand(pid string) string {
	out, err := exec.Command("ps", "-p", pid, "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func processCWD(pid string) string {
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-p", pid).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "\n") + 1
}

func monitorHotReload(logfile string, linesBefore int, device string, featurePath string) string {
	for i := 0; i < 100; i++ {
		time.Sleep(300 * time.Millisecond)
		newLines, err := readLinesAfter(logfile, linesBefore)
		if err != nil {
			continue
		}
		lower := strings.ToLower(newLines)
		if strings.Contains(lower, "reloaded ") && (strings.Contains(lower, " libraries") || strings.Contains(lower, " of ") || strings.Contains(lower, "application in")) {
			return "Reloaded the application"
		}
		if strings.Contains(lower, "restarted ") && (strings.Contains(lower, "application in") || strings.Contains(lower, "libraries")) {
			return "Restarted the application"
		}
		if strings.Contains(lower, "page requires refresh") {
			_ = syncChromeForFeature(featurePath, true)
			_ = refreshChromeForFeature(featurePath)
			return "Page refreshed"
		}
		if strings.Contains(lower, "no client connected") || strings.Contains(lower, "no connected devices") || strings.Contains(lower, "hot reload rejected") {
			if device == "web-server" {
				_ = syncChromeForFeature(featurePath, true)
				_ = refreshChromeForFeature(featurePath)
			}
			return "Reconnected browser"
		}
	}
	return "Timeout waiting for reload confirmation"
}

func readLinesAfter(path string, startLine int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if startLine >= len(lines) {
		return "", nil
	}
	return strings.Join(lines[startLine:], "\n"), nil
}
