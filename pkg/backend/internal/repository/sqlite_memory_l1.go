package repository

import (
	mem "arenea/backend/internal/memory/domain"

	memsqlite "arenea/backend/internal/memory/adapters/sqlite"
)

func (r *SQLiteRepository) memL1() *memsqlite.L1Repository {
	return memsqlite.NewL1Repository(r.db)
}

func (r *SQLiteRepository) CreateL1Task(t mem.MemoryL1Task) (mem.MemoryL1Task, error) {
	return r.memL1().CreateL1Task(t)
}
func (r *SQLiteRepository) UpdateL1TaskStatus(taskID string, status mem.L1TaskStatus, endedAt string, archivedAt string) error {
	return r.memL1().UpdateL1TaskStatus(taskID, status, endedAt, archivedAt)
}
func (r *SQLiteRepository) UpdateL1TaskUsedTokens(taskID string, usedTokens int) error {
	return r.memL1().UpdateL1TaskUsedTokens(taskID, usedTokens)
}
func (r *SQLiteRepository) UpdateL1TaskShared(taskID string, shared []mem.L1FieldShare) error {
	return r.memL1().UpdateL1TaskShared(taskID, shared)
}
func (r *SQLiteRepository) UpdateL1TaskBudget(taskID string, budgetTokens int) error {
	return r.memL1().UpdateL1TaskBudget(taskID, budgetTokens)
}
func (r *SQLiteRepository) GetL1TaskByID(taskID string) (mem.MemoryL1Task, error) { return r.memL1().GetL1TaskByID(taskID) }
func (r *SQLiteRepository) GetL1TaskByKey(sessionID, taskKey, agentID string) (mem.MemoryL1Task, error) {
	return r.memL1().GetL1TaskByKey(sessionID, taskKey, agentID)
}
func (r *SQLiteRepository) ListL1TasksBySession(query mem.L1TaskListQuery) ([]mem.MemoryL1Task, error) {
	return r.memL1().ListL1TasksBySession(query)
}
func (r *SQLiteRepository) ArchiveIdleL1Tasks(before string) (int, error) { return r.memL1().ArchiveIdleL1Tasks(before) }
func (r *SQLiteRepository) UpsertL1Field(f mem.MemoryL1Field, history mem.MemoryL1FieldHistory, keepRevisions int) (mem.MemoryL1Field, error) {
	return r.memL1().UpsertL1Field(f, history, keepRevisions)
}
func (r *SQLiteRepository) GetL1Field(taskID, fieldPath string) (mem.MemoryL1Field, error) {
	return r.memL1().GetL1Field(taskID, fieldPath)
}
func (r *SQLiteRepository) GetL1FieldByID(fieldID string) (mem.MemoryL1Field, error) { return r.memL1().GetL1FieldByID(fieldID) }
func (r *SQLiteRepository) ListL1FieldsByTask(taskID string, includeInternal bool) ([]mem.MemoryL1Field, error) {
	return r.memL1().ListL1FieldsByTask(taskID, includeInternal)
}
func (r *SQLiteRepository) DeleteL1Field(fieldID string) error { return r.memL1().DeleteL1Field(fieldID) }
func (r *SQLiteRepository) BumpL1FieldRead(fieldID string, atISO string) error {
	return r.memL1().BumpL1FieldRead(fieldID, atISO)
}
func (r *SQLiteRepository) ListL1FieldHistory(fieldID string, limit int) ([]mem.MemoryL1FieldHistory, error) {
	return r.memL1().ListL1FieldHistory(fieldID, limit)
}
func (r *SQLiteRepository) GetL1FieldHistory(fieldID string, revision int) (mem.MemoryL1FieldHistory, error) {
	return r.memL1().GetL1FieldHistory(fieldID, revision)
}
func (r *SQLiteRepository) UpsertL1Schema(s mem.MemoryL1Schema) (mem.MemoryL1Schema, error) { return r.memL1().UpsertL1Schema(s) }
func (r *SQLiteRepository) ListL1Schemas(scopeType, scopeID string) ([]mem.MemoryL1Schema, error) {
	return r.memL1().ListL1Schemas(scopeType, scopeID)
}
func (r *SQLiteRepository) GetL1SchemaByID(id string) (mem.MemoryL1Schema, error) { return r.memL1().GetL1SchemaByID(id) }
func (r *SQLiteRepository) DeleteL1Schema(id string) error { return r.memL1().DeleteL1Schema(id) }
