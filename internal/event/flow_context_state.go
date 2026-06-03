package event

import (
	"sync"
	"time"
)

// FlowContext holds flow step timing data with its own mutex.
type FlowContext struct {
	mu     sync.Mutex
	timers map[string]time.Time
}

// NewFlowContext creates a FlowContext with initialized timers map.
func NewFlowContext() *FlowContext {
	return &FlowContext{
		timers: make(map[string]time.Time),
	}
}

// RecordStart stores the start time for a step.
func (fc *FlowContext) RecordStart(stepID string) {
	fc.mu.Lock()
	fc.timers[stepID] = time.Now()
	fc.mu.Unlock()
}

// TakeTiming returns and removes the timing for a step.
func (fc *FlowContext) TakeTiming(stepID string) *FlowTiming {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if start, ok := fc.timers[stepID]; ok {
		delete(fc.timers, stepID)
		return &FlowTiming{DurationMS: time.Since(start).Milliseconds()}
	}
	return nil
}
