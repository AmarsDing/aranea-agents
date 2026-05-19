package event

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// FlowPhase marks the phase of a flow step.
type FlowPhase string

const (
	FlowPhaseStart FlowPhase = "start"
	FlowPhaseDone  FlowPhase = "done"
	FlowPhaseError FlowPhase = "error"
	FlowPhaseSkip  FlowPhase = "skip"
)

// FlowLogger publishes structured flow-step logs to the event bus (monitor
// channel) so the frontend can display real-time progress of a chat turn.
// It also writes to stderr for terminal observability.
//
// Unlike slog-based logging, FlowLogger publishes events directly to the bus,
// avoiding the SlogBridge reentrancy/deadlock risk inside plugin event handlers.
type FlowLogger struct {
	bus       Bus
	sessionID string
	agentKey  string

	// mu guards the timers map for step duration tracking.
	mu     sync.Mutex
	timers map[string]time.Time
}

// NewFlowLogger creates a FlowLogger bound to the given bus and session.
// If bus is nil, events are only written to stderr.
func NewFlowLogger(bus Bus, sessionID, agentKey string) *FlowLogger {
	return &FlowLogger{
		bus:       bus,
		sessionID: sessionID,
		agentKey:  agentKey,
		timers:    make(map[string]time.Time),
	}
}

// Log records a flow step. It publishes an EnvelopeTypeLog event to the
// "monitor" channel on the event bus and also writes to stderr.
func (f *FlowLogger) Log(step string, phase FlowPhase, msg string, extra ...Pair) {
	f.log(step, phase, msg, nil, extra)
}

// LogStart records the start of a step and begins timing it.
// Call LogDone or LogError with the same step to record duration.
func (f *FlowLogger) LogStart(step string, msg string, extra ...Pair) {
	f.mu.Lock()
	f.timers[step] = time.Now()
	f.mu.Unlock()
	f.log(step, FlowPhaseStart, msg, nil, extra)
}

// LogDone records the completion of a step started with LogStart,
// automatically computing the duration.
func (f *FlowLogger) LogDone(step string, msg string, extra ...Pair) {
	var durationMS *int64
	f.mu.Lock()
	if start, ok := f.timers[step]; ok {
		d := time.Since(start).Milliseconds()
		durationMS = &d
		delete(f.timers, step)
	}
	f.mu.Unlock()
	f.log(step, FlowPhaseDone, msg, durationMS, extra)
}

// LogError records an error for a step started with LogStart,
// automatically computing the duration.
func (f *FlowLogger) LogError(step string, msg string, extra ...Pair) {
	var durationMS *int64
	f.mu.Lock()
	if start, ok := f.timers[step]; ok {
		d := time.Since(start).Milliseconds()
		durationMS = &d
		delete(f.timers, step)
	}
	f.mu.Unlock()
	f.log(step, FlowPhaseError, msg, durationMS, extra)
}

// LogSkip records that a step was skipped.
func (f *FlowLogger) LogSkip(step string, msg string, extra ...Pair) {
	f.log(step, FlowPhaseSkip, msg, nil, extra)
}

func (f *FlowLogger) log(step string, phase FlowPhase, msg string, durationMS *int64, extra []Pair) {
	metadata := map[string]any{
		"flow_step":  step,
		"flow_phase": string(phase),
		"agent_key":  f.agentKey,
	}
	if durationMS != nil {
		metadata["duration_ms"] = *durationMS
	}
	for _, p := range extra {
		metadata[p.Key] = p.Value
	}

	// Build human-readable log line.
	var buf strings.Builder
	buf.WriteString("[flow] ")
	buf.WriteString(step)
	buf.WriteByte('.')
	buf.WriteString(string(phase))
	if msg != "" {
		buf.WriteString(": ")
		buf.WriteString(msg)
	}
	if durationMS != nil {
		fmt.Fprintf(&buf, " (%dms)", *durationMS)
	}
	for _, p := range extra {
		fmt.Fprintf(&buf, " %s=%v", p.Key, p.Value)
	}

	text := buf.String()

	// Always write to stderr for terminal observability.
	fmt.Fprintln(os.Stderr, text)
	os.Stderr.Sync()

	// Publish to event bus for frontend consumption.
	if f.bus != nil {
		env := NewEnvelope(EnvelopeTypeLog, "system", f.sessionID)
		env.Channel = "monitor"
		env.Content = &EnvelopeContent{
			Text:      text,
			IsPartial: false,
		}
		env.Metadata = metadata
		// Publish asynchronously to avoid blocking the caller.
		bus := f.bus
		go bus.Publish(context.Background(), env)
	}
}

// Pair is a key-value pair for extra metadata.
type Pair struct {
	Key   string
	Value any
}

// P is a shorthand for creating a Pair.
func P(key string, value any) Pair {
	return Pair{Key: key, Value: value}
}
