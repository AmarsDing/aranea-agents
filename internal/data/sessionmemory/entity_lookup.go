package sessionmemory

import (
	"context"
	"database/sql"
	"strings"
)

// EntitySnapshot is a minimal row for L4 governance decisions.
type EntitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
	UpdatedAt      string
}

// GetEntityByScopeKey returns an active entity matching scope + type + normalized name.
func (st *Store) GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (EntitySnapshot, bool, error) {
	if st == nil || st.client == nil {
		return EntitySnapshot{}, false, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	entityType = strings.TrimSpace(entityType)
	nameNormalized = strings.TrimSpace(strings.ToLower(nameNormalized))
	if scopeType == "" || scopeID == "" || entityType == "" || nameNormalized == "" {
		return EntitySnapshot{}, false, nil
	}
	const q = `
SELECT id, name, name_normalized, confidence, metadata_json, updated_at
FROM memory_entities
WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND name_normalized = ?
  AND status = 'active' AND deleted_at = ''
LIMIT 1`
	var snap EntitySnapshot
	err := queryOne(ctx, st.client, q, []any{scopeType, scopeID, entityType, nameNormalized},
		&snap.ID, &snap.Name, &snap.NameNormalized, &snap.Confidence, &snap.MetadataJSON, &snap.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return EntitySnapshot{}, false, nil
	}
	if err != nil {
		return EntitySnapshot{}, false, err
	}
	return snap, true, nil
}

// GetFirstEntityByType returns one active entity of the given type in scope.
func (st *Store) GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (EntitySnapshot, bool, error) {
	if st == nil || st.client == nil {
		return EntitySnapshot{}, false, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	entityType = strings.TrimSpace(entityType)
	if scopeType == "" || scopeID == "" || entityType == "" {
		return EntitySnapshot{}, false, nil
	}
	const q = `
SELECT id, name, name_normalized, confidence, metadata_json, updated_at
FROM memory_entities
WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND status = 'active' AND deleted_at = ''
ORDER BY updated_at DESC
LIMIT 1`
	var snap EntitySnapshot
	err := queryOne(ctx, st.client, q, []any{scopeType, scopeID, entityType},
		&snap.ID, &snap.Name, &snap.NameNormalized, &snap.Confidence, &snap.MetadataJSON, &snap.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return EntitySnapshot{}, false, nil
	}
	if err != nil {
		return EntitySnapshot{}, false, err
	}
	return snap, true, nil
}

// ApplyConfidenceDecay reduces confidence on stale entities for a scope (best-effort).
func (st *Store) ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID string, olderThanRFC3339 string, factor float64) (int64, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	if factor <= 0 || factor >= 1 {
		factor = 0.95
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	olderThanRFC3339 = strings.TrimSpace(olderThanRFC3339)
	if scopeType == "" || scopeID == "" || olderThanRFC3339 == "" {
		return 0, nil
	}
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_entities
SET confidence = CASE WHEN confidence * ? < 0.1 THEN 0.1 ELSE confidence * ? END,
    updated_at = ?
WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = ''
  AND updated_at < ? AND confidence > 0.1`,
		factor, factor, olderThanRFC3339, scopeType, scopeID, olderThanRFC3339,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
