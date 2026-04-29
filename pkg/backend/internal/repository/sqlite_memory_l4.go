package repository

import (
	mem "arenea/backend/internal/memory/domain"

	memsqlite "arenea/backend/internal/memory/adapters/sqlite"
)

func (r *SQLiteRepository) memL4() *memsqlite.L4Repository {
	return memsqlite.NewL4Repository(r.db)
}

func (r *SQLiteRepository) UpsertEntity(e mem.MemoryEntity) (mem.MemoryEntity, error) { return r.memL4().UpsertEntity(e) }
func (r *SQLiteRepository) GetEntity(id string) (mem.MemoryEntity, error)              { return r.memL4().GetEntity(id) }
func (r *SQLiteRepository) GetEntityByName(scope mem.ScopeType, scopeID string, t mem.EntityType, normalized string) (mem.MemoryEntity, error) {
	return r.memL4().GetEntityByName(scope, scopeID, t, normalized)
}
func (r *SQLiteRepository) ListEntities(q EntityListQuery) ([]mem.MemoryEntity, int, error) { return r.memL4().ListEntities(q) }
func (r *SQLiteRepository) UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt string) error {
	return r.memL4().UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt)
}
func (r *SQLiteRepository) UpdateEntityName(id, name, normalized string) error { return r.memL4().UpdateEntityName(id, name, normalized) }
func (r *SQLiteRepository) UpsertEntityFact(entityID, factID string, weight float64) error {
	return r.memL4().UpsertEntityFact(entityID, factID, weight)
}
func (r *SQLiteRepository) ListFactsForEntity(entityID string, limit int) ([]mem.MemoryEntityFactLink, error) {
	return r.memL4().ListFactsForEntity(entityID, limit)
}
func (r *SQLiteRepository) InsertEntityVersion(v mem.MemoryEntityVersion) error { return r.memL4().InsertEntityVersion(v) }
func (r *SQLiteRepository) ListEntityVersions(entityID string, limit int) ([]mem.MemoryEntityVersion, error) {
	return r.memL4().ListEntityVersions(entityID, limit)
}
func (r *SQLiteRepository) BumpEntityUseCount(id string, atISO string) error { return r.memL4().BumpEntityUseCount(id, atISO) }
func (r *SQLiteRepository) UpsertRelation(rel mem.MemoryRelation) (mem.MemoryRelation, error) {
	return r.memL4().UpsertRelation(rel)
}
func (r *SQLiteRepository) GetRelation(id string) (mem.MemoryRelation, error) { return r.memL4().GetRelation(id) }
func (r *SQLiteRepository) ListRelationsForNode(nodeID string, limit int) ([]mem.MemoryRelation, error) {
	return r.memL4().ListRelationsForNode(nodeID, limit)
}
func (r *SQLiteRepository) UpdateRelationStatus(id, status, archivedAt, deletedAt string) error {
	return r.memL4().UpdateRelationStatus(id, status, archivedAt, deletedAt)
}
func (r *SQLiteRepository) BumpRelationUseCount(id string, atISO string) error { return r.memL4().BumpRelationUseCount(id, atISO) }
func (r *SQLiteRepository) GetNeighborhood(centerID string, hops, maxNodes int) (mem.GraphNeighborhood, error) {
	return r.memL4().GetNeighborhood(centerID, hops, maxNodes)
}
