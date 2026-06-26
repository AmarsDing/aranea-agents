package biz

import (
	"context"
	"strings"
)

// callbackConsumer dispatches outbound webhooks on terminal run_status
// ActivityEvents.
//
// Phase 5 Blocker B: migrated from legacy Envelope-based SessionBus to
// ActivityEventBus. The run_status publisher (service.PublishRunStatusFull)
// emits ActivityEvent{Stage:"run_status", Meta:{"status","run_id","error_message"}}.
// This consumer filters at the bus level by Stage=="run_status" and extracts
// the same fields from Activity.Meta.
type callbackConsumer struct {
	bus      ActivityEventBus
	webhooks *WebhookDispatcher
	logger   SessionLogWriter
}

func newCallbackConsumer(bus ActivityEventBus, webhooks *WebhookDispatcher, logger SessionLogWriter) *callbackConsumer {
	if webhooks == nil {
		return nil
	}
	return &callbackConsumer{bus: bus, webhooks: webhooks, logger: logger}
}

func terminalRunStatuses() map[string]struct{} {
	return map[string]struct{}{
		"completed": {},
		"failed":    {},
		"cancelled": {},
		"canceled":  {},
	}
}

func (c *callbackConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runActivityConsumerWithOpts(ctx, "event-bus-callback", c.bus, ActivityEventSubscribeOptions{
		BufferSize: 128,
		GlobalMode: true,
		Filter: func(ev ActivityEvent) bool {
			return ev.Activity.Stage == "run_status"
		},
	}, c.handle, offerOption[ActivityEvent]{FallbackSync: true, FallbackFn: c.handle}, c.logger)
}

func (c *callbackConsumer) handle(ctx context.Context, ev ActivityEvent) {
	if c == nil || c.webhooks == nil {
		return
	}
	meta := ev.Activity.Meta
	if meta == nil {
		return
	}
	status, _ := meta["status"].(string)
	status = strings.TrimSpace(status)
	if status == "" {
		return
	}
	if _, ok := terminalRunStatuses()[strings.ToLower(status)]; !ok {
		return
	}
	runID, _ := meta["run_id"].(string)
	errMsg, _ := meta["error_message"].(string)
	eventType := RunStatusToWebhookEvent(status)
	if eventType == "" {
		return
	}
	data := map[string]any{}
	if msg := strings.TrimSpace(errMsg); msg != "" {
		data["error_message"] = msg
	}
	c.webhooks.Dispatch(ctx, eventType, runID, ev.Activity.SessionID, status, data)
}
