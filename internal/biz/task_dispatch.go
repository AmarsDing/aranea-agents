package biz

import (
	"context"
	"strings"
)

// allParentTasksComplete reports whether every linked parent task is complete.
func (uc *TaskUsecase) allParentTasksComplete(ctx context.Context, childTaskID string) (bool, error) {
	if uc == nil || uc.linkRepo == nil {
		return true, nil
	}
	parents, err := uc.linkRepo.ListParentLinks(ctx, childTaskID)
	if err != nil {
		return false, err
	}
	if len(parents) == 0 {
		return true, nil
	}
	for _, pl := range parents {
		pt, err := uc.repo.GetTask(ctx, pl.ParentTaskID)
		if err != nil {
			return false, err
		}
		if pt.Status != TaskStatusComplete {
			return false, nil
		}
	}
	return true, nil
}

// isTaskReadyForDispatch gates dispatcher claim until dependency parents are complete.
func (uc *TaskUsecase) isTaskReadyForDispatch(ctx context.Context, task *GraphTask) bool {
	ready, err := uc.allParentTasksComplete(ctx, task.TaskID)
	return err == nil && ready
}

// resolveDispatchAssignee picks a static assignee from task row or graph node definition.
func (uc *TaskUsecase) resolveDispatchAssignee(ctx context.Context, task *GraphTask) string {
	if task == nil {
		return ""
	}
	if key := strings.TrimSpace(task.Assignee); key != "" {
		return key
	}
	if strings.TrimSpace(task.AssignmentMode) == "dynamic" {
		return ""
	}
	exec, err := uc.graphUC.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return ""
	}
	node := uc.graphUC.FindGraphNode(ctx, exec.GraphID, task.NodeID)
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.AgentName)
}

func (uc *TaskUsecase) publishTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any) {
	if uc == nil || uc.statusPublisher == nil || task == nil {
		return
	}
	uc.statusPublisher.PublishTaskStatus(ctx, task, extra)
}
