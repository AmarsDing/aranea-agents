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

// Compile-time interface checks.
var (
	_ biz.L4EntityStore    = (*l4EntityRepo)(nil)
	_ biz.L4EvolutionStore = (*l4EntityRepo)(nil)
)

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
		clauses = append(clauses, "workspace_id = ?")
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
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), "SELECT COUNT(*) FROM memory_entities"+where, args, &total); err != nil {
		return nil, 0, err
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	lg := r.data.lg
	var out [][]byte
	for rows.Next() {
		b, err := scanEntityRowJSON(rows, lg)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
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
		rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, relQ, args...)
		if err != nil {
			return nil, err
		}
		lg := r.data.lg
		for rows.Next() {
			b, err := scanRelationRowJSON(rows)
			if err != nil {
				rows.Close()
				return nil, err
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
			entRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, entQ, args2...)
			if err != nil {
				return nil, err
			}
			for entRows.Next() {
				b, err := scanEntityRowJSON(entRows, lg)
				if err != nil {
					entRows.Close()
					return nil, err
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
	centerRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		"SELECT"+sqlEntityCols+" FROM memory_entities WHERE id = ? AND status = 'active' AND deleted_at = ''", centerID)
	if err != nil {
		return nil, err
	}
	lg := r.data.lg
	for centerRows.Next() {
		b, err := scanEntityRowJSON(centerRows, lg)
		if err != nil {
			centerRows.Close()
			return nil, err
		}
		entities = append([][]byte{b}, entities...)
	}
	centerRows.Close()

	result := map[string]any{
		"center_id":  centerID,
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
		`SELECT agent_id, persona, values_json, tone, domains_json, user_expectations,
		        current_phase, metadata_json, version, created_at, updated_at
		 FROM agent_identity WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return json.Marshal(map[string]any{"agent_id": agentID, "identity": nil})
	}
	var aid, persona, valuesJSON, tone, domainsJSON, userExp, phase, meta, ca, ua string
	var version int
	if err := rows.Scan(&aid, &persona, &valuesJSON, &tone, &domainsJSON, &userExp, &phase, &meta, &version, &ca, &ua); err != nil {
		return nil, err
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
		`SELECT agent_id, exploration, conciseness, caution, delegation,
		        tool_preference_json, tool_blacklist_json,
		        provider_preference_json, model_preference_json,
		        stats_json, metadata_json, version, created_at, updated_at
		 FROM agent_strategy_profile WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	strategy := map[string]any{
		"agent_id":     aid,
		"exploration":  exploration,
		"conciseness":  conciseness,
		"caution":      caution,
		"delegation":   delegation,
		"tool_preference":     decodeJSONFloatMap(toolPrefJSON, r.data.lg),
		"tool_blacklist":      decodeJSONStringSlice(toolBlacklistJSON, r.data.lg),
		"provider_preference": decodeJSONFloatMap(providerPrefJSON, r.data.lg),
		"model_preference":    decodeJSONFloatMap(modelPrefJSON, r.data.lg),
		"stats_json":   statsJSON,
		"metadata_json": meta,
		"version":      version,
		"created_at":   ca,
		"updated_at":   ua,
	}
	return json.Marshal(map[string]any{"agent_id": agentID, "strategy": strategy})
}

func (r *l4EntityRepo) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Soft-delete entities whose scope points to this session
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_entities SET deleted_at = ?, status = 'deleted' WHERE scope_id = ? AND scope_type = 'session' AND entity_type = 'event'`,
		now, sessionID)
	return err
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
	q := cascadeProposalSelect + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanCascadeProposalJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lg := r.data.lg
	var out [][]byte
	for rows.Next() {
		var id, aid, wid, ek, tf, reason, tk, ts, meta, ca string
		var reverted int
		if err := rows.Scan(&id, &aid, &wid, &ek, &tf, &reason, &tk, &ts, &meta, &ca, &reverted); err != nil {
			return nil, err
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
		b, _ := json.Marshal(m)
		out = append(out, b)
	}
	return out, rows.Err()
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
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), metricsQ, args, &total, &pending, &approved, &rejected); err != nil {
		return nil, err
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO agent_evolution_events (
		id, agent_id, workspace_id, event_kind, target_field,
		before_json, after_json, diff_json,
		trigger_kind, trigger_source, evidence_json, reason,
		applied, reverted, reverted_by_event_id,
		metadata_json, created_at, applied_at, reverted_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
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
		1,    // applied
		0,    // reverted
		"",   // reverted_by_event_id
		meta, now, now, "", // applied_at=now, reverted_at=''
	)
	if err != nil {
		return nil, err
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, agent_id, workspace_id, event_kind, target_field, reason,
		        trigger_kind, trigger_source, metadata_json, created_at, reverted
		 FROM agent_evolution_events WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound("MEMORY", "evolution event not found after insert")
	}
	var rowID, aid, wid, ek, tf, reason, tk, ts, m, ca string
	var reverted int
	if err := rows.Scan(&rowID, &aid, &wid, &ek, &tf, &reason, &tk, &ts, &m, &ca, &reverted); err != nil {
		return nil, err
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
