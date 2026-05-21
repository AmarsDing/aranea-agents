package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestResolvePluginOrchestration_runnerExclusive(t *testing.T) {
	p := biz.Plugin{
		Key:        "audit_log",
		ConfigJSON: `{"callback_orchestration":"chain"}`,
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationRunner {
		t.Fatalf("got %q want runner", got)
	}
}

func TestResolvePluginOrchestration_chainOptIn(t *testing.T) {
	p := biz.Plugin{
		Key:            "skill_usage_tracker",
		ConfigJSON:     `{"callback_orchestration":"chain"}`,
		CallbackPoints: []string{"before_tool", "after_tool"},
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationChain {
		t.Fatalf("got %q want chain", got)
	}
}

func TestResolvePluginOrchestration_onEventForcesRunner(t *testing.T) {
	p := biz.Plugin{
		Key:            "custom_plugin",
		ConfigJSON:     `{"callback_orchestration":"chain"}`,
		CallbackPoints: []string{"on_event"},
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationRunner {
		t.Fatalf("got %q want runner", got)
	}
}

func TestResolvePluginOrchestration_builtinNotOnAllowlist(t *testing.T) {
	p := biz.Plugin{
		Key:        "sensitive_data_mask",
		ConfigJSON: `{"callback_orchestration":"chain"}`,
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationRunner {
		t.Fatalf("got %q want runner", got)
	}
}

func TestRuntime_PluginsForAgent_excludesChainOnly(t *testing.T) {
	rt := NewRuntime(nil)
	p := biz.Plugin{
		Key:         "skill_usage_tracker",
		Enabled:     true,
		Scope:       "global",
		SortOrder:   2,
		ConfigJSON:  `{"callback_orchestration":"chain"}`,
		DefaultConfigJSON: `{}`,
	}
	rt.Apply(context.Background(), []biz.Plugin{p})

	runner := rt.PluginsForAgent("a1")
	if len(runner) != 0 {
		t.Fatalf("runner plugins=%d want 0", len(runner))
	}
	chain, orders := rt.ChainPluginsForAgent("a1")
	if len(chain) != 1 || len(orders) != 1 {
		t.Fatalf("chain=%d orders=%d", len(chain), len(orders))
	}
}
