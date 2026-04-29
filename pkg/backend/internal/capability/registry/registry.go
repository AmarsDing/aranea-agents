package registry

import (
	"sort"
	"strings"
	"sync"

	"arenea/backend/internal/capability/tooldef"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]tooldef.Tool
}

func New() *Registry {
	return &Registry{tools: map[string]tooldef.Tool{}}
}

func (r *Registry) Register(t tooldef.Tool) {
	if r == nil || t == nil || strings.TrimSpace(t.Name()) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (tooldef.Tool, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[strings.TrimSpace(name)]
	return t, ok
}

func (r *Registry) List() []tooldef.Tool {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tooldef.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}
