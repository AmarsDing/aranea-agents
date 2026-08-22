package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// l4EntityRepo implements biz.L4EntityStore + biz.L4EvolutionStore using direct Raw SQL.
type l4EntityRepo struct {
	data *Data
}

func newL4EntityRepo(data *Data) *l4EntityRepo {
	if data == nil {
		return nil
	}
	return &l4EntityRepo{data: data}
}

// NewL4GraphTraverser returns the narrow L4GraphTraverser interface for
// spreading-activation subgraph fetching. Exposed so the wire layer can inject
// it into SpreadingActivationEngine without depending on the full L4GraphRepo.
func NewL4GraphTraverser(data *Data) biz.L4GraphTraverser {
	return newL4EntityRepo(data)
}

// NewL4RelationAdminReader creates a biz.L4RelationAdminReader backed by data.
// Used by the memory-center layer overview and unified graph assembly.
func NewL4RelationAdminReader(data *Data) biz.L4RelationAdminReader {
	return newL4EntityRepo(data)
}

// NewL4ReconsolidationStore returns the narrow L4ReconsolidationStore port for
// memory reconsolidation (activation boost). Exposed so
// the wire layer can inject it into ReconsolidationService without depending
// on the full L4GraphRepo.
func NewL4ReconsolidationStore(data *Data) biz.L4ReconsolidationStore {
	return newL4EntityRepo(data)
}

// NewL4HebbianStore returns the narrow L4HebbianStore port for Hebbian weight
// updates. Exposed so the wire layer can inject it into HebbianUpdater without
// depending on the full L4GraphRepo.
func NewL4HebbianStore(data *Data) biz.L4HebbianStore {
	return newL4EntityRepo(data)
}

// Compile-time interface checks.
var (
	_ biz.L4EntityStore          = (*l4EntityRepo)(nil)
	_ biz.L4EvolutionStore       = (*l4EntityRepo)(nil)
	_ biz.L4GraphTraverser       = (*l4EntityRepo)(nil)
	_ biz.L4HebbianStore         = (*l4EntityRepo)(nil)
	_ biz.L4ReconsolidationStore = (*l4EntityRepo)(nil)
	_ biz.L4ConflictStore        = (*l4EntityRepo)(nil)
	_ biz.L4KnowledgeBridgeStore = (*l4EntityRepo)(nil)
	_ biz.L4RelationAdminReader  = (*l4EntityRepo)(nil)
)

// --- L4RelationAdminReader ---

func (r *l4EntityRepo) CountActiveRelations(ctx context.Context, scopeType, scopeID string) (int32, error) {
	if r == nil {
		return 0, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	var count int32
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM memory_relations WHERE status = 'active' AND deleted_at = '' AND scope_type = ? AND scope_id = ?`),
		[]any{scopeType, scopeID}, &count)
	if err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L4")
	}
	return count, nil
}

func (r *l4EntityRepo) ListActiveRelationRows(ctx context.Context, scopeType, scopeID string) ([][]byte, error) {
	if r == nil {
		return nil, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT`+sqlRelationCols+` FROM memory_relations WHERE status = 'active' AND deleted_at = '' AND scope_type = ? AND scope_id = ? ORDER BY weight DESC`),
		scopeType, scopeID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanRelationRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L4")
}

func (r *l4EntityRepo) TopConnectedEntityID(ctx context.Context, scopeType, scopeID string) (string, error) {
	if r == nil {
		return "", apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	// Endpoint aggregation: count each relation's source and target as one hit,
	// restricted to endpoints that resolve to active entities.
	const q = `SELECT t.endpoint_id, COUNT(*) AS c FROM (
		SELECT source_id AS endpoint_id FROM memory_relations WHERE status = 'active' AND deleted_at = '' AND scope_type = ? AND scope_id = ?
		UNION ALL
		SELECT target_id AS endpoint_id FROM memory_relations WHERE status = 'active' AND deleted_at = '' AND scope_type = ? AND scope_id = ?
	) t
	JOIN memory_entities e ON e.id = t.endpoint_id AND e.status = 'active' AND e.deleted_at = ''
	GROUP BY t.endpoint_id
	ORDER BY c DESC, t.endpoint_id ASC
	LIMIT 1`
	var top string
	var hitCount int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(q),
		[]any{scopeType, scopeID, scopeType, scopeID}, &top, &hitCount)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			return "", nil
		}
		return "", entErrToBizErr(err, "MEMORY_L4")
	}
	return top, nil
}

// --- L4EntityStore ---

