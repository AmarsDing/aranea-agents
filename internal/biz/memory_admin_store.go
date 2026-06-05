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
	GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error)
	InsertL0AssemblySnapshot(ctx context.Context, in L0AssemblySnapshotInsert) error
	UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error
}

// L1AdminReader lists L1 working-memory tasks and fields.
type L1AdminReader interface {
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error)
	GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error)
	GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error)
}

// L1TaskInsert is the domain-level DTO for creating L1 tasks.
type L1TaskInsert struct {
	ID           string
	SessionID    string
	RunID        string
	TeamID       string
	AgentID      string
	TaskKey      string
	TaskTitle    string
	TaskGoal     string
	BudgetTokens int
	ParentTaskID string
}

// L1FieldInsert is the domain-level DTO for upserting L1 fields.
type L1FieldInsert struct {
	ID            string
	TaskID        string
	SessionID     string
	AgentID       string
	FieldPath     string
	FieldKind     string
	Visibility    string
	PinToPrompt   bool
	IsRequired    bool
	ValueText     string
	ValueJSON     string
	ValueRef      string
	Preview       string
	TokenEstimate int
	Source        string
	SourceRef     string
	TTLSeconds    int
	ChangedBy     string
}

// L1TaskWriter exposes L1 task write operations.
type L1TaskWriter interface {
	StartL1Task(ctx context.Context, in L1TaskInsert) ([]byte, error)
	EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error)
	GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error)
	ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error)
}

// L1FieldWriter exposes L1 field write operations.
type L1FieldWriter interface {
	UpsertL1Field(ctx context.Context, in L1FieldInsert) ([]byte, error)
	DeleteL1Field(ctx context.Context, taskID, fieldPath string) error
	GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error)
	PatchL1Fields(ctx context.Context, fields []L1FieldInsert) ([][]byte, error)
}

// L1IdleTaskReader lists idle L1 tasks for the auto-archive worker.
type L1IdleTaskReader interface {
	ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error)
}

// L2EpisodeWriter inserts L2 episodes (used by L1 archive hook).
type L2EpisodeWriter interface {
	InsertL1ArchiveEpisode(ctx context.Context, in L1ArchiveEpisodeInsert) error
}

// L2ConsolidationStore manages episode consolidation.
type L2ConsolidationStore interface {
	ListPendingConsolidationEpisodes(ctx context.Context, agentID string, limit int) ([][]byte, error)
	MarkEpisodeConsolidated(ctx context.Context, id string, l3Count, l4Count int) error
}

// L1ArchiveEpisodeInsert is the domain-level DTO for inserting an L2 episode from an L1 archive.
type L1ArchiveEpisodeInsert struct {
	SessionID      string
	AgentID        string
	TaskID         string
	TaskTitle      string
	Status         string
	L1SnapshotJSON string
}

// L2RecallStore retrieves episodic (L2) memories for prompt injection and admin.
type L2RecallStore interface {
	ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error)
	RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error)
}

// L3FactReader exposes read operations for semantic facts.
type L3FactReader interface {
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
	ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error)
	RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
}

// L3FactWriter exposes write operations for semantic facts.
type L3FactWriter interface {
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
	DeleteFactRow(ctx context.Context, factID string) error
	DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)
}

// L3FactAdminStore manages semantic facts and L3 recall.
//
// Deprecated: Use L3FactReader and L3FactWriter directly instead.
type L3FactAdminStore interface {
	L3FactReader
	L3FactWriter
}

// PIIReviewStore manages PII-flagged fact review.
type PIIReviewStore interface {
	ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error)
	ApprovePIIFact(ctx context.Context, factID string) error
	RejectPIIFact(ctx context.Context, factID string) error
}

// L4EntityStore exposes L4 entity and graph operations.
type L4EntityStore interface {
	ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error)
	AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error)
	AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error)
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

// L4EvolutionStore exposes L4 evolution operations.
type L4EvolutionStore interface {
	EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error)
	EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
}

// L3ConflictStore manages L3 fact conflict detection.
type L3ConflictStore interface {
	IncrementConflictCount(ctx context.Context, factID string) (int32, error)
	ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error)
}

// SessionAdminStore is the composed admin port for L0–L4 session memory.
//
// Deprecated: This composed interface has grown too large. New code should depend on
// the fine-grained sub-interfaces (L0AdminStore, L1AdminReader, L1TaskWriter, etc.)
// directly rather than embedding SessionAdminStore. It is retained only for backward
// compatibility with existing Wire providers.
//
// Deprecated: Use fine-grained sub-interfaces (L0AdminStore, L1AdminReader, L2RecallStore, etc.)
// instead of this aggregate. This interface is retained only for Wire binding convenience.
type SessionAdminStore interface {
	L0AdminStore
	L1AdminReader
	L1TaskWriter
	L1FieldWriter
	L1IdleTaskReader
	L2RecallStore
	L2EpisodeWriter
	L2ConsolidationStore
	L3FactAdminStore
	L3ConflictStore
	PIIReviewStore
	L4EntityStore
	L4EvolutionStore
}
