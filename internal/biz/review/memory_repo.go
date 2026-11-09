package review

import (
	"fmt"
	"sync"
	"time"
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
		return Task{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return t, nil
}

func (r *MemoryRepo) Update(t Task) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID]; !ok {
		return Task{}, fmt.Errorf("%w: %s", ErrNotFound, t.ID)
	}
	r.byID[t.ID] = t
	return t, nil
}

// Decide 在写锁内完成「读-校验-写」（进程内 CAS）：当前状态不等于 from
// 时返回 ErrConflict——并发决议恰一个成功。
func (r *MemoryRepo) Decide(id, from, to, reviewerID, note string, decidedAt time.Time) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return Task{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if t.Status != from {
		return Task{}, ErrConflict
	}
	t.Status = to
	t.ReviewerID = reviewerID
	t.DecisionNote = note
	t.DecidedAt = &decidedAt
	t.UpdatedAt = decidedAt
	r.byID[id] = t
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

func (r *MemoryRepo) GetByGraphExecution(executionID, nodeID string) (Task, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.byID {
		if t.GraphExecutionID == executionID && (nodeID == "" || t.GraphNodeID == nodeID) {
			return t, true, nil
		}
	}
	return Task{}, false, nil
}
