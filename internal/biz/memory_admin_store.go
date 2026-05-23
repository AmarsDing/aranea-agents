package biz

import (
	"context"
)

// FactUpsert is the domain-level DTO for upserting memory facts.
type FactUpsert struct {
	ID                    string
	ScopeType             string
	ScopeID               string
	WorkspaceID           string
	UserID                string
	TeamID                string
	AgentID               string
	Statement             string
	Fingerprint           string
	DetailsMarkdown       string
	FactKind              string
	TagsJSON              string
	Confidence            float64
	Importance            float64
	UseCount              int32
	HitCount              int32
	PositiveFeedbackCount int32
	NegativeFeedbackCount int32
	ConflictCount         int32
	SourceKind            string
	SourceEpisodeID       string
	SourceSessionID       string
	SourceMessageID       string
	SourceExternal        string
	Version               int32
	Status                string
	PIIFlag               bool
	MetadataJSON          string
	CreatedAt             string
	UpdatedAt             string
}

// EvolutionEventInsert is the domain-level DTO for inserting evolution events.
type EvolutionEventInsert struct {
	AgentID       string
	WorkspaceID   string
	EventKind     string
	Kind          string
	TargetField   string
	Reason        string
	TriggerKind   string
	TriggerSource string
	MetadataJSON  string
}

type SessionAdminStore interface {
	ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error)
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error)
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
	ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32) ([]byte, error)
	AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error)
	AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error)
	EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error)
	EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error)
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}
