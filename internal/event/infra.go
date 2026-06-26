package event

import (
	"github.com/google/wire"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// Infra holds session vs monitor event buses (P0: isolate flow_log from chat envelopes).
//
// Phase 1c-2: WAL and CrossProcessStore fields have been removed along with
// the deletion of the event_store subsystem. Critical events no longer have
// WBPF (Write-Before-Publish-Fanout) protection; subscribers must be
// idempotent. Cross-process WS reconnect replay now relies on Activity
// records fetched via the ListActivities RPC (see service.SessionService).
//
// Phase 5 Blocker E: the Buffer field has been removed. Blocker A deleted the
// WS replay path (Buffer.Replay is no longer called); the buffer was
// write-only with no reader. All Append callsites were dead writes and have
// been cleaned up (FlowTracker.emit / EventBusConsumer.handleEnvelope).
//
// Dual-bus unification (2026-06-25): MonitorEventBus is the new typed bus
// carrying contract.MonitorEvent. The legacy MonitorBus (envelope-based)
// is retained temporarily for migration; it will be removed in Phase 5
// once all monitor publishers have switched to MonitorEventBus.
type Infra struct {
	// SessionBus — TECH-DEBT(ADR-03 Phase 5 Blocker F): legacy envelope bus
	// (chat/session events). Bound as the default event.Bus via
	// ProvideSessionBus (consumed by EventBusConsumer and 7+ other callers).
	// Phase 5 Blocker D removed all publishers to SessionBus
	// (EmitProgress / Error envelope / session_revision publish /
	// Infra.Publish routing). The remaining consumers subscribe to a bus
	// that no longer receives events — they will be migrated to
	// ActivityEventBus/MonitorEventBus in Blocker F. See ProvideSessionBus
	// doc for the full caller list and migration blocker.
	SessionBus      Bus
	MonitorBus      Bus                 // legacy envelope bus (monitor events — being migrated)
	MonitorEventBus contract.MonitorBus  // NEW: typed monitor event bus (replacement for MonitorBus)
	lg              loggateway.Logger
}

// NewInfra wires dual buses for dependency injection.
func NewInfra(lg loggateway.Logger) *Infra {
	return &Infra{
		SessionBus:      NewBus(lg),
		MonitorBus:      NewBus(lg),
		MonitorEventBus: NewMonitorBus(lg),
		lg:              lg,
	}
}

// ProvideSessionBus exposes the interactive/session bus for wire.
//
// TECH-DEBT(ADR-03 Phase 5 Blocker F): ProvideSessionBus is the default
// event.Bus Wire binding and is still actively consumed by 8+ callers via
// the generated `contractBus` local in cmd/admin/wire_gen.go:
//   - biz.NewEventBusConsumer (subscribes to SessionBus for Critical envelope
//     dedup / eventBuffer / domain-event routing — see event_bus_consumer.go)
//   - service.NewGraphOrchestrationProjector
//   - provideChatServiceDeps / provideRunnerConfig / provideTeamTurnDeps
//   - team.ProvideTeamGraphRunCoordinator
//   - provideCronRunnerDeps / provideChannelIngress
//
// Phase 5 Blocker D removed all direct publishers to SessionBus:
//   - TraceEmitter.EmitProgress (deleted — ExecutionProgress envelopes)
//   - FlowTracker.LogError Error envelope publish (deleted)
//   - PublishSessionRevisionEnvelope (deleted — RunStatus envelopes)
//   - Infra.Publish / publishToBuses routing (deleted — no callers remained)
// The remaining consumers subscribe to a bus that receives no events;
// they will be migrated to ActivityEventBus/MonitorEventBus in Blocker F.
// ProvideSessionBus cannot be removed until that migration completes.
// Note: provideTraceProjector's sessionBus argument was a dead binding and
// has been removed (Blocker F cleanup); see cmd/admin/wire.go.
func ProvideSessionBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus(nil)
	}
	return infra.SessionBus
}

// ProvideMonitorBus exposes the legacy monitor/flow envelope bus for wire.
// Deprecated: use ProvideMonitorEventBus for new code. This is retained
// during the dual-bus migration period and will be removed in Phase 5.
func ProvideMonitorBus(infra *Infra) Bus {
	if infra == nil {
		return NewBus(nil)
	}
	return infra.MonitorBus
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

// InfraProviderSet is the wire set replacing standalone NewBus/NewBuffer.
// SessionBus is the default event.Bus binding; MonitorBus is accessed via
// *Infra (e.g. provideTraceProjector) rather than a separate Bus binding
// to avoid Wire's "multiple bindings for event.Bus" error.
//
// Dual-bus unification: ProvideMonitorEventBus is added to make
// contract.MonitorBus available to any provider that needs it.
var InfraProviderSet = wire.NewSet(
	NewInfra,
	ProvideSessionBus,
	ProvideMonitorEventBus,
)
