package contract

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MonitorEventType labels the kind of event carried by a MonitorEvent.
//
// MonitorEvents replace the legacy Envelope for monitor-channel events.
// They carry observability and alerting signals (logs, flow logs, MCP
// health, alerts, self-healing) that are NOT chat Activities and do NOT
// need persistence. Subscribers are typically the WS monitor pump that
// forwards them to the frontend monitor panel.
type MonitorEventType string

const (
	// MonitorEventTypeLog carries a structured log line from runtime/business code.
	MonitorEventTypeLog MonitorEventType = "log"
	// MonitorEventTypeFlowLog carries a flow-context-aware log entry
	// (step/session/run scoped) from internal/event/flow_log.go.
	MonitorEventTypeFlowLog MonitorEventType = "flow_log"
	// MonitorEventTypeMCPSessionReconnect signals that an MCP session
	// reconnected after a transient failure.
	MonitorEventTypeMCPSessionReconnect MonitorEventType = "mcp.session.reconnect"
	// MonitorEventTypeMCPHealthAlert signals an MCP server health state
	// change (healthy ↔ degraded ↔ unhealthy).
	MonitorEventTypeMCPHealthAlert MonitorEventType = "mcp.health.alert"
	// MonitorEventTypeAlertNotify carries a generic alert notification
	// (threshold breach, anomaly, manual alert).
	MonitorEventTypeAlertNotify MonitorEventType = "alert.notify"
	// MonitorEventTypeMonitorAutoHealed signals that the monitor subsystem
	// automatically healed a previously-detected issue.
	MonitorEventTypeMonitorAutoHealed MonitorEventType = "monitor.auto_healed"
	// MonitorEventTypeMonitorSelfCheckCompleted signals that a periodic
	// monitor self-check finished (with status in Metadata).
	MonitorEventTypeMonitorSelfCheckCompleted MonitorEventType = "monitor.self_check_completed"

	// === Extended types for legacy string-literal EnvelopeTypes ===
	// These were previously published as ad-hoc string literals (not in the
	// EnvelopeType constant set). They are promoted to first-class
	// MonitorEventType constants during the dual-bus unification migration.

	// MonitorEventTypeCronDeadLetter carries a dead-letter notification from
	// the cron runner (a scheduled job exhausted retries). Metadata carries
	// the cron job ID, last error, and retry count.
	MonitorEventTypeCronDeadLetter MonitorEventType = "cron.dead_letter"

	// MonitorEventTypeSkillReload signals that the skill registry reloaded
	// its definitions from disk (operator info).
	MonitorEventTypeSkillReload MonitorEventType = "skill.reload"

	// MonitorEventTypeSkillFilesystemUpdated signals that a skill's filesystem
	// state was updated (file changed on disk). Emitted by skill/watch/reporter.
	MonitorEventTypeSkillFilesystemUpdated MonitorEventType = "skill.filesystem.updated"

	// MonitorEventTypeSkillFilesystemRecovered signals that a skill's filesystem
	// recovered from a previously-degraded state.
	MonitorEventTypeSkillFilesystemRecovered MonitorEventType = "skill.filesystem.recovered"

	// MonitorEventTypeSkillFilesystemImported signals that a skill was imported
	// from an external source into the local filesystem.
	MonitorEventTypeSkillFilesystemImported MonitorEventType = "skill.filesystem.imported"
)

// MonitorEvent is the unified transport for monitor-channel events.
//
// It replaces the legacy Envelope for the 7 monitor event types that
// previously rode on EnvelopeType constants (log/flow_log/mcp.*/alert.*/monitor.*).
// Unlike ActivityEvent, MonitorEvent is NEVER persisted to the activities
// table; it is only delivered to live WS subscribers (monitor pump).
//
// Reliability level (AS-EVT-01): Informational — best-effort delivery,
// drop-on-full-buffer is acceptable. Loss only degrades observability
// visibility, never corrupts state.
type MonitorEvent struct {
	// ID is a unique per-event identifier (UUID). Used for deduplication
	// and trace correlation.
	ID string `json:"id"`

	// Type identifies the monitor event variant (see MonitorEventType constants).
	Type MonitorEventType `json:"type"`

	// Timestamp is the UTC time the event was emitted.
	Timestamp time.Time `json:"timestamp"`

	// Level carries the severity for log/flow_log events
	// ("debug"/"info"/"warn"/"error"). Empty for non-log types.
	Level string `json:"level,omitempty"`

	// Message carries the human-readable log message for log/flow_log events.
	// Empty for non-log types (use Metadata for structured payload).
	Message string `json:"message,omitempty"`

	// SessionID scopes the event to a chat session when applicable.
	// Empty for global monitor events (e.g. MCP server health).
	SessionID string `json:"session_id,omitempty"`

	// Source identifies the emitting component
	// (e.g. "flow_tracker", "mcp_health", "monitor_loop").
	Source string `json:"source,omitempty"`

	// Metadata carries the type-specific structured payload.
	// For alert.notify: alert_code/severity/labels/current_value/threshold.
	// For mcp.health.alert: server_id/health_state/error.
	// For monitor.auto_healed: issue_id/healing_action/result.
	// For monitor.self_check_completed: checks_total/checks_passed/failures.
	// For log/flow_log: structured fields (step_id/run_id/etc.).
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewMonitorEvent creates a MonitorEvent with a generated ID and current UTC timestamp.
func NewMonitorEvent(typ MonitorEventType, source string) MonitorEvent {
	return MonitorEvent{
		ID:        uuid.NewString(),
		Type:      typ,
		Timestamp: time.Now().UTC(),
		Source:    source,
	}
}

// MonitorBus is the in-process event fanout hub for MonitorEvents.
//
// It replaces the legacy Envelope-based Bus for monitor-channel events
// (log/flow_log/mcp/alert/self-healing). Implementations must be safe for
// concurrent use.
//
// Stability:evolving
type MonitorBus interface {
	// Publish broadcasts a MonitorEvent to all matching subscribers.
	Publish(ctx context.Context, event MonitorEvent)

	// Subscribe registers a subscriber that receives MonitorEvents matching
	// the given options. Returns a channel of events and an unsubscribe function.
	// When globalMode is false, only events for the given sessionID are delivered
	// (events with empty SessionID are always delivered in either mode).
	Subscribe(opts MonitorSubscribeOptions) (<-chan MonitorEvent, func())

	// DropCount returns the total number of dropped events due to full buffers.
	DropCount() uint64
}

// MonitorSubscribeOptions configures a MonitorBus subscription.
type MonitorSubscribeOptions struct {
	// SessionID filters events to a specific session when GlobalMode is false.
	// Events with empty SessionID are always delivered (global alerts etc.).
	SessionID  string
	BufferSize int  // subscriber channel buffer size
	GlobalMode bool // true = receive all events (ignores SessionID filter)
	// Filter is an optional predicate applied at the bus level. When set, only
	// events for which Filter returns true are delivered. This prevents
	// non-matching events from filling the subscriber queue. When nil, the
	// session-scoped filter derived from SessionID/GlobalMode is used.
	Filter func(MonitorEvent) bool
}
