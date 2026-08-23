package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// autoTitleRunner is the narrow port consumed by the session auto-title
// subscriber. *session.SessionUsecase satisfies it via the facade method
// AutoTitleFromUserMessage (kept narrow so tests can fake it).
type autoTitleRunner interface {
	AutoTitleFromUserMessage(ctx context.Context, sessionID, content string) error
}

// startSessionAutoTitleSubscriber restores session auto-titling for the v2
// native chat path (BUG-01, chat-e2e-20260823). In v2, user messages persist
// via ActivityProjector → task.created events, bypassing the legacy
// AppendChatTurn/AppendChatMessage hooks where auto-title used to trigger.
//
// ActivityProjector.OnTurnStart only emits task.created for root turns
// (TeamStageID == ""), so team member sub-sessions are naturally excluded.
// The bus is full-broadcast (V2Bus) — filtering happens here by event type.
// Title failures are Warn-only and must never affect the chat path.
func startSessionAutoTitleSubscriber(ctx context.Context, bus biz.EventBus, titler autoTitleRunner, lg loggateway.Logger) {
	if bus == nil || titler == nil {
		return
	}
	ch, cancel := bus.Subscribe(biz.EventSubscribeOptions{})
	safego.Go(ctx, "session-auto-title-subscriber", func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				te, ok := ev.(*biz.TaskCreatedEvent)
				if !ok || te == nil {
					continue
				}
				sessionID := te.Task.SessionID
				content := te.Task.UserMessage
				if sessionID == "" || content == "" {
					continue
				}
				// Handle synchronously in the drain loop: handling is one DB
				// read (fast no-op for already-titled sessions), and serial
				// processing lets the first message's snippet rename gate out
				// duplicate triggers for rapid follow-ups.
				if err := titler.AutoTitleFromUserMessage(ctx, sessionID, content); err != nil {
					lg.Warn("auto title from task.created failed",
						loggateway.StepID("session.auto_title"),
						loggateway.SessionID(sessionID),
						loggateway.Err(err))
				}
			}
		}
	})
}
