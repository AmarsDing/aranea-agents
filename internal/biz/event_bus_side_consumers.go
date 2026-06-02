package biz

import (
	"context"

	"aranea-agents/internal/biz/monitor"
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
	usageRollup  *usageRollupConsumer
	webhooks     *WebhookDispatcher
	traceProj    *monitor.TraceProjector
	fileAppender *monitor.FlowFileAppender
	logger       SessionLogWriter
}

func NewEventBusSideConsumers(
	sessionBus contract.Bus,
	monitorBus contract.Bus,
	tools *ToolUsecase,
	webhooks *WebhookDispatcher,
	sessions *SessionUsecase,
	flowLogs *FlowLogUsecase,
	monitorUC *MonitorUsecase,
	memWorker *TurnMemoryWorker,
	traceProj *monitor.TraceProjector,
	fileAppender *monitor.FlowFileAppender,
	usage *UsageUsecase,
	logger SessionLogWriter,
) *EventBusSideConsumers {
	if sessionBus == nil {
		return nil
	}
	return &EventBusSideConsumers{
		toolCall:     newToolCallConsumer(sessionBus, tools, logger),
		callback:     newCallbackConsumer(sessionBus, webhooks, logger),
		messageStore: newMessageStoreConsumer(sessionBus, sessions, logger),
		flowLog:      newFlowLogPersistConsumer(flowLogs, logger, sessionBus, monitorBus),
		userFeedback: newUserFeedbackConsumer(sessionBus, monitorUC, memWorker, logger),
		usageRollup:  newUsageRollupConsumer(sessionBus, usage, logger),
		webhooks:     webhooks,
		traceProj:    traceProj,
		fileAppender: fileAppender,
		logger:       logger,
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
	if c.usageRollup != nil {
		c.usageRollup.Start(ctx)
	}
	if c.traceProj != nil {
		c.traceProj.Start(ctx)
	}
	if c.fileAppender != nil {
		c.fileAppender.Start(ctx, c.fileAppenderBuses()...)
	}
}

func (c *EventBusSideConsumers) fileAppenderBuses() []contract.Bus {
	var buses []contract.Bus
	if c.flowLog != nil {
		for _, b := range c.flowLog.buses {
			buses = append(buses, b)
		}
	}
	return buses
}

func runTypedConsumer(ctx context.Context, name string, bus contract.Bus, opts contract.SubscribeOptions, fn func(context.Context, contract.Envelope), logger SessionLogWriter) {
	runTypedConsumerWithOpts(ctx, name, bus, opts, fn, OfferOption{}, logger)
}

func runTypedConsumerWithOpts(ctx context.Context, name string, bus contract.Bus, opts contract.SubscribeOptions, fn func(context.Context, contract.Envelope), offerOpts OfferOption, logger SessionLogWriter) {
	if bus == nil || fn == nil {
		return
	}
	worker := newAsyncEnvelopeWorker(name, sideConsumerQueueSize(), 0, logger)
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
				worker.OfferWithOptions(ctx, env, offerOpts)
			}
		}
	})
}
