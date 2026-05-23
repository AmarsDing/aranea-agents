package biz

import (
	"aranea-agents/internal/event/contract"
)

// eventBufferHandler appends envelopes to the in-process replay buffer.
type eventBufferHandler struct {
	buffer EnvelopeBuffer
}

func newEventBufferHandler(buffer EnvelopeBuffer) *eventBufferHandler {
	return &eventBufferHandler{buffer: buffer}
}

func (h *eventBufferHandler) Handle(env contract.Envelope) {
	if h == nil || h.buffer == nil {
		return
	}
	h.buffer.Append(env)
}
