package provider

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type metricsModel struct {
	inner    trpcmodel.Model
	provider string
	model    string
}

// WrapModelWithMetrics records ProviderRequestTotal and ProviderRequestDuration.
// When inner implements IterModel, the returned wrapper also implements IterModel
// so callers can keep the lower-overhead iterator path (P3 IterModel).
func WrapModelWithMetrics(inner trpcmodel.Model, provider, model string) trpcmodel.Model {
	if inner == nil {
		return nil
	}
	base := &metricsModel{
		inner:    inner,
		provider: strings.TrimSpace(provider),
		model:    strings.TrimSpace(model),
	}
	if _, ok := inner.(trpcmodel.IterModel); ok {
		return &metricsIterModel{metricsModel: base}
	}
	return base
}

type metricsIterModel struct {
	*metricsModel
}

func (m *metricsIterModel) GenerateContentIter(ctx context.Context, request *trpcmodel.Request) (trpcmodel.Seq[*trpcmodel.Response], error) {
	iter, ok := m.inner.(trpcmodel.IterModel)
	if !ok {
		return nil, ErrModelNilChannel
	}
	start := time.Now()
	seq, err := iter.GenerateContentIter(ctx, request)
	if err != nil {
		m.recordMetrics(start, "error")
		return nil, err
	}
	if seq == nil {
		m.recordMetrics(start, "error")
		return nil, ErrModelNilChannel
	}
	return func(yield func(*trpcmodel.Response) bool) {
		status := "ok"
		seq(func(resp *trpcmodel.Response) bool {
			if resp != nil && resp.Error != nil {
				status = "error"
			}
			return yield(resp)
		})
		if ctx.Err() != nil {
			status = "cancelled"
		}
		m.recordMetrics(start, status)
	}, nil
}

func (m *metricsModel) Info() trpcmodel.Info {
	return m.inner.Info()
}

func (m *metricsModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	start := time.Now()
	ch, err := m.inner.GenerateContent(ctx, request)
	if err != nil {
		m.recordMetrics(start, "error")
		return nil, err
	}
	// BUG-6 fix: inner model returning (nil, nil) violates the framework
	// contract and would cause a goroutine leak (range on nil channel blocks
	// forever). Surface the violation as an explicit error.
	if ch == nil {
		m.recordMetrics(start, "error")
		return nil, ErrModelNilChannel
	}
	out := make(chan *trpcmodel.Response, 16)
	safego.Go(ctx, "metrics-model-stream", func() {
		defer close(out)
		status := "ok"
		// BUG-9 fix: goroutine must respect ctx cancellation to avoid leak
		// when the consumer stops reading (e.g. error early-return) or ctx
		// is cancelled. Without this, the goroutine blocks forever on
		// `out <- resp` after the consumer abandons the channel.
		for {
			select {
			case <-ctx.Done():
				// ctx 取消：排空 ch 避免 inner model goroutine 阻塞，
				// 记录 cancelled 指标后退出。
				safego.Go(ctx, "metrics-model-drain", func() { drainResponseChannel(ch) })
				m.recordMetrics(start, "cancelled")
				return
			case resp, ok := <-ch:
				if !ok {
					// channel 正常关闭，记录最终指标。
					m.recordMetrics(start, status)
					return
				}
				if resp != nil && resp.Error != nil {
					status = "error"
				}
				select {
				case <-ctx.Done():
					safego.Go(ctx, "metrics-model-drain", func() { drainResponseChannel(ch) })
					m.recordMetrics(start, "cancelled")
					return
				case out <- resp:
				}
			}
		}
	})
	return out, nil
}

// recordMetrics records request total and duration for the given status.
func (m *metricsModel) recordMetrics(start time.Time, status string) {
	metrics.ProviderRequestTotal.WithLabelValues(m.provider, m.model, status).Inc()
	metrics.ProviderRequestDuration.WithLabelValues(m.provider, m.model).Observe(time.Since(start).Seconds())
}

// drainResponseChannel exhausts a response channel to unblock the producer
// goroutine when the consumer is no longer reading (e.g. after ctx cancel).
func drainResponseChannel(ch <-chan *trpcmodel.Response) {
	for range ch {
	}
}
