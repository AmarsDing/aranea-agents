package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/strutil"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// SessionService implements kratos session.v1.
type SessionService struct {
	v1.UnimplementedSessionServiceServer

	uc               *biz.SessionUsecase
	mon              *biz.MonitorUsecase
	runs             *biz.SessionRunUsecase
	compress         biz.ManualCompressor
	compressStatus   biz.CompressStatusReader
	metricsCache     biz.SessionMetricsReader
	activityReader   biz.ActivityReader
}

func NewSessionService(
	uc *biz.SessionUsecase,
	mon *biz.MonitorUsecase,
	runs *biz.SessionRunUsecase,
	compress biz.ManualCompressor,
	compressStatus biz.CompressStatusReader,
	metricsCache biz.SessionMetricsReader,
	activityReader biz.ActivityReader,
) *SessionService {
	return &SessionService{uc: uc, mon: mon, runs: runs, compress: compress, compressStatus: compressStatus, metricsCache: metricsCache, activityReader: activityReader}
}

func toProtoSession(s biz.Session, metrics *biz.SessionMetrics) *v1.Session {
	p := &v1.Session{
		Id:                         s.ID,
		WorkspaceId:                s.WorkspaceID,
		UserId:                     s.UserID,
		OwnerType:                  s.OwnerType,
		AgentId:                    s.AgentID,
		TeamId:                     s.TeamID,
		Title:                      s.Title,
		Summary:                    s.Summary,
		TagsJson:                   s.TagsJSON,
		DialogMode:                 s.DialogMode,
		DefaultProvider:            s.DefaultProvider,
		DefaultModel:               s.DefaultModel,
		DefaultContextWindowTokens: int32(s.DefaultContextWindowTokens),
		LastProvider:               s.LastProvider,
		LastModel:                  s.LastModel,
		LastContextWindowTokens:    int32(s.LastContextWindowTokens),
		Status:                     s.Status,
		StatusReason:               s.StatusReason,
		StatusChangedAt:            s.StatusChangedAt,
		Visibility:                 s.Visibility,
		MessageCount:               int32(s.MessageCount),
		RunCount:                   int32(s.RunCount),
		ModelCallCount:             int32(s.ModelCallCount),
		ToolCallCount:              int32(s.ToolCallCount),
		SkillCallCount:             int32(s.SkillCallCount),
		McpCallCount:               int32(s.MCPCallCount),
		InputTokens:                int32(s.InputTokens),
		OutputTokens:               int32(s.OutputTokens),
		TotalTokens:                int32(s.TotalTokens),
		TotalCostMicroUsd:          s.TotalCostMicroUSD,
		AvgLatencyMs:               s.AvgLatencyMs,
		ErrorCount:                 int32(s.ErrorCount),
		ContextUsedTokens:          int32(s.ContextUsedTokens),
		ContextUsedRatio:           s.ContextUsedRatio,
		MaxContextUsedRatio:        s.MaxContextUsedRatio,
		ContextStatus:              s.ContextStatus,
		FirstMessageAt:             s.FirstMessageAt,
		LastMessageAt:              s.LastMessageAt,
		LastRunAt:                  s.LastRunAt,
		CreatedAt:                  s.CreatedAt,
		UpdatedAt:                  s.UpdatedAt,
		ArchivedAt:                 s.ArchivedAt,
		DeletedAt:                  s.DeletedAt,
		PinnedAt:                   s.PinnedAt,
		RunnerSnapshotJson:         s.RunnerSnapshotJSON,
		MetadataJson:               s.MetadataJSON,
		StateJson:                  s.StateJSON,
		ParentSessionId:            s.ParentSessionID,
		RootSessionId:              s.RootSessionID,
		AgentDepth:                 int32(s.AgentDepth),
	}
	// 如果有 metrics 数据，用新表数据覆盖
	if metrics != nil {
		p.MessageCount = int32(metrics.MessageCount)
		p.RunCount = int32(metrics.RunCount)
		p.ModelCallCount = int32(metrics.ModelCallCount)
		p.ToolCallCount = int32(metrics.ToolCallCount)
		p.SkillCallCount = int32(metrics.SkillCallCount)
		p.McpCallCount = int32(metrics.MCPCallCount)
		p.InputTokens = int32(metrics.InputTokens)
		p.OutputTokens = int32(metrics.OutputTokens)
		p.TotalTokens = int32(metrics.TotalTokens)
		p.TotalCostMicroUsd = metrics.TotalCostMicroUSD
		p.AvgLatencyMs = metrics.AvgLatencyMs
		p.ErrorCount = int32(metrics.ErrorCount)
		p.ContextUsedTokens = int32(metrics.ContextUsedTokens)
		p.ContextUsedRatio = metrics.ContextUsedRatio
		p.MaxContextUsedRatio = metrics.MaxContextUsedRatio
		p.ContextStatus = metrics.ContextStatus
		if metrics.LastMessageAt != "" {
			p.LastMessageAt = metrics.LastMessageAt
		}
	}
	return p
}

