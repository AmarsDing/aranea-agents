package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestResolvePluginOrchestration_alwaysRunner(t *testing.T) {
	p := biz.Plugin{
		Key:        "audit_log",
		ConfigJSON: `{"callback_orchestration":"chain"}`,
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationRunner {
		t.Fatalf("got %q want runner", got)
	}
}

func TestResolvePluginOrchestration_skillTracker(t *testing.T) {
	p := biz.Plugin{
		Key:            "skill_usage_tracker",
		ConfigJSON:     `{"callback_orchestration":"chain"}`,
		CallbackPoints: []string{"before_tool", "after_tool"},
	}
	if got := ResolvePluginOrchestration(p); got != OrchestrationRunner {
		t.Fatalf("got %q want runner (unified path)", got)
	}
}

func TestRuntime_PluginsForAgent_includesAll(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())
	p := biz.Plugin{
		Key:               "skill_usage_tracker",
		Enabled:           true,
		Scope:             "global",
		SortOrder:         2,
		ConfigJSON:        `{"callback_orchestration":"chain"}`,
		DefaultConfigJSON: `{}`,
	}
	rt.Apply(context.Background(), []biz.Plugin{p})

	runner := rt.PluginsForAgent("a1")
	if len(runner) != 1 {
		t.Fatalf("runner plugins=%d want 1 (unified path)", len(runner))
	}
}
