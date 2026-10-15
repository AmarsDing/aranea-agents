package agent

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"

	"github.com/redis/go-redis/v9"
)

// AgentLoadTracker is a process-local soft concurrent-run counter for
// AgentCapability.Capacity (DEV-03 / ORCH-02).
//
// When ConfigureCapacityRedis is called with a live client, Acquire/Release
// dual-write to Redis and SnapshotFor merges remote counts so Allocator soft
// gate (and optional hard block) stays coherent across admin replicas.
// Capacity <= 0 is treated as unlimited. Soft gate means Allocate prefers
// under-capacity agents and may still assign when every candidate is busy
// unless CapacityHardGate is enabled (Redis wired).
type AgentLoadTracker struct {
	mu    sync.Mutex
	loads map[string]int // agentKey -> in-flight assignments
	redis *redisCapacityBackend
}

// CapacityHardGate is set true when a Redis capacity backend is configured.
// Soft gate then blocks keep-on-busy (clears AssignedKey) instead of annotating
// capacity_soft_kept.
var CapacityHardGate bool

// GlobalAgentLoadTracker is shared by Allocator soft-gate and PlanExecutor
// Acquire/Release around live team runs.
var GlobalAgentLoadTracker = NewAgentLoadTracker()

// NewAgentLoadTracker returns an empty tracker.
func NewAgentLoadTracker() *AgentLoadTracker {
	return &AgentLoadTracker{loads: make(map[string]int)}
}

// ConfigureCapacityRedis enables durable cross-instance capacity accounting.
// A nil client leaves the process-local soft gate only (CapacityHardGate=false).
func ConfigureCapacityRedis(client *redis.Client) {
	if GlobalAgentLoadTracker == nil {
		GlobalAgentLoadTracker = NewAgentLoadTracker()
	}
	GlobalAgentLoadTracker.mu.Lock()
	defer GlobalAgentLoadTracker.mu.Unlock()
	if client == nil {
		GlobalAgentLoadTracker.redis = nil
		CapacityHardGate = false
		return
	}
	GlobalAgentLoadTracker.redis = newRedisCapacityBackend(client)
	CapacityHardGate = true
}

// Load returns the current in-flight count for agentKey (process-local only).
func (t *AgentLoadTracker) Load(agentKey string) int {
	if t == nil || agentKey == "" {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loads[agentKey]
}

// AtCapacity reports whether agentKey is already at or over capacity.
// capacity <= 0 means unlimited. Uses process-local count only; prefer
// SnapshotFor when deciding Allocate soft/hard gate.
func (t *AgentLoadTracker) AtCapacity(agentKey string, capacity int) bool {
	if capacity <= 0 || agentKey == "" {
		return false
	}
	return t.Load(agentKey) >= capacity
}

// Acquire increments in-flight counts for the given agent keys (local + Redis).
func (t *AgentLoadTracker) Acquire(agentKeys ...string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.loads == nil {
		t.loads = make(map[string]int)
	}
	var keys []string
	for _, k := range agentKeys {
		if k == "" {
			continue
		}
		t.loads[k]++
		keys = append(keys, k)
	}
	backend := t.redis
	t.mu.Unlock()
	if backend != nil && len(keys) > 0 {
		backend.acquire(context.Background(), keys...)
	}
}

// Release decrements in-flight counts (floored at zero).
func (t *AgentLoadTracker) Release(agentKeys ...string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	var keys []string
	for _, k := range agentKeys {
		if k == "" {
			continue
		}
		if n := t.loads[k]; n <= 1 {
			delete(t.loads, k)
		} else {
			t.loads[k] = n - 1
		}
		keys = append(keys, k)
	}
	backend := t.redis
	t.mu.Unlock()
	if backend != nil && len(keys) > 0 {
		backend.release(context.Background(), keys...)
	}
}

// Snapshot copies current process-local loads for provisional Allocate planning.
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

// SnapshotFor returns max(local, redis) loads for the given agent keys so
// Allocator soft/hard gate sees cross-instance pressure when Redis is wired.
func (t *AgentLoadTracker) SnapshotFor(agentKeys []string) map[string]int {
	out := t.Snapshot()
	if t == nil {
		return out
	}
	t.mu.Lock()
	backend := t.redis
	t.mu.Unlock()
	if backend == nil {
		return out
	}
	ctx := context.Background()
	for _, k := range agentKeys {
		if k == "" {
			continue
		}
		if n := backend.load(ctx, k); n > out[k] {
			out[k] = n
		}
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
