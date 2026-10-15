package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"context"
	"strings"
	"time"
)

// CreateInhibitRelation creates (or upserts) a directed INHIBIT relation from
// SourceID to TargetID. The relation_type is forced to INHIBIT (callers cannot
// override it). Weight defaults to 0.8 if params.Weight is zero.
func (r *l4EntityRepo) CreateInhibitRelation(ctx context.Context, params biz.L4InhibitRelationCreate) error {
	if r == nil {
		return apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	sourceID := strings.TrimSpace(params.SourceID)
	targetID := strings.TrimSpace(params.TargetID)
	if sourceID == "" || targetID == "" {
		return apierror.BadRequest("MEMORY_L4", "source_id and target_id are required for INHIBIT relation")
	}
	if sourceID == targetID {
		return apierror.BadRequest("MEMORY_L4", "self-inhibition not allowed (source_id == target_id)")
	}

	weight := params.Weight
	if weight <= 0 {
		weight = 0.8
	}

	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(createInhibitRelationSQL),
		id,
		strings.TrimSpace(params.ScopeType),
		strings.TrimSpace(params.ScopeID),
		"",
		sourceID,
		targetID,
		biz.RelationInhibit,
		0,
		weight, 0.5, 0.5, 0,
		"{}", "{}", "active", "",
		"{}", now, "", now, now,
		params.ContextNote,
	)
	return entErrToBizErr(err, "MEMORY_L4")
}

// adjustConfidenceSQL atomically adjusts confidence by delta (saturated to
// [0, 1]) using a CASE expression for cross-dialect compatibility (SQLite and
// Postgres both support standard CASE WHEN). Only matches non-deleted entities.
const adjustConfidenceSQL = `UPDATE memory_entities
SET confidence = CASE
        WHEN confidence + ? > 1.0 THEN 1.0
        WHEN confidence + ? < 0.0 THEN 0.0
        ELSE confidence + ?
    END,
    updated_at = ?
WHERE id = ? AND deleted_at = ''`

// AdjustConfidence atomically adjusts confidence by delta (saturated to [0, 1])
// for the given entity. Returns ok=false if the entity was not found or is
// soft-deleted (no rows affected).
func (r *l4EntityRepo) AdjustConfidence(ctx context.Context, entityID string, delta float64) (bool, error) {
	if r == nil {
		return false, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return false, apierror.BadRequest("MEMORY_L4", "entity_id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(adjustConfidenceSQL),
		delta, delta, delta, now, entityID)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_L4")
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// FindBySourceCollection returns all active, non-deleted entities whose
// metadata_json.source_collection_id matches collectionID. Uses dialect-aware
// JSON extraction (SQLite json_extract / Postgres ->>) for cross-dialect
// compatibility. AdjustConfidence is shared with L4ConflictStore (same method
// signature, implemented once above).
func (r *l4EntityRepo) FindBySourceCollection(ctx context.Context, collectionID string) ([]biz.L4EntitySnapshot, error) {
	if r == nil {
		return nil, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil, nil
	}

	expr := r.data.Dialect().JSONExtract("metadata_json", "source_collection_id")
	q := `SELECT id, name, name_normalized, confidence, metadata_json, updated_at
FROM memory_entities
WHERE deleted_at = '' AND status = 'active' AND ` + expr + ` = ?`

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(q), collectionID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()

	var out []biz.L4EntitySnapshot
	for rows.Next() {
		var snap biz.L4EntitySnapshot
		var updatedAt string
		if err := rows.Scan(&snap.ID, &snap.Name, &snap.NameNormalized,
			&snap.Confidence, &snap.MetadataJSON, &updatedAt); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		out = append(out, snap)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L4")
}
