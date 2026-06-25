package event

import (
	"context"

	"github.com/google/wire"

	"aranea-agents/pkg/loggateway"
)

// Infra holds session vs monitor event buses (P0: isolate flow_log from chat envelopes).
//
// Phase 1c-2: WAL and CrossProcessStore fields have been removed along with
// the deletion of the event_store subsystem. Critical events no longer have
// WBPF (Write-Before-Publish-Fanout) protection; subscribers must be
// idempotent. Cross-process WS reconnect replay now relies on Activity
// records fetched via the ListActivities RPC (see service.SessionService).
type Infra struct {
	SessionBus Bus
	MonitorBus Bus
	Buffer     *Buffer
	lg         loggateway.Logger
}

// NewInfra wires dual buses for dependency injection.
func NewInfra(lg loggateway.Logger) *Infra {
	return &Infra{
		SessionBus: NewBus(lg),
		MonitorBus: NewBus(lg),
		Buffer:     NewBuffer(),
		lg:         lg,
	}
}

// ProvideSessionBus exposes the interactive/session bus for wire.
func ProvideSessionBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus(nil)
	}
	return infra.SessionBus
}

// ProvideMonitorBus exposes the monitor/flow bus for wire.
func ProvideMonitorBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus(nil)
	}
	return infra.MonitorBus
}

// ProvideBuffer exposes the session replay buffer for wire.
func ProvideBuffer(infra *Infra) *Buffer {
	if infra == nil {
		return NewBuffer()
	}
	return infra.Buffer
}

// Publish routes an envelope to the correct bus(es) based on its type.
//
// Phase 1c-2: WAL/Write-Before-Publish-Fanout has been removed along with the
// event_store subsystem. All envelopes are published directly to the routing
// buses without crash-recovery guarantees; subscribers must be idempotent.
//
// Routing policy (fixed, formerly controlled by MONITOR_BUS_ROUTING env var):
//   - flow_log and log go to MonitorBus ONLY (split mode), isolating
//     high-frequency monitor events from chat/team envelopes.
//   - Alert and MCP health events are dual-published so both session-scoped
//     and global monitor connections receive them.
//   - All other types go to SessionBus ONLY.
func (infra *Infra) Publish(ctx context.Context, env Envelope) {
	infra.publishToBuses(ctx, env)
}

// publishToBuses routes an envelope to the correct bus(es) based on its type.
func (infra *Infra) publishToBuses(ctx context.Context, env Envelope) {
	switch env.Type {
	case EnvelopeTypeFlowLog, EnvelopeTypeLog:
		if infra.MonitorBus != nil {
			infra.MonitorBus.Publish(ctx, env)
		}
	case EnvelopeTypeAlertNotify, EnvelopeTypeMCPHealthAlert:
		if infra.SessionBus != nil {
			infra.SessionBus.Publish(ctx, env)
		}
		if infra.MonitorBus != nil {
			infra.MonitorBus.Publish(ctx, env)
		}
	default:
		if infra.SessionBus != nil {
			infra.SessionBus.Publish(ctx, env)
		}
	}
}

// InfraProviderSet is the wire set replacing standalone NewBus/NewBuffer.
// SessionBus is the default event.Bus binding; MonitorBus is accessed via
// *Infra (e.g. provideTraceProjector) rather than a separate Bus binding
// to avoid Wire's "multiple bindings for event.Bus" error.
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideSessionBus,
	ProvideBuffer,
)
