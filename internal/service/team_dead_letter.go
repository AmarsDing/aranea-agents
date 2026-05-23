package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func taskDeadLetterToProto(dl biz.TaskDeadLetter) *v1.TaskDeadLetter {
	return &v1.TaskDeadLetter{
		Id:               dl.ID,
		SourceType:       dl.SourceType,
		SourceId:         dl.SourceID,
		TeamId:           dl.TeamID,
		TeamRunId:        dl.TeamRunID,
		SessionId:        dl.SessionID,
		GraphExecutionId: dl.GraphExecutionID,
		ErrorMessage:     dl.ErrorMessage,
		PayloadJson:      dl.PayloadJSON,
		Status:           dl.Status,
		CreatedAt:        dl.CreatedAt,
		ResolvedAt:       dl.ResolvedAt,
	}
}

// ListTaskDeadLetters lists halted orchestration dead letters for admin review (FP-04).
func (s *TeamService) ListTaskDeadLetters(ctx context.Context, req *v1.ListTaskDeadLettersRequest) (*v1.ListTaskDeadLettersResponse, error) {
	if s == nil || s.uc == nil {
		return &v1.ListTaskDeadLettersResponse{}, nil
	}
	if req == nil {
		req = &v1.ListTaskDeadLettersRequest{}
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	teamID := strings.TrimSpace(req.GetTeamId())
	if sessionID == "" && teamID == "" {
		return nil, kerrors.BadRequest("TEAM", "session_id or team_id is required")
	}
	items, err := s.uc.ListTaskDeadLetters(ctx, biz.TaskDeadLetterListFilter{
		SessionID: sessionID,
		TeamID:    teamID,
		Status:    strings.TrimSpace(req.GetStatus()),
		Limit:     int(req.GetLimit()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.TaskDeadLetter, 0, len(items))
	for _, item := range items {
		cp := item
		out = append(out, taskDeadLetterToProto(cp))
	}
	return &v1.ListTaskDeadLettersResponse{Items: out}, nil
}

// ResolveTaskDeadLetter marks a dead letter as resolved (FP-04).
func (s *TeamService) ResolveTaskDeadLetter(ctx context.Context, req *v1.ResolveTaskDeadLetterRequest) (*v1.ResolveTaskDeadLetterResponse, error) {
	if s == nil || s.uc == nil {
		return nil, kerrors.InternalServer("TEAM", "team service not configured")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, kerrors.BadRequest("TEAM", "id is required")
	}
	item, err := s.uc.ResolveTaskDeadLetter(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.ResolveTaskDeadLetterResponse{Item: taskDeadLetterToProto(item)}, nil
}
