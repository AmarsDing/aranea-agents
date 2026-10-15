package data

import (
	"aranea-agents/pkg/loggateway"
	"database/sql"
	"encoding/json"
)

func scanL0SnapshotRow(rows *sql.Rows) ([]byte, error) {
	var (
		id, sessID, runID, turnID, spanID, agentID, teamID, provider, model string
		cwt, bt, rwt, rwtok, ste, l1fc, l1te, l3c, l3te, l4p, l4te          int
		pte, pta                                                            int
		ur                                                                  float64
		ts                                                                  string
		tmc, stf, ste2                                                      int
		segs, warns, meta, cat                                              string
	)
	if err := rows.Scan(
		&id, &sessID, &runID, &turnID, &spanID, &agentID, &teamID, &provider, &model,
		&cwt, &bt, &rwt, &rwtok, &ste, &l1fc, &l1te, &l3c, &l3te, &l4p, &l4te,
		&pte, &pta, &ur, &ts, &tmc, &stf, &ste2,
		&segs, &warns, &meta, &cat,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "session_id": sessID, "run_id": runID, "turn_id": turnID, "span_id": spanID,
		"agent_id": agentID, "team_id": teamID, "provider": provider, "model": model,
		"context_window_tokens": cwt, "budget_tokens": bt,
		"recent_window_turns": rwt, "recent_window_tokens": rwtok,
		"summary_token_estimate": ste,
		"l1_field_count":         l1fc, "l1_token_estimate": l1te,
		"l3_chunk_count": l3c, "l3_token_estimate": l3te,
		"l4_path_count": l4p, "l4_token_estimate": l4te,
		"prompt_token_estimate": pte, "prompt_token_actual": pta,
		"used_ratio": ur, "truncate_strategy": ts,
		"truncated_message_count": tmc, "summarized_turn_from": stf, "summarized_turn_to": ste2,
		"segments_json": segs, "warning_codes_json": warns, "metadata_json": meta,
		"created_at": cat,
	}
	return json.Marshal(m)
}

func scanL1TaskRow(rows *sql.Rows) ([]byte, error) {
	var (
		id, sessID, runID, teamID, agentID   string
		taskKey, taskTitle, taskGoal, status string
		schemaVer                            int
		budgetTok, usedTok                   int
		parentTaskID, sharedWithJSON         string
		startedAt, endedAt, archivedAt       string
		metadataJSON, createdAt, updatedAt   string
	)
	if err := rows.Scan(
		&id, &sessID, &runID, &teamID, &agentID,
		&taskKey, &taskTitle, &taskGoal, &status,
		&schemaVer, &budgetTok, &usedTok,
		&parentTaskID, &sharedWithJSON,
		&startedAt, &endedAt, &archivedAt,
		&metadataJSON, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "session_id": sessID, "run_id": runID, "team_id": teamID, "agent_id": agentID,
		"task_key": taskKey, "task_title": taskTitle, "task_goal": taskGoal, "status": status,
		"schema_version": schemaVer, "budget_tokens": budgetTok, "used_tokens": usedTok,
		"parent_task_id": parentTaskID, "shared_with_json": sharedWithJSON,
		"started_at": startedAt, "ended_at": endedAt, "archived_at": archivedAt,
		"metadata_json": metadataJSON, "created_at": createdAt, "updated_at": updatedAt,
	}
	return json.Marshal(m)
}

