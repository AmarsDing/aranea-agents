package service

import (
	"context"
	"database/sql"
	stderrors "errors"
	"strings"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// SessionService implements kratos session.v1.
type SessionService struct {
	v1.UnimplementedSessionServiceServer

	uc    *biz.SessionUsecase
	mon   *biz.MonitorUsecase
	runs  *biz.SessionRunUsecase
}

func NewSessionService(
	uc *biz.SessionUsecase,
	mon *biz.MonitorUsecase,
	runs *biz.SessionRunUsecase,
) *SessionService {
	return &SessionService{uc: uc, mon: mon, runs: runs}
}

func toProtoSession(s biz.Session) *v1.Session {
	return &v1.Session{
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
	}
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
	if stderrors.Is(err, sql.ErrNoRows) {
		return kerrors.NotFound("SESSION", "session not found")
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
		out.Items = append(out.Items, toProtoSession(res.Items[i]))
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
	biz.RecordAdminAudit(ctx, s.mon, "create.session", "session", created.ID, "title="+created.Title)
	return toProtoSession(created), nil
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
	return toProtoSession(out), nil
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
	biz.RecordAdminAudit(ctx, s.mon, "update.session", "session", req.GetId(), "fields updated")
	return toProtoSession(out), nil
}

// DeleteSession implements DELETE /v1/sessions/{id}.
func (s *SessionService) DeleteSession(ctx context.Context, req *v1.DeleteSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	biz.RecordAdminAudit(ctx, s.mon, "delete.session", "session", req.GetId(), "single delete")
	return &emptypb.Empty{}, nil
}

// ArchiveSession implements POST /v1/sessions/{id}/archive.
func (s *SessionService) ArchiveSession(ctx context.Context, req *v1.ArchiveSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Archive(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	biz.RecordAdminAudit(ctx, s.mon, "archive.session", "session", req.GetId(), "single archive")
	return &emptypb.Empty{}, nil
}

// RestoreSession implements POST /v1/sessions/{id}/restore.
func (s *SessionService) RestoreSession(ctx context.Context, req *v1.RestoreSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Restore(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoSession(out), nil
}

// PinSession implements POST /v1/sessions/{id}/pin.
func (s *SessionService) PinSession(ctx context.Context, req *v1.PinSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Pin(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	biz.RecordAdminAudit(ctx, s.mon, "pin.session", "session", req.GetId(), "pin")
	return toProtoSession(out), nil
}

// UnpinSession implements POST /v1/sessions/{id}/unpin.
func (s *SessionService) UnpinSession(ctx context.Context, req *v1.UnpinSessionRequest) (*v1.Session, error) {
	out, err := s.uc.Unpin(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	biz.RecordAdminAudit(ctx, s.mon, "unpin.session", "session", req.GetId(), "unpin")
	return toProtoSession(out), nil
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
		return nil, kerrors.BadRequest("SESSION", "session id is required")
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
