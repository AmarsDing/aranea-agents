package repository

import (
	mem "arenea/backend/internal/memory/domain"

	memsqlite "arenea/backend/internal/memory/adapters/sqlite"
)

func (r *SQLiteRepository) memL3() *memsqlite.L3Repository {
	return memsqlite.NewL3Repository(r.db)
}

func (r *SQLiteRepository) CreateFact(f mem.MemoryFact) (mem.MemoryFact, error) { return r.memL3().CreateFact(f) }
func (r *SQLiteRepository) UpdateFact(f mem.MemoryFact) error                         { return r.memL3().UpdateFact(f) }
func (r *SQLiteRepository) GetFact(id string) (mem.MemoryFact, error)              { return r.memL3().GetFact(id) }
func (r *SQLiteRepository) GetFactByFingerprint(scopeType mem.ScopeType, scopeID, fp string) (mem.MemoryFact, error) {
	return r.memL3().GetFactByFingerprint(scopeType, scopeID, fp)
}
func (r *SQLiteRepository) ListFacts(q FactListQuery) ([]mem.MemoryFact, int, error) { return r.memL3().ListFacts(q) }
func (r *SQLiteRepository) UpdateFactConfidence(id string, newConfidence float64, hitInc, posInc, negInc int) error {
	return r.memL3().UpdateFactConfidence(id, newConfidence, hitInc, posInc, negInc)
}
func (r *SQLiteRepository) UpdateFactStatus(id, status, supersededBy, archivedAt string) error {
	return r.memL3().UpdateFactStatus(id, status, supersededBy, archivedAt)
}
func (r *SQLiteRepository) BumpFactUseStat(id string, hit bool, atISO string) error {
	return r.memL3().BumpFactUseStat(id, hit, atISO)
}
func (r *SQLiteRepository) InsertFactVersion(fv mem.FactVersion) error { return r.memL3().InsertFactVersion(fv) }
func (r *SQLiteRepository) ListFactVersions(factID string, limit int) ([]mem.FactVersion, error) {
	return r.memL3().ListFactVersions(factID, limit)
}
func (r *SQLiteRepository) GetFactVersion(factID string, version int) (mem.FactVersion, error) {
	return r.memL3().GetFactVersion(factID, version)
}
func (r *SQLiteRepository) InsertFactFeedback(fb mem.FactFeedback) (mem.FactFeedback, error) {
	return r.memL3().InsertFactFeedback(fb)
}
func (r *SQLiteRepository) ListFactFeedback(factID string, limit int) ([]mem.FactFeedback, error) {
	return r.memL3().ListFactFeedback(factID, limit)
}
func (r *SQLiteRepository) CountRecentFactFeedback(factID, feedbackType string, limit int) (int, error) {
	return r.memL3().CountRecentFactFeedback(factID, feedbackType, limit)
}
func (r *SQLiteRepository) CountAgentFactFeedbackSince(agentID string, feedbackTypes []string, since string) (int, error) {
	return r.memL3().CountAgentFactFeedbackSince(agentID, feedbackTypes, since)
}
func (r *SQLiteRepository) UpsertFactConflict(c mem.FactConflict) (mem.FactConflict, error) {
	return r.memL3().UpsertFactConflict(c)
}
func (r *SQLiteRepository) GetFactConflict(id string) (mem.FactConflict, error) { return r.memL3().GetFactConflict(id) }
func (r *SQLiteRepository) ListOpenFactConflicts(scope mem.ScopeType, scopeID string, limit int) ([]mem.FactConflict, error) {
	return r.memL3().ListOpenFactConflicts(scope, scopeID, limit)
}
func (r *SQLiteRepository) UpdateFactConflictResolution(id, status, resolution, by, resolvedAt string) error {
	return r.memL3().UpdateFactConflictResolution(id, status, resolution, by, resolvedAt)
}
func (r *SQLiteRepository) UpsertFactEmbedding(id, model string, dim int, blob []byte, norm float64) error {
	return r.memL3().UpsertFactEmbedding(id, model, dim, blob, norm)
}
func (r *SQLiteRepository) UpsertFactsFTS(factID string, scopeType mem.ScopeType, scopeID, kind, text string) error {
	return r.memL3().UpsertFactsFTS(factID, scopeType, scopeID, kind, text)
}
func (r *SQLiteRepository) DeleteFactIndex(factID string) error { return r.memL3().DeleteFactIndex(factID) }
func (r *SQLiteRepository) SearchFactsBM25(scopes []mem.ScopeType, scopeIDs []string, query string, limit int) ([]mem.FactRecallHit, error) {
	return r.memL3().SearchFactsBM25(scopes, scopeIDs, query, limit)
}
func (r *SQLiteRepository) SearchFactsVector(scopes []mem.ScopeType, scopeIDs []string, q []float32, limit int) ([]mem.FactRecallHit, error) {
	return r.memL3().SearchFactsVector(scopes, scopeIDs, q, limit)
}
func (r *SQLiteRepository) ListFactsDueForDecay(before string, limit int) ([]mem.MemoryFact, error) {
	return r.memL3().ListFactsDueForDecay(before, limit)
}
func (r *SQLiteRepository) ApplyFactDecay(factID string, factor float64, nextAt string) error {
	return r.memL3().ApplyFactDecay(factID, factor, nextAt)
}
func (r *SQLiteRepository) ArchiveFactsBelowConfidence(threshold float64, limit int) (int, error) {
	return r.memL3().ArchiveFactsBelowConfidence(threshold, limit)
}
func (r *SQLiteRepository) CountFactsByStatus(scope mem.ScopeType, scopeID string) (map[string]int, error) {
	return r.memL3().CountFactsByStatus(scope, scopeID)
}
