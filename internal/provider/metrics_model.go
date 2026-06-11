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
func WrapModelWithMetrics(inner trpcmodel.Model, provider, model string) trpcmodel.Model {
	if inner == nil {
		return nil
	}
	return &metricsModel{
		inner:    inner,
		provider: strings.TrimSpace(provider),
		model:    strings.TrimSpace(model),
	}
}

func (m *metricsModel) Info() trpcmodel.Info {
	return m.inner.Info()
}

func (m *metricsModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	start := time.Now()
	ch, err := m.inner.GenerateContent(ctx, request)
	if err != nil {
		metrics.ProviderRequestTotal.WithLabelValues(m.provider, m.model, "error").Inc()
		metrics.ProviderRequestDuration.WithLabelValues(m.provider, m.model).Observe(time.Since(start).Seconds())
		return nil, err
	}
	// BUG-6 fix: inner model returning (nil, nil) violates the framework
	// contract and would cause a goroutine leak (range on nil channel blocks
	// forever). Surface the violation as an explicit error.
	if ch == nil {
		metrics.ProviderRequestTotal.WithLabelValues(m.provider, m.model, "error").Inc()
		metrics.ProviderRequestDuration.WithLabelValues(m.provider, m.model).Observe(time.Since(start).Seconds())
		return nil, ErrModelNilChannel
	}
	out := make(chan *trpcmodel.Response, 16)
	safego.Go(ctx, "metrics-model-stream", func() {
		defer close(out)
		status := "ok"
		for resp := range ch {
			if resp != nil && resp.Error != nil {
				status = "error"
			}
			out <- resp
		}
		metrics.ProviderRequestTotal.WithLabelValues(m.provider, m.model, status).Inc()
		metrics.ProviderRequestDuration.WithLabelValues(m.provider, m.model).Observe(time.Since(start).Seconds())
	})
	return out, nil
}
