package commerce

import (
	"fmt"
	"sync"
)

type MemoryOpportunityRepo struct {
	mu   sync.RWMutex
	byID map[string]Opportunity
}

func NewMemoryOpportunityRepo() *MemoryOpportunityRepo {
	return &MemoryOpportunityRepo{byID: map[string]Opportunity{}}
}

func (r *MemoryOpportunityRepo) Create(o Opportunity) (Opportunity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[o.ID] = o
	return o, nil
}
func (r *MemoryOpportunityRepo) Get(id string) (Opportunity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.byID[id]
	if !ok {
		return Opportunity{}, fmt.Errorf("commerce: opportunity not found %s", id)
	}
	return o, nil
}
func (r *MemoryOpportunityRepo) Update(o Opportunity) (Opportunity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[o.ID] = o
	return o, nil
}
func (r *MemoryOpportunityRepo) List(audience, status string, limit, offset int) ([]Opportunity, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Opportunity, 0)
	for _, o := range r.byID {
		if audience != "" && o.Audience != audience && o.Audience != AudienceBoth {
			continue
		}
		if status != "" && o.Status != status {
			continue
		}
		all = append(all, o)
	}
	total := len(all)
	if offset > len(all) {
		return nil, total, nil
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total, nil
}

type MemorySupplierRepo struct {
	mu   sync.RWMutex
	byID map[string]SupplierCandidate
}

func NewMemorySupplierRepo() *MemorySupplierRepo {
	return &MemorySupplierRepo{byID: map[string]SupplierCandidate{}}
}

func (r *MemorySupplierRepo) Create(s SupplierCandidate) (SupplierCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.ID] = s
	return s, nil
}
func (r *MemorySupplierRepo) Get(id string) (SupplierCandidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	if !ok {
		return SupplierCandidate{}, fmt.Errorf("commerce: supplier not found %s", id)
	}
	return s, nil
}
func (r *MemorySupplierRepo) Update(s SupplierCandidate) (SupplierCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.ID] = s
	return s, nil
}
func (r *MemorySupplierRepo) ListByOpportunity(opportunityID string) ([]SupplierCandidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SupplierCandidate, 0)
	for _, s := range r.byID {
		if s.OpportunityID == opportunityID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *MemorySupplierRepo) List(limit, offset int) ([]SupplierCandidate, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]SupplierCandidate, 0, len(r.byID))
	for _, s := range r.byID {
		all = append(all, s)
	}
	total := len(all)
	if offset > len(all) {
		return nil, total, nil
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total, nil
}
