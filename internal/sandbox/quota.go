package sandbox

import "sync"

// Quota is the sandbox analog of the token 双闸 (design §6.3): a global
// concurrency gate plus a per-agent concurrency gate plus a per-run
// cumulative-creation budget (P2-2). Only LEASED (attributed) instances count
// against concurrency; warm-pool instances are bounded separately by pool
// max_ready. The run budget counts successful creations cumulatively and is
// dropped via ReleaseRun when the owning team run ends — it is NOT freed by
// instance destroy (same semantics as the run-level token budget).
type Quota struct {
	mu           sync.Mutex
	globalMax    int
	perAgentMax  int
	perRunMax    int
	globalActive int
	agentActive  map[string]int
	runCreated   map[string]int
}

func NewQuota(globalMax, perAgentMax, perRunMax int) *Quota {
	return &Quota{
		globalMax:   globalMax,
		perAgentMax: perAgentMax,
		perRunMax:   perRunMax,
		agentActive: map[string]int{},
		runCreated:  map[string]int{},
	}
}

// Admit reserves one concurrency slot and, when runID is given, counts one
// creation against the run budget. On rejection it returns a *QuotaError
// naming the scope that fired; on success the caller MUST later call Release
// exactly once (after destroy completes), or UndoAdmit if the create failed.
func (q *Quota) Admit(agentKey, runID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.globalMax > 0 && q.globalActive >= q.globalMax {
		return &QuotaError{Scope: QuotaScopeGlobal, Limit: q.globalMax}
	}
	if q.perAgentMax > 0 && agentKey != "" && q.agentActive[agentKey] >= q.perAgentMax {
		return &QuotaError{Scope: QuotaScopeAgent, Limit: q.perAgentMax}
	}
	if q.perRunMax > 0 && runID != "" && q.runCreated[runID] >= q.perRunMax {
		return &QuotaError{Scope: QuotaScopeRun, Limit: q.perRunMax}
	}
	q.globalActive++
	if agentKey != "" {
		q.agentActive[agentKey]++
	}
	if runID != "" {
		q.runCreated[runID]++
	}
	return nil
}

// Release frees one concurrency slot previously reserved by Admit (destroy
// path). The run creation budget is intentionally NOT decremented — it is a
// cumulative budget, not a concurrency slot.
func (q *Quota) Release(agentKey string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.releaseConcurrencyLocked(agentKey)
}

// UndoAdmit rolls back a successful Admit whose sandbox creation failed: the
// concurrency slot is freed AND the run creation counter is decremented, so
// failed creations do not burn the run budget.
func (q *Quota) UndoAdmit(agentKey, runID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.releaseConcurrencyLocked(agentKey)
	if runID != "" && q.runCreated[runID] > 0 {
		q.runCreated[runID]--
		if q.runCreated[runID] == 0 {
			delete(q.runCreated, runID)
		}
	}
}

func (q *Quota) releaseConcurrencyLocked(agentKey string) {
	if q.globalActive > 0 {
		q.globalActive--
	}
	if agentKey != "" && q.agentActive[agentKey] > 0 {
		q.agentActive[agentKey]--
		if q.agentActive[agentKey] == 0 {
			delete(q.agentActive, agentKey)
		}
	}
}

// ReleaseRun drops the run's cumulative-creation counter (team run end).
// Instances already created keep their concurrency slots until destroyed.
func (q *Quota) ReleaseRun(runID string) {
	if runID == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.runCreated, runID)
}

// Snapshot reports current usage (metrics).
func (q *Quota) Snapshot() (globalActive int, agentActive map[string]int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]int, len(q.agentActive))
	for k, v := range q.agentActive {
		out[k] = v
	}
	return q.globalActive, out
}

// RunSnapshot reports per-run cumulative creation counts (metrics/debug).
func (q *Quota) RunSnapshot() map[string]int {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]int, len(q.runCreated))
	for k, v := range q.runCreated {
		out[k] = v
	}
	return out
}
