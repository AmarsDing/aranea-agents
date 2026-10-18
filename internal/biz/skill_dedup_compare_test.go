package biz

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// 审计 T6 验收：并行快路径与串行路径在相同候选集上必须产出完全一致的结果
// （同阈值超界对集合 + 相同的 (i,j) 字典序），且 CompareTokenized 与 Compare
// 逐对分数一致（embedder=nil 的纯 Jaccard 路径）。
func TestComparePairs_ParallelMatchesSerial(t *testing.T) {
	engine := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	uc := &SkillDedupUsecase{engine: engine, lg: loggateway.NewNoop()}

	// 构造 60 个候选：前 3 个高度相似（应成组），其余互不相似。
	body := "数据库查询优化：使用索引、执行计划分析与慢查询日志定位瓶颈，覆盖分页与缓存策略"
	candidates := make([]SkillDedupCandidate, 0, 60)
	for i := 0; i < 3; i++ {
		candidates = append(candidates, SkillDedupCandidate{
			ID:          fmt.Sprintf("sim-%d", i),
			Name:        "数据库查询优化助手",
			Slug:        fmt.Sprintf("db-query-opt-%d", i),
			Description: "分析慢查询并给出索引与执行计划优化建议",
			BodyPreview: body,
			Tags:        []string{"database", "optimization"},
		})
	}
	for i := 0; i < 57; i++ {
		candidates = append(candidates, SkillDedupCandidate{
			ID:          fmt.Sprintf("uniq-%d", i),
			Name:        fmt.Sprintf("completely different skill number %d", i),
			Slug:        fmt.Sprintf("uniq-%d", i),
			Description: fmt.Sprintf("unique description about topic %d with no overlap", i),
			BodyPreview: fmt.Sprintf("body %d %d %d %d", i, i+1, i+2, i+3),
			Tags:        []string{fmt.Sprintf("tag-%d", i)},
		})
	}

	ctx := context.Background()
	serial, err := uc.comparePairsSerial(ctx, candidates)
	if err != nil {
		t.Fatalf("comparePairsSerial: %v", err)
	}
	parallel, err := uc.comparePairsParallel(ctx, engine, candidates)
	if err != nil {
		t.Fatalf("comparePairsParallel: %v", err)
	}

	if len(serial) != len(parallel) {
		t.Fatalf("pair count mismatch: serial=%d parallel=%d", len(serial), len(parallel))
	}
	if len(serial) == 0 {
		t.Fatal("expected at least one over-threshold pair from the 3 similar candidates")
	}
	for k := range serial {
		s, p := serial[k], parallel[k]
		if s.i != p.i || s.j != p.j {
			t.Fatalf("pair order mismatch at %d: serial=(%d,%d) parallel=(%d,%d)", k, s.i, s.j, p.i, p.j)
		}
		if s.result.TotalScore != p.result.TotalScore {
			t.Fatalf("score mismatch at pair (%d,%d): serial=%v parallel=%v", s.i, s.j, s.result.TotalScore, p.result.TotalScore)
		}
	}

	// CompareTokenized 必须与 Compare 逐对一致（抽相似对与一对不相似对）。
	for _, ij := range [][2]int{{0, 1}, {0, 2}, {0, 3}} {
		a, b := candidates[ij[0]], candidates[ij[1]]
		ro, err := engine.Compare(ctx, a, b)
		if err != nil {
			t.Fatalf("Compare(%d,%d): %v", ij[0], ij[1], err)
		}
		rt, err := engine.CompareTokenized(ctx, TokenizeDedupCandidate(a), TokenizeDedupCandidate(b))
		if err != nil {
			t.Fatalf("CompareTokenized(%d,%d): %v", ij[0], ij[1], err)
		}
		if ro.TotalScore != rt.TotalScore || ro.ConflictRisk != rt.ConflictRisk || ro.Recommendation != rt.Recommendation {
			t.Fatalf("Compare vs CompareTokenized mismatch at (%d,%d): %+v vs %+v", ij[0], ij[1], ro, rt)
		}
	}
}

// 候选数小于 worker 数、仅 2 个候选等边界下，分段并行不得 panic 或漏对。
func TestComparePairs_ParallelEdgeCases(t *testing.T) {
	engine := NewSkillSimilarityEngine(nil, DefaultDedupWeights(), loggateway.NewNoop())
	uc := &SkillDedupUsecase{engine: engine, lg: loggateway.NewNoop()}
	ctx := context.Background()

	two := []SkillDedupCandidate{
		{ID: "a", Name: "same name", Description: "same desc", BodyPreview: "same body", Tags: []string{"t"}},
		{ID: "b", Name: "same name", Description: "same desc", BodyPreview: "same body", Tags: []string{"t"}},
	}
	pairs, err := uc.comparePairsParallel(ctx, engine, two)
	if err != nil {
		t.Fatalf("two candidates: %v", err)
	}
	if len(pairs) != 1 || pairs[0].i != 0 || pairs[0].j != 1 {
		t.Fatalf("expected exactly one pair (0,1), got %+v", pairs)
	}

	// ctx 已取消：返回 ctx.Err，不得返回部分结果。
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := uc.comparePairsParallel(cancelled, engine, two); err == nil {
		t.Fatal("expected ctx.Err on cancelled context")
	}
}
