package service

import (
	"context"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func batchScopeFromProto(s *v1.SessionBatchScope) biz.SessionBatchScope {
	if s == nil {
		return biz.SessionBatchScope{}
	}
	return biz.SessionBatchScope{
		OwnerType:     s.GetOwnerType(),
		AgentID:       s.GetAgentId(),
		TeamID:        s.GetTeamId(),
		Status:        s.GetStatus(),
		ContextStatus: s.GetContextStatus(),
		Keyword:       s.GetKeyword(),
		UserID:        s.GetUserId(),
	}
}

func validateBatchHTTPRequest(ids []string, olderThanDays int32) error {
	if len(ids) == 0 && olderThanDays < 1 {
		return apierror.BadRequest("SESSION", "ids or older_than_days >= 1 is required")
	}
	if olderThanDays < 0 {
		return apierror.BadRequest("SESSION", "older_than_days must be >= 0")
	}
	return nil
}

func toProtoBatchPreview(p biz.SessionBatchPreview) *v1.BatchPreviewSessionsResponse {
	return &v1.BatchPreviewSessionsResponse{
		Matched:         int32(p.Matched),
		SkippedRunning:  int32(p.SkippedRunning),
		SampleIds:       p.SampleIDs,
		SkippedNotFound: int32(p.SkippedNotFound),
		Truncated:       p.Truncated,
	}
}

func toProtoBatchResult(r biz.SessionBatchResult) *v1.BatchSessionsResponse {
	return &v1.BatchSessionsResponse{
		Matched:         int32(r.Matched),
		Processed:       int32(r.Processed),
		SkippedRunning:  int32(r.SkippedRunning),
		FailedIds:       r.FailedIDs,
		SkippedNotFound: int32(r.SkippedNotFound),
		Truncated:       r.Truncated,
	}
}

func (s *SessionService) BatchPreviewSessions(ctx context.Context, req *v1.BatchPreviewSessionsRequest) (*v1.BatchPreviewSessionsResponse, error) {
	if strings.TrimSpace(req.GetMode()) == "" {
		return nil, apierror.BadRequest("SESSION", "mode is required")
	}
	if err := validateBatchHTTPRequest(req.GetIds(), req.GetOlderThanDays()); err != nil {
		return nil, err
	}
	out, err := s.uc.PreviewBatch(ctx, biz.BatchOperationParams{
		Mode:            req.GetMode(),
		IDs:             req.GetIds(),
		OlderThanDays:   int(req.GetOlderThanDays()),
		Scope:           batchScopeFromProto(req.GetScope()),
		IncludeArchived: req.GetIncludeArchived(),
	})
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoBatchPreview(out), nil
}

func (s *SessionService) BatchArchiveSessions(ctx context.Context, req *v1.BatchArchiveSessionsRequest) (*v1.BatchSessionsResponse, error) {
	if err := validateBatchHTTPRequest(req.GetIds(), req.GetOlderThanDays()); err != nil {
		return nil, err
	}
	out, err := s.uc.BatchArchive(ctx, req.GetIds(), int(req.GetOlderThanDays()), batchScopeFromProto(req.GetScope()))
	if err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "archive.session.batch", "session", "",
		fmt.Sprintf("matched=%d processed=%d skipped_running=%d skipped_not_found=%d truncated=%v",
			out.Matched, out.Processed, out.SkippedRunning, out.SkippedNotFound, out.Truncated))
	return toProtoBatchResult(out), nil
}

func (s *SessionService) BatchDeleteSessions(ctx context.Context, req *v1.BatchDeleteSessionsRequest) (*v1.BatchSessionsResponse, error) {
	if err := validateBatchHTTPRequest(req.GetIds(), req.GetOlderThanDays()); err != nil {
		return nil, err
	}
	out, err := s.uc.BatchDelete(ctx, req.GetIds(), int(req.GetOlderThanDays()), batchScopeFromProto(req.GetScope()), req.GetIncludeArchived())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "delete.session.batch", "session", "",
		fmt.Sprintf("matched=%d processed=%d skipped_running=%d skipped_not_found=%d truncated=%v include_archived=%v",
			out.Matched, out.Processed, out.SkippedRunning, out.SkippedNotFound, out.Truncated, req.GetIncludeArchived()))
	return toProtoBatchResult(out), nil
}
