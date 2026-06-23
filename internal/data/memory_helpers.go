package data

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ──────────────────────────────────────────────────────────
// Types moved from sessionmemory package
// ──────────────────────────────────────────────────────────

// memoryFactUpsert captures inputs for upsertFactRow.
type memoryFactUpsert struct {
	ID                    string
	ScopeType             string
	ScopeID               string
	WorkspaceID           string
	UserID                string
	TeamID                string
	AgentID               string
	Statement             string
	Fingerprint           string
	DetailsMarkdown       string
	FactKind              string
	TagsJSON              string
	Confidence            float64
	Importance            float64
	UseCount              int32
	HitCount              int32
	PositiveFeedbackCount int32
	NegativeFeedbackCount int32
	ConflictCount         int32
	SourceKind            string
	SourceEpisodeID       string
	SourceSessionID       string
	SourceMessageID       string
	SourceExternal        string
	Version               int32
	Status                string
	PIIFlag               bool
	RedactedStatement     string
	PIIPolicy             string
	PIITypes              []string
	QualityScore          float64
	MetadataJSON          string
	CreatedAt             string
	UpdatedAt             string
}

// episodeInsert captures inputs for insertEpisodeRow.
type episodeInsert struct {
	ID                  string
	SessionID           string
	AgentID             string
	UserID              string
	Title               string
	OutcomeSummary      string
	Importance          float64
	MessageCount        int
	ConsolidatedL3      int
	ConsolidationStatus string
	MetadataJSON        string
	SourceSessionID     string
	L1TaskID            string
	L1SnapshotJSON      string
	KeyDecisionsJSON    string
	KeyArtifactsJSON    string
}

// cascadeProposalInsert captures one L4 cascade review row.
type cascadeProposalInsert struct {
	AgentID           string
	WorkspaceID       string
	TriggerEntityID   string
	TriggerEntityName string
	TriggerAttribute  string
	OldValue          string
	NewValue          string
	AffectedJSON      string
	RiskLevel         string
	Rationale         string
	MetadataJSON      string
	ExpiresAt         string
}

