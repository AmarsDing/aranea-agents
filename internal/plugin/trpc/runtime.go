package plugintrpc

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// Runtime manages the set of active trpc-agent-go plugin.Plugin instances
// derived from enabled biz.Plugin DB rows.  It is safe for concurrent use.
//
// Call Apply whenever the enabled state of any plugin changes; the next runner
// creation will pick up the updated plugin slice automatically.
type Runtime struct {
	mu     sync.RWMutex
	active []trpcplugin.Plugin
}

// NewRuntime creates an empty Runtime (no active plugins).
func NewRuntime() *Runtime { return &Runtime{} }

// Apply replaces the active plugin set from the supplied DB snapshot.
// Only enabled plugins with a known built-in key are instantiated.
func (rt *Runtime) Apply(_ context.Context, plugins []biz.Plugin) {
	built := make([]trpcplugin.Plugin, 0, len(plugins))
	for _, p := range plugins {
		if tp := adapt(p); tp != nil {
			built = append(built, tp)
		}
	}
	rt.mu.Lock()
	rt.active = built
	rt.mu.Unlock()
}

// Plugins returns a snapshot of the currently active plugin instances.
func (rt *Runtime) Plugins() []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]trpcplugin.Plugin, len(rt.active))
	copy(out, rt.active)
	return out
}
