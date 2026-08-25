package sandbox

import "sync"

// Quota is the sandbox analog of the token 双闸 (design §6.3): a global
// concurrency gate plus a per-agent concurrency gate. Only LEASED
// (attributed) instances count; warm-pool instances are bounded separately
// by pool max_ready. The per-run creation budget lands with P2 team wiring.
type Quota struct {
	mu           sync.Mutex
	globalMax    int
	perAgentMax  int
	globalActive int
	agentActive  map[string]int
}

func NewQuota(globalMax, perAgentMax int) *Quota {
	return &Quota{
		globalMax:   globalMax,
		perAgentMax: perAgentMax,
		agentActive: map[string]int{},
	}
}

// Admit reserves one slot. On rejection it returns a *QuotaError naming the
// scope that fired; on success the caller MUST later call Release exactly
// once (after destroy completes).
func (q *Quota) Admit(agentKey string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.globalMax > 0 && q.globalActive >= q.globalMax {
		return &QuotaError{Scope: QuotaScopeGlobal, Limit: q.globalMax}
	}
	if q.perAgentMax > 0 && agentKey != "" && q.agentActive[agentKey] >= q.perAgentMax {
		return &QuotaError{Scope: QuotaScopeAgent, Limit: q.perAgentMax}
	}
	q.globalActive++
	if agentKey != "" {
		q.agentActive[agentKey]++
	}
	return nil
}

// Release frees one slot previously reserved by Admit.
func (q *Quota) Release(agentKey string) {
	q.mu.Lock()
	defer q.mu.Unlock()
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
