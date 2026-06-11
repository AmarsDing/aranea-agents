package biz

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

const defaultEventPersistQueueSize = 512

// eventPersistHandler asynchronously writes Envelope snapshots to EventStore via a bounded queue.
type eventPersistHandler struct {
	store  *EventStoreUsecase
	jobs   chan EventStoreRecord
	logger SessionLogWriter
}

func newEventPersistHandler(store *EventStoreUsecase, logger SessionLogWriter) *eventPersistHandler {
	if store == nil {
		return nil
	}
	return &eventPersistHandler{
		store:  store,
		jobs:   make(chan EventStoreRecord, eventPersistQueueSize()),
		logger: logger,
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
					if h.logger != nil {
						h.logger.LogSessionWarn(context.Background(), rec.SessionID, "event_store.persist", "事件持久化失败",
							LogPair{Key: "type", Value: rec.Type}, LogPair{Key: "error", Value: err})
					}
				}
			}
		}
	})
}

// isCriticalEnvelopeType returns true for event types that must never be silently
// dropped (AS-EVT-01 Critical tier: WBPF — Write-Before-Process-Forward).
func isCriticalEnvelopeType(t contract.EnvelopeType) bool {
	switch t {
	case contract.EnvelopeTypeToolResult,
		contract.EnvelopeTypeError,
		contract.EnvelopeTypeRunnerCompletion,
		contract.EnvelopeTypeStateDelta,
		contract.EnvelopeTypeTokenUsage,
		contract.EnvelopeTypeCheckpoint:
		return true
	default:
		return false
	}
}

func (h *eventPersistHandler) Handle(ctx context.Context, env contract.Envelope) {
	if h == nil || h.store == nil || !shouldPersistEnvelope(env) {
		return
	}
	rec, ok := envelopeToStoreRecord(env)
	if !ok {
		if h.logger != nil {
			h.logger.LogSessionWarn(ctx, env.SessionID, "event_store.persist", "事件序列化失败",
				LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
		}
		return
	}

	// Try non-blocking send first.
	select {
	case h.jobs <- rec:
		return
	default:
	}

	// Queue is full. For critical events (AS-EVT-01), fall back to synchronous
	// write so they are never silently dropped. Non-critical events are dropped.
	if !isCriticalEnvelopeType(env.Type) {
		if h.logger != nil {
			h.logger.LogSessionWarn(ctx, env.SessionID, "event_store.persist", "持久化队列已满，丢弃事件",
				LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
		}
		return
	}

	// Synchronous fallback for critical events — use Background context to
	// avoid cancellation from the request context.
	if h.logger != nil {
		h.logger.LogSessionWarn(ctx, env.SessionID, "event_store.persist", "持久化队列已满，关键事件同步写入",
			LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
	}
	if err := h.store.SaveRecord(context.Background(), rec); err != nil {
		if h.logger != nil {
			h.logger.LogSessionWarn(context.Background(), rec.SessionID, "event_store.persist", "关键事件同步写入失败",
				LogPair{Key: "type", Value: rec.Type}, LogPair{Key: "id", Value: rec.ID}, LogPair{Key: "error", Value: err})
		}
	}
}

func shouldPersistEnvelope(env contract.Envelope) bool {
	switch env.Type {
	case contract.EnvelopeTypeLog, contract.EnvelopeTypeFlowLog,
		contract.EnvelopeTypeTextDelta, contract.EnvelopeTypeMemberDelta:
		return false
	default:
		return strings.TrimSpace(env.ID) != ""
	}
}

func envelopeToStoreRecord(env contract.Envelope) (EventStoreRecord, bool) {
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