func (r *l4EntityRepo) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	clauses := []string{"deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if workspaceID != "" {
		// Shared-or-own visibility, aligned with data.workspaceSharedOrOwnIDs:
		// rows with workspace_id = '' are shared/legacy (system writers such as
		// AutoMemory worker and L4 extraction store ''), caller-owned rows carry
		// the caller workspace. An equality filter here would hide all shared
		// rows from every tenant, including default.
		clauses = append(clauses, "workspace_id IN ('', ?)")
		args = append(args, workspaceID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	if entityType != "" {
		clauses = append(clauses, "entity_type = ?")
		args = append(args, entityType)
	}
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	} else {
		clauses = append(clauses, "status = 'active'")
	}
	if keyword != "" {
		clauses = append(clauses, "name_normalized LIKE ?")
		args = append(args, "%"+strings.ToLower(keyword)+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_entities"+where), args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L4")
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := "SELECT" + sqlEntityCols + " FROM memory_entities" + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	lg := r.data.lg
	var out [][]byte
	for rows.Next() {
		b, err := scanEntityRowJSON(rows, lg)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY_L4")
		}
		out = append(out, b)
	}
	return out, total, entErrToBizErr(rows.Err(), "MEMORY_L4")
}

// CountEntitiesCreatedSince counts active, non-deleted entities in a scope
// whose created_at is on or after todayStart (RFC3339 / RFC3339Nano text).
func (r *l4EntityRepo) CountEntitiesCreatedSince(ctx context.Context, scopeType, scopeID, todayStart string) (int32, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	todayStart = strings.TrimSpace(todayStart)
	if scopeType == "" || scopeID == "" {
		return 0, nil
	}
	q := `SELECT COUNT(*) FROM memory_entities WHERE deleted_at = '' AND status = 'active' AND scope_type = ? AND scope_id = ? AND created_at >= ?`
	var count int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders(q), []any{scopeType, scopeID, todayStart}, &count); err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L4")
	}
	return count, nil
}

