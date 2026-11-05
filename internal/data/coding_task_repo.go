package data

import (
	"context"
	"fmt"

	bridge "aranea-agents/internal/biz/agentbridge"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/codingagent"
	"aranea-agents/internal/data/ent/codingtask"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// codingTaskRepo implements bridge.TaskRepo via Ent.
type codingTaskRepo struct {
	data *Data
}

var _ bridge.TaskRepo = (*codingTaskRepo)(nil)

// NewCodingTaskRepo returns the Ent-backed TaskRepo.
func NewCodingTaskRepo(d *Data) bridge.TaskRepo {
	return &codingTaskRepo{data: d}
}

func entCodingTaskToBiz(e *dataent.CodingTask) *bridge.CodingTask {
	return &bridge.CodingTask{
		ID:            e.ID,
		Workspace:     e.Workspace,
		SessionID:     e.SessionID,
		AgentID:       e.AgentID,
		ProjectID:     e.ProjectID,
		Prompt:        e.Prompt,
		Status:        bridge.TaskStatus(e.Status),
		ACPSessionID:  e.AcpSessionID,
		Summary:       e.Summary,
		Error:         e.Error,
		ProgressCount: e.ProgressCount,
		// M4 P0-2b：子进程 PID + 活跃性监管字段
		ChildPID:      e.ChildPid,
		LastActiveAt:  e.LastActiveAt,
		LastExcerpt:   e.LastExcerpt,
		HeartbeatMiss: e.HeartbeatMiss,
		LivenessState: e.LivenessState,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
		CompletedAt:   e.CompletedAt,
	}
}

func (r *codingTaskRepo) Create(ctx context.Context, t *bridge.CodingTask) error {
	now := nowRFC3339()
	if t.Workspace == "" {
		t.Workspace = "default"
	}
	if t.Status == "" {
		t.Status = bridge.StatusDispatched
	}
	id := t.ID
	if id == "" {
		id = "codingtask_" + uuid.NewString()
	}
	row, err := r.data.RW().Write(ctx).CodingTask.Create().
		SetID(id).
		SetWorkspace(t.Workspace).
		SetSessionID(t.SessionID).
		SetAgentID(t.AgentID).
		SetProjectID(t.ProjectID).
		SetPrompt(t.Prompt).
		SetStatus(string(t.Status)).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	origin := t.Origin
	*t = *entCodingTaskToBiz(row)
	t.Origin = origin
	// 03 P0：origin 是 raw SQL 列（ent 模型不含，迁移 20261308），建任务时落库。
	if t.Origin != "" {
		db := r.data.RWDB().WriteDB(ctx)
		if db == nil {
			return apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
		}
		if _, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
			`UPDATE coding_tasks SET origin=? WHERE id=?`), t.Origin, t.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *codingTaskRepo) Get(ctx context.Context, id string) (*bridge.CodingTask, error) {
	row, err := r.data.RW().Read(ctx).CodingTask.Get(ctx, id)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	t := entCodingTaskToBiz(row)
	if _, err := r.mergeTaskExtras(ctx, []*bridge.CodingTask{t}); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateStatus 以 CAS 方式转换状态：WHERE id=? AND status=?，0 行命中时区分
// NotFound（任务不存在）与 Conflict（状态已被并发推进）。
func (r *codingTaskRepo) UpdateStatus(ctx context.Context, id string, from, to bridge.TaskStatus, patch bridge.TaskPatch) error {
	upd := r.data.RW().Write(ctx).CodingTask.Update().
		Where(codingtask.ID(id), codingtask.StatusEQ(string(from))).
		SetStatus(string(to)).
		SetUpdatedAt(nowRFC3339())
	if patch.ACPSessionID != nil {
		upd = upd.SetAcpSessionID(*patch.ACPSessionID)
	}
	if patch.Summary != nil {
		upd = upd.SetSummary(*patch.Summary)
	}
	if patch.Error != nil {
		upd = upd.SetError(*patch.Error)
	}
	if patch.CompletedAt != nil {
		upd = upd.SetCompletedAt(*patch.CompletedAt)
	}
	if patch.ProgressCount != nil {
		upd = upd.SetProgressCount(*patch.ProgressCount)
	}
	// M4 P0-2b：child_pid + 活跃性监管补丁
	if patch.ChildPID != nil {
		upd = upd.SetChildPid(*patch.ChildPID)
	}
	if patch.LastActiveAt != nil {
		upd = upd.SetLastActiveAt(*patch.LastActiveAt)
	}
	if patch.LastExcerpt != nil {
		upd = upd.SetLastExcerpt(*patch.LastExcerpt)
	}
	if patch.HeartbeatMiss != nil {
		upd = upd.SetHeartbeatMiss(*patch.HeartbeatMiss)
	}
	if patch.LivenessState != nil {
		upd = upd.SetLivenessState(*patch.LivenessState)
	}
	n, err := upd.Save(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	if n > 0 {
		return nil
	}
	// 0 rows: distinguish missing task from stale from-state.
	if _, getErr := r.data.RW().Read(ctx).CodingTask.Get(ctx, id); getErr != nil {
		return entErrToBizErr(getErr, apierror.DomainAgentBridge)
	}
	return apierror.Conflict(apierror.DomainAgentBridge,
		"coding task %s status no longer %q (lost-update guard)", id, from)
}

func (r *codingTaskRepo) ListBySession(ctx context.Context, sessionID string, limit int) ([]*bridge.CodingTask, error) {
	q := r.data.RW().Read(ctx).CodingTask.Query().
		Where(codingtask.SessionIDEQ(sessionID)).
		Order(dataent.Desc(codingtask.FieldCreatedAt), dataent.Desc(codingtask.FieldID))
	if limit > 0 {
		q = q.Limit(limit)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	out := make([]*bridge.CodingTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, entCodingTaskToBiz(row))
	}
	return out, nil
}

// --- 03 P0：看板跨会话视图 / 验收（origin/review_* 为 raw SQL 列，迁移 20261308） ---

// ListRecent 跨会话看板：按创建时间倒序；agentKey 非空时按工具过滤。
func (r *codingTaskRepo) ListRecent(ctx context.Context, limit int, agentKey string) ([]*bridge.CodingTask, error) {
	q := r.data.RW().Read(ctx).CodingTask.Query().
		Order(dataent.Desc(codingtask.FieldCreatedAt), dataent.Desc(codingtask.FieldID))
	if agentKey != "" {
		agentRow, err := r.data.RW().Read(ctx).CodingAgent.Query().
			Where(codingagent.AgentKeyEQ(agentKey)).First(ctx)
		if err != nil {
			if dataent.IsNotFound(err) {
				return []*bridge.CodingTask{}, nil
			}
			return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
		}
		q = q.Where(codingtask.AgentIDEQ(agentRow.ID))
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	out := make([]*bridge.CodingTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, entCodingTaskToBiz(row))
	}
	return r.mergeTaskExtras(ctx, out)
}

// mergeTaskExtras 回填 raw SQL 列（origin/review_*），ent 模型不含这些列。
func (r *codingTaskRepo) mergeTaskExtras(ctx context.Context, tasks []*bridge.CodingTask) ([]*bridge.CodingTask, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	idx := make(map[string]*bridge.CodingTask, len(tasks))
	ids := make([]string, 0, len(tasks))
	args := make([]any, 0, len(tasks))
	for _, t := range tasks {
		idx[t.ID] = t
		ids = append(ids, t.ID)
		args = append(args, t.ID)
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	q := fmt.Sprintf(`SELECT id, origin, review_status, review_comment, reviewed_at FROM coding_tasks WHERE id IN (%s)`,
		r.data.Dialect().Placeholders(len(ids)))
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, origin, status, comment, at string
		if err := rows.Scan(&id, &origin, &status, &comment, &at); err != nil {
			return nil, err
		}
		if t := idx[id]; t != nil {
			t.Origin, t.ReviewStatus, t.ReviewComment, t.ReviewedAt = origin, status, comment, at
		}
	}
	return tasks, rows.Err()
}

// UpdateReview 验收三动作落库（status: approved/rejected/archived）。
func (r *codingTaskRepo) UpdateReview(ctx context.Context, id, status, comment, reviewedAt string) error {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	res, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`UPDATE coding_tasks SET review_status=?, review_comment=?, reviewed_at=?, updated_at=? WHERE id=?`),
		status, comment, reviewedAt, nowRFC3339(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apierror.NotFound(apierror.DomainAgentBridge, "coding task %s not found", id)
	}
	return nil
}

func (r *codingTaskRepo) ListActive(ctx context.Context) ([]*bridge.CodingTask, error) {
	rows, err := r.data.RW().Read(ctx).CodingTask.Query().
		Where(codingtask.StatusNotIn(
			string(bridge.StatusDone),
			string(bridge.StatusFailed),
			string(bridge.StatusCancelled),
		)).
		Order(dataent.Asc(codingtask.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	out := make([]*bridge.CodingTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, entCodingTaskToBiz(row))
	}
	return out, nil
}
