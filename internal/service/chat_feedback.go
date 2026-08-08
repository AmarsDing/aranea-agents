package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func (s *ChatService) SubmitMessageFeedback(ctx context.Context, req *chatv1.SubmitMessageFeedbackRequest) (*chatv1.SubmitMessageFeedbackResponse, error) {
	if s == nil || s.orch == nil || s.orch.td().Sessions == nil {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable")
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	messageID := strings.TrimSpace(req.GetMessageId())
	rating := strings.TrimSpace(strings.ToLower(req.GetRating()))
	comment := strings.TrimSpace(req.GetComment())
	if sessionID == "" || messageID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id and message_id are required")
	}
	if rating != "positive" && rating != "negative" {
		return nil, apierror.BadRequest(apierror.DomainChat, "rating must be positive or negative")
	}
	if err := s.orch.td().Sessions.UpdateMessageFeedback(ctx, sessionID, messageID, rating, comment); err != nil {
		return nil, err
	}
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		meta := map[string]any{
			"message_id": messageID,
			"rating":     rating,
			"comment":    comment,
		}
		// P1-2: context snapshot (input/output/task_id) keeps the feedback
		// review list self-contained even if the task is later deleted.
		if ctxJSON := strings.TrimSpace(req.GetContextJson()); ctxJSON != "" {
			meta["context_json"] = ctxJSON
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "user_feedback", "", meta))
	}
	return &chatv1.SubmitMessageFeedbackResponse{Accepted: true}, nil
}
