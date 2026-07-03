package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/safego"
)

// callbackConsumer dispatches outbound webhooks on terminal run_status
// v2 RunStatusEvents.
//
// Phase 3b-D: migrated from v1 ActivityEventBus to v2 EventBus. The
// run_status publisher (service.PublishRunStatusFull) emits
// *biz.RunStatusEvent{RunID, Status, Meta}. This consumer filters at the
// bus level by EventKind == EventKindSystemRunStatus and extracts the
// same fields from the event's exported fields.
type callbackConsumer struct {
	bus      EventBus
	webhooks *WebhookDispatcher
	logger   SessionLogWriter
}

func newCallbackConsumer(bus EventBus, webhooks *WebhookDispatcher, logger SessionLogWriter) *callbackConsumer {
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
	worker := newAsyncEventWorker("event-bus-callback", sideConsumerQueueSize(), 0, c.logger, v2EventLogFields)
	worker.Start(ctx, c.handle)
	ch, unsub := c.bus.Subscribe(EventSubscribeOptions{})
	safego.Go(ctx, "event-bus-callback", func() {
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if _, ok := ev.(*RunStatusEvent); !ok {
					continue
				}
				worker.OfferWithOptions(ctx, ev, offerOption[Event]{FallbackSync: true, FallbackFn: c.handle})
			}
		}
	})
}

func (c *callbackConsumer) handle(ctx context.Context, ev Event) {
	if c == nil || c.webhooks == nil {
		return
	}
	rse, ok := ev.(*RunStatusEvent)
	if !ok {
		return
	}
	status := strings.TrimSpace(rse.Status)
	if status == "" {
		return
	}
	if _, ok := terminalRunStatuses()[strings.ToLower(status)]; !ok {
		return
	}
	runID := rse.RunID
	errMsg, _ := rse.Meta["error_message"].(string)
	eventType := RunStatusToWebhookEvent(status)
	if eventType == "" {
		return
	}
	data := map[string]any{}
	if msg := strings.TrimSpace(errMsg); msg != "" {
		data["error_message"] = msg
	}
	c.webhooks.Dispatch(ctx, eventType, runID, rse.SpiritSessionID(), status, data)
}

// v2EventLogFields extracts identifying fields from a v2 Event for
// queue-full warning logging.
func v2EventLogFields(ev Event) (sessionID, typeName, eventID string) {
	if ev == nil {
		return "", "", ""
	}
	return ev.SpiritSessionID(), string(ev.EventKind()), ev.EntityID()
}
