package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/event/contract"
)

// callbackConsumer dispatches outbound webhooks on terminal run_status envelopes.
type callbackConsumer struct {
	bus      contract.Bus
	webhooks *WebhookDispatcher
	logger   SessionLogWriter
}

func newCallbackConsumer(bus contract.Bus, webhooks *WebhookDispatcher, logger SessionLogWriter) *callbackConsumer {
	if webhooks == nil {
		return nil
	}
	return &callbackConsumer{bus: bus, webhooks: webhooks, logger: logger}
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
	runTypedConsumerWithOpts(ctx, "event-bus-callback", c.bus, contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{contract.EnvelopeTypeRunStatus},
		BufferSize: 128,
		Reliable:   true,
	}, c.handle, OfferOption{FallbackSync: true, FallbackFn: c.handle}, c.logger)
}

func (c *callbackConsumer) handle(ctx context.Context, env contract.Envelope) {
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
