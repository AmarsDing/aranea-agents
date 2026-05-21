package plugintrpc

import (
	"aranea-agents/internal/agent/callbacks"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

const pluginChainPriorityBase = 200

// PluginToChainEntries mirrors one Runner plugin's lifecycle hooks into Chain callbacks.
// Caller must ensure the same plugin is excluded from Runner when path is OrchestrationChain.
func PluginToChainEntries(p trpcplugin.Plugin, sortOrder int) ([]callbacks.Callback, error) {
	if p == nil {
		return nil, nil
	}
	mgr, err := trpcplugin.NewManager(p)
	if err != nil {
		return nil, err
	}
	priority := pluginChainPriorityBase + sortOrder
	var entries []callbacks.Callback

	if ac := mgr.AgentCallbacks(); ac != nil {
		for _, cb := range ac.BeforeAgent {
			if cb != nil {
				entries = append(entries, callbacks.NewBeforeAgentHook(priority, cb))
			}
		}
		for _, cb := range ac.AfterAgent {
			if cb != nil {
				entries = append(entries, callbacks.NewAfterAgentHook(priority, cb))
			}
		}
	}
	if mc := mgr.ModelCallbacks(); mc != nil {
		for _, cb := range mc.BeforeModel {
			if cb != nil {
				entries = append(entries, callbacks.NewBeforeModelHook(priority, cb))
			}
		}
		for _, cb := range mc.AfterModel {
			if cb != nil {
				entries = append(entries, callbacks.NewAfterModelHook(priority, cb))
			}
		}
	}
	if tc := mgr.ToolCallbacks(); tc != nil {
		for _, cb := range tc.BeforeTool {
			if cb != nil {
				entries = append(entries, callbacks.NewBeforeToolHook(priority, cb))
			}
		}
		for _, cb := range tc.AfterTool {
			if cb != nil {
				entries = append(entries, callbacks.NewAfterToolHook(priority, cb))
			}
		}
	}
	return entries, nil
}

// ChainEntriesForPlugins converts chain-orchestrated plugins into sorted Chain entries.
func ChainEntriesForPlugins(plugins []trpcplugin.Plugin, sortOrders []int) ([]callbacks.Callback, error) {
	if len(plugins) == 0 {
		return nil, nil
	}
	var all []callbacks.Callback
	for i, p := range plugins {
		order := 0
		if i < len(sortOrders) {
			order = sortOrders[i]
		}
		entries, err := PluginToChainEntries(p, order)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	return all, nil
}
