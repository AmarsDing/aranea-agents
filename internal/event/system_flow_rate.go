package event

import (
	"strings"
	"sync"
	"time"
)

const systemFlowMinInterval = 500 * time.Millisecond

var systemFlowRate = struct {
	mu    sync.Mutex
	last  map[string]time.Time
	count map[string]uint64
}{
	last:  make(map[string]time.Time),
	count: make(map[string]uint64),
}

func shouldThrottleSystemFlow(stepID string) bool {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return false
	}
	// High-frequency audit/stream steps must not flood the monitor bus.
	switch {
	case strings.HasPrefix(stepID, "plugin.audit_log"):
		return true
	case strings.Contains(stepID, "chat.completion"):
		return true
	case strings.HasPrefix(stepID, "chat.llm"):
		return true
	default:
		return false
	}
}

func allowSystemFlowEmit(stepID string) bool {
	if !shouldThrottleSystemFlow(stepID) {
		return true
	}
	now := time.Now()
	systemFlowRate.mu.Lock()
	defer systemFlowRate.mu.Unlock()
	prev, ok := systemFlowRate.last[stepID]
	if ok && now.Sub(prev) < systemFlowMinInterval {
		systemFlowRate.count[stepID]++
		return false
	}
	if n := systemFlowRate.count[stepID]; n > 0 {
		// Coalesce skipped bursts into the next allowed emit (handled by caller via metadata if needed).
		systemFlowRate.count[stepID] = 0
	}
	systemFlowRate.last[stepID] = now
	return true
}