// cascadeSagaStep is one step in a cascade saga.
type cascadeSagaStep struct {
	ID          int64  `json:"id"`
	ProposalID  string `json:"proposal_id"`
	StepIndex   int    `json:"step_index"`
	StepName    string `json:"step_name"`
	State       string `json:"state"`
	IsCritical  bool   `json:"is_critical"`
	Attempts    int    `json:"attempts"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	ResultJSON  string `json:"result_json,omitempty"`
	Error       string `json:"error,omitempty"`
}

// evolutionEventInsert is service-layer input for insertEvolutionEventRow.
type evolutionEventInsert struct {
	AgentID       string
	WorkspaceID   string
	EventKind     string
	Kind          string
	TargetField   string
	Reason        string
	TriggerKind   string
	TriggerSource string
	MetadataJSON  string
}

// eventEntityParams is an L4 entity upsert payload.
type eventEntityParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	UserID           string
	EntityType       string
	Name             string
	NameNormalized   string
	Description      string
	Importance       float64
	Confidence       float64
	UseCount         int
	MetadataJSON     string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

// relationParams is an L4 graph edge upsert payload.
type relationParams struct {
	ID               string
	ScopeType        string
	ScopeID          string
	WorkspaceID      string
	SourceID         string
	TargetID         string
	RelationType     string
	Weight           float64
	Confidence       float64
	MetadataJSON     string
	ValidFromRFC3339 string
	ValidToRFC3339   string
	CreatedAtRFC3339 string
	UpdatedAtRFC3339 string
}

// memoryActionLogInsert records a policy-level memory mutation (Ledger audit trail).
type memoryActionLogInsert struct {
	Action         string
	TargetKind     string
	TargetID       string
	Reason         string
	PolicyVersion  string
	TurnID         string
	SourceEventIDs []string
	MetadataJSON   string
}

// entitySnapshot is a minimal row for L4 governance decisions.
type entitySnapshot struct {
	ID             string
	Name           string
	NameNormalized string
	Confidence     float64
	MetadataJSON   string
	UpdatedAt      string
}

// episodeEmbedCandidate is one episode row pending vector embedding.
type episodeEmbedCandidate struct {
	ID      string
	AgentID string
	Title   string
	Summary string
}

// consolidateWriteResult holds JSON rows written atomically by upsertFactsAndEpisodeBatch.
type consolidateWriteResult struct {
	FactRows     [][]byte
	EpisodeRow   []byte
	FactsWritten int
}

// ──────────────────────────────────────────────────────────
// SQL column constants
// ──────────────────────────────────────────────────────────

const sqlL0Select = `SELECT id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
 FROM memory_l0_assembly_snapshots`

const sqlL0Insert = `INSERT INTO memory_l0_assembly_snapshots (
 id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const sqlL1Task = `SELECT id, session_id, run_id, team_id, agent_id,
 task_key, task_title, task_goal, status,
 schema_version, budget_tokens, used_tokens,
 parent_task_id, shared_with_json,
 started_at, ended_at, archived_at,
 metadata_json, created_at, updated_at FROM memory_l1_tasks`

const sqlL1Field = `SELECT id, task_id, session_id, agent_id,
 field_path, field_kind, visibility, pin_to_prompt, is_required,
 value_text, value_json, value_ref, preview, token_estimate,
 source, source_ref, ttl_seconds, expires_at,
 revision, last_read_at, read_count,
 metadata_json, created_at, updated_at FROM memory_l1_fields`

const sqlFactSelect = `SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
 statement, statement_normalized, fingerprint, details_markdown,
 fact_kind, tags_json,
 confidence, importance, use_count, hit_count,
 positive_feedback_count, negative_feedback_count, conflict_count,
 source_kind, source_episode_id, source_session_id, source_message_id, source_external,
 version, status, superseded_by,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 pii_flag, redacted_statement,
 ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
 metadata_json, quality_score, pii_types, created_at, updated_at, archived_at, deleted_at,
 valid_from, valid_until, links, keywords, tags,
 decay_score
 FROM memory_facts`

const sqlEpisodeSelect = `SELECT id, session_id, agent_id, episode_kind, title, outcome_summary, importance,
 consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at FROM memory_episodes`

const cascadeProposalSelect = `SELECT id, agent_id, workspace_id, trigger_entity_id, trigger_entity_name, trigger_attribute,
 old_value, new_value, affected_json, status, risk_level, rationale, metadata_json,
 reviewed_by, reviewed_at, expires_at, created_at, updated_at FROM memory_cascade_proposals`

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

// ──────────────────────────────────────────────────────────
// Scan helpers
// ──────────────────────────────────────────────────────────

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
	)
	if err := rows.Scan(
		&id, &stype, &sid, &wid, &uid, &tid, &aid,
		&stmt, &snorm, &fp, &details,
		&fkind, &tags,
		&conf, &imp, &uc, &hc, &pfc, &nfc, &cc,
		&srcKind, &epID, &sessID, &msgID, &ext,
		&ver, &st, &sup,
		&embSt, &embModel, &embDim, &embBlob, &embNorm,
		&pii, &redacted,
		&ttlD, &decay, &nextD, &lastU, &exp,
		&meta, &qScore, &piiTypes, &ca, &ua, &arch, &del,
		&validFrom, &validUntil, &links, &keywords, &llmTags,
		&decayScore,
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
		"positive_feedback_count": pfc, "negative_feedback_count": nfc, "conflict_count": cc,
		"source_kind": srcKind, "source_episode_id": epID,
		"source_session_id": sessID, "source_message_id": msgID, "source_external": ext,
		"version": ver, "status": st, "superseded_by": sup,
		"embedding_status": embSt, "embedding_model": embModel, "embedding_dim": embDim,
		"embedding_norm":     embNorm,
		"pii_flag":           pii != 0,
		"redacted_statement": redacted,
		"ttl_days":           ttlD, "decay_factor": decay,
		"next_decay_at": nextD, "last_used_at": lastU, "expires_at": exp,
		"metadata_json": meta, "quality_score": qScore, "pii_types": piiTypes, "created_at": ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
		"valid_from": validFrom, "valid_until": validUntil,
		"links": links, "keywords": keywords, "tags": llmTags,
		"decay_score": decayScore,
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
		"created_at":    ca, "updated_at": ua,
		"archived_at": arch, "deleted_at": del,
	}
	return json.Marshal(m)
}

