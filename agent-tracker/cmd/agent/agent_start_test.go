package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCheckoutBranch(t *testing.T, repo, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b %s: %v: %s", branch, err, strings.TrimSpace(string(output)))
	}
}

func TestResolveStartSourceBranchUsesCurrentLocalBranch(t *testing.T) {
	repo := t.TempDir()
	initTestGitRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	gitAddPath(t, repo, "README.md")
	gitCommitAll(t, repo, "initial commit")
	gitCheckoutBranch(t, repo, "release")

	branch := resolveStartSourceBranch(repo, &repoConfig{BaseBranch: "main"})
	if branch != "release" {
		t.Fatalf("expected current local branch release, got %q", branch)
	}
}

func TestResolveBootstrapStartOptionsPrefersRecordedValues(t *testing.T) {
	options := resolveBootstrapStartOptions("/tmp/repo", &repoConfig{BaseBranch: "main"}, &agentRecord{SourceBranch: "release", KeepWorktree: true})
	if options.SourceBranch != "release" {
		t.Fatalf("expected recorded source branch release, got %q", options.SourceBranch)
	}
	if !options.KeepWorktree {
		t.Fatalf("expected keep-worktree to stay enabled")
	}

	options = resolveBootstrapStartOptions("/tmp/repo", &repoConfig{BaseBranch: "main"}, nil)
	if options.SourceBranch != "main" {
		t.Fatalf("expected repo config base branch main, got %q", options.SourceBranch)
	}
	if options.KeepWorktree {
		t.Fatalf("expected keep-worktree to default off")
	}
}

func TestAgentRunPaneCommandLeavesFlutterPaneIdleWithoutDevice(t *testing.T) {
	record := &agentRecord{WorkspaceRoot: "/tmp/demo", Runtime: "flutter", Device: ""}
	cmd := agentRunPaneCommand(record)
	if strings.Contains(cmd, "./ensure-server.sh") {
		t.Fatalf("expected empty-device flutter run pane to stay idle, got %q", cmd)
	}
	if !strings.Contains(cmd, "exec ${SHELL:-/bin/zsh}") {
		t.Fatalf("expected empty-device flutter run pane to open a shell, got %q", cmd)
	}

	record.Device = "web-server"
	cmd = agentRunPaneCommand(record)
	if !strings.Contains(cmd, "./ensure-server.sh") {
		t.Fatalf("expected configured flutter device to auto-start, got %q", cmd)
	}
}

func TestLoadFeatureConfigDefaultsMissingFlutterDeviceToWebServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	payload := map[string]any{"feature": "demo", "is_flutter": true}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadFeatureConfig(path)
	if err != nil {
		t.Fatalf("load feature config: %v", err)
	}
	if cfg.Device != defaultManagedDeviceID {
		t.Fatalf("expected missing flutter device to default to %q, got %q", defaultManagedDeviceID, cfg.Device)
	}
}

func TestSaveFeatureConfigPreservesExplicitEmptyDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := saveFeatureConfig(path, featureConfig{Feature: "demo", Device: "", IsFlutter: true}); err != nil {
		t.Fatalf("save feature config: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read feature config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "\"device\": \"\"") {
		t.Fatalf("expected explicit empty device to be persisted, got %q", text)
	}
}

func TestRunFeatureCommandSyncsRegistryDeviceAndBrowserState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workspace := filepath.Join(home, "repo", ".agents", "demo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	featurePath := filepath.Join(workspace, "agent.json")
	if err := saveFeatureConfig(featurePath, featureConfig{Feature: "demo", Device: "ipm", IsFlutter: true}); err != nil {
		t.Fatalf("save feature config: %v", err)
	}
	reg := &registry{Agents: map[string]*agentRecord{
		"demo": {
			ID:             "demo",
			WorkspaceRoot:  workspace,
			FeatureConfig:  featurePath,
			Device:         "ipm",
			BrowserEnabled: false,
		},
	}}
	if err := saveRegistry(reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	if err := runFeatureCommand([]string{"--workspace", workspace, "--device", "web-server"}); err != nil {
		t.Fatalf("run feature command: %v", err)
	}

	updatedCfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		t.Fatalf("load feature config: %v", err)
	}
	if updatedCfg.Device != "web-server" {
		t.Fatalf("expected feature config device web-server, got %q", updatedCfg.Device)
	}
	updatedReg, err := loadRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	record := updatedReg.Agents["demo"]
	if record == nil {
		t.Fatal("expected registry record")
	}
	if record.Device != "web-server" {
		t.Fatalf("expected registry device web-server, got %q", record.Device)
	}
	if !record.BrowserEnabled {
		t.Fatal("expected browser to be enabled for web-server device")
	}
}

func TestRunDestroyRequiresConfirmWhenRepoHasUncommittedChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workspace := filepath.Join(home, "repo", ".agents", "demo")
	repoCopy := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(repoCopy, 0o755); err != nil {
		t.Fatalf("mkdir repo copy: %v", err)
	}
	initTestGitRepo(t, repoCopy)
	if err := os.WriteFile(filepath.Join(repoCopy, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty repo file: %v", err)
	}
	reg := &registry{Agents: map[string]*agentRecord{
		"demo": {
			ID:            "demo",
			WorkspaceRoot: workspace,
			RepoCopyPath:  repoCopy,
		},
	}}
	if err := saveRegistry(reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	err := runDestroy([]string{"--id", "demo"})
	if err == nil || !strings.Contains(err.Error(), "--confirm destroy") {
		t.Fatalf("expected destroy confirm error, got %v", err)
	}
}