func (r *l4EntityRepo) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if centerID == "" {
		return nil, apierror.BadRequest("MEMORY", "center_id is required")
	}
	maxH := int(hops)
	if maxH <= 0 {
		maxH = 2
	}
	mxN := int(maxNodes)
	if mxN <= 0 {
		mxN = 50
	}
	qt := defaultQueryTimeRFC3339(queryAtRFC3339)

	// BFS traversal
	visited := map[string]bool{centerID: true}
	var entities [][]byte
	var relations [][]byte
	frontier := []string{centerID}

	for step := 0; step < maxH; step++ {
		if len(frontier) == 0 {
			break
		}
		var nextFrontier []string
		// Collect unvisited neighbors via relations
		ph := make([]string, len(frontier))
		args := make([]any, 0, len(frontier))
		for i, id := range frontier {
			ph[i] = "?"
			args = append(args, id)
		}
		relQ := fmt.Sprintf(`SELECT%s FROM memory_relations WHERE (source_id IN (%s) OR target_id IN (%s)) AND status = 'active' AND deleted_at = ''`,
			sqlRelationCols, strings.Join(ph, ","), strings.Join(ph, ","))
		// Duplicate args for second IN clause
		args2 := make([]any, len(args))
		copy(args2, args)
		args = append(args, args2...)
		rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(relQ), args...)
		if err != nil {
			r.data.lg.Warn("neighborhood: relation query failed",
				loggateway.StepID("data.l4.neighborhood"),
				loggateway.Err(err),
				loggateway.Str("sql", relQ))
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		lg := r.data.lg
		for rows.Next() {
			b, err := scanRelationRowJSON(rows)
			if err != nil {
				rows.Close()
				r.data.lg.Warn("neighborhood: relation scan failed",
					loggateway.StepID("data.l4.neighborhood"),
					loggateway.Err(err))
				return nil, entErrToBizErr(err, "MEMORY_L4")
			}
			relations = append(relations, b)
			var rm map[string]any
			if json.Unmarshal(b, &rm) == nil {
				if sid, ok := rm["source_id"].(string); ok && !visited[sid] {
					nextFrontier = append(nextFrontier, sid)
				}
				if tid, ok := rm["target_id"].(string); ok && !visited[tid] {
					nextFrontier = append(nextFrontier, tid)
				}
			}
		}
		rows.Close()

		// Deduplicate frontier
		seen := map[string]bool{}
		var deduped []string
		for _, id := range nextFrontier {
			if !seen[id] && !visited[id] {
				seen[id] = true
				deduped = append(deduped, id)
			}
		}
		nextFrontier = deduped

		// Mark visited
		for _, id := range nextFrontier {
			visited[id] = true
		}

		// Fetch entity rows for new nodes
		if len(nextFrontier) > 0 {
			ph2 := make([]string, len(nextFrontier))
			args2 := make([]any, len(nextFrontier))
			for i, id := range nextFrontier {
				ph2[i] = "?"
				args2[i] = id
			}
			entQ := fmt.Sprintf(`SELECT%s FROM memory_entities WHERE id IN (%s) AND status = 'active' AND deleted_at = ''`,
				sqlEntityCols, strings.Join(ph2, ","))
			entRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(entQ), args2...)
			if err != nil {
				r.data.lg.Warn("neighborhood: entity query failed",
					loggateway.StepID("data.l4.neighborhood"),
					loggateway.Err(err),
					loggateway.Str("sql", entQ))
				return nil, entErrToBizErr(err, "MEMORY_L4")
			}
			for entRows.Next() {
				b, err := scanEntityRowJSON(entRows, lg)
				if err != nil {
					entRows.Close()
					r.data.lg.Warn("neighborhood: entity scan failed",
						loggateway.StepID("data.l4.neighborhood"),
						loggateway.Err(err))
					return nil, entErrToBizErr(err, "MEMORY_L4")
				}
				entities = append(entities, b)
			}
			entRows.Close()
		}

		frontier = nextFrontier
		if len(visited) >= mxN {
			break
		}
	}

	// Also fetch center entity
	var centerEntity []byte
	centerRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders("SELECT"+sqlEntityCols+" FROM memory_entities WHERE id = ? AND status = 'active' AND deleted_at = ''"), centerID)
	if err != nil {
		r.data.lg.Warn("neighborhood: center entity query failed",
			loggateway.StepID("data.l4.neighborhood"),
			loggateway.Err(err),
			loggateway.Str("center_id", centerID))
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	lg := r.data.lg
	for centerRows.Next() {
		b, err := scanEntityRowJSON(centerRows, lg)
		if err != nil {
			centerRows.Close()
			r.data.lg.Warn("neighborhood: center entity scan failed",
				loggateway.StepID("data.l4.neighborhood"),
				loggateway.Err(err))
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		if centerEntity == nil {
			centerEntity = b
		} else {
			entities = append(entities, b)
		}
	}
	centerRows.Close()

	// Decode centerEntity to a map for the "center" field; nil if not found.
	var centerMap map[string]any
	if centerEntity != nil {
		_ = json.Unmarshal(centerEntity, &centerMap)
	}
	result := map[string]any{
		"center_id":  centerID,
		"center":     centerMap,
		"entities":   entities,
		"relations":  relations,
		"query_at":   qt,
		"hops":       maxH,
		"max_nodes":  mxN,
		"node_count": len(visited),
	}
	return json.Marshal(result)
}

func (r *l4EntityRepo) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	if agentID == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT agent_id, persona, values_json, tone, domains_json, user_expectations,
		        current_phase, metadata_json, version, created_at, updated_at
		 FROM agent_identity WHERE agent_id = ?`), agentID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	if !rows.Next() {
		return json.Marshal(map[string]any{"agent_id": agentID, "identity": nil})
	}
	var aid, persona, valuesJSON, tone, domainsJSON, userExp, phase, meta, ca, ua string
	var version int
	if err := rows.Scan(&aid, &persona, &valuesJSON, &tone, &domainsJSON, &userExp, &phase, &meta, &version, &ca, &ua); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	identity := map[string]any{
		"agent_id":          aid,
		"persona":           persona,
		"values":            decodeJSONStringSlice(valuesJSON, r.data.lg),
		"tone":              tone,
		"domains":           decodeJSONStringSlice(domainsJSON, r.data.lg),
		"user_expectations": userExp,
		"current_phase":     phase,
		"metadata_json":     meta,
		"version":           version,
		"created_at":        ca,
		"updated_at":        ua,
	}
	return json.Marshal(map[string]any{"agent_id": agentID, "identity": identity})
}

func (r *l4EntityRepo) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	if agentID == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT agent_id, exploration, conciseness, caution, delegation,
		        tool_preference_json, tool_blacklist_json,
		        provider_preference_json, model_preference_json,
		        stats_json, metadata_json, version, created_at, updated_at
		 FROM agent_strategy_profile WHERE agent_id = ?`), agentID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	if !rows.Next() {
		return json.Marshal(map[string]any{"agent_id": agentID, "strategy": nil})
	}
	var aid, toolPrefJSON, toolBlacklistJSON, providerPrefJSON, modelPrefJSON, statsJSON, meta, ca, ua string
	var exploration, conciseness, caution, delegation float64
	var version int
	if err := rows.Scan(&aid, &exploration, &conciseness, &caution, &delegation,
		&toolPrefJSON, &toolBlacklistJSON, &providerPrefJSON, &modelPrefJSON,
		&statsJSON, &meta, &version, &ca, &ua); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	strategy := map[string]any{
		"agent_id":            aid,
		"exploration":         exploration,
		"conciseness":         conciseness,
		"caution":             caution,
		"delegation":          delegation,
		"tool_preference":     decodeJSONFloatMap(toolPrefJSON, r.data.lg),
		"tool_blacklist":      decodeJSONStringSlice(toolBlacklistJSON, r.data.lg),
		"provider_preference": decodeJSONFloatMap(providerPrefJSON, r.data.lg),
		"model_preference":    decodeJSONFloatMap(modelPrefJSON, r.data.lg),
		"stats_json":          statsJSON,
		"metadata_json":       meta,
		"version":             version,
		"created_at":          ca,
		"updated_at":          ua,
	}
	return json.Marshal(map[string]any{"agent_id": agentID, "strategy": strategy})
}

