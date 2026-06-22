package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/sessionrun"
	"aranea-agents/pkg/apierror"
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

func (r *sessionRunRepo) readClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Read(ctx)
}

func (r *sessionRunRepo) writeClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Write(ctx)
}

func (r *sessionRunRepo) writeDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().WriteDB(ctx)
}

// entSessionRunToBiz converts an Ent SessionRun entity to a biz SessionRun.
func entSessionRunToBiz(e *ent.SessionRun) biz.SessionRun {
	if e == nil {
		return biz.SessionRun{}
	}
	return biz.SessionRun{
		ID:              e.ID,
		SessionID:       e.SessionID,
		TurnID:          e.TurnID,
		RuntimeRunID:    e.RuntimeRunID,
		Source:          e.Source,
		Phase:           e.Phase,
		SoftBudgetSec:   e.SoftBudgetSec,
		HardBudgetSec:   e.HardBudgetSec,
		CheckpointID:    e.CheckpointID,
		WorkflowJobID:   e.WorkflowJobID,
		AgentID:         e.AgentID,
		ErrorMessage:    e.ErrorMessage,
		StartedAt:       e.StartedAt,
		PhaseChangedAt:  e.PhaseChangedAt,
		FinishedAt:      e.FinishedAt,
		ResumeStartedAt: e.ResumeStartedAt,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

func (r *sessionRunRepo) Create(ctx context.Context, run biz.SessionRun) (string, error) {
	client := r.writeClient(ctx)
	if client == nil {
		return run.ID, nil
	}
	id := strings.TrimSpace(run.ID)
	if id == "" {
		return "", apierror.NotFound(apierror.DomainSession, "not found")
	}
	_, err := client.SessionRun.Create().
		SetID(id).
		SetSessionID(run.SessionID).
		SetTurnID(run.TurnID).
		SetRuntimeRunID(run.RuntimeRunID).
		SetSource(run.Source).
		SetPhase(biz.NormalizeSessionRunPhase(run.Phase)).
		SetSoftBudgetSec(run.SoftBudgetSec).
		SetHardBudgetSec(run.HardBudgetSec).
		SetCheckpointID(run.CheckpointID).
		SetWorkflowJobID(run.WorkflowJobID).
		SetAgentID(run.AgentID).
		SetErrorMessage(run.ErrorMessage).
		SetStartedAt(run.StartedAt).
		SetPhaseChangedAt(run.PhaseChangedAt).
		SetFinishedAt(run.FinishedAt).
		SetResumeStartedAt(run.ResumeStartedAt).
		SetCreatedAt(run.CreatedAt).
		SetUpdatedAt(run.UpdatedAt).
		Save(ctx)
	return id, entErrToBizErr(err, "SESSION_RUN")
}

func (r *sessionRunRepo) UpdatePhase(ctx context.Context, id, phase string) error {
	client := r.writeClient(ctx)
	if client == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := client.SessionRun.UpdateOneID(strings.TrimSpace(id)).
		SetPhase(biz.NormalizeSessionRunPhase(phase)).
		SetPhaseChangedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("update phase failed", loggateway.StepID("data.session_run.update_phase"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION_RUN")
}

func (r *sessionRunRepo) MarkTerminal(ctx context.Context, id, phase, errMsg string) error {
	client := r.writeClient(ctx)
	if client == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := client.SessionRun.UpdateOneID(strings.TrimSpace(id)).
		SetPhase(biz.NormalizeSessionRunPhase(phase)).
		SetErrorMessage(errMsg).
		SetFinishedAt(now).
		SetPhaseChangedAt(now).
		SetUpdatedAt(now).
		SetResumeStartedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("mark terminal failed", loggateway.StepID("data.session_run.mark_terminal"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION_RUN")
}

// MarkTerminalWherePhase performs a CAS terminal transition for SessionRun.
// It only updates the run if its current phase matches fromPhase, preventing
// TOCTOU races where a concurrent writer changes the phase between Get and
// MarkTerminal. Returns true if the row was updated.
func (r *sessionRunRepo) MarkTerminalWherePhase(ctx context.Context, id, fromPhase, toPhase, errMsg string) (bool, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	now := biz.ChannelTurnJobNow()
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE session_runs
SET phase=?, error_message=?, finished_at=?, phase_changed_at=?, updated_at=?, resume_started_at=''
WHERE id=? AND phase=?`),
		biz.NormalizeSessionRunPhase(toPhase), errMsg, now, now, now, id, biz.NormalizeSessionRunPhase(fromPhase),
	)
	if err != nil {
		r.data.lg.Warn("mark terminal where phase failed", loggateway.StepID("data.session_run.mark_terminal_where_phase"), loggateway.Err(err))
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	return n > 0, nil
}

func (r *sessionRunRepo) UpdateCheckpointID(ctx context.Context, id, checkpointID string) error {
	client := r.writeClient(ctx)
	if client == nil {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := client.SessionRun.UpdateOneID(strings.TrimSpace(id)).
		SetCheckpointID(strings.TrimSpace(checkpointID)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("update checkpoint id failed", loggateway.StepID("data.session_run.update_checkpoint"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION_RUN")
}

// TryClaimDurableResume uses Raw SQL because it relies on conditional WHERE with
// multi-column checks that are not expressible via Ent's predicate system.
func (r *sessionRunRepo) TryClaimDurableResume(ctx context.Context, id, staleBefore string) (bool, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	now := biz.ChannelTurnJobNow()
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE session_runs SET resume_started_at=?, updated_at=?
WHERE id=? AND phase='durable'
  AND checkpoint_id != '' AND checkpoint_id IS NOT NULL
  AND (finished_at IS NULL OR finished_at='')
  AND (resume_started_at IS NULL OR resume_started_at='' OR resume_started_at < ?)`),
		now, now, id, strings.TrimSpace(staleBefore),
	)
	if err != nil {
		r.data.lg.Error("claim durable resume failed", loggateway.StepID("data.session_run.claim_durable"), loggateway.Err(err))
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	return n > 0, nil
}

func (r *sessionRunRepo) ClearResumeClaim(ctx context.Context, id string) error {
	client := r.writeClient(ctx)
	if client == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := client.SessionRun.UpdateOneID(id).
		SetResumeStartedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		r.data.lg.Warn("clear resume claim failed", loggateway.StepID("data.session_run.clear_resume"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "SESSION_RUN")
}

func (r *sessionRunRepo) ListByPhase(ctx context.Context, phase string, limit int) ([]biz.SessionRun, error) {
	client := r.readClient(ctx)
	if client == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	phase = biz.NormalizeSessionRunPhase(phase)
	items, err := client.SessionRun.Query().
		Where(
			sessionrun.PhaseEQ(phase),
			sessionrun.FinishedAtEQ(""),
		).
		Order(ent.Asc(sessionrun.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_RUN")
	}
	out := make([]biz.SessionRun, len(items))
	for i, item := range items {
		out[i] = entSessionRunToBiz(item)
	}
	return out, nil
}

// ListForJobs uses Raw SQL because it JOINs sessions table for agent_id lookup,
// which is not expressible via Ent's predicate system.
func (r *sessionRunRepo) ListForJobs(ctx context.Context, q biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	db := r.data.RWDB().ReadDB(ctx)
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
		query += ` AND r.phase IN ('durable','interactive') AND (r.finished_at IS NULL OR r.finished_at='')`
	}
	query += ` ORDER BY r.created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(query), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_RUN")
	}
	defer rows.Close()
	return scanSessionRunRows(rows)
}

func scanSessionRunRows(rows *sql.Rows) ([]biz.SessionRun, error) {
	var out []biz.SessionRun
	for rows.Next() {
		run, err := scanSessionRunRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "SESSION_RUN")
		}
		out = append(out, run)
	}
	return out, entErrToBizErr(rows.Err(), "SESSION_RUN")
}

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
	return run, entErrToBizErr(err, "SESSION_RUN")
}

func (r *sessionRunRepo) GetActiveForSession(ctx context.Context, sessionID string) (biz.SessionRun, error) {
	client := r.readClient(ctx)
	if client == nil {
		return biz.SessionRun{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return biz.SessionRun{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	item, err := client.SessionRun.Query().
		Where(
			sessionrun.SessionIDEQ(sessionID),
			sessionrun.PhaseIn(biz.SessionRunPhaseInteractive, biz.SessionRunPhaseDurable),
			sessionrun.FinishedAtEQ(""),
		).
		Order(ent.Desc(sessionrun.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SessionRun{}, apierror.NotFound(apierror.DomainSession, "not found")
		}
		return biz.SessionRun{}, entErrToBizErr(err, "SESSION_RUN")
	}
	return entSessionRunToBiz(item), nil
}

func (r *sessionRunRepo) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]biz.SessionRun, int, error) {
	client := r.readClient(ctx)
	if client == nil {
		return nil, 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, 0, apierror.NotFound(apierror.DomainSession, "not found")
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
	total, err := client.SessionRun.Query().
		Where(sessionrun.SessionIDEQ(sessionID)).
		Count(ctx)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "SESSION_RUN")
	}
	items, err := client.SessionRun.Query().
		Where(sessionrun.SessionIDEQ(sessionID)).
		Order(ent.Desc(sessionrun.FieldCreatedAt)).
		Offset(offset).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "SESSION_RUN")
	}
	out := make([]biz.SessionRun, len(items))
	for i, item := range items {
		out[i] = entSessionRunToBiz(item)
	}
	return out, total, nil
}

func (r *sessionRunRepo) Get(ctx context.Context, id string) (biz.SessionRun, error) {
	client := r.readClient(ctx)
	if client == nil {
		return biz.SessionRun{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	item, err := client.SessionRun.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SessionRun{}, apierror.NotFound(apierror.DomainSession, "not found")
		}
		return biz.SessionRun{}, entErrToBizErr(err, "SESSION_RUN")
	}
	return entSessionRunToBiz(item), nil
}

// TransitionPhase performs a CAS (Compare-And-Swap) phase transition.
// It only updates the row if the current phase matches fromPhase, preventing
// TOCTOU races where a concurrent writer changes the phase between a Get and
// an UpdatePhase call (N-04 fix).
// Returns true if the transition succeeded (row was updated).
func (r *sessionRunRepo) TransitionPhase(ctx context.Context, id, fromPhase, toPhase string) (bool, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return false, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	now := biz.ChannelTurnJobNow()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE session_runs SET phase=?, phase_changed_at=?, updated_at=?
WHERE id=? AND phase=?`),
		biz.NormalizeSessionRunPhase(toPhase), now, nowStr, id, biz.NormalizeSessionRunPhase(fromPhase),
	)
	if err != nil {
		r.data.lg.Warn("transition phase CAS failed", loggateway.StepID("data.session_run.transition_phase"), loggateway.Err(err))
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "SESSION_RUN")
	}
	return n > 0, nil
}

// MarkOrphanedRunsCancelled uses Raw SQL because it does a bulk conditional UPDATE
// with a WHERE clause that is not easily expressible via Ent's Update API.
//
// B01 fix: durable runs with valid checkpoints are preserved (they will be
// resumed by SessionRunDurableWorker). Only durable runs without a checkpoint
// (anomalous data) are cleaned up alongside interactive orphans.
func (r *sessionRunRepo) MarkOrphanedRunsCancelled(ctx context.Context) (int, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return 0, nil
	}
	now := biz.ChannelTurnJobNow()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE session_runs SET phase=?, error_message='orphaned: process restarted', finished_at=?, phase_changed_at=?, updated_at=?
WHERE (
    phase IN ('interactive')
    OR (phase='durable' AND (checkpoint_id IS NULL OR checkpoint_id=''))
  )
  AND (finished_at IS NULL OR finished_at='')`),
		biz.SessionRunPhaseCancelled, now, now, nowStr,
	)
	if err != nil {
		r.data.lg.Error("mark orphaned runs cancelled failed", loggateway.StepID("data.session_run.orphan_cleanup"), loggateway.Err(err))
		return 0, entErrToBizErr(err, "SESSION_RUN")
	}
	n, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		r.data.lg.Warn("rows affected error", loggateway.StepID("session_run.repo"), loggateway.Err(rowsErr))
	}
	return int(n), nil
}
