package diagnostics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/biz/usage"
)

// --- fakes ---

type fakeProviderModels struct {
	models     []biz.ProviderModel
	listErr    error
	refreshErr error
}

func (f *fakeProviderModels) RunHealthChecks(ctx context.Context) error { return f.refreshErr }
func (f *fakeProviderModels) List(ctx context.Context) ([]biz.ProviderModel, error) {
	return f.models, f.listErr
}

type fakeMCPServers struct {
	servers []biz.MCPServer
	err     error
}

func (f *fakeMCPServers) List(ctx context.Context, q biz.MCPListQuery) ([]biz.MCPServer, error) {
	return f.servers, f.err
}

type fakeToolAssembly struct {
	report biz.ToolAssemblyReport
	err    error
}

func (f *fakeToolAssembly) ReconcileToolAssembly(ctx context.Context) (biz.ToolAssemblyReport, error) {
	return f.report, f.err
}

type fakePendingStore struct {
	rows []biz.MemoryFactPendingRecord
	err  error
}

func (f *fakePendingStore) InsertPending(ctx context.Context, rec biz.MemoryFactPendingRecord) error {
	return nil
}
func (f *fakePendingStore) GetPending(ctx context.Context, id string) (biz.MemoryFactPendingRecord, bool, error) {
	return biz.MemoryFactPendingRecord{}, false, nil
}
func (f *fakePendingStore) ListPending(ctx context.Context, agentID, status string, limit int) ([]biz.MemoryFactPendingRecord, error) {
	return f.rows, f.err
}
func (f *fakePendingStore) MarkDecided(ctx context.Context, id, status, approver string, decidedAt int64) (bool, error) {
	return false, nil
}

type fakeCacheStats struct {
	stats []usage.CacheHitRatioStat
	err   error
}

func (f *fakeCacheStats) CacheHitRatioStats(ctx context.Context, window time.Duration) ([]usage.CacheHitRatioStat, error) {
	return f.stats, f.err
}

type fakeConfigGraph struct {
	report *configgraph.HealthReport
	err    error
}

func (f *fakeConfigGraph) Health(ctx context.Context) (*configgraph.HealthReport, error) {
	return f.report, f.err
}

// --- helpers ---

var testNow = time.Unix(1_800_000_000, 0)

func newTestUsecase(d UsecaseDeps) *Usecase {
	d.Now = func() time.Time { return testNow }
	return NewUsecase(d)
}

// healthyDeps 返回全部依赖装配且健康的 deps，各测试在其上局部破坏。
func healthyDeps() UsecaseDeps {
	return UsecaseDeps{
		ProviderModels: &fakeProviderModels{models: []biz.ProviderModel{
			{Provider: "deepseek", Model: "chat", Enabled: true, Status: "ok"},
		}},
		MCPServers: &fakeMCPServers{servers: []biz.MCPServer{
			{Name: "fs", Enabled: true, MetadataJSON: `{"health_status":"ok"}`},
		}},
		ToolAssembly: &fakeToolAssembly{report: biz.ToolAssemblyReport{AgentsChecked: 3}},
		MemPending:   &fakePendingStore{},
		CacheStats: &fakeCacheStats{stats: []usage.CacheHitRatioStat{
			{Provider: "deepseek", Model: "chat", Samples: 10, P50Ratio: 0.45},
		}},
		ConfigGraph: &fakeConfigGraph{report: &configgraph.HealthReport{Generation: 7}},
	}
}

func itemByKey(t *testing.T, r Report, key string) Item {
	t.Helper()
	for _, it := range r.Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("item %s missing in report %+v", key, r.Items)
	return Item{}
}

// --- tests ---

// 全健康：六项全 pass，config_graph 在装配时出席。
func TestRun_allHealthy(t *testing.T) {
	u := newTestUsecase(healthyDeps())
	r := u.Run(context.Background())
	if len(r.Items) != 6 {
		t.Fatalf("want 6 items, got %d: %+v", len(r.Items), r.Items)
	}
	for _, it := range r.Items {
		if it.Status != StatusPass {
			t.Fatalf("item %s want pass, got %s (%s)", it.Key, it.Status, it.Summary)
		}
		if it.DetailRef == "" {
			t.Fatalf("item %s missing detail_ref", it.Key)
		}
	}
}

// ConfigGraph 未装配时该项缺席（C9 对未启用部署透明），其余五项仍在。
func TestRun_configGraphAbsentWhenNotWired(t *testing.T) {
	d := healthyDeps()
	d.ConfigGraph = nil
	r := newTestUsecase(d).Run(context.Background())
	if len(r.Items) != 5 {
		t.Fatalf("want 5 items without config graph, got %d", len(r.Items))
	}
	for _, it := range r.Items {
		if it.Key == KeyConfigGraph {
			t.Fatalf("config_graph item must be absent when querier not wired")
		}
	}
}

