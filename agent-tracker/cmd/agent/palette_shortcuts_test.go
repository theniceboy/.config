package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func setupPromptRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".agent.yaml"), []byte("base_branch: main\n"), 0o644); err != nil {
		t.Fatalf("write .agent.yaml: %v", err)
	}
	return repo
}

func TestPaletteAltCOpensStartAgentPrompt(t *testing.T) {
	repo := setupPromptRepo(t)
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})

	updated, cmd := model.updateList("alt+c")
	if cmd != nil {
		t.Fatalf("expected no command when opening start prompt, got %v", cmd)
	}

	palette, ok := updated.(*paletteModel)
	if !ok {
		t.Fatalf("expected palette model, got %T", updated)
	}
	if palette.state.Mode != paletteModePrompt {
		t.Fatalf("expected prompt mode, got %v", palette.state.Mode)
	}
	if palette.state.PromptKind != palettePromptStartAgent {
		t.Fatalf("expected start-agent prompt, got %v", palette.state.PromptKind)
	}
	if palette.state.PromptRepoRoot != repo {
		t.Fatalf("expected prompt repo root %q, got %q", repo, palette.state.PromptRepoRoot)
	}
}

func TestPaletteStartPromptAddsNoDeviceOptionForFlutterRepo(t *testing.T) {
	repo := setupPromptRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatalf("write pubspec: %v", err)
	}
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)

	if len(model.state.PromptDevices) < 2 {
		t.Fatalf("expected flutter prompt devices to include no-device option and launch devices, got %v", model.state.PromptDevices)
	}
	if model.state.PromptDevices[0] != paletteNoDeviceOption {
		t.Fatalf("expected first prompt device to be no-device option, got %q", model.state.PromptDevices[0])
	}
	if model.state.PromptDeviceIndex != 1 {
		t.Fatalf("expected default device selection to stay on %q, got index %d", defaultManagedDeviceID, model.state.PromptDeviceIndex)
	}
}

func TestPaletteStartPromptDeviceSelectionUsesNI(t *testing.T) {
	repo := setupPromptRepo(t)
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)
	model.state.PromptField = palettePromptFieldDevice
	model.state.PromptDevices = []string{"ios", "android", "web-server"}
	model.state.PromptDeviceIndex = 1

	updated, _ := model.updatePrompt("n")
	palette := updated.(*paletteModel)
	if palette.state.PromptDeviceIndex != 0 {
		t.Fatalf("expected n to move device selection left, got %d", palette.state.PromptDeviceIndex)
	}

	updated, _ = palette.updatePrompt("i")
	palette = updated.(*paletteModel)
	if palette.state.PromptDeviceIndex != 1 {
		t.Fatalf("expected i to move device selection right, got %d", palette.state.PromptDeviceIndex)
	}

	updated, _ = palette.updatePrompt("e")
	palette = updated.(*paletteModel)
	if palette.state.PromptDeviceIndex != 1 {
		t.Fatalf("expected e to leave device selection unchanged, got %d", palette.state.PromptDeviceIndex)
	}
}

func TestPaletteStartPromptAltDTogglesDevices(t *testing.T) {
	repo := setupPromptRepo(t)
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)
	model.state.PromptDevices = []string{"ios", "android", "web-server"}
	model.state.PromptDeviceIndex = 1
	model.state.PromptField = palettePromptFieldName

	updated, _ := model.updatePrompt("alt+d")
	palette := updated.(*paletteModel)
	if palette.state.PromptField != palettePromptFieldName {
		t.Fatalf("expected alt+d to keep the current focus field, got %v", palette.state.PromptField)
	}
	if palette.state.PromptDeviceIndex != 2 {
		t.Fatalf("expected alt+d to advance to next device, got %d", palette.state.PromptDeviceIndex)
	}

	updated, _ = palette.updatePrompt("alt+d")
	palette = updated.(*paletteModel)
	if palette.state.PromptDeviceIndex != 0 {
		t.Fatalf("expected alt+d to wrap device selection, got %d", palette.state.PromptDeviceIndex)
	}

	updated, _ = palette.updatePrompt("alt+D")
	palette = updated.(*paletteModel)
	if palette.state.PromptDeviceIndex != 2 {
		t.Fatalf("expected alt+shift+d to move device selection backward, got %d", palette.state.PromptDeviceIndex)
	}
}

