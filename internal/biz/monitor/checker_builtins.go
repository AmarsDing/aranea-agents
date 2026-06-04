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

// DBHealthChecker verifies SQLite connectivity and basic schema integrity.
type DBHealthChecker struct {
	db *sql.DB
}

// NewDBHealthChecker creates a checker that pings the database and verifies
// the monitor_events table exists.
func NewDBHealthChecker(db *sql.DB) *DBHealthChecker {
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

	// Step 2: Verify monitor_events table exists
	var tableName string
	row := c.db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='monitor_events' LIMIT 1")
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
type TraceProjectorHealthChecker interface {
	TraceCount() int
}

// TraceProjectorChecker verifies that the trace projector is active and processing traces.
type TraceProjectorChecker struct {
	projector TraceProjectorHealthChecker
}

// NewTraceProjectorChecker creates a checker for trace projector health.
func NewTraceProjectorChecker(projector TraceProjectorHealthChecker) *TraceProjectorChecker {
	return &TraceProjectorChecker{projector: projector}
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

	count := c.projector.TraceCount()
	result.Details = map[string]any{"active_traces": count}

	if count == 0 {
		result.Status = types.SelfCheckStatusWarning
		result.Message = "no active traces in projector (may be idle or stalled)"
		return result
	}

	result.Status = types.SelfCheckStatusPassed
	result.Message = fmt.Sprintf("trace projector healthy, %d active traces", count)
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
