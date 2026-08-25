package sandbox

import (
	"sync"
	"time"
)

// entry is the registry record for one live sandbox.
type entry struct {
	view       LeaseView
	handle     Handle
	entryState LeaseState // mirrors view.State; guarded by registry mu
	quotaHeld  bool       // true while a quota slot is reserved for this entry
}

// Registry is the in-memory lease table + state machine (ADR-82-4: no DB).
// All state transitions are serialized under mu; LEASED/READY → DESTROYING
// is a compare-and-swap so concurrent destroy paths fire exactly once.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// register inserts a new entry in the given state.
func (r *Registry) register(e *entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.view.SandboxID] = e
}

// get returns the entry for id.
func (r *Registry) get(id string) (*entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	return e, ok
}

// claimReady atomically moves a ready pool instance of profile to leased
// with the given attribution. Returns nil when no ready instance exists.
func (r *Registry) claimReady(profile, agentKey, sessionID, runID string, deadline time.Time) *entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.entryState != StateReady || e.view.Profile != profile {
			continue
		}
		e.entryState = StateLeased
		e.view.State = StateLeased
		e.view.AgentKey = agentKey
		e.view.SessionID = sessionID
		e.view.RunID = runID
		e.view.Deadline = deadline
		e.view.LastExecAt = time.Time{}
		e.view.ExecCount = 0
		e.quotaHeld = true
		return e
	}
	return nil
}

// transitionDestroying CAS-moves id from ready|leased into destroying.
// Returns the entry and true only for the first caller (single-fire).
func (r *Registry) transitionDestroying(id string) (*entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok || e.entryState == StateDestroying {
		return nil, false
	}
	e.entryState = StateDestroying
	e.view.State = StateDestroying
	return e, true
}

// remove drops id from the table (called after engine destroy completes).
func (r *Registry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, id)
}

// touch records one execution (count + timestamp) for a leased entry.
func (r *Registry) touch(id string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[id]; ok && e.entryState == StateLeased {
		e.view.ExecCount++
		e.view.LastExecAt = at
	}
}

// renewDeadline extends the deadline of a leased entry, capped at maxDeadline.
func (r *Registry) renewDeadline(id string, extend time.Duration, maxDeadline time.Time) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok || e.entryState != StateLeased {
		return time.Time{}, false
	}
	nd := e.view.Deadline.Add(extend)
	if nd.After(maxDeadline) {
		nd = maxDeadline
	}
	e.view.Deadline = nd
	return nd, true
}

// list returns snapshots of all live entries.
func (r *Registry) list() []LeaseView {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LeaseView, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.view)
	}
	return out
}

// listEntries returns a copy of the live entry set (GC scans).
func (r *Registry) listEntries() []*entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// countReady returns the number of warm-pool instances for profile.
func (r *Registry) countReady(profile string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.entries {
		if e.entryState == StateReady && e.view.Profile == profile {
			n++
		}
	}
	return n
}

// countLeasedByProfile maps profile → leased count (active gauge).
func (r *Registry) countLeasedByProfile() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for _, e := range r.entries {
		if e.entryState == StateLeased {
			out[e.view.Profile]++
		}
	}
	return out
}