func TestRenderPaletteFooterTogglesAltHints(t *testing.T) {
	styles := newPaletteStyles()
	defaultFooter := renderPaletteFooter(styles, 140, "", false)
	if !strings.Contains(defaultFooter, "more") {
		t.Fatalf("expected default footer to advertise more options, got %q", defaultFooter)
	}
	if !strings.Contains(defaultFooter, footerHintToggleKey) {
		t.Fatalf("expected default footer to show %q toggle, got %q", footerHintToggleKey, defaultFooter)
	}
	if strings.Contains(defaultFooter, "create") {
		t.Fatalf("expected default footer to hide alt shortcuts, got %q", defaultFooter)
	}

	altFooter := renderPaletteFooter(styles, 140, "", true)
	if !strings.Contains(altFooter, "create") || !strings.Contains(altFooter, "tracker") {
		t.Fatalf("expected alt footer to show alt shortcuts, got %q", altFooter)
	}
	if strings.Contains(altFooter, "filter") || strings.Contains(altFooter, "Enter") {
		t.Fatalf("expected alt footer to hide default shortcuts, got %q", altFooter)
	}
}

func TestPalettePromptAltToggleShowsAltHints(t *testing.T) {
	repo := setupPromptRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "pubspec.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatalf("write pubspec: %v", err)
	}
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)

	defaultView := model.renderPrompt(newPaletteStyles(), 100, 24)
	if !strings.Contains(defaultView, footerHintToggleKey) || !strings.Contains(defaultView, "more") {
		t.Fatalf("expected default prompt to advertise alt options, got %q", defaultView)
	}
	if strings.Contains(defaultView, "Alt-D") {
		t.Fatalf("expected default prompt to hide alt device shortcuts, got %q", defaultView)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	palette := updated.(*paletteModel)
	altView := palette.renderPrompt(newPaletteStyles(), 100, 24)
	if !strings.Contains(altView, "Alt-D") || !strings.Contains(altView, "Alt-S") {
		t.Fatalf("expected alt prompt view to show alt shortcuts, got %q", altView)
	}
	if strings.Contains(altView, "Tab") || strings.Contains(altView, "n/i") {
		t.Fatalf("expected alt prompt view to hide default shortcuts, got %q", altView)
	}

	updated, _ = palette.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	palette = updated.(*paletteModel)
	if palette.state.ShowAltHints {
		t.Fatalf("expected normal keypress to hide alt hints")
	}
}

func TestPaletteStartPromptWorktreeToggleUsesNI(t *testing.T) {
	repo := setupPromptRepo(t)
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)
	model.state.PromptField = palettePromptFieldWorktree

	updated, _ := model.updatePrompt("i")
	palette := updated.(*paletteModel)
	if !palette.state.PromptKeepWorktree {
		t.Fatalf("expected i to switch worktree mode to keep")
	}

	updated, _ = palette.updatePrompt("n")
	palette = updated.(*paletteModel)
	if palette.state.PromptKeepWorktree {
		t.Fatalf("expected n to switch worktree mode back to clear")
	}
}

func TestPaletteStartPromptEnterCarriesKeepWorktree(t *testing.T) {
	repo := setupPromptRepo(t)
	model := newPaletteModel(&paletteRuntime{mainRepoRoot: repo}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", repo)
	model.state.PromptKeepWorktree = true

	updated, cmd := model.updatePrompt("enter")
	if cmd == nil {
		t.Fatalf("expected enter to quit palette for action execution")
	}
	palette := updated.(*paletteModel)
	if !palette.result.KeepWorktree {
		t.Fatalf("expected start-agent result to preserve keep-worktree selection")
	}
}

func TestPaletteStartPromptHidesCreateOptionsWithoutRepo(t *testing.T) {
	model := newPaletteModel(&paletteRuntime{}, paletteUIState{Mode: paletteModeList})
	model.openPrompt(palettePromptStartAgent, "feature-x", "")
	view := model.renderPrompt(newPaletteStyles(), 80, 20)

	if !strings.Contains(view, "Main repo not found") {
		t.Fatalf("expected missing repo message in prompt: %q", view)
	}
	for _, unwanted := range []string{"BRANCH", "DEVICE", "WORKTREE", "Enter create", "CLEAR", "KEEP"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("did not expect %q in missing-repo prompt: %q", unwanted, view)
		}
	}

	updated, cmd := model.updatePrompt("enter")
	if cmd != nil {
		t.Fatalf("expected no command when repo is missing, got %v", cmd)
	}
	palette := updated.(*paletteModel)
	if palette.state.Mode != paletteModePrompt {
		t.Fatalf("expected prompt to remain open when repo is missing, got %v", palette.state.Mode)
	}
}

