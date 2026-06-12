package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type l4GraphRepo struct {
	data *Data
}

var _ biz.L4GraphRepo = (*l4GraphRepo)(nil)

func NewL4GraphRepo(data *Data) biz.L4GraphRepo {
	if data == nil {
		return nil
	}
	return &l4GraphRepo{data: data}
}

var errL4RepoNil = errors.New("l4 graph repo not initialized")

func (r *l4GraphRepo) UpsertEntity(ctx context.Context, params biz.L4EntityWrite) error {
	if r == nil {
		return errL4RepoNil
	}
	id := strings.TrimSpace(params.ID)
	if id == "" {
		id = newUUIDString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	nameNorm := strings.TrimSpace(params.NameNormalized)
	if nameNorm == "" {
		nameNorm = strings.ToLower(strings.TrimSpace(params.Name))
	}
	meta := strings.TrimSpace(params.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_entities (
		id, scope_type, scope_id, workspace_id, user_id,
		entity_type, name, name_normalized, aliases_json, description, attributes_json,
		importance, confidence, use_count, source_kind,
		status, metadata_json, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(scope_type, scope_id, entity_type, name_normalized) DO UPDATE SET
		name = excluded.name, description = excluded.description,
		importance = excluded.importance, confidence = excluded.confidence,
		metadata_json = excluded.metadata_json, updated_at = excluded.updated_at`,
		id,
		strings.TrimSpace(params.ScopeType),
		strings.TrimSpace(params.ScopeID),
		"", // workspace_id
		strings.TrimSpace(params.UserID),
		strings.TrimSpace(params.EntityType),
		strings.TrimSpace(params.Name),
		nameNorm, "[]",
		strings.TrimSpace(params.Description),
		"{}",
		params.Importance, params.Confidence, 0, "",
		"active", meta, now, now,
	)
	if err != nil {
		return err
	}
	// Write action log (best-effort)
	if logErr := r.writeActionLog(ctx, "UPSERT", "entity", id, params.EntityType, "consolidate_v1", params.MetadataJSON); logErr != nil {
		r.data.lg.Warn("failed to write action log",
			loggateway.StepID("data.l4.action_log"),
			loggateway.Err(logErr))
	}
	return nil
}

func (r *l4GraphRepo) UpsertRelation(ctx context.Context, params biz.L4RelationWrite) error {
	if r == nil {
		return errL4RepoNil
	}
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_relations (
		id, scope_type, scope_id, workspace_id,
		source_id, target_id, relation_type, bidirectional,
		weight, confidence, importance, use_count,
		attributes_json, evidence_json, status, source_kind,
		metadata_json, valid_from, valid_to, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
		weight = excluded.weight, confidence = excluded.confidence,
		updated_at = excluded.updated_at`,
		id,
		strings.TrimSpace(params.ScopeType),
		strings.TrimSpace(params.ScopeID),
		"", // workspace_id
		strings.TrimSpace(params.SourceID),
		strings.TrimSpace(params.TargetID),
		strings.TrimSpace(params.RelationType),
		0,
		params.Weight, params.Confidence, 0, 0,
		"{}", "{}", "active", "",
		"{}", "", "", now, now,
	)
	return err
}

func (r *l4GraphRepo) GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (biz.L4EntitySnapshot, bool, error) {
	if r == nil {
		return biz.L4EntitySnapshot{}, false, errL4RepoNil
	}
	var id, name, nnorm, meta, updatedAt string
	var conf float64
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT id, name, name_normalized, confidence, metadata_json, updated_at FROM memory_entities WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND name_normalized = ? AND status = 'active' AND deleted_at = ''`,
		[]any{scopeType, scopeID, entityType, nameNormalized}, &id, &name, &nnorm, &conf, &meta, &updatedAt)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			return biz.L4EntitySnapshot{}, false, nil
		}
		return biz.L4EntitySnapshot{}, false, err
	}
	return biz.L4EntitySnapshot{
		ID:             id,
		Name:           name,
		NameNormalized: nnorm,
		Confidence:     conf,
		MetadataJSON:   meta,
	}, true, nil
}

func (r *l4GraphRepo) GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (biz.L4EntitySnapshot, bool, error) {
	if r == nil {
		return biz.L4EntitySnapshot{}, false, errL4RepoNil
	}
	var id, name, nnorm, meta, updatedAt string
	var conf float64
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT id, name, name_normalized, confidence, metadata_json, updated_at FROM memory_entities WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND status = 'active' AND deleted_at = '' LIMIT 1`,
		[]any{scopeType, scopeID, entityType}, &id, &name, &nnorm, &conf, &meta, &updatedAt)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			return biz.L4EntitySnapshot{}, false, nil
		}
		return biz.L4EntitySnapshot{}, false, err
	}
	return biz.L4EntitySnapshot{
		ID:             id,
		Name:           name,
		NameNormalized: nnorm,
		Confidence:     conf,
		MetadataJSON:   meta,
	}, true, nil
}

