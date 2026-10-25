package biz

import (
	"context"
	"math"
	"strings"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// OrchestrationCache 领域配方（B.10.21.7 / B.10.21.9 biz cache 层）测试。
// ---------------------------------------------------------------------------

func newTestDomainCache() *OrchestrationCache {
	return NewOrchestrationCache(loggateway.NewNoop(), nil)
}

func TestRecordAndBestRecipeForDomain_ExactHit(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.85, 1, []string{"agent-a"})

	entry, ok := c.BestRecipeForDomain(ctx, "创作/文学")
	if !ok {
		t.Fatal("expected hit for exact domain")
	}
	if entry.DomainPath != "创作/文学" || entry.DQScore != 0.85 {
		t.Errorf("entry = %+v", entry)
	}
	if len(entry.AgentKeys) != 1 || entry.AgentKeys[0] != "agent-a" {
		t.Errorf("AgentKeys = %v, want [agent-a]", entry.AgentKeys)
	}
}

func TestBestRecipeForDomain_PrefixMatchBothDirections(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作", TopologyDirect, 0.8, 1, []string{"agent-a"})

	// 查询更具体：配方域是查询域的前缀。
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); !ok {
		t.Fatal("query 创作/文学 should hit recipe 创作（前缀匹配）")
	}
	// 查询更宽泛：查询域是配方域的前缀。
	c2 := newTestDomainCache()
	c2.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.8, 1, []string{"agent-a"})
	if _, ok := c2.BestRecipeForDomain(ctx, "创作"); !ok {
		t.Fatal("query 创作 should hit recipe 创作/文学（反向前缀匹配）")
	}
	// 路径边界：不同一级域不命中。
	if _, ok := c.BestRecipeForDomain(ctx, "软件/后端"); ok {
		t.Fatal("query 软件/后端 must not hit recipe 创作")
	}
}

func TestBestRecipeForDomain_PicksHighestDQ(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作", TopologyDirect, 0.75, 1, []string{"agent-low"})
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-high"})

	entry, ok := c.BestRecipeForDomain(ctx, "创作/文学")
	if !ok {
		t.Fatal("expected hit")
	}
	if entry.AgentKeys[0] != "agent-high" {
		t.Errorf("winner = %q, want agent-high（DQ 0.9 > 0.75）", entry.AgentKeys[0])
	}
}

func TestBestRecipeForDomain_BelowDQThresholdSkips(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, DomainRecipeMinDQ-0.01, 1, []string{"agent-a"})
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); ok {
		t.Fatalf("DQ < %.2f must not be returned", DomainRecipeMinDQ)
	}
}

func TestBestRecipeForDomain_EmptyAgentKeysSkips(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 0, nil)
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); ok {
		t.Fatal("recipe with empty AgentKeys must not be returned")
	}
}

func TestRecordDomainRecipe_HigherDQReplacesOnly(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})
	// 更低 DQ 不覆盖。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.8, 1, []string{"agent-b"})
	entry, _ := c.BestRecipeForDomain(ctx, "创作/文学")
	if entry.AgentKeys[0] != "agent-a" || entry.DQScore != 0.9 {
		t.Errorf("lower DQ must not replace: %+v", entry)
	}
	// 更高 DQ 覆盖。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.95, 1, []string{"agent-c"})
	entry, _ = c.BestRecipeForDomain(ctx, "创作/文学")
	if entry.AgentKeys[0] != "agent-c" || entry.DQScore != 0.95 {
		t.Errorf("higher DQ must replace: %+v", entry)
	}
}

func TestRecordDomainRecipe_EmptyDomainNoop(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "", TopologyDirect, 0.9, 1, []string{"agent-a"})
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); ok {
		t.Fatal("empty domainPath record must be a no-op")
	}
}

