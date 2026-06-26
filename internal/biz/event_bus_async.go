package biz

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/safego"
)

const defaultSideConsumerQueueSize = 256
const defaultHandleTimeout = 45 * time.Second

// eventLogFields extracts identifying fields from an event for logging.
// Returns (sessionID, typeName, eventID) used in queue-full warning messages.
type eventLogFields[T any] func(T) (sessionID, typeName, eventID string)

// offerOption configures fallback behavior when the worker queue is full.
// T is either ActivityEvent or contract.MonitorEvent.
type offerOption[T any] struct {
	// FallbackSync, when true, calls FallbackFn synchronously instead of
	// dropping the event when the queue is full.
	FallbackSync bool
	// FallbackFn is called synchronously when the queue is full and
	// FallbackSync is true. This preserves the "Reliable" delivery semantic
	// (event is processed even under backpressure, at the cost of blocking
	// the publisher).
	FallbackFn func(ctx context.Context, ev T)
}

// asyncEventWorker processes events asynchronously with a bounded queue.
// Each event is handled in a dedicated goroutine with a timeout, preventing
// a slow handler from blocking the queue drain loop.
//
// T is either ActivityEvent or contract.MonitorEvent. The logFields
// callback extracts identifying fields for queue-full warnings, avoiding
// type switches inside the generic worker.
type asyncEventWorker[T any] struct {
	name          string
	jobs          chan T
	logger        SessionLogWriter
	handleTimeout time.Duration
	logFields     eventLogFields[T]
}

func newAsyncEventWorker[T any](name string, queueSize int, handleTimeout time.Duration, logger SessionLogWriter, logFields eventLogFields[T]) *asyncEventWorker[T] {
	if queueSize <= 0 {
		queueSize = defaultSideConsumerQueueSize
	}
	if handleTimeout <= 0 {
		handleTimeout = defaultHandleTimeout
	}
	return &asyncEventWorker[T]{
		name:          name,
		jobs:          make(chan T, queueSize),
		logger:        logger,
		handleTimeout: handleTimeout,
		logFields:     logFields,
	}
}

func (w *asyncEventWorker[T]) Start(ctx context.Context, handle func(context.Context, T)) {
	if w == nil || handle == nil {
		return
	}
	safego.Go(ctx, w.name+".worker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.jobs:
				if !ok {
					return
				}
				handleCtx, cancel := context.WithTimeout(context.Background(), w.handleTimeout)
				handle(handleCtx, ev)
				cancel()
			}
		}
	})
}

// OfferWithOptions attempts to enqueue an event. When the queue is full:
//   - if FallbackSync is true and FallbackFn is set: calls FallbackFn
//     synchronously (preserving the event, blocking the caller)
//   - otherwise: drops the event and logs a warning
func (w *asyncEventWorker[T]) OfferWithOptions(ctx context.Context, ev T, opts offerOption[T]) {
	if w == nil {
		if opts.FallbackSync && opts.FallbackFn != nil {
			opts.FallbackFn(ctx, ev)
		}
		return
	}
	select {
	case w.jobs <- ev:
	default:
		if opts.FallbackSync && opts.FallbackFn != nil {
			w.logQueueFull(ctx, ev, "event_bus.queue_full_fallback", "队列已满，回退同步写入")
			opts.FallbackFn(ctx, ev)
		} else {
			w.logQueueFull(ctx, ev, "event_bus.queue_full_drop", "侧效消费者队列已满，丢弃事件")
		}
	}
}

func (w *asyncEventWorker[T]) logQueueFull(ctx context.Context, ev T, stepID, msg string) {
	if w.logger == nil || w.logFields == nil {
		return
	}
	sessionID, typeName, eventID := w.logFields(ev)
	w.logger.LogSessionWarn(ctx, sessionID, stepID, msg,
		LogPair{Key: "consumer", Value: w.name},
		LogPair{Key: "type", Value: typeName},
		LogPair{Key: "id", Value: eventID})
}

func sideConsumerQueueSize() int {
	raw := strings.TrimSpace(os.Getenv("EVENT_BUS_SIDE_QUEUE"))
	if raw == "" {
		return defaultSideConsumerQueueSize
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultSideConsumerQueueSize
	}
	return n
}
