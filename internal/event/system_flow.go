package event

import (
	"context"
	"fmt"
	"os"
	"sync"

	"aranea-agents/pkg/safego"
)

var (
	globalBus   Bus
	globalBusMu sync.RWMutex
)

// SetGlobalBus wires the process-wide event bus for system-domain flow logs.
// Call once from app startup (replaces InstallSlogBridge).
func SetGlobalBus(b Bus) {
	globalBusMu.Lock()
	globalBus = b
	globalBusMu.Unlock()
}

func globalBusRef() Bus {
	globalBusMu.RLock()
	defer globalBusMu.RUnlock()
	return globalBus
}

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

	if os.Getenv("FLOW_LOG_STDERR") == "1" || globalBusRef() == nil {
		fmt.Fprintf(os.Stderr, "[flow][system] %s\n", entry.displayText())
		_ = os.Stderr.Sync()
	}

	bus := globalBusRef()
	if bus == nil {
		return
	}
	env := NewEnvelope(EnvelopeTypeFlowLog, "system", sessionID)
	env.Channel = "monitor"
	env.Content = &EnvelopeContent{Text: entry.displayText(), IsPartial: false}
	env.Metadata = entry.toMetadata()
	safego.Go(context.Background(), "system-flow-publish", func() {
		bus.Publish(context.Background(), env)
	})
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
func SysLogInfo(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseDone, FlowSeverityInfo, message, extra)
}

// SysLogWarn emits a system-domain warning flow log.
func SysLogWarn(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseDone, FlowSeverityWarn, message, extra)
}

// SysLogError emits a system-domain error flow log.
func SysLogError(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseError, FlowSeverityError, message, extra)
}

// SysLogDebug emits a low-noise debug flow log (info severity, skip Monitor highlight).
func SysLogDebug(stepID, message string, extra ...Pair) {
	emitSystem(context.Background(), "", "", stepID, FlowPhaseStart, FlowSeverityInfo, message, extra)
}

// SessionSysLogWarn attaches session context for Monitor filtering.
func SessionSysLogWarn(ctx context.Context, sessionID, stepID, message string, extra ...Pair) {
	emitSystem(ctx, sessionID, "", stepID, FlowPhaseDone, FlowSeverityWarn, message, extra)
}

// SessionSysLogInfo emits an ok-severity flow log scoped to a session.
func SessionSysLogInfo(ctx context.Context, sessionID, stepID, message string, extra ...Pair) {
	emitSystem(ctx, sessionID, "", stepID, FlowPhaseDone, FlowSeverityOK, message, extra)
}
