package biz

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const defaultEventPersistQueueSize = 512

// eventPersistHandler asynchronously writes Envelope snapshots to EventStore via a bounded queue.
type eventPersistHandler struct {
	store *EventStoreUsecase
	jobs  chan EventStoreRecord
}

func newEventPersistHandler(store *EventStoreUsecase) *eventPersistHandler {
	if store == nil {
		return nil
	}
	return &eventPersistHandler{
		store: store,
		jobs:  make(chan EventStoreRecord, eventPersistQueueSize()),
	}
}

func (h *eventPersistHandler) Start(ctx context.Context) {
	if h == nil {
		return
	}
	safego.Go(ctx, "event.store.persist.worker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case rec := <-h.jobs:
				if err := h.store.SaveRecord(context.Background(), rec); err != nil {
					event.SessionSysLogWarn(context.Background(), rec.SessionID, "event_store.persist", "事件持久化失败",
						event.P("type", rec.Type), event.P("error", err))
				}
			}
		}
	})
}

func (h *eventPersistHandler) Handle(ctx context.Context, env event.Envelope) {
	if h == nil || h.store == nil || !shouldPersistEnvelope(env) {
		return
	}
	rec, ok := envelopeToStoreRecord(env)
	if !ok {
		event.SessionSysLogWarn(ctx, env.SessionID, "event_store.persist", "事件序列化失败",
			event.P("type", string(env.Type)), event.P("id", env.ID))
		return
	}
	select {
	case h.jobs <- rec:
	default:
		event.SessionSysLogWarn(ctx, env.SessionID, "event_store.persist", "持久化队列已满，丢弃事件",
			event.P("type", string(env.Type)), event.P("id", env.ID))
	}
}

func shouldPersistEnvelope(env event.Envelope) bool {
	switch env.Type {
	case event.EnvelopeTypeLog, event.EnvelopeTypeFlowLog,
		event.EnvelopeTypeTextDelta, event.EnvelopeTypeMemberDelta:
		return false
	default:
		return strings.TrimSpace(env.ID) != ""
	}
}

func envelopeToStoreRecord(env event.Envelope) (EventStoreRecord, bool) {
	raw, err := json.Marshal(env)
	if err != nil {
		return EventStoreRecord{}, false
	}
	createdAt := time.Now().UTC()
	if env.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, env.Timestamp); err == nil {
			createdAt = t.UTC()
		}
	}
	return EventStoreRecord{
		ID:           env.ID,
		SessionID:    env.SessionID,
		Type:         string(env.Type),
		Author:       env.Author,
		Channel:      env.Channel,
		EnvelopeJSON: string(raw),
		CreatedAt:    createdAt,
	}, true
}

func eventPersistQueueSize() int {
	raw := strings.TrimSpace(os.Getenv("EVENT_STORE_PERSIST_QUEUE"))
	if raw == "" {
		return defaultEventPersistQueueSize
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultEventPersistQueueSize
	}
	return n
}
