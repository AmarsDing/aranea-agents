package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// flowLogPersistWarnInterval throttles the Save-failure Warn/FlowError
// emissions. When the DB is down every incoming flow event fails to persist;
// unthrottled, each failure would emit a process-log Warn AND a
// self-referential flow-log error event (which re-enters this consumer and
// fails again), doubling the storm volume.
const flowLogPersistWarnInterval = 10 * time.Second

// flowLogPersistConsumer writes flow_log MonitorEvents to flow_log_events.
//
// Phase 5 Blocker B: migrated from legacy Envelope-based MonitorBus to
// typed contract.MonitorBus. The flow_log publisher (event.FlowTracker.emit)
// publishes MonitorEvent{Type:MonitorEventTypeFlowLog, Metadata:<flow fields>}.
// This consumer filters at the bus level by Type==flow_log and extracts the
// same fields from Metadata. The Message field replaces the legacy
// Envelope.Content.Text fallback.
type flowLogPersistConsumer struct {
	bus      contract.MonitorBus
	flowLogs *FlowLogUsecase
	logger   SessionLogWriter
	flowLog  FlowLogWriter

	saveThrottle *loggateway.Throttle
}

func newFlowLogPersistConsumer(flowLogs *FlowLogUsecase, logger SessionLogWriter, bus contract.MonitorBus, flowLog FlowLogWriter) *flowLogPersistConsumer {
	if flowLogs == nil || bus == nil {
		return nil
	}
	return &flowLogPersistConsumer{
		bus:          bus,
		flowLogs:     flowLogs,
		logger:       logger,
		flowLog:      flowLog,
		saveThrottle: loggateway.NewThrottle(flowLogPersistWarnInterval),
	}
}

func (c *flowLogPersistConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	opts := contract.MonitorSubscribeOptions{
		BufferSize: 512,
		GlobalMode: true,
		Filter: func(ev contract.MonitorEvent) bool {
			return ev.Type == contract.MonitorEventTypeFlowLog
		},
	}
	offerOpts := offerOption[contract.MonitorEvent]{FallbackSync: true, FallbackFn: c.handle}
	runMonitorConsumerWithOpts(ctx, "event-bus-flow-log", c.bus, opts, c.handle, offerOpts, c.logger)
}

func (c *flowLogPersistConsumer) handle(ctx context.Context, ev contract.MonitorEvent) {
	if c == nil || c.flowLogs == nil || ev.Metadata == nil {
		return
	}
	m := ev.Metadata
	flowID := metaStringFromFlowLog(m, "flow_id")
	if flowID == "" {
		flowID = ev.ID
	}
	createdAt := ev.Timestamp
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	payload, _ := json.Marshal(m)
	rec := FlowLogRecord{
		ID:          flowID,
		TraceID:     metaStringFromFlowLog(m, "trace_id"),
		SessionID:   coalesceNonEmpty(metaStringFromFlowLog(m, "session_id"), ev.SessionID),
		RunID:       metaStringFromFlowLog(m, "run_id"),
		TeamID:      metaStringFromFlowLog(m, "team_id"),
		Domain:      metaStringFromFlowLog(m, "domain"),
		AgentKey:    metaStringFromFlowLog(m, "agent_key"),
		StepID:      metaStringFromFlowLog(m, "step_id"),
		FlowPhase:   metaStringFromFlowLog(m, "flow_phase"),
		Severity:    metaStringFromFlowLog(m, "severity"),
		Title:       metaStringFromFlowLog(m, "title"),
		Message:     metaStringFromFlowLog(m, "message"),
		PayloadJSON: string(payload),
		CreatedAt:   createdAt.UTC(),
	}
	if rec.Message == "" {
		rec.Message = strings.TrimSpace(ev.Message)
	}
	if err := c.flowLogs.Save(ctx, rec); err != nil {
		// P2: throttle the failure emissions. Only the log lines are
		// throttled — every record still attempts the Save above, so no
		// recovery is delayed once the DB comes back.
		ok, suppressed := c.saveThrottle.Allow()
		if !ok {
			return
		}
		pairs := []LogPair{{Key: "step_id", Value: rec.StepID}, {Key: "error", Value: err}}
		if suppressed > 0 {
			pairs = append(pairs, LogPair{Key: "suppressed_failures", Value: suppressed})
		}
		if c.logger != nil {
			c.logger.LogSessionWarn(ctx, rec.SessionID, "flow_log.persist", "流程日志落库失败", pairs...)
		}
		// Recursion guard: the emitted flow log re-enters this consumer via the
		// bus; if it is itself an event_bus.flow_log.persist record, persisting
		// it would fail again and loop forever. Each failed record may emit at
		// most one self-referential flow log, which then stops here.
		if c.flowLog != nil && rec.StepID != "event_bus.flow_log.persist" {
			// The flow-log payload is JSON-marshaled downstream; an error
			// value would serialize as "{}", so pass the string form here.
			flowPairs := make([]LogPair, 0, len(pairs))
			for _, p := range pairs {
				if p.Key == "error" {
					p.Value = err.Error()
				}
				flowPairs = append(flowPairs, p)
			}
			c.flowLog.LogFlowError(ctx, rec.SessionID, "event_bus.flow_log.persist", "流程日志落库失败", flowPairs...)
		}
	}
}

func metaStringFromFlowLog(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toString(v))
}

func toString(v any) string {
	return fmt.Sprint(v)
}

func coalesceNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
