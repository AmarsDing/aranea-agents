package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestManager_RunnerPlugins_includesEventBridge(t *testing.T) {
	mgr := NewManager(NewRuntime(nil), nil)
	plugins := mgr.RunnerPlugins()
	names := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		names[p.Name()] = true
	}
	if !names["aranea_event_bridge"] {
		t.Fatal("missing aranea_event_bridge plugin")
	}
	if !names["guardrail"] {
		t.Fatal("missing guardrail plugin")
	}
	if !names["tool_call_id"] {
		t.Fatal("missing tool_call_id plugin")
	}
	if !names["consecutive_message_merger"] {
		t.Fatal("missing consecutive_message_merger plugin")
	}
}

func TestManager_MergeChain_addsHookCallbacks(t *testing.T) {
	repo := &hookRepoStub{items: []biz.Hook{{
		Key:        "audit-hook",
		Enabled:    true,
		Status:     "active",
		SortOrder:  1,
		ConfigJSON: `{"callback_point":"after_agent","action":{"type":"log"}}`,
	}}}
	resolver := biz.NewHookResolver(biz.NewHookUsecase(repo), loggateway.NewNoop())
	mgr := NewManager(NewRuntime(nil), resolver)

	base := callbacks.NewChain(callbacks.NewBeforeAgentHook(0, nil))
	merged := mgr.MergeChain(context.Background(), "aid", "akey", base)
	if merged == nil || !merged.HasAgentHooks() {
		t.Fatal("expected merged chain with agent hooks")
	}
}

type hookRepoStub struct {
	items []biz.Hook
}

func (h *hookRepoStub) ListHooks(context.Context) ([]biz.Hook, error) { return h.items, nil }
func (h *hookRepoStub) GetHook(context.Context, string) (biz.Hook, error) {
	return biz.Hook{}, nil
}
func (h *hookRepoStub) CreateHook(context.Context, biz.Hook) (biz.Hook, error) {
	return biz.Hook{}, nil
}
func (h *hookRepoStub) UpdateHook(context.Context, biz.Hook) (biz.Hook, error) {
	return biz.Hook{}, nil
}
func (h *hookRepoStub) DeleteHook(context.Context, string) error { return nil }
