package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type paletteMode int

const (
	paletteModeList paletteMode = iota
	paletteModePrompt
	paletteModeConfirmDestroy
	paletteModeSnippets
	paletteModeSnippetVars
	paletteModeTodos
	paletteModeActivity
	paletteModeDevices
	paletteModeStatusRight
	paletteModeTracker
)

type palettePromptField int

const (
	palettePromptFieldName palettePromptField = iota
	palettePromptFieldDevice
	palettePromptFieldWorktree
)

type palettePromptKind int

const (
	palettePromptStartAgent palettePromptKind = iota
	palettePromptSnippetVar
)

type paletteActionKind int

const (
	paletteActionPromptStartAgent paletteActionKind = iota
	paletteActionOpenActivityMonitor
	paletteActionConfirmDestroy
	paletteActionReloadTmuxConfig
	paletteActionOpenStatusRight
	paletteActionOpenSnippets
	paletteActionOpenTodos
	paletteActionOpenDevices
	paletteActionOpenTracker
)

type paletteAction struct {
	Section  string
	Title    string
	Subtitle string
	Keywords []string
	Kind     paletteActionKind
	RepoRoot string
}

type paletteResultKind int

const (
	paletteResultClose paletteResultKind = iota
	paletteResultRunAction
	paletteResultOpenActivityMonitor
	paletteResultOpenSnippets
	paletteResultOpenTodos
)

type paletteResult struct {
	Kind         paletteResultKind
	Action       paletteAction
	Input        string
	Device       string
	KeepWorktree bool
	State        paletteUIState
}

type paletteUIState struct {
	Filter              []rune
	FilterCursor        int
	Selected            int
	ActionOffset        int
	SnippetOffset       int
	Mode                paletteMode
	PromptText          []rune
	PromptCursor        int
	PromptKind          palettePromptKind
	PromptField         palettePromptField
	PromptRepoRoot      string
	PromptDevices       []string
	PromptDeviceIndex   int
	PromptKeepWorktree  bool
	ShowAltHints        bool
	Message             string
	ConfirmRequiresText bool
	SnippetName         string
	SnippetContent      string
	SnippetVars         []string
	SnippetVarIndex     int
	SnippetVarValues    map[string]string
	SnippetVarPrompts   []string
	SnippetVarPromptIdx int
}

type snippet struct {
	Name        string
	Description string
	Content     string
	Vars        []string
}

var snippetVarRegex = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

func extractSnippetVars(content string) []string {
	seen := make(map[string]bool)
	var vars []string
	for _, match := range snippetVarRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 && !seen[match[1]] {
			seen[match[1]] = true
			vars = append(vars, match[1])
		}
	}
	return vars
}

func renderSnippet(content string, values map[string]string) string {
	result := content
	for name, value := range values {
		result = strings.ReplaceAll(result, "{{"+name+"}}", value)
	}
	return result
}

func loadSnippets() []snippet {
	snippetsDir := filepath.Join(os.Getenv("HOME"), ".config", "snippets")
	entries, err := os.ReadDir(snippetsDir)
	if err != nil {
		return nil
	}
	var snippets []snippet
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		path := filepath.Join(snippetsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		lines := strings.SplitN(content, "\n", 2)
		description := ""
		body := content
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#") {
			description = strings.TrimSpace(strings.TrimPrefix(lines[0], "#"))
			if len(lines) > 1 {
				body = strings.TrimPrefix(content, lines[0]+"\n")
			} else {
				body = ""
			}
		}
		snippets = append(snippets, snippet{
			Name:        name,
			Description: description,
			Content:     strings.TrimRight(body, "\n"),
			Vars:        extractSnippetVars(body),
		})
	}
	return snippets
}

func pasteToTmuxPane(text string) error {
	return runTmux("send-keys", "-l", text)
}

func detectPaletteMainRepoRoot(currentPath string, record *agentRecord) string {
	if record != nil && strings.TrimSpace(record.RepoRoot) != "" {
		return strings.TrimSpace(record.RepoRoot)
	}
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" {
		return ""
	}
	clean := filepath.Clean(currentPath)
	needle := string(filepath.Separator) + ".agents" + string(filepath.Separator)
	if idx := strings.Index(clean, needle); idx >= 0 {
		return clean[:idx]
	}
	if fileExists(filepath.Join(clean, ".agent.yaml")) {
		return clean
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = clean
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	repoRoot := strings.TrimSpace(string(out))
	if repoRoot == "" {
		return ""
	}
	if fileExists(filepath.Join(repoRoot, ".agent.yaml")) {
		return repoRoot
	}
	if idx := strings.Index(repoRoot, needle); idx >= 0 {
		return repoRoot[:idx]
	}
	return ""
}

func detectPaletteAgentIDFromPath(currentPath string) string {
	clean := filepath.Clean(strings.TrimSpace(currentPath))
	if clean == "" {
		return ""
	}
	needle := string(filepath.Separator) + ".agents" + string(filepath.Separator)
	idx := strings.Index(clean, needle)
	if idx < 0 {
		return ""
	}
	rest := clean[idx+len(needle):]
	if rest == "" {
		return ""
	}
	parts := strings.Split(rest, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return sanitizeFeatureName(parts[0])
}

func looksLikeTmuxFormatLiteral(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "#{") && strings.Contains(value, "}")
}

func startAgentSubtitle(mainRepoRoot, currentPath string) string {
	if strings.TrimSpace(mainRepoRoot) == "" {
		return "No agent-enabled repo detected for this pane"
	}
	if filepath.Clean(strings.TrimSpace(mainRepoRoot)) == filepath.Clean(strings.TrimSpace(currentPath)) {
		return "Open a small prompt here to start a new agent"
	}
	return fmt.Sprintf("Open a small prompt in %s", mainRepoRoot)
}

func runPalette(args []string) error {
	return runBubbleTeaPalette(args)
}
