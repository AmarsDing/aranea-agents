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

	"aranea-agents/internal/biz"
)

// Default confidence/importance values for auto-generated episodes.
const (
	defaultEpisodeConfidence = 0.75
	defaultL1ArchiveImportance = 0.6
)

// EpisodeInsert captures inputs for InsertEpisodeRow.
type EpisodeInsert struct {
	ID                 string
	SessionID          string
	AgentID            string
	UserID             string
	Title              string
	OutcomeSummary     string
	Importance         float64
	MessageCount       int
	ConsolidatedL3     int
	ConsolidationStatus string
	MetadataJSON       string
	SourceSessionID    string
	L1TaskID           string
	L1SnapshotJSON     string
	KeyDecisionsJSON   string
	KeyArtifactsJSON   string
}

// InsertEpisodeRow appends one L2 episodic memory row after auto-memory consolidation.
func (st *Store) InsertEpisodeRow(ctx context.Context, in EpisodeInsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	return st.insertEpisodeRowOn(ctx, st.client, in)
}

func (st *Store) insertEpisodeRowOn(ctx context.Context, db sqlRunner, in EpisodeInsert) ([]byte, error) {
	if db == nil {
		return nil, errors.New("db runner is required")
	}
	sid := strings.TrimSpace(in.SessionID)
	title := strings.TrimSpace(in.Title)
	if sid == "" || title == "" {
		return nil, errors.New("session_id and title are required")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	imp := in.Importance
	if imp <= 0 {
		imp = 0.5
	}
	summary := strings.TrimSpace(in.OutcomeSummary)
	l1TaskID := strings.TrimSpace(in.L1TaskID)
	l1Snapshot := strings.TrimSpace(in.L1SnapshotJSON)
	if l1Snapshot == "" {
		l1Snapshot = "{}"
	}
	keyDecisions := strings.TrimSpace(in.KeyDecisionsJSON)
	if keyDecisions == "" {
		keyDecisions = "[]"
	}
	keyArtifacts := strings.TrimSpace(in.KeyArtifactsJSON)
	if keyArtifacts == "" {
		keyArtifacts = "[]"
	}
	consStatus := strings.TrimSpace(in.ConsolidationStatus)
	if consStatus == "" {
		consStatus = "pending"
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO memory_episodes (
 id, session_id, run_id, team_id, agent_id, l1_task_id,
 episode_kind, title, goal, outcome, outcome_summary, result_preview,
 importance, confidence, message_count,
 consolidation_status, consolidated_l3_count,
 l1_snapshot_json, key_decisions_json, key_artifacts_json,
 metadata_json, started_at, ended_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, sid, "", "", strings.TrimSpace(in.AgentID), l1TaskID,
		"auto_memory", title, "", "completed", summary, summary,
		imp, defaultEpisodeConfidence, in.MessageCount,
		consStatus, in.ConsolidatedL3,
		l1Snapshot, keyDecisions, keyArtifacts,
		meta, now, now, now, now,
	)
	if err != nil {
		return nil, err
	}
	row := map[string]any{
		"id": id, "session_id": sid, "agent_id": in.AgentID,
		"title": title, "outcome_summary": summary,
		"consolidated_l3_count": in.ConsolidatedL3,
		"created_at": now,
	}
	return json.Marshal(row)
}

// ListPendingConsolidationEpisodes returns episodes with consolidation_status='pending', ordered by created_at.
func (st *Store) ListPendingConsolidationEpisodes(ctx context.Context, agentID string, limit int) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	lim := limit
	if lim <= 0 || lim > 50 {
		lim = 20
	}
	q := `SELECT id, session_id, agent_id, episode_kind, title, outcome_summary,
		importance, consolidation_status,
		consolidated_l3_count, consolidated_l4_count,
		l1_task_id, l1_snapshot_json, key_decisions_json, key_artifacts_json,
		created_at, updated_at
	FROM memory_episodes WHERE consolidation_status = 'pending'`
	args := []any{}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	q += ` ORDER BY created_at ASC LIMIT ?`
	args = append(args, lim)
	rows, err := st.client.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var (
			id, sessID, agID, kind, title, summary string
			importance                              float64
			consStatus                              string
			consL3, consL4                          int
			l1TaskID, l1Snap, keyDec, keyArt        string
			cat, uat                                string
		)
		if err := rows.Scan(&id, &sessID, &agID, &kind, &title, &summary,
			&importance, &consStatus,
			&consL3, &consL4,
			&l1TaskID, &l1Snap, &keyDec, &keyArt,
			&cat, &uat); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "session_id": sessID, "agent_id": agID,
			"episode_kind": kind, "title": title, "outcome_summary": summary,
			"importance": importance,
			"consolidation_status": consStatus,
			"consolidated_l3_count": consL3, "consolidated_l4_count": consL4,
			"l1_task_id": l1TaskID, "l1_snapshot_json": l1Snap,
			"key_decisions_json": keyDec, "key_artifacts_json": keyArt,
			"created_at": cat, "updated_at": uat,
		}
		b, _ := json.Marshal(m)
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkEpisodeConsolidated updates consolidation_status and counts.
func (st *Store) MarkEpisodeConsolidated(ctx context.Context, id string, l3Count, l4Count int) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_episodes SET consolidation_status = 'consolidated',
			consolidated_l3_count = ?, consolidated_l4_count = ?, updated_at = ?
		 WHERE id = ?`,
		l3Count, l4Count, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// InsertL1ArchiveEpisode creates an L2 episode from an archived L1 task.
func (st *Store) InsertL1ArchiveEpisode(ctx context.Context, in biz.L1ArchiveEpisodeInsert) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	sid := strings.TrimSpace(in.SessionID)
	if sid == "" {
		return errors.New("session_id is required")
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return errors.New("task_id is required")
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(in.TaskTitle)
	if title == "" {
		title = fmt.Sprintf("L1 task archived: %s", taskID)
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "completed"
	}
	l1Snapshot := strings.TrimSpace(in.L1SnapshotJSON)
	if l1Snapshot == "" {
		l1Snapshot = "{}"
	}
	_, err := st.client.ExecContext(ctx, `
INSERT INTO memory_episodes (
 id, session_id, run_id, team_id, agent_id, l1_task_id,
 episode_kind, title, goal, outcome, outcome_summary, result_preview,
 importance, confidence, message_count,
 consolidation_status, consolidated_l3_count,
 l1_snapshot_json, key_decisions_json, key_artifacts_json,
 metadata_json, started_at, ended_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, sid, "", "", strings.TrimSpace(in.AgentID), taskID,
		"l1_archive", title, "", status, title, "",
		defaultL1ArchiveImportance, defaultEpisodeConfidence, 0,
		"consolidated", 0,
		l1Snapshot, "[]", "[]",
		"{}", now, now, now, now,
	)
	return err
}