func TestCheckModelProviders(t *testing.T) {
	cases := []struct {
		name   string
		models []biz.ProviderModel
		want   string
	}{
		{"all reachable", []biz.ProviderModel{{Provider: "a", Model: "m1", Enabled: true, Status: "ok"}}, StatusPass},
		{"no enabled model", []biz.ProviderModel{{Provider: "a", Model: "m1", Enabled: false, Status: "ok"}}, StatusFail},
		{"empty catalog", nil, StatusFail},
		{"degraded model", []biz.ProviderModel{
			{Provider: "a", Model: "m1", Enabled: true, Status: "ok"},
			{Provider: "b", Model: "m2", Enabled: true, Status: "degraded"},
		}, StatusFail},
		{"deleted ignored", []biz.ProviderModel{
			{Provider: "a", Model: "m1", Enabled: true, Status: "ok"},
			{Provider: "b", Model: "m2", Enabled: true, Status: "degraded", DeletedAt: "2026-01-01"},
		}, StatusPass},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDeps()
			d.ProviderModels = &fakeProviderModels{models: tc.models}
			it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyModelProviders)
			if it.Status != tc.want {
				t.Fatalf("status=%s want %s (%s)", it.Status, tc.want, it.Summary)
			}
		})
	}

	// ping 刷新失败只记日志、不翻转结果；目录读取失败才 fail。
	d := healthyDeps()
	d.ProviderModels = &fakeProviderModels{
		models:     []biz.ProviderModel{{Provider: "a", Model: "m1", Enabled: true, Status: "ok"}},
		refreshErr: errors.New("ping timeout"),
	}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyModelProviders); it.Status != StatusPass {
		t.Fatalf("refresh failure must be tolerated, got %s", it.Status)
	}
	d.ProviderModels = &fakeProviderModels{listErr: errors.New("db down")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyModelProviders); it.Status != StatusFail {
		t.Fatalf("list failure must fail, got %s", it.Status)
	}
	// 源未装配 → fail（模型目录是硬依赖）。
	d = healthyDeps()
	d.ProviderModels = nil
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyModelProviders); it.Status != StatusFail {
		t.Fatalf("nil provider source must fail, got %s", it.Status)
	}
}

func TestCheckMCPServers(t *testing.T) {
	srv := func(name, health string) biz.MCPServer {
		return biz.MCPServer{Name: name, Enabled: true, MetadataJSON: `{"health_status":"` + health + `"}`}
	}
	cases := []struct {
		name    string
		servers []biz.MCPServer
		want    string
	}{
		{"all ok", []biz.MCPServer{srv("a", "ok")}, StatusPass},
		{"error fails", []biz.MCPServer{srv("a", "ok"), srv("b", "error")}, StatusFail},
		{"auth_required warns", []biz.MCPServer{srv("a", "auth_required")}, StatusWarn},
		{"degraded warns", []biz.MCPServer{srv("a", "degraded")}, StatusWarn},
		{"unknown not penalized", []biz.MCPServer{srv("a", "unknown")}, StatusPass},
		{"empty metadata not penalized", []biz.MCPServer{{Name: "a", Enabled: true}}, StatusPass},
		{"error beats warn", []biz.MCPServer{srv("a", "error"), srv("b", "auth_required")}, StatusFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDeps()
			d.MCPServers = &fakeMCPServers{servers: tc.servers}
			it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMCPServers)
			if it.Status != tc.want {
				t.Fatalf("status=%s want %s (%s)", it.Status, tc.want, it.Summary)
			}
		})
	}

	// 未装配 → pass「未装配」（MCP 非硬依赖）；读取失败 → fail。
	d := healthyDeps()
	d.MCPServers = nil
	it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMCPServers)
	if it.Status != StatusPass || !strings.Contains(it.Summary, "未装配") {
		t.Fatalf("nil mcp source want pass+未装配, got %s (%s)", it.Status, it.Summary)
	}
	d = healthyDeps()
	d.MCPServers = &fakeMCPServers{err: errors.New("db down")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMCPServers); it.Status != StatusFail {
		t.Fatalf("list error must fail, got %s", it.Status)
	}
}

