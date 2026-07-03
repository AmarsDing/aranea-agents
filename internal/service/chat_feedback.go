package service

import (
	"context"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
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
	// Phase 3b-D: migrated to v2 EventBus via ActivityBridgeEvent.
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		bus.Publish(ctx, biz.NewActivityBridgeEvent(biz.ActivityEvent{
			Event: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:        uuid.NewString(),
				Kind:      biz.ActivityKindNotice,
				SessionID: sessionID,
				Timestamp: time.Now().UTC(),
				Meta: map[string]any{
					"notice_type": "user_feedback",
					"message_id":  messageID,
					"rating":      rating,
					"comment":     comment,
				},
			},
			Domain: biz.ActivityDomainChat,
		}))
	}
	return &chatv1.SubmitMessageFeedbackResponse{Accepted: true}, nil
}
