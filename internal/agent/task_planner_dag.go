package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func validateSubTaskDAG(tasks []biz.SubTask) error {
	idSet := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		// F7a/B2：重复 ID 必须拒绝——同名节点互相遮蔽，依赖悬空/错接，
		// 执行器会静默跑错图。
		if idSet[t.ID] {
			return apierror.BadRequest(apierror.DomainSpirit, "duplicate subtask ID %s", t.ID)
		}
		idSet[t.ID] = true
	}

	// Check all depends_on references exist
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			if !idSet[depID] {
				return apierror.BadRequest(apierror.DomainSpirit, "subtask %s depends on non-existent subtask %s", t.ID, depID)
			}
		}
	}

	// Check for cycles using DFS
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := make(map[string]int, len(tasks))
	for _, t := range tasks {
		colors[t.ID] = white
	}

	taskMap := make(map[string]*biz.SubTask, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	var dfs func(id string) error
	dfs = func(id string) error {
		colors[id] = gray
		t := taskMap[id]
		if t != nil {
			for _, depID := range t.DependsOn {
				switch colors[depID] {
				case gray:
					return apierror.BadRequest(apierror.DomainSpirit, "cycle detected: %s → %s", id, depID)
				case white:
					if err := dfs(depID); err != nil {
						return err
					}
				}
			}
		}
		colors[id] = black
		return nil
	}

	for _, t := range tasks {
		if colors[t.ID] == white {
			if err := dfs(t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildDAGFromSubTasks constructs a PlanTaskDAG from subtasks.
func buildDAGFromSubTasks(tasks []biz.SubTask) *biz.PlanTaskDAG {
	if len(tasks) == 0 {
		return nil
	}

	dag := &biz.PlanTaskDAG{
		Nodes: tasks,
	}

	// Compute root IDs (no dependencies)
	hasIncoming := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		hasIncoming[t.ID] = false
	}
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasIncoming[depID] = true
		}
	}
	for id, has := range hasIncoming {
		if !has {
			dag.RootIDs = append(dag.RootIDs, id)
		}
	}

	// Compute leaf IDs (nothing depends on them)
	hasOutgoing := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasOutgoing[depID] = true
		}
	}
	for _, t := range tasks {
		if !hasOutgoing[t.ID] {
			dag.LeafIDs = append(dag.LeafIDs, t.ID)
		}
	}

	return dag
}

// mergeSubTasks merges specified subtasks into one.
func mergeSubTasks(tasks []biz.SubTask, ids []string) []biz.SubTask {
	if len(ids) < 2 {
		return tasks
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var merged biz.SubTask
	var remaining []biz.SubTask
	first := true
	for _, t := range tasks {
		if idSet[t.ID] {
			if first {
				merged = biz.SubTask{
					ID:                   t.ID,
					Name:                 t.Name,
					Description:          t.Description,
					DependsOn:            t.DependsOn,
					RequiredCapabilities: t.RequiredCapabilities,
					Priority:             t.Priority,
					EstimatedComplexity:  t.EstimatedComplexity,
				}
				first = false
			} else {
				merged.Description += "; " + t.Description
				for _, cap := range t.RequiredCapabilities {
					merged.RequiredCapabilities = append(merged.RequiredCapabilities, cap)
				}
				for _, dep := range t.DependsOn {
					if !idSet[dep] {
						merged.DependsOn = append(merged.DependsOn, dep)
					}
				}
				if t.Priority < merged.Priority {
					merged.Priority = t.Priority
				}
				merged.EstimatedComplexity = max(merged.EstimatedComplexity, t.EstimatedComplexity)
			}
		} else {
			remaining = append(remaining, t)
		}
	}
	if !first {
		remaining = append(remaining, merged)
	}
	return remaining
}

// splitSubTask splits a subtask into two.
func splitSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			// Split into two: original + new
			split := biz.SubTask{
				ID:                   id + "_b",
				Name:                 t.Name + " (Part 2)",
				Description:          t.Description,
				DependsOn:            []string{id},
				RequiredCapabilities: t.RequiredCapabilities,
				Priority:             t.Priority + 1,
				EstimatedComplexity:  t.EstimatedComplexity / 2,
			}
			t.Name = t.Name + " (Part 1)"
			t.EstimatedComplexity = t.EstimatedComplexity / 2
			result = append(result, t, split)
		} else {
			// Update references to the split task
			for i, dep := range t.DependsOn {
				if dep == id {
					t.DependsOn[i] = id + "_b"
				}
			}
			result = append(result, t)
		}
	}
	return result
}

// removeSubTask removes a subtask by ID.
func removeSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			continue
		}
		// Remove the deleted ID from depends_on
		var filteredDeps []string
		for _, dep := range t.DependsOn {
			if dep != id {
				filteredDeps = append(filteredDeps, dep)
			}
		}
		t.DependsOn = filteredDeps
		result = append(result, t)
	}
	return result
}
