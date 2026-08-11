package service_test

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// memPluginRepo is an in-memory PluginRepo.
type memPluginRepo struct {
	items map[string]biz.Plugin
}

func newMemPluginRepo() *memPluginRepo {
	return &memPluginRepo{items: make(map[string]biz.Plugin)}
}

func (m *memPluginRepo) SearchPlugins(_ context.Context, q biz.PluginListQuery) (biz.PluginListResult, error) {
	out := make([]biz.Plugin, 0, len(m.items))
	for _, p := range m.items {
		out = append(out, p)
	}
	total := len(out)
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return biz.PluginListResult{Items: out, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (m *memPluginRepo) GetPlugin(_ context.Context, id string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", id)
	}
	return p, nil
}

func (m *memPluginRepo) GetByKey(_ context.Context, key string) (biz.Plugin, error) {
	for _, p := range m.items {
		if p.Key == key {
			return p, nil
		}
	}
	return biz.Plugin{}, fmt.Errorf("plugin not found: %s", key)
}

func (m *memPluginRepo) CreatePlugin(_ context.Context, p biz.Plugin) (biz.Plugin, error) {
	m.items[p.ID] = p
	return p, nil
}

func (m *memPluginRepo) UpdatePluginEnabled(_ context.Context, id string, enabled bool) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", id)
	}
	p.Enabled = enabled
	m.items[id] = p
	return p, nil
}

func (m *memPluginRepo) UpdatePluginConfig(_ context.Context, id string, configJSON string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", id)
	}
	p.ConfigJSON = configJSON
	m.items[id] = p
	return p, nil
}

func (m *memPluginRepo) UpdateSortOrder(_ context.Context, id string, sortOrder int) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", id)
	}
	p.SortOrder = sortOrder
	m.items[id] = p
	return p, nil
}

func (m *memPluginRepo) UpdatePluginScope(_ context.Context, id string, scope string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", id)
	}
	p.Scope = scope
	m.items[id] = p
	return p, nil
}

func (m *memPluginRepo) IncrementStats(_ context.Context, pluginKey string, delta biz.PluginStatUpdate) error {
	for id, p := range m.items {
		if p.Key == pluginKey {
			p.InvokeCount += delta.InvokeCount
			p.BlockCount += delta.BlockDelta
			p.ErrorCount += delta.ErrorDelta
			if delta.LastStatus != "" {
				p.LastStatus = delta.LastStatus
			}
			m.items[id] = p
			return nil
		}
	}
	return nil
}

func newPluginService() *service.PluginService {
	repo := newMemPluginRepo()
	repo.items["p1"] = biz.Plugin{ID: "p1", Key: "test-plugin", Name: "Test Plugin", Enabled: false, WorkspaceID: workspace.DefaultWorkspaceID}
	return service.NewPluginService(biz.NewPluginUsecase(repo, nil, nil), nil, loggateway.NewNoop(), nil)
}

func TestPluginService_List(t *testing.T) {
	svc := newPluginService()
	ctx := context.Background()

	resp, err := svc.ListPlugins(ctx, &v1.ListPluginsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(resp.GetItems()))
	}
	if resp.GetItems()[0].GetKey() != "test-plugin" {
		t.Errorf("key mismatch: %s", resp.GetItems()[0].GetKey())
	}
}

func TestPluginService_ToggleEnabled(t *testing.T) {
	svc := newPluginService()
	ctx := context.Background()

	out, err := svc.TogglePluginEnabled(ctx, &v1.TogglePluginEnabledRequest{Id: "p1", Enabled: true})
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !out.GetEnabled() {
		t.Error("expected enabled=true")
	}
}

func TestPluginService_UpdateConfig(t *testing.T) {
	svc := newPluginService()
	ctx := context.Background()

	out, err := svc.UpdatePluginConfig(ctx, &v1.UpdatePluginConfigRequest{Id: "p1", ConfigJson: `{"k":"v"}`})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if out.GetConfigJson() != `{"k":"v"}` {
		t.Errorf("config mismatch: %s", out.GetConfigJson())
	}
}

func TestPluginService_UpdateSortOrder(t *testing.T) {
	svc := newPluginService()
	ctx := context.Background()

	out, err := svc.UpdatePluginSortOrder(ctx, &v1.UpdatePluginSortOrderRequest{Id: "p1", SortOrder: 5})
	if err != nil {
		t.Fatalf("update sort order: %v", err)
	}
	if out.GetSortOrder() != 5 {
		t.Errorf("sort_order mismatch: %d", out.GetSortOrder())
	}
}

func TestPluginService_UpdateScope(t *testing.T) {
	svc := newPluginService()
	ctx := context.Background()

	out, err := svc.UpdatePluginScope(ctx, &v1.UpdatePluginScopeRequest{Id: "p1", Scope: "agent-42"})
	if err != nil {
		t.Fatalf("update scope: %v", err)
	}
	if out.GetScope() != "agent-42" {
		t.Errorf("scope mismatch: %s", out.GetScope())
	}
}

// 回归：内置共享插件（workspace_id=""）属平台级配置，租户管理员可变更。
// 此前 P2-B 对共享资源 fail-closed 导致管理页全部写操作 404（dogfood ISSUE-002/003）。
func newPluginServiceWith(items ...biz.Plugin) *service.PluginService {
	repo := newMemPluginRepo()
	for _, p := range items {
		repo.items[p.ID] = p
	}
	return service.NewPluginService(biz.NewPluginUsecase(repo, nil, nil), nil, loggateway.NewNoop(), nil)
}

func TestPluginService_SharedBuiltin_ToggleAndConfig(t *testing.T) {
	svc := newPluginServiceWith(biz.Plugin{ID: "builtin-audit_log", Key: "audit_log", Name: "审计", WorkspaceID: ""})
	ctx := context.Background() // tenant caller（默认 default workspace）

	if _, err := svc.TogglePluginEnabled(ctx, &v1.TogglePluginEnabledRequest{Id: "builtin-audit_log", Enabled: true}); err != nil {
		t.Fatalf("toggle shared builtin plugin: %v", err)
	}
	out, err := svc.UpdatePluginConfig(ctx, &v1.UpdatePluginConfigRequest{Id: "builtin-audit_log", ConfigJson: `{"k":"v"}`})
	if err != nil {
		t.Fatalf("update shared builtin config: %v", err)
	}
	if out.GetConfigJson() != `{"k":"v"}` {
		t.Errorf("config mismatch: %s", out.GetConfigJson())
	}
	if _, err := svc.UpdatePluginSortOrder(ctx, &v1.UpdatePluginSortOrderRequest{Id: "builtin-audit_log", SortOrder: 7}); err != nil {
		t.Fatalf("update shared builtin sort order: %v", err)
	}
	if _, err := svc.UpdatePluginScope(ctx, &v1.UpdatePluginScopeRequest{Id: "builtin-audit_log", Scope: "global"}); err != nil {
		t.Fatalf("update shared builtin scope: %v", err)
	}
}

// 跨租户写私有插件仍须拒绝。
func TestPluginService_PrivatePlugin_CrossTenantDenied(t *testing.T) {
	svc := newPluginServiceWith(biz.Plugin{ID: "p9", Key: "private-plugin", WorkspaceID: "ws-a"})
	ctx := workspace.WithContext(context.Background(), "ws-b")
	if _, err := svc.TogglePluginEnabled(ctx, &v1.TogglePluginEnabledRequest{Id: "p9", Enabled: true}); err == nil {
		t.Fatal("expected cross-tenant mutate to be denied")
	}
}
