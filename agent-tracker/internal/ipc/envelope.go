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
	Scope     string `json:"scope,omitempty"`
	NoteID    string `json:"note_id,omitempty"`
	GoalID    string `json:"goal_id,omitempty"`
	Position  string `json:"position,omitempty"`
	Visible   *bool  `json:"visible,omitempty"`
	Message   string `json:"message,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Tasks     []Task `json:"tasks,omitempty"`
	Notes     []Note `json:"notes,omitempty"`
	Archived  []Note `json:"archived,omitempty"`
	Goals     []Goal `json:"goals,omitempty"`
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

type Note struct {
	ID         string `json:"id"`
	Scope      string `json:"scope,omitempty"`
	SessionID  string `json:"session_id"`
	Session    string `json:"session"`
	WindowID   string `json:"window_id"`
	Window     string `json:"window"`
	Pane       string `json:"pane,omitempty"`
	Summary    string `json:"summary"`
	Completed  bool   `json:"completed"`
	Archived   bool   `json:"archived"`
	CreatedAt  string `json:"created_at"`
	ArchivedAt string `json:"archived_at,omitempty"`
}

type Goal struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Session   string `json:"session"`
	Summary   string `json:"summary"`
	Completed bool   `json:"completed"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
