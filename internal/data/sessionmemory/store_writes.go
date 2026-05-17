package sessionmemory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func factFingerprint(statement, scopeType, scopeID string) string {
	n := strings.ToLower(strings.TrimSpace(statement))
	sum := sha256.Sum256([]byte(n + "\x00" + strings.TrimSpace(scopeType) + "\x00" + strings.TrimSpace(scopeID)))
	return hex.EncodeToString(sum[:])
}

// UpsertFactRow inserts or upserts memory_facts keyed by UNIQUE(scope_type,scope_id,fingerprint).
func (st *Store) UpsertFactRow(ctx context.Context, in MemoryFactUpsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	stype := strings.TrimSpace(in.ScopeType)
	stmt := strings.TrimSpace(in.Statement)
	if stype == "" || stmt == "" {
		return nil, errors.New("scope_type and statement are required")
	}
	fp := strings.TrimSpace(in.Fingerprint)
	if fp == "" {
		fp = factFingerprint(stmt, stype, in.ScopeID)
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	snorm := strings.ToLower(strings.TrimSpace(stmt))

	workspaceID := in.WorkspaceID
	userID := in.UserID
	teamID := in.TeamID
	agentID := in.AgentID
	details := in.DetailsMarkdown
	fkind := strings.TrimSpace(in.FactKind)
	if fkind == "" {
		fkind = "fact"
	}
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	conf := in.Confidence
	if conf == 0 {
		conf = 0.7
	}
	imp := in.Importance
	if imp == 0 {
		imp = 0.5
	}
	sk := strings.TrimSpace(in.SourceKind)
	if sk == "" {
		sk = "manual"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	stTxt := strings.TrimSpace(in.Status)
	if stTxt == "" {
		stTxt = "active"
	}
	v := int(in.Version)
	if v <= 0 {
		v = 1
	}
	pii := 0
	if in.PIIFlag {
		pii = 1
	}

	useC := int(in.UseCount)
	hitC := int(in.HitCount)
	pfc := int(in.PositiveFeedbackCount)
	nfc := int(in.NegativeFeedbackCount)
	cc := int(in.ConflictCount)

	epID := in.SourceEpisodeID
	sessID := in.SourceSessionID
	msgID := in.SourceMessageID
	ext := in.SourceExternal

	createdAt := strings.TrimSpace(in.CreatedAt)
	updatedAt := strings.TrimSpace(in.UpdatedAt)
	if createdAt == "" {
		createdAt = now
	}
	if updatedAt == "" {
		updatedAt = now
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", 45), ",")
	insertSQL := strings.TrimSpace(fmt.Sprintf(`
INSERT INTO memory_facts (
 id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
 statement, statement_normalized, fingerprint, details_markdown,
 fact_kind, tags_json,
 confidence, importance,
 use_count, hit_count, positive_feedback_count, negative_feedback_count, conflict_count,
 source_kind, source_episode_id, source_session_id, source_message_id, source_external,
 version, status, superseded_by,
 embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
 pii_flag, redacted_statement,
 ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
 metadata_json, created_at, updated_at, archived_at, deleted_at
) VALUES (%s)
ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
 statement = excluded.statement,
 statement_normalized = excluded.statement_normalized,
 details_markdown = excluded.details_markdown,
 fact_kind = excluded.fact_kind,
 tags_json = excluded.tags_json,
 confidence = excluded.confidence,
 importance = excluded.importance,
 workspace_id = excluded.workspace_id,
 user_id = excluded.user_id,
 team_id = excluded.team_id,
 agent_id = excluded.agent_id,
 source_kind = excluded.source_kind,
 source_episode_id = excluded.source_episode_id,
 source_session_id = excluded.source_session_id,
 source_message_id = excluded.source_message_id,
 status = excluded.status,
 metadata_json = excluded.metadata_json,
 updated_at = excluded.updated_at,
 pii_flag = excluded.pii_flag,
 version = memory_facts.version + 1,
 use_count = memory_facts.use_count,
 hit_count = memory_facts.hit_count,
 positive_feedback_count = memory_facts.positive_feedback_count,
 negative_feedback_count = memory_facts.negative_feedback_count,
 conflict_count = memory_facts.conflict_count`, placeholders))

	args := []any{
		id, stype, in.ScopeID, workspaceID, userID, teamID, agentID,
		stmt, snorm, fp, details,
		fkind, tags,
		conf, imp,
		useC, hitC, pfc, nfc, cc,
		sk, epID, sessID, msgID, ext,
		v, stTxt, "",
		"pending", "", 0, ([]byte)(nil), 0.0,
		pii, "",
		0, 0.98, "", "", "",
		meta, createdAt, updatedAt, "", "",
	}

	if _, err := st.client.ExecContext(ctx, insertSQL, args...); err != nil {
		return nil, err
	}

	rows, err := st.client.QueryContext(ctx, sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ? AND deleted_at = '' LIMIT 1`, stype, in.ScopeID, fp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanFactRowJSON(rows)
}

// MemoryFactUpsert captures inputs for UpsertFactRow (service maps from protobuf).
type MemoryFactUpsert struct {
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
	MetadataJSON          string
	CreatedAt             string
	UpdatedAt             string
}

// InsertEvolutionEventRow appends agent_evolution_events.
func (st *Store) InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	aid := strings.TrimSpace(in.AgentID)
	if aid == "" {
		return nil, errors.New("agent_id is required")
	}
	eventKind := strings.TrimSpace(in.EventKind)
	if eventKind == "" {
		eventKind = strings.TrimSpace(in.Kind)
	}
	if eventKind == "" {
		return nil, errors.New("event_kind is required")
	}
	trigKind := strings.TrimSpace(in.TriggerKind)
	if trigKind == "" {
		return nil, errors.New("trigger_kind is required")
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	tgt := strings.TrimSpace(in.TargetField)
	reason := strings.TrimSpace(in.Reason)
	trSrc := strings.TrimSpace(in.TriggerSource)
	wsid := strings.TrimSpace(in.WorkspaceID)

	evq := `INSERT INTO agent_evolution_events (
 id, agent_id, workspace_id, event_kind, target_field,
 before_json, after_json, diff_json,
 trigger_kind, trigger_source, evidence_json, reason,
 applied, reverted, reverted_by_event_id,
 metadata_json, created_at, applied_at, reverted_at
 ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

	if _, err := st.client.ExecContext(ctx, evq,
		id, aid, wsid, eventKind, tgt,
		"{}", "{}", "{}",
		trigKind, trSrc, "[]", reason,
		1, 0, "",
		meta, now, "", "",
	); err != nil {
		return nil, err
	}

	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = eventKind
	}
	row := map[string]any{
		"id":           id,
		"agent_id":     aid,
		"workspace_id": wsid,
		"event_kind":   eventKind, "kind": kind,
		"target_field": tgt,
		"reason":       reason,
		"applied":      true, "reverted": false,
		"created_at": now,
	}
	return json.Marshal(row)
}

// EvolutionEventInsert is service-layer input for InsertEvolutionEventRow.
type EvolutionEventInsert struct {
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
