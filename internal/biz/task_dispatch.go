package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

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
	parentIDs := make([]string, 0, len(parents))
	for _, pl := range parents {
		parentIDs = append(parentIDs, pl.ParentTaskID)
	}
	parentTasks, err := uc.reader.GetTasksByIDs(ctx, parentIDs)
	if err != nil {
		return false, err
	}
	for _, pt := range parentTasks {
		if pt.Status != GraphTaskStatusComplete {
			return false, nil
		}
	}
	return true, nil
}

func (uc *TaskUsecase) IsTaskReadyForDispatch(ctx context.Context, task *GraphTask) bool {
	ready, err := uc.allParentTasksComplete(ctx, task.TaskID)
	return err == nil && ready
}

func (uc *TaskUsecase) BatchResolveReadiness(ctx context.Context, tasks []*GraphTask) map[string]bool {
	readyMap := make(map[string]bool, len(tasks))
	if uc == nil || len(tasks) == 0 {
		return readyMap
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t != nil {
			taskIDs = append(taskIDs, t.TaskID)
		}
	}
	noDeps := make(map[string]bool)
	if uc.linkRepo != nil {
		links, err := uc.linkRepo.ListParentLinksByChildren(ctx, taskIDs)
		if err != nil {
			uc.lg.Warn("batch resolve readiness: ListParentLinksByChildren failed",
				loggateway.StepID("task.batch_readiness"),
				loggateway.Int("task_count", len(taskIDs)), loggateway.Err(err))
			for _, t := range tasks {
				if t != nil {
					readyMap[t.TaskID] = false
				}
			}
			return readyMap
		}
		childHasParents := make(map[string][]string)
		var allParentIDs []string
		for _, link := range links {
			childHasParents[link.ChildTaskID] = append(childHasParents[link.ChildTaskID], link.ParentTaskID)
			allParentIDs = append(allParentIDs, link.ParentTaskID)
		}
		for _, t := range tasks {
			if t != nil {
				if _, has := childHasParents[t.TaskID]; !has {
					noDeps[t.TaskID] = true
				}
			}
		}
		if len(allParentIDs) > 0 {
			parentTasks, err := uc.reader.GetTasksByIDs(ctx, allParentIDs)
			if err != nil {
				uc.lg.Warn("batch resolve readiness: GetTasksByIDs for parents failed",
					loggateway.StepID("task.batch_readiness"),
					loggateway.Int("parent_count", len(allParentIDs)), loggateway.Err(err))
				for _, t := range tasks {
					if t != nil {
						readyMap[t.TaskID] = false
					}
				}
				return readyMap
			}
			completeParents := make(map[string]bool)
			for _, pt := range parentTasks {
				if pt.Status == GraphTaskStatusComplete {
					completeParents[pt.TaskID] = true
				}
			}
			for childID, parentIDs := range childHasParents {
				allComplete := true
				for _, pid := range parentIDs {
					if !completeParents[pid] {
						allComplete = false
						break
					}
				}
				readyMap[childID] = allComplete
			}
		}
	} else {
		for _, t := range tasks {
			if t != nil {
				noDeps[t.TaskID] = true
			}
		}
	}
	for id := range noDeps {
		readyMap[id] = true
	}
	readyCount := 0
	for _, ready := range readyMap {
		if ready {
			readyCount++
		}
	}
	uc.lg.Info("batch resolve readiness completed",
		loggateway.StepID("task.batch_readiness"),
		loggateway.Int("task_count", len(taskIDs)),
		loggateway.Int("ready_count", readyCount),
		loggateway.Int("with_deps_count", len(readyMap)-len(noDeps)),
		loggateway.Int("no_deps_count", len(noDeps)))
	return readyMap
}

func (uc *TaskUsecase) ResolveDispatchAssignee(ctx context.Context, task *GraphTask) string {
	if task == nil {
		return ""
	}
	if key := strings.TrimSpace(task.Assignee); key != "" {
		return key
	}
	if strings.TrimSpace(task.AssignmentMode) == "dynamic" {
		return ""
	}
	exec, err := uc.graph.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return ""
	}
	node := uc.graph.FindGraphNode(ctx, exec.GraphID, task.NodeID)
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.AgentName)
}

func (uc *TaskUsecase) publishGraphTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any) {
	if uc == nil || uc.statusPublisher == nil || task == nil {
		return
	}
	uc.statusPublisher.PublishGraphTaskStatus(ctx, task, extra)
}
