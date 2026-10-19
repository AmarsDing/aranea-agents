package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
)

// NewL2RecallStore creates a biz.L2RecallStore backed by data.
// Used by EpisodeConsolidator (Phase 6A-06) for reading L2 episodes.
func NewL2RecallStore(data *Data, vs vector.VectorStore) biz.L2RecallStore {
	return newL2EpisodeRepo(data, vs)
}

// NewL2EpisodeAdminReader creates a biz.L2EpisodeAdminReader backed by data.
// Used by the memory-center layer overview and unified graph assembly.
func NewL2EpisodeAdminReader(data *Data, vs vector.VectorStore) biz.L2EpisodeAdminReader {
	return newL2EpisodeRepo(data, vs)
}

// Compile-time interface checks.
var (
	_ biz.L2EpisodeWriter      = (*l2EpisodeRepo)(nil)
	_ biz.L2RecallStore        = (*l2EpisodeRepo)(nil)
	_ biz.L2EpisodeAdminReader = (*l2EpisodeRepo)(nil)
)