func toProtoTimeline(t biz.SessionTimeline) *v1.SessionTimeline {
	items := make([]*v1.SessionTimelineItem, 0, len(t.Items))
	for i := range t.Items {
		items = append(items, toProtoTimelineItem(t.Items[i]))
	}
	return &v1.SessionTimeline{
		SessionId: t.SessionID,
		Items:     items,
		Summary: &v1.SessionTimelineSummary{
			Total:        int32(t.Summary.Total),
			MessageCount: int32(t.Summary.MessageCount),
			ToolCount:    int32(t.Summary.ToolCount),
			SkillCount:   int32(t.Summary.SkillCount),
			McpCount:     int32(t.Summary.MCPCount),
		},
	}
}

func toProtoTimelineItem(it biz.SessionTimelineItem) *v1.SessionTimelineItem {
	tags := it.Tags
	if tags == nil {
		tags = []string{}
	}
	return &v1.SessionTimelineItem{
		Id:              it.ID,
		Kind:            it.Kind,
		Side:            it.Side,
		Title:           it.Title,
		Subtitle:        it.Subtitle,
		ActorId:         it.ActorID,
		ActorName:       it.ActorName,
		Status:          it.Status,
		OccurredAt:      it.OccurredAt,
		DurationMs:      int32(it.DurationMS),
		ContentMarkdown: it.ContentMarkdown,
		Preview:         it.Preview,
		DetailJson:      it.DetailJSON,
		Tags:            tags,
	}
}

func mapSessionErr(err error) error {
	if err == nil {
		return nil
	}
	if apierror.IsCode(err, apierror.CodeNotFound) {
		return apierror.NotFound("SESSION", "session not found")
	}
	return err
}

func searchQueryFromProto(req *v1.SearchSessionsRequest) biz.SessionSearchQuery {
	if req == nil {
		return biz.SessionSearchQuery{}
	}
	return biz.SessionSearchQuery{
		OwnerType:     req.GetOwnerType(),
		AgentID:       req.GetAgentId(),
		TeamID:        req.GetTeamId(),
		Status:        req.GetStatus(),
		ContextStatus: req.GetContextStatus(),
		Keyword:       req.GetKeyword(),
		UserID:        req.GetUserId(),
		Limit:         int(req.GetLimit()),
		Offset:        int(req.GetOffset()),
		Page:          int(req.GetPage()),
		PageSize:      int(req.GetPageSize()),
		SortBy:        req.GetSortBy(),
		SortOrder:     req.GetSortOrder(),
	}
}

// getSessionMetrics returns metrics from the session_metrics table if the feature is enabled.
func (s *SessionService) getSessionMetrics(ctx context.Context, sessionID string) *biz.SessionMetrics {
	if !conf.DAOSessionMetricsTable() || s.metricsCache == nil {
		return nil
	}
	metrics, err := s.metricsCache.GetSessionMetrics(ctx, sessionID)
	if err != nil {
		return nil
	}
	return metrics
}

// SearchSessions implements GET /v1/sessions.
func (s *SessionService) SearchSessions(ctx context.Context, req *v1.SearchSessionsRequest) (*v1.SearchSessionsResponse, error) {
	q := searchQueryFromProto(req)
	res, err := s.uc.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &v1.SearchSessionsResponse{
		Total:  int32(res.Total),
		Limit:  int32(res.Limit),
		Offset: int32(res.Offset),
		Items:  make([]*v1.Session, 0, len(res.Items)),
	}
	for i := range res.Items {
		out.Items = append(out.Items, toProtoSession(res.Items[i], s.getSessionMetrics(ctx, res.Items[i].ID)))
	}
	return out, nil
}

