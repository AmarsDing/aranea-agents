package biz

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

// EventBusSideConsumers runs typed EventBus subscriptions (P3 + FlowLog persist).
type EventBusSideConsumers struct {
	toolCall     *toolCallConsumer
	callback     *callbackConsumer
	messageStore *messageStoreConsumer
	flowLog      *flowLogPersistConsumer
	userFeedback *userFeedbackConsumer
	webhooks     *WebhookDispatcher
	logger       SessionLogWriter
}

func NewEventBusSideConsumers(
	sessionBus contract.Bus,
	monitorBus contract.Bus,
	tools *ToolUsecase,
	webhooks *WebhookDispatcher,
	sessions *SessionUsecase,
	flowLogs *FlowLogUsecase,
	monitor *MonitorUsecase,
	memWorker *TurnMemoryWorker,
) *EventBusSideConsumers {
	if sessionBus == nil {
		return nil
	}
	return &EventBusSideConsumers{
		toolCall:     newToolCallConsumer(sessionBus, tools),
		callback:     newCallbackConsumer(sessionBus, webhooks),
		messageStore: newMessageStoreConsumer(sessionBus, sessions),
		flowLog:      newFlowLogPersistConsumer(flowLogs, sessionBus, monitorBus),
		userFeedback: newUserFeedbackConsumer(sessionBus, monitor, memWorker),
		webhooks:     webhooks,
	}
}

func (c *EventBusSideConsumers) SetLogger(logger SessionLogWriter) {
	c.logger = logger
	if c.toolCall != nil {
		c.toolCall.logger = logger
	}
	if c.flowLog != nil {
		c.flowLog.logger = logger
	}
	if c.messageStore != nil {
		c.messageStore.logger = logger
	}
	if c.userFeedback != nil {
		c.userFeedback.logger = logger
	}
	if c.webhooks != nil {
		c.webhooks.SetLogger(logger)
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
	if c.userFeedback != nil {
		c.userFeedback.Start(ctx)
	}
}

func runTypedConsumer(ctx context.Context, name string, bus contract.Bus, opts contract.SubscribeOptions, fn func(context.Context, contract.Envelope)) {
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