func (r *l4EntityRepo) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Soft-delete entities whose scope points to this session
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_entities SET deleted_at = ?, status = 'deleted' WHERE scope_id = ? AND scope_type = 'session' AND entity_type = 'event'`),
		now, sessionID)
	return entErrToBizErr(err, "MEMORY_L4")
}

// --- L4EvolutionStore ---

func (r *l4EntityRepo) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	clauses := []string{}
	args := []any{}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	q := evolutionProposalSelect + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanEvolutionProposalJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L4")
}

func (r *l4EntityRepo) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	q := `SELECT id, agent_id, workspace_id, event_kind, target_field, reason,
		        trigger_kind, trigger_source, metadata_json, created_at, reverted
		 FROM agent_evolution_events`
	args := []any{}
	if agentID != "" {
		q += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	lg := r.data.lg
	var out [][]byte
	for rows.Next() {
		var id, aid, wid, ek, tf, reason, tk, ts, meta, ca string
		var reverted int
		if err := rows.Scan(&id, &aid, &wid, &ek, &tf, &reason, &tk, &ts, &meta, &ca, &reverted); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		// `kind` is not a column on agent_evolution_events; it is round-tripped
		// via metadata_json (see InsertEvolutionEventRow). If absent, fall back to event_kind.
		kind := ""
		if m := decodeJSONObject(meta, lg); m != nil {
			if v, ok := m["kind"].(string); ok {
				kind = v
			}
		}
		if kind == "" {
			kind = ek
		}
		m := map[string]any{
			"id": id, "agent_id": aid, "workspace_id": wid,
			"event_kind": ek, "kind": kind, "target_field": tf,
			"reason": reason, "trigger_kind": tk, "trigger_source": ts,
			"metadata_json": meta, "created_at": ca,
			"reverted": reverted != 0,
		}
		b, mErr := json.Marshal(m)
		if mErr != nil {
			return nil, entErrToBizErr(mErr, "MEMORY_L4")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L4")
}

func (r *l4EntityRepo) EvolutionMetricsJSON(ctx context.Context, agentID string, timeRange string) ([]byte, error) {
	var total, pending, approved, rejected int
	metricsQ := `SELECT
		COUNT(*) as total,
		SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
		SUM(CASE WHEN status = 'approved' THEN 1 ELSE 0 END) as approved,
		SUM(CASE WHEN status = 'rejected' THEN 1 ELSE 0 END) as rejected
		FROM agent_evolution_events`
	args := []any{}
	whereClauses := []string{}
	if agentID != "" {
		whereClauses = append(whereClauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if cutoff := parseTimeRangeCutoff(timeRange); !cutoff.IsZero() {
		whereClauses = append(whereClauses, "created_at >= ?")
		args = append(args, cutoff.UTC().Format(time.RFC3339))
	}
	if len(whereClauses) > 0 {
		metricsQ += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders(metricsQ), args, &total, &pending, &approved, &rejected); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	return json.Marshal(map[string]any{
		"agent_id": agentID,
		"total":    total,
		"pending":  pending,
		"approved": approved,
		"rejected": rejected,
	})
}

// parseTimeRangeCutoff parses a time range string (e.g. "7d", "30d", "1h") and
// returns the cutoff time. Returns zero time if the string is empty or unparseable.
func parseTimeRangeCutoff(timeRange string) time.Time {
	timeRange = strings.TrimSpace(timeRange)
	if timeRange == "" {
		return time.Time{}
	}
	now := time.Now()
	// Support formats: "7d", "30d", "1h", "24h", "1w"
	if len(timeRange) < 2 {
		return time.Time{}
	}
	unit := timeRange[len(timeRange)-1]
	valueStr := timeRange[:len(timeRange)-1]
	var value int
	if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil || value <= 0 {
		return time.Time{}
	}
	switch unit {
	case 'h':
		return now.Add(-time.Duration(value) * time.Hour)
	case 'd':
		return now.AddDate(0, 0, -value)
	case 'w':
		return now.AddDate(0, 0, -value*7)
	default:
		return time.Time{}
	}
}

func (r *l4EntityRepo) InsertEvolutionEventRow(ctx context.Context, in biz.EvolutionEventInsert) ([]byte, error) {
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Merge `kind` (not a column) into metadata_json to preserve round-trip.
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	kind := strings.TrimSpace(in.Kind)
	if kind != "" {
		m := decodeJSONObject(meta, r.data.lg)
		if m == nil {
			m = map[string]any{}
		}
		m["kind"] = kind
		if b, err := json.Marshal(m); err == nil {
			meta = string(b)
		}
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`INSERT INTO agent_evolution_events (
		id, agent_id, workspace_id, event_kind, target_field,
		before_json, after_json, diff_json,
		trigger_kind, trigger_source, evidence_json, reason,
		applied, reverted, reverted_by_event_id,
		metadata_json, created_at, applied_at, reverted_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		id,
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.WorkspaceID),
		strings.TrimSpace(in.EventKind),
		strings.TrimSpace(in.TargetField),
		"", "", "{}", // before/after/diff
		strings.TrimSpace(in.TriggerKind),
		strings.TrimSpace(in.TriggerSource),
		"[]", // evidence_json
		strings.TrimSpace(in.Reason),
		1,                  // applied
		0,                  // reverted
		"",                 // reverted_by_event_id
		meta, now, now, "", // applied_at=now, reverted_at=''
	)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	if err := applyEvolutionEventSideEffects(ctx, r, in); err != nil {
		return nil, err
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, agent_id, workspace_id, event_kind, target_field, reason,
		        trigger_kind, trigger_source, metadata_json, created_at, reverted
		 FROM agent_evolution_events WHERE id = ?`), id)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound("MEMORY", "evolution event not found after insert")
	}
	var rowID, aid, wid, ek, tf, reason, tk, ts, m, ca string
	var reverted int
	if err := rows.Scan(&rowID, &aid, &wid, &ek, &tf, &reason, &tk, &ts, &m, &ca, &reverted); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	outKind := kind
	if outKind == "" {
		outKind = ek
	}
	result := map[string]any{
		"id": rowID, "agent_id": aid, "workspace_id": wid,
		"event_kind": ek, "kind": outKind, "target_field": tf,
		"reason": reason, "trigger_kind": tk, "trigger_source": ts,
		"metadata_json": m, "created_at": ca,
		"reverted": reverted != 0,
	}
	return json.Marshal(result)
}

func evolutionReviewTarget(kind, metadataJSON, fallback string, lg loggateway.Logger) string {
	target := strings.TrimSpace(fallback)
	meta := decodeJSONObject(metadataJSON, lg)
	if meta == nil {
		return target
	}
	switch strings.TrimSpace(kind) {
	case "proposal_approved", "proposal_rejected":
		if id, _ := meta["proposal_id"].(string); strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	case "event_reverted":
		if id, _ := meta["event_id"].(string); strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return target
}

func applyEvolutionEventSideEffects(ctx context.Context, r *l4EntityRepo, in biz.EvolutionEventInsert) error {
	kind := strings.TrimSpace(in.EventKind)
	target := evolutionReviewTarget(kind, in.MetadataJSON, in.TargetField, r.data.lg)
	if target == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch kind {
	case "proposal_approved", "proposal_rejected":
		status := "approved"
		if kind == "proposal_rejected" {
			status = "rejected"
		}
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE agent_evolution_proposals SET status = ?, reviewed_at = ?, updated_at = ? WHERE id = ? AND status = 'pending'`),
			status, now, now, target)
		return entErrToBizErr(err, "MEMORY_L4")
	case "event_reverted":
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE agent_evolution_events SET reverted = 1, reverted_at = ? WHERE id = ? AND reverted = 0`),
			now, target)
		return entErrToBizErr(err, "MEMORY_L4")
	default:
		return nil
	}
}

// --- L4GraphTraverser ---

// graphTraverseCTESQL is the recursive CTE that traverses the memory_relations
// graph up to `hops` levels from a center node, propagating activation as
// parent_activation * edge_weight. Nodes are aggregated by ID (strongest
// activation wins), then ordered by activation DESC and limited to topK.
//
// Placeholders (in order): center_id, max_hops, top_k
//
// SQLite and PostgreSQL both support WITH RECURSIVE; the ? placeholders are
// renumbered to $N by Dialect().RenumberPlaceholders() for Postgres.
const graphTraverseCTESQL = `WITH RECURSIVE graph_traverse(node_id, hop, activation) AS (
  SELECT ?, 0, CAST(1.0 AS REAL)
  UNION ALL
  SELECT
    CASE WHEN r.source_id = gt.node_id THEN r.target_id ELSE r.source_id END,
    gt.hop + 1,
    gt.activation * r.weight
  FROM graph_traverse gt
  JOIN memory_relations r ON
    r.status = 'active' AND r.deleted_at = ''
    AND (r.source_id = gt.node_id OR r.target_id = gt.node_id)
  WHERE gt.hop < ? AND gt.activation >= 0.01
)
SELECT gt.node_id, MIN(gt.hop) AS hop, MAX(gt.activation) AS activation,
       COALESCE(MIN(e.entity_type), '') AS entity_type, COALESCE(MIN(e.name), '') AS name
