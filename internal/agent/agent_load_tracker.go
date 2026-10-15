package agent

import (
	"sync"

	"aranea-agents/internal/biz"
)

// AgentLoadTracker is a process-local soft concurrent-run counter for
// AgentCapability.Capacity (DEV-03 / ORCH-02).
//
// It is intentionally in-memory: single-process admin is the production
// deployment shape, and a durable cross-instance counter is not wired yet.
// Capacity <= 0 is treated as unlimited. Soft gate means Allocate prefers
// under-capacity agents and may still assign when every candidate is busy.
type AgentLoadTracker struct {
	mu    sync.Mutex
	loads map[string]int // agentKey -> in-flight assignments
}

// GlobalAgentLoadTracker is shared by Allocator soft-gate and PlanExecutor
// Acquire/Release around live team runs.
var GlobalAgentLoadTracker = NewAgentLoadTracker()

// NewAgentLoadTracker returns an empty tracker.
func NewAgentLoadTracker() *AgentLoadTracker {
	return &AgentLoadTracker{loads: make(map[string]int)}
}

// Load returns the current in-flight count for agentKey.
func (t *AgentLoadTracker) Load(agentKey string) int {
	if t == nil || agentKey == "" {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loads[agentKey]
}

// AtCapacity reports whether agentKey is already at or over capacity.
// capacity <= 0 means unlimited.
func (t *AgentLoadTracker) AtCapacity(agentKey string, capacity int) bool {
	if capacity <= 0 || agentKey == "" {
		return false
	}
	return t.Load(agentKey) >= capacity
}

// Acquire increments in-flight counts for the given agent keys.
func (t *AgentLoadTracker) Acquire(agentKeys ...string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loads == nil {
		t.loads = make(map[string]int)
	}
	for _, k := range agentKeys {
		if k == "" {
			continue
		}
		t.loads[k]++
	}
}

// Release decrements in-flight counts (floored at zero).
func (t *AgentLoadTracker) Release(agentKeys ...string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range agentKeys {
		if k == "" {
			continue
		}
		if n := t.loads[k]; n <= 1 {
			delete(t.loads, k)
		} else {
			t.loads[k] = n - 1
		}
	}
}

// Snapshot copies current loads for provisional Allocate planning.
func (t *AgentLoadTracker) Snapshot() map[string]int {
	out := make(map[string]int)
	if t == nil {
		return out
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, v := range t.loads {
		out[k] = v
	}
	return out
}

// capacityByKey builds agentKey -> Capacity from a capability roster.
func capacityByKey(caps []biz.AgentCapability) map[string]int {
	out := make(map[string]int, len(caps))
	for _, c := range caps {
		if c.AgentKey == "" {
			continue
		}
		out[c.AgentKey] = c.Capacity
	}
	return out
}

// softGateBusy reports whether assigning agentKey would exceed capacity given
// provisional loads (plan-local) on top of the tracker snapshot.
func softGateBusy(loads map[string]int, caps map[string]int, agentKey string) bool {
	if agentKey == "" {
		return false
	}
	capN, ok := caps[agentKey]
	if !ok || capN <= 0 {
		return false
	}
	return loads[agentKey] >= capN
}

// noteAssignment increments provisional loads for lead + members.
func noteAssignment(loads map[string]int, alloc biz.TaskAllocation) {
	if alloc.AssignedKey != "" {
		loads[alloc.AssignedKey]++
	}
	for _, k := range alloc.TeamMemberKeys {
		if k != "" {
			loads[k]++
		}
	}
}