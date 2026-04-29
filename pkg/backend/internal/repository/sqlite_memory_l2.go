package repository

import (
	mem "arenea/backend/internal/memory/domain"

	memsqlite "arenea/backend/internal/memory/adapters/sqlite"
)

func (r *SQLiteRepository) memL2() *memsqlite.L2Repository {
	return memsqlite.NewL2Repository(r.db)
}

func (r *SQLiteRepository) CreateEpisode(e mem.MemoryEpisode) (mem.MemoryEpisode, error) { return r.memL2().CreateEpisode(e) }
func (r *SQLiteRepository) UpdateEpisode(e mem.MemoryEpisode) error                    { return r.memL2().UpdateEpisode(e) }
func (r *SQLiteRepository) GetEpisode(id string) (mem.MemoryEpisode, error) { return r.memL2().GetEpisode(id) }
func (r *SQLiteRepository) ListEpisodes(sessionID, kind string, limit, offset int) ([]mem.MemoryEpisode, int, error) {
	return r.memL2().ListEpisodes(sessionID, kind, limit, offset)
}
func (r *SQLiteRepository) ListPendingConsolidation(minImportance float64, limit int) ([]mem.MemoryEpisode, error) {
	return r.memL2().ListPendingConsolidation(minImportance, limit)
}
func (r *SQLiteRepository) UpdateEpisodeConsolidationStatus(id, status string, l3Count, l4Count int) error {
	return r.memL2().UpdateEpisodeConsolidationStatus(id, status, l3Count, l4Count)
}
func (r *SQLiteRepository) UpdateEpisodeEmbedding(id, status, model string, dim int, norm float64) error {
	return r.memL2().UpdateEpisodeEmbedding(id, status, model, dim, norm)
}
func (r *SQLiteRepository) SoftDeleteEpisode(id string) error { return r.memL2().SoftDeleteEpisode(id) }
func (r *SQLiteRepository) UpsertL2Index(entry mem.MemoryL2IndexEntry, text string) error {
	return r.memL2().UpsertL2Index(entry, text)
}
func (r *SQLiteRepository) DeleteL2Index(episodeID string) error { return r.memL2().DeleteL2Index(episodeID) }
func (r *SQLiteRepository) SearchL2BM25(sessionID, query string, minImportance float64, limit int) ([]mem.MemoryL2RecallResult, error) {
	return r.memL2().SearchL2BM25(sessionID, query, minImportance, limit)
}
func (r *SQLiteRepository) UpsertEventMark(m mem.MemoryEventMark) (mem.MemoryEventMark, error) {
	return r.memL2().UpsertEventMark(m)
}
func (r *SQLiteRepository) SoftDeleteEventMark(id string) error { return r.memL2().SoftDeleteEventMark(id) }
func (r *SQLiteRepository) ListEventMarks(sessionID, markType string, limit int) ([]mem.MemoryEventMark, error) {
	return r.memL2().ListEventMarks(sessionID, markType, limit)
}
func (r *SQLiteRepository) ListMarksForEpisode(episodeID string) ([]mem.MemoryEventMark, error) {
	return r.memL2().ListMarksForEpisode(episodeID)
}
func (r *SQLiteRepository) ListL2Events(q mem.MemoryL2EventQuery) ([]mem.MemoryL2Event, int, error) {
	return r.memL2().ListL2Events(q)
}
func (r *SQLiteRepository) ArchiveEpisodesBeforeDate(sessionID, before string) (int, error) {
	return r.memL2().ArchiveEpisodesBeforeDate(sessionID, before)
}
func (r *SQLiteRepository) CountAgentEpisodesSince(agentID, since string) (int, error) {
	return r.memL2().CountAgentEpisodesSince(agentID, since)
}
func (r *SQLiteRepository) DeleteArchivedEpisodesBefore(before string) (int, error) {
	return r.memL2().DeleteArchivedEpisodesBefore(before)
}