FROM graph_traverse gt
LEFT JOIN memory_entities e ON e.id = gt.node_id AND e.status = 'active' AND e.deleted_at = ''
GROUP BY gt.node_id
ORDER BY activation DESC
LIMIT ?`

// GraphTraverseCTE performs a single recursive-CTE traversal starting from
// centerID, propagating activation = parent_activation * edge_weight up to
// `hops` levels. It returns the reachable nodes (with activation + hop) and
// the edges among them.
//
// The traversal uses a single recursive CTE for node discovery (1 SQL round
// trip), plus 1 additional round trip to fetch the edges among the discovered
// nodes. This replaces the previous application-layer BFS which required 2N
// round trips for N hops (FR-10.11 / NFR-1.7).
//
// topK limits the number of returned nodes (highest activation first). hops
// is the maximum traversal depth (center node is hop=0; direct neighbors are
// hop=1). Edges with weight 0 are still traversed but contribute 0 activation.
func (r *l4EntityRepo) GraphTraverseCTE(ctx context.Context, centerID string, hops, topK int) (*biz.MemoryGraphTraversal, error) {
	if r == nil {
		return nil, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	centerID = strings.TrimSpace(centerID)
	if centerID == "" {
		return nil, apierror.BadRequest("MEMORY_L4", "center_id is required")
	}
	if hops <= 0 {
		hops = 3
	}
	if topK <= 0 {
		topK = 20
	}

	// Query 1: recursive CTE for nodes (1 SQL round trip).
	nodeRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(graphTraverseCTESQL),
		centerID, hops, topK)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer nodeRows.Close()

	var nodes []biz.MemoryGraphNode
	var nodeIDs []string
	for nodeRows.Next() {
		var n biz.MemoryGraphNode
		var nodeID, entityType, name string
		var hop int
		var activation float64
		if err := nodeRows.Scan(&nodeID, &hop, &activation, &entityType, &name); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		n.ID = nodeID
		n.Hop = hop
		n.Activation = activation
		n.EntityType = entityType
		n.Name = name
		nodes = append(nodes, n)
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}

	traversal := &biz.MemoryGraphTraversal{
		CenterID: centerID,
		Hops:     hops,
		Nodes:    nodes,
	}

	if len(nodeIDs) == 0 {
		return traversal, nil
	}

	// Query 2: fetch edges among the discovered nodes (1 SQL round trip).
	// We look up relations where either endpoint is in the node set.
	placeholders := make([]string, len(nodeIDs))
	args := make([]any, 0, len(nodeIDs)*2)
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, args...) // duplicate for second IN clause
	edgeQ := fmt.Sprintf(`SELECT source_id, target_id, relation_type, weight
