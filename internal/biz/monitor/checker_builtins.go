package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"aranea-agents/internal/biz/types"

	"github.com/google/uuid"
)

// DBPinger abstracts database health-check operations so the biz layer
// does not depend directly on *sql.DB (infrastructure).
type DBPinger interface {
	PingContext(ctx context.Context) error
	QueryRowContext(ctx context.Context, query string, args ...any) DBRow
	// Dialect returns the database dialect ("postgres" or "sqlite").
	// Used to pick dialect-specific system-table queries.
	Dialect() string
}

// DBRow abstracts sql.Row.Scan for health-check queries.
type DBRow interface {
	Scan(dest ...any) error
}

// dbPingerAdapter wraps *sql.DB to implement DBPinger.
type dbPingerAdapter struct {
	db      *sql.DB
	dialect string
}

func (a *dbPingerAdapter) PingContext(ctx context.Context) error {
	return a.db.PingContext(ctx)
}

func (a *dbPingerAdapter) QueryRowContext(ctx context.Context, query string, args ...any) DBRow {
	return a.db.QueryRowContext(ctx, query, args...)
}

func (a *dbPingerAdapter) Dialect() string { return a.dialect }

// NewDBPinger creates a DBPinger from *sql.DB. Returns nil if db is nil.
// dialect is the database dialect ("postgres" or "sqlite") used to pick
// dialect-specific system-table queries.
func NewDBPinger(db *sql.DB, dialect string) DBPinger {
	if db == nil {
		return nil
	}
	return &dbPingerAdapter{db: db, dialect: dialect}
}

// defaultTraceProjectorIdleTimeout is the threshold beyond which a
// previously-active trace projector is considered stalled. The value
// must be larger than the self-check interval (5 minutes by default) to
// avoid flapping, and larger than the traceActiveTTL (10 minutes) so a
// long-running run does not look like a stall.
const defaultTraceProjectorIdleTimeout = 30 * time.Minute

// DBHealthChecker verifies database connectivity and basic schema integrity.
type DBHealthChecker struct {
	db DBPinger
}

// NewDBHealthChecker creates a checker that pings the database and verifies
// the monitor_events table exists.
func NewDBHealthChecker(db DBPinger) *DBHealthChecker {
	return &DBHealthChecker{db: db}
}

func (c *DBHealthChecker) Name() string { return "db_health" }

