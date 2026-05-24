package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ExportSession implements GET /v1/sessions/{id}/export.
func (s *SessionService) ExportSession(ctx context.Context, req *v1.ExportSessionRequest) (*v1.ExportSessionResponse, error) {
	if s.uc == nil {
		return nil, kerrors.InternalServer("SESSION", "session usecase unavailable")
	}
	content, filename, contentType, err := s.uc.Export(ctx, req.GetId(), req.GetFormat())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return &v1.ExportSessionResponse{
		Content:     content,
		Filename:    filename,
		ContentType: contentType,
	}, nil
}

// ListSessionRuns implements GET /v1/sessions/{session_id}/runs.
func (s *SessionService) ListSessionRuns(ctx context.Context, req *v1.ListSessionRunsRequest) (*v1.ListSessionRunsResponse, error) {
	if s.runs == nil {
		return &v1.ListSessionRunsResponse{}, nil
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session_id is required")
	}
	items, total, err := s.runs.ListBySession(ctx, sessionID, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.SessionRunRecord, 0, len(items))
	for _, run := range items {
		out = append(out, toProtoSessionRunRecord(run))
	}
	return &v1.ListSessionRunsResponse{Items: out, Total: int32(total)}, nil
}

// ListSessionParticipants implements GET /v1/sessions/{session_id}/participants.
func (s *SessionService) ListSessionParticipants(ctx context.Context, req *v1.ListSessionParticipantsRequest) (*v1.ListSessionParticipantsResponse, error) {
	if s.uc == nil || s.participants == nil {
		return &v1.ListSessionParticipantsResponse{}, nil
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session_id is required")
	}
	rows, err := s.uc.ListParticipants(ctx, sessionID, s.participants)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.SessionParticipant, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProtoSessionParticipant(row))
	}
	return &v1.ListSessionParticipantsResponse{Items: out}, nil
}

func toProtoSessionRunRecord(run biz.SessionRun) *v1.SessionRunRecord {
	return &v1.SessionRunRecord{
		Id:              run.ID,
		SessionId:       run.SessionID,
		TurnId:          run.TurnID,
		RuntimeRunId:    run.RuntimeRunID,
		Source:          run.Source,
		Phase:           run.Phase,
		SoftBudgetSec:   int32(run.SoftBudgetSec),
		HardBudgetSec:   int32(run.HardBudgetSec),
		CheckpointId:    run.CheckpointID,
		WorkflowJobId:   run.WorkflowJobID,
		AgentId:         run.AgentID,
		ErrorMessage:    run.ErrorMessage,
		StartedAt:       run.StartedAt,
		PhaseChangedAt:  run.PhaseChangedAt,
		FinishedAt:      run.FinishedAt,
		CreatedAt:       run.CreatedAt,
		UpdatedAt:       run.UpdatedAt,
	}
}

func toProtoSessionParticipant(row sessionsess.SessionParticipant) *v1.SessionParticipant {
	return &v1.SessionParticipant{
		Id:               row.ID,
		SessionId:        row.SessionID,
		ParticipantType:  row.ParticipantType,
		ParticipantId:    row.ParticipantID,
		DisplayName:      row.DisplayName,
		RoleInSession:    row.RoleInSession,
		Status:           row.Status,
		FirstActiveAt:    row.FirstActiveAt,
		LastActiveAt:     row.LastActiveAt,
		MessageCount:     int32(row.MessageCount),
		RunStepCount:     int32(row.RunStepCount),
		InputTokens:      int32(row.InputTokens),
		OutputTokens:     int32(row.OutputTokens),
		ContextUsedRatio: row.ContextUsedRatio,
		MetadataJson:     row.MetadataJSON,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
