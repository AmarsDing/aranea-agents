package service

import (
	"context"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func (h *ChannelIngress) logTurnFlow(ctx context.Context, sessionID, step, message string, err error, pairs ...event.Pair) {
	if sessionID == "" || h == nil {
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
	var bus event.Bus
	var buf *event.Buffer
	if h.eventBus != nil {
		bus = h.eventBus
	}
	if h.flowBuffer != nil {
		buf = h.flowBuffer
	}
	flow := event.NewFlowLogger(bus, buf, sessionID, "", h.lg)
	if err != nil {
		flow.LogError(step, message, append(pairs, event.P("error", err.Error()))...)
		return
	}
	flow.LogDone(step, message, pairs...)
}
