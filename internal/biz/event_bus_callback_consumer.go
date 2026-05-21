package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
)

// callbackConsumer dispatches outbound webhooks on terminal run_status envelopes.
type callbackConsumer struct {
	bus      event.Bus
	webhooks *WebhookDispatcher
}

func newCallbackConsumer(bus event.Bus, webhooks *WebhookDispatcher) *callbackConsumer {
	if webhooks == nil {
		return nil
	}
	return &callbackConsumer{bus: bus, webhooks: webhooks}
}

func terminalRunStatuses() map[string]struct{} {
	return map[string]struct{}{
		"completed":  {},
		"failed":     {},
		"cancelled":  {},
		"canceled":   {},
	}
}

func (c *callbackConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumer(ctx, "event-bus-callback", c.bus, event.SubscribeOptions{
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeRunStatus},
		BufferSize: 128,
		Reliable:   true,
	}, c.handle)
}

func (c *callbackConsumer) handle(ctx context.Context, env event.Envelope) {
	if c == nil || c.webhooks == nil || env.Metadata == nil {
		return
	}
	status, _ := env.Metadata["status"].(string)
	status = strings.TrimSpace(status)
	if status == "" {
		return
	}
	if _, ok := terminalRunStatuses()[strings.ToLower(status)]; !ok {
		return
	}
	runID, _ := env.Metadata["run_id"].(string)
	errMsg, _ := env.Metadata["error_message"].(string)
	eventType := RunStatusToWebhookEvent(status)
	if eventType == "" {
		return
	}
	data := map[string]any{}
	if msg := strings.TrimSpace(errMsg); msg != "" {
		data["error_message"] = msg
	}
	c.webhooks.Dispatch(ctx, eventType, runID, env.SessionID, status, data)
}