FROM memory_relations
WHERE status = 'active' AND deleted_at = ''
  AND (source_id IN (%s) OR target_id IN (%s))`,
		strings.Join(placeholders, ","), strings.Join(placeholders, ","))
	edgeRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(edgeQ), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}
	defer edgeRows.Close()

	// Deduplicate edges: the IN (...) OR IN (...) clause may return the same
	// edge twice (once matching source_id, once matching target_id).
	seen := make(map[string]bool)
	for edgeRows.Next() {
		var e biz.MemoryGraphEdge
		if err := edgeRows.Scan(&e.SourceID, &e.TargetID, &e.RelationType, &e.Weight); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L4")
		}
		key := e.SourceID + "\x00" + e.TargetID + "\x00" + e.RelationType
		if seen[key] {
			continue
		}
		seen[key] = true
		traversal.Edges = append(traversal.Edges, e)
	}
	if err := edgeRows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L4")
	}

	return traversal, nil
}

// --- L4HebbianStore (Phase E, FR-10.4) ---

// findRelationSQL finds an active relation between nodeA and nodeB with the
// given relation type, JOINing memory_entities to read both endpoints'
// activation values. For bidirectional types, either (A→B) or (B→A) matches.
const findRelationSQL = `SELECT r.id, r.source_id, r.target_id, r.relation_type,
       r.weight, r.co_activation_count,
       COALESCE(es.activation, 0) AS source_activation,
       COALESCE(et.activation, 0) AS target_activation
