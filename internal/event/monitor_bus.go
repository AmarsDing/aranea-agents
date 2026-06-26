package event

import (
	"context"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// monitorBus implements contract.MonitorBus using a GenericBus.
//
// It replaces the legacy Envelope-based Bus for monitor-channel events
// (log/flow_log/mcp/alert/self-healing). MonitorEvents are never persisted
// to the activities table; they are only delivered to live WS subscribers
// via the monitor pump.
//
// The implementation delegates to GenericBus[contract.MonitorEvent] which
// wraps the framework's type-safe bus, providing drop-on-full-buffer
// semantics (Informational reliability per AS-EVT-01).
type monitorBus struct {
	inner *GenericBus[contract.MonitorEvent]
}

// newMonitorBus creates a contract.MonitorBus backed by a GenericBus.
// Drop events are logged via loggateway when lg is non-nil.
func newMonitorBus(lg loggateway.Logger) contract.MonitorBus {
	return &monitorBus{
		inner: NewGenericBus[contract.MonitorEvent](lg),
	}
}

// NewMonitorBus is the exported constructor for contract.MonitorBus.
// Used by Wire providers and external callers that need to create a
// standalone MonitorBus (e.g. tests, infra initialization).
func NewMonitorBus(lg loggateway.Logger) contract.MonitorBus {
	return newMonitorBus(lg)
}

// Publish broadcasts a MonitorEvent to all matching subscribers.
func (b *monitorBus) Publish(ctx context.Context, ev contract.MonitorEvent) {
	b.inner.Publish(ctx, ev)
}

// Subscribe registers a subscriber that receives MonitorEvents matching
// the given options.
//
// When GlobalMode is false and SessionID is set, only events for that
// session are delivered. Events with empty SessionID (global alerts,
// MCP server health) are ALWAYS delivered regardless of mode, so
// session-scoped subscribers still receive system-wide alerts.
func (b *monitorBus) Subscribe(opts contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	genericOpts := GenericSubscribeOptions[contract.MonitorEvent]{
		BufferSize: opts.BufferSize,
	}
	if opts.Filter != nil {
		genericOpts.Filter = opts.Filter
	} else if !opts.GlobalMode && opts.SessionID != "" {
		sessionID := opts.SessionID
		genericOpts.Filter = func(ev contract.MonitorEvent) bool {
			// Events with empty SessionID are always delivered (global alerts).
			return ev.SessionID == "" || ev.SessionID == sessionID
		}
	}
	return b.inner.Subscribe(genericOpts)
}

// DropCount returns the total number of dropped events.
func (b *monitorBus) DropCount() uint64 {
	return b.inner.DropCount()
}

// Compile-time interface check.
var _ contract.MonitorBus = (*monitorBus)(nil)
