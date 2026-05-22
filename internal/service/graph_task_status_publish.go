package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func (s *GraphService) publishTaskOrchestrationStatus(ctx context.Context, task *biz.GraphTask, extra map[string]any) {
	if s == nil || s.orchProjector == nil || task == nil {
		return
	}
	execID := strings.TrimSpace(task.ExecutionID)
	if execID == "" {
		return
	}
	exec, err := s.uc.GetExecution(ctx, execID)
	if err != nil || exec == nil {
		return
	}
	s.orchProjector.PublishGraphTaskStatus(ctx, exec.SessionID, execID, exec.GraphID, task, extra)
}
