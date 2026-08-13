package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/service"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
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

func (m *memPluginRepo) SyncBuiltinMeta(_ context.Context, p biz.Plugin) (biz.Plugin, error) {
	cur, ok := m.items[p.ID]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("plugin not found: %s", p.ID)
	}
	cur.Name = p.Name
	cur.Description = p.Description
	cur.Category = p.Category
	cur.RiskLevel = p.RiskLevel
	cur.CallbackPoints = p.CallbackPoints
	cur.ConfigSchemaJSON = p.ConfigSchemaJSON
	cur.DefaultConfigJSON = p.DefaultConfigJSON
	m.items[p.ID] = cur
	return cur, nil
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

// memPluginRunRepo 是内存版 PluginRunRepo，模拟 N-B5 租户可见性语义：
// 空 workspaceID = 系统调用（全部）；非空 = 共享行 ”+ 自身行。
type memPluginRunRepo struct {
	items []biz.PluginRun
}

func (m *memPluginRunRepo) Insert(_ context.Context, run biz.PluginRun) error {
	m.items = append(m.items, run)
	return nil
}

func (m *memPluginRunRepo) visible(workspaceID string) []biz.PluginRun {
	out := make([]biz.PluginRun, 0, len(m.items))
	for _, r := range m.items {
		if workspaceID == "" || r.WorkspaceID == "" || r.WorkspaceID == workspaceID {
			out = append(out, r)
		}
	}
	return out
}

func (m *memPluginRunRepo) List(_ context.Context, q biz.PluginRunQuery) (biz.PluginRunListResult, error) {
	out := m.visible(q.WorkspaceID)
	return biz.PluginRunListResult{Items: out, Total: int32(len(out)), Limit: q.Limit, Offset: q.Offset}, nil
}

func (m *memPluginRunRepo) DeleteAll(_ context.Context, workspaceID string) (int32, error) {
	kept := make([]biz.PluginRun, 0, len(m.items))
	var deleted int32
	for _, r := range m.items {
		if workspaceID == "" || r.WorkspaceID == "" || r.WorkspaceID == workspaceID {
			deleted++
			continue
		}
		kept = append(kept, r)
	}
	m.items = kept
	return deleted, nil
}

func newPluginServiceWithRuns(runs ...biz.PluginRun) *service.PluginService {
	repo := newMemPluginRepo()
	runRepo := &memPluginRunRepo{items: append([]biz.PluginRun{}, runs...)}
	return service.NewPluginService(biz.NewPluginUsecase(repo, runRepo, nil), nil, loggateway.NewNoop(), nil)
}

// N-B5：租户 ListPluginRuns 只能看到共享行 + 本租户行；系统调用看全部。
func TestPluginService_ListPluginRuns_TenantVisibility(t *testing.T) {
	svc := newPluginServiceWithRuns(
		biz.PluginRun{ID: "r-a", PluginKey: "audit_log", WorkspaceID: "ws-a"},
		biz.PluginRun{ID: "r-b", PluginKey: "audit_log", WorkspaceID: "ws-b"},
		biz.PluginRun{ID: "r-shared", PluginKey: "audit_log", WorkspaceID: ""},
	)

	tenantA := workspace.WithContext(context.Background(), "ws-a")
	resp, err := svc.ListPluginRuns(tenantA, &v1.ListPluginRunsRequest{})
	if err != nil {
		t.Fatalf("list runs (tenant ws-a): %v", err)
	}
	got := map[string]bool{}
	for _, it := range resp.GetItems() {
		got[it.GetId()] = true
	}
	if !got["r-a"] || !got["r-shared"] {
		t.Errorf("tenant ws-a should see own + shared, got %v", got)
	}
	if got["r-b"] {
		t.Errorf("tenant ws-a must not see ws-b run")
	}

	sysResp, err := svc.ListPluginRuns(workspace.WithSystemWorkspace(context.Background()), &v1.ListPluginRunsRequest{})
	if err != nil {
		t.Fatalf("list runs (system): %v", err)
	}
	if len(sysResp.GetItems()) != 3 {
		t.Errorf("system should see all 3 runs, got %d", len(sysResp.GetItems()))
	}
}

