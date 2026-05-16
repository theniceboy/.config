package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const agentBrowserMCPName = "agent_browser"

type browserMCPNoInput struct{}

type browserMCPEvaluateInput struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise,omitempty"`
}

type browserMCPLogsInput struct {
	DurationSeconds int  `json:"duration_seconds,omitempty"`
	All             bool `json:"all,omitempty"`
	Limit           int  `json:"limit,omitempty"`
	MaxChars        int  `json:"max_chars,omitempty"`
}

type browserLogOptions struct {
	All      bool
	Limit    int
	MaxChars int
}

type browserScreenshotOptions struct {
	OutPath   string
	Grid      bool
	MajorStep int
	MinorStep int
}

type browserScreenshotResult struct {
	Path     string
	JPEGData []byte
	Metadata map[string]any
}

type browserMCPScreenshotInput struct {
	Grid      bool `json:"grid,omitempty"`
	MajorStep int  `json:"major_step,omitempty"`
	MinorStep int  `json:"minor_step,omitempty"`
}

type browserMCPSnapshotInput struct {
	MaxText     int `json:"max_text,omitempty"`
	MaxElements int `json:"max_elements,omitempty"`
}

type browserMCPClickInput struct {
	Ref            string   `json:"ref,omitempty"`
	Selector       string   `json:"selector,omitempty"`
	Text           string   `json:"text,omitempty"`
	X              *float64 `json:"x,omitempty"`
	Y              *float64 `json:"y,omitempty"`
	ImageX         *float64 `json:"image_x,omitempty"`
	ImageY         *float64 `json:"image_y,omitempty"`
	ImageWidth     *float64 `json:"image_width,omitempty"`
	ImageHeight    *float64 `json:"image_height,omitempty"`
	ScreenshotPath string   `json:"screenshot_path,omitempty"`
	Button         string   `json:"button,omitempty"`
	ClickCount     int      `json:"click_count,omitempty"`
}

type browserMCPInspectPointInput struct {
	X              *float64 `json:"x,omitempty"`
	Y              *float64 `json:"y,omitempty"`
	ImageX         *float64 `json:"image_x,omitempty"`
	ImageY         *float64 `json:"image_y,omitempty"`
	ImageWidth     *float64 `json:"image_width,omitempty"`
	ImageHeight    *float64 `json:"image_height,omitempty"`
	ScreenshotPath string   `json:"screenshot_path,omitempty"`
}

type browserMCPTypeInput struct {
	Ref      string `json:"ref,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text"`
	Clear    bool   `json:"clear,omitempty"`
}

type browserMCPKeyInput struct {
	Key string `json:"key"`
}

type browserMCPScrollInput struct {
	X      *float64 `json:"x,omitempty"`
	Y      *float64 `json:"y,omitempty"`
	DeltaX float64  `json:"delta_x,omitempty"`
	DeltaY float64  `json:"delta_y,omitempty"`
}

type browserRuntimeEvaluateResult struct {
	Result           browserRuntimeRemoteObject      `json:"result"`
	ExceptionDetails *browserRuntimeExceptionDetails `json:"exceptionDetails,omitempty"`
}

