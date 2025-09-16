package ipc

type Envelope struct {
	Kind      string `json:"kind"`
	Command   string `json:"command,omitempty"`
	Client    string `json:"client,omitempty"`
	Session   string `json:"session,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Window    string `json:"window,omitempty"`
	WindowID  string `json:"window_id,omitempty"`
	Pane      string `json:"pane,omitempty"`
	Position  string `json:"position,omitempty"`
	Visible   *bool  `json:"visible,omitempty"`
	Message   string `json:"message,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Tasks     []Task `json:"tasks,omitempty"`
}

type Task struct {
	SessionID       string  `json:"session_id"`
	Session         string  `json:"session"`
	WindowID        string  `json:"window_id"`
	Window          string  `json:"window"`
	Pane            string  `json:"pane,omitempty"`
	Status          string  `json:"status"`
	Summary         string  `json:"summary"`
	CompletionNote  string  `json:"completion_note,omitempty"`
	StartedAt       string  `json:"started_at,omitempty"`
	CompletedAt     string  `json:"completed_at,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
	Acknowledged    bool    `json:"acknowledged"`
}
