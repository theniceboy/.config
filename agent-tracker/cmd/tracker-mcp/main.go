package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david/agent-tracker/internal/ipc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	implementationName    = "tracker_mcp"
	implementationVersion = "0.1.0"
	commandTimeout        = 5 * time.Second
)

type trackerClient struct {
	socket string
}

func newTrackerClient() *trackerClient {
	socket := os.Getenv("CODEX_TRACKER_SOCKET")
	if strings.TrimSpace(socket) == "" {
		socket = socketPath()
	}
	return &trackerClient{socket: socket}
}

func (c *trackerClient) sendCommand(ctx context.Context, env ipc.Envelope) error {
	env.Kind = "command"
	d := net.Dialer{}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, commandTimeout)
		defer cancel()
	}
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(&env); err != nil {
		return err
	}

	for {
		var reply ipc.Envelope
		if err := dec.Decode(&reply); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return fmt.Errorf("tracker server disconnected")
			}
			return err
		}
		if reply.Kind == "ack" {
			return nil
		}
	}
}

type startInput struct {
	Summary string `json:"summary"`
	TmuxID  string `json:"tmux_id"`
}

type finishInput struct {
	Summary string `json:"summary"`
	TmuxID  string `json:"tmux_id"`
}

func main() {
	log.SetFlags(0)
	client := newTrackerClient()

	server := mcp.NewServer(&mcp.Implementation{Name: implementationName, Version: implementationVersion}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tracker_mark_start_working",
		Description: "Record that work has started for the currently focused tmux session/window.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input startInput) (*mcp.CallToolResult, any, error) {
		tmuxID := strings.TrimSpace(input.TmuxID)
		if tmuxID == "" {
			return nil, nil, fmt.Errorf("tmux_id is required; pass session_id::window_id::pane_id (for example, $3::@12::%30)")
		}
		target, err := determineContext(tmuxID)
		if err != nil {
			return nil, nil, err
		}
		summary := strings.TrimSpace(input.Summary)
		if summary == "" {
			return nil, nil, fmt.Errorf("summary is required")
		}
		env := ipc.Envelope{
			Command:   "start_task",
			Session:   target.SessionID,
			SessionID: target.SessionID,
			Window:    target.WindowID,
			WindowID:  target.WindowID,
			Pane:      target.PaneID,
			Summary:   summary,
		}
		if err := client.sendCommand(ctx, env); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Status recorded. Do the work, then call `tracker_mark_respond_to_user` exactly once, right before you send the user their reply."},
			},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tracker_mark_respond_to_user",
		Description: "Record that work has completed for the currently focused tmux session/window.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input finishInput) (*mcp.CallToolResult, any, error) {
		tmuxID := strings.TrimSpace(input.TmuxID)
		if tmuxID == "" {
			return nil, nil, fmt.Errorf("tmux_id is required; pass session_id::window_id::pane_id (for example, $3::@12::%30)")
		}
		target, err := determineContext(tmuxID)
		if err != nil {
			return nil, nil, err
		}
		summary := strings.TrimSpace(input.Summary)
		env := ipc.Envelope{
			Command:   "finish_task",
			Session:   target.SessionID,
			SessionID: target.SessionID,
			Window:    target.WindowID,
			WindowID:  target.WindowID,
			Pane:      target.PaneID,
			Summary:   summary,
		}
		if summary != "" {
			env.Message = summary
		}
		if err := client.sendCommand(ctx, env); err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Send your reply now. You MUST NOT not call `tracker_mark_start_working` from now on until the user sends you a message."},
			},
		}, nil, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

type tmuxContext struct {
	SessionID string
	WindowID  string
	PaneID    string
}

func determineContext(tmuxID string) (tmuxContext, error) {
	parts := strings.Split(strings.TrimSpace(tmuxID), "::")
	if len(parts) != 3 {
		return tmuxContext{}, fmt.Errorf("tmux_id must be session_id::window_id::pane_id")
	}
	sessionID := strings.TrimSpace(parts[0])
	windowID := strings.TrimSpace(parts[1])
	paneID := strings.TrimSpace(parts[2])
	if sessionID == "" || windowID == "" || paneID == "" {
		return tmuxContext{}, fmt.Errorf("tmux_id must include non-empty session, window, and pane identifiers")
	}
	return tmuxContext{SessionID: sessionID, WindowID: windowID, PaneID: paneID}, nil
}

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}
