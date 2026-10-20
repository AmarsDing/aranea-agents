package publish

import (
	"fmt"
	"sync"
)

type MemoryRepo struct {
	mu   sync.RWMutex
	byID map[string]Job
	byKey map[string]string
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{byID: map[string]Job{}, byKey: map[string]string{}}
}

func (r *MemoryRepo) Create(j Job) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[j.ID] = j
	if j.IdempotencyKey != "" {
		r.byKey[j.IdempotencyKey] = j.ID
	}
	return j, nil
}

func (r *MemoryRepo) Get(id string) (Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.byID[id]
	if !ok {
		return Job{}, fmt.Errorf("publish: not found %s", id)
	}
	return j, nil
}

func (r *MemoryRepo) Update(j Job) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[j.ID] = j
	return j, nil
}

func (r *MemoryRepo) ListByStatus(status string, limit int) ([]Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Job, 0)
	for _, j := range r.byID {
		if j.Status == status {
			out = append(out, j)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *MemoryRepo) GetByIdempotency(key string) (Job, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byKey[key]
	if !ok {
		return Job{}, false, nil
	}
	return r.byID[id], true, nil
}