// CreateSession implements POST /v1/sessions.
func (s *SessionService) CreateSession(ctx context.Context, req *v1.CreateSessionRequest) (*v1.Session, error) {
	in := biz.Session{
		WorkspaceID:     req.GetWorkspaceId(),
		UserID:          req.GetUserId(),
		OwnerType:       req.GetOwnerType(),
		AgentID:         req.GetAgentId(),
		TeamID:          req.GetTeamId(),
		Title:           req.GetTitle(),
		DialogMode:      req.GetDialogMode(),
		DefaultProvider: req.GetDefaultProvider(),
		DefaultModel:    req.GetDefaultModel(),
		TagsJSON:        req.GetTagsJson(),
		MetadataJSON:    req.GetMetadataJson(),
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.mon.RecordAdminAudit(ctx, "create.session", "session", created.ID, "title="+created.Title)
	return toProtoSession(created, s.getSessionMetrics(ctx, created.ID)), nil
}

// DeleteSessionsByAgent implements DELETE /v1/sessions.
func (s *SessionService) DeleteSessionsByAgent(ctx context.Context, req *v1.DeleteSessionsByAgentRequest) (*emptypb.Empty, error) {
	if err := s.uc.DeleteByAgent(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetSession implements GET /v1/sessions/{id}.
func (s *SessionService) GetSession(ctx context.Context, req *v1.GetSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoSession(out, s.getSessionMetrics(ctx, out.ID)), nil
}

// UpdateSession implements PATCH /v1/sessions/{id}.
func (s *SessionService) UpdateSession(ctx context.Context, req *v1.UpdateSessionRequest) (*v1.Session, error) {
	var fields biz.SessionUpdateFields
	if v := req.GetTitle(); v != "" {
		fields.Title = &v
	}
	if v := req.GetTagsJson(); v != "" {
		fields.TagsJSON = &v
	}
	if v := req.GetVisibility(); v != "" {
		fields.Visibility = &v
	}
	if v := req.GetMetadataJson(); v != "" {
		fields.MetadataJSON = &v
	}
	if v := req.GetDialogMode(); v != "" {
		fields.DialogMode = &v
	}
	if v := req.GetDefaultProvider(); v != "" {
		fields.DefaultProvider = &v
	}
	if v := req.GetDefaultModel(); v != "" {
		fields.DefaultModel = &v
	}
	out, err := s.uc.Update(ctx, req.GetId(), fields)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "update.session", "session", req.GetId(), "fields updated")
	return toProtoSession(out, s.getSessionMetrics(ctx, out.ID)), nil
}

// DeleteSession implements DELETE /v1/sessions/{id}.
func (s *SessionService) DeleteSession(ctx context.Context, req *v1.DeleteSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "delete.session", "session", req.GetId(), "single delete")
	return &emptypb.Empty{}, nil
}

// ArchiveSession implements POST /v1/sessions/{id}/archive.
func (s *SessionService) ArchiveSession(ctx context.Context, req *v1.ArchiveSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Archive(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "archive.session", "session", req.GetId(), "single archive")
	return &emptypb.Empty{}, nil
}

// RestoreSession implements POST /v1/sessions/{id}/restore.
func (s *SessionService) RestoreSession(ctx context.Context, req *v1.RestoreSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Restore(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoSession(out, s.getSessionMetrics(ctx, out.ID)), nil
}

// PinSession implements POST /v1/sessions/{id}/pin.
func (s *SessionService) PinSession(ctx context.Context, req *v1.PinSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Pin(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "pin.session", "session", req.GetId(), "pin")
	return toProtoSession(out, s.getSessionMetrics(ctx, out.ID)), nil
}

// UnpinSession implements POST /v1/sessions/{id}/unpin.
func (s *SessionService) UnpinSession(ctx context.Context, req *v1.UnpinSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Unpin(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	s.mon.RecordAdminAudit(ctx, "unpin.session", "session", req.GetId(), "unpin")
	return toProtoSession(out, s.getSessionMetrics(ctx, out.ID)), nil
}

func toProtoChatMessageRow(m biz.ChatMessage) *v1.ChatMessageRow {
	return &v1.ChatMessageRow{
		Id:               m.ID,
		SessionId:        m.SessionID,
		ParentMessageId:  m.ParentMessageID,
		TurnId:           m.TurnID,
		TurnNumber:       int32(m.TurnNumber),
		SeqInTurn:        int32(m.SeqInTurn),
		Role:             m.Role,
		ContentMarkdown:  m.ContentMarkdown,
		ModelName:        m.ModelName,
		TokenIn:          int32(m.TokenIn),
		TokenOut:         int32(m.TokenOut),
		LatencyMs:        int32(m.LatencyMS),
		Status:           m.Status,
		AttachmentsCount: int32(m.AttachmentsCount),
		OptionsJson:      m.OptionsJSON,
		ErrorMessage:     m.ErrorMessage,
		CreatedAt:        m.CreatedAt,
	}
}

// ListSessionMessages implements GET /v1/sessions/{id}/messages.
func (s *SessionService) ListSessionMessages(ctx context.Context, req *v1.ListSessionMessagesRequest) (*v1.ListSessionMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetId())
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session id is required")
	}
	currentRev, err := s.uc.GetSessionRevision(ctx, sessionID)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	if req.AfterRevision != nil && *req.AfterRevision > 0 {
		items, err := s.uc.ListMessagesAfterRevision(ctx, sessionID, *req.AfterRevision)
		if err != nil {
			return nil, mapSessionErr(err)
		}
		out := make([]*v1.ChatMessageRow, 0, len(items))
		for i := range items {
			out = append(out, toProtoChatMessageRow(items[i]))
		}
		return &v1.ListSessionMessagesResponse{Items: out, Total: int32(len(out)), CurrentRevision: currentRev}, nil
	}
	res, err := s.uc.ListMessagesPaged(ctx, sessionID, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.ChatMessageRow, 0, len(res.Items))
	for i := range res.Items {
		out = append(out, toProtoChatMessageRow(res.Items[i]))
	}
	return &v1.ListSessionMessagesResponse{Items: out, Total: int32(res.Total), CurrentRevision: currentRev}, nil
}

// SearchSessionMessages implements GET /v1/sessions/messages/search.
func (s *SessionService) SearchSessionMessages(ctx context.Context, req *v1.SearchSessionMessagesRequest) (*v1.SearchSessionMessagesResponse, error) {
	result, err := s.uc.SearchMessages(ctx, biz.MessageSearchQuery{
		SessionID: strings.TrimSpace(req.GetSessionId()),
		Keyword:   strings.TrimSpace(req.GetKeyword()),
		Limit:     int(req.GetLimit()),
		Offset:    int(req.GetOffset()),
	})
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.MessageSearchHit, 0, len(result.Items))
	for i := range result.Items {
		h := result.Items[i]
		out = append(out, &v1.MessageSearchHit{
			Id:              h.ID,
			SessionId:       h.SessionID,
			Role:            h.Role,
			ContentMarkdown: h.ContentMarkdown,
			Highlight:       h.Highlight,
			CreatedAt:       h.CreatedAt,
		})
	}
	return &v1.SearchSessionMessagesResponse{Items: out, Total: int32(result.Total)}, nil
}

