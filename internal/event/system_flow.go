package event

import (
	"context"
	"fmt"
	"os"
)

func systemTraceContext(ctx context.Context, sessionID, agentKey string) TraceContext {
	domain := TraceDomainSystem
	opts := TraceOpts{Domain: domain, SessionID: sessionID, AgentKey: agentKey}
	if sessionID != "" {
		domain = TraceDomainChat
		opts.Domain = domain
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return NewTraceContext(ctx, opts)
}

func emitSystem(ctx context.Context, sessionID, agentKey, stepID string, phase FlowPhase, sev FlowSeverity, message string, extra []Pair) {
	entry := newFlowLogEntry(systemTraceContext(ctx, sessionID, agentKey), stepID, phase, sev, "", message, "", nil, nil, pairsToMap(extra))

	bus := monitorBusRef()
	if os.Getenv("FLOW_LOG_STDERR") == "1" || bus == nil {
		fmt.Fprintf(os.Stderr, "[flow][system] %s\n", entry.displayText())
		_ = os.Stderr.Sync()
	}

	if bus == nil {
		return
	}
	// Info-level high-frequency steps are stderr-only unless explicitly enabled.
	if sev == FlowSeverityInfo && shouldThrottleSystemFlow(stepID) {
		if os.Getenv("FLOW_LOG_BUS") != "1" {
			return
		}
		if !allowSystemFlowEmit(stepID) {
			return
		}
	}
	env := NewEnvelope(EnvelopeTypeFlowLog, "system", sessionID)
	env.Channel = "monitor"
	env.Content = &EnvelopeContent{Text: entry.displayText(), IsPartial: false}
	env.Metadata = entry.toMetadata()
	// Synchronous publish avoids goroutine storms during streaming turns.
	bus.Publish(context.Background(), env)
}

func pairsToMap(extra []Pair) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	m := make(map[string]any, len(extra))
	for _, p := range extra {
		m[p.Key] = p.Value
	}
	return m
}

// SysLogInfo emits a system-domain informational flow log.
//
// Deprecated: use loggateway.Logger.Info(msg, loggateway.StepID(stepID), ...) instead.
func SysLogInfo(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseDone, FlowSeverityInfo, message, extra)
}

// SysLogWarn emits a system-domain warning flow log.
//
// Deprecated: use loggateway.Logger.Warn(msg, loggateway.StepID(stepID), ...) instead.
func SysLogWarn(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseDone, FlowSeverityWarn, message, extra)
}

// SysLogError emits a system-domain error flow log.
//
// Deprecated: use loggateway.Logger.Error(msg, loggateway.StepID(stepID), ...) instead.
func SysLogError(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseError, FlowSeverityError, message, extra)
}

// SysLogDebug emits a low-noise debug flow log (info severity, skip Monitor highlight).
//
// Deprecated: use loggateway.Logger.Debug(msg, loggateway.StepID(stepID), ...) instead.
func SysLogDebug(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseStart, FlowSeverityInfo, message, extra)
}

// SessionSysLogWarn attaches session context for Monitor filtering.
//
// Deprecated: use lg.With(loggateway.SessionID(sessionID)).Warn(msg, loggateway.StepID(stepID), ...) instead.
func SessionSysLogWarn(ctx context.Context, sessionID, stepID, message string, extra ...Pair) {
	emitSystem(ctx, sessionID, "", stepID, FlowPhaseDone, FlowSeverityWarn, message, extra)
}

// SessionSysLogInfo emits an ok-severity flow log scoped to a session.
//
// Deprecated: use lg.With(loggateway.SessionID(sessionID)).Info(msg, loggateway.StepID(stepID), ...) instead.
func SessionSysLogInfo(ctx context.Context, sessionID, stepID, message string, extra ...Pair) {
	emitSystem(ctx, sessionID, "", stepID, FlowPhaseDone, FlowSeverityOK, message, extra)
}

// SessionSysLogError emits an error-severity flow log scoped to a session.
//
// Deprecated: use lg.With(loggateway.SessionID(sessionID)).Error(msg, loggateway.StepID(stepID), ...) instead.
func SessionSysLogError(ctx context.Context, sessionID, stepID, message string, extra ...Pair) {
	emitSystem(ctx, sessionID, "", stepID, FlowPhaseError, FlowSeverityError, message, extra)
}