FROM memory_relations r
LEFT JOIN memory_entities es ON es.id = r.source_id AND es.deleted_at = ''
LEFT JOIN memory_entities et ON et.id = r.target_id AND et.deleted_at = ''
WHERE r.deleted_at = '' AND r.status = 'active'
  AND r.relation_type = ?
  AND ((r.source_id = ? AND r.target_id = ?) OR (r.source_id = ? AND r.target_id = ?))
LIMIT 1`

// FindRelation finds an active relation between nodeA and nodeB with the
// given relation type. For bidirectional types, either direction matches.
func (r *l4EntityRepo) FindRelation(ctx context.Context, nodeA, nodeB, relationType string) (biz.L4HebbianRelation, bool, error) {
	if r == nil {
		return biz.L4HebbianRelation{}, false, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	nodeA = strings.TrimSpace(nodeA)
	nodeB = strings.TrimSpace(nodeB)
	if nodeA == "" || nodeB == "" || relationType == "" {
		return biz.L4HebbianRelation{}, false, nil
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(findRelationSQL),
		relationType, nodeA, nodeB, nodeB, nodeA)
	if err != nil {
		return biz.L4HebbianRelation{}, false, entErrToBizErr(err, "MEMORY_L4")
	}
	defer rows.Close()

	if !rows.Next() {
		return biz.L4HebbianRelation{}, false, entErrToBizErr(rows.Err(), "MEMORY_L4")
	}
	var rel biz.L4HebbianRelation
	if err := rows.Scan(&rel.ID, &rel.SourceID, &rel.TargetID, &rel.RelationType,
		&rel.Weight, &rel.CoActivationCount, &rel.SourceActivation, &rel.TargetActivation); err != nil {
		return biz.L4HebbianRelation{}, false, entErrToBizErr(err, "MEMORY_L4")
	}
	return rel, true, entErrToBizErr(rows.Err(), "MEMORY_L4")
}

// updateRelationWeightSQL updates weight, co_activation_count, and
// last_reinforced_at for a single relation.
const updateRelationWeightSQL = `UPDATE memory_relations
SET weight = ?, co_activation_count = ?, last_reinforced_at = ?, updated_at = ?
WHERE id = ? AND deleted_at = ''`

// UpdateRelationWeight updates weight, co_activation_count, and
// last_reinforced_at for the relation with the given ID.
func (r *l4EntityRepo) UpdateRelationWeight(ctx context.Context, relationID string, newWeight float64, coActivationCount int, lastReinforcedAtRFC3339 string) error {
	if r == nil {
		return apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	if relationID == "" {
		return apierror.BadRequest("MEMORY_L4", "relation_id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(updateRelationWeightSQL),
		newWeight, coActivationCount, lastReinforcedAtRFC3339, now, relationID)
	if err != nil {
		return entErrToBizErr(err, "MEMORY_L4")
	}
	return nil
}

// decayUnusedRelationsSQL decays weights of active relations whose
// last_reinforced_at (falling back to created_at) is older than the cutoff.
// Weight *= 0.95; relations whose weight drops below 0.1 are archived.
// This is done in two steps to avoid a complex single UPDATE.
const decayUnusedDecaySQL = `UPDATE memory_relations
SET weight = weight * 0.95, updated_at = ?
WHERE deleted_at = '' AND status = 'active'
  AND COALESCE(NULLIF(last_reinforced_at, ''), created_at) < ?`

const decayUnusedArchiveSQL = `UPDATE memory_relations
SET status = 'archived', archived_at = ?, updated_at = ?
WHERE deleted_at = '' AND status = 'active' AND weight < 0.1`

// DecayUnusedRelations decays weights of active relations whose
// last_reinforced_at (or created_at fallback) is older than olderThanRFC3339.
// Weight *= 0.95; relations whose weight drops below 0.1 are marked
// status='archived'. Returns counts of decayed and archived relations.
func (r *l4EntityRepo) DecayUnusedRelations(ctx context.Context, olderThanRFC3339 string) (biz.L4DecayResult, error) {
	if r == nil {
		return biz.L4DecayResult{}, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	if olderThanRFC3339 == "" {
		return biz.L4DecayResult{}, nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	writeDB := r.data.RWDB().WriteDB(ctx)

	// Step 1: decay weights of old relations.
	decayRes, err := writeDB.ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(decayUnusedDecaySQL),
		now, olderThanRFC3339)
	if err != nil {
		return biz.L4DecayResult{}, entErrToBizErr(err, "MEMORY_L4")
	}
	decayed, _ := decayRes.RowsAffected()

	// Step 2: archive relations whose weight dropped below 0.1.
	archiveRes, err := writeDB.ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(decayUnusedArchiveSQL),
		now, now)
	if err != nil {
		return biz.L4DecayResult{}, entErrToBizErr(err, "MEMORY_L4")
	}
	archived, _ := archiveRes.RowsAffected()

	return biz.L4DecayResult{
		Decayed:  int(decayed),
		Archived: int(archived),
	}, nil
}

// --- L4ReconsolidationStore (Phase E, FR-10.5) ---

// boostActivationSQL atomically increases activation by delta (saturated to
// 1.0) using a CASE expression for cross-dialect compatibility (SQLite and
// Postgres both support standard CASE WHEN). activation_updated_at and
// updated_at are stamped to now. Only matches non-deleted entities.
const boostActivationSQL = `UPDATE memory_entities
SET activation = CASE
        WHEN activation + ? > 1.0 THEN 1.0
        ELSE activation + ?
    END,
    activation_updated_at = ?,
    updated_at = ?