func TestOrchestrationCache_LegacyJSONWithoutDomainPath_Loads(t *testing.T) {
	// 不变量 4：旧 JSON（无 domain_path 字段）加载不报错，且不参与领域查询。
	legacy := `[{"task_pattern":"old-key","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-a"],"updated_at":"2099-01-01T00:00:00Z"}]`
	c := newTestDomainCache()
	if err := c.LoadFromJSON(legacy); err != nil {
		t.Fatalf("LoadFromJSON legacy: %v", err)
	}
	if _, ok := c.BestRecipeForDomain(context.Background(), "创作/文学"); ok {
		t.Fatal("legacy entry without domain_path must not match domain query")
	}
}

// ---------------------------------------------------------------------------
// D7-F1 双向学习信号：失败降权 + 触底淘汰。
// ---------------------------------------------------------------------------

func TestRecordDomainRecipe_FailureDecaysDQ(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})

	// 连续失败信号（dq < DQSuccessThreshold）→ 乘法衰减 ×0.8 单调下降。
	prev := 0.9
	failCount := 0
	for i := 0; i < 32; i++ {
		c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.2, 1, []string{"agent-a"})
		entry, ok := c.Get("domain:创作/文学")
		if !ok {
			failCount = i + 1
			break
		}
		if entry.DQScore >= prev {
			t.Fatalf("step %d: DQ %.6f must strictly decrease (prev %.6f)", i, entry.DQScore, prev)
		}
		if entry.DQScore <= DQScoreMin {
			t.Fatalf("step %d: DQ %.6f must stay above floor until eviction", i, entry.DQScore)
		}
		want := prev * DQRecipeFailureDecay
		if d := entry.DQScore - want; d > 1e-9 || d < -1e-9 {
			t.Fatalf("step %d: DQ %.6f, want %.6f (prev ×0.8)", i, entry.DQScore, want)
		}
		prev = entry.DQScore
	}
	if failCount == 0 {
		t.Fatal("0.9 连续失败必须最终触底下架")
	}
	if _, ok := c.Get("domain:创作/文学"); ok {
		t.Fatal("触底后 Get 必须 miss（已淘汰）")
	}
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); ok {
		t.Fatal("触底后 BestRecipeForDomain 必须 miss")
	}
}

func TestRecordDomainRecipe_FailurePreservesAgentKeys(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipeWithProvenance(ctx, "创作/文学", TopologyHybrid, 0.9, 2,
		[]string{"agent-lead", "agent-m1"}, RecipeProvenance{PlaybookID: "pb-1", ConstraintFingerprint: "fp-1"})
	before, _ := c.Get("domain:创作/文学")

	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.2, 1, []string{"agent-x"})
	entry, ok := c.Get("domain:创作/文学")
	if !ok {
		t.Fatal("entry must survive first decay")
	}
	if math.Abs(entry.DQScore-0.72) > 1e-9 {
		t.Errorf("DQ = %v, want ≈0.72 (0.9×0.8)", entry.DQScore)
	}
	// 降权不毁配方体：AgentKeys/Topology/DomainPath/provenance 全部不动。
	if len(entry.AgentKeys) != 2 || entry.AgentKeys[0] != "agent-lead" || entry.AgentKeys[1] != "agent-m1" {
		t.Errorf("AgentKeys mutated by failure: %v", entry.AgentKeys)
	}
	if entry.Topology != TopologyHybrid {
		t.Errorf("Topology mutated by failure: %s", entry.Topology)
	}
	if entry.PlaybookID != "pb-1" || entry.ConstraintFingerprint != "fp-1" {
		t.Errorf("provenance mutated by failure: %+v", entry)
	}
	// UpdatedAt 只能断言不早于 before——RFC3339 秒级精度下，同秒内两次 Record
	// 必然相等；回拨才是真正的回归（字符串比较与时序一致）。
	if entry.UpdatedAt < before.UpdatedAt {
		t.Error("UpdatedAt must not move backward on signal（TTL 从最近信号起算）")
	}
}

