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
// idempotent. Cross-process WS reconnect hydrate uses v2 REST
// (/v2/sessions/... tasks/turns/steps) via activityV2Store.fetchSessionHistory.
//
// Phase 5 Blocker E: the Buffer field has been removed. Blocker A deleted the
// WS replay path (Buffer.Replay is no longer called); the buffer was
// write-only with no reader. All Append callsites were dead writes and have
// been cleaned up (FlowTracker.emit / EventBusConsumer.handleEnvelope).
//
// Phase 5 Blocker F (2026-06-26): legacy envelope SessionBus removed.
// 2026-07-16: chat/session realtime uses biz.EventBus (v2 events → WS v2_event);
// ActivityEventBus is retired from the production publish path. Monitor stays
 // on typed MonitorEventBus (contract.MonitorEvent).
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
// Chat/session: biz.EventBus (v2). Monitor: contract.MonitorBus.
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideMonitorEventBus,
)
