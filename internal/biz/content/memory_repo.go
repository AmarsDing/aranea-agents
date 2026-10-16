package content

import (
	"fmt"
	"sync"
)

// MemoryRepo is an in-process store for tests and MVP boot without Ent generate.
type MemoryRepo struct {
	mu    sync.RWMutex
	byID  map[string]Item
	byHash map[string]string // workspaceID|hash → id
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{byID: map[string]Item{}, byHash: map[string]string{}}
}

func hashKey(ws, h string) string { return ws + "|" + h }

func (r *MemoryRepo) Create(item Item) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[item.ID]; ok {
		return Item{}, fmt.Errorf("content: duplicate id %s", item.ID)
	}
	r.byID[item.ID] = item
	if item.DedupeHash != "" {
		r.byHash[hashKey(item.WorkspaceID, item.DedupeHash)] = item.ID
	}
	return item, nil
}

func (r *MemoryRepo) Get(id string) (Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	it, ok := r.byID[id]
	if !ok {
		return Item{}, fmt.Errorf("content: not found %s", id)
	}
	return it, nil
}

func (r *MemoryRepo) Update(item Item) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[item.ID]; !ok {
		return Item{}, fmt.Errorf("content: not found %s", item.ID)
	}
	r.byID[item.ID] = item
	return item, nil
}

func (r *MemoryRepo) List(f ListFilter) ([]Item, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Item, 0)
	for _, it := range r.byID {
		if f.WorkspaceID != "" && it.WorkspaceID != f.WorkspaceID {
			continue
		}
		if f.SourceType != "" && it.SourceType != f.SourceType {
			continue
		}
		if f.Status != "" && it.Status != f.Status {
			continue
		}
		out = append(out, it)
	}
	total := len(out)
	if f.Offset > len(out) {
		return nil, total, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, total, nil
}

func (r *MemoryRepo) GetByDedupeHash(workspaceID, hash string) (Item, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byHash[hashKey(workspaceID, hash)]
	if !ok {
		return Item{}, false, nil
	}
	return r.byID[id], true, nil
}