type browserRuntimeRemoteObject struct {
	Type        string `json:"type,omitempty"`
	Subtype     string `json:"subtype,omitempty"`
	Value       any    `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
}

type browserRuntimeExceptionDetails struct {
	Text      string `json:"text,omitempty"`
	Exception struct {
		Description string `json:"description,omitempty"`
		Value       any    `json:"value,omitempty"`
	} `json:"exception,omitempty"`
}

func runBrowserMCP(args []string) error {
	fs := flagSet("agent browser mcp")
	var workspace string
	fs.StringVar(&workspace, "workspace", "", "workspace root containing agent.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	workspaceRoot, featurePath, err := resolveBrowserWorkspace(workspace)
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: agentBrowserMCPName, Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "status",
		Description: "Return the configured Flutter web page status, including target URL, tab presence, ready state, viewport, Flutter launch flags, and the current active Chrome-for-Testing URL.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ browserMCPNoInput) (*mcp.CallToolResult, any, error) {
		status, err := browserMCPStatus(featurePath)
		return nil, status, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "open",
		Description: "Open or select this agent's configured Flutter web tab in Chrome for Testing. This may change Chrome's active tab but does not make Chrome frontmost.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ browserMCPNoInput) (*mcp.CallToolResult, any, error) {
		if err := syncChromeForFeature(featurePath, true); err != nil {
			return nil, nil, err
		}
		status, err := browserMCPStatus(featurePath)
		return nil, status, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "refresh",
		Description: "Reload this agent's configured Flutter web tab through Chrome DevTools Protocol without relying on the active browser tab.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ browserMCPNoInput) (*mcp.CallToolResult, any, error) {
		if err := refreshChromeForFeature(featurePath); err != nil {
			return nil, nil, err
		}
		status, err := browserMCPStatus(featurePath)
		return nil, status, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluate",
		Description: "Evaluate JavaScript in this agent's Flutter web tab. Use for diagnostics; it can read or mutate page state depending on the expression.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPEvaluateInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionEvaluate(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "logs",
		Description: "Collect console, exception, and browser log messages from this agent's Flutter web tab for a short duration.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPLogsInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionLogs(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "screenshot",
		Description: "Capture a compressed JPEG screenshot of this agent's Flutter web tab. The text metadata includes viewport and image dimensions; use viewport x/y with click.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPScreenshotInput) (*mcp.CallToolResult, any, error) {
		return browserMCPScreenshotResult(featurePath, input.Grid, input.MajorStep, input.MinorStep)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "screenshot_grid",
		Description: "Capture a screenshot with an overlaid coordinate grid. Grid labels are viewport CSS pixels; use those x/y values with click or inspect_point.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPScreenshotInput) (*mcp.CallToolResult, any, error) {
		return browserMCPScreenshotResult(featurePath, true, input.MajorStep, input.MinorStep)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "snapshot",
		Description: "Return page text and visible interactive DOM elements with CSS refs that can be used by click/type tools. Coordinates are viewport CSS pixels.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPSnapshotInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionSnapshot(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "click",
		Description: "Click this agent's Flutter web tab by CSS ref/selector, visible text, viewport x/y, or screenshot image_x/image_y. For image coordinates, pass screenshot_path or image_width/image_height from screenshot metadata.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPClickInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionClick(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "inspect_point",
		Description: "Inspect what is under a viewport coordinate or screenshot image coordinate. Returns elementFromPoint details and nearby text/ARIA info.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPInspectPointInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionInspectPoint(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "type",
		Description: "Type text into the current focus or into a CSS ref/selector in this agent's Flutter web tab. Set clear=true to clear editable fields before typing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPTypeInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionType(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "key",
		Description: "Send a keyboard key such as Enter, Escape, Tab, Backspace, ArrowLeft, ArrowRight, ArrowUp, or ArrowDown to this agent's Flutter web tab.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPKeyInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionKey(featurePath, input)
		return nil, value, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "scroll",
		Description: "Scroll this agent's Flutter web tab using mouse wheel deltas at optional viewport coordinates. Defaults to page center and delta_y=500.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input browserMCPScrollInput) (*mcp.CallToolResult, any, error) {
		value, err := browserActionScroll(featurePath, input)
		return nil, value, err
	})

	_ = workspaceRoot
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func agentBrowserShellExports(record *agentRecord) string {
	if record == nil {
		return ""
	}
	parts := []string{}
	if strings.TrimSpace(record.WorkspaceRoot) != "" {
		parts = append(parts, "export AGENT_WORKSPACE="+shellQuote(record.WorkspaceRoot))
	}
	if strings.TrimSpace(record.ID) != "" {
		parts = append(parts, "export AGENT_FEATURE="+shellQuote(record.ID))
	}
	if strings.TrimSpace(record.URL) != "" {
		parts = append(parts, "export AGENT_BROWSER_URL="+shellQuote(record.URL))
	}
	if record.Runtime == "flutter" && strings.TrimSpace(record.Device) == "web-server" && strings.TrimSpace(record.WorkspaceRoot) != "" {
		config, err := agentBrowserMCPConfigContent(record)
		if err == nil && strings.TrimSpace(config) != "" {
			parts = append(parts, "export OPENCODE_CONFIG_CONTENT="+shellQuote(config))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + "; "
}

func agentBrowserMCPConfigContent(record *agentRecord) (string, error) {
	agentBin := installedAgentBinaryPath()
	entry := map[string]any{
		"type":    "local",
		"command": []string{agentBin, "browser", "mcp", "--workspace", record.WorkspaceRoot},
		"environment": map[string]string{
			"AGENT_WORKSPACE":   record.WorkspaceRoot,
			"AGENT_FEATURE":     record.ID,
			"AGENT_BROWSER_URL": record.URL,
		},
		"enabled": true,
		"timeout": 10000,
	}
	data, err := json.Marshal(map[string]any{"mcp": map[string]any{agentBrowserMCPName: entry}})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func installedAgentBinaryPath() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_BIN")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "agent"
	}
	return filepath.Join(home, ".config", "agent-tracker", "bin", "agent")
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func resolveBrowserWorkspace(workspace string) (string, string, error) {
	workspace = strings.TrimSpace(firstNonEmpty(workspace, os.Getenv("AGENT_WORKSPACE")))
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		workspace = cwd
	}
	path, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}
	for {
		featurePath := filepath.Join(path, "agent.json")
		if fileExists(featurePath) {
			return path, featurePath, nil
		}
		if filepath.Base(path) == "repo" {
			workspaceRoot := filepath.Dir(path)
			featurePath = filepath.Join(workspaceRoot, "agent.json")
			if fileExists(featurePath) {
				return workspaceRoot, featurePath, nil
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return "", "", fmt.Errorf("unable to find agent.json for workspace %s", workspace)
}

func browserMCPRequireTarget(featurePath string) (*featureConfig, *browserTarget, error) {
	cfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(cfg.Device) != "web-server" {
		return cfg, nil, fmt.Errorf("browser tools are disabled for device %q", strings.TrimSpace(cfg.Device))
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return cfg, nil, fmt.Errorf("agent browser URL is empty")
	}
	if _, err := browserCDPVersion(); err != nil {
		return cfg, nil, fmt.Errorf("Chrome for Testing is not running or not exposing CDP: %w", err)
	}
	target, err := browserTargetForURL(cfg.URL)
	if err != nil {
		return cfg, nil, err
	}
	if target == nil {
		return cfg, nil, fmt.Errorf("browser tab not found for %s; run agent_browser_open first", cfg.URL)
	}
	return cfg, target, nil
}

func browserMCPStatus(featurePath string) (map[string]any, error) {
	cfg, err := loadFeatureConfig(featurePath)
	if err != nil {
		return nil, err
	}
	status := map[string]any{
		"feature":         cfg.Feature,
		"device":          cfg.Device,
		"ready":           cfg.Ready,
		"configured_url":  cfg.URL,
		"browser_enabled": strings.TrimSpace(cfg.Device) == "web-server" && strings.TrimSpace(cfg.URL) != "",
	}
	if !status["browser_enabled"].(bool) {
		return status, nil
	}
	if activeURL, err := browserActivePageURL(); err == nil {
		status["active_chrome_url"] = activeURL
	}
	if _, err := browserCDPVersion(); err != nil {
		status["browser_running"] = false
		status["browser_error"] = err.Error()
		return status, nil
	}
	status["browser_running"] = true
	target, err := browserTargetForURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	status["tab_found"] = target != nil
	if target == nil {
		return status, nil
	}
	status["tab"] = map[string]any{"id": target.ID, "url": target.URL, "title": target.Title}
	page, err := browserRuntimeEvaluateValue(target.WebSocketDebuggerURL, `(() => ({
url: location.href,
title: document.title,
readyState: document.readyState,
visibilityState: document.visibilityState,
viewport: {width: innerWidth, height: innerHeight, devicePixelRatio: devicePixelRatio, scrollX: scrollX, scrollY: scrollY},
flutter: {appReady: Boolean(window.flutterAppReady), launchStage: window.flutterLaunchStage ?? null}
}))()`, true)
	if err != nil {
		status["page_error"] = err.Error()
		return status, nil
	}
	status["page"] = page
	return status, nil
}

func browserActionEvaluate(featurePath string, input browserMCPEvaluateInput) (map[string]any, error) {
	expr := strings.TrimSpace(input.Expression)
	if expr == "" {
		return nil, fmt.Errorf("expression is required")
	}
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	value, err := browserRuntimeEvaluateValue(target.WebSocketDebuggerURL, expr, input.AwaitPromise)
	if err != nil {
		return nil, err
	}
	return map[string]any{"value": value}, nil
}

func browserActionLogs(featurePath string, input browserMCPLogsInput) (map[string]any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	duration := input.DurationSeconds
	if duration <= 0 {
		duration = 3
	}
	if duration > 30 {
		duration = 30
	}
	entries, err := browserCollectLogs(target.WebSocketDebuggerURL, time.Duration(duration)*time.Second)
	if err != nil {
		return nil, err
	}
	entries, omitted := browserPrepareLogEntries(entries, browserLogOptions{All: input.All, Limit: input.Limit, MaxChars: input.MaxChars})
	return map[string]any{
		"title":            firstNonEmpty(strings.TrimSpace(target.Title), "(untitled)"),
		"url":              strings.TrimSpace(target.URL),
		"duration_seconds": duration,
		"all":              input.All,
		"entries":          entries,
		"omitted":          omitted,
	}, nil
}

func browserActionScreenshot(featurePath string, options browserScreenshotOptions) (*browserScreenshotResult, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	if options.Grid {
		if options.MajorStep <= 0 {
			options.MajorStep = 100
		}
		if options.MinorStep <= 0 {
			options.MinorStep = 25
		}
		if err := browserSetCoordinateOverlay(target.WebSocketDebuggerURL, options.MajorStep, options.MinorStep); err != nil {
			return nil, err
		}
		defer func() { _ = browserClearCoordinateOverlay(target.WebSocketDebuggerURL) }()
	}
	viewport, _ := browserViewportMetrics(target.WebSocketDebuggerURL)
	pngData, err := browserCaptureScreenshot(target.WebSocketDebuggerURL)
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(options.OutPath)
	createdTemp := false
	if path == "" {
		path, err = defaultBrowserScreenshotPath(featurePath)
		if err != nil {
			return nil, err
		}
		createdTemp = true
	}
	if err := writeCompressedScreenshot(pngData, path); err != nil {
		if createdTemp {
			_ = os.Remove(path)
		}
		return nil, err
	}
	jpegData, err := os.ReadFile(path)
	if err != nil {
		if createdTemp {
			_ = os.Remove(path)
		}
		return nil, err
	}
	imageWidth, imageHeight, _ := imageDimensions(jpegData)
	meta := map[string]any{
		"url":       target.URL,
		"title":     target.Title,
		"path":      path,
		"bytes":     len(jpegData),
		"mime_type": "image/jpeg",
		"grid":      options.Grid,
		"image": map[string]any{
			"width":  imageWidth,
			"height": imageHeight,
		},
		"viewport": viewport,
		"coordinate_system": map[string]any{
			"origin": "top-left",
			"x":      "viewport CSS pixels increasing right",
			"y":      "viewport CSS pixels increasing down",
			"click":  "Pass x/y directly to click. If using image pixels from a resized screenshot, pass image_x/image_y with screenshot_path.",
		},
	}
	if scale := browserImageToViewportScale(viewport, imageWidth, imageHeight); scale != nil {
		meta["image_to_viewport"] = scale
	}
	if options.Grid {
		meta["grid_steps"] = map[string]any{"major": options.MajorStep, "minor": options.MinorStep}
	}
	return &browserScreenshotResult{Path: path, JPEGData: jpegData, Metadata: meta}, nil
}

func browserActionSnapshot(featurePath string, input browserMCPSnapshotInput) (any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	maxText := input.MaxText
	if maxText <= 0 {
		maxText = 4000
	}
	if maxText > 20000 {
		maxText = 20000
	}
	maxElements := input.MaxElements
	if maxElements <= 0 {
		maxElements = 80
	}
	if maxElements > 300 {
		maxElements = 300
	}
	return browserRuntimeEvaluateValue(target.WebSocketDebuggerURL, browserSnapshotExpression(maxText, maxElements), true)
}

func browserActionClick(featurePath string, input browserMCPClickInput) (map[string]any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	x, y, err := browserClickPoint(target.WebSocketDebuggerURL, input)
	if err != nil {
		return nil, err
	}
	button := strings.TrimSpace(input.Button)
	if button == "" {
		button = "left"
	}
	clickCount := input.ClickCount
	if clickCount <= 0 {
		clickCount = 1
	}
	if err := browserDispatchMouseClick(target.WebSocketDebuggerURL, x, y, button, clickCount); err != nil {
		return nil, err
	}
	return map[string]any{"clicked": map[string]any{"x": x, "y": y, "button": button, "click_count": clickCount}}, nil
}

func browserActionInspectPoint(featurePath string, input browserMCPInspectPointInput) (any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	x, y, err := browserInspectPoint(target.WebSocketDebuggerURL, input)
	if err != nil {
		return nil, err
	}
	return browserRuntimeEvaluateValue(target.WebSocketDebuggerURL, browserInspectPointExpression(x, y), true)
}

func browserActionType(featurePath string, input browserMCPTypeInput) (map[string]any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	if input.Text == "" && !input.Clear {
		return nil, fmt.Errorf("text is required")
	}
	selector := firstNonEmpty(strings.TrimSpace(input.Ref), strings.TrimSpace(input.Selector))
	if selector != "" || input.Clear {
		if _, err := browserRuntimeEvaluateValue(target.WebSocketDebuggerURL, browserFocusExpression(selector, input.Clear), true); err != nil {
			return nil, err
		}
	}
	if input.Text != "" {
		if err := browserInsertText(target.WebSocketDebuggerURL, input.Text); err != nil {
			return nil, err
		}
	}
	return map[string]any{"typed": len([]rune(input.Text)), "selector": selector, "cleared": input.Clear}, nil
}

func browserActionKey(featurePath string, input browserMCPKeyInput) (map[string]any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	if err := browserDispatchKey(target.WebSocketDebuggerURL, key); err != nil {
		return nil, err
	}
	return map[string]any{"key": key}, nil
}

func browserActionScroll(featurePath string, input browserMCPScrollInput) (map[string]any, error) {
	_, target, err := browserMCPRequireTarget(featurePath)
	if err != nil {
		return nil, err
	}
	x, y, err := browserScrollPoint(target.WebSocketDebuggerURL, input)
	if err != nil {
		return nil, err
	}
	deltaX := input.DeltaX
	deltaY := input.DeltaY
	if deltaX == 0 && deltaY == 0 {
		deltaY = 500
	}
	result, err := browserDispatchScroll(target.WebSocketDebuggerURL, x, y, deltaX, deltaY)
	if err != nil {
		return nil, err
	}
	return map[string]any{"x": x, "y": y, "delta_x": deltaX, "delta_y": deltaY, "result": result}, nil
}

func browserMCPScreenshotResult(featurePath string, grid bool, majorStep int, minorStep int) (*mcp.CallToolResult, any, error) {
	result, err := browserActionScreenshot(featurePath, browserScreenshotOptions{Grid: grid, MajorStep: majorStep, MinorStep: minorStep})
	if err != nil {
		return nil, nil, err
	}
	metaText, _ := json.MarshalIndent(result.Metadata, "", "  ")
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: string(metaText)},
		&mcp.ImageContent{Data: result.JPEGData, MIMEType: "image/jpeg"},
	}}, nil, nil
}

func browserViewportMetrics(pageWSURL string) (map[string]any, error) {
	value, err := browserRuntimeEvaluateValue(pageWSURL, `(() => ({width: innerWidth, height: innerHeight, devicePixelRatio: devicePixelRatio || 1, scrollX, scrollY, visibilityState: document.visibilityState}))()`, true)
	if err != nil {
		return nil, err
	}
	metrics, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unable to read viewport metrics")
	}
	return metrics, nil
}

func browserImageToViewportScale(viewport map[string]any, imageWidth, imageHeight int) map[string]any {
	if viewport == nil || imageWidth <= 0 || imageHeight <= 0 {
		return nil
	}
	viewportWidth, okWidth := numberValue(viewport["width"])
	viewportHeight, okHeight := numberValue(viewport["height"])
	if !okWidth || !okHeight || viewportWidth <= 0 || viewportHeight <= 0 {
		return nil
	}
	return map[string]any{
		"x_multiplier": viewportWidth / float64(imageWidth),
		"y_multiplier": viewportHeight / float64(imageHeight),
		"formula":      "viewport_x = image_x * x_multiplier; viewport_y = image_y * y_multiplier",
	}
}

func browserSetCoordinateOverlay(pageWSURL string, majorStep int, minorStep int) error {
	_, err := browserRuntimeEvaluateValue(pageWSURL, browserCoordinateOverlayExpression(majorStep, minorStep), true)
	return err
}

func browserClearCoordinateOverlay(pageWSURL string) error {
	_, err := browserRuntimeEvaluateValue(pageWSURL, `(() => { document.getElementById('__agent_browser_coordinate_overlay__')?.remove(); return true; })()`, true)
	return err
}

func browserCoordinateOverlayExpression(majorStep int, minorStep int) string {
	return fmt.Sprintf(`(() => new Promise((resolve) => {
document.getElementById('__agent_browser_coordinate_overlay__')?.remove();
const major = %d;
const minor = %d;
const overlay = document.createElement('div');
overlay.id = '__agent_browser_coordinate_overlay__';
overlay.style.cssText = 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;font:11px ui-monospace,SFMono-Regular,Menlo,monospace;color:#b91c1c;';
const svgNS = 'http://www.w3.org/2000/svg';
const svg = document.createElementNS(svgNS, 'svg');
svg.setAttribute('width', innerWidth);
svg.setAttribute('height', innerHeight);
svg.setAttribute('viewBox', '0 0 ' + innerWidth + ' ' + innerHeight);
svg.style.cssText = 'position:absolute;inset:0;width:100%%;height:100%%;';
const addLine = (x1, y1, x2, y2, color, width) => {
  const line = document.createElementNS(svgNS, 'line');
  line.setAttribute('x1', x1); line.setAttribute('y1', y1); line.setAttribute('x2', x2); line.setAttribute('y2', y2);
  line.setAttribute('stroke', color); line.setAttribute('stroke-width', width); line.setAttribute('vector-effect', 'non-scaling-stroke');
  svg.appendChild(line);
};
const addText = (x, y, text) => {
  const label = document.createElementNS(svgNS, 'text');
  label.setAttribute('x', x); label.setAttribute('y', y); label.setAttribute('fill', '#b91c1c'); label.setAttribute('stroke', 'white'); label.setAttribute('stroke-width', '3'); label.setAttribute('paint-order', 'stroke');
  label.textContent = text;
  svg.appendChild(label);
};
for (let x = 0; x <= innerWidth; x += minor) addLine(x, 0, x, innerHeight, x %% major === 0 ? 'rgba(185,28,28,.55)' : 'rgba(185,28,28,.18)', x %% major === 0 ? 1.5 : 1);
for (let y = 0; y <= innerHeight; y += minor) addLine(0, y, innerWidth, y, y %% major === 0 ? 'rgba(185,28,28,.55)' : 'rgba(185,28,28,.18)', y %% major === 0 ? 1.5 : 1);
for (let x = 0; x <= innerWidth; x += major) addText(x + 3, 13, String(x));
for (let y = major; y <= innerHeight; y += major) addText(3, y - 3, String(y));
overlay.appendChild(svg);
document.documentElement.appendChild(overlay);
requestAnimationFrame(() => resolve(true));
}))()`, majorStep, minorStep)
}

func browserRuntimeEvaluateValue(pageWSURL, expression string, awaitPromise bool) (any, error) {
	var result browserRuntimeEvaluateResult
	if err := browserCDPRequest(pageWSURL, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}, &result); err != nil {
		return nil, err
	}
	if result.ExceptionDetails != nil {
		return nil, fmt.Errorf("browser evaluation failed: %s", browserExceptionText(result.ExceptionDetails))
	}
	if result.Result.Value != nil {
		return result.Result.Value, nil
	}
	return map[string]any{"type": result.Result.Type, "subtype": result.Result.Subtype, "description": result.Result.Description}, nil
}

func browserExceptionText(details *browserRuntimeExceptionDetails) string {
	if details == nil {
		return "unknown exception"
	}
	return firstNonEmpty(strings.TrimSpace(details.Exception.Description), strings.TrimSpace(details.Text), fmt.Sprint(details.Exception.Value), "unknown exception")
}

func browserCollectLogs(pageWSURL string, duration time.Duration) ([]map[string]any, error) {
	conn, _, err := websocket.DefaultDialer.Dial(pageWSURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(duration)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	for id, method := range []string{"Runtime.enable", "Log.enable", "Console.enable"} {
		if err := conn.WriteJSON(map[string]any{"id": id + 1, "method": method, "params": map[string]any{}}); err != nil {
			return nil, err
		}
	}
	entries := []map[string]any{}
	for {
		var envelope browserCDPEnvelope
		if err := conn.ReadJSON(&envelope); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return entries, nil
			}
			return entries, err
		}
		switch envelope.Method {
		case "Runtime.consoleAPICalled":
			var params struct {
				Type string `json:"type"`
				Args []struct {
					Type        string `json:"type"`
					Value       any    `json:"value,omitempty"`
					Description string `json:"description,omitempty"`
				} `json:"args"`
			}
			if err := json.Unmarshal(envelope.Params, &params); err == nil {
				entries = append(entries, map[string]any{"source": "console", "level": firstNonEmpty(params.Type, "log"), "text": browserConsoleArgsText(params.Args)})
			}
		case "Runtime.exceptionThrown":
			var params struct {
				ExceptionDetails browserRuntimeExceptionDetails `json:"exceptionDetails"`
			}
			if err := json.Unmarshal(envelope.Params, &params); err == nil {
				entries = append(entries, map[string]any{"source": "exception", "level": "error", "text": browserExceptionText(&params.ExceptionDetails)})
			}
		case "Log.entryAdded":
			var params struct {
				Entry struct {
					Source string `json:"source"`
					Level  string `json:"level"`
					Text   string `json:"text"`
				} `json:"entry"`
			}
			if err := json.Unmarshal(envelope.Params, &params); err == nil {
				entries = append(entries, map[string]any{"source": firstNonEmpty(params.Entry.Source, "log"), "level": firstNonEmpty(params.Entry.Level, "info"), "text": params.Entry.Text})
			}
		}
	}
}

func browserPrepareLogEntries(entries []map[string]any, options browserLogOptions) ([]map[string]any, int) {
	limit := options.Limit
	if limit == 0 {
		limit = 40
	}
	if limit < 0 {
		limit = 0
	}
	maxChars := options.MaxChars
	if maxChars == 0 {
		maxChars = 600
	}
	if maxChars < 0 {
		maxChars = 0
	}
	prepared := []map[string]any{}
	indexes := map[string]int{}
	counts := map[string]int{}
	for _, entry := range entries {
		if !options.All && !browserLogEntryImportant(entry) {
			continue
		}
		text := browserLogString(entry, "text")
		key := browserLogString(entry, "source") + "\x00" + browserLogString(entry, "level") + "\x00" + text
		if index, ok := indexes[key]; ok {
			counts[key]++
			prepared[index]["count"] = counts[key]
			continue
		}
		copy := cloneMap(entry)
		if maxChars > 0 {
			copy["text"] = browserTruncateASCII(text, maxChars)
		}
		indexes[key] = len(prepared)
		counts[key] = 1
		prepared = append(prepared, copy)
	}
	omitted := 0
	if limit > 0 && len(prepared) > limit {
		omitted = len(prepared) - limit
		prepared = prepared[:limit]
	}
	return prepared, omitted
}

func browserLogEntryImportant(entry map[string]any) bool {
	source := strings.ToLower(browserLogString(entry, "source"))
	level := strings.ToLower(browserLogString(entry, "level"))
	text := strings.ToLower(browserLogString(entry, "text"))
	if browserLogLooksLikeStackFrame(text) {
		return false
	}
	if source == "exception" || level == "warning" || level == "error" || level == "fatal" {
		return true
	}
	for _, fragment := range []string{"exception", "error", "failed", "invalid argument", "boxconstraints", "overflowed", "file://"} {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func browserLogLooksLikeStackFrame(text string) bool {
	text = strings.TrimSpace(text)
	if strings.Contains(text, "this was the stack") {
		return true
	}
	for _, prefix := range []string{"dart-sdk/", "package:flutter/", "package:collection/", "lib/_engine/"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func browserLogLine(entry map[string]any) string {
	source := browserLogString(entry, "source")
	level := browserLogString(entry, "level")
	if source == "" {
		source = "log"
	}
	if level == "" {
		level = "info"
	}
	prefix := source + "." + level
	if source == "exception" {
		prefix = "exception"
	}
	line := "[" + prefix + "] " + browserLogString(entry, "text")
	if count, ok := numberValue(entry["count"]); ok && count > 1 {
		line += fmt.Sprintf(" (x%d)", int(count))
	}
	return line
}

func browserLogString(entry map[string]any, key string) string {
	if entry == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(entry[key]))
}

func browserTruncateASCII(value string, maxChars int) string {
	if maxChars <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}

func browserSnapshotExpression(maxText, maxElements int) string {
	return fmt.Sprintf(`(() => {
const maxText = %d;
const maxElements = %d;
const visible = (el) => {
  const style = getComputedStyle(el);
  const rect = el.getBoundingClientRect();
  return style.visibility !== 'hidden' && style.display !== 'none' && rect.width > 0 && rect.height > 0 && rect.bottom >= 0 && rect.right >= 0 && rect.top <= innerHeight && rect.left <= innerWidth;
};
const cssEscape = (value) => window.CSS && CSS.escape ? CSS.escape(value) : String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
const selectorFor = (el) => {
  if (el.id) return '#' + cssEscape(el.id);
  const testId = el.getAttribute('data-testid') || el.getAttribute('data-test-id');
  if (testId) return '[' + (el.hasAttribute('data-testid') ? 'data-testid' : 'data-test-id') + '=' + JSON.stringify(testId) + ']';
  const parts = [];
  let node = el;
  while (node && node.nodeType === 1 && node !== document.documentElement) {
    let part = node.localName;
    const parent = node.parentElement;
    if (!parent) break;
    const same = Array.from(parent.children).filter((child) => child.localName === node.localName);
    if (same.length > 1) part += ':nth-of-type(' + (same.indexOf(node) + 1) + ')';
    parts.unshift(part);
    const selector = parts.join(' > ');
    if (document.querySelectorAll(selector).length === 1) return selector;
    node = parent;
  }
  return parts.join(' > ');
};
const candidates = Array.from(document.querySelectorAll('a,button,input,textarea,select,summary,canvas,[role],[tabindex],[aria-label],[data-testid],[data-test-id]'));
const interactive = [];
for (const el of candidates) {
  if (!visible(el)) continue;
  const rect = el.getBoundingClientRect();
  interactive.push({
    ref: selectorFor(el),
    tag: el.localName,
    role: el.getAttribute('role') || '',
    text: (el.innerText || '').trim().slice(0, 200),
    ariaLabel: el.getAttribute('aria-label') || '',
    placeholder: el.getAttribute('placeholder') || '',
    value: 'value' in el ? String(el.value).slice(0, 200) : '',
    rect: {x: rect.x, y: rect.y, width: rect.width, height: rect.height}
  });
  if (interactive.length >= maxElements) break;
}
return {
  url: location.href,
  title: document.title,
  readyState: document.readyState,
  text: (document.body && document.body.innerText ? document.body.innerText : '').slice(0, maxText),
  interactive
};
})()`, maxText, maxElements)
}

func browserClickPoint(pageWSURL string, input browserMCPClickInput) (float64, float64, error) {
	if x, y, ok, err := browserPointFromImageCoordinates(pageWSURL, input.ImageX, input.ImageY, input.ImageWidth, input.ImageHeight, input.ScreenshotPath); ok || err != nil {
		return x, y, err
	}
	if input.X != nil && input.Y != nil {
		return *input.X, *input.Y, nil
	}
	selector := firstNonEmpty(strings.TrimSpace(input.Ref), strings.TrimSpace(input.Selector))
	text := strings.TrimSpace(input.Text)
	if selector == "" && text == "" {
		return 0, 0, fmt.Errorf("click requires ref, selector, text, x/y, or image_x/image_y")
	}
	value, err := browserRuntimeEvaluateValue(pageWSURL, browserPointExpression(selector, text), true)
	if err != nil {
		return 0, 0, err
	}
	point, ok := value.(map[string]any)
	if !ok || point == nil {
		return 0, 0, fmt.Errorf("target element not found")
	}
	x, okX := numberValue(point["x"])
	y, okY := numberValue(point["y"])
	if !okX || !okY {
		return 0, 0, fmt.Errorf("target element did not provide click coordinates")
	}
	return x, y, nil
}

func browserInspectPoint(pageWSURL string, input browserMCPInspectPointInput) (float64, float64, error) {
	if x, y, ok, err := browserPointFromImageCoordinates(pageWSURL, input.ImageX, input.ImageY, input.ImageWidth, input.ImageHeight, input.ScreenshotPath); ok || err != nil {
		return x, y, err
	}
	if input.X != nil && input.Y != nil {
		return *input.X, *input.Y, nil
	}
	return 0, 0, fmt.Errorf("inspect_point requires x/y or image_x/image_y")
}

func browserPointFromImageCoordinates(pageWSURL string, imageX, imageY, imageWidth, imageHeight *float64, screenshotPath string) (float64, float64, bool, error) {
	if imageX == nil || imageY == nil {
		return 0, 0, false, nil
	}
	width, height := 0.0, 0.0
	if imageWidth != nil && imageHeight != nil && *imageWidth > 0 && *imageHeight > 0 {
		width = *imageWidth
		height = *imageHeight
	} else if strings.TrimSpace(screenshotPath) != "" {
		data, err := os.ReadFile(strings.TrimSpace(screenshotPath))
		if err != nil {
			return 0, 0, true, err
		}
		imageW, imageH, ok := imageDimensions(data)
		if !ok {
			return 0, 0, true, fmt.Errorf("unable to read screenshot dimensions")
		}
		width = float64(imageW)
		height = float64(imageH)
	}
	if width <= 0 || height <= 0 {
		return *imageX, *imageY, true, nil
	}
	viewport, err := browserViewportMetrics(pageWSURL)
	if err != nil {
		return 0, 0, true, err
	}
	viewportWidth, okWidth := numberValue(viewport["width"])
	viewportHeight, okHeight := numberValue(viewport["height"])
	if !okWidth || !okHeight || viewportWidth <= 0 || viewportHeight <= 0 {
		return 0, 0, true, fmt.Errorf("unable to read viewport dimensions")
	}
	return (*imageX) * viewportWidth / width, (*imageY) * viewportHeight / height, true, nil
}

func browserInspectPointExpression(x, y float64) string {
	return fmt.Sprintf(`(() => {
const x = %s;
const y = %s;
const cssEscape = (value) => window.CSS && CSS.escape ? CSS.escape(value) : String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
const selectorFor = (el) => {
  if (!el) return '';
  if (el.id) return '#' + cssEscape(el.id);
  const testId = el.getAttribute('data-testid') || el.getAttribute('data-test-id');
  if (testId) return '[' + (el.hasAttribute('data-testid') ? 'data-testid' : 'data-test-id') + '=' + JSON.stringify(testId) + ']';
  const parts = [];
  let node = el;
  while (node && node.nodeType === 1 && node !== document.documentElement) {
    let part = node.localName;
    const parent = node.parentElement;
    if (!parent) break;
    const same = Array.from(parent.children).filter((child) => child.localName === node.localName);
    if (same.length > 1) part += ':nth-of-type(' + (same.indexOf(node) + 1) + ')';
    parts.unshift(part);
    const selector = parts.join(' > ');
    if (document.querySelectorAll(selector).length === 1) return selector;
    node = parent;
  }
  return parts.join(' > ');
};
const describe = (el) => {
  if (!el) return null;
  const rect = el.getBoundingClientRect();
  return {
    ref: selectorFor(el),
    tag: el.localName,
    id: el.id || '',
    className: typeof el.className === 'string' ? el.className : '',
    role: el.getAttribute('role') || '',
    ariaLabel: el.getAttribute('aria-label') || '',
    placeholder: el.getAttribute('placeholder') || '',
    text: (el.innerText || '').trim().slice(0, 200),
    value: 'value' in el ? String(el.value).slice(0, 500) : '',
    rect: {x: rect.x, y: rect.y, width: rect.width, height: rect.height}
  };
};
const el = document.elementFromPoint(x, y);
const chain = [];
let node = el;
while (node && node.nodeType === 1 && chain.length < 6) {
  chain.push(describe(node));
  node = node.parentElement;
}
return {point: {x, y}, viewport: {width: innerWidth, height: innerHeight, devicePixelRatio: devicePixelRatio || 1}, element: describe(el), ancestors: chain};
})()`, jsNumber(x), jsNumber(y))
}

func browserPointExpression(selector, text string) string {
	return fmt.Sprintf(`(() => {
const selector = %s;
const text = %s;
const visible = (el) => {
  const style = getComputedStyle(el);
  const rect = el.getBoundingClientRect();
  return style.visibility !== 'hidden' && style.display !== 'none' && rect.width > 0 && rect.height > 0;
};
let el = null;
if (selector) el = document.querySelector(selector);
if (!el && text) {
  const needle = text.toLowerCase();
  const candidates = Array.from(document.querySelectorAll('a,button,input,textarea,select,summary,canvas,[role],[tabindex],[aria-label],[data-testid],[data-test-id]'));
  el = candidates.find((node) => visible(node) && (((node.innerText || '') + ' ' + (node.getAttribute('aria-label') || '') + ' ' + (node.getAttribute('placeholder') || '')).toLowerCase().includes(needle)));
}
if (!el || !visible(el)) return null;
el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
const rect = el.getBoundingClientRect();
return {x: Math.max(0, Math.min(innerWidth - 1, rect.left + rect.width / 2)), y: Math.max(0, Math.min(innerHeight - 1, rect.top + rect.height / 2))};
})()`, jsString(selector), jsString(text))
}

func browserFocusExpression(selector string, clear bool) string {
	return fmt.Sprintf(`(() => {
const selector = %s;
const clear = %s;
const el = selector ? document.querySelector(selector) : document.activeElement;
if (!el) throw new Error(selector ? 'element not found' : 'no active element');
if (el.scrollIntoView) el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
if (el.focus) el.focus();
if (clear) {
  if ('value' in el) {
    el.value = '';
    el.dispatchEvent(new Event('input', {bubbles: true}));
    el.dispatchEvent(new Event('change', {bubbles: true}));
  } else if (el.isContentEditable) {
    el.textContent = '';
    el.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'deleteContentBackward'}));
  }
}
return true;
})()`, jsString(selector), strconv.FormatBool(clear))
}

func browserScrollPoint(pageWSURL string, input browserMCPScrollInput) (float64, float64, error) {
	if input.X != nil && input.Y != nil {
		return *input.X, *input.Y, nil
	}
	value, err := browserRuntimeEvaluateValue(pageWSURL, `(() => ({x: innerWidth / 2, y: innerHeight / 2}))()`, true)
	if err != nil {
		return 0, 0, err
	}
	point, ok := value.(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("unable to compute viewport center")
	}
	x, okX := numberValue(point["x"])
	y, okY := numberValue(point["y"])
	if !okX || !okY {
		return 0, 0, fmt.Errorf("unable to compute viewport center")
	}
	return x, y, nil
}

func browserDispatchMouseClick(pageWSURL string, x, y float64, button string, clickCount int) error {
	params := map[string]any{"x": x, "y": y, "button": button, "clickCount": clickCount}
	pressed := cloneMap(params)
	pressed["type"] = "mousePressed"
	if err := browserCDPRequest(pageWSURL, "Input.dispatchMouseEvent", pressed, nil); err != nil {
		return err
	}
	released := cloneMap(params)
	released["type"] = "mouseReleased"
	return browserCDPRequest(pageWSURL, "Input.dispatchMouseEvent", released, nil)
}

func browserDispatchScroll(pageWSURL string, x, y, deltaX, deltaY float64) (any, error) {
	return browserRuntimeEvaluateValue(pageWSURL, fmt.Sprintf(`(() => {
const x = %s;
const y = %s;
const deltaX = %s;
const deltaY = %s;
const target = document.elementFromPoint(x, y) || document.scrollingElement || document.body;
const event = new WheelEvent('wheel', {deltaX, deltaY, clientX: x, clientY: y, bubbles: true, cancelable: true});
const defaultAllowed = target.dispatchEvent(event);
if (defaultAllowed) window.scrollBy(deltaX, deltaY);
return {defaultAllowed, scrollX: window.scrollX, scrollY: window.scrollY};
})()`, jsNumber(x), jsNumber(y), jsNumber(deltaX), jsNumber(deltaY)), true)
}

func browserInsertText(pageWSURL, text string) error {
	return browserCDPRequest(pageWSURL, "Input.insertText", map[string]any{"text": text}, nil)
}

func browserDispatchKey(pageWSURL, key string) error {
	info := browserKeyInfo(key)
	down := map[string]any{"type": "keyDown", "key": info.Key, "code": info.Code, "windowsVirtualKeyCode": info.VirtualKeyCode}
	if info.Text != "" {
		down["text"] = info.Text
		down["unmodifiedText"] = info.Text
	}
	if err := browserCDPRequest(pageWSURL, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	up := map[string]any{"type": "keyUp", "key": info.Key, "code": info.Code, "windowsVirtualKeyCode": info.VirtualKeyCode}
	return browserCDPRequest(pageWSURL, "Input.dispatchKeyEvent", up, nil)
}

type browserKey struct {
	Key            string
	Code           string
	VirtualKeyCode int
	Text           string
}

func browserKeyInfo(key string) browserKey {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "return":
		return browserKey{Key: "Enter", Code: "Enter", VirtualKeyCode: 13, Text: "\r"}
	case "escape", "esc":
		return browserKey{Key: "Escape", Code: "Escape", VirtualKeyCode: 27}
	case "tab":
		return browserKey{Key: "Tab", Code: "Tab", VirtualKeyCode: 9, Text: "\t"}
	case "backspace":
		return browserKey{Key: "Backspace", Code: "Backspace", VirtualKeyCode: 8}
	case "delete":
		return browserKey{Key: "Delete", Code: "Delete", VirtualKeyCode: 46}
	case "arrowleft", "left":
		return browserKey{Key: "ArrowLeft", Code: "ArrowLeft", VirtualKeyCode: 37}
	case "arrowup", "up":
		return browserKey{Key: "ArrowUp", Code: "ArrowUp", VirtualKeyCode: 38}
	case "arrowright", "right":
		return browserKey{Key: "ArrowRight", Code: "ArrowRight", VirtualKeyCode: 39}
	case "arrowdown", "down":
		return browserKey{Key: "ArrowDown", Code: "ArrowDown", VirtualKeyCode: 40}
	}
	if len([]rune(key)) == 1 {
		r := []rune(key)[0]
		code := ""
		vk := int(r)
		if r >= 'a' && r <= 'z' {
			code = "Key" + strings.ToUpper(string(r))
			vk = int(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			code = "Key" + string(r)
		} else if r >= '0' && r <= '9' {
			code = "Digit" + string(r)
		}
		return browserKey{Key: key, Code: code, VirtualKeyCode: vk, Text: key}
	}
	return browserKey{Key: key, Code: key, VirtualKeyCode: 0}
}

func jsString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func jsNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
