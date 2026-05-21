package chatactivity

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// CancelRunningActivityMessages marks in-flight tool_running cards as cancelled when the user stops generation.
func CancelRunningActivityMessages(ctx context.Context, sessions *biz.SessionUsecase, sessionID string) (int, error) {
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
			event.CtxFlowLogWarn(ctx, "chat.activity.cancel", "取消执行卡片落库失败",
				event.P("session_id", sessionID),
				event.P("message_id", next.ID),
				event.P("error", err.Error()),
			)
			continue
		}
		cancelled++
	}
	return cancelled, nil
}
