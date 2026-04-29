package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const InstrumentationName = "arenea/backend/internal/capability"

type Provider struct {
	tracer trace.Tracer
	meter  metric.Meter
}

func NewProvider() *Provider {
	return &Provider{
		tracer: otel.Tracer(InstrumentationName),
		meter:  otel.Meter(InstrumentationName),
	}
}

func DefaultProvider() *Provider {
	return NewProvider()
}

func (p *Provider) Tracer() trace.Tracer {
	if p == nil || p.tracer == nil {
		return otel.Tracer(InstrumentationName)
	}
	return p.tracer
}

func (p *Provider) Meter() metric.Meter {
	if p == nil || p.meter == nil {
		return otel.Meter(InstrumentationName)
	}
	return p.meter
}