func (c *DBHealthChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.db == nil {
		result.Status = types.SelfCheckStatusFailed
		result.Message = "database connection is nil"
		return result
	}

	// Step 1: Ping
	start := time.Now()
	if err := c.db.PingContext(ctx); err != nil {
		result.Status = types.SelfCheckStatusFailed
		result.Message = "database ping failed"
		result.Details = map[string]any{"error": err.Error()}
		return result
	}
	pingMs := time.Since(start).Milliseconds()

	// Step 2: Verify monitor_events table exists (dialect-aware).
	//   Postgres: information_schema.tables (SQL standard)
	//   SQLite:   sqlite_master (SQLite system catalog)
	var tableName string
	var query string
	if c.db.Dialect() == "postgres" {
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'monitor_events' LIMIT 1"
	} else {
		query = "SELECT name FROM sqlite_master WHERE type='table' AND name='monitor_events' LIMIT 1"
	}
	row := c.db.QueryRowContext(ctx, query)
	if err := row.Scan(&tableName); err != nil {
		result.Status = types.SelfCheckStatusFailed
		result.Message = "schema integrity check failed: monitor_events table not found"
		result.Details = map[string]any{"error": err.Error(), "ping_ms": pingMs}
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = "database is healthy"
	result.Details = map[string]any{"ping_ms": pingMs, "table_check": "ok"}
	return result
}

// EventBusChecker verifies EventBus subscriber health.
type EventBusChecker struct {
	bus EventBusHealthChecker
}

// EventBusHealthChecker is the port for checking EventBus subscription health.
type EventBusHealthChecker interface {
	SubscriberCount(topic string) int
	IsHealthy(topic string) bool
}

// NewEventBusChecker creates a checker for EventBus health.
func NewEventBusChecker(bus EventBusHealthChecker) *EventBusChecker {
	return &EventBusChecker{bus: bus}
}

func (c *EventBusChecker) Name() string { return "eventbus" }

func (c *EventBusChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.bus == nil {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "event bus not available (nil)"
		return result
	}

	topic := "monitor"
	count := c.bus.SubscriberCount(topic)
	healthy := c.bus.IsHealthy(topic)

	if !healthy && count == 0 {
		result.Status = types.SelfCheckStatusFailed
		result.Message = fmt.Sprintf("all subscribers disconnected on topic %q", topic)
		result.Details = map[string]any{"topic": topic, "subscriber_count": 0}
		return result
	}

	if !healthy {
		result.Status = types.SelfCheckStatusWarning
		result.Message = fmt.Sprintf("some subscribers unhealthy on topic %q", topic)
		result.Details = map[string]any{"topic": topic, "subscriber_count": count}
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = fmt.Sprintf("event bus healthy, %d subscribers on topic %q", count, topic)
	result.Details = map[string]any{"topic": topic, "subscriber_count": count}
	return result
}

// FlowFileChecker verifies disk space and flow file write capability.
type FlowFileChecker struct {
	appender *FlowFileAppender
	dataDir  string
}

// NewFlowFileChecker creates a checker for flow file health.
func NewFlowFileChecker(appender *FlowFileAppender, dataDir string) *FlowFileChecker {
	return &FlowFileChecker{appender: appender, dataDir: dataDir}
}

func (c *FlowFileChecker) Name() string { return "flow_file" }

func (c *FlowFileChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.appender == nil {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "flow file appender not available"
		return result
	}

	details := map[string]any{}
	dir := c.appender.Dir()
	details["dir"] = dir

	// Write test: create and delete a temp file
	if dir != "" {
		testPath := filepath.Join(dir, ".selfcheck_write_test")
		start := time.Now()
		if err := os.WriteFile(testPath, []byte("selfcheck"), 0644); err != nil {
			result.Status = types.SelfCheckStatusFailed
			result.Message = "disk write test failed"
			result.Details = map[string]any{"error": err.Error(), "dir": dir}
			return result
		}
		os.Remove(testPath)
		details["write_test_ms"] = time.Since(start).Milliseconds()
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = "flow file appender healthy"
	result.Details = details
	return result
}

// TraceProjectorHealthChecker is the port for checking TraceProjector health.
//
// The interface exposes liveness signals (Started), activity signals
// (LastEventAt, HasEverProcessed) and the in-flight trace counter
// (TraceCount). The self-check uses all four to distinguish "idle but
// healthy" from "stalled subscription".
type TraceProjectorHealthChecker interface {
	// TraceCount returns the number of currently in-flight traces.
	// In-flight means the projector has received a trace_id-bearing
	// FlowLog for the trace and the matching runner.completion has
	// not yet been observed.
	TraceCount() int
	// Started reports whether the projector's Start() has been invoked.
	Started() bool
	// LastEventAt returns the wall-clock time of the last envelope
	// the projector received from the bus. The zero time means the
	// projector has never received an envelope.
	LastEventAt() time.Time
	// HasEverProcessed reports whether the projector has received at
	// least one envelope since it was started.
	HasEverProcessed() bool
}

// TraceProjectorChecker verifies that the trace projector is active and
// processing traces. The checker distinguishes three states:
//
//   - Idle (no traces in flight, but projector started and recent
//     activity observed) → Passed. This is the steady state of a
//     healthy system with no chat/team runs in progress.
//   - Idle (projector started, never received an envelope, or hasn't
//     received one in a long time) → Warning. This is a potential stall
//     worth surfacing.
//   - Not started (Start() was never invoked, e.g., missing wire binding)
//     → Warning. Catches wiring bugs.
type TraceProjectorChecker struct {
	projector   TraceProjectorHealthChecker
	idleTimeout time.Duration
}

// NewTraceProjectorChecker creates a checker for trace projector health.
// A zero idleTimeout falls back to the default (30 minutes).
func NewTraceProjectorChecker(projector TraceProjectorHealthChecker) *TraceProjectorChecker {
	return &TraceProjectorChecker{projector: projector, idleTimeout: defaultTraceProjectorIdleTimeout}
}

func (c *TraceProjectorChecker) Name() string { return "trace_projector" }

func (c *TraceProjectorChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.projector == nil {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "trace projector not available (nil)"
		return result
	}

	started := c.projector.Started()
	lastAt := c.projector.LastEventAt()
	hasEver := c.projector.HasEverProcessed()
	count := c.projector.TraceCount()
	timeout := c.idleTimeout
	if timeout <= 0 {
		timeout = defaultTraceProjectorIdleTimeout
	}

	result.Details = map[string]any{
		"active_traces":     count,
		"started":           started,
		"last_event_at":     lastAt,
		"has_ever_received": hasEver,
		"idle_timeout_sec":  int(timeout.Seconds()),
	}

	switch {
	case !started:
		// Projector was constructed but never started. Most likely cause
		// is a wiring bug in cmd/admin/wire.go. Worth flagging.
		result.Status = types.SelfCheckStatusWarning
		result.Message = "trace projector not started (Start() never invoked)"
		return result

	case hasEver && !lastAt.IsZero() && now.Sub(lastAt) > timeout:
		// Projector used to receive events but hasn't seen any in a while.
		// This is the actual stall condition: subscription is up but
		// traffic has stopped.
		result.Status = types.SelfCheckStatusWarning
		result.Message = fmt.Sprintf(
			"trace projector stalled: no event received in the last %s (last event at %s)",
			timeout.Truncate(time.Second), lastAt.Format(time.RFC3339),
		)
		return result
	}

	// Healthy: projector started, and either still receiving events or
	// simply idle. In-flight trace count is reported but not used as a
	// health signal — a count of 0 is the normal idle state.
	if count > 0 {
		result.Status = types.SelfCheckStatusPassed
		result.Message = fmt.Sprintf("trace projector healthy, %d active trace(s)", count)
	} else {
		result.Status = types.SelfCheckStatusPassed
		result.Message = "trace projector healthy, idle (no in-flight traces)"
	}
	return result
}

// AlertEvalHealthChecker is the port for checking AlertEvalWorker health.
type AlertEvalHealthChecker interface {
	Ready() bool
}

// AlertEvalChecker verifies that the alert evaluation worker is running and responsive.
type AlertEvalChecker struct {
	worker AlertEvalHealthChecker
}

// NewAlertEvalChecker creates a checker for alert eval worker health.
func NewAlertEvalChecker(worker AlertEvalHealthChecker) *AlertEvalChecker {
	return &AlertEvalChecker{worker: worker}
}

func (c *AlertEvalChecker) Name() string { return "alert_eval" }

func (c *AlertEvalChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.worker == nil {
		result.Status = types.SelfCheckStatusFailed
		result.Message = "alert eval worker not available (nil)"
		return result
	}

	if !c.worker.Ready() {
		result.Status = types.SelfCheckStatusFailed
		result.Message = "alert eval worker not ready (may be stalled)"
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = "alert eval worker is ready"
	return result
}

// WSConnectionCounter is the port for checking WebSocket connection health.
type WSConnectionCounter interface {
	CountGlobalMonitorConns() int
}

// WebSocketChecker verifies WebSocket connection status.
type WebSocketChecker struct {
	counter WSConnectionCounter
}

// NewWebSocketChecker creates a checker for WebSocket health.
func NewWebSocketChecker(counter WSConnectionCounter) *WebSocketChecker {
	return &WebSocketChecker{counter: counter}
}

func (c *WebSocketChecker) Name() string { return "websocket" }

func (c *WebSocketChecker) Check(ctx context.Context) types.SelfCheckResult {
	now := time.Now().UTC()
	result := types.SelfCheckResult{
		CheckID:   uuid.NewString(),
		Checker:   c.Name(),
		CheckedAt: now,
	}

	if c.counter == nil {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "websocket connection counter not available (nil)"
		return result
	}

	count := c.counter.CountGlobalMonitorConns()
	result.Details = map[string]any{"monitor_connections": count}

	if count == 0 {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "no active WebSocket monitor connections"
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = fmt.Sprintf("websocket healthy, %d monitor connections", count)
	return result
}