func (r *l4GraphRepo) GetEntityRelations(ctx context.Context, entityID string) ([]biz.L4Relation, error) {
	if r == nil {
		return nil, errL4RepoNil
	}
	if strings.TrimSpace(entityID) == "" {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, scope_type, scope_id, source_id, target_id, relation_type, weight, confidence, metadata_json
		 FROM memory_relations
		 WHERE status = 'active' AND deleted_at = '' AND (source_id = ? OR target_id = ?)
		 ORDER BY weight DESC`,
		entityID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.L4Relation
	for rows.Next() {
		var rel biz.L4Relation
		if err := rows.Scan(&rel.ID, &rel.ScopeType, &rel.ScopeID, &rel.SourceID, &rel.TargetID, &rel.RelationType, &rel.Weight, &rel.Confidence, &rel.MetadataJSON); err != nil {
			r.data.lg.Warn("GetEntityRelations: scan row failed",
				loggateway.StepID("data.l4.scan"),
				loggateway.Err(err))
			continue
		}
		result = append(result, rel)
	}
	return result, rows.Err()
}

func (r *l4GraphRepo) GetEntitiesByType(ctx context.Context, scope, entityType string) ([]biz.L4Entity, error) {
	if r == nil {
		return nil, errL4RepoNil
	}
	scope = strings.TrimSpace(scope)
	entityType = strings.TrimSpace(entityType)
	if scope == "" || entityType == "" {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, scope_type, scope_id, user_id, entity_type, name, name_normalized, description, importance, confidence, metadata_json
		 FROM memory_entities
		 WHERE scope_id = ? AND entity_type = ? AND status = 'active' AND deleted_at = ''
		 ORDER BY importance DESC`,
		scope, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.L4Entity
	for rows.Next() {
		var ent biz.L4Entity
		if err := rows.Scan(&ent.ID, &ent.ScopeType, &ent.ScopeID, &ent.UserID, &ent.EntityType, &ent.Name, &ent.NameNormalized, &ent.Description, &ent.Importance, &ent.Confidence, &ent.MetadataJSON); err != nil {
			r.data.lg.Warn("GetEntitiesByType: scan row failed",
				loggateway.StepID("data.l4.scan"),
				loggateway.Err(err))
			continue
		}
		result = append(result, ent)
	}
	return result, rows.Err()
}

func (r *l4GraphRepo) SearchEntitiesByName(ctx context.Context, scope, nameQuery string, limit int) ([]biz.L4Entity, error) {
	if r == nil {
		return nil, errL4RepoNil
	}
	scope = strings.TrimSpace(scope)
	nameQuery = strings.TrimSpace(nameQuery)
	if scope == "" || nameQuery == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	pattern := "%" + strings.ToLower(nameQuery) + "%"
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, scope_type, scope_id, user_id, entity_type, name, name_normalized, description, importance, confidence, metadata_json
		 FROM memory_entities
		 WHERE scope_id = ? AND name_normalized LIKE ? AND status = 'active' AND deleted_at = ''
		 ORDER BY importance DESC
		 LIMIT ?`,
		scope, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.L4Entity
	for rows.Next() {
		var ent biz.L4Entity
		if err := rows.Scan(&ent.ID, &ent.ScopeType, &ent.ScopeID, &ent.UserID, &ent.EntityType, &ent.Name, &ent.NameNormalized, &ent.Description, &ent.Importance, &ent.Confidence, &ent.MetadataJSON); err != nil {
			r.data.lg.Warn("SearchEntitiesByName: scan row failed",
				loggateway.StepID("data.l4.scan"),
				loggateway.Err(err))
			continue
		}
		result = append(result, ent)
	}
	return result, rows.Err()
}

func (r *l4GraphRepo) ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID, olderThanRFC3339 string, factor float64) (int64, error) {
	if r == nil {
		return 0, errL4RepoNil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_entities SET confidence = confidence * ?, updated_at = ? WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = '' AND updated_at < ?`,
		factor, now, scopeType, scopeID, olderThanRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *l4GraphRepo) RecordEntityReinforcement(ctx context.Context, entityID string, signal biz.ReinforcementSignal, source string) error {
	if r == nil {
		return errL4RepoNil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	delta := 0.0
	switch signal {
	case biz.ReinforcementHit:
		delta = 0.05
	case biz.ReinforcementRefuted:
		delta = -0.05
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_entities SET confidence = MIN(1.0, MAX(0.0, confidence + ?)), use_count = use_count + 1, updated_at = ? WHERE id = ?`,
		delta, now, entityID)
	return err
}

func (r *l4GraphRepo) GetRecentReinforcementCounts(ctx context.Context, scopeType, scopeID string, windowDays int) (map[string]int, error) {
	if r == nil {
		return nil, errL4RepoNil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -windowDays).Format(time.RFC3339Nano)
	// Query reinforcement counts for entities belonging to the given scope.
	// Entity IDs follow the pattern "l4-{type}-{scopeID}-..." or "l4-user-{scopeID}".
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT a.target_id, COUNT(*) FROM memory_action_log a
		 JOIN memory_entities e ON e.id = a.target_id
		 WHERE a.target_kind = 'entity' AND a.action = 'REINFORCE'
		   AND e.scope_type = ? AND e.scope_id = ?
		   AND a.created_at > ?
		 GROUP BY a.target_id`,
		scopeType, scopeID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			continue
		}
		result[id] = count
	}
	return result, rows.Err()
}

func (r *l4GraphRepo) ApplyBusinessConfidenceDecay(ctx context.Context, scopeType, scopeID string, cfg biz.L4DecayConfig, nowUnixMs int64) (int64, error) {
	if r == nil {
		return 0, errL4RepoNil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Alpha is the decay rate per period (0 < Alpha < 1).
	// Retention factor = 1 - Alpha, so Alpha=0.15 means 85% retention per period.
	factor := 1.0 - cfg.Alpha
	if factor <= 0 || factor >= 1 {
		factor = 0.95
	}
	// Cutoff: use the shortest half-life from config to determine when decay starts.
	// Entities should begin decaying after at least one half-life has elapsed.
	minHalfLifeDays := 365.0
	for _, hl := range cfg.HalfLifeDays {
		if hl > 0 && hl < minHalfLifeDays {
			minHalfLifeDays = hl
		}
	}
	cutoffMs := int64(minHalfLifeDays * 2 * 86400000) // 2x shortest half-life as decay onset
	if cutoffMs <= 0 {
		cutoffMs = 365 * 86400000
	}
	cutoff := time.UnixMilli(nowUnixMs - cutoffMs).Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_entities SET confidence = confidence * ?, updated_at = ? WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = '' AND updated_at < ? AND confidence > 0.01`,
		factor, now, scopeType, scopeID, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *l4GraphRepo) ArchiveLowConfidenceEntities(ctx context.Context, scopeType, scopeID string, threshold float64) (int64, error) {
	if r == nil {
		return 0, errL4RepoNil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_entities SET status = 'archived', updated_at = ? WHERE scope_type = ? AND scope_id = ? AND status = 'active' AND deleted_at = '' AND confidence < ?`,
		now, scopeType, scopeID, threshold)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *l4GraphRepo) writeActionLog(ctx context.Context, action, targetKind, targetID, reason, policyVersion, meta string) error {
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if meta == "" {
		meta = "{}"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_action_log (
		id, action, target_kind, target_id, reason, policy_version, turn_id, source_event_ids_json, metadata_json, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, action, targetKind, targetID, reason, policyVersion, "", "[]", meta, now)
	return err
}

type l4GraphWriterAdapter struct {
	uc *biz.L4GraphUsecase
}

var _ biz.L4GraphWriter = (*l4GraphWriterAdapter)(nil)

func NewL4GraphWriterAdapter(uc *biz.L4GraphUsecase) biz.L4GraphWriter {
	if uc == nil {
		return nil
	}
	return &l4GraphWriterAdapter{uc: uc}
}

func (a *l4GraphWriterAdapter) WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error) {
	return a.uc.WriteFromUserText(ctx, agentID, userID, text)
}

func (a *l4GraphWriterAdapter) RunDecay(ctx context.Context, agentID string) {
	a.uc.RunDecay(ctx, agentID)
}

func (a *l4GraphWriterAdapter) RunDecayWithConfig(ctx context.Context, agentID string, cfg biz.L4DecayConfig) biz.L4DecayResult {
	return a.uc.RunDecayWithConfig(ctx, agentID, cfg)
}

func (a *l4GraphWriterAdapter) RecordEntityReinforcement(ctx context.Context, entityID string, signal biz.ReinforcementSignal, source string) error {
	return a.uc.RecordEntityReinforcement(ctx, entityID, signal, source)
}

func NewL4GraphUsecaseFromData(data *Data, cascade *biz.L4CascadeUsecase, lg loggateway.Logger) *biz.L4GraphUsecase {
	repo := NewL4GraphRepo(data)
	uc := biz.NewL4GraphUsecase(repo, lg)
	if cascade != nil {
		uc.SetCascade(cascade)
	}
	return uc
}


