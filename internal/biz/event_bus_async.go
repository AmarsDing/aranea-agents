package biz

import (
	"context"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

const defaultSideConsumerQueueSize = 256

// asyncEnvelopeWorker drains a bounded queue so Bus subscribers return quickly.
type asyncEnvelopeWorker struct {
	name   string
	jobs   chan contract.Envelope
	logger SessionLogWriter
}

func newAsyncEnvelopeWorker(name string, queueSize int) *asyncEnvelopeWorker {
	if queueSize <= 0 {
		queueSize = defaultSideConsumerQueueSize
	}
	return &asyncEnvelopeWorker{
		name: name,
		jobs: make(chan contract.Envelope, queueSize),
	}
}

func (w *asyncEnvelopeWorker) SetLogger(logger SessionLogWriter) {
	w.logger = logger
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
				handle(context.Background(), env)
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