// ListEpisodeRowsForRecall returns recent consolidated episodes for L2 prompt injection.
// When sessionID is set, current-session episodes are preferred; agent-wide recall fills remaining slots.
func (st *Store) ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 3
	}
	if lim > 10 {
		lim = 10
	}
	seen := make(map[string]struct{}, lim)
	var out [][]byte
	appendRows := func(rows *sql.Rows) error {
		defer rows.Close()
		for rows.Next() {
			if len(out) >= lim {
				return nil
			}
			raw, err := scanEpisodeRowJSON(rows)
			if err != nil {
				return err
			}
			var row map[string]any
			if json.Unmarshal(raw, &row) != nil {
				continue
			}
			id, _ := row["id"].(string)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, raw)
		}
		return rows.Err()
	}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		rows, err := st.client.QueryContext(ctx, sqlEpisodeSelect+`
 WHERE agent_id = ? AND session_id = ? AND deleted_at = '' AND consolidation_status = 'consolidated'
 ORDER BY importance DESC, ended_at DESC LIMIT ?`, agentID, sid, lim)
		if err != nil {
			return nil, err
		}
		if err := appendRows(rows); err != nil {
			return nil, err
		}
	}
	if len(out) >= lim {
		return out, nil
	}
	remain := lim - len(out)
	rows, err := st.client.QueryContext(ctx, sqlEpisodeSelect+`
 WHERE agent_id = ? AND deleted_at = '' AND consolidation_status = 'consolidated'
 ORDER BY importance DESC, ended_at DESC LIMIT ?`, agentID, remain*2)
	if err != nil {
		return out, err
	}
	if err := appendRows(rows); err != nil {
		return out, err
	}
	return out, nil
}

const sqlEpisodeSelect = `SELECT id, session_id, agent_id, episode_kind, title, outcome_summary, importance,
 consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at FROM memory_episodes`

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