// ──────────────────────────────────────────────────────────
// General helpers
// ──────────────────────────────────────────────────────────

func memBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func decodeJSONStringSlice(raw string, lg loggateway.Logger) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONObject(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONFloatMap(raw string, lg loggateway.Logger) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return map[string]float64{}
	}
	var out map[string]float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return map[string]float64{}
	}
	return out
}

func nameReplacePattern(oldName string) (*regexp.Regexp, error) {
	return regexp.Compile(`(?i)\b` + regexp.QuoteMeta(oldName) + `\b`)
}

func replaceNameWordBoundary(text string, re *regexp.Regexp, newName string) string {
	if text == "" || re == nil {
		return text
	}
	return re.ReplaceAllString(text, newName)
}

func composeTurnRef(sessionID, messageID string) string {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	switch {
	case sessionID != "" && messageID != "":
		return sessionID + ":" + messageID
	case messageID != "":
		return messageID
	default:
		return sessionID
	}
}

func jsonStrMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mergeCascadeFactMeta(base, oldName, newName string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		m = map[string]any{}
	} else if m == nil {
		m = map[string]any{}
	}
	m["cascade_renamed_from"] = oldName
	m["cascade_renamed_to"] = newName
	m["source"] = "cascade_approve"
	b, mErr := json.Marshal(m)
	if mErr != nil {
		lg.Warn("session memory json marshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(mErr))
		return base
	}
	return string(b)
}

func mergeCascadeReviewNote(metaJSON, status, note string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		m = map[string]any{}
	} else if m == nil {
		m = map[string]any{}
	}
	m["review_status"] = status
	m["review_note"] = note
	b, mErr := json.Marshal(m)
	if mErr != nil {
		lg.Warn("session memory json marshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(mErr))
		return metaJSON
	}
	return string(b)
}

func anyStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func factIDFromRow(raw []byte, fallback string) string {
	if id := strings.TrimSpace(fallback); id != "" {
		return id
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err == nil {
		if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
			return id
		}
	}
	return fallback
}

// ──────────────────────────────────────────────────────────
// Embedding / vector helpers
// ──────────────────────────────────────────────────────────

func encodeFloat32Blob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeFloat32Blob(b []byte) []float32 {
	if len(b) < 4 {
		return nil
	}
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func vectorL2Norm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		x := float64(f)
		sum += x * x
	}
	return math.Sqrt(sum)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// ──────────────────────────────────────────────────────────
// Temporal helpers
// ──────────────────────────────────────────────────────────

func relationValidAt(validFrom, validTo, createdAt, queryTime string) bool {
	q, ok := parseMemoryTime(queryTime)
	if !ok {
		return true
	}
	fromText := strings.TrimSpace(validFrom)
	if fromText == "" {
		fromText = strings.TrimSpace(createdAt)
	}
	if fromText != "" {
		if from, ok := parseMemoryTime(fromText); ok && q.Before(from) {
			return false
		}
	}
	toText := strings.TrimSpace(validTo)
	if toText != "" {
		if to, ok := parseMemoryTime(toText); ok && q.After(to) {
			return false
		}
	}
	return true
}

func parseMemoryTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func defaultQueryTimeRFC3339(queryAt string) string {
	if strings.TrimSpace(queryAt) != "" {
		return strings.TrimSpace(queryAt)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// ──────────────────────────────────────────────────────────
// Recall scoring helpers
// ──────────────────────────────────────────────────────────

const (
	l2RecallCandidatePool = 40
	l2DecayHalfLifeDays   = 14.0
	l2ScoreWeightKeyword  = 0.35
	l2ScoreWeightVector   = 0.45
	l2ScoreWeightImport   = 0.15
	l2ScoreWeightSession  = 0.05

	l3RecallCandidatePool = 60
	l3DecayHalfLifeDays   = 30.0
	l3ScoreWeightKeyword  = 0.25
	l3ScoreWeightVector   = 0.30
	l3ScoreWeightImport   = 0.20
	l3ScoreWeightRecency  = 0.15
	l3ScoreWeightQuality  = 0.10

	ceRerankWeight = 0.15

	defaultEpisodeConfidence   = 0.75
	defaultL1ArchiveImportance = 0.6
	defaultFactQualityScore    = 0.5
)

type recallScoreBreakdown struct {
	Keyword      float64 `json:"keyword"`
	Vector       float64 `json:"vector"`
	Importance   float64 `json:"importance"`
	Recency      float64 `json:"recency"`
	QualityScore float64 `json:"quality_score"`
	CrossEncoder float64 `json:"cross_encoder"`
	Total        float64 `json:"total"`
}

// recallDebugRow is one scored recall candidate for admin debug RPC.
type recallDebugRow struct {
	Layer     string               `json:"layer"`
	ID        string               `json:"id"`
	Title     string               `json:"title,omitempty"`
	Summary   string               `json:"summary,omitempty"`
	Statement string               `json:"statement,omitempty"`
	Scores    recallScoreBreakdown `json:"scores"`
	Raw       json.RawMessage      `json:"raw,omitempty"`
}

type scoredEpisode struct {
	raw       []byte
	id        string
	title     string
	summary   string
	score     float64
	breakdown recallScoreBreakdown
}

type scoredFact struct {
	raw       []byte
	id        string
	stmt      string
	details   string
	score     float64
	breakdown recallScoreBreakdown
}

func tokenizeQuery(q string) []string {
	if q == "" {
		return nil
	}
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) >= 2 {
			out = append(out, p)
		}
	}
	return out
}

func keywordOverlapScore(tokens []string, text string) float64 {
	if len(tokens) == 0 || text == "" {
		return 0
	}
	lowerText := strings.ToLower(text)
	hits := 0
	for _, tok := range tokens {
		if strings.Contains(lowerText, strings.ToLower(tok)) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

func decayFactor(endedAt string, now time.Time) float64 {
	endedAt = strings.TrimSpace(endedAt)
	if endedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, endedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, endedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l2DecayHalfLifeDays)
}

func factRecencyDecay(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l3DecayHalfLifeDays)
}

func recencyBoost(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 0
	}
	days := now.Sub(t).Hours() / 24
	if days <= 7 {
		return 1.0
	}
	if days <= 30 {
		return 0.5
	}
	return 0.1
}

func episodePassage(raw []byte) string {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil {
		return ""
	}
	title, _ := row["title"].(string)
	summary, _ := row["outcome_summary"].(string)
	return strings.TrimSpace(title + " " + summary)
}

func factPassage(stmt, details string) string {
	return strings.TrimSpace(stmt + " " + details)
}

func applyCrossEncoderRerankToScored(reranker biz.Reranker, query string, scored []scoredEpisode, passages []string, apply func(i int, ceScore, total float64)) {
	if reranker == nil || strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	for i := range scored {
		ceScore := reranker.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		apply(i, ceScore, total)
	}
}

func applyCrossEncoderRerankToFactScored(reranker biz.Reranker, query string, scored []scoredFact, passages []string) {
	if reranker == nil || strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	for i := range scored {
		ceScore := reranker.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		scored[i].breakdown.CrossEncoder = ceScore
		scored[i].breakdown.Total = total
		scored[i].score = total
	}
}

// VectorSearcher abstracts vector similarity search for recall operations.
type VectorSearcher interface {
	Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorSearchHit, error)
}

// VectorSearchHit represents a single search result from a VectorSearcher.
type VectorSearchHit struct {
	ID    string
	Score float64
}

// ──────────────────────────────────────────────────────────
// Schema / DDL helpers (moved from sessionmemory/schema.go)
// ──────────────────────────────────────────────────────────

