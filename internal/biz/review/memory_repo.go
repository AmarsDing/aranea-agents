package review

import (
	"fmt"
	"sync"
)

type MemoryRepo struct {
	mu   sync.RWMutex
	byID map[string]Task
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{byID: map[string]Task{}}
}

func (r *MemoryRepo) Create(t Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[t.ID] = t
	return t, nil
}

func (r *MemoryRepo) Get(id string) (Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[id]
	if !ok {
		return Task{}, fmt.Errorf("review: not found %s", id)
	}
	return t, nil
}

func (r *MemoryRepo) Update(t Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID]; !ok {
		return Task{}, fmt.Errorf("review: not found %s", t.ID)
	}
	r.byID[t.ID] = t
	return t, nil
}

func (r *MemoryRepo) ListPending(limit, offset int) ([]Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Task, 0)
	for _, t := range r.byID {
		if t.Status == StatusPending {
			all = append(all, t)
		}
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
