package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/pkg/loggateway"
)

const sqlEntityCols = `
	id, scope_type, scope_id, workspace_id, user_id,
	entity_type, name, name_normalized, aliases_json, description, attributes_json,
	importance, confidence, use_count, source_kind,
	embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
	status, merged_into,
	metadata_json, created_at, updated_at, archived_at, deleted_at`

const sqlRelationCols = `
	id, scope_type, scope_id, workspace_id,
	source_id, target_id, relation_type, bidirectional,
	weight, confidence, importance, use_count,
	attributes_json, evidence_json, status, source_kind,
	metadata_json, valid_from, valid_to, created_at, updated_at, archived_at, deleted_at`

// ListEntityRows lists entities (**snake_case** JSON per row).
func (st *Store) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	conds := []string{}
	args := []any{}
	if scopeType != "" {
		conds = append(conds, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		conds = append(conds, "scope_id = ?")
		args = append(args, scopeID)
	}
	if workspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if userID != "" {
		conds = append(conds, "user_id = ?")
		args = append(args, userID)
	}
	if entityType != "" {
		conds = append(conds, "entity_type = ?")
		args = append(args, entityType)
	}
	if status != "" {
		conds = append(conds, "status = ?")
		args = append(args, status)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		conds = append(conds, "(LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := queryOne(ctx, st.client, `SELECT COUNT(*) FROM memory_entities`+where, args, &total); err != nil {
		return nil, 0, err
	}
	lim := int(limit)
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, lim, off)
	q := `SELECT ` + strings.TrimSpace(sqlEntityCols) + ` FROM memory_entities` + where + ` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`
	rows, err := st.client.QueryContext(ctx, q, listArgs...)
	if err != nil {
		return nil, int32(total), err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanEntityRowJSON(rows, st.lg)
		if err != nil {
			return nil, int32(total), err
		}
		out = append(out, b)
	}
	return out, int32(total), rows.Err()
}

func scanEntityRowJSON(rows *sql.Rows, lg loggateway.Logger) ([]byte, error) {
	var (
		id, scopeType, scopeID, wid, uid, etype, name, nnorm, aliases, desc, attr string
		imp, conf                                                                 float64
		uc                                                                        int
		src                                                                       string
		embSt, embModel                                                           string
		embDim                                                                    int
		embBlob                                                                   []byte
		embNorm                                                                   float64
		status, merged, meta, ca, ua, arch, del                                   string
	)
	if err := rows.Scan(
		&id, &scopeType, &scopeID, &wid, &uid, &etype, &name, &nnorm, &aliases, &desc, &attr,
		&imp, &conf, &uc, &src,
		&embSt, &embModel, &embDim, &embBlob, &embNorm,
		&status, &merged,
		&meta, &ca, &ua, &arch, &del,
	); err != nil {
		return nil, err
	}
	aliasesArr := decodeJSONStringSlice(aliases, lg)
	m := map[string]any{
		"id": id, "scope_type": scopeType, "scope_id": scopeID,
		"workspace_id": wid, "user_id": uid,
		"entity_type": etype, "name": name, "name_normalized": nnorm,
		"aliases":     aliasesArr,
		"description": desc,
		"importance":  imp, "confidence": conf, "use_count": uc,
		"source_kind":      src,
		"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,
		"embedding_norm": embNorm,
		"status":         status, "merged_into": merged,
		"metadata_json": meta, "created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
	}
	if attr != "" && attr != "{}" {
		m["attributes_json"] = attr
	}
	return json.Marshal(m)
}