func sessionMemoryEnsurePatches(ctx context.Context, client execer, d Dialect) error {
	if client == nil {
		return nil
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{"tool_invocation_params", "param_name", "ALTER TABLE tool_invocation_params ADD COLUMN param_name TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "param_type", "ALTER TABLE tool_invocation_params ADD COLUMN param_type TEXT NOT NULL DEFAULT 'string'"},
		{"tool_invocation_params", "value_preview", "ALTER TABLE tool_invocation_params ADD COLUMN value_preview TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "value_hash", "ALTER TABLE tool_invocation_params ADD COLUMN value_hash TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "value_size_bytes", "ALTER TABLE tool_invocation_params ADD COLUMN value_size_bytes INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "is_required", "ALTER TABLE tool_invocation_params ADD COLUMN is_required INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "is_sensitive", "ALTER TABLE tool_invocation_params ADD COLUMN is_sensitive INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "redaction_reason", "ALTER TABLE tool_invocation_params ADD COLUMN redaction_reason TEXT NOT NULL DEFAULT ''"},
	}
	for _, p := range patches {
		has, err := memColumnExists(ctx, client, d, p.table, p.col)
		if err != nil {
			return fmt.Errorf("sessionmemory patch check %s.%s: %w", p.table, p.col, err)
		}
		if has {
			continue
		}
		if _, err := client.ExecContext(ctx, p.ddl); err != nil {
			return fmt.Errorf("sessionmemory patch %s.%s: %w", p.table, p.col, err)
		}
	}
	return nil
}

func sessionMemoryEnsureMonitorSchemaPatches(ctx context.Context, client execer, d Dialect) error {
	if client == nil {
		return nil
	}
	// audit_logs is a raw-SQL table (no Ent schema). Ensure it exists before patching columns.
	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		action TEXT NOT NULL DEFAULT '',
		resource TEXT NOT NULL DEFAULT '',
		resource_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create audit_logs: %w", err)
	}
	// monitor_events is a raw-SQL table (no Ent schema). Ensure it exists before ALTER TABLE patches.
	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_events (
		id TEXT PRIMARY KEY,
		event_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create monitor_events: %w", err)
	}
	// monitor_traces is a raw-SQL table (no Ent schema). Ensure it exists before ALTER TABLE patches.
	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_traces (
		id TEXT PRIMARY KEY,
		trace_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create monitor_traces: %w", err)
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{"audit_logs", "actor", "ALTER TABLE audit_logs ADD COLUMN actor TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "ip", "ALTER TABLE audit_logs ADD COLUMN ip TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "user_agent", "ALTER TABLE audit_logs ADD COLUMN user_agent TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "severity", "ALTER TABLE audit_logs ADD COLUMN severity TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "metadata_json", "ALTER TABLE audit_logs ADD COLUMN metadata_json TEXT NOT NULL DEFAULT ''"},
	}
	for _, p := range patches {
		has, err := memColumnExists(ctx, client, d, p.table, p.col)
		if err != nil {
			return fmt.Errorf("monitor patch check %s.%s: %w", p.table, p.col, err)
		}
		if has {
			continue
		}
		if _, err := client.ExecContext(ctx, p.ddl); err != nil {
			return fmt.Errorf("monitor patch %s.%s: %w", p.table, p.col, err)
		}
	}
	// Post-migration verification: ensure raw-SQL tables actually exist before
	// the caller records this migration as applied. Without this check, a
	// silent DDL failure could leave the tables missing while the migration is
	// marked done, making it impossible to recover without manual intervention.
	for _, tbl := range []string{"monitor_events", "monitor_traces"} {
		exists, err := memTableExists(ctx, client, d, tbl)
		if err != nil {
			return fmt.Errorf("monitor patch verify %s: %w", tbl, err)
		}
		if !exists {
			return fmt.Errorf("monitor patch verify: table %s not found after CREATE TABLE IF NOT EXISTS", tbl)
		}
	}
	return nil
}

// memColumnExists is the dialect-aware variant of memSqliteColumnExists.
// SQLite: pragma_table_info(table) WHERE name = ?
// Postgres: information_schema.columns WHERE table_name = $1 AND column_name = $2
func memColumnExists(ctx context.Context, client execer, d Dialect, table, column string) (bool, error) {
	var query string
	var args []any
	if d.IsPostgres() {
		query = `SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2 LIMIT 1`
		args = []any{table, column}
	} else {
		query = `SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1`
		args = []any{table, column}
	}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

// memTableExists is the dialect-aware variant of memSqliteTableExists.
// SQLite: sqlite_master WHERE type='table' AND name = ?
// Postgres: information_schema.tables WHERE table_schema='public' AND table_name = $1
func memTableExists(ctx context.Context, client execer, d Dialect, table string) (bool, error) {
	var query string
	var args []any
	if d.IsPostgres() {
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1 LIMIT 1`
		args = []any{table}
	} else {
		query = `SELECT 1 FROM sqlite_master WHERE type='table' AND name = ? LIMIT 1`
		args = []any{table}
	}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}
