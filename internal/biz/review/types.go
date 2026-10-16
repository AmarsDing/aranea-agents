package review

import "time"

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// Task is a single-person final review gate over draft content.
type Task struct {
	ID              string
	ContentItemID   string
	DraftArtifactID string
	PlatformTargets []string
	MachineFlags    []string
	DraftTitle      string
	DraftBody       string
	Status          string
	DecisionNote    string
	ReviewerID      string
	DecidedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Repo interface {
	Create(t Task) (Task, error)
	Get(id string) (Task, error)
	Update(t Task) (Task, error)
	ListPending(limit, offset int) ([]Task, int, error)
}