// GetSessionTimeline implements GET /v1/sessions/{id}/timeline.
func (s *SessionService) GetSessionTimeline(ctx context.Context, req *v1.GetSessionTimelineRequest) (*v1.SessionTimeline, error) {
	q := biz.TimelineQuery{
		Limit:      int(req.GetLimit()),
		Offset:     int(req.GetOffset()),
		KindFilter: req.GetKindFilter(),
		SortOrder:  req.GetSortOrder(),
	}
	out, err := s.uc.Timeline(ctx, req.GetId(), q)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoTimeline(out), nil
}

func toProtoSessionTurn(t biz.SessionTurn) *v1.SessionTurn {
	return &v1.SessionTurn{
		Id:                  t.ID,
		SessionId:           t.SessionID,
		RunId:               t.RunID,
		TurnNumber:         int32(t.TurnNumber),
		UserMessageId:       t.UserMessageID,
		AssistantMessageId:  t.AssistantMessageID,
		OwnerType:           t.OwnerType,
		AgentId:             t.AgentID,
		TeamId:              t.TeamID,
		Status:              t.Status,
		StartedAt:           t.StartedAt,
		EndedAt:             t.EndedAt,
		DurationMs:          int32(t.DurationMs),
		FirstTokenMs:        int32(t.FirstTokenMs),
		ModelCallCount:      int32(t.ModelCallCount),
		ToolCallCount:       int32(t.ToolCallCount),
		SkillCallCount:      int32(t.SkillCallCount),
		McpCallCount:        int32(t.MCPCallCount),
		InputTokens:         int32(t.InputTokens),
		OutputTokens:        int32(t.OutputTokens),
		TotalTokens:         int32(t.TotalTokens),
		TotalCostMicroUsd:   t.TotalCostMicroUSD,
		FinalProvider:       t.FinalProvider,
		FinalModel:          t.FinalModel,
		FinalContentPreview: strutil.ValidUTF8(t.FinalContentPreview),
		ErrorCode:           t.ErrorCode,
		ErrorMessage:        strutil.ValidUTF8(t.ErrorMessage),
		MetadataJson:        t.MetadataJSON,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
	}
}

func (s *SessionService) ListSessionTurns(ctx context.Context, req *v1.ListSessionTurnsRequest) (*v1.ListSessionTurnsResponse, error) {
	res, err := s.uc.ListTurns(ctx, req.GetSessionId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, mapSessionErr(err)
	}
	items := make([]*v1.SessionTurn, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toProtoSessionTurn(res.Items[i]))
	}
	return &v1.ListSessionTurnsResponse{Items: items, Total: int32(res.Total)}, nil
}

