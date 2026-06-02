package event

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/logpipeline"
)

type busPublisher struct {
	bus contract.Bus
}

func NewLogPipelinePublisher(bus contract.Bus) logpipeline.Publisher {
	return &busPublisher{bus: bus}
}

func (p *busPublisher) Publish(ctx context.Context, kind logpipeline.EntryKind, level, message, sessionID string, fields map[string]any) {
	if p.bus == nil {
		return
	}
	var envType contract.EnvelopeType
	switch kind {
	case logpipeline.KindFlow:
		envType = contract.EnvelopeTypeFlowLog
	default:
		envType = contract.EnvelopeTypeLog
	}
	envelope := contract.NewEnvelope(envType, "system", sessionID)
	envelope.Channel = "monitor"
	envelope.Content = &contract.EnvelopeContent{
		Text:      message,
		IsPartial: false,
	}
	envelope.Metadata = fields
	p.bus.Publish(ctx, envelope)
}
