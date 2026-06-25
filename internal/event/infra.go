package event

import (
	"context"
	"os"
	"sync"

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
	// routing caches MONITOR_BUS_ROUTING once at construction to avoid per-call os.Getenv (M-01).
	routing routingMode
}

var (
	boundInfra   *Infra
	boundInfraMu sync.RWMutex
)

// BindInfra wires the process-wide event infra for monitor flow logs (replaces SetGlobalBus).
//
// Deprecated: prefer injecting *Infra directly via Wire (InfraProviderSet). BindInfra
// remains for legacy call-sites that rely on the process-wide singleton.
func BindInfra(infra *Infra) {
	boundInfraMu.Lock()
	boundInfra = infra
	boundInfraMu.Unlock()
}

// boundInfraRef returns the process-wide bound Infra.
//
// Deprecated: prefer injecting *Infra directly. boundInfraRef remains for
// monitorBusRef and legacy call-sites that have not yet migrated to DI.
func boundInfraRef() *Infra {
	boundInfraMu.RLock()
	defer boundInfraMu.RUnlock()
	return boundInfra
}

func monitorBusRef() Bus {
	if infra := boundInfraRef(); infra != nil {
		return infra.MonitorBus
	}
	return nil
}

// NewInfra wires dual buses for dependency injection.
func NewInfra(lg loggateway.Logger) *Infra {
	mode := routingMode(os.Getenv("MONITOR_BUS_ROUTING"))
	if mode == "" {
		mode = routingModeSplit
	}
	return &Infra{
		SessionBus: NewBus(lg),
		MonitorBus: NewBus(lg),
		Buffer:     NewBuffer(),
		lg:         lg,
		routing:    mode,
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

// routingMode caches the MONITOR_BUS_ROUTING env var value so Publish does not
// call os.Getenv on every hot-path invocation (M-01).
// The value is read once per Infra instance at construction time.
type routingMode string

const (
	routingModeDual  routingMode = "dual"
	routingModeSplit routingMode = "split"
)

// Publish routes an envelope to the correct bus(es) based on its type.
//
// Phase 1c-2: WAL/Write-Before-Publish-Fanout has been removed along with the
// event_store subsystem. All envelopes are now published directly to the
// routing buses. Critical events (ToolResult/Error/Checkpoint — AS-EVT-01)
// no longer have crash-recovery guarantees; subscribers must be idempotent.
//
// Routing mode is controlled by the MONITOR_BUS_ROUTING environment variable
// (read once at NewInfra / BindInfra time):
//   - "split" (default, Phase 1): flow_log and log go to MonitorBus ONLY.
//   - "dual"  (Phase 0 fallback): flow_log and log go to BOTH SessionBus and MonitorBus.
//
// Monitor-only types (flow_log, log) are isolated from the session bus to prevent
// high-frequency monitor events from crowding out chat/team envelopes.
//
// Alert and MCP health events are dual-published so both session-scoped and
// global monitor connections receive them.
func (infra *Infra) Publish(ctx context.Context, env Envelope) {
	infra.publishToBuses(ctx, env)
}

// publishToBuses routes an envelope to the correct bus(es) based on its type.
func (infra *Infra) publishToBuses(ctx context.Context, env Envelope) {
	switch env.Type {
	case EnvelopeTypeFlowLog, EnvelopeTypeLog:
		if infra.routing != routingModeSplit {
			if infra.SessionBus != nil {
				infra.SessionBus.Publish(ctx, env)
			}
		}
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