func TestCheckToolAssembly(t *testing.T) {
	cases := []struct {
		name   string
		issues []biz.ToolAssemblyIssue
		want   string
	}{
		{"no issues", nil, StatusPass},
		{"low only passes", []biz.ToolAssemblyIssue{{Severity: biz.ToolAssemblySeverityLow, Code: biz.ToolAssemblyCodeDeadTool}}, StatusPass},
		{"mid warns", []biz.ToolAssemblyIssue{{Severity: biz.ToolAssemblySeverityMid, Code: biz.ToolAssemblyCodeFewTools}}, StatusWarn},
		{"high fails", []biz.ToolAssemblyIssue{
			{Severity: biz.ToolAssemblySeverityHigh, Code: biz.ToolAssemblyCodeZeroTools},
			{Severity: biz.ToolAssemblySeverityMid, Code: biz.ToolAssemblyCodeFewTools},
		}, StatusFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDeps()
			d.ToolAssembly = &fakeToolAssembly{report: biz.ToolAssemblyReport{AgentsChecked: 2, Issues: tc.issues}}
			it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyToolAssembly)
			if it.Status != tc.want {
				t.Fatalf("status=%s want %s (%s)", it.Status, tc.want, it.Summary)
			}
		})
	}

	d := healthyDeps()
	d.ToolAssembly = nil
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyToolAssembly); it.Status != StatusFail {
		t.Fatalf("nil source must fail, got %s", it.Status)
	}
	d = healthyDeps()
	d.ToolAssembly = &fakeToolAssembly{err: errors.New("reconcile boom")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyToolAssembly); it.Status != StatusFail {
		t.Fatalf("reconcile error must fail, got %s", it.Status)
	}
}

func TestCheckMemoryStack(t *testing.T) {
	// 未装配（R3 未启用）→ pass 注记，不扣分。
	d := healthyDeps()
	d.MemPending = nil
	it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack)
	if it.Status != StatusPass || !strings.Contains(it.Summary, "未装配") {
		t.Fatalf("nil pending store want pass+未装配, got %s (%s)", it.Status, it.Summary)
	}

	// 少量 pending、无陈旧 → pass。
	d = healthyDeps()
	d.MemPending = &fakePendingStore{rows: []biz.MemoryFactPendingRecord{
		{ID: "p1", CreatedAt: testNow.Add(-time.Hour).Unix()},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack); it.Status != StatusPass {
		t.Fatalf("small fresh backlog want pass, got %s (%s)", it.Status, it.Summary)
	}

	// 积压 >20 条 → warn。
	d = healthyDeps()
	rows := make([]biz.MemoryFactPendingRecord, 21)
	for i := range rows {
		rows[i] = biz.MemoryFactPendingRecord{ID: "p", CreatedAt: testNow.Unix()}
	}
	d.MemPending = &fakePendingStore{rows: rows}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack); it.Status != StatusWarn {
		t.Fatalf("backlog >20 want warn, got %s (%s)", it.Status, it.Summary)
	}

	// 单条超 24h → warn；超 72h → fail。
	d = healthyDeps()
	d.MemPending = &fakePendingStore{rows: []biz.MemoryFactPendingRecord{
		{ID: "old", CreatedAt: testNow.Add(-25 * time.Hour).Unix()},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack); it.Status != StatusWarn {
		t.Fatalf("stale >24h want warn, got %s (%s)", it.Status, it.Summary)
	}
	d.MemPending = &fakePendingStore{rows: []biz.MemoryFactPendingRecord{
		{ID: "ancient", CreatedAt: testNow.Add(-73 * time.Hour).Unix()},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack); it.Status != StatusFail {
		t.Fatalf("stale >72h want fail, got %s (%s)", it.Status, it.Summary)
	}

	// 读取失败 → fail。
	d.MemPending = &fakePendingStore{err: errors.New("db down")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyMemoryStack); it.Status != StatusFail {
		t.Fatalf("list error must fail, got %s", it.Status)
	}
}

func TestCheckCacheBaseline(t *testing.T) {
	stat := func(samples int, p50 float64) usage.CacheHitRatioStat {
		return usage.CacheHitRatioStat{Provider: "a", Model: "m", Samples: samples, P50Ratio: p50}
	}
	cases := []struct {
		name  string
		stats []usage.CacheHitRatioStat
		want  string
	}{
		{"healthy", []usage.CacheHitRatioStat{stat(10, 0.45)}, StatusPass},
		{"no samples", nil, StatusPass},
		{"insufficient samples skipped", []usage.CacheHitRatioStat{stat(2, 0.01)}, StatusPass},
		{"warn band", []usage.CacheHitRatioStat{stat(10, 0.20)}, StatusWarn},
		{"fail band", []usage.CacheHitRatioStat{stat(10, 0.10)}, StatusFail},
		{"worst group keys status", []usage.CacheHitRatioStat{stat(10, 0.50), stat(8, 0.10)}, StatusFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDeps()
			d.CacheStats = &fakeCacheStats{stats: tc.stats}
			it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyCacheBaseline)
			if it.Status != tc.want {
				t.Fatalf("status=%s want %s (%s)", it.Status, tc.want, it.Summary)
			}
		})
	}

	d := healthyDeps()
	d.CacheStats = nil
	it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyCacheBaseline)
	if it.Status != StatusPass || !strings.Contains(it.Summary, "未装配") {
		t.Fatalf("nil cache source want pass+未装配, got %s (%s)", it.Status, it.Summary)
	}
	d = healthyDeps()
	d.CacheStats = &fakeCacheStats{err: errors.New("db down")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyCacheBaseline); it.Status != StatusFail {
		t.Fatalf("stats error must fail, got %s", it.Status)
	}
}