func TestRenderPaletteDeviceChipKeepsPaddingWhenActive(t *testing.T) {
	styles := newPaletteStyles()
	inactive := renderPaletteDeviceChip(styles, "ios", false)
	active := renderPaletteDeviceChip(styles, "ios", true)

	if lipgloss.Width(active) != lipgloss.Width(inactive) {
		t.Fatalf("expected active chip width %d to match inactive chip width %d", lipgloss.Width(active), lipgloss.Width(inactive))
	}
}

func TestBuildAgentStartArgsIncludesKeepWorktree(t *testing.T) {
	got := buildAgentStartArgs("feature-x", "ios", true)
	want := []string{"start", "--keep-worktree", "-d", "ios", "feature-x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected args %v, got %v", want, got)
	}

	got = buildAgentStartArgs("feature-x", "", false)
	want = []string{"start", "feature-x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected args %v, got %v", want, got)
	}

	got = buildAgentStartArgs("feature-x", paletteNoDeviceOption, false)
	want = []string{"start", "--no-device", "feature-x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected args %v, got %v", want, got)
	}
}

func TestPaletteDestroyQueuesAndClosesImmediately(t *testing.T) {
	prev := paletteDestroyLauncher
	t.Cleanup(func() { paletteDestroyLauncher = prev })

	calledWith := ""
	calledConfirm := ""
	paletteDestroyLauncher = func(agentID string, confirmText string) error {
		calledWith = agentID
		calledConfirm = confirmText
		return nil
	}

	runtime := &paletteRuntime{agentID: "feature-x"}
	reopen, message, err := runtime.execute(paletteResult{Action: paletteAction{Kind: paletteActionConfirmDestroy}})
	if err != nil {
		t.Fatalf("execute destroy: %v", err)
	}
	if reopen {
		t.Fatalf("expected destroy action to close the palette immediately")
	}
	if message != "" {
		t.Fatalf("expected no destroy message, got %q", message)
	}
	if calledWith != "feature-x" {
		t.Fatalf("expected destroy launcher to receive feature-x, got %q", calledWith)
	}
	if calledConfirm != "" {
		t.Fatalf("expected empty destroy confirm text, got %q", calledConfirm)
	}
}

func TestPaletteDestroyRequiresTypedDestroyForDirtyRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryDir := filepath.Join(home, ".config", "agent-tracker", "run")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("mkdir registry dir: %v", err)
	}
	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	initTestGitRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty repo file: %v", err)
	}
	reg := &registry{Agents: map[string]*agentRecord{
		"feature-x": {ID: "feature-x", RepoCopyPath: repo},
	}}
	if err := saveRegistry(reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}

	model := newPaletteModel(&paletteRuntime{agentID: "feature-x", record: &agentRecord{ID: "feature-x", RepoCopyPath: repo}}, paletteUIState{Mode: paletteModeList})
	updated, cmd := model.selectAction(paletteAction{Kind: paletteActionConfirmDestroy})
	if cmd != nil {
		t.Fatalf("expected no quit when opening destroy confirm, got %v", cmd)
	}
	palette := updated.(*paletteModel)
	if !palette.state.ConfirmRequiresText {
		t.Fatal("expected typed destroy confirmation for dirty repo")
	}

	updated, cmd = palette.updateConfirm("enter")
	if cmd != nil {
		t.Fatalf("expected no quit for missing typed confirmation, got %v", cmd)
	}
	palette = updated.(*paletteModel)
	if palette.state.Message != "Type destroy to confirm" {
		t.Fatalf("expected typed destroy message, got %q", palette.state.Message)
	}

	for _, key := range []string{"d", "e", "s", "t", "r", "o", "y"} {
		updated, _ = palette.updateConfirm(key)
		palette = updated.(*paletteModel)
	}
	updated, cmd = palette.updateConfirm("enter")
	if cmd == nil {
		t.Fatal("expected quit after typed destroy confirmation")
	}
	palette = updated.(*paletteModel)
	if palette.result.Input != "destroy" {
		t.Fatalf("expected destroy confirm input, got %q", palette.result.Input)
	}
}

