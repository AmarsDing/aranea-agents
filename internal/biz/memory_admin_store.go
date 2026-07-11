package biz

import (
	"context"

	"aranea-agents/pkg/apierror"
)

// DefaultFactBruteForceThreshold is the maximum number of facts below which
// linear scan is used instead of vector similarity search.
const DefaultFactBruteForceThreshold = 5000

// ErrL1BudgetOverflow is returned when an L1 field upsert would exceed the task's budget_tokens.
var ErrL1BudgetOverflow = apierror.BadRequest(apierror.DomainMemory, "L1 budget overflow: field would exceed task budget_tokens")

// ValidFieldKinds lists all valid field_kind enum values.
var ValidFieldKinds = []string{
	"string", "number", "boolean", "json", "reference", "markdown",
	"decision", "artifact", "progress", "constraint",
}

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
	PIITypes              []string
	OriginalStatement     string // preserved original when PII redaction replaces Statement
	MetadataJSON          string
	CreatedAt             string
	UpdatedAt             string
	// Bi-temporal validity (P3-8). ValidFrom marks when the fact became
	// effective; ValidUntil marks when it was superseded. Empty ValidUntil
	// means the fact is currently valid. When a conflict is detected the
	// old fact is invalidated (ValidUntil set) rather than deleted.
	ValidFrom  string
	ValidUntil string
	// Links/Keywords (P3-12): A-MEM style memory linking and evolution.
	// LinksJSON stores related memory IDs as a JSON array. KeywordsJSON
	// stores LLM-generated keywords as a JSON array. Both default to "[]".
	LinksJSON    string
	KeywordsJSON string
	// ContextNote (Phase 6A-03): A-MEM style contextual annotation explaining
	// how this fact relates to or evolved from related memories. LLM-generated
	// during link evolution when a RELATED_TO/EVOLVED_FROM relationship is
	// established. Empty string means no evolution context has been attached.
	ContextNote string
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
	ListL0SnapshotRows(ctx context.Context, sessionID, agentID string, limit int32) ([][]byte, error)
	GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error)
	InsertL0AssemblySnapshot(ctx context.Context, in L0AssemblySnapshotInsert) error
	UpdateL0SnapshotActual(ctx context.Context, id, sessionID string, actualPromptTokens, contextWindowTokens int) error
}

// L1AdminReader lists L1 working-memory tasks and fields.
type L1AdminReader interface {
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool, requestingAgentID ...string) ([][]byte, error)
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
	ID             string
	TaskID         string
	SessionID      string
	AgentID        string
	FieldPath      string
	FieldKind      string
	Visibility     string
	PinToPrompt    bool
	IsRequired     bool
	ValueText      string
	ValueJSON      string
	ValueRef       string
	Preview        string
	TokenEstimate  int
	Source         string
	SourceRef      string
	TTLSeconds     int
	ChangedBy      string
	HistoryEnabled bool // whether to archive old value to field_history
}

// L1TaskWriter exposes L1 task write operations.
type L1TaskWriter interface {
	StartL1Task(ctx context.Context, in L1TaskInsert) ([]byte, error)
	EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error)
	GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error)
	ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error)
	UnarchiveL1Task(ctx context.Context, sessionID, taskID string) error
	// ArchiveAndCreateEpisodeTx atomically archives an L1 task and creates
	// the corresponding L2 episode within a single database transaction.
	// If the episode insert fails, the L1 archive is rolled back automatically.
	ArchiveAndCreateEpisodeTx(ctx context.Context, sessionID, taskID string, episode L1ArchiveEpisodeInsert) ([]byte, error)
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

// L1ExpiredFieldCleaner batch-deletes expired L1 fields (where expires_at != ''
// and expires_at < now). Used by the auto-archive worker for periodic cleanup
// so expired fields don't accumulate in the database.
type L1ExpiredFieldCleaner interface {
	DeleteExpiredL1Fields(ctx context.Context) (int, error)
}

// L1SchemaReader reads L1 schema definitions from memory_l1_schemas.
type L1SchemaReader interface {
	GetL1SchemaRow(ctx context.Context, schemaID string) ([]byte, error)
}