func TestCheckConfigGraph(t *testing.T) {
	// 首启未建图 → warn 而非 fail。
	d := healthyDeps()
	d.ConfigGraph = &fakeConfigGraph{err: configgraph.ErrNotReady}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusWarn {
		t.Fatalf("ErrNotReady want warn, got %s (%s)", it.Status, it.Summary)
	}

	// 环 → fail；断边 → fail；god node → warn；重复 prompt → warn。
	d.ConfigGraph = &fakeConfigGraph{report: &configgraph.HealthReport{
		Generation: 3,
		Cycles:     []configgraph.Cycle{{Nodes: []configgraph.NodeRef{{ID: "a"}, {ID: "b"}}}},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusFail {
		t.Fatalf("cycle want fail, got %s", it.Status)
	}
	d.ConfigGraph = &fakeConfigGraph{report: &configgraph.HealthReport{
		Generation:   3,
		BrokenByType: []configgraph.BrokenGroup{{EdgeType: "uses_tool", Count: 2}},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusFail {
		t.Fatalf("broken edges want fail, got %s", it.Status)
	}
	d.ConfigGraph = &fakeConfigGraph{report: &configgraph.HealthReport{
		Generation: 3,
		GodNodes:   []configgraph.GodNode{{FanIn: 30}},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusWarn {
		t.Fatalf("god node want warn, got %s", it.Status)
	}
	d.ConfigGraph = &fakeConfigGraph{report: &configgraph.HealthReport{
		Generation:       3,
		DuplicatePrompts: []configgraph.PromptDupGroup{{BodyHash: "h", Count: 2}},
	}}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusWarn {
		t.Fatalf("dup prompt want warn, got %s", it.Status)
	}

	// 健康检查自身报错 → fail。
	d.ConfigGraph = &fakeConfigGraph{err: errors.New("repo down")}
	if it := itemByKey(t, newTestUsecase(d).Run(context.Background()), KeyConfigGraph); it.Status != StatusFail {
		t.Fatalf("health error must fail, got %s", it.Status)
	}
}

// 单项依赖错误只置该项 fail，不影响其余项（降级不 500 的 usecase 层契约）。
func TestRun_singleFailureIsolated(t *testing.T) {
	d := healthyDeps()
	d.ToolAssembly = &fakeToolAssembly{err: errors.New("reconcile boom")}
	d.CacheStats = &fakeCacheStats{err: errors.New("db down")}
	r := newTestUsecase(d).Run(context.Background())
	if got := itemByKey(t, r, KeyToolAssembly).Status; got != StatusFail {
		t.Fatalf("tool_assembly want fail, got %s", got)
	}
	if got := itemByKey(t, r, KeyCacheBaseline).Status; got != StatusFail {
		t.Fatalf("cache_baseline want fail, got %s", got)
	}
	for _, key := range []string{KeyModelProviders, KeyMCPServers, KeyMemoryStack, KeyConfigGraph} {
		if got := itemByKey(t, r, key).Status; got != StatusPass {
			t.Fatalf("%s must stay pass, got %s", key, got)
		}
	}
}

// joinCapped：超 3 个名字截断为 "+N more"。
func TestJoinCapped(t *testing.T) {
	if got := joinCapped([]string{"a", "b"}); got != "a, b" {
		t.Fatalf("short list: %q", got)
	}
	got := joinCapped([]string{"a", "b", "c", "d", "e"})
	if !strings.Contains(got, "a, b, c") || !strings.Contains(got, "+2 more") {
		t.Fatalf("capped: %q", got)
	}
}

// mcpHealthStatusOf：非法 JSON / 缺 key 返回空。
func TestMCPHealthStatusOf(t *testing.T) {
	if got := mcpHealthStatusOf("not-json"); got != "" {
		t.Fatalf("invalid json want empty, got %q", got)
	}
	if got := mcpHealthStatusOf(`{"other":1}`); got != "" {
		t.Fatalf("missing key want empty, got %q", got)
	}
	if got := mcpHealthStatusOf(`{"health_status":" error "}`); got != "error" {
		t.Fatalf("trim: got %q", got)
	}
}
