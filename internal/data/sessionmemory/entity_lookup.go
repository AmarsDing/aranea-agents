package sessionmemory

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"aranea-agents/internal/biz"
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

func (st *Store) RecordEntityReinforcement(ctx context.Context, entityID string, signal biz.ReinforcementSignal, source string) error {
	if st == nil || st.client == nil {
		return nil
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return nil
	}
	nowMs := time.Now().UTC().UnixMilli()
	_, err := st.client.ExecContext(ctx, `
INSERT INTO entity_reinforcements (entity_id, signal, occurred_at, source)
VALUES (?, ?, ?, ?)`, entityID, string(signal), nowMs, source)
	return err
}

func (st *Store) GetRecentReinforcementCounts(ctx context.Context, scopeType, scopeID string, windowDays int) (map[string]int, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	cutoffMs := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour).UnixMilli()
	rows, err := st.client.QueryContext(ctx, `
SELECT e.id, COALESCE(r.cnt, 0)
FROM memory_entities e
LEFT JOIN (
    SELECT entity_id, COUNT(*) as cnt
    FROM entity_reinforcements
    WHERE occurred_at > ?
    GROUP BY entity_id
) r ON e.id = r.entity_id
WHERE e.scope_type = ? AND e.scope_id = ? AND e.status = 'active' AND e.deleted_at = ''`, cutoffMs, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		if cnt > 0 {
			out[id] = cnt
		}
	}
	return out, rows.Err()
}

func (st *Store) ApplyBusinessConfidenceDecay(ctx context.Context, scopeType, scopeID string, cfg biz.L4DecayConfig, nowUnixMs int64) (int64, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return 0, nil
	}
	if nowUnixMs <= 0 {
		nowUnixMs = time.Now().UTC().UnixMilli()
	}
	windowDays := 7
	hitCounts, err := st.GetRecentReinforcementCounts(ctx, scopeType, scopeID, windowDays)
	if err != nil {
		return 0, err
	}
	rows, err := st.client.QueryContext(ctx, `
SELECT id, entity_type, confidence, updated_at, use_count
FROM memory_entities
WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = ''
  AND confidence > 0.05`, scopeType, scopeID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type entityRow struct {
		ID         string
		EntityType string
		Confidence float64
		UpdatedAt  string
		UseCount   int
	}
	var entities []entityRow
	for rows.Next() {
		var e entityRow
		if err := rows.Scan(&e.ID, &e.EntityType, &e.Confidence, &e.UpdatedAt, &e.UseCount); err != nil {
			return 0, err
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	alpha := cfg.Alpha
	if alpha <= 0 {
		alpha = 0.15
	}
	var total int64
	for _, e := range entities {
		updatedAt, parseErr := time.Parse(time.RFC3339, e.UpdatedAt)
		if parseErr != nil {
			continue
		}
		deltaDays := time.UnixMilli(nowUnixMs).Sub(updatedAt).Hours() / 24
		if deltaDays < 0 {
			deltaDays = 0
		}
		halfLife := cfg.HalfLifeForEntityType(e.EntityType, e.UseCount >= 5)
		lambda := math.Ln2 / halfLife
		timeDecay := math.Exp(-lambda * deltaDays)
		hits := hitCounts[e.ID]
		reinforcementFactor := 1.0 + alpha*math.Log(1+float64(hits)/7)
		newConf := e.Confidence * timeDecay * reinforcementFactor
		if newConf < 0.05 {
			newConf = 0.05
		}
		if newConf > 1.0 {
			newConf = 1.0
		}
		if math.Abs(newConf-e.Confidence) < 0.001 {
			continue
		}
		_, err := st.client.ExecContext(ctx, `
UPDATE memory_entities SET confidence = ?, updated_at = updated_at
WHERE id = ? AND scope_type = ? AND scope_id = ? AND status = 'active'`,
			newConf, e.ID, scopeType, scopeID)
		if err != nil {
			continue
		}
		total++
	}
	return total, nil
}

func (st *Store) ArchiveLowConfidenceEntities(ctx context.Context, scopeType, scopeID string, threshold float64) (int64, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return 0, nil
	}
	if threshold <= 0 {
		threshold = 0.1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_entities
SET status = 'archived', archived_at = ?, updated_at = ?
WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = ''
  AND confidence < ? AND entity_type != 'user_profile'`,
		now, now, scopeType, scopeID, threshold)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (st *Store) ListEntitiesByConfidenceRange(ctx context.Context, scopeType, scopeID string, minConf, maxConf float64, limit int) ([]EntitySnapshot, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := st.client.QueryContext(ctx, fmt.Sprintf(`
SELECT id, name, name_normalized, confidence, metadata_json, updated_at
FROM memory_entities
WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = ''
  AND confidence >= ? AND confidence < ?
ORDER BY confidence ASC
LIMIT %d`, limit), scopeType, scopeID, minConf, maxConf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntitySnapshot
	for rows.Next() {
		var snap EntitySnapshot
		if err := rows.Scan(&snap.ID, &snap.Name, &snap.NameNormalized, &snap.Confidence, &snap.MetadataJSON, &snap.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}
