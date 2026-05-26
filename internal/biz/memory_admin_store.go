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

// L0AdminStore lists and persists L0 assembly snapshots.
type L0AdminStore interface {
	ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error)
	InsertL0AssemblySnapshot(ctx context.Context, in L0AssemblySnapshotInsert) error
	UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error
}

// L1AdminReader lists L1 working-memory tasks and fields.
type L1AdminReader interface {
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error)
}

// L2RecallStore retrieves episodic (L2) memories for prompt injection and admin.
type L2RecallStore interface {
	ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error)
	RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error)
}

// L3FactAdminStore manages semantic facts and L3 recall.
type L3FactAdminStore interface {
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
	ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error)
	RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
}

// L4GraphAdminStore exposes L4 graph entities, neighborhood, and agent evolution rows.
type L4GraphAdminStore interface {
	ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error)
	AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error)
	AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error)
	EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error)
	EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

// SessionAdminStore is the composed admin port for L0–L4 session memory (typed sub-interfaces are preferred for new code).
type SessionAdminStore interface {
	L0AdminStore
	L1AdminReader
	L2RecallStore
	L3FactAdminStore
	L4GraphAdminStore
}
