package agent

import (
	"context"
	"errors"
	"math"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// D7-F1/F2/F3 orchestrator 学习写路径测试：
//   - F1：SynthesizeResults 失败 —— 双向信号，配方失败衰减（dq=0）；
//   - F2：成功写带 provenance（plan.MemoryHit）；
//   - F3：主导域「其他」双重禁——拒绝写 domain 键，回落旧 pattern 键。
// ---------------------------------------------------------------------------

type fakeSynthesisPort struct {
	out *biz.SynthesisOutput
	err error
}

func (f *fakeSynthesisPort) SynthesizeResults(context.Context, string, string) (*biz.SynthesisOutput, error) {
	return f.out, f.err
}

func newLearnOrchestrator(repo biz.OrchestrationRepository, cache *biz.OrchestrationCache, plans *planStore, synth tools.SpiritSynthesisPort) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		repo:      repo,
		plans:     plans,
		orchCache: cache,
		synthesis: synth,
		lg:        loggateway.NewNoop(),
	}
}

func TestSynthesize_ErrorPathDecaysDomainRecipe(t *testing.T) {
	ctx := context.Background()
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordDomainRecipe(ctx, "创作/文学", biz.TopologyParallel, 0.9, 1, []string{"agent-a"})

	plan := &biz.TaskPlan{
		ID:       "tp_learn_1",
		Strategy: biz.StrategyParallel,
		SubTasks: []biz.SubTask{{ID: "st_1", Name: "写诗", DomainPath: "创作/文学"}},
	}
	handle := &biz.OrchestrationHandle{
		ID:              "orch_learn_1",
		TaskPlanID:      plan.ID,
		SpiritSessionID: "sess_1",
		Strategy:        biz.StrategyParallel,
		TeamIDs:         []string{"team-1"},
		Status:          biz.OrchestrationStatusRunning,
	}
	repo := newMemOrchestrationRepo()
	if _, err := repo.Create(ctx, handle); err != nil {
		t.Fatal(err)
	}
	orch := newLearnOrchestrator(repo, cache, newPlanStore(newMemTaskPlanRepo(plan), nil),
		&fakeSynthesisPort{err: errors.New("synthesis exploded")})

	if _, err := orch.Synthesize(ctx, handle.ID); err == nil {
		t.Fatal("expected synthesis error to propagate")
	}
	entry, ok := cache.Get("domain:创作/文学")
	if !ok {
		t.Fatal("失败信号必须衰减而非删除既有条目（0.9×0.8=0.72 > 0.1）")
	}
	if math.Abs(entry.DQScore-0.72) > 1e-9 {
		t.Fatalf("DQ = %v, want ≈0.72（失败信号 ×0.8）", entry.DQScore)
	}
}

func TestLearnFromOrchestration_WritesProvenance(t *testing.T) {
	ctx := context.Background()
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	plan := &biz.TaskPlan{
		ID:       "tp_learn_2",
		Strategy: biz.StrategyParallel,
		SubTasks: []biz.SubTask{{ID: "st_1", Name: "写诗", DomainPath: "创作/文学"}},
		MemoryHit: &biz.MemoryHit{
			PlaybookID:            "pb-lit",
			ConstraintFingerprint: "fp-lit",
		},
	}
	handle := &biz.OrchestrationHandle{
		ID:              "orch_learn_2",
		TaskPlanID:      plan.ID,
		SpiritSessionID: "sess_2",
		Strategy:        biz.StrategyParallel,
		TeamIDs:         []string{"team-1"},
	}
	orch := newLearnOrchestrator(nil, cache, newPlanStore(newMemTaskPlanRepo(plan), nil), nil)

	orch.learnFromOrchestration(ctx, handle, &biz.SynthesisOutput{
		Content:     "合成报告",
		TeamResults: []biz.TeamSynthesisResult{{TeamID: "team-1", Status: "completed", Summary: "done"}},
	})

	entry, ok := cache.Get("domain:创作/文学")
	if !ok {
		t.Fatal("成功信号必须写入领域配方")
	}
	if entry.PlaybookID != "pb-lit" || entry.ConstraintFingerprint != "fp-lit" {
		t.Fatalf("provenance 未随学习落库： %+v", entry)
	}
	// 全成功 + 无 key findings → dq=0.8。
	if math.Abs(entry.DQScore-0.8) > 1e-9 {
		t.Fatalf("DQ = %v, want ≈0.8", entry.DQScore)
	}
}

