package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type sessionRunRepo struct {
	data *Data
}

var (
	_ biz.SessionRunReader      = (*sessionRunRepo)(nil)
	_ biz.SessionRunWriter      = (*sessionRunRepo)(nil)
	_ biz.SessionRunDurableRepo = (*sessionRunRepo)(nil)
	_ biz.SessionRunRepo        = (*sessionRunRepo)(nil)
)

// NewSessionRunRepo implements biz.SessionRunRepo.
func NewSessionRunRepo(d *Data) biz.SessionRunRepo {
	return &sessionRunRepo{data: d}
}

func (r *sessionRunRepo) db() *sql.DB {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RawDB()
}

const sessionRunSelectSQL = `
SELECT id, session_id, turn_id, runtime_run_id, source, phase,
  soft_budget_sec, hard_budget_sec, checkpoint_id, workflow_job_id, agent_id,
  error_message, started_at, phase_changed_at, finished_at, resume_started_at, created_at, updated_at
FROM session_runs`

func scanSessionRunRow(scanner interface {
	Scan(dest ...any) error
}) (biz.SessionRun, error) {
	var run biz.SessionRun
	err := scanner.Scan(
		&run.ID, &run.SessionID, &run.TurnID, &run.RuntimeRunID, &run.Source, &run.Phase,
		&run.SoftBudgetSec, &run.HardBudgetSec, &run.CheckpointID, &run.WorkflowJobID, &run.AgentID,
		&run.ErrorMessage, &run.StartedAt, &run.PhaseChangedAt, &run.FinishedAt, &run.ResumeStartedAt,
		&run.CreatedAt, &run.UpdatedAt,
	)
	return run, err
}