func TestRecordDomainRecipe_LowerSuccessKeepsExisting(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})
	// 0.75 ≥ 阈值 → 成功信号 → strict-upgrade 早退，0.9 保持（防回归）。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.75, 1, []string{"agent-b"})
	entry, _ := c.BestRecipeForDomain(ctx, "创作/文学")
	if entry.DQScore != 0.9 || entry.AgentKeys[0] != "agent-a" {
		t.Fatalf("lower success must keep existing entry: %+v", entry)
	}
}

func TestRecordDomainRecipe_DemotedStopsServing(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.72, 1, []string{"agent-a"})
	// 一次失败 → 0.576 < DomainRecipeMinDQ(0.7) → demoted：停止服务但保留配方体。
	c.RecordDomainRecipe(ctx, "创作/文学", TopologyDirect, 0.2, 1, []string{"agent-a"})
	if _, ok := c.BestRecipeForDomain(ctx, "创作/文学"); ok {
		t.Fatal("demoted recipe (DQ<0.7) must stop serving")
	}
	entry, ok := c.Get("domain:创作/文学")
	if !ok {
		t.Fatal("demoted (≥0.1) entry must be retained（下架≠删除）")
	}
	if entry.DQScore >= DomainRecipeMinDQ {
		t.Fatalf("entry DQ %.3f must be below serving threshold", entry.DQScore)
	}

	// pattern 键同规则：衰减至 <0.7 但 ≥0.1 时 QueryByTaskPattern 仍可列出。
	msg := "写一篇市场分析报告"
	pattern := ExtractTaskPattern(msg)
	c.RecordCompletionWithAgents(ctx, pattern, TopologyDirect, 0.72, 1, 0, []string{"agent-p"})
	c.RecordCompletionWithAgents(ctx, pattern, TopologyDirect, 0.2, 1, 0, []string{"agent-p"})
	entries, err := c.QueryByTaskPattern(ctx, msg)
	if err != nil || len(entries) != 1 {
		t.Fatalf("demoted pattern entry must still be listed, got %v (err=%v)", entries, err)
	}
	if entries[0].DQScore >= DomainRecipeMinDQ {
		t.Fatalf("listed entry DQ %.3f unexpected", entries[0].DQScore)
	}
}

func TestRecordCompletion_FailureEvictsAtFloor(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	msg := "整理会议纪要"
	pattern := ExtractTaskPattern(msg)
	c.RecordCompletionWithAgents(ctx, pattern, TopologyDirect, 0.6, 1, 0, []string{"agent-p"})

	// 空缓存上的失败信号为零操作（不造负面首条）。
	if _, ok := c.Get(ExtractTaskPattern("从未见过的任务")); ok {
		t.Fatal("sanity: unknown pattern must miss")
	}
	c.RecordCompletionWithAgents(ctx, ExtractTaskPattern("从未见过的任务"), TopologyDirect, 0.2, 1, 0, []string{"agent-x"})
	if _, ok := c.Get(ExtractTaskPattern("从未见过的任务")); ok {
		t.Fatal("failure on unknown key must be a no-op")
	}

	evicted := false
	for i := 0; i < 16; i++ {
		c.RecordCompletionWithAgents(ctx, pattern, TopologyDirect, 0.2, 1, 0, []string{"agent-p"})
		if _, ok := c.Get(pattern); !ok {
			evicted = true
			break
		}
	}
	if !evicted {
		t.Fatal("0.6 连续失败必须最终触底下架且 Get miss")
	}
}

func TestRecordCompletionWithAgents_WithProvenanceWritesFields(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordCompletionWithAgentsWithProvenance(ctx, "pat-1", TopologyDirect, 0.9, 1, 0,
		[]string{"agent-a"}, RecipeProvenance{PlaybookID: "pb-9", ConstraintFingerprint: "fp-9"})
	entry, ok := c.Get("pat-1")
	if !ok {
		t.Fatal("entry must exist")
	}
	if entry.PlaybookID != "pb-9" || entry.ConstraintFingerprint != "fp-9" {
		t.Fatalf("provenance not persisted: %+v", entry)
	}
	// 兼容旧签名：零值 provenance。
	c.RecordCompletionWithAgents(ctx, "pat-2", TopologyDirect, 0.9, 1, 0, []string{"agent-a"})
	entry2, _ := c.Get("pat-2")
	if entry2.PlaybookID != "" || entry2.ConstraintFingerprint != "" {
		t.Fatalf("legacy variant must write zero provenance: %+v", entry2)
	}
}

