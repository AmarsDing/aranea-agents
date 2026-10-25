package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Allocator L0（domain_recipe）/ L1（mission）匹配层测试（B.10.21.5 /
// B.10.21.9 allocator 层）。
// ---------------------------------------------------------------------------

// fakeMissionPerfRepo implements biz.AgentPerformanceRepository for tests.
type fakeMissionPerfRepo struct {
	perfs map[string]*biz.AgentPerformance // agentKey|taskType → perf
}

func (f *fakeMissionPerfRepo) Get(_ context.Context, agentKey, taskType string) (*biz.AgentPerformance, error) {
	return f.perfs[agentKey+"|"+taskType], nil
}
func (f *fakeMissionPerfRepo) GetBestForTaskType(_ context.Context, taskType string, _ int) ([]*biz.AgentPerformance, error) {
	var out []*biz.AgentPerformance
	for _, p := range f.perfs {
		if p.TaskType == taskType {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeMissionPerfRepo) Upsert(_ context.Context, perf *biz.AgentPerformance) error {
	if f.perfs == nil {
		f.perfs = make(map[string]*biz.AgentPerformance)
	}
	f.perfs[perf.AgentKey+"|"+perf.TaskType] = perf
	return nil
}

func newRecipeCache(t *testing.T, domainPath string, dq float64, agentKeys []string) *biz.OrchestrationCache {
	t.Helper()
	c := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	c.RecordDomainRecipe(context.Background(), domainPath, biz.TopologyDirect, dq, len(agentKeys), agentKeys)
	return c
}

// --- L0 domain_recipe ---

func TestTryDomainRecipe_Hit(t *testing.T) {
	cache := newRecipeCache(t, "创作/文学", 0.85, []string{"agent-lead", "agent-m1"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-lead", DisplayName: "文学写手"},
		{AgentKey: "agent-m1", DisplayName: "校对"},
	}
	lead, members, dq, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", caps, "trace-l0-1")
	if !ok {
		t.Fatal("expected L0 hit")
	}
	if lead.AgentKey != "agent-lead" {
		t.Errorf("lead = %q, want agent-lead（AgentKeys[0]）", lead.AgentKey)
	}
	if len(members) != 1 || members[0] != "agent-m1" {
		t.Errorf("members = %v, want [agent-m1]（配方成员直接复用）", members)
	}
	// 键全存活：有效 DQ = 0.85 × 2/2 = 0.85。
	if dq != 0.85 {
		t.Errorf("dq = %v, want 0.85", dq)
	}
}

func TestTryDomainRecipe_PrefixQueryHitsBroaderRecipe(t *testing.T) {
	// 配方记录在 "创作"，查询 "创作/文学"（前缀匹配任一方向）仍可命中。
	cache := newRecipeCache(t, "创作", 0.8, []string{"agent-lead"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", caps, "trace-l0-2"); !ok {
		t.Fatal("expected prefix-match hit for broader recipe domain")
	}
}

func TestTryDomainRecipe_LeadAgentDeleted_Skips(t *testing.T) {
	cache := newRecipeCache(t, "创作/文学", 0.85, []string{"agent-gone"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-other"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", caps, "trace-l0-3"); ok {
		t.Fatal("lead agent 已删除时必须跳过复用（落 L1/L2）")
	}
}

func TestTryDomainRecipe_DeadMemberFiltered(t *testing.T) {
	// D7-F4：DQ 1.0 × 4 键折损 1 名成员 → 有效 DQ 0.75 ≥ 0.7 仍可复用，
	// 成员列表剔除已删除者（缓存可能滞后）。
	cache := newRecipeCache(t, "创作/文学", 1.0, []string{"agent-lead", "agent-m1", "agent-m2", "agent-ghost"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-lead"},
		{AgentKey: "agent-m1"},
		{AgentKey: "agent-m2"},
	}
	_, members, dq, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", caps, "trace-l0-4")
	if !ok {
		t.Fatal("expected hit")
	}
	if len(members) != 2 || members[0] != "agent-m1" || members[1] != "agent-m2" {
		t.Errorf("members = %v, want [agent-m1 agent-m2]（已删除成员被剔除）", members)
	}
	if dq != 0.75 {
		t.Errorf("dq = %v, want 0.75（1.0 × 3/4 成员存活折算）", dq)
	}
}

func TestTryDomainRecipe_MemberShrinkBelowThreshold_Misses(t *testing.T) {
	// D7-F4：lead-only 存活（0.9 × 1/3 = 0.3 < 0.7）→ miss，防止 half-dead 配方
	// 虚高复用后团队在现场塌编。
	cache := newRecipeCache(t, "创作/文学", 0.9, []string{"agent-lead", "agent-m1", "agent-m2"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", caps, "trace-l0-4b"); ok {
		t.Fatal("成员缩水至有效 DQ<0.7 必须 miss")
	}
}

func TestTryDomainRecipe_NilCacheOrMiss(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", nil, "trace-l0-5"); ok {
		t.Fatal("nil orchCache must miss")
	}
	empty := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	impl.orchCache = empty
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", "", nil, "trace-l0-6"); ok {
		t.Fatal("empty cache must miss")
	}
}

// --- D7-F2 provenance 指纹门（fail-closed） ---

func newRecipeCacheWithProvenance(t *testing.T, domainPath string, dq float64, agentKeys []string, prov biz.RecipeProvenance) *biz.OrchestrationCache {
	t.Helper()
	c := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	c.RecordDomainRecipeWithProvenance(context.Background(), domainPath, biz.TopologyDirect, dq, len(agentKeys), agentKeys, prov)
	return c
}

func TestTryDomainRecipe_FingerprintGate(t *testing.T) {
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	cases := []struct {
		name   string
		prov   biz.RecipeProvenance
		wantFp string
		hit    bool
	}{
		{"受约束任务 + 无 provenance 旧配方 → 一次性降级", biz.RecipeProvenance{}, "fp-pb-x", false},
		{"受约束任务 + 指纹一致 → 复用", biz.RecipeProvenance{PlaybookID: "pb-x", ConstraintFingerprint: "fp-pb-x"}, "fp-pb-x", true},
		{"受约束任务 + 指纹相左 → 降级", biz.RecipeProvenance{PlaybookID: "pb-x", ConstraintFingerprint: "fp-pb-x"}, "fp-pb-y", false},
		{"无约束任务 + 无 provenance 旧配方 → 复用（兼容）", biz.RecipeProvenance{}, "", true},
		{"无约束任务 + 有 provenance 配方 → 复用", biz.RecipeProvenance{PlaybookID: "pb-x", ConstraintFingerprint: "fp-pb-x"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := newRecipeCacheWithProvenance(t, "创作/文学", 0.85, []string{"agent-lead"}, tc.prov)
			impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
			_, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/文学", tc.wantFp, caps, "trace-fp")
			if ok != tc.hit {
				t.Fatalf("hit = %v, want %v", ok, tc.hit)
			}
		})
	}
}

// --- D7-F3 读路径归一 + 「其他」双重禁 ---

func TestTryDomainRecipe_QueryNormalizedAtRead(t *testing.T) {
	// 读侧归一：带空白的多级路径归一后命中「创作/文学」狭义配方。
	cache := newRecipeCache(t, "创作/文学", 0.85, []string{"agent-lead"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作 / 文学", "", caps, "trace-norm-1"); !ok {
		t.Fatal("未归一的查询路径必须经 NormalizeDomainPath 后命中")
	}
}

func TestTryDomainRecipe_LexiconFoldBeforeQuery(t *testing.T) {
	// 词表折叠先于查询：「创作/诗歌」词表折叠到主域「创作」→ 命中主域配方。
	cache := newRecipeCache(t, "创作", 0.85, []string{"agent-lead"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "创作/诗歌", "", caps, "trace-norm-2"); !ok {
		t.Fatal("词表折叠路径必须命中主域配方")
	}
}

func TestTryDomainRecipe_OtherDomainDoubleBan(t *testing.T) {
	// 「其他」读侧永不命中 L0（即使有人误写过 domain：其他 键也无意义——
	// 归一查询在进缓存前被拦）。
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordDomainRecipe(context.Background(), "创作", biz.TopologyDirect, 0.9, 1, []string{"agent-lead"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead"}}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "其他", "", caps, "trace-ban-1"); ok {
		t.Fatal("「其他」必须 miss（双重禁读侧）")
	}
	if _, _, _, ok := impl.tryDomainRecipe(context.Background(), "", "", caps, "trace-ban-2"); ok {
		t.Fatal("空路径必须 miss")
	}
}

// --- L1 mission ---

func TestTryMissionMatch_PerfBreaksTie(t *testing.T) {
	// 同域两候选，TF-IDF 相似度接近时履历成功率决定排序。
	perf := &fakeMissionPerfRepo{perfs: map[string]*biz.AgentPerformance{
		"agent-b|domain:创作/文学": {AgentKey: "agent-b", TaskType: "domain:创作/文学", SuccessRate: 0.95},
	}}
	impl := &agentAllocatorImpl{perfRepo: perf, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "诗歌写手", Mission: "chinese poetry verses", DomainPath: "创作/文学"},
		{AgentKey: "agent-b", DisplayName: "散文写手", Mission: "chinese poetry essays", DomainPath: "创作/文学"},
		{AgentKey: "agent-c", DisplayName: "后端专家", Mission: "go backend", DomainPath: "软件/后端"},
	}
	cap, score, candCount, ok := impl.tryMissionMatch(context.Background(), "write chinese poetry", "创作/文学", caps, "trace-l1-1")
	if !ok {
		t.Fatal("expected L1 hit")
	}
	if candCount != 2 {
		t.Errorf("candCount = %d, want 2（同域收敛排除 agent-c）", candCount)
	}
	if cap.AgentKey != "agent-b" {
		t.Errorf("winner = %q, want agent-b（履历 0.95 × 0.6 权重胜出）", cap.AgentKey)
	}
	if score <= 0.3 {
		t.Errorf("score = %v, want > 0.3", score)
	}
}

func TestTryMissionMatch_EmbedderSimilarityWins(t *testing.T) {
	// embedder 非 nil → cosine 路径；相似度差异决定排序（双方无履历取 0.5）。
	emb := &mockEmbedder{vectors: [][]float32{
		{1, 0, 0}, // task
		{1, 0, 0}, // agent-a mission → cosine 1.0
		{0, 1, 0}, // agent-b mission → cosine 0.0
	}}
	impl := &agentAllocatorImpl{embedder: emb, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", Mission: "m-a", DomainPath: "创作/文学"},
		{AgentKey: "agent-b", Mission: "m-b", DomainPath: "创作/文学"},
	}
	cap, score, _, ok := impl.tryMissionMatch(context.Background(), "写诗", "创作/文学", caps, "trace-l1-2")
	if !ok {
		t.Fatal("expected L1 hit")
	}
	if cap.AgentKey != "agent-a" {
		t.Errorf("winner = %q, want agent-a（cosine 1.0）", cap.AgentKey)
	}
	if emb.Calls() != 1 {
		t.Errorf("embedder calls = %d, want 1", emb.Calls())
	}
	// score = 1.0×0.4 + 0.5×0.6 = 0.7
	if score < 0.69 || score > 0.71 {
		t.Errorf("score = %v, want ≈0.70（similarity×0.4 + 0.5×0.6）", score)
	}
}

func TestTryMissionMatch_NoSameDomainCandidate_Misses(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", Mission: "go backend", DomainPath: "软件/后端"},
		{AgentKey: "agent-b", Mission: "legacy no domain"},
	}
	if _, _, _, ok := impl.tryMissionMatch(context.Background(), "写诗", "创作/文学", caps, "trace-l1-3"); ok {
		t.Fatal("无同域候选必须 miss（落 L2 旧管线）")
	}
}

func TestTryMissionMatch_ZeroOverlapDefaultPerfMisses(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "aeo", Mission: "实施 llms.txt 与 robots.txt", DomainPath: "创作/文案"},
	}
	if _, _, _, ok := impl.tryMissionMatch(context.Background(), "小红书种草文案", "创作/文案", caps, "trace-l1-floor"); ok {
		t.Fatal("TF-IDF floor + default 0.5 perf must not pass L1")
	}
}

func TestTryMissionMatch_SiblingSpecialtyExcluded(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "author", Mission: "中文诗歌创作", DomainPath: "创作/文学"},
	}
	if _, _, _, ok := impl.tryMissionMatch(context.Background(), "写诗", "创作/文案", caps, "trace-l1-sib"); ok {
		t.Fatal("创作/文学 must not enter 创作/文案 L1")
	}
}

func TestTryMissionMatch_LowPerfBelowThreshold_Misses(t *testing.T) {
	// TF-IDF 零重叠（sigmoid 下界 ≈0.047）+ 低履历（0.1）
	// → score ≈ 0.047×0.4 + 0.1×0.6 ≈ 0.08，阈值 >0.3 不命中。
	perf := &fakeMissionPerfRepo{perfs: map[string]*biz.AgentPerformance{
		"agent-a|domain:创作/文学": {AgentKey: "agent-a", TaskType: "domain:创作/文学", SuccessRate: 0.1},
	}}
	impl := &agentAllocatorImpl{perfRepo: perf, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", Mission: "zzz qqq", DomainPath: "创作/文学"},
	}
	if _, _, _, ok := impl.tryMissionMatch(context.Background(), "xxx yyy", "创作/文学", caps, "trace-l1-4"); ok {
		t.Fatal("零相似 + 低履历不应越过 >0.3 阈值")
	}
}

func TestTryMissionMatch_EmbedderError_FallsBackToTFIDF(t *testing.T) {
	emb := &mockEmbedder{err: errors.New("embedder unavailable")}
	impl := &agentAllocatorImpl{embedder: emb, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", Mission: "chinese poetry prose writing", DomainPath: "创作/文学"},
	}
	cap, _, _, ok := impl.tryMissionMatch(context.Background(), "chinese poetry writing", "创作/文学", caps, "trace-l1-5")
	if !ok {
		t.Fatal("embedding 失败应回退 TF-IDF 并按规则命中")
	}
	if cap.AgentKey != "agent-a" {
		t.Errorf("winner = %q, want agent-a", cap.AgentKey)
	}
}

// --- matchSubTask 管线集成（层序 + 回归） ---

func TestMatchSubTask_L0RecipeShortCircuits(t *testing.T) {
	cache := newRecipeCache(t, "创作/文学", 0.9, []string{"agent-lead"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{AgentKey: "agent-lead", DisplayName: "文学写手"}}
	alloc, err := impl.matchSubTask(context.Background(), biz.SubTask{
		ID: "st_1", Name: "写诗", Description: "写一首春天的诗", DomainPath: "创作/文学",
	}, caps, "trace-pipe-1")
	if err != nil {
		t.Fatalf("matchSubTask: %v", err)
	}
	if alloc.MatchLayer != "domain_recipe" {
		t.Errorf("MatchLayer = %q, want domain_recipe（L0 短路后续层）", alloc.MatchLayer)
	}
	if alloc.AssignedKey != "agent-lead" {
		t.Errorf("AssignedKey = %q, want agent-lead", alloc.AssignedKey)
	}
	if !strings.Contains(alloc.MatchReason, "创作/文学") {
		t.Errorf("MatchReason 应含 domain_path 可解释信息, got %q", alloc.MatchReason)
	}
}

func TestMatchSubTask_EmptyDomainPath_LegacyPipeline(t *testing.T) {
	// 不变量 1：DomainPath 为空时行为与旧管线完全一致（exact 层命中）。
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "研究员", Roles: []string{"research"}},
	}
	alloc, err := impl.matchSubTask(context.Background(), biz.SubTask{
		ID: "st_1", Name: "调研", Description: "调研竞品", RequiredCapabilities: []string{"research"},
	}, caps, "trace-pipe-2")
	if err != nil {
		t.Fatalf("matchSubTask: %v", err)
	}
	if alloc.MatchLayer == "domain_recipe" || alloc.MatchLayer == "mission" {
		t.Errorf("空 DomainPath 不应进入 L0/L1, MatchLayer = %q", alloc.MatchLayer)
	}
	if alloc.AssignedKey != "agent-a" {
		t.Errorf("AssignedKey = %q, want agent-a", alloc.AssignedKey)
	}
}

func TestMatchSubTask_EmptyDomainTaskWords_RosterMiss(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-chat", DisplayName: "闲聊助手", Roles: []string{"chat"}},
	}
	_, err := impl.matchSubTask(context.Background(), biz.SubTask{
		ID: "st_miss", Name: "排查告警并生成巡检报告", Description: "核对昨天的数据",
	}, caps, "trace-empty-task")
	if err == nil {
		t.Fatal("empty domain + task words must roster-miss, not L3 cold-start")
	}
}

func TestMatchSubTask_FingerprintGateBlocksL0(t *testing.T) {
	// D7-F2 端到端：ctx 携带受约束指纹 + 旧配方无 provenance → L0 fail-closed，
	// 钥不复用，匹配落到后续层（roster/…），不产出 domain_recipe 分配。
	cache := newRecipeCache(t, "创作/文学", 0.9, []string{"agent-recipe"})
	impl := &agentAllocatorImpl{orchCache: cache, lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{
		{AgentKey: "agent-recipe"},
		{AgentKey: "agent-z", DomainPath: "创作/文学"},
	}
	ctx := withRecipeWantFingerprint(context.Background(), "fp-pb-x")
	alloc, err := impl.matchSubTask(ctx, biz.SubTask{
		ID: "st_1", Name: "写诗", Description: "写一首春天的诗", DomainPath: "创作/文学",
	}, caps, "trace-gate-1")
	if err != nil {
		t.Fatalf("matchSubTask: %v", err)
	}
	if alloc.MatchLayer == "domain_recipe" {
		t.Fatalf("受约束任务 + 无 provenance 配方必须跳过 L0, got MatchLayer=%q", alloc.MatchLayer)
	}
}