func scanL1FieldRow(rows *sql.Rows) ([]byte, error) {
	var (
		id, taskID, sessID, agentID             string
		fieldPath, fieldKind, visibility        string
		pinToPrompt, isRequired                 int
		valueText, valueJSON, valueRef, preview string
		tokenEst                                int
		source, sourceRef                       string
		ttlSec                                  int
		expiresAt                               string
		revision                                int
		lastReadAt                              string
		readCount                               int
		metadataJSON, createdAt, updatedAt      string
	)
	if err := rows.Scan(
		&id, &taskID, &sessID, &agentID,
		&fieldPath, &fieldKind, &visibility, &pinToPrompt, &isRequired,
		&valueText, &valueJSON, &valueRef, &preview, &tokenEst,
		&source, &sourceRef, &ttlSec, &expiresAt,
		&revision, &lastReadAt, &readCount,
		&metadataJSON, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "task_id": taskID, "session_id": sessID, "agent_id": agentID,
		"field_path": fieldPath, "field_kind": fieldKind, "visibility": visibility,
		"pin_to_prompt": pinToPrompt != 0, "is_required": isRequired != 0,
		"value_text": valueText, "value_json": valueJSON, "value_ref": valueRef,
		"preview": preview, "token_estimate": tokenEst,
		"source": source, "source_ref": sourceRef,
		"ttl_seconds": ttlSec, "expires_at": expiresAt,
		"revision": revision, "last_read_at": lastReadAt, "read_count": readCount,
		"metadata_json": metadataJSON, "created_at": createdAt, "updated_at": updatedAt,
	}
	return json.Marshal(m)
}

func scanFactRowJSON(rows *sql.Rows) ([]byte, error) {
	var (
		id, stype, sid, wid, uid, tid, aid string
		stmt, snorm, fp, details           string
		fkind, tags                        string
		conf, imp                          float64
		uc, hc, pfc, nfc, cc               int
		rc, ic, cic                        int
		srcKind, epID, sessID, msgID, ext  string
		ver                                int
		st, sup                            string
		embSt, embModel                    string
		embDim                             int
		embBlob                            []byte
		embNorm                            float64
		pii                                int
		redacted                           string
		ttlD                               int
		decay                              float64
		nextD, lastU, exp                  string
		meta, ca, ua, arch, del            string
		qScore                             float64
		piiTypes                           string
		validFrom, validUntil              string
		links, keywords, llmTags           string
		decayScore                         float64
		contextNote                        string
		indexAttempts                      int
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &uid, &tid, &aid,
		&stmt, &snorm, &fp, &details,
		&fkind, &tags,
		&conf, &imp, &uc, &hc,
		&rc, &ic, &cic,
		&pfc, &nfc, &cc,
		&srcKind, &epID, &sessID, &msgID, &ext,
		&ver, &st, &sup,
		&embSt, &embModel, &embDim, &embBlob, &embNorm,
		&pii, &redacted,
		&ttlD, &decay, &nextD, &lastU, &exp,
		&meta, &qScore, &piiTypes, &ca, &ua, &arch, &del,
		&validFrom, &validUntil, &links, &keywords, &llmTags,
		&decayScore, &contextNote, &indexAttempts,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "scope_type": stype, "scope_id": sid, "workspace_id": wid,
		"user_id": uid, "team_id": tid, "agent_id": aid,
		"statement": stmt, "statement_normalized": snorm, "fingerprint": fp,
		"details_markdown": details, "fact_kind": fkind, "tags_json": tags,
		"confidence": conf, "importance": imp,
		"use_count": uc, "hit_count": hc,
		"recalled_count": rc, "injected_count": ic, "cited_count": cic,
		"positive_feedback_count": pfc, "negative_feedback_count": nfc, "conflict_count": cc,
		"source_kind": srcKind, "source_episode_id": epID,
		"source_session_id": sessID, "source_message_id": msgID, "source_external": ext,
		"version": ver, "status": st, "superseded_by": sup,
		"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,

		"embedding_blob":     embBlob,
		"embedding_norm":     embNorm,
		"pii_flag":           pii != 0,
		"redacted_statement": redacted,
		"ttl_days":           ttlD, "decay_factor": decay,
		"next_decay_at": nextD, "last_used_at": lastU, "expires_at": exp,
		"metadata_json": meta, "quality_score": qScore, "pii_types": piiTypes, "created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
		"valid_from": validFrom, "valid_until": validUntil,
		"links": links, "keywords": keywords, "tags": llmTags,
		"decay_score": decayScore, "context_note": contextNote,
		"index_attempts": indexAttempts,
	}
	return json.Marshal(m)
}