// ---------------------------------------------------------------------------
// D7-F4 同分确定性全序：DQ 降序 → UpdatedAt 降序 → TaskPattern 字典序升序。
// ---------------------------------------------------------------------------

func TestBestRecipeForDomain_DeterministicTieBreak(t *testing.T) {
	// 等分：新 UpdatedAt 者胜。
	c := newTestDomainCache()
	if err := c.LoadFromJSON(`[
		{"task_pattern":"domain:创作","domain_path":"创作","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-old"],"updated_at":"2099-01-01T00:00:00Z"},
		{"task_pattern":"domain:创作/文学","domain_path":"创作/文学","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-new"],"updated_at":"2099-01-02T00:00:00Z"}
	]`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		entry, ok := c.BestRecipeForDomain(context.Background(), "创作/文学")
		if !ok {
			t.Fatal("expected hit")
		}
		if entry.AgentKeys[0] != "agent-new" {
			t.Fatalf("iter %d: winner = %q, want agent-new（同分更新者胜，防 map 随机序回归）", i, entry.AgentKeys[0])
		}
	}

	// 同 ts：TaskPattern 字典序小者胜。
	c2 := newTestDomainCache()
	if err := c2.LoadFromJSON(`[
		{"task_pattern":"domain:创作/文学","domain_path":"创作/文学","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-z"],"updated_at":"2099-01-01T00:00:00Z"},
		{"task_pattern":"domain:创作","domain_path":"创作","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-a"],"updated_at":"2099-01-01T00:00:00Z"}
	]`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		entry, ok := c2.BestRecipeForDomain(context.Background(), "创作/文学")
		if !ok {
			t.Fatal("expected hit")
		}
		if entry.AgentKeys[0] != "agent-a" {
			t.Fatalf("iter %d: winner = %q, want agent-a（同 ts 字典序小者胜）", i, entry.AgentKeys[0])
		}
	}
}

// ---------------------------------------------------------------------------
// D7-F5 持久化分行：InitFromRepo 一次性 legacy 导入 + EvictStale 接活。
// ---------------------------------------------------------------------------

// fakeOrchestrationCacheRepo 记录调用（D7-F5 测试夹具）。
type fakeOrchestrationCacheRepo struct {
	legacyJSON string
	entries    []OrchestrationCacheEntry
	upserts    []OrchestrationCacheEntry
	deletes    [][2]string
	loadErr    error
}

func (f *fakeOrchestrationCacheRepo) LoadCacheJSON(context.Context) (string, error) {
	return f.legacyJSON, f.loadErr
}

func (f *fakeOrchestrationCacheRepo) LoadEntries(context.Context) ([]OrchestrationCacheEntry, error) {
	out := make([]OrchestrationCacheEntry, len(f.entries))
	copy(out, f.entries)
	return out, f.loadErr
}

