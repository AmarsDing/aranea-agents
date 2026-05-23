package service

import (
	"context"

	"aranea-agents/internal/event"
)

func (h *ChannelIngress) logTurnFlow(ctx context.Context, sessionID, step, message string, err error, pairs ...event.Pair) {
	if sessionID == "" || h == nil {
		if err != nil {
			event.SysLogWarn(step, message, pairs...)
		} else {
			event.SysLogInfo(step, message, pairs...)
		}
		return
	}
	var bus event.Bus
	var buf *event.Buffer
	if h.eventBus != nil {
		bus = h.eventBus
	}
	if h.chat != nil {
		buf = h.chat.ChannelFlowBuffer()
	}
	flow := event.NewFlowLogger(bus, buf, sessionID, "")
	if err != nil {
		flow.LogError(step, message, pairs...)
		event.SysLogWarn(step, message, append(pairs, event.P("error", err.Error()))...)
		return
	}
	flow.LogDone(step, message, pairs...)
}
