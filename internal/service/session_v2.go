package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// SessionV2Service implements the v2 entity read RPCs (Task/Turn/Step) by
// delegating to the v2 repo readers. It replaces the v1 ListActivities path.
type SessionV2Service struct {
	v1.UnimplementedSessionV2ServiceServer

	taskReader biz.TaskV2Reader
	turnReader biz.TurnV2Reader
	stepReader biz.StepV2Reader
}

// NewSessionV2Service constructs a SessionV2Service from v2 repo readers.
func NewSessionV2Service(taskReader biz.TaskV2Reader, turnReader biz.TurnV2Reader, stepReader biz.StepV2Reader) *SessionV2Service {
	return &SessionV2Service{
		taskReader: taskReader,
		turnReader: turnReader,
		stepReader: stepReader,
	}
}

// ListTasks returns all tasks for a session.
func (s *SessionV2Service) ListTasks(ctx context.Context, req *v1.ListTasksV2Request) (*v1.ListTasksV2Response, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required")
	}
	tasks, err := s.taskReader.ListTasksBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TaskV2, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, bizTaskToProto(t))
	}
	return &v1.ListTasksV2Response{Tasks: out}, nil
}

// ListTurns returns all turns for a task.
func (s *SessionV2Service) ListTurns(ctx context.Context, req *v1.ListTurnsV2Request) (*v1.ListTurnsV2Response, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "task_id is required")
	}
	turns, err := s.turnReader.ListTurnsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TurnV2, 0, len(turns))
	for _, t := range turns {
		out = append(out, bizTurnToProto(t))
	}
	return &v1.ListTurnsV2Response{Turns: out}, nil
}

// ListSteps returns steps filtered by turn_id, task_id, or session_id.
func (s *SessionV2Service) ListSteps(ctx context.Context, req *v1.ListStepsV2Request) (*v1.ListStepsV2Response, error) {
	var steps []biz.Step
	var err error
	switch {
	case strings.TrimSpace(req.GetTurnId()) != "":
		steps, err = s.stepReader.ListStepsByTurn(ctx, req.GetTurnId())
	case strings.TrimSpace(req.GetTaskId()) != "":
		steps, err = s.stepReader.ListStepsByTask(ctx, req.GetTaskId())
	default:
		sessionID := strings.TrimSpace(req.GetSessionId())
		if sessionID == "" {
			return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required when turn_id and task_id are empty")
		}
		steps, err = s.stepReader.ListStepsBySession(ctx, sessionID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*v1.StepV2, 0, len(steps))
	for _, st := range steps {
		out = append(out, bizStepToProto(st))
	}
	return &v1.ListStepsV2Response{Steps: out}, nil
}

// GetStep returns a single step by ID.
func (s *SessionV2Service) GetStep(ctx context.Context, req *v1.GetStepV2Request) (*v1.GetStepV2Response, error) {
	stepID := strings.TrimSpace(req.GetStepId())
	if stepID == "" {
		return nil, apierror.BadRequest(apierror.DomainShared, "step_id is required")
	}
	step, err := s.stepReader.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	return &v1.GetStepV2Response{Step: bizStepToProto(step)}, nil
}

// === Conversion helpers (biz → proto) ===

func bizTaskToProto(t biz.Task) *v1.TaskV2 {
	return &v1.TaskV2{
		Id:          t.ID,
		SessionId:   t.SessionID,
		UserMessage: t.UserMessage,
		Status:      string(t.Status),
		Seq:         t.Seq,
		Version:     t.Version,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
		CompletedAt: formatTimePtr(t.CompletedAt),
	}
}

func bizTurnToProto(t biz.Turn) *v1.TurnV2 {
	return &v1.TurnV2{
		Id:              t.ID,
		TaskId:          t.TaskID,
		SessionId:       t.SessionID,
		SpiritSessionId: t.SpiritSessionID,
		ParentTurnId:    t.ParentTurnID,
		AgentKey:        t.AgentKey,
		TeamId:          t.TeamID,
		TeamStageId:     t.TeamStageID,
		Seq:             t.Seq,
		Version:         t.Version,
		Status:          string(t.Status),
		StartedAt:       t.StartedAt.Format(time.RFC3339),
		CompletedAt:     formatTimePtr(t.CompletedAt),
	}
}

func bizStepToProto(s biz.Step) *v1.StepV2 {
	return &v1.StepV2{
		Id:              s.ID,
		TurnId:          s.TurnID,
		TaskId:          s.TaskID,
		SessionId:       s.SessionID,
		SpiritSessionId: s.SpiritSessionID,
		Kind:            string(s.Kind),
		AuthorAgentKey:  s.AuthorAgentKey,
		Seq:             s.Seq,
		Version:         s.Version,
		Content:         s.Content,
		Reasoning:       s.Reasoning,
		ToolName:        s.ToolName,
		ToolCallId:      s.ToolCallID,
		ToolArgs:        []byte(s.ToolArgs),
		ToolResult:      []byte(s.ToolResult),
		ToolDurationMs:   s.ToolDurationMs,
		ToolErrorCode:   s.ToolErrorCode,
		NoticeType:      s.NoticeType,
		Status:          string(s.Status),
		IsFinal:         s.IsFinal,
		StartedAt:       s.StartedAt.Format(time.RFC3339),
		CompletedAt:     formatTimePtr(s.CompletedAt),
	}
}

// formatTimePtr formats a *time.Time as RFC3339, returning "" for nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