WHERE id = ? AND deleted_at = ''`

// BoostActivation atomically increases activation by delta (saturated to 1.0)
// and sets activation_updated_at to nowRFC3339. Returns ok=false if the entity
// was not found or is soft-deleted (no rows affected).
func (r *l4EntityRepo) BoostActivation(ctx context.Context, nodeID string, delta float64, nowRFC3339 string) (bool, error) {
	if r == nil {
		return false, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, apierror.BadRequest("MEMORY_L4", "node_id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(boostActivationSQL),
		delta, delta, nowRFC3339, now, nodeID)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_L4")
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// incrementUseCountSQL atomically increments use_count by 1 for a single
// non-deleted entity.
const incrementUseCountSQL = `UPDATE memory_entities
SET use_count = use_count + 1, updated_at = ?
WHERE id = ? AND deleted_at = ''`

// IncrementUseCount atomically increments use_count by 1 for the given entity.
// Returns ok=false if the entity was not found or is soft-deleted (no rows
// affected).
func (r *l4EntityRepo) IncrementUseCount(ctx context.Context, nodeID string) (bool, error) {
	if r == nil {
		return false, apierror.Internal("MEMORY_L4", "l4 repo not initialized")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false, apierror.BadRequest("MEMORY_L4", "node_id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(incrementUseCountSQL),
		now, nodeID)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_L4")
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// --- L4ConflictStore (Phase E, FR-10.6) ---

// createInhibitRelationSQL inserts (or upserts) a directed INHIBIT relation
// from SourceID to TargetID with the given weight and context_note. The
// ON CONFLICT clause updates weight/context_note/status on existing relations
// (idempotent re-resolution of the same conflict).
const createInhibitRelationSQL = `INSERT INTO memory_relations (
	id, scope_type, scope_id, workspace_id,
	source_id, target_id, relation_type, bidirectional,
	weight, confidence, importance, use_count,
	attributes_json, evidence_json, status, source_kind,
	metadata_json, valid_from, valid_to, created_at, updated_at, context_note
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
	weight = excluded.weight, context_note = excluded.context_note,
	status = 'active', updated_at = excluded.updated_at`

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
		weight = 0.8 // default strong inhibition
	}

	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(createInhibitRelationSQL),
		id,
		strings.TrimSpace(params.ScopeType),
		strings.TrimSpace(params.ScopeID),
		"", // workspace_id
		sourceID,
		targetID,
		biz.RelationInhibit,
		0,                   // bidirectional=0 (INHIBIT is directed)
		weight, 0.5, 0.5, 0, // weight, confidence, importance, use_count
		"{}", "{}", "active", "", // attributes_json, evidence_json, status, source_kind
		"{}", now, "", now, now, // metadata_json, valid_from, valid_to, created_at, updated_at
		params.ContextNote, // context_note
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

// --- L4KnowledgeBridgeStore (Phase E, FR-11.7 / FR-10.10) ---

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

	// Build SQL with dialect-aware JSON extraction expression.
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
