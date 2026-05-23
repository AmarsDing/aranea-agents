package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
)

type sessionRunCheckpointRepo struct {
	data *Data
}

// NewSessionRunCheckpointRepo implements biz.SessionRunCheckpointRepo.
func NewSessionRunCheckpointRepo(d *Data) biz.SessionRunCheckpointRepo {
	return &sessionRunCheckpointRepo{data: d}
}

func (r *sessionRunCheckpointRepo) db() *sql.DB {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RawDB()
}

func (r *sessionRunCheckpointRepo) Create(ctx context.Context, cp biz.SessionRunCheckpoint) (string, error) {
	db := r.db()
	if db == nil {
		return cp.ID, nil
	}
	id := strings.TrimSpace(cp.ID)
	_, err := db.ExecContext(ctx, `
INSERT INTO session_run_checkpoints (id, session_run_id, session_id, turn_id, agent_id, payload_json, created_at)
VALUES (?,?,?,?,?,?,?)`,
		id, cp.SessionRunID, cp.SessionID, cp.TurnID, cp.AgentID, cp.PayloadJSON, cp.CreatedAt,
	)
	return id, err
}

func (r *sessionRunCheckpointRepo) Get(ctx context.Context, id string) (biz.SessionRunCheckpoint, error) {
	db := r.db()
	if db == nil {
		return biz.SessionRunCheckpoint{}, sql.ErrNoRows
	}
	row := db.QueryRowContext(ctx, `
SELECT id, session_run_id, session_id, turn_id, agent_id, payload_json, created_at
FROM session_run_checkpoints WHERE id=? LIMIT 1`, strings.TrimSpace(id))
	return scanSessionRunCheckpoint(row)
}

func (r *sessionRunCheckpointRepo) GetBySessionRunID(ctx context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	db := r.db()
	if db == nil {
		return biz.SessionRunCheckpoint{}, sql.ErrNoRows
	}
	row := db.QueryRowContext(ctx, `
SELECT id, session_run_id, session_id, turn_id, agent_id, payload_json, created_at
FROM session_run_checkpoints WHERE session_run_id=? ORDER BY created_at DESC LIMIT 1`, strings.TrimSpace(sessionRunID))
	return scanSessionRunCheckpoint(row)
}

func scanSessionRunCheckpoint(row *sql.Row) (biz.SessionRunCheckpoint, error) {
	var cp biz.SessionRunCheckpoint
	err := row.Scan(&cp.ID, &cp.SessionRunID, &cp.SessionID, &cp.TurnID, &cp.AgentID, &cp.PayloadJSON, &cp.CreatedAt)
	return cp, err
}
