package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event"
)

// flowLogPersistConsumer writes flow_log envelopes to flow_log_events.
type flowLogPersistConsumer struct {
	bus      event.Bus
	flowLogs *FlowLogUsecase
}

func newFlowLogPersistConsumer(bus event.Bus, flowLogs *FlowLogUsecase) *flowLogPersistConsumer {
	if flowLogs == nil {
		return nil
	}
	return &flowLogPersistConsumer{bus: bus, flowLogs: flowLogs}
}

func (c *flowLogPersistConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumer(ctx, "event-bus-flow-log", c.bus, event.SubscribeOptions{
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeFlowLog},
		BufferSize: 512,
		Reliable:   true,
	}, c.handle)
}

func (c *flowLogPersistConsumer) handle(ctx context.Context, env event.Envelope) {
	if c == nil || c.flowLogs == nil || env.Metadata == nil {
		return
	}
	m := env.Metadata
	flowID := metaStringFromFlowLog(m, "flow_id")
	if flowID == "" {
		flowID = env.ID
	}
	createdAt := time.Now().UTC()
	if ts := strings.TrimSpace(env.Timestamp); ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			createdAt = t.UTC()
		} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
			createdAt = t.UTC()
		}
	}
	payload, _ := json.Marshal(m)
	rec := FlowLogRecord{
		ID:          flowID,
		TraceID:     metaStringFromFlowLog(m, "trace_id"),
		SessionID:   coalesceNonEmpty(metaStringFromFlowLog(m, "session_id"), env.SessionID),
		RunID:       metaStringFromFlowLog(m, "run_id"),
		TeamID:      strings.TrimSpace(env.TeamID),
		Domain:      metaStringFromFlowLog(m, "domain"),
		AgentKey:    metaStringFromFlowLog(m, "agent_key"),
		StepID:      metaStringFromFlowLog(m, "step_id"),
		FlowPhase:   metaStringFromFlowLog(m, "flow_phase"),
		Severity:    metaStringFromFlowLog(m, "severity"),
		Title:       metaStringFromFlowLog(m, "title"),
		Message:     metaStringFromFlowLog(m, "message"),
		PayloadJSON: string(payload),
		CreatedAt:   createdAt,
	}
	if rec.Message == "" && env.Content != nil {
		rec.Message = strings.TrimSpace(env.Content.Text)
	}
	if err := c.flowLogs.Save(ctx, rec); err != nil {
		event.SessionSysLogWarn(ctx, rec.SessionID, "flow_log.persist", "流程日志落库失败",
			event.P("step_id", rec.StepID), event.P("error", err))
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