func (st *Store) getEntityJSON(ctx context.Context, id string) ([]byte, error) {
	rows, err := st.client.QueryContext(ctx, `SELECT `+strings.TrimSpace(sqlEntityCols)+` FROM memory_entities WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanEntityRowJSON(rows, st.lg)
}

func (st *Store) listRelationsForNode(ctx context.Context, nodeID, queryAt string, limit int) ([][]byte, error) {
	if nodeID == "" {
		return nil, errors.New("node id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	queryAt = defaultQueryTimeRFC3339(queryAt)
	rows, err := st.client.QueryContext(ctx,
		`SELECT `+strings.TrimSpace(sqlRelationCols)+` FROM memory_relations
		 WHERE (source_id = ? OR target_id = ?) AND status = 'active' AND deleted_at = ''
		 ORDER BY weight DESC, updated_at DESC LIMIT ?`,
		nodeID, nodeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanRelationRowJSON(rows)
		if err != nil {
			return nil, err
		}
		var rel map[string]any
		if err := json.Unmarshal(b, &rel); err != nil {
			st.lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
			continue
		}
		vf, _ := rel["valid_from"].(string)
		vt, _ := rel["valid_to"].(string)
		ca, _ := rel["created_at"].(string)
		if !relationValidAt(vf, vt, ca, queryAt) {
			continue
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanRelationRowJSON(rows *sql.Rows) ([]byte, error) {
	var (
		id, stype, sid, wid, srcID, tgtID, relType string
		bidir                                      int
		w, conf, imp                               float64
		uc                                         int
		attrJ, evidJ, status, srcKind, metaJ       string
		validFrom, validTo                         string
		ca, ua, arch, del                          string
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &srcID, &tgtID, &relType, &bidir,
		&w, &conf, &imp, &uc,
		&attrJ, &evidJ, &status, &srcKind,
		&metaJ, &validFrom, &validTo, &ca, &ua, &arch, &del,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "scope_type": stype, "scope_id": sid, "workspace_id": wid,
		"source_id": srcID, "target_id": tgtID, "relation_type": relType,
		"bidirectional": bidir != 0,
		"weight":        w, "confidence": conf, "importance": imp, "use_count": uc,
		"attributes_json": attrJ, "evidence_json": evidJ,
		"status": status, "source_kind": srcKind,
		"metadata_json": metaJ,
		"valid_from":    validFrom,
		"valid_to":      validTo,
		"created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
	}
	return json.Marshal(m)
}

// NeighborhoodJSON returns the legacy **GraphNeighborhood** JSON shape.
func (st *Store) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if strings.TrimSpace(centerID) == "" {
		return nil, errors.New("center id is required")
	}
	h := int(hops)
	if h <= 0 {
		h = 1
	}
	if h > 3 {
		h = 3
	}
	mx := int(maxNodes)
	if mx <= 0 || mx > 200 {
		mx = 25
	}
	queryAt := defaultQueryTimeRFC3339(queryAtRFC3339)
	centerRaw, err := st.getEntityJSON(ctx, centerID)
	if err != nil {
		return nil, err
	}
	var centerObj map[string]any
	if err := json.Unmarshal(centerRaw, &centerObj); err != nil {
		st.lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil, err
	}

	visited := map[string]bool{centerID: true}
	frontier := []string{centerID}
	relSeen := map[string]bool{}
	var entities []any
	var relations []any

	for hop := 1; hop <= h && len(visited) < mx+1 && len(frontier) > 0; hop++ {
		next := []string{}
		for _, node := range frontier {
			rels, err := st.listRelationsForNode(ctx, node, queryAt, 100)
			if err != nil {
				return nil, err
			}
			for _, raw := range rels {
				var rel map[string]any
				if err := json.Unmarshal(raw, &rel); err != nil {
					st.lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
					continue
				}
				rid, _ := rel["id"].(string)
				if rid == "" || relSeen[rid] {
					continue
				}
				relSeen[rid] = true
				relations = append(relations, rel)
				srcID, _ := rel["source_id"].(string)
				tgtID, _ := rel["target_id"].(string)
				other := tgtID
				if other == node {
					other = srcID
				}
				if other == "" || visited[other] {
					continue
				}
				if len(visited) >= mx+1 {
					continue
				}
				visited[other] = true
				entRaw, err := st.getEntityJSON(ctx, other)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					return nil, err
				}
				var entObj map[string]any
				if err := json.Unmarshal(entRaw, &entObj); err != nil {
					st.lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
					continue
				}
				entObj["hop"] = hop
				entities = append(entities, entObj)
				next = append(next, other)
			}
		}
		frontier = next
	}

	out := map[string]any{
		"center":    centerObj,
		"hops":      h,
		"query_at":  queryAt,
		"entities":  entities,
		"relations": relations,
	}
	return json.Marshal(out)
}

// --- Evolution (SQLite) ---

// AgentIdentityJSON returns JSON for **GET …/identity** wire shape.
func (st *Store) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	rows, err := st.client.QueryContext(ctx,
		`SELECT agent_id, persona, values_json, tone, domains_json, user_expectations, current_phase, metadata_json, version, created_at, updated_at
		 FROM agent_identity WHERE agent_id = ?`, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		m := map[string]any{
			"agent_id": agentID, "persona": "", "values": []string{}, "tone": "",
			"domains": []string{}, "user_expectations": "", "current_phase": "cold-start",
			"version": 1,
		}
		return json.Marshal(m)
	}
	var (
		aid, persona, vals, tone, doms, ue, phase, meta string
		ver                                             int
		ca, ua                                          string
	)
	err = rows.Scan(&aid, &persona, &vals, &tone, &doms, &ue, &phase, &meta, &ver, &ca, &ua)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m := map[string]any{
		"agent_id":          aid,
		"persona":           persona,
		"values":            decodeJSONStringSlice(vals, st.lg),
		"tone":              tone,
		"domains":           decodeJSONStringSlice(doms, st.lg),
		"user_expectations": ue,
		"current_phase":     phase,
		"version":           ver,
	}
	if meta != "" && meta != "{}" {
		m["metadata"] = decodeJSONObject(meta, st.lg)
	}
	return json.Marshal(m)
}

func (st *Store) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	rows, err := st.client.QueryContext(ctx,
		`SELECT agent_id, exploration, conciseness, caution, delegation,
				tool_preference_json, tool_blacklist_json, provider_preference_json, model_preference_json,
				stats_json, metadata_json, version, created_at, updated_at
		 FROM agent_strategy_profile WHERE agent_id = ?`, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		m := map[string]any{
			"agent_id": agentID, "exploration": 0.5, "conciseness": 0.5, "caution": 0.5, "delegation": 0.5,
			"tool_preference": map[string]float64{}, "tool_blacklist": []string{},
			"provider_preference": map[string]float64{}, "model_preference": map[string]float64{},
			"version": 1,
		}
		return json.Marshal(m)
	}
	var (
		aid                                   string
		ex, co, ca, de                        float64
		toolPref, toolBL, provPref, modelPref string
		statsRaw, metaRaw                     string
		ver                                   int
		caS, uaS                              string
	)
	if err := rows.Scan(&aid, &ex, &co, &ca, &de, &toolPref, &toolBL, &provPref, &modelPref, &statsRaw, &metaRaw, &ver, &caS, &uaS); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m := map[string]any{
		"agent_id":            aid,
		"exploration":         ex,
		"conciseness":         co,
		"caution":             ca,
		"delegation":          de,
		"tool_preference":     decodeJSONFloatMap(toolPref, st.lg),
		"tool_blacklist":      decodeJSONStringSlice(toolBL, st.lg),
		"provider_preference": decodeJSONFloatMap(provPref, st.lg),
		"model_preference":    decodeJSONFloatMap(modelPref, st.lg),
		"version":             ver,
	}
	if statsRaw != "" && statsRaw != "{}" {
		m["stats"] = decodeJSONObject(statsRaw, st.lg)
	}
	if metaRaw != "" && metaRaw != "{}" {
		m["metadata"] = decodeJSONObject(metaRaw, st.lg)
	}
	return json.Marshal(m)
}

// EvolutionProposalRows lists proposals for an agent.
func (st *Store) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	conds := []string{"agent_id = ?"}
	args := []any{agentID}
	if status != "" {
		conds = append(conds, "status = ?")
		args = append(args, status)
	}
	where := " WHERE " + strings.Join(conds, " AND ")
	lim := int(limit)
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	args = append(args, lim)
	q := `SELECT id, agent_id, workspace_id, proposal_kind, target_field,
				proposed_value_json, current_value_json, diff_json,
				rationale, evidence_json, expected_impact, risk_level, approval_required,
				status, reviewed_by, reviewed_at, applied_event_id, expires_at,
				source, metadata_json, created_at, updated_at
		 FROM agent_evolution_proposals` + where + ` ORDER BY created_at DESC LIMIT ?`
	rows, err := st.client.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, aid, wid, pkind, tgt, prop, curr, diff, rat, evid, expimp, risk, st, revBy, revAt, applEv, expAt, src, meta, ca, ua string
			appReq                                                                                                                  int
		)
		if err := rows.Scan(&id, &aid, &wid, &pkind, &tgt, &prop, &curr, &diff, &rat, &evid, &expimp, &risk, &appReq, &st, &revBy, &revAt, &applEv, &expAt, &src, &meta, &ca, &ua); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "agent_id": aid, "workspace_id": wid,
			"proposal_kind": pkind, "kind": pkind,
			"target_field":        tgt,
			"proposed_value_json": prop, "current_value_json": curr, "diff_json": diff,
			"rationale": rat, "expected_impact": expimp, "risk_level": risk,
			"status": st, "source": src,
			"reviewed_by": revBy, "reviewed_at": revAt, "applied_event_id": applEv,
			"expires_at": expAt, "approval_required": appReq != 0,
			"created_at": ca, "updated_at": ua,
		}
		if evid != "" && evid != "[]" {
			m["evidence_json"] = evid
		}
		if meta != "" && meta != "{}" {
			m["metadata_json"] = meta
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// EvolutionEventRows lists evolution events.
func (st *Store) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	lim := int(limit)
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	q := `SELECT id, agent_id, workspace_id, event_kind, target_field,
				before_json, after_json, diff_json,
				trigger_kind, trigger_source, evidence_json, reason,
				applied, reverted, reverted_by_event_id,
				metadata_json, created_at, applied_at, reverted_at
		 FROM agent_evolution_events WHERE agent_id = ? ORDER BY created_at DESC LIMIT ?`
	rows, err := st.client.QueryContext(ctx, q, agentID, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, aid, wid, ekind, tgt, before, after, diff string
			trig, trSrc, evid, reason, meta               string
			applied, reverted                             int
			revBy, ca, applAt, revAt                      string
		)
		if err := rows.Scan(&id, &aid, &wid, &ekind, &tgt, &before, &after, &diff, &trig, &trSrc, &evid, &reason, &applied, &reverted, &revBy, &meta, &ca, &applAt, &revAt); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "agent_id": aid, "workspace_id": wid,
			"event_kind": ekind, "kind": ekind,
			"target_field": tgt,
			"before_json":  before, "after_json": after, "diff_json": diff,
			"trigger_kind": trig, "trigger_source": trSrc,
			"reason":  reason,
			"applied": applied != 0, "reverted": reverted != 0,
			"reverted_by_event_id": revBy,
			"created_at":           ca, "applied_at": applAt, "reverted_at": revAt,
		}
		if evid != "" && evid != "[]" {
			m["evidence_json"] = evid
		}
		if meta != "" && meta != "{}" {
			m["metadata_json"] = meta
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// EvolutionMetricsJSON mirrors legacy **Metrics** aggregation.
func (st *Store) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	if agentID == "" {
		return nil, errors.New("agent id is required")
	}
	evRows, err := st.client.QueryContext(ctx,
		`SELECT reverted FROM agent_evolution_events WHERE agent_id = ? LIMIT 500`, agentID)
	if err != nil {
		return nil, err
	}
	defer evRows.Close()
	eventsTotal := 0
	eventsRev := 0
	for evRows.Next() {
		var rev int
		if err := evRows.Scan(&rev); err != nil {
			return nil, err
		}
		eventsTotal++
		if rev != 0 {
			eventsRev++
		}
	}
	if err := evRows.Err(); err != nil {
		return nil, err
	}

	propRows, err := st.client.QueryContext(ctx,
		`SELECT status FROM agent_evolution_proposals WHERE agent_id = ? LIMIT 500`, agentID)
	if err != nil {
		return nil, err
	}
	defer propRows.Close()
	propTotals := 0
	byStatus := map[string]int{}
	for propRows.Next() {
		var st string
		if err := propRows.Scan(&st); err != nil {
			return nil, err
		}
		propTotals++
		byStatus[st]++
	}
	if err := propRows.Err(); err != nil {
		return nil, err
	}

	statsRows, err := st.client.QueryContext(ctx,
		`SELECT agent_id, scope, scope_value, tool_key,
				invocations, successes, failures, user_overrides,
				avg_latency_ms, avg_tokens, preference_score, last_used_at,
				metadata_json, updated_at
		 FROM agent_skill_stats WHERE agent_id = ? ORDER BY preference_score DESC, invocations DESC LIMIT 20`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer statsRows.Close()
	var skillStats []any
	for statsRows.Next() {
		var aid, scope, sval, tool string
		var inv, ok, fail, uo int
		var lat, tok, pref float64
		var last, meta, up string
		if err := statsRows.Scan(&aid, &scope, &sval, &tool, &inv, &ok, &fail, &uo, &lat, &tok, &pref, &last, &meta, &up); err != nil {
			return nil, err
		}
		skillStats = append(skillStats, map[string]any{
			"agent_id":         aid,
			"scope":            scope,
			"scope_value":      sval,
			"tool_key":         tool,
			"invocations":      inv,
			"successes":        ok,
			"failures":         fail,
			"user_overrides":   uo,
			"avg_latency_ms":   lat,
			"avg_tokens":       tok,
			"preference_score": pref,
			"last_used_at":     last,
			"metadata_json":    meta,
			"updated_at":       up,
		})
	}
	if err := statsRows.Err(); err != nil {
		return nil, err
	}

	byStatusNum := map[string]int32{}
	for k, v := range byStatus {
		byStatusNum[k] = int32(v)
	}

	out := map[string]any{
		"events_total":        eventsTotal,
		"events_reverted":     eventsRev,
		"proposals_total":     propTotals,
		"proposals_by_status": byStatusNum,
		"skill_stats":         skillStats,
	}
	return json.Marshal(out)
}
