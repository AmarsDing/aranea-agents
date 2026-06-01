package biz

import "context"

type MemoryFactWrite struct {
	ScopeType       string
	ScopeID         string
	UserID          string
	AgentID         string
	Statement       string
	DetailsMarkdown string
	FactKind        string
	TagsJSON        string
	Confidence      float64
	Importance      float64
	SourceKind      string
	SourceSessionID string
	SourceMessageID string
	Status          string
	MetadataJSON    string
}

type EpisodeWrite struct {
	SessionID      string
	AgentID        string
	UserID         string
	Title          string
	OutcomeSummary string
	Importance     float64
	MessageCount   int
	ConsolidatedL3 int
	MetadataJSON   string
}

type ConsolidationResult struct {
	FactRows     [][]byte
	EpisodeRow   []byte
	FactsWritten int
}

type EpisodeEmbedCandidate struct {
	ID      string
	AgentID string
	Title   string
	Summary string
}

type MemoryConsolidationWriter interface {
	UpsertFactsAndEpisodeBatch(ctx context.Context, facts []MemoryFactWrite, ep *EpisodeWrite) (*ConsolidationResult, error)
}

type MemoryFactIndexMaintainer interface {
	ListStaleIndexFacts(ctx context.Context, maxAttempts, batchSize int) ([][]byte, error)
	MarkFactIndexDisabled(ctx context.Context, factID string) error
}

type MemoryEpisodeDecayer interface {
	ApplyEpisodeImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error)
	PurgeEpisodesOlderThan(ctx context.Context, agentID, cutoffRFC3339 string) (int, error)
	ApplyAllEpisodeImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error)
}

type MemoryFactDecayer interface {
	ApplyAgentFactImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error)
	ApplyAllFactImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error)
}

type MemoryEpisodeBackfillReader interface {
	ListEpisodesPendingEmbedding(ctx context.Context, limit int) ([]EpisodeEmbedCandidate, error)
}

type MemoryLegacyMigrator interface {
	RunLegacyMigration(ctx context.Context) (migrated int, skipped bool, err error)
	LegacyMigrationVersion() int
}

type MemoryFactEntry struct {
	Statement  string
	Scope      string
	Confidence float64
}

type MemoryFactReader interface {
	ReadSessionMemoryFacts(ctx context.Context, sessionID string) ([]MemoryFactEntry, error)
}
