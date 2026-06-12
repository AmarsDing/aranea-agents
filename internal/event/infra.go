package event

import (
	"context"
	"database/sql"
	"os"
	"sync"

	"github.com/google/wire"

	"aranea-agents/pkg/loggateway"
)

// Infra holds session vs monitor event buses (P0: isolate flow_log from chat envelopes).
type Infra struct {
	SessionBus Bus
	MonitorBus Bus
	Buffer     *Buffer
	WAL        *EventWAL // nil when WAL is not configured (e.g., no SQLite)
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
func NewInfra(lg loggateway.Logger, wal *EventWAL) *Infra {
	mode := routingMode(os.Getenv("MONITOR_BUS_ROUTING"))
	if mode == "" {
		mode = routingModeSplit
	}
	return &Infra{
		SessionBus: NewBus(lg),
		MonitorBus: NewBus(lg),
		Buffer:     NewBuffer(),
		WAL:        wal,
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

// ProvideEventWAL creates an EventWAL instance. Returns nil if the database
// is not available (e.g., in test environments without SQLite).
func ProvideEventWAL(db *sql.DB, lg loggateway.Logger) *EventWAL {
	if db == nil {
		return nil
	}
	wal, err := NewEventWAL(db, lg)
	if err != nil {
		if lg != nil {
			lg.Warn("event_wal: failed to create, Critical events will not have WBPF protection",
				loggateway.Err(err),
			)
		}
		return nil
	}
	return wal
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
// For Critical events (AS-EVT-01), if WAL is available, the event is persisted
// before publishing (Write-Before-Publish-Fanout). If WAL write fails, the event
// is still published directly (better to publish without WAL protection than to
// lose the event entirely).
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
	if infra.WAL != nil {
		publish := func() { infra.publishToBuses(ctx, env) }
		if err := infra.WAL.WriteBeforePublish(ctx, env, publish); err != nil {
			// WAL write failed — log and fall back to direct publish.
			// Better to publish without WAL protection than to lose the event entirely.
			if infra.lg != nil {
				infra.lg.Warn("event_wal: WBPF failed, falling back to direct publish",
					loggateway.Str("type", string(env.Type)),
					loggateway.Str("id", env.ID),
					loggateway.Err(err),
				)
			}
			infra.publishToBuses(ctx, env)
		}
		return
	}
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
// SessionBus is the default event.Bus binding; MonitorBus is accessed via *Infra (WS + flow logs).
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideEventWAL,
	ProvideSessionBus,
	ProvideBuffer,
)
