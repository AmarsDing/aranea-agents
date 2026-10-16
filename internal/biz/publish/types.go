package publish

import "time"

const (
	ModePackage = "package"
	ModeAPI     = "api"

	StatusPending    = "pending"
	StatusPackaging  = "packaging"
	StatusPackaged   = "packaged"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

// Job is a Publish Outbox row (NOT event_delivery_outbox).
type Job struct {
	ID              string
	ReviewTaskID    string
	ContentItemID   string
	Platform        string
	Mode            string
	Status          string
	Attempt         int
	LastError       string
	ArtifactID      string
	PackageDir      string
	IdempotencyKey  string
	Title           string
	Body            string
	Tags            []string
	MediaPaths      []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PackageManifest is written into the release package (mode A).
type PackageManifest struct {
	SchemaVersion   string            `json:"schema_version"`
	Platform        string            `json:"platform"`
	Mode            string            `json:"mode"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	Tags            []string          `json:"tags"`
	MediaPaths      []string          `json:"media_paths"`
	UTM             map[string]string `json:"utm"`
	ComplianceNote  string            `json:"compliance_note"`
	ReviewTaskID    string            `json:"review_task_id"`
	ContentItemID   string            `json:"content_item_id"`
	StatusSemantics string            `json:"status_semantics"` // packaged != posted_to_platform
}

type Repo interface {
	Create(j Job) (Job, error)
	Get(id string) (Job, error)
	Update(j Job) (Job, error)
	ListByStatus(status string, limit int) ([]Job, error)
	GetByIdempotency(key string) (Job, bool, error)
}
