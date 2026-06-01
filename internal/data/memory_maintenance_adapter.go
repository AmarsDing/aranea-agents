package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type memoryConsolidationWriterAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryConsolidationWriterAdapter(store *sessionmemory.Store) biz.MemoryConsolidationWriter {
	if store == nil {
		return nil
	}
	return &memoryConsolidationWriterAdapter{store: store}
}

func (a *memoryConsolidationWriterAdapter) UpsertFactsAndEpisodeBatch(ctx context.Context, facts []biz.MemoryFactWrite, ep *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	var dataFacts []sessionmemory.MemoryFactUpsert
	for _, f := range facts {
		dataFacts = append(dataFacts, bizFactWriteToData(f))
	}
	var dataEp *sessionmemory.EpisodeInsert
	if ep != nil {
		dataEp = bizEpisodeWriteToData(ep)
	}
	result, err := a.store.UpsertFactsAndEpisodeBatch(ctx, dataFacts, dataEp)
	if err != nil {
		return nil, err
	}
	return &biz.ConsolidationResult{
		FactRows:     result.FactRows,
		EpisodeRow:   result.EpisodeRow,
		FactsWritten: result.FactsWritten,
	}, nil
}

type memoryFactIndexMaintainerAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryFactIndexMaintainerAdapter(store *sessionmemory.Store) biz.MemoryFactIndexMaintainer {
	if store == nil {
		return nil
	}
	return &memoryFactIndexMaintainerAdapter{store: store}
}

func (a *memoryFactIndexMaintainerAdapter) ListStaleIndexFacts(ctx context.Context, maxAttempts, batchSize int) ([][]byte, error) {
	return a.store.ListStaleIndexFacts(ctx, maxAttempts, batchSize)
}

func (a *memoryFactIndexMaintainerAdapter) MarkFactIndexDisabled(ctx context.Context, factID string) error {
	return a.store.MarkFactIndexDisabled(ctx, factID)
}

type memoryEpisodeDecayerAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryEpisodeDecayerAdapter(store *sessionmemory.Store) biz.MemoryEpisodeDecayer {
	if store == nil {
		return nil
	}
	return &memoryEpisodeDecayerAdapter{store: store}
}

func (a *memoryEpisodeDecayerAdapter) ApplyEpisodeImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error) {
	return a.store.ApplyEpisodeImportanceDecay(ctx, agentID, cutoffRFC3339, factor)
}

func (a *memoryEpisodeDecayerAdapter) PurgeEpisodesOlderThan(ctx context.Context, agentID, cutoffRFC3339 string) (int, error) {
	return a.store.PurgeEpisodesOlderThan(ctx, agentID, cutoffRFC3339)
}

func (a *memoryEpisodeDecayerAdapter) ApplyAllEpisodeImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	return a.store.ApplyAllEpisodeImportanceDecay(ctx, cutoffRFC3339, factor)
}

type memoryFactDecayerAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryFactDecayerAdapter(store *sessionmemory.Store) biz.MemoryFactDecayer {
	if store == nil {
		return nil
	}
	return &memoryFactDecayerAdapter{store: store}
}

func (a *memoryFactDecayerAdapter) ApplyAgentFactImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error) {
	return a.store.ApplyAgentFactImportanceDecay(ctx, agentID, cutoffRFC3339, factor)
}

func (a *memoryFactDecayerAdapter) ApplyAllFactImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	return a.store.ApplyAllFactImportanceDecay(ctx, cutoffRFC3339, factor)
}

type memoryEpisodeBackfillReaderAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryEpisodeBackfillReaderAdapter(store *sessionmemory.Store) biz.MemoryEpisodeBackfillReader {
	if store == nil {
		return nil
	}
	return &memoryEpisodeBackfillReaderAdapter{store: store}
}

func (a *memoryEpisodeBackfillReaderAdapter) ListEpisodesPendingEmbedding(ctx context.Context, limit int) ([]biz.EpisodeEmbedCandidate, error) {
	cands, err := a.store.ListEpisodesPendingEmbedding(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]biz.EpisodeEmbedCandidate, len(cands))
	for i, c := range cands {
		out[i] = biz.EpisodeEmbedCandidate{
			ID:      c.ID,
			AgentID: c.AgentID,
			Title:   c.Title,
			Summary: c.Summary,
		}
	}
	return out, nil
}

type memoryLegacyMigratorAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryLegacyMigratorAdapter(store *sessionmemory.Store) biz.MemoryLegacyMigrator {
	if store == nil {
		return nil
	}
	return &memoryLegacyMigratorAdapter{store: store}
}

func (a *memoryLegacyMigratorAdapter) RunLegacyMigration(ctx context.Context) (int, bool, error) {
	return RunLegacyTRPCMemoryMigration(ctx, a.store)
}

func (a *memoryLegacyMigratorAdapter) LegacyMigrationVersion() int {
	return MigrationLegacyTRPCMemoryFacts
}

func bizFactWriteToData(f biz.MemoryFactWrite) sessionmemory.MemoryFactUpsert {
	return sessionmemory.MemoryFactUpsert{
		ScopeType:       f.ScopeType,
		ScopeID:         f.ScopeID,
		UserID:          f.UserID,
		AgentID:         f.AgentID,
		Statement:       f.Statement,
		DetailsMarkdown: f.DetailsMarkdown,
		FactKind:        f.FactKind,
		TagsJSON:        f.TagsJSON,
		Confidence:      f.Confidence,
		Importance:      f.Importance,
		SourceKind:      f.SourceKind,
		SourceSessionID: f.SourceSessionID,
		SourceMessageID: f.SourceMessageID,
		Status:          f.Status,
		MetadataJSON:    f.MetadataJSON,
	}
}

func bizEpisodeWriteToData(ep *biz.EpisodeWrite) *sessionmemory.EpisodeInsert {
	return &sessionmemory.EpisodeInsert{
		SessionID:      ep.SessionID,
		AgentID:        ep.AgentID,
		UserID:         ep.UserID,
		Title:          ep.Title,
		OutcomeSummary: ep.OutcomeSummary,
		Importance:     ep.Importance,
		MessageCount:   ep.MessageCount,
		ConsolidatedL3: ep.ConsolidatedL3,
		MetadataJSON:   ep.MetadataJSON,
	}
}