func (s *SessionService) CompactSession(ctx context.Context, req *v1.CompactSessionRequest) (*v1.CompactSessionResponse, error) {
	if req.GetSessionId() == "" {
		return nil, apierror.BadRequest("SESSION", "session_id is required")
	}
	result, err := s.compress.CompactSession(ctx, req.GetSessionId(), req.GetPreserveInstruction())
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Compacted {
		return &v1.CompactSessionResponse{Compacted: false}, nil
	}
	return &v1.CompactSessionResponse{
		Compacted:            true,
		FromTurn:             int32(result.FromTurn),
		ToTurn:               int32(result.ToTurn),
		EstimatedTokensBefore: int32(result.EstimatedTokensBefore),
		EstimatedTokensAfter:  int32(result.EstimatedTokensAfter),
		CompressionLevel:      result.CompressionLevel,
	}, nil
}

// GetCompressStatus implements GET /api/v1/sessions/{session_id}/compress-status.
func (s *SessionService) GetCompressStatus(ctx context.Context, req *v1.GetCompressStatusRequest) (*v1.GetCompressStatusReply, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session_id is required")
	}
	status, err := s.compressStatus.CompressStatus(ctx, sessionID)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return &v1.GetCompressStatusReply{Status: status}, nil
}

// ListChildSessions returns child sessions of a parent session (B-2).
func (s *SessionService) ListChildSessions(ctx context.Context, req *v1.ListChildSessionsRequest) (*v1.ListChildSessionsResponse, error) {
	if s == nil || s.uc == nil {
		return nil, apierror.Internal("SESSION", "session service not configured")
	}
	parentSessionID := strings.TrimSpace(req.GetParentSessionId())
	if parentSessionID == "" {
		return nil, apierror.BadRequest("SESSION", "parent_session_id is required")
	}
	sessions, err := s.uc.ListChildSessions(ctx, parentSessionID)
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.Session, 0, len(sessions))
	for i := range sessions {
		out = append(out, toProtoSession(sessions[i], nil))
	}
	return &v1.ListChildSessionsResponse{Sessions: out}, nil
}

// ListActivities returns activities for a session/turn (AF-BE-17).
func (s *SessionService) ListActivities(ctx context.Context, req *v1.ListActivitiesRequest) (*v1.ListActivitiesResponse, error) {
	if s == nil || s.activityReader == nil {
		return nil, apierror.Internal("SESSION", "session service not configured")
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session_id is required")
	}

	var activities []biz.Activity
	var err error
	turnID := strings.TrimSpace(req.GetTurnId())
	if turnID != "" {
		activities, err = s.activityReader.ListBySessionTurn(ctx, sessionID, turnID)
	} else {
		activities, err = s.activityReader.ListBySession(ctx, sessionID)
	}
	if err != nil {
		return nil, mapSessionErr(err)
	}

	out := make([]*v1.Activity, 0, len(activities))
	for i := range activities {
		out = append(out, toProtoActivity(activities[i]))
	}
	return &v1.ListActivitiesResponse{Items: out}, nil
}

func toProtoActivity(a biz.Activity) *v1.Activity {
	var dependsOnJSON string
	if len(a.DependsOn) > 0 {
		if b, err := json.Marshal(a.DependsOn); err == nil {
			dependsOnJSON = string(b)
		}
	} else {
		dependsOnJSON = "[]"
	}
	return &v1.Activity{
		Id:               a.ID,
		Kind:             string(a.Kind),
		Status:           string(a.Status),
		SessionId:        a.SessionID,
		TurnId:           a.TurnID,
		ParentActivityId: a.ParentActivityID,
		Timestamp:        a.Timestamp.Format(time.RFC3339),
		DurationMs:       a.DurationMs,
		Content:          a.Content,
		Reasoning:        a.Reasoning,
		ToolName:         a.ToolName,
		ToolCallId:       a.ToolCallID,
		ToolArguments:    a.ToolArguments,
		ToolResult:       a.ToolResult,
		ToolDurationMs:   a.ToolDurationMs,
		ToolErrorCode:    a.ToolErrorCode,
		ChildBoardId:     a.ChildBoardID,
		SpiritSessionId:  a.SpiritSessionID,
		TeamId:           a.TeamID,
		DagNodeId:        a.DagNodeID,
		DependsOnJson:    dependsOnJSON,
		AgentKey:         a.AgentKey,
		AgentName:        a.AgentName,
		Collapsed:        a.Collapsed,
		Label:            a.Label,
		PromptTokens:     a.PromptTokens,
		CompletionTokens: a.CompletionTokens,
	}
}
