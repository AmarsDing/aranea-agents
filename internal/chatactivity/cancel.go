package chatactivity

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// CancelRunningActivityMessages marks in-flight tool_running cards as cancelled when the user stops generation.
func CancelRunningActivityMessages(ctx context.Context, sessions *biz.SessionUsecase, sessionID string, lg loggateway.Logger) (int, error) {
	if sessions == nil {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	msgs, err := sessions.ListMessagesByStatus(ctx, sessionID, "tool_running", biz.ActivityCancelScanLimit)
	if err != nil {
		return 0, err
	}
	cancelled := 0
	for _, msg := range msgs {
		if strings.TrimSpace(msg.Status) != "tool_running" {
			continue
		}
		next, ok := chatagent.CancelledActivityMessage(msg)
		if !ok {
			continue
		}
		if err := sessions.UpsertChatActivityMessage(ctx, sessionID, next); err != nil {
			lg.Warn("取消执行卡片落库失败",
				loggateway.StepID("chat.activity.cancel"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("message_id", next.ID),
				loggateway.Err(err))
			continue
		}
		cancelled++
	}
	return cancelled, nil
}
