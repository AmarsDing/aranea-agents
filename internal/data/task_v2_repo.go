package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/taskv2"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// taskV2Repo implements biz.TaskV2Repo.
// Stability:evolving
type taskV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TaskV2Repo = (*taskV2Repo)(nil)

// NewTaskV2Repo creates a new TaskV2Repo.
// Logger is preset with domain "TASK_V2" per loggateway convention.
func NewTaskV2Repo(d *Data, lg loggateway.Logger) biz.TaskV2Repo {
	return &taskV2Repo{data: d, lg: lg.With(loggateway.Domain("TASK_V2"))}
}

// GetTask returns the Task by ID.
func (r *taskV2Repo) GetTask(ctx context.Context, id string) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	preds := []predicate.TaskV2{taskv2.IDEQ(id)}
	if wsPred := taskV2WorkspacePredicate(ctx); wsPred != nil {
		preds = append(preds, wsPred)
	}
	row, err := r.data.RW().Read(ctx).TaskV2.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// ListTasksBySession returns all tasks for the given session, ordered by
// created_at asc (id asc as tiebreaker for same-second creations).
// 2026-07-26 修复：排序键从 seq 改为 created_at。seq 赋值源头不可靠（澄清门
// 硬编码 Seq=1、正常 turn 路径默认 0），多任务会话下按 seq 排序使
// resolveLatestUserTaskID 取"最后一个"时选错父任务，导致总结 turn 挂到旧
// 任务上（执行中不显示、刷新后位置不对）。created_at 是"最近用户任务"的
// 语义真相，前端 compareByTimeThenSeq 也以 CreatedAt 为主排序键。
func (r *taskV2Repo) ListTasksBySession(ctx context.Context, sessionID string) ([]biz.Task, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("task v2 repo: database not configured")
	}
	preds := []predicate.TaskV2{taskv2.SessionIDEQ(sessionID)}
	if wsPred := taskV2WorkspacePredicate(ctx); wsPred != nil {
		preds = append(preds, wsPred)
	}
	rows, err := r.data.RW().Read(ctx).TaskV2.Query().
		Where(preds...).
		Order(ent.Asc(taskv2.FieldCreatedAt), ent.Asc(taskv2.FieldID)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_V2")
	}
	return entTasksV2ToBiz(rows), nil
}

// MaxSeqBySession returns the highest Seq stored for the session (0 if none).
// H2 (2026-10-18): task.seq shares the per-session shared counter space with
// steps; used together with StepV2Reader.MaxSeqBySpiritSession to seed the
// SeqAssigner after process restart so neither entity nor outbox delivery seqs
// are re-issued (the outbox has UNIQUE(session_id, seq)).
func (r *taskV2Repo) MaxSeqBySession(ctx context.Context, sessionID string) (int64, error) {
	if r == nil || r.data == nil {
		return 0, fmt.Errorf("task v2 repo: database not configured")
	}
	if sessionID == "" {
		return 0, nil
	}
	preds := []predicate.TaskV2{taskv2.SessionIDEQ(sessionID)}
	if wsPred := taskV2WorkspacePredicate(ctx); wsPred != nil {
		preds = append(preds, wsPred)
	}
	row, err := r.data.RW().Read(ctx).TaskV2.Query().
		Where(preds...).
		Order(ent.Desc(taskv2.FieldSeq)).
		Select(taskv2.FieldSeq).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, entErrToBizErr(err, "TASK_V2")
	}
	return row.Seq, nil
}

// taskV2WorkspacePredicate returns a workspace visibility predicate for tasks_v2 (C-25).
// System callers see all; default workspace also sees legacy empty workspace_id.
func taskV2WorkspacePredicate(ctx context.Context) predicate.TaskV2 {
	ids := workspacePrivateIDs(ctx)
	if ids == nil {
		return nil
	}
	return taskv2.WorkspaceIDIn(ids...)
}

// CreateTask inserts a new Task with the caller's claimed Version.
func (r *taskV2Repo) CreateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	if t.WorkspaceID == "" && !workspace.IsSystem(ctx) {
		t.WorkspaceID = workspace.IDFromContext(ctx)
	}
	b := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetWorkspaceID(t.WorkspaceID).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// UpdateTask overwrites all mutable fields (no version guard).