// L2EpisodeWriter inserts L2 episodes (used by L1 archive hook).
type L2EpisodeWriter interface {
	InsertL1ArchiveEpisode(ctx context.Context, in L1ArchiveEpisodeInsert) error
}

// RecentMessageLister lists recent messages for a session (used by Path B extraction).
// Implementations convert from the storage-level message type to ConsolidateMessage.
type RecentMessageLister interface {
	ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]ConsolidateMessage, error)
}

// L1ArchiveEpisodeInsert is the domain-level DTO for inserting an L2 episode from an L1 archive.
type L1ArchiveEpisodeInsert struct {
	SessionID      string
	AgentID        string
	TaskID         string
	TaskTitle      string
	Status         string
	L1SnapshotJSON string
	// Path A structured fields
	Goal             string
	Outcome          string
	OutcomeSummary   string
	KeyDecisionsJSON string
	KeyArtifactsJSON string
	EpisodeKind      string
	Importance       float64
	Confidence       float64
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
	// ListFactRowsForUserAll returns facts for a user including invalidated
	// ones (valid_until != ''). Used for historical reconstruction queries
	// when SearchOptions.IncludeInvalidated is true.
	ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error)
	GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error)
	RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
}

// L3FactWriter exposes write operations for semantic facts.
type L3FactWriter interface {
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
	DeleteFactRow(ctx context.Context, factID string) error
	DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)
	// ClearFactsByScope soft-deletes all active facts matching the given scope
	// and cascades the deletion to the pgvector index. It returns the IDs of
	// the deleted facts so callers can perform additional cleanup if needed.
	ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error)
	// InvalidateFact marks a fact as no longer valid by setting valid_until
	// to the current time (bi-temporal validity, P3-8). The fact row is
	// preserved for historical reconstruction queries. Returns the raw
	// updated row as JSON (same shape as UpsertFactRow) so callers can
	// sync downstream indexes.
	InvalidateFact(ctx context.Context, factID string) ([]byte, error)
	// InvalidateAndUpsertFactTx atomically invalidates the old fact (by setting
	// valid_until) and upserts the new fact in a single transaction (P0-2 fix).
	// This ensures bi-temporal consistency: either both operations succeed or
	// neither does, preventing data loss (invalidate succeeds but upsert fails)
	// and duplicate active facts (invalidate fails silently but upsert succeeds).
	// If oldFactID is empty, only the upsert is performed (no invalidation).
	// Returns the upserted fact row as JSON.
	InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in FactUpsert) ([]byte, error)
}

// DecayScoreWriter updates the persisted Ebbinghaus decay score (R_t) for
// memory facts. The cron job (MemoryEbbinghausDecayWorker) computes R_t per
// fact and writes it back via this port so that fused recall can down-weight
// forgotten memories without recomputing the score on every recall.
// Stability:evolving
type DecayScoreWriter interface {
	// UpdateDecayScores batch-updates decay_score for the given fact IDs.
	// scores maps fact ID → R_t ∈ (0, 1]. Implementations must wrap the
	// updates in a single transaction so the batch is atomic.
	UpdateDecayScores(ctx context.Context, scores map[string]float64) error
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
	EvolutionMetricsJSON(ctx context.Context, agentID string, timeRange string) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
}

// L3ConflictStore manages L3 fact conflict detection.
type L3ConflictStore interface {
	IncrementConflictCount(ctx context.Context, factID string) (int32, error)
	ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error)
	// BatchIncrementConflictCounts increments conflict_count for multiple facts in a single query.
	BatchIncrementConflictCounts(ctx context.Context, factIDs []string) error
}

// MemoryAdminDeps composes the fine-grained admin store interfaces required by MemoryAdminUsecase.
// Unlike SessionAdminStore, this does not include L2RecallStore or L3FactWriter
// (which are injected via separate fields). This narrows the interface to only
// what MemoryAdminUsecase actually needs.
type MemoryAdminDeps interface {
	L0AdminStore
	L1AdminReader
	L1TaskWriter
	L1FieldWriter
	L1IdleTaskReader
	L1ExpiredFieldCleaner
	L2EpisodeWriter
	L3FactReader
	L3ConflictStore
	PIIReviewStore
	L4EntityStore
	L4EvolutionStore
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
	MemoryAdminDeps
	L2RecallStore
	L3FactWriter
}
