package repository

import (
	mem "arenea/backend/internal/memory/domain"

	memsqlite "arenea/backend/internal/memory/adapters/sqlite"
)

func (r *SQLiteRepository) memL0() *memsqlite.L0Repository {
	return memsqlite.NewL0Repository(r.db)
}

func (r *SQLiteRepository) InsertL0AssemblySnapshot(snap mem.L0AssemblySnapshot) error {
	return r.memL0().InsertL0AssemblySnapshot(snap)
}

func (r *SQLiteRepository) UpdateL0AssemblySnapshotActualTokens(snapshotID string, actualPromptTokens int, usedRatio float64) error {
	return r.memL0().UpdateL0AssemblySnapshotActualTokens(snapshotID, actualPromptTokens, usedRatio)
}

func (r *SQLiteRepository) GetL0AssemblySnapshotByID(id string) (mem.L0AssemblySnapshot, error) {
	return r.memL0().GetL0AssemblySnapshotByID(id)
}

func (r *SQLiteRepository) ListL0AssemblySnapshotsBySession(sessionID string, limit int) ([]mem.L0AssemblySnapshot, error) {
	return r.memL0().ListL0AssemblySnapshotsBySession(sessionID, limit)
}

func (r *SQLiteRepository) ListL0AssemblySnapshotsBySpan(spanID string) ([]mem.L0AssemblySnapshot, error) {
	return r.memL0().ListL0AssemblySnapshotsBySpan(spanID)
}

func (r *SQLiteRepository) ListSessionSummaries(sessionID string, limit int) ([]mem.SessionSummary, error) {
	return r.memL0().ListSessionSummaries(sessionID, limit)
}

func (r *SQLiteRepository) AddSessionSummary(summary mem.SessionSummary) (mem.SessionSummary, error) {
	return r.memL0().AddSessionSummary(summary)
}
