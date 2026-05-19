package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
)

func TestManager_RunnerPlugins_includesEventBridge(t *testing.T) {
	mgr := NewManager(NewRuntime(nil), nil)
	plugins := mgr.RunnerPlugins()
	if len(plugins) != 1 {
		t.Fatalf("len=%d want 1 (event bridge only)", len(plugins))
	}
	if plugins[0].Name() != "aranea_event_bridge" {
		t.Fatalf("name=%q", plugins[0].Name())
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
	resolver := biz.NewHookResolver(biz.NewHookUsecase(repo))
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
