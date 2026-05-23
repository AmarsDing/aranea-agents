package service

import (
	"context"
	"fmt"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ListChatBackgroundJobs aggregates session_runs + channel_turn_job rows (M55 CC-D-01 · CC-R-04).
func (s *ChatService) ListChatBackgroundJobs(ctx context.Context, req *chatv1.ListChatBackgroundJobsRequest) (*chatv1.ListChatBackgroundJobsResponse, error) {
	q := biz.SessionRunListQuery{Limit: int(req.GetLimit())}
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

	out := make([]*chatv1.ChatBackgroundJob, 0)
	if s != nil && s.orch.chTurn.SessionRuns != nil {
		runs, err := s.orch.chTurn.SessionRuns.ListForJobs(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			out = append(out, sessionRunToChatBackgroundJob(r))
		}
	}
	if s != nil && s.orch.chTurn.TurnJobs != nil {
		cq := biz.ChannelTurnJobListQuery{
			SessionID: q.SessionID,
			AgentID:   q.AgentID,
			Status:    q.Status,
			Limit:     q.Limit,
		}
		jobs, err := s.orch.chTurn.TurnJobs.ListFiltered(ctx, cq)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			out = append(out, bizTurnJobToChatBackgroundJob(j))
		}
	}
	return &chatv1.ListChatBackgroundJobsResponse{Items: out}, nil
}

func sessionRunToChatBackgroundJob(r biz.SessionRun) *chatv1.ChatBackgroundJob {
	summary := fmt.Sprintf("Run · %s", biz.NormalizeSessionRunPhase(r.Phase))
	item := &chatv1.ChatBackgroundJob{
		Id:         strutil.ValidUTF8(r.ID),
		Source:     "session_run",
		SessionId:  strutil.ValidUTF8(r.SessionID),
		AgentId:    strutil.ValidUTF8(r.AgentID),
		Status:     biz.NormalizeSessionRunPhase(r.Phase),
		TargetType: "agent_turn",
		TargetId:   strutil.ValidUTF8(r.TurnID),
		CreatedAt:  strutil.ValidUTF8(r.CreatedAt),
		UpdatedAt:  strutil.ValidUTF8(r.UpdatedAt),
		Summary:    &summary,
	}
	phase := biz.NormalizeSessionRunPhase(r.Phase)
	item.Phase = &phase
	turnID := strutil.ValidUTF8(r.TurnID)
	item.TurnId = &turnID
	srid := strutil.ValidUTF8(r.ID)
	item.SessionRunId = &srid
	if msg := strings.TrimSpace(r.ErrorMessage); msg != "" {
		item.ErrorMessage = &msg
	}
	return item
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