// N-B5：租户 DeleteAllPluginRuns 只删共享行 + 本租户行；系统调用全删。
func TestPluginService_DeleteAllPluginRuns_TenantScope(t *testing.T) {
	svc := newPluginServiceWithRuns(
		biz.PluginRun{ID: "r-a", PluginKey: "audit_log", WorkspaceID: "ws-a"},
		biz.PluginRun{ID: "r-b", PluginKey: "audit_log", WorkspaceID: "ws-b"},
		biz.PluginRun{ID: "r-shared", PluginKey: "audit_log", WorkspaceID: ""},
	)

	tenantA := workspace.WithContext(context.Background(), "ws-a")
	resp, err := svc.DeleteAllPluginRuns(tenantA, &v1.DeleteAllPluginRunsRequest{})
	if err != nil {
		t.Fatalf("delete all (tenant ws-a): %v", err)
	}
	if resp.GetDeletedCount() != 2 {
		t.Errorf("tenant delete should remove shared+own = 2, got %d", resp.GetDeletedCount())
	}

	listResp, err := svc.ListPluginRuns(workspace.WithSystemWorkspace(context.Background()), &v1.ListPluginRunsRequest{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(listResp.GetItems()) != 1 || listResp.GetItems()[0].GetId() != "r-b" {
		t.Errorf("only ws-b run should remain, got %+v", listResp.GetItems())
	}
}

// ISSUE-009：内置插件元数据演进（如 schema 增加中文 title/description）必须
// 在 bootstrap 时同步到已有行；seeder 此前对已有行直接跳过，修复对存量安装
// 永不生效。同步仅限平台自有字段（name/category/schema/默认值等），保留管理
// 员字段（enabled/config_json/sort_order/scope）。
func TestPluginService_SeedBuiltin_SyncsMetadata(t *testing.T) {
	repo := newMemPluginRepo()
	repo.items["builtin-audit_log"] = biz.Plugin{
		ID: "builtin-audit_log", Key: "audit_log",
		Name: "旧审计", Enabled: true, // 管理员已启用 + 自定义配置/排序/作用域
		ConfigJSON: `{"max_content_length":100}`, DefaultConfigJSON: "{}",
		ConfigSchemaJSON: "{}", SortOrder: 7, Scope: "agent-1", WorkspaceID: "",
	}
	svc := service.NewPluginService(biz.NewPluginUsecase(repo, nil, nil), nil, loggateway.NewNoop(), nil)
	svc.Bootstrap(context.Background())

	got, err := repo.GetPlugin(context.Background(), "builtin-audit_log")
	if err != nil {
		t.Fatalf("get builtin-audit_log: %v", err)
	}
	if !strings.Contains(got.ConfigSchemaJSON, "记录模型请求") {
		t.Errorf("config_schema_json not synced from registry, got: %s", got.ConfigSchemaJSON)
	}
	if got.Name != "运行日志和审计" {
		t.Errorf("name not synced, got: %s", got.Name)
	}
	if !strings.Contains(got.DefaultConfigJSON, "max_content_length") {
		t.Errorf("default_config_json not synced, got: %s", got.DefaultConfigJSON)
	}
	// 管理员字段必须保留
	if !got.Enabled {
		t.Error("enabled must be preserved (admin enabled it)")
	}
	if got.ConfigJSON != `{"max_content_length":100}` {
		t.Errorf("config_json must be preserved, got: %s", got.ConfigJSON)
	}
	if got.SortOrder != 7 {
		t.Errorf("sort_order must be preserved, got: %d", got.SortOrder)
	}
	if got.Scope != "agent-1" {
		t.Errorf("scope must be preserved, got: %s", got.Scope)
	}
}

// captureBus 捕获流程日志事件的 MonitorBus 假实现。
type captureBus struct{ n int }

func (b *captureBus) Publish(_ context.Context, _ contract.MonitorEvent) { b.n++ }

func (b *captureBus) Subscribe(contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	ch := make(chan contract.MonitorEvent)
	return ch, func() {}
}

func (b *captureBus) DropCount() uint64 { return 0 }

// seedFailRepo 模拟 bootstrap 种子 Create 失败场景：GetByKey 恒 NotFound
// （走 Create 分支），CreatePlugin 返回注入的错误。
type seedFailRepo struct {
	*memPluginRepo
	createErr error
}

func (r *seedFailRepo) GetByKey(_ context.Context, _ string) (biz.Plugin, error) {
	return biz.Plugin{}, apierror.NotFound("PLUGIN", "plugin not found")
}

func (r *seedFailRepo) CreatePlugin(_ context.Context, _ biz.Plugin) (biz.Plugin, error) {
	return biz.Plugin{}, r.createErr
}

// A5：并发 bootstrap 种子冲突（CONFLICT）属良性竞争——降级 Debug，不发流程错误事件。
func TestPluginService_SeedBuiltin_ConflictBenign(t *testing.T) {
	bus := &captureBus{}
	repo := &seedFailRepo{memPluginRepo: newMemPluginRepo(), createErr: apierror.Conflict("PLUGIN", "duplicate key")}
	svc := service.NewPluginService(biz.NewPluginUsecase(repo, nil, nil), nil, loggateway.NewNoop(), bus)
	svc.Bootstrap(context.Background())
	if bus.n != 0 {
		t.Errorf("conflict seed must not emit flow error events, got %d", bus.n)
	}
}

// A5：非冲突种子失败仍须 Warn + 发流程错误事件（不吞真实故障）。
func TestPluginService_SeedBuiltin_NonConflictEmitsFlowError(t *testing.T) {
	bus := &captureBus{}
	repo := &seedFailRepo{memPluginRepo: newMemPluginRepo(), createErr: apierror.BadRequest("PLUGIN", "bad config")}
	svc := service.NewPluginService(biz.NewPluginUsecase(repo, nil, nil), nil, loggateway.NewNoop(), bus)
	svc.Bootstrap(context.Background())
	if bus.n == 0 {
		t.Error("non-conflict seed failure should emit flow error events")
	}
}
