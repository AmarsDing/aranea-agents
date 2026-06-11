package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
)

func (s *ChatService) SubmitMessageFeedback(ctx context.Context, req *chatv1.SubmitMessageFeedbackRequest) (*chatv1.SubmitMessageFeedbackResponse, error) {
	if s == nil || s.orch == nil || s.orch.td.Sessions == nil {
		return nil, apierror.Internal("CHAT", "session store unavailable")
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	messageID := strings.TrimSpace(req.GetMessageId())
	rating := strings.TrimSpace(strings.ToLower(req.GetRating()))
	comment := strings.TrimSpace(req.GetComment())
	if sessionID == "" || messageID == "" {
		return nil, apierror.BadRequest("CHAT", "session_id and message_id are required")
	}
	if rating != "positive" && rating != "negative" {
		return nil, apierror.BadRequest("CHAT", "rating must be positive or negative")
	}
	if err := s.orch.td.Sessions.UpdateMessageFeedback(ctx, sessionID, messageID, rating, comment); err != nil {
		return nil, err
	}
	if bus := s.orch.td.Pipeline.Bus; bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeUserFeedback, "chat-feedback", sessionID)
		env.Metadata = map[string]any{
			"message_id": messageID,
			"rating":     rating,
			"comment":    comment,
		}
		bus.Publish(ctx, env)
	}
	return &chatv1.SubmitMessageFeedbackResponse{Accepted: true}, nil
}
