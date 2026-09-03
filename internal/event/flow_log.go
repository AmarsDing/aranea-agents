package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const FlowLogSchemaVersion = "flow_log/v1"

// FlowPhase marks the phase of a flow step.
type FlowPhase string

const (
	FlowPhaseStart FlowPhase = "start"
	FlowPhaseDone  FlowPhase = "done"
	FlowPhaseError FlowPhase = "error"
	FlowPhaseSkip  FlowPhase = "skip"
)

// Pair is a key-value pair for extra metadata.
type Pair struct {
	Key   string
	Value any
}

// P is a shorthand for creating a Pair.
func P(key string, value any) Pair {
	return Pair{Key: key, Value: value}
}

// FlowSeverity is the user-facing alert level (red/yellow/green).
type FlowSeverity string

const (
	FlowSeverityOK       FlowSeverity = "ok"
	FlowSeverityInfo     FlowSeverity = "info"
	FlowSeverityWarn     FlowSeverity = "warn"
	FlowSeverityError    FlowSeverity = "error"
	FlowSeverityCritical FlowSeverity = "critical"
)

// FlowCorrelation ties a log entry to a trace/run.
type FlowCorrelation struct {
	TraceID   string `json:"trace_id"`
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
	TeamID    string `json:"team_id,omitempty"`
	Domain    string `json:"domain"`
	AgentKey  string `json:"agent_key,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

// FlowStep describes the step identity and lifecycle phase.
type FlowStep struct {
	ID        string `json:"id"`
	Phase     string `json:"phase"`
	Subsystem string `json:"subsystem,omitempty"`
}

// FlowTiming holds optional duration data.
type FlowTiming struct {
	DurationMS int64  `json:"duration_ms,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

// FlowError is set when severity is error or critical.
type FlowError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// FlowLogEntry is the v2 flow log payload (schema flow_log/v1).
type FlowLogEntry struct {
	SchemaVersion string          `json:"schema_version"`
	ID            string          `json:"id"`
	Timestamp     string          `json:"timestamp"`
	Correlation   FlowCorrelation `json:"correlation"`
	Step          FlowStep        `json:"step"`
	Severity      FlowSeverity    `json:"severity"`
	Title         string          `json:"title"`
	Message       string          `json:"message"`
	Hint          string          `json:"hint,omitempty"`
	Timing        *FlowTiming     `json:"timing,omitempty"`
	Error         *FlowError      `json:"error,omitempty"`
	// SpanID is the OTel span ID of the turn root, enabling cross-reference
	// between FlowLog and OTel trace (Jaeger). Populated via SetOtelRefs.
	// Empty when OTel tracing is not configured. Phase 1 of Problem 4.
	SpanID string `json:"span_id,omitempty"`
	// ParentSpanID is the OTel parent span ID of the turn root. Empty for
	// turn-root spans (no upstream OTel parent). Reserved for future phases
	// that may populate per-step parent linkage.
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// stepTitleRegistry 已拆分至 flow_log_steps.go（AS-COG-01：本文件行数封顶 500）。

func stepTitle(stepID string) string {
	if t, ok := stepTitleRegistry[stepID]; ok {
		return t
	}
	// Dynamic-suffix step IDs (e.g. "team.member.member-1") fall back to
	// their static prefix ("team.member") for title resolution.
	if i := strings.LastIndex(stepID, "."); i > 0 {
		if t, ok := stepTitleRegistry[stepID[:i]]; ok {
			return t
		}
	}
	return stepID
}

func stepSubsystem(stepID string) string {
	parts := strings.SplitN(stepID, ".", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func severityForPhase(phase FlowPhase, explicit FlowSeverity) FlowSeverity {
	if explicit != "" {
		return explicit
	}
	switch phase {
	case FlowPhaseError:
		return FlowSeverityError
	case FlowPhaseSkip:
		return FlowSeverityWarn
	case FlowPhaseDone:
		return FlowSeverityOK
	case FlowPhaseStart:
		return FlowSeverityInfo
	default:
		return FlowSeverityInfo
	}
}

func (e FlowLogEntry) displayText() string {
	var b strings.Builder
	if e.Title != "" {
		b.WriteString(e.Title)
	}
	if e.Message != "" {
		if b.Len() > 0 {
			b.WriteString(" — ")
		}
		b.WriteString(e.Message)
	}
	if e.Timing != nil && e.Timing.DurationMS > 0 {
		fmt.Fprintf(&b, " (%dms)", e.Timing.DurationMS)
	}
	if b.Len() == 0 {
		b.WriteString(e.Step.ID)
		b.WriteByte('.')
		b.WriteString(e.Step.Phase)
	}
	return b.String()
}

func (e FlowLogEntry) toMetadata() map[string]any {
	m := map[string]any{
		"schema_version": e.SchemaVersion,
		"flow_id":        e.ID,
		"trace_id":       e.Correlation.TraceID,
		"session_id":     e.Correlation.SessionID,
		"run_id":         e.Correlation.RunID,
		"domain":         e.Correlation.Domain,
		"agent_key":      e.Correlation.AgentKey,
		"agent_id":       e.Correlation.AgentID,
		"step_id":        e.Step.ID,
		"flow_phase":     e.Step.Phase,
		"severity":       string(e.Severity),
		"title":          e.Title,
		"message":        e.Message,
	}
	if e.SpanID != "" {
		m["span_id"] = e.SpanID
	}
	if e.ParentSpanID != "" {
		m["parent_span_id"] = e.ParentSpanID
	}
	if e.Hint != "" {
		m["hint"] = e.Hint
	}
	if e.Timing != nil {
		if e.Timing.DurationMS > 0 {
			m["duration_ms"] = e.Timing.DurationMS
		}
	}
	if e.Error != nil {
		m["error_code"] = e.Error.Code
		m["error_message"] = e.Error.Message
	}
	for k, v := range e.Extra {
		m[k] = v
	}
	return m
}

func newFlowLogEntry(tc TraceContext, rootSpanID, stepID string, phase FlowPhase, sev FlowSeverity, title, message, hint string, timing *FlowTiming, flowErr *FlowError, extra map[string]any) FlowLogEntry {
	if title == "" {
		title = stepTitle(stepID)
	}
	return FlowLogEntry{
		SchemaVersion: FlowLogSchemaVersion,
		ID:            "fl_" + uuid.NewString(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Correlation: FlowCorrelation{
			TraceID:   tc.TraceID,
			SessionID: tc.SessionID,
			RunID:     tc.RunID,
			TeamID:    tc.TeamID,
			Domain:    string(tc.Domain),
			AgentKey:  tc.AgentKey,
			AgentID:   tc.AgentID,
		},
		Step: FlowStep{
			ID:        stepID,
			Phase:     string(phase),
			Subsystem: stepSubsystem(stepID),
		},
		Severity: severityForPhase(phase, sev),
		Title:    title,
		Message:  message,
		Hint:     hint,
		Timing:   timing,
		Error:    flowErr,
		SpanID:   rootSpanID,
		Extra:    extra,
	}
}
