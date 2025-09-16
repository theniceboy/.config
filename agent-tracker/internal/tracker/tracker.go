package tracker

import "time"

type Status string

const (
    StatusIdle       Status = "idle"
    StatusInProgress Status = "in_progress"
    StatusCompleted  Status = "completed"
)

type Entry struct {
    Session      string    `json:"session"`
    Pane         string    `json:"pane"`
    Status       Status    `json:"status"`
    Description  string    `json:"description"`
    StartedAt    time.Time `json:"started_at"`
    CompletedAt  time.Time `json:"completed_at"`
    Acknowledged bool      `json:"acknowledged"`
}

type UpdateManager interface {
    StartWork(session, pane, description string, now time.Time) (*Entry, error)
    CompleteWork(session, pane, summary string, now time.Time) (*Entry, error)
    Acknowledge(session, pane string) (*Entry, error)
    Get(session, pane string) (*Entry, bool)
    List() []*Entry
}