func TestPaletteBuildActionsIncludesMemoryToggle(t *testing.T) {
	prevOutput := paletteTmuxOutput
	t.Cleanup(func() { paletteTmuxOutput = prevOutput })

	paletteTmuxOutput = func(args ...string) (string, error) {
		return "TMUX_STATUS_MEMORY=0\n", nil
	}

	actions := (&paletteRuntime{mainRepoRoot: "/tmp/repo"}).buildActions()
	for _, action := range actions {
		if action.Kind != paletteActionToggleMemoryDisplay {
			continue
		}
		if action.Title != "Show memory display" {
			t.Fatalf("expected show title, got %q", action.Title)
		}
		return
	}
	t.Fatalf("memory toggle action missing: %#v", actions)
}

func TestPaletteToggleMemoryDisplayTurnsItOffAndRefreshesTmux(t *testing.T) {
	prevOutput := paletteTmuxOutput
	prevRunner := paletteTmuxRunner
	t.Cleanup(func() {
		paletteTmuxOutput = prevOutput
		paletteTmuxRunner = prevRunner
	})

	paletteTmuxOutput = func(args ...string) (string, error) {
		return "TMUX_STATUS_MEMORY=1\n", nil
	}
	var calls []string
	paletteTmuxRunner = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}

	runtime := &paletteRuntime{}
	_, _, err := runtime.execute(paletteResult{Action: paletteAction{Kind: paletteActionToggleMemoryDisplay}})
	if err != nil {
		t.Fatalf("execute memory toggle: %v", err)
	}
	want := []string{
		"set-environment -g TMUX_STATUS_MEMORY 0",
		"refresh-client -S",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
}

func TestPaletteIgnoresImmediateAltSCloseOnOpen(t *testing.T) {
	model := newPaletteModel(&paletteRuntime{}, paletteUIState{Mode: paletteModeList})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}, Alt: true})
	if cmd != nil {
		t.Fatalf("expected no quit command for immediate alt+s, got %v", cmd)
	}
	palette := updated.(*paletteModel)
	if palette.state.Mode != paletteModeList {
		t.Fatalf("expected immediate alt+s to leave palette open, got mode %v", palette.state.Mode)
	}

	palette.openedAt = time.Now().Add(-time.Second)
	updated, cmd = palette.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}, Alt: true})
	if cmd == nil {
		t.Fatalf("expected delayed alt+s to close the palette")
	}
	palette = updated.(*paletteModel)
	if palette.result.Kind != paletteResultClose {
		t.Fatalf("expected delayed alt+s to close the palette")
	}
}

func TestLoadPaletteRuntimeSurvivesMalformedRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryDir := filepath.Join(home, ".config", "agent-tracker", "run")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("mkdir registry dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(registryDir, "agents.json"), []byte("{}\n}"), 0o644); err != nil {
		t.Fatalf("write malformed registry: %v", err)
	}

	runtime, err := loadPaletteRuntime([]string{"--window=@1", "--path=" + home, "--session-name=test", "--window-name=shell"})
	if err != nil {
		t.Fatalf("loadPaletteRuntime returned error: %v", err)
	}
	if runtime.reg == nil || runtime.reg.Agents == nil {
		t.Fatalf("expected fallback empty registry, got %#v", runtime.reg)
	}
	if got := runtime.startupMessage; !strings.Contains(got, "Ignoring malformed registry") {
		t.Fatalf("expected startup warning, got %q", got)
	}

	model := newPaletteModel(runtime, paletteUIState{Mode: paletteModeList, Message: runtime.startupMessage})
	view := model.View()
	if !strings.Contains(view, "Ignoring malformed registry") {
		t.Fatalf("expected warning in palette view, got %s", fmt.Sprintf("%q", view))
	}
	if !strings.Contains(view, "Command Palette") {
		t.Fatalf("expected palette to still render, got %s", fmt.Sprintf("%q", view))
	}
}
