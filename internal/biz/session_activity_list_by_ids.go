package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/session"
)

// ListByIDs implements session.ActivityIDLister: fetch steps/tasks by primary
// key instead of ListBySession + memory filter (COMPRESS-IO / W1-5).
func (a *sessionActivityLister) ListByIDs(ctx context.Context, sessionID string, ids []string) ([]session.ActivityEntry, error) {
	if a == nil || a.stepReader == nil {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ids))
	acts := make([]Activity, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		if step, err := a.stepReader.GetStep(ctx, id); err == nil && step.ID != "" {
			if sessionID == "" || step.SessionID == "" || step.SessionID == sessionID {
				acts = append(acts, StepToActivity(step))
				continue
			}
		}
		if a.taskReader == nil {
			continue
		}
		if task, err := a.taskReader.GetTask(ctx, id); err == nil && task.ID != "" {
			if sessionID == "" || task.SessionID == "" || task.SessionID == sessionID {
				acts = append(acts, taskToActivity(task))
			}
		}
	}
	return activitiesToSessionEntries(acts), nil
}
