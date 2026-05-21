package biz

import (
	"context"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

// EventBusSideConsumers runs typed EventBus subscriptions (P3 + FlowLog persist).
type EventBusSideConsumers struct {
	toolCall     *toolCallConsumer
	callback     *callbackConsumer
	messageStore *messageStoreConsumer
	flowLog      *flowLogPersistConsumer
}

func NewEventBusSideConsumers(
	bus event.Bus,
	tools *ToolUsecase,
	webhooks *WebhookDispatcher,
	sessions *SessionUsecase,
	flowLogs *FlowLogUsecase,
) *EventBusSideConsumers {
	if bus == nil {
		return nil
	}
	return &EventBusSideConsumers{
		toolCall:     newToolCallConsumer(bus, tools),
		callback:     newCallbackConsumer(bus, webhooks),
		messageStore: newMessageStoreConsumer(bus, sessions),
		flowLog:      newFlowLogPersistConsumer(bus, flowLogs),
	}
}

func (c *EventBusSideConsumers) Start(ctx context.Context) {
	if c == nil {
		return
	}
	if c.toolCall != nil {
		c.toolCall.Start(ctx)
	}
	if c.callback != nil {
		c.callback.Start(ctx)
	}
	if c.messageStore != nil {
		c.messageStore.Start(ctx)
	}
	if c.flowLog != nil {
		c.flowLog.Start(ctx)
	}
}

func runTypedConsumer(ctx context.Context, name string, bus event.Bus, opts event.SubscribeOptions, fn func(context.Context, event.Envelope)) {
	if bus == nil || fn == nil {
		return
	}
	worker := newAsyncEnvelopeWorker(name, sideConsumerQueueSize())
	worker.Start(ctx, fn)
	ch, unsub := bus.Subscribe(opts)
	safego.Go(ctx, name, func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				worker.Offer(ctx, env)
			}
		}
	})
}
