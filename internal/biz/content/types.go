package content

import "time"

const (
	StatusCandidate = "candidate"
	StatusSelected  = "selected"
	StatusDrafting  = "drafting"
	StatusInReview  = "in_review"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusArchived  = "archived"

	SourceRSS     = "rss"
	SourceSearch  = "search"
	SourceManual  = "manual"
	SourceConnector = "connector"
)

// Item is a candidate or in-pipeline content unit (MVP ContentItem).
type Item struct {
	ID               string
	WorkspaceID      string
	OrgKey           string
	SourceType       string
	SourceURL        string
	Title            string
	Body             string
	Lang             string
	MediaArtifactIDs []string
	DedupeHash       string
	HotScore         float64
	Status           string
	RawPayload       string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ListFilter scopes ContentItem queries.
type ListFilter struct {
	WorkspaceID string
	SourceType  string
	Status      string
	Limit       int
	Offset      int
}

// Repo persists ContentItems.
type Repo interface {
	Create(item Item) (Item, error)
	Get(id string) (Item, error)
	Update(item Item) (Item, error)
	List(f ListFilter) ([]Item, int, error)
	GetByDedupeHash(workspaceID, hash string) (Item, bool, error)
}
