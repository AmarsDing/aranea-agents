package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ListChatBackgroundJobs aggregates channel_turn_job rows for the chat jobs panel (M55 CC-D-01).
func (s *ChatService) ListChatBackgroundJobs(ctx context.Context, req *chatv1.ListChatBackgroundJobsRequest) (*chatv1.ListChatBackgroundJobsResponse, error) {
	if s == nil || s.turnJobs == nil {
		return &chatv1.ListChatBackgroundJobsResponse{}, nil
	}
	q := biz.ChannelTurnJobListQuery{
		Limit: int(req.GetLimit()),
	}
	if req.SessionId != nil {
		q.SessionID = strings.TrimSpace(*req.SessionId)
	}
	if req.AgentId != nil {
		q.AgentID = strings.TrimSpace(*req.AgentId)
	}
	if req.Status != nil {
		q.Status = strings.TrimSpace(*req.Status)
	}
	if q.SessionID == "" && q.AgentID == "" {
		return nil, kerrors.BadRequest("CHAT_JOBS", "session_id or agent_id is required")
	}
	jobs, err := s.turnJobs.ListFiltered(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.ChatBackgroundJob, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, bizTurnJobToChatBackgroundJob(j))
	}
	return &chatv1.ListChatBackgroundJobsResponse{Items: out}, nil
}

func bizTurnJobToChatBackgroundJob(j biz.ChannelTurnJob) *chatv1.ChatBackgroundJob {
	summary := strings.TrimSpace(strutil.ValidUTF8(j.ContentPreview))
	errMsg := strings.TrimSpace(strutil.ValidUTF8(j.ErrorMessage))
	graphID := strings.TrimSpace(strutil.ValidUTF8(j.GraphID))
	item := &chatv1.ChatBackgroundJob{
		Id:         strutil.ValidUTF8(j.ID),
		Source:     "channel",
		SessionId:  strutil.ValidUTF8(j.SessionID),
		AgentId:    strings.TrimSpace(strutil.ValidUTF8(j.AgentID)),
		Status:     biz.NormalizeChannelTurnJobStatus(j.Status),
		TargetType: strings.TrimSpace(strutil.ValidUTF8(j.AsyncTargetType)),
		TargetId:   strings.TrimSpace(strutil.ValidUTF8(j.AsyncTargetID)),
		CreatedAt:  strutil.ValidUTF8(j.CreatedAt),
		UpdatedAt:  strutil.ValidUTF8(j.UpdatedAt),
		ChannelId:  strutil.ValidUTF8(j.ChannelID),
	}
	if summary != "" {
		item.Summary = &summary
	}
	if errMsg != "" {
		item.ErrorMessage = &errMsg
	}
	if graphID != "" {
		item.GraphId = &graphID
	}
	return item
}
