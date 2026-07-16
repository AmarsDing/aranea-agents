package biz

import (
	"context"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

// EventBusSideConsumers runs typed event subscriptions for side effects
// (webhooks, flow-log persistence, user-feedback recording, usage rollup).
//
// Phase 5 Blocker B: migrated from legacy Envelope-based SessionBus to
// typed ActivityEventBus (chat business events) and MonitorBus (monitor
// events). Each consumer subscribes to exactly one bus and filters at the
// bus level to avoid queue pressure from non-matching events.
type EventBusSideConsumers struct {
	callback        *callbackConsumer
	flowLog         *flowLogPersistConsumer
	userFeedback    *userFeedbackConsumer
	usageRollup     *usageRollupConsumer
	webhooks        *WebhookDispatcher
	traceProj       *monitor.TraceProjector
	fileAppender    *monitor.FlowFileAppender
	monitorEventBus contract.MonitorBus
	logger          SessionLogWriter
}

func NewEventBusSideConsumers(
	eventBus EventBus,
	monitorEventBus contract.MonitorBus,
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
	if eventBus == nil && monitorEventBus == nil {
		return nil
	}
	return &EventBusSideConsumers{
		callback:        newCallbackConsumer(eventBus, webhooks, logger),
		flowLog:         newFlowLogPersistConsumer(flowLogs, logger, monitorEventBus),
		userFeedback:    newUserFeedbackConsumer(eventBus, monitorUC, memWorker, logger),
		usageRollup:     newUsageRollupConsumer(eventBus, usage, logger),
		webhooks:        webhooks,
		traceProj:       traceProj,
		fileAppender:    fileAppender,
		monitorEventBus: monitorEventBus,
		logger:          logger,
	}
}

func (c *EventBusSideConsumers) Start(ctx context.Context) {
	if c == nil {
		return
	}
	if c.callback != nil {
		c.callback.Start(ctx)
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
		c.fileAppender.Start(ctx, c.monitorEventBus)
	}
}

// activityLogFields extracts identifying fields from an ActivityEvent for
// queue-full warning logging.
func activityLogFields(ev ActivityEvent) (sessionID, typeName, eventID string) {
	return ev.Activity.SessionID, string(ev.Event), ev.Activity.ID
}

// monitorLogFields extracts identifying fields from a MonitorEvent for
// queue-full warning logging.
func monitorLogFields(ev contract.MonitorEvent) (sessionID, typeName, eventID string) {
	return ev.SessionID, string(ev.Type), ev.ID
}

// runSystemNoticeConsumerWithOpts subscribes to v2 EventBus, filters
// SystemNoticeEvent payloads, and dispatches matches to fn via an async worker.
func runSystemNoticeConsumerWithOpts(
	ctx context.Context,
	name string,
	bus EventBus,
	filter func(*SystemNoticeEvent) bool,
	fn func(context.Context, *SystemNoticeEvent),
	offerOpts offerOption[*SystemNoticeEvent],
	logger SessionLogWriter,
) {
	if bus == nil || fn == nil {
		return
	}
	logFields := func(ev *SystemNoticeEvent) (sessionID, typeName, eventID string) {
		if ev == nil {
			return "", "", ""
		}
		return ev.SpiritSessionID(), ev.NoticeType, ev.SpiritSessionID()
	}
	worker := newAsyncEventWorker(name, sideConsumerQueueSize(), 0, logger, logFields)
	worker.Start(ctx, fn)
	ch, unsub := bus.Subscribe(EventSubscribeOptions{})
	safego.Go(ctx, name, func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				notice, ok := e.(*SystemNoticeEvent)
				if !ok {
					continue
				}
				if filter != nil && !filter(notice) {
					continue
				}
				worker.OfferWithOptions(ctx, notice, offerOpts)
			}
		}
	})
}

// runMonitorConsumer subscribes to MonitorBus and dispatches matching events
// to fn via an async worker with bounded queue.
func runMonitorConsumer(ctx context.Context, name string, bus contract.MonitorBus, opts contract.MonitorSubscribeOptions, fn func(context.Context, contract.MonitorEvent), logger SessionLogWriter) {
	runMonitorConsumerWithOpts(ctx, name, bus, opts, fn, offerOption[contract.MonitorEvent]{}, logger)
}

func runMonitorConsumerWithOpts(ctx context.Context, name string, bus contract.MonitorBus, opts contract.MonitorSubscribeOptions, fn func(context.Context, contract.MonitorEvent), offerOpts offerOption[contract.MonitorEvent], logger SessionLogWriter) {
	if bus == nil || fn == nil {
		return
	}
	worker := newAsyncEventWorker(name, sideConsumerQueueSize(), 0, logger, monitorLogFields)
	worker.Start(ctx, fn)
	ch, unsub := bus.Subscribe(opts)
	safego.Go(ctx, name, func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				worker.OfferWithOptions(ctx, ev, offerOpts)
			}
		}
	})
}
