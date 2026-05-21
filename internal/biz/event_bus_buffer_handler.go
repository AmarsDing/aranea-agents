package biz

import (
	"aranea-agents/internal/event"
)

// eventBufferHandler appends envelopes to the in-process replay buffer.
type eventBufferHandler struct {
	buffer *event.Buffer
}

func newEventBufferHandler(buffer *event.Buffer) *eventBufferHandler {
	return &eventBufferHandler{buffer: buffer}
}

func (h *eventBufferHandler) Handle(env event.Envelope) {
	if h == nil || h.buffer == nil {
		return
	}
	h.buffer.Append(env)
}
