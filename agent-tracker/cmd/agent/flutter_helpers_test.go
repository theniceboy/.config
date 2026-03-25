package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTestGitRepo(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func gitAddPath(t *testing.T, repo, relPath string) {
	t.Helper()
	cmd := exec.Command("git", "add", "--", relPath)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %s: %v: %s", relPath, err, strings.TrimSpace(string(output)))
	}
}

func gitCommitAll(t *testing.T, repo, message string) {
	t.Helper()
	cmd := exec.Command("git", "-c", "user.name=Test User", "-c", "user.email=test@example.com", "commit", "-m", message)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func gitStatusPath(t *testing.T, repo, relPath string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--", relPath)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status %s: %v: %s", relPath, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func gitLsFilesPath(t *testing.T, repo, relPath string) string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-v", "--", relPath)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files %s: %v: %s", relPath, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func TestWriteFlutterHelperScriptsCreatesHotReloadScript(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initTestGitRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "hot-reload.sh"), []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("seed tracked hot-reload.sh: %v", err)
	}
	gitAddPath(t, repo, "hot-reload.sh")
	gitCommitAll(t, repo, "seed hot reload")
	if err := writeFlutterHelperScripts(workspace, repo, "http://localhost:9100", "web-server"); err != nil {
		t.Fatalf("write flutter helper scripts: %v", err)
	}

	for _, path := range []string{filepath.Join(workspace, "ensure-server.sh"), filepath.Join(repo, "hot-reload.sh")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected %s to be executable, mode=%v", path, info.Mode())
		}
	}

	if _, err := os.Stat(filepath.Join(workspace, "hot-reload.sh")); !os.IsNotExist(err) {
		t.Fatalf("expected workspace hot-reload.sh to be removed, err=%v", err)
	}

	ensureData, err := os.ReadFile(filepath.Join(workspace, "ensure-server.sh"))
	if err != nil {
		t.Fatalf("read ensure-server.sh: %v", err)
	}
	ensureText := string(ensureData)
	for _, snippet := range []string{
		`tmux capture-pane -p -S -200 -t "$TMUX_PANE"`,
		`browser refresh --workspace "$DIR" --preserve-focus`,
	} {
		if !strings.Contains(ensureText, snippet) {
			t.Fatalf("expected ensure-server.sh to contain %q", snippet)
		}
	}

	data, err := os.ReadFile(filepath.Join(repo, "hot-reload.sh"))
	if err != nil {
		t.Fatalf("read hot-reload.sh: %v", err)
	}
	text := string(data)
	for _, snippet := range []string{
		`WORKSPACE_DIR="$(dirname "$REPO_DIR")"`,
		`flutter analyze lib --no-fatal-infos --no-fatal-warnings`,
		`tmux send-keys -t "$target_pane" r`,
		`cd '$WORKSPACE_DIR' && ./ensure-server.sh`,
		`browser refresh --workspace "$WORKSPACE_DIR" --preserve-focus`,
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected hot-reload.sh to contain %q", snippet)
		}
	}

	if status := gitStatusPath(t, repo, "hot-reload.sh"); status != "" {
		t.Fatalf("expected repo hot-reload.sh to stay hidden from git status, got %q", status)
	}
}

func TestWriteFlutterHelperScriptsRemovesLegacyBrowserHelpers(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initTestGitRepo(t, repo)
	for _, name := range []string{"open-tab.sh", "refresh-tab.sh", "on-tmux-window-activate.sh"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("legacy"), 0o755); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "hot-reload.sh"), []byte("legacy"), 0o755); err != nil {
		t.Fatalf("seed workspace hot-reload.sh: %v", err)
	}

	if err := writeFlutterHelperScripts(workspace, repo, "http://localhost:9100", "web-server"); err != nil {
		t.Fatalf("write flutter helper scripts: %v", err)
	}

	for _, name := range []string{"hot-reload.sh", "open-tab.sh", "refresh-tab.sh", "on-tmux-window-activate.sh"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", name, err)
		}
	}
}

func TestRunFeatureCommandWriteHelperScripts(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initTestGitRepo(t, repo)
	if err := saveFeatureConfig(filepath.Join(workspace, "agent.json"), featureConfig{
		Feature:   "demo",
		Port:      9100,
		URL:       "http://localhost:9100",
		Device:    "web-server",
		IsFlutter: true,
	}); err != nil {
		t.Fatalf("save feature config: %v", err)
	}

	if err := runFeatureCommand([]string{"--workspace", workspace, "--write-helper-scripts"}); err != nil {
		t.Fatalf("run feature command: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "hot-reload.sh")); err != nil {
		t.Fatalf("expected hot-reload.sh after rewrite: %v", err)
	}
}

func TestWriteFlutterHelperScriptsIgnoresUntrackedRepoHelper(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initTestGitRepo(t, repo)

	if err := writeFlutterHelperScripts(workspace, repo, "http://localhost:9100", "web-server"); err != nil {
		t.Fatalf("write flutter helper scripts: %v", err)
	}

	if status := gitStatusPath(t, repo, "hot-reload.sh"); status != "" {
		t.Fatalf("expected untracked repo hot-reload.sh to stay hidden from git status, got %q", status)
	}

	data, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	if !strings.Contains(string(data), "hot-reload.sh") {
		t.Fatalf("expected .git/info/exclude to contain hot-reload.sh")
	}
}

func TestRunBootstrapRewritesTrackedHotReloadScript(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoRoot := t.TempDir()
	initTestGitRepo(t, repoRoot)
	if err := os.WriteFile(filepath.Join(repoRoot, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatalf("write pubspec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "hot-reload.sh"), []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write source hot-reload: %v", err)
	}
	gitAddPath(t, repoRoot, "pubspec.yaml")
	gitAddPath(t, repoRoot, "hot-reload.sh")
	gitCommitAll(t, repoRoot, "seed flutter repo")

	workspace := filepath.Join(repoRoot, ".agents", "feature-x")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := saveFeatureConfig(filepath.Join(workspace, "agent.json"), featureConfig{
		Feature:   "feature-x",
		Port:      9100,
		URL:       "http://localhost:9100",
		Device:    "web-server",
		IsFlutter: true,
	}); err != nil {
		t.Fatalf("save feature config: %v", err)
	}

	if err := runBootstrap([]string{"--workspace", workspace}); err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}

	repoCopy := filepath.Join(workspace, "repo")
	data, err := os.ReadFile(filepath.Join(repoCopy, "hot-reload.sh"))
	if err != nil {
		t.Fatalf("read rewritten hot-reload: %v", err)
	}
	text := string(data)
	for _, snippet := range []string{
		`INFO="$WORKSPACE_DIR/agent.json"`,
		`browser refresh --workspace "$WORKSPACE_DIR" --preserve-focus`,
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected rewritten hot-reload to contain %q", snippet)
		}
	}
	if status := gitStatusPath(t, repoCopy, "hot-reload.sh"); status != "" {
		t.Fatalf("expected rewritten hot-reload to stay hidden from git status, got %q", status)
	}
	if marker := gitLsFilesPath(t, repoCopy, "hot-reload.sh"); !strings.HasPrefix(marker, "S ") {
		t.Fatalf("expected rewritten hot-reload to be marked skip-worktree, got %q", marker)
	}
}
