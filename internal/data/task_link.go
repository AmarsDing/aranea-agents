package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphtasklink"

	"github.com/go-kratos/kratos/v2/errors"
)

func (r *taskRepo) SaveLink(ctx context.Context, link *biz.TaskLink) error {
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTaskLink.Create().
		SetID(link.ID).
		SetParentTaskID(link.ParentTaskID).
		SetChildTaskID(link.ChildTaskID).
		SetExecutionID(link.ExecutionID).
		SetCreatedAt(link.CreatedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("task link save: %w", err)
	}
	return nil
}

func (r *taskRepo) DeleteLink(ctx context.Context, parentTaskID, childTaskID string) error {
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTaskLink.Delete().
		Where(
			graphtasklink.ParentTaskIDEQ(parentTaskID),
			graphtasklink.ChildTaskIDEQ(childTaskID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("task link delete: %w", err)
	}
	return nil
}

func (r *taskRepo) ListParentLinks(ctx context.Context, childTaskID string) ([]*biz.TaskLink, error) {
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTaskLink.Query().
		Where(graphtasklink.ChildTaskIDEQ(childTaskID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task link list parents: %w", err)
	}
	return entLinksToBiz(rows), nil
}

func (r *taskRepo) ListParentLinksByChildren(ctx context.Context, childTaskIDs []string) ([]*biz.TaskLink, error) {
	if len(childTaskIDs) == 0 {
		return nil, nil
	}
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTaskLink.Query().
		Where(graphtasklink.ChildTaskIDIn(childTaskIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task link list parents by children: %w", err)
	}
	return entLinksToBiz(rows), nil
}

func (r *taskRepo) ListChildLinks(ctx context.Context, parentTaskID string) ([]*biz.TaskLink, error) {
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTaskLink.Query().
		Where(graphtasklink.ParentTaskIDEQ(parentTaskID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task link list children: %w", err)
	}
	return entLinksToBiz(rows), nil
}

func entLinksToBiz(rows []*ent.GraphTaskLink) []*biz.TaskLink {
	out := make([]*biz.TaskLink, len(rows))
	for i, row := range rows {
		out[i] = &biz.TaskLink{
			ID:           row.ID,
			ParentTaskID: row.ParentTaskID,
			ChildTaskID:  row.ChildTaskID,
			ExecutionID:  row.ExecutionID,
			CreatedAt:    row.CreatedAt,
		}
	}
	return out
}

// Ensure taskRepo implements TaskLinkRepo.
var _ biz.TaskLinkRepo = (*taskRepo)(nil)

func (r *taskRepo) SaveLinkGuard(ctx context.Context, link *biz.TaskLink) error {
	if link == nil {
		return errors.BadRequest("TASK", "link required")
	}
	return r.SaveLink(ctx, link)
}
