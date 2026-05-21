package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestPluginToChainEntries_skillTracker(t *testing.T) {
	p := biz.Plugin{Key: "skill_usage_tracker", Enabled: true}
	plug := adapt(p, nil, nil, NewRuntime(nil))
	if plug == nil {
		t.Fatal("nil plugin")
	}
	entries, err := PluginToChainEntries(plug, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("entries=%d want >=2", len(entries))
	}
}

func TestChainEntriesForPlugins_noDoubleRunner(t *testing.T) {
	rt := NewRuntime(nil)
	plug := biz.Plugin{
		Key:         "skill_usage_tracker",
		Enabled:     true,
		ConfigJSON:  `{"callback_orchestration":"chain"}`,
		DefaultConfigJSON: `{}`,
	}
	rt.Apply(context.Background(), []biz.Plugin{plug})

	if n := len(rt.PluginsForAgent("")); n != 0 {
		t.Fatalf("runner count=%d", n)
	}
	plugins, orders := rt.ChainPluginsForAgent("")
	entries, err := ChainEntriesForPlugins(plugins, orders)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected chain entries")
	}
}