// Use this only when the caller has already established ordering; for
// concurrent-safe updates prefer UpsertTask.
func (r *taskV2Repo) UpdateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// UpsertTask applies optimistic-concurrency upsert: only succeeds if the
// stored version is older than t.Version. If the version guard fails (either
// no existing row, or stored version >= t.Version), falls back to Create; if
// Create hits a unique constraint (race), returns the existing row.
func (r *taskV2Repo) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	// 1) Try update if stored version is older than t.Version.
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		Where(taskv2.VersionLT(t.Version)).
		SetSessionID(t.SessionID).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version)
	// 2026-07-06 修复：UserMessage/UpdatedAt 空值不覆盖。
	// synthesis turn 的 OnTurnEnd（projector.go OnTurnEnd synthesis 路径）构造
	// 最小 task 对象（仅 ID/SessionID/Status/CompletedAt/Version），UserMessage
	// 为空字符串、UpdatedAt 为零值。如果无条件 SetUserMessage("") SetUpdatedAt(zero)
	// 会覆盖 publishSpiritTeamAssembled 之前持久化的真实 UserMessage 和 UpdatedAt，
	// 导致前端用户指令显示为空、updated_at 异常。采用与 UpsertTeamStage
	// TaskID/TurnID 相同的防御性策略：非空才设置。
	if t.UserMessage != "" {
		b = b.SetUserMessage(t.UserMessage)
	}
	if !t.UpdatedAt.IsZero() {
		b = b.SetUpdatedAt(t.UpdatedAt)
	}
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
		}
		return entTaskV2ToBiz(row), nil
	}
	// UPDATE failed. Two possible causes:
	//   1. Record doesn't exist yet → fall through to CREATE.
	//   2. Record exists but Version >= t.Version (WHERE didn't match) →
	//      return existing record (idempotent: a newer version is already
	//      persisted, e.g. sync persist wrote before the async event arrived).
	//      Without this check, the CREATE fallback would fail with CONFLICT
	//      and propagate an error to the v2 sequencer's retry loop.
	if existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID); getErr == nil {
		return entTaskV2ToBiz(existing), nil
	}
	// 2) Insert if not exists (or version guard rejected the update).
	if t.WorkspaceID == "" && !workspace.IsSystem(ctx) {
		t.WorkspaceID = workspace.IDFromContext(ctx)
	}
	cb := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetWorkspaceID(t.WorkspaceID).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		cb.SetCompletedAt(*t.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
			// Race: another writer inserted first; return the winner.
			existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
			if getErr != nil {
				return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
			}
			return entTaskV2ToBiz(existing), nil
		}
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// CompleteTaskTerminal implements biz.TaskV2Writer.CompleteTaskTerminal:
// unconditional terminalization with version bumped from the DB value (the
// event's Version is ignored — terminal events are monotonic by nature, and
// the synthesis turn's OnTurnEnd hardcodes Version=2 which the UpsertTask
// guard would reject after a resume CAS). Already-terminal tasks
// (completed/failed/cancelled) are not overwritten; interrupted is a recovery
// placeholder and may be terminalized. Unknown IDs fall through to Create so
// shadow tasks (chatTurnEventPublisher.PublishTurnFailure) keep working.
//
// The terminal status set and transition legality come from the v2 task state
// machine (biz.TerminalTaskV2Statuses / biz.CanTransitionTaskV2Status) — the
// single source of truth (P2-Y1). The CAS below enforces the machine's
// {pending, running, interrupted} → terminal edges atomically.
func (r *taskV2Repo) CompleteTaskTerminal(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	if !biz.IsTerminalTaskV2Status(t.Status) {
		return biz.Task{}, fmt.Errorf("task v2 repo: CompleteTaskTerminal requires a terminal status, got %q", t.Status)
	}
	now := time.Now().UTC()
	if !t.UpdatedAt.IsZero() {
		now = t.UpdatedAt
	}
	terminalSet := biz.TerminalTaskV2Statuses()
	terminalStrs := make([]string, 0, len(terminalSet))
	for _, s := range terminalSet {
		terminalStrs = append(terminalStrs, string(s))
	}
	b := r.data.RW().Write(ctx).TaskV2.Update().
		Where(
			taskv2.ID(t.ID),
			taskv2.StatusNotIn(terminalStrs...),
		).
		SetStatus(string(t.Status)).
		SetUpdatedAt(now).
		AddVersion(1)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	} else {
		b.SetCompletedAt(now)
	}
	n, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	if n > 0 {
		row, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
		}
		return entTaskV2ToBiz(row), nil
	}
	// 0 rows: either already terminal (idempotent — return existing) or the
	// task does not exist yet (fall through to Create for shadow tasks).
	if existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID); getErr == nil {
		return entTaskV2ToBiz(existing), nil
	}
	return r.CreateTask(ctx, t)
}

// ResumeInterruptedTask implements biz.TaskV2Writer.ResumeInterruptedTask:
// single-statement CAS interrupted → running so a double click or concurrent
// resume cannot resurrect the task twice. completed_at is cleared because the
// task is active again; version bumps keep the v2 sequencer's VersionLT guard
// monotonic. The CAS and the (entity_kind=task, entity_id) pending
// dead-letter cleanup commit in ONE transaction (FR-10): a stale terminal
// dead-letter replayed after the resurrection would otherwise clobber the
// running task back into a terminal state.
func (r *taskV2Repo) ResumeInterruptedTask(ctx context.Context, id string, resumeAt time.Time) (biz.Task, bool, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, false, fmt.Errorf("task v2 repo: database not configured")
	}
	var resumed biz.Task
	ok := false
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		n, err := r.data.RW().Write(txCtx).TaskV2.Update().
			Where(taskv2.ID(id), taskv2.StatusEQ(string(biz.TaskStatusInterrupted))).
			SetStatus(string(biz.TaskStatusRunning)).
			ClearCompletedAt().
			SetUpdatedAt(resumeAt).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return entErrToBizErr(err, "TASK_V2")
		}
		if n == 0 {
			return nil
		}
		if err := deletePendingEventDeadLetters(txCtx, r.data, biz.EventDeadLetterEntityKindTask, id); err != nil {
			return err
		}
		row, err := r.data.RW().Read(txCtx).TaskV2.Get(txCtx, id)
		if err != nil {
			return entErrToBizErr(err, "TASK_V2")
		}
		resumed = entTaskV2ToBiz(row)
		ok = true
		return nil
	})
	if err != nil {
		return biz.Task{}, false, err
	}
	if !ok {
		return biz.Task{}, false, nil
	}
	return resumed, true, nil
}

// entTaskV2ToBiz converts an Ent TaskV2 row to biz.Task.
func entTaskV2ToBiz(row *ent.TaskV2) biz.Task {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.Task{
		ID:          row.ID,
		SessionID:   row.SessionID,
		UserMessage: row.UserMessage,
		Status:      biz.TaskStatus(row.Status),
		Seq:         row.Seq,
		Version:     row.Version,
		WorkspaceID: row.WorkspaceID,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		CompletedAt: completedAt,
	}
}

func entTasksV2ToBiz(rows []*ent.TaskV2) []biz.Task {
	out := make([]biz.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTaskV2ToBiz(r))
	}
	return out
}