func TestLearnFromOrchestration_OtherDomainFallsToPatternKey(t *testing.T) {
	ctx := context.Background()
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	plan := &biz.TaskPlan{
		ID:       "tp_learn_3",
		Strategy: biz.StrategyDirect,
		SubTasks: []biz.SubTask{{ID: "st_1", Name: "随便聊聊", DomainPath: "其他"}},
	}
	handle := &biz.OrchestrationHandle{
		ID:              "orch_learn_3",
		TaskPlanID:      plan.ID,
		SpiritSessionID: "sess_3",
		Strategy:        biz.StrategyDirect,
		TeamIDs:         []string{"team-1"},
	}
	orch := newLearnOrchestrator(nil, cache, newPlanStore(newMemTaskPlanRepo(plan), nil), nil)

	orch.learnFromOrchestration(ctx, handle, &biz.SynthesisOutput{
		Content:     "闲聊总结",
		TeamResults: []biz.TeamSynthesisResult{{TeamID: "team-1", Status: "completed", Summary: "done"}},
	})

	if _, ok := cache.Get("domain:其他"); ok {
		t.Fatal("D7-F3 双重禁：主导域「其他」禁止写 domain 键")
	}
	if _, ok := cache.Get(biz.ExtractTaskPattern(handle.ID)); !ok {
		t.Fatal("「其他」必须回落旧 pattern 键（write-only 兼容分支）")
	}
}

func TestLearnRecipeFailure_NoPlanIsNoop(t *testing.T) {
	ctx := context.Background()
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	orch := newLearnOrchestrator(nil, cache, nil, nil)

	// 无 plan：失败信号落在空缓存的旧键分支 → 零操作（不造负面首条）。
	orch.learnRecipeFailure(ctx, &biz.OrchestrationHandle{ID: "orch_none", TeamIDs: []string{"team-1"}})
	if got := cache.GetStats().Size; got != 0 {
		t.Fatalf("空缓存上的失败信号必须零操作, size=%d", got)
	}
}

func TestLearnFromOrchestration_FailureSignalDecays(t *testing.T) {
	// 学习主入口的下行合法性自检：computeDQScoreFromSynthesis 全失败 → 0.2 < 0.5
	// 同样经 recordRecipeSignal 失败分支衰减既有配方（与错误分支共享语义）。
	ctx := context.Background()
	cache := biz.NewOrchestrationCache(loggateway.NewNoop(), nil)
	cache.RecordDomainRecipe(ctx, "创作/文学", biz.TopologyParallel, 0.9, 1, []string{"agent-a"})
	plan := &biz.TaskPlan{
		ID:       "tp_learn_5",
		Strategy: biz.StrategyParallel,
		SubTasks: []biz.SubTask{{ID: "st_1", Name: "写诗", DomainPath: "创作/文学"}},
	}
	handle := &biz.OrchestrationHandle{
		ID:              "orch_learn_5",
		TaskPlanID:      plan.ID,
		SpiritSessionID: "sess_5",
		Strategy:        biz.StrategyParallel,
		TeamIDs:         []string{"team-1"},
	}
	orch := newLearnOrchestrator(nil, cache, newPlanStore(newMemTaskPlanRepo(plan), nil), nil)

	orch.learnFromOrchestration(ctx, handle, &biz.SynthesisOutput{
		Content:     "全部失败",
		TeamResults: []biz.TeamSynthesisResult{{TeamID: "team-1", Status: "failed"}},
	})

	entry, ok := cache.Get("domain:创作/文学")
	if !ok {
		t.Fatal("单次失败衰减后条目仍在（0.72 > 0.1）")
	}
	if math.Abs(entry.DQScore-0.72) > 1e-9 {
		t.Fatalf("DQ = %v, want ≈0.72", entry.DQScore)
	}
}
