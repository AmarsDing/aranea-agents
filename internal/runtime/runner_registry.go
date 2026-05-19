package runtime

import (
	"sync"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// RunnerInstanceRegistry tracks optional long-lived trpc runners by caller key.
// This is separate from RunRegistry, which tracks per-session gateway state.
type RunnerInstanceRegistry struct {
	mu      sync.Mutex
	runners map[string]trpcrunner.Runner
}

func NewRunnerInstanceRegistry() *RunnerInstanceRegistry {
	return &RunnerInstanceRegistry{
		runners: make(map[string]trpcrunner.Runner),
	}
}

func (r *RunnerInstanceRegistry) Register(key string, runner trpcrunner.Runner) {
	if r == nil || key == "" || runner == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runners[key] = runner
}

// Replace registers runner and returns any previous instance for the same key.
func (r *RunnerInstanceRegistry) Replace(key string, runner trpcrunner.Runner) (trpcrunner.Runner, bool) {
	if r == nil || key == "" || runner == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, ok := r.runners[key]
	r.runners[key] = runner
	return prev, ok
}

func (r *RunnerInstanceRegistry) Get(key string) (trpcrunner.Runner, bool) {
	if r == nil || key == "" {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	runner, ok := r.runners[key]
	return runner, ok
}

func (r *RunnerInstanceRegistry) Unregister(key string) (trpcrunner.Runner, bool) {
	if r == nil || key == "" {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	runner, ok := r.runners[key]
	delete(r.runners, key)
	return runner, ok
}

func (r *RunnerInstanceRegistry) List() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.runners))
	for k := range r.runners {
		keys = append(keys, k)
	}
	return keys
}
