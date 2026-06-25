package event

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/logpipeline"
)

type busPublisher struct {
	monitorBus contract.MonitorBus
}

// NewLogPipelinePublisher publishes logpipeline entries as MonitorEvents on
// the typed contract.MonitorBus. Replaces the legacy Envelope-based publisher.
func NewLogPipelinePublisher(monitorBus contract.MonitorBus) logpipeline.Publisher {
	return &busPublisher{monitorBus: monitorBus}
}

func (p *busPublisher) Publish(ctx context.Context, kind logpipeline.EntryKind, level, message, sessionID string, fields map[string]any) {
	if p.monitorBus == nil {
		return
	}
	var typ contract.MonitorEventType
	switch kind {
	case logpipeline.KindFlow:
		typ = contract.MonitorEventTypeFlowLog
	default:
		typ = contract.MonitorEventTypeLog
	}
	ev := contract.NewMonitorEvent(typ, "system")
	ev.Level = level
	ev.Message = message
	ev.SessionID = sessionID
	ev.Metadata = fields
	p.monitorBus.Publish(ctx, ev)
}
