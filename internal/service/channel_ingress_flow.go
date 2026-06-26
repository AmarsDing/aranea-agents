package service

import (
	"context"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func (h *ChannelIngress) logTurnFlow(ctx context.Context, sessionID, step, message string, err error, pairs ...event.Pair) {
	if h == nil {
		return
	}
	if sessionID == "" {
		if err != nil {
			h.lg.Warn(message,
				loggateway.StepID(step),
				loggateway.Err(err),
			)
		} else {
			h.lg.Info(message,
				loggateway.StepID(step),
			)
		}
		return
	}
	flow := event.NewFlowLogger(sessionID, "", h.lg)
	if err != nil {
		flow.LogError(step, message, append(pairs, event.P("error", err.Error()))...)
		return
	}
	flow.LogDone(step, message, pairs...)
}
