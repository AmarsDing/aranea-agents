package service

import (
	"context"
	"database/sql"
	stderrors "errors"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// SessionService implements kratos session.v1.
type SessionService struct {
	v1.UnimplementedSessionServiceServer

	uc *biz.SessionUsecase
}

// NewSessionService constructs the service.
func NewSessionService(uc *biz.SessionUsecase) *SessionService {
	return &SessionService{uc: uc}
}

func toProtoSession(s biz.Session) *v1.Session {
	return &v1.Session{
		Id:                      s.ID,
		OwnerType:               s.OwnerType,
		AgentId:                 s.AgentID,
		TeamId:                  s.TeamID,
		Title:                   s.Title,
		Summary:                 s.Summary,
		ContextUsedRatio:        s.ContextUsedRatio,
		ContextUsedTokens:       int32(s.ContextUsedTokens),
		MaxContextUsedRatio:     s.MaxContextUsedRatio,
		LastContextWindowTokens: int32(s.LastContextWindowTokens),
		ContextStatus:           s.ContextStatus,
		DialogMode:              s.DialogMode,
		Provider:                s.Provider,
		Model:                   s.Model,
		Status:                  s.Status,
		MessageCount:            int32(s.MessageCount),
		RunCount:                int32(s.RunCount),
		ModelCallCount:          int32(s.ModelCallCount),
		ToolCallCount:           int32(s.ToolCallCount),
		SkillCallCount:          int32(s.SkillCallCount),
		McpCallCount:            int32(s.MCPCallCount),
		InputTokens:             int32(s.InputTokens),
		OutputTokens:            int32(s.OutputTokens),
		TotalTokens:             int32(s.TotalTokens),
		TotalCostMicroUsd:       s.TotalCostMicroUSD,
		LastMessageAt:           s.LastMessageAt,
		CreatedAt:               s.CreatedAt,
		UpdatedAt:               s.UpdatedAt,
		ArchivedAt:              s.ArchivedAt,
		DeletedAt:               s.DeletedAt,
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
		Limit:         int(req.GetLimit()),
		Offset:        int(req.GetOffset()),
		Page:          int(req.GetPage()),
		PageSize:      int(req.GetPageSize()),
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
		OwnerType:  req.GetOwnerType(),
		AgentID:    req.GetAgentId(),
		TeamID:     req.GetTeamId(),
		Title:      req.GetTitle(),
		DialogMode: req.GetDialogMode(),
		Provider:   req.GetProvider(),
		Model:      req.GetModel(),
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
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
	out, err := s.uc.Rename(ctx, req.GetId(), req.GetTitle())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoSession(out), nil
}

// DeleteSession implements DELETE /v1/sessions/{id}.
func (s *SessionService) DeleteSession(ctx context.Context, req *v1.DeleteSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ArchiveSession implements POST /v1/sessions/{id}/archive.
func (s *SessionService) ArchiveSession(ctx context.Context, req *v1.ArchiveSessionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Archive(ctx, req.GetId()); err != nil {
		return nil, mapSessionErr(err)
	}
	return &emptypb.Empty{}, nil
}

func toProtoChatMessageRow(m biz.ChatMessage) *v1.ChatMessageRow {
	return &v1.ChatMessageRow{
		Id:                 m.ID,
		SessionId:          m.SessionID,
		ParentMessageId:    m.ParentMessageID,
		TurnIndex:          int32(m.TurnIndex),
		Role:               m.Role,
		ContentMarkdown:    m.ContentMarkdown,
		ModelName:          m.ModelName,
		TokenIn:            int32(m.TokenIn),
		TokenOut:           int32(m.TokenOut),
		LatencyMs:          int32(m.LatencyMS),
		Status:             m.Status,
		AttachmentsCount:   int32(m.AttachmentsCount),
		OptionsJson:        m.OptionsJSON,
		ErrorMessage:       m.ErrorMessage,
		CreatedAt:          m.CreatedAt,
	}
}

// ListSessionMessages implements GET /v1/sessions/{id}/messages.
func (s *SessionService) ListSessionMessages(ctx context.Context, req *v1.ListSessionMessagesRequest) (*v1.ListSessionMessagesResponse, error) {
	rows, err := s.uc.ListMessages(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make([]*v1.ChatMessageRow, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoChatMessageRow(rows[i]))
	}
	return &v1.ListSessionMessagesResponse{Items: out}, nil
}

// GetSessionTimeline implements GET /v1/sessions/{id}/timeline.
func (s *SessionService) GetSessionTimeline(ctx context.Context, req *v1.GetSessionTimelineRequest) (*v1.SessionTimeline, error) {
	out, err := s.uc.Timeline(ctx, req.GetId())
	if err != nil {
		return nil, mapSessionErr(err)
	}
	return toProtoTimeline(out), nil
}