func (r *sessionRunRepo) Create(ctx context.Context, run biz.SessionRun) (string, error) {
	db := r.db()
	if db == nil {
		return run.ID, nil
	}
	id := strings.TrimSpace(run.ID)
	if id == "" {
		return "", sql.ErrNoRows
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO session_runs (
  id, session_id, turn_id, runtime_run_id, source, phase,
  soft_budget_sec, hard_budget_sec, checkpoint_id, workflow_job_id, agent_id,
  error_message, started_at, phase_changed_at, finished_at, resume_started_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id,
		run.SessionID,
		run.TurnID,
		run.RuntimeRunID,
		run.Source,
		biz.NormalizeSessionRunPhase(run.Phase),
		run.SoftBudgetSec,
		run.HardBudgetSec,
		run.CheckpointID,
		run.WorkflowJobID,
		run.AgentID,
		run.ErrorMessage,
		run.StartedAt,
		run.PhaseChangedAt,
		run.FinishedAt,
		run.ResumeStartedAt,
		run.CreatedAt,
		run.UpdatedAt,
	)
	return id, err
}

func (r *sessionRunRepo) UpdatePhase(ctx context.Context, id, phase string) error {
	db := r.db()
	if db == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := db.ExecContext(ctx, `
UPDATE session_runs SET phase=?, phase_changed_at=?, updated_at=? WHERE id=?`,
		biz.NormalizeSessionRunPhase(phase), now, now, strings.TrimSpace(id),
	)
	return err
}

func (r *sessionRunRepo) MarkTerminal(ctx context.Context, id, phase, errMsg string) error {
	db := r.db()
	if db == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := db.ExecContext(ctx, `
UPDATE session_runs SET phase=?, error_message=?, finished_at=?, phase_changed_at=?, updated_at=?, resume_started_at='' WHERE id=?`,
		biz.NormalizeSessionRunPhase(phase), errMsg, now, now, now, strings.TrimSpace(id),
	)
	return err
}

func (r *sessionRunRepo) UpdateCheckpointID(ctx context.Context, id, checkpointID string) error {
	db := r.db()
	if db == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := db.ExecContext(ctx, `
UPDATE session_runs SET checkpoint_id=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(checkpointID), now, strings.TrimSpace(id),
	)
	return err
}

func (r *sessionRunRepo) TryClaimDurableResume(ctx context.Context, id, staleBefore string) (bool, error) {
	db := r.db()
	if db == nil {
		return false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	now := biz.ChannelTurnJobNow()
	res, err := db.ExecContext(ctx, `
UPDATE session_runs SET resume_started_at=?, updated_at=?
WHERE id=? AND phase='durable'
  AND checkpoint_id != '' AND checkpoint_id IS NOT NULL
  AND (finished_at IS NULL OR finished_at='')
  AND (resume_started_at IS NULL OR resume_started_at='' OR resume_started_at < ?)`,
		now, now, id, strings.TrimSpace(staleBefore),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *sessionRunRepo) ClearResumeClaim(ctx context.Context, id string) error {
	db := r.db()
	if db == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := db.ExecContext(ctx, `
UPDATE session_runs SET resume_started_at='', updated_at=? WHERE id=?`, now, id)
	return err
}

func (r *sessionRunRepo) ListByPhase(ctx context.Context, phase string, limit int) ([]biz.SessionRun, error) {
	db := r.db()
	if db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	phase = biz.NormalizeSessionRunPhase(phase)
	rows, err := db.QueryContext(ctx, sessionRunSelectSQL+`
WHERE phase=? AND (finished_at IS NULL OR finished_at='')
ORDER BY created_at ASC LIMIT ?`, phase, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionRunRows(rows)
}

func (r *sessionRunRepo) ListForJobs(ctx context.Context, q biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	db := r.db()
	if db == nil {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `
SELECT r.id, r.session_id, r.turn_id, r.runtime_run_id, r.source, r.phase,
  r.soft_budget_sec, r.hard_budget_sec, r.checkpoint_id, r.workflow_job_id, r.agent_id,
  r.error_message, r.started_at, r.phase_changed_at, r.finished_at, r.resume_started_at, r.created_at, r.updated_at
FROM session_runs r
LEFT JOIN sessions s ON s.id = r.session_id
WHERE 1=1`
	args := []any{}
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		query += ` AND r.session_id=?`
		args = append(args, sid)
	}
	if aid := strings.TrimSpace(q.AgentID); aid != "" {
		query += ` AND (r.agent_id=? OR s.agent_id=?)`
		args = append(args, aid, aid)
	}
	if st := strings.TrimSpace(q.Status); st != "" {
		query += ` AND r.phase=?`
		args = append(args, biz.NormalizeSessionRunPhase(st))
	} else {
		query += ` AND r.phase IN ('escalating','durable','interactive') AND (r.finished_at IS NULL OR r.finished_at='')`
	}
	query += ` ORDER BY r.created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionRunRows(rows)
}

func scanSessionRunRows(rows *sql.Rows) ([]biz.SessionRun, error) {
	var out []biz.SessionRun
	for rows.Next() {
		run, err := scanSessionRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *sessionRunRepo) GetActiveForSession(ctx context.Context, sessionID string) (biz.SessionRun, error) {
	db := r.db()
	if db == nil {
		return biz.SessionRun{}, sql.ErrNoRows
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return biz.SessionRun{}, sql.ErrNoRows
	}
	row := db.QueryRowContext(ctx, sessionRunSelectSQL+`
WHERE session_id=? AND phase IN ('interactive','escalating','durable')
  AND (finished_at IS NULL OR finished_at='')
ORDER BY created_at DESC LIMIT 1`, sessionID)
	return scanSessionRunRow(row)
}

func (r *sessionRunRepo) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]biz.SessionRun, int, error) {
	db := r.db()
	if db == nil {
		return nil, 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, 0, sql.ErrNoRows
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_runs WHERE session_id=?`, sessionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, sessionRunSelectSQL+`
WHERE session_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`, sessionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanSessionRunRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *sessionRunRepo) Get(ctx context.Context, id string) (biz.SessionRun, error) {
	db := r.db()
	if db == nil {
		return biz.SessionRun{}, sql.ErrNoRows
	}
	row := db.QueryRowContext(ctx, sessionRunSelectSQL+` WHERE id=? LIMIT 1`, strings.TrimSpace(id))
	return scanSessionRunRow(row)
}

// MarkOrphanedRunsCancelled marks all active session_runs with no finished_at as cancelled.
// Called on startup to clean up zombie runs left from a previous process crash/restart.
func (r *sessionRunRepo) MarkOrphanedRunsCancelled(ctx context.Context) (int, error) {
	db := r.db()
	if db == nil {
		return 0, nil
	}
	now := biz.ChannelTurnJobNow()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `
UPDATE session_runs SET phase='cancelled', error_message='orphaned: process restarted', finished_at=?, phase_changed_at=?, updated_at=?
WHERE phase IN ('interactive','escalating','durable') AND (finished_at IS NULL OR finished_at='')`,
		now, now, nowStr,
	)
	if err != nil {
		return 0, err
	}
	n, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		loggateway.Global().Warn("rows affected error", loggateway.StepID("session_run.repo"), loggateway.Err(rowsErr))
	}
	return int(n), nil
}