func scanEpisodeRowJSON(rows *sql.Rows) ([]byte, error) {
	var (
		id, sessionID, agentID, kind, title, summary, status, meta, endedAt, createdAt string
		importance                                                                     float64
		l3Count                                                                        int
	)
	if err := rows.Scan(&id, &sessionID, &agentID, &kind, &title, &summary, &importance, &status, &l3Count, &meta, &endedAt, &createdAt); err != nil {
		return nil, err
	}
	row := map[string]any{
		"id": id, "session_id": sessionID, "agent_id": agentID, "episode_kind": kind,
		"title": title, "outcome_summary": summary, "importance": importance,
		"consolidation_status": status, "consolidated_l3_count": l3Count,
		"metadata_json": meta, "ended_at": endedAt, "created_at": createdAt,
	}
	return json.Marshal(row)
}

func scanCascadeProposalJSON(rows *sql.Rows) ([]byte, error) {
	var (
		id, aid, wid, teid, tename, attr, oldV, newV, affected, status, risk, rationale, meta string
		reviewedBy, reviewedAt, expiresAt, ca, ua                                             string
	)
	if err := rows.Scan(&id, &aid, &wid, &teid, &tename, &attr, &oldV, &newV, &affected, &status, &risk, &rationale, &meta,
		&reviewedBy, &reviewedAt, &expiresAt, &ca, &ua); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "agent_id": aid, "workspace_id": wid,
		"trigger_entity_id": teid, "trigger_entity_name": tename, "trigger_attribute": attr,
		"old_value": oldV, "new_value": newV, "affected_json": affected,
		"status": status, "risk_level": risk, "rationale": rationale, "metadata_json": meta,
		"reviewed_by": reviewedBy, "reviewed_at": reviewedAt, "expires_at": expiresAt,
		"created_at": ca, "updated_at": ua,
	}
	return json.Marshal(m)
}

func scanEvolutionProposalJSON(rows *sql.Rows) ([]byte, error) {
	var id, aid, wid, kind, field, rationale, impact, risk, status, ca string
	if err := rows.Scan(&id, &aid, &wid, &kind, &field, &rationale, &impact, &risk, &status, &ca); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "agent_id": aid, "workspace_id": wid,
		"proposal_kind": kind, "kind": kind, "target_field": field,
		"rationale": rationale, "expected_impact": impact,
		"risk_level": risk, "status": status, "created_at": ca,
	}
	return json.Marshal(m)
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
		// 20261005 neuron enhancement fields
		activation, valence, arousal    float64
		activationUpdatedAt, sourceType string
	)
	if err := rows.Scan(
		&id, &scopeType, &scopeID, &wid, &uid, &etype, &name, &nnorm, &aliases, &desc, &attr,
		&imp, &conf, &uc, &src,
		&embSt, &embModel, &embDim, &embBlob, &embNorm,
		&status, &merged,
		&meta, &ca, &ua, &arch, &del,
		&activation, &activationUpdatedAt, &sourceType, &valence, &arousal,
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

		"activation":            activation,
		"activation_updated_at": activationUpdatedAt,
		"source_type":           sourceType,
		"valence":               valence,
		"arousal":               arousal,
	}
	if attr != "" && attr != "{}" {
		m["attributes_json"] = attr
	}
	return json.Marshal(m)
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
		// 20261005 neuron enhancement fields
		coActCount                    int
		lastReinforcedAt, contextNote string
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &srcID, &tgtID, &relType, &bidir,
		&w, &conf, &imp, &uc,
		&attrJ, &evidJ, &status, &srcKind,
		&metaJ, &validFrom, &validTo, &ca, &ua, &arch, &del,
		&coActCount, &lastReinforcedAt, &contextNote,
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
		"created_at":    ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,

		"co_activation_count": coActCount,
		"last_reinforced_at":  lastReinforcedAt,
		"context_note":        contextNote,
	}
	return json.Marshal(m)
}