func (f *fakeOrchestrationCacheRepo) UpsertEntry(_ context.Context, entry OrchestrationCacheEntry) error {
	f.upserts = append(f.upserts, entry)
	for i, e := range f.entries {
		if e.WorkspaceID == entry.WorkspaceID && e.TaskPattern == entry.TaskPattern {
			f.entries[i] = entry
			return nil
		}
	}
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeOrchestrationCacheRepo) DeleteEntry(_ context.Context, workspaceID, taskPattern string) error {
	f.deletes = append(f.deletes, [2]string{workspaceID, taskPattern})
	for i, e := range f.entries {
		if e.WorkspaceID == workspaceID && e.TaskPattern == taskPattern {
			f.entries = append(f.entries[:i], f.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestInitFromRepo_ImportsLegacyJSONOnce(t *testing.T) {
	repo := &fakeOrchestrationCacheRepo{
		legacyJSON: `[
			{"task_pattern":"pat-a","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-a"],"updated_at":"2099-01-01T00:00:00Z"},
			{"task_pattern":"domain:创作","domain_path":"创作","topology":"parallel","dq_score":0.8,"team_count":2,"agent_keys":["agent-b"],"updated_at":"2099-01-01T00:00:00Z"}
		]`,
	}
	c := NewOrchestrationCache(loggateway.NewNoop(), repo)
	ctx := context.Background()

	c.InitFromRepo(ctx)
	if _, ok := c.Get("pat-a"); !ok {
		t.Fatal("legacy pat-a must seed from one-shot import")
	}
	if _, ok := c.BestRecipeForDomain(ctx, "创作"); !ok {
		t.Fatal("legacy domain recipe must seed from one-shot import")
	}
	if len(repo.upserts) != 2 {
		t.Fatalf("legacy import must upsert each entry once, got %d", len(repo.upserts))
	}
	// 导入归一 workspace（空 → default）。
	for _, u := range repo.upserts {
		if u.WorkspaceID != workspace.DefaultWorkspaceID {
			t.Fatalf("legacy entry must normalize to default bucket, got %q", u.WorkspaceID)
		}
	}

	// 再次 InitFromRepo：新表非空，不再走 legacy 导入。
	c2 := NewOrchestrationCache(loggateway.NewNoop(), repo)
	c2.InitFromRepo(ctx)
	if len(repo.upserts) != 2 {
		t.Fatalf("second init must not re-import legacy JSON, upserts=%d", len(repo.upserts))
	}
}

func TestInitFromRepo_RunsEvictStale(t *testing.T) {
	repo := &fakeOrchestrationCacheRepo{
		entries: []OrchestrationCacheEntry{
			{TaskPattern: "pat-stale", DomainPath: "", Topology: TopologyDirect, DQScore: 0.9,
				WorkspaceID: workspace.DefaultWorkspaceID, UpdatedAt: "2000-01-01T00:00:00Z"},
			{TaskPattern: "pat-fresh", DomainPath: "", Topology: TopologyDirect, DQScore: 0.9,
				WorkspaceID: workspace.DefaultWorkspaceID, UpdatedAt: "2099-01-01T00:00:00Z"},
		},
	}
	c := NewOrchestrationCache(loggateway.NewNoop(), repo)
	c.InitFromRepo(context.Background())

	if _, ok := c.Get("pat-stale"); ok {
		t.Fatal("过期条目必须在启动清理中被淘汰")
	}
	if _, ok := c.Get("pat-fresh"); !ok {
		t.Fatal("未过期条目必须保留")
	}
	if len(repo.deletes) != 1 || repo.deletes[0] != [2]string{workspace.DefaultWorkspaceID, "pat-stale"} {
		t.Fatalf("过期条目须逐条 DeleteEntry，got %v", repo.deletes)
	}
}

// ---------------------------------------------------------------------------
// D7-F6 配方按 workspace 隔离。
// ---------------------------------------------------------------------------

func TestWorkspaceIsolation_WriteReadScoped(t *testing.T) {
	c := newTestDomainCache()
	ctxA := workspace.WithContext(context.Background(), "ws-a")
	ctxB := workspace.WithContext(context.Background(), "ws-b")

	c.RecordDomainRecipe(ctxA, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})

	if _, ok := c.BestRecipeForDomain(ctxB, "创作/文学"); ok {
		t.Fatal("ws-a 配方对 ws-b 必须 miss")
	}
	if _, ok := c.BestRecipeForDomain(context.Background(), "创作/文学"); ok {
		t.Fatal("ws-a 配方对 default bucket 必须 miss")
	}
	if _, ok := c.BestRecipeForDomain(ctxA, "创作/文学"); !ok {
		t.Fatal("ws-a 配方对 ws-a 必须 hit")
	}

	// pattern 键同理。
	msg := "写一篇产品发布稿"
	c.RecordCompletionWithAgents(ctxA, ExtractTaskPattern(msg), TopologyDirect, 0.9, 1, 0, []string{"agent-a"})
	if got, _ := c.QueryByTaskPattern(ctxB, msg); len(got) != 0 {
		t.Fatal("ws-a pattern 条目对 ws-b 必须 miss")
	}
	if got, _ := c.QueryByTaskPattern(ctxA, msg); len(got) != 1 {
		t.Fatal("ws-a pattern 条目对 ws-a 必须 hit")
	}
}

func TestLegacyEntryNoWorkspace_FallsIntoDefaultBucket(t *testing.T) {
	legacy := `[{"task_pattern":"domain:创作/文学","domain_path":"创作/文学","topology":"direct","dq_score":0.9,"team_count":1,"agent_keys":["agent-a"],"updated_at":"2099-01-01T00:00:00Z"}]`
	c := newTestDomainCache()
	if err := c.LoadFromJSON(legacy); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.BestRecipeForDomain(context.Background(), "创作/文学"); !ok {
		t.Fatal("legacy 无 workspace 条目须归一 default 桶并对 default 调用方可见")
	}
	ctxA := workspace.WithContext(context.Background(), "ws-a")
	if _, ok := c.BestRecipeForDomain(ctxA, "创作/文学"); ok {
		t.Fatal("legacy default 桶条目对其他租户必须即刻不可见（fail-closed）")
	}
}

func TestSystemWorkspace_NormalizesToDefault(t *testing.T) {
	c := newTestDomainCache()
	sysCtx := workspace.WithSystemWorkspace(context.Background())
	c.RecordDomainRecipe(sysCtx, "创作/文学", TopologyDirect, 0.9, 1, []string{"agent-a"})

	if _, ok := c.BestRecipeForDomain(context.Background(), "创作/文学"); !ok {
		t.Fatal("system 写入须归一 default 桶（后台 cron 学习无跨租户泄漏通道）")
	}
	if _, ok := c.BestRecipeForDomain(sysCtx, "创作/文学"); !ok {
		t.Fatal("system 读取须等同 default")
	}
}

// ---------------------------------------------------------------------------
// D7-F2 provenance 落库：RecordDomainRecipeWithProvenance。
// ---------------------------------------------------------------------------

func TestRecordDomainRecipeWithProvenance_WritesFields(t *testing.T) {
	c := newTestDomainCache()
	ctx := context.Background()
	c.RecordDomainRecipeWithProvenance(ctx, "创作/文学", TopologyDirect, 0.9, 1,
		[]string{"agent-a"}, RecipeProvenance{PlaybookID: "pb-lit", ConstraintFingerprint: "fp-lit"})
	entry, ok := c.Get("domain:创作/文学")
	if !ok {
		t.Fatal("entry must exist")
	}
	if entry.PlaybookID != "pb-lit" || entry.ConstraintFingerprint != "fp-lit" {
		t.Fatalf("provenance not persisted: %+v", entry)
	}
	// JSON 往返无损（含 workspace）。
	js, err := c.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	c2 := newTestDomainCache()
	if err := c2.LoadFromJSON(js); err != nil {
		t.Fatal(err)
	}
	entry2, ok := c2.Get("domain:创作/文学")
	if !ok || entry2.PlaybookID != "pb-lit" || entry2.ConstraintFingerprint != "fp-lit" || entry2.WorkspaceID != workspace.DefaultWorkspaceID {
		t.Fatalf("round-trip lost provenance/workspace: %+v", entry2)
	}
	if !strings.Contains(js, "fp-lit") {
		t.Fatalf("json missing provenance: %s", js)
	}
}
