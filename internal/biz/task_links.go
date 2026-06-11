package biz

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

type TaskLink struct {
	ID            string
	ParentTaskID  string
	ChildTaskID   string
	ExecutionID   string
	CreatedAt     time.Time
}

type TaskLinkRepo interface {
	SaveLink(ctx context.Context, link *TaskLink) error
	DeleteLink(ctx context.Context, parentTaskID, childTaskID string) error
	ListParentLinks(ctx context.Context, childTaskID string) ([]*TaskLink, error)
	ListChildLinks(ctx context.Context, parentTaskID string) ([]*TaskLink, error)
	ListParentLinksByChildren(ctx context.Context, childTaskIDs []string) ([]*TaskLink, error)
}

func (uc *TaskUsecase) LinkTasks(ctx context.Context, parentTaskID, childTaskID string) error {
	if uc.linkRepo == nil {
		return apierror.Internal("TASK", "task link repo not configured")
	}
	parent, err := uc.reader.GetTask(ctx, parentTaskID)
	if err != nil {
		return err
	}
	child, err := uc.reader.GetTask(ctx, childTaskID)
	if err != nil {
		return err
	}
	if parent.ExecutionID != child.ExecutionID {
		return apierror.BadRequest("TASK", "tasks must belong to the same execution")
	}
	link := &TaskLink{
		ID:           uuid.New().String(),
		ParentTaskID: parentTaskID,
		ChildTaskID:  childTaskID,
		ExecutionID:  parent.ExecutionID,
		CreatedAt:    time.Now(),
	}
	if err := uc.linkRepo.SaveLink(ctx, link); err != nil {
		return err
	}
	uc.recordTaskEvent(ctx, childTaskID, "task_linked", child.NodeID, "linked from "+parentTaskID)
	return nil
}

func (uc *TaskUsecase) UnlinkTasks(ctx context.Context, parentTaskID, childTaskID string) error {
	if uc.linkRepo == nil {
		return apierror.Internal("TASK", "task link repo not configured")
	}
	if err := uc.linkRepo.DeleteLink(ctx, parentTaskID, childTaskID); err != nil {
		return err
	}
	uc.recordTaskEvent(ctx, childTaskID, "task_unlinked", "", "unlinked from "+parentTaskID)
	return nil
}

func (uc *TaskUsecase) promoteReadyChildren(ctx context.Context, parentTask *GraphTask) {
	if uc.linkRepo == nil || parentTask == nil {
		return
	}
	links, err := uc.linkRepo.ListChildLinks(ctx, parentTask.TaskID)
	if err != nil {
		return
	}
	for _, link := range links {
		child, err := uc.reader.GetTask(ctx, link.ChildTaskID)
		if err != nil || child.Status != TaskStatusPending {
			continue
		}
		ready, err := uc.allParentTasksComplete(ctx, child.TaskID)
		if err != nil || !ready {
			continue
		}
		uc.recordTaskEvent(ctx, child.TaskID, "task_ready", child.NodeID, fmt.Sprintf("promoted after parent %s complete", parentTask.TaskID))
		uc.publishTaskStatus(ctx, child, map[string]any{"promoted": true})
	}
}
