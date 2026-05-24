package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EpisodeInsert captures inputs for InsertEpisodeRow.
type EpisodeInsert struct {
	ID              string
	SessionID       string
	AgentID         string
	UserID          string
	Title           string
	OutcomeSummary  string
	Importance      float64
	MessageCount    int
	ConsolidatedL3  int
	MetadataJSON    string
	SourceSessionID string
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
	_, err := db.ExecContext(ctx, `
INSERT INTO memory_episodes (
 id, session_id, run_id, team_id, agent_id, l1_task_id,
 episode_kind, title, goal, outcome, outcome_summary, result_preview,
 importance, confidence, message_count,
 consolidation_status, consolidated_l3_count,
 metadata_json, started_at, ended_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, sid, "", "", strings.TrimSpace(in.AgentID), "",
		"auto_memory", title, "", "completed", summary, summary,
		imp, 0.75, in.MessageCount,
		"consolidated", in.ConsolidatedL3,
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
