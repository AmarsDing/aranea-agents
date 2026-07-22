package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/deptleadmessage"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

// deptLeadMessageRepo implements biz.DeptLeadMailboxRepo over Ent (M71).
type deptLeadMessageRepo struct {
	data *Data
}

var _ biz.DeptLeadMailboxRepo = (*deptLeadMessageRepo)(nil)

// NewDeptLeadMessageRepo creates the mailbox repo.
func NewDeptLeadMessageRepo(d *Data) biz.DeptLeadMailboxRepo {
	return &deptLeadMessageRepo{data: d}
}

func entToBizDeptLeadMessage(e *ent.DeptLeadMessage) biz.DeptLeadMessage {
	if e == nil {
		return biz.DeptLeadMessage{}
	}
	return biz.DeptLeadMessage{
		ID:          e.ID,
		FromAgentID: e.FromAgentID,
		FromDeptID:  e.FromDeptID,
		ToAgentID:   e.ToAgentID,
		ToDeptID:    e.ToDeptID,
		Subject:     e.Subject,
		Body:        e.Body,
		RefsJSON:    e.RefsJSON,
		Status:      e.Status,
		ReplyToID:   e.ReplyToID,
		CreatedAt:   e.CreatedAt,
		ReadAt:      e.ReadAt,
	}
}

func (r *deptLeadMessageRepo) CreateMessage(ctx context.Context, msg biz.DeptLeadMessage) (biz.DeptLeadMessage, error) {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	saved, err := r.data.RW().Write(ctx).DeptLeadMessage.Create().
		SetID(msg.ID).
		SetFromAgentID(msg.FromAgentID).
		SetFromDeptID(msg.FromDeptID).
		SetToAgentID(msg.ToAgentID).
		SetToDeptID(msg.ToDeptID).
		SetSubject(msg.Subject).
		SetBody(msg.Body).
		SetRefsJSON(msg.RefsJSON).
		SetStatus(msg.Status).
		SetReplyToID(msg.ReplyToID).
		SetCreatedAt(msg.CreatedAt).
		Save(ctx)
	if err != nil {
		return biz.DeptLeadMessage{}, entErrToBizErr(err, "DEPT_MAIL")
	}
	return entToBizDeptLeadMessage(saved), nil
}

func (r *deptLeadMessageRepo) ListInbox(ctx context.Context, toAgentID, status string, limit int) ([]biz.DeptLeadMessage, error) {
	query := r.data.RW().Read(ctx).DeptLeadMessage.Query().
		Where(deptleadmessage.ToAgentIDEQ(toAgentID))
	if status != "" {
		query = query.Where(deptleadmessage.StatusEQ(status))
	}
	rows, err := query.
		Order(deptleadmessage.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "DEPT_MAIL")
	}
	out := make([]biz.DeptLeadMessage, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizDeptLeadMessage(e))
	}
	return out, nil
}

func (r *deptLeadMessageRepo) GetMessage(ctx context.Context, id string) (biz.DeptLeadMessage, error) {
	row, err := r.data.RW().Read(ctx).DeptLeadMessage.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.DeptLeadMessage{}, apierror.NotFound("DEPT_MAIL", "message not found")
		}
		return biz.DeptLeadMessage{}, entErrToBizErr(err, "DEPT_MAIL")
	}
	return entToBizDeptLeadMessage(row), nil
}

func (r *deptLeadMessageRepo) MarkRead(ctx context.Context, id string, readAt time.Time) error {
	err := r.data.RW().Write(ctx).DeptLeadMessage.UpdateOneID(id).
		SetStatus(biz.DeptMailStatusRead).
		SetReadAt(readAt).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, "DEPT_MAIL")
	}
	return nil
}

func (r *deptLeadMessageRepo) MarkReplied(ctx context.Context, id string) error {
	err := r.data.RW().Write(ctx).DeptLeadMessage.UpdateOneID(id).
		SetStatus(biz.DeptMailStatusReplied).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, "DEPT_MAIL")
	}
	return nil
}
