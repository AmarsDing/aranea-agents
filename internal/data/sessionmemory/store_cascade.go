package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CascadeProposalInsert captures one L4 cascade review row.
type CascadeProposalInsert struct {
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

func (st *Store) InsertCascadeProposal(ctx context.Context, in CascadeProposalInsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	agentID := strings.TrimSpace(in.AgentID)
	triggerID := strings.TrimSpace(in.TriggerEntityID)
	if agentID == "" || triggerID == "" {
		return nil, errors.New("agent_id and trigger_entity_id are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	affected := strings.TrimSpace(in.AffectedJSON)
	if affected == "" {
		affected = "[]"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	risk := strings.TrimSpace(in.RiskLevel)
	if risk == "" {
		risk = "medium"
	}
	attr := strings.TrimSpace(in.TriggerAttribute)
	if attr == "" {
		attr = "name"
	}
	exp := strings.TrimSpace(in.ExpiresAt)
	if exp == "" {
		exp = time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
	}
	q := `INSERT INTO memory_cascade_proposals (
 id, agent_id, workspace_id, trigger_entity_id, trigger_entity_name, trigger_attribute,
 old_value, new_value, affected_json, status, risk_level, rationale, metadata_json,
 reviewed_by, reviewed_at, expires_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	_, err := st.client.ExecContext(ctx, q,
		id, agentID, strings.TrimSpace(in.WorkspaceID), triggerID,
		strings.TrimSpace(in.TriggerEntityName), attr,
		in.OldValue, in.NewValue, affected,
		"pending", risk, strings.TrimSpace(in.Rationale), meta,
		"", "", exp, now, now,
	)
	if err != nil {
		return nil, err
	}
	if err := st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
		Action:        "PROPOSE",
		TargetKind:    "cascade_proposal",
		TargetID:      id,
		Reason:        strings.TrimSpace(in.Rationale),
		PolicyVersion: "cascade_v1",
		MetadataJSON:  meta,
	}); err != nil {
		return nil, err
	}
	return st.GetCascadeProposalRow(ctx, id)
}

func (st *Store) GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("proposal id is required")
	}
	rows, err := st.client.QueryContext(ctx, cascadeProposalSelect+` WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanCascadeProposalJSON(rows)
}

func (st *Store) ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent_id is required")
	}
	w := []string{"agent_id = ?"}
	args := []any{agentID}
	if status != "" {
		w = append(w, "status = ?")
		args = append(args, status)
	}
	lim := int(limit)
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	args = append(args, lim)
	q := cascadeProposalSelect + ` WHERE ` + strings.Join(w, " AND ") + ` ORDER BY created_at DESC LIMIT ?`
	rows, err := st.client.QueryContext(ctx, q, args...)
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

func (st *Store) UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" || status == "" {
		return nil, errors.New("id and status are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if note := strings.TrimSpace(reviewNote); note != "" {
		raw, err := st.GetCascadeProposalRow(ctx, id)
		if err == nil {
			var row map[string]any
			if json.Unmarshal(raw, &row) == nil {
				meta := anyStr(row["metadata_json"])
				merged := mergeCascadeReviewNote(meta, status, note)
				_, _ = st.client.ExecContext(ctx,
					`UPDATE memory_cascade_proposals SET metadata_json = ? WHERE id = ?`,
					merged, id)
			}
		}
	}
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_cascade_proposals SET status = ?, reviewed_by = ?, reviewed_at = ?, updated_at = ? WHERE id = ?`,
		status, strings.TrimSpace(reviewedBy), now, now, id)
	if err != nil {
		return nil, err
	}
	if err := st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
		Action:        strings.ToUpper(status),
		TargetKind:    "cascade_proposal",
		TargetID:      id,
		Reason:        strings.TrimSpace(reviewNote),
		PolicyVersion: "cascade_v1",
	}); err != nil {
		return nil, err
	}
	return st.GetCascadeProposalRow(ctx, id)
}

func mergeCascadeReviewNote(metaJSON, status, note string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["review_status"] = status
	m["review_note"] = note
	b, _ := json.Marshal(m)
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

func (st *Store) GetEntityRow(ctx context.Context, id string) ([]byte, error) {
	return st.getEntityJSON(ctx, id)
}

const cascadeProposalSelect = `SELECT id, agent_id, workspace_id, trigger_entity_id, trigger_entity_name, trigger_attribute,
 old_value, new_value, affected_json, status, risk_level, rationale, metadata_json,
 reviewed_by, reviewed_at, expires_at, created_at, updated_at FROM memory_cascade_proposals`

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
