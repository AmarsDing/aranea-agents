package event

import (
	"context"
)

// TurnObserver is the unified publish hook for chat-visible envelopes produced during a turn.
// FlowLog / TraceEmitter steps remain on TraceEmitter; runner_completion side effects
// (usage, memory, monitor) stay on EventBusConsumer — this type covers the chat channel fan-out.
type TurnObserver struct {
	bus Bus
}

// NewTurnObserver returns a turn-scoped observer backed by the shared EventBus.
func NewTurnObserver(bus Bus) *TurnObserver {
	return &TurnObserver{bus: bus}
}

// PublishChat delivers one or more envelopes to EventBus subscribers (WS chat channel).
func (o *TurnObserver) PublishChat(ctx context.Context, envs ...Envelope) {
	if o == nil || o.bus == nil {
		return
	}
	for _, env := range envs {
		o.bus.Publish(ctx, env)
	}
}
