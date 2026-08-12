package data

import (
	"context"

	bridge "aranea-agents/internal/biz/agentbridge"
	dataent "aranea-agents/internal/data/ent"
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
	*t = *entCodingTaskToBiz(row)
	return nil
}

func (r *codingTaskRepo) Get(ctx context.Context, id string) (*bridge.CodingTask, error) {
	row, err := r.data.RW().Read(ctx).CodingTask.Get(ctx, id)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	return entCodingTaskToBiz(row), nil
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
