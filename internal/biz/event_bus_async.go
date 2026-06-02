package biz

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

const defaultSideConsumerQueueSize = 256
const defaultHandleTimeout = 45 * time.Second

type asyncEnvelopeWorker struct {
	name          string
	jobs          chan contract.Envelope
	logger        SessionLogWriter
	handleTimeout time.Duration
}

func newAsyncEnvelopeWorker(name string, queueSize int, handleTimeout time.Duration, logger SessionLogWriter) *asyncEnvelopeWorker {
	if queueSize <= 0 {
		queueSize = defaultSideConsumerQueueSize
	}
	if handleTimeout <= 0 {
		handleTimeout = defaultHandleTimeout
	}
	return &asyncEnvelopeWorker{
		name:          name,
		jobs:          make(chan contract.Envelope, queueSize),
		logger:        logger,
		handleTimeout: handleTimeout,
	}
}

func (w *asyncEnvelopeWorker) Start(ctx context.Context, handle func(context.Context, contract.Envelope)) {
	if w == nil || handle == nil {
		return
	}
	safego.Go(ctx, w.name+".worker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-w.jobs:
				if !ok {
					return
				}
				handleCtx, cancel := context.WithTimeout(context.Background(), w.handleTimeout)
				handle(handleCtx, env)
				cancel()
			}
		}
	})
}

func (w *asyncEnvelopeWorker) Offer(ctx context.Context, env contract.Envelope) {
	if w == nil {
		return
	}
	select {
	case w.jobs <- env:
	default:
		if w.logger != nil {
			w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full", "侧效消费者队列已满，丢弃事件",
				LogPair{Key: "consumer", Value: w.name}, LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
		}
	}
}

type OfferOption struct {
	FallbackSync bool
	FallbackFn   func(ctx context.Context, env contract.Envelope)
}

func (w *asyncEnvelopeWorker) OfferWithOptions(ctx context.Context, env contract.Envelope, opts OfferOption) {
	if w == nil {
		if opts.FallbackSync && opts.FallbackFn != nil {
			opts.FallbackFn(ctx, env)
		}
		return
	}
	select {
	case w.jobs <- env:
	default:
		if opts.FallbackSync && opts.FallbackFn != nil {
			if w.logger != nil {
				w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full_fallback", "队列已满，回退同步写入",
					LogPair{Key: "consumer", Value: w.name}, LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
			}
			opts.FallbackFn(ctx, env)
		} else {
			if w.logger != nil {
				w.logger.LogSessionWarn(ctx, env.SessionID, "event_bus.queue_full_drop", "侧效消费者队列已满，丢弃事件",
					LogPair{Key: "consumer", Value: w.name}, LogPair{Key: "type", Value: string(env.Type)}, LogPair{Key: "id", Value: env.ID})
			}
		}
	}
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
