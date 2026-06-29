package event

import (
	"github.com/google/wire"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// Infra holds the typed monitor event bus.
//
// Phase 1c-2: WAL and CrossProcessStore fields have been removed along with
// the deletion of the event_store subsystem. Important events no longer have
// WBPF (Write-Before-Publish-Fanout) protection; subscribers must be
// idempotent. Cross-process WS reconnect replay now relies on Activity
// records fetched via the ListActivities RPC (see service.SessionService).
//
// Phase 5 Blocker E: the Buffer field has been removed. Blocker A deleted the
// WS replay path (Buffer.Replay is no longer called); the buffer was
// write-only with no reader. All Append callsites were dead writes and have
// been cleaned up (FlowTracker.emit / EventBusConsumer.handleEnvelope).
//
// Phase 5 Blocker F (2026-06-26): the legacy envelope-based SessionBus has
// been removed. All chat/session event publishers and subscribers now use
// biz.ActivityEventBus (transporting biz.ActivityEvent). The legacy
// envelope-based MonitorBus was also removed; all monitor publishers and
// subscribers (FlowFileAppender, SelfHealObserver, TraceProjector,
// FlowTracker) now use the typed MonitorEventBus carrying
// contract.MonitorEvent.
type Infra struct {
	MonitorEventBus contract.MonitorBus // typed monitor event bus
	lg              loggateway.Logger
}

// NewInfra wires the monitor bus for dependency injection.
func NewInfra(lg loggateway.Logger) *Infra {
	return &Infra{
		MonitorEventBus: NewMonitorBus(lg),
		lg:              lg,
	}
}

// ProvideMonitorEventBus exposes the typed MonitorEventBus for wire.
// This is the canonical monitor bus for new code; it transports
// contract.MonitorEvent (not legacy Envelope).
func ProvideMonitorEventBus(infra *Infra) contract.MonitorBus {
	if infra == nil {
		return NewMonitorBus(nil)
	}
	return infra.MonitorEventBus
}

// InfraProviderSet is the wire set for event infrastructure.
//
// Phase 5 Blocker F: the legacy ProvideSessionBus (envelope-based) has been
// removed along with Infra.SessionBus. All chat/session subscribers now
// consume biz.ActivityEventBus; all monitor subscribers consume
// contract.MonitorBus via ProvideMonitorEventBus.
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideMonitorEventBus,
)
