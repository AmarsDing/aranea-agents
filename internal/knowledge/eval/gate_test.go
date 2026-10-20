package eval

import (
	"strings"
	"testing"
)

// 门禁层单测：gold_snapshot_test.go 与 cmd/goldeval 共用的 CheckGoldGate /
// ValidateGoldBaseline / ValidateGoldExpectations 的每个分支都在此钉死
//（审查 F5：此前这些分支只以内联形式存在于两个入口，零覆盖且已分叉）。

func perfectCase(id string) CaseResult {
	return CaseResult{
		CaseID: id,
		Chunk:  CaseMetrics{RecallAtK: 1, MRR: 1, Hit: 1, FirstHitRank: 1},
		Doc:    CaseMetrics{RecallAtK: 1, MRR: 1, Hit: 1, FirstHitRank: 1},
	}
}

func perfectReport() RunReport {
	return RunReport{
		SetName:  "gold-autolink-1hop",
		TopK:     8,
		Cases:    []CaseResult{perfectCase("a"), perfectCase("b")},
		ChunkAgg: Aggregates{CaseCount: 2, RecallAtK: 1, MRR: 1, HitRate: 1},
		DocAgg:   Aggregates{CaseCount: 2, RecallAtK: 1, MRR: 1, HitRate: 1},
	}
}

func kindsOf(violations []GateViolation) map[GateKind]int {
	counts := make(map[GateKind]int, len(violations))
	for _, v := range violations {
		counts[v.Kind]++
	}
	return counts
}

func TestCheckGoldGate(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(base, cand *RunReport)
		wantKinds map[GateKind]int // 空 = 期望零违规
	}{
		{name: "identical perfect runs pass"},
		{
			name: "stale set name",
			mutate: func(_, cand *RunReport) {
				cand.SetName = "gold-v2"
			},
			wantKinds: map[GateKind]int{GateStale: 1},
		},
		{
			name: "stale topK",
			mutate: func(_, cand *RunReport) {
				cand.TopK = 16
			},
			wantKinds: map[GateKind]int{GateStale: 1},
		},
		{
			name: "candidate search error",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Err = "no seed anchor for query"
				cand.Cases[0].Chunk = CaseMetrics{}
			},
			// 带错用例全零分，会同时跌 recall 与 mrr——只钉 search-error 必含，
			// 不钉死伴随违规的种类数。
			wantKinds: map[GateKind]int{GateSearchError: 1},
		},
		{
			name: "baseline with baked-in search error",
			mutate: func(base, _ *RunReport) {
				base.Cases[1].Err = "transient corpus breakage"
			},
			wantKinds: map[GateKind]int{GateDegradedBaseline: 1},
		},
		{
			name: "degraded baseline recall caught even when candidate improves",
			mutate: func(base, _ *RunReport) {
				base.Cases[0].Chunk.RecallAtK = 0.5
			},
			wantKinds: map[GateKind]int{GateDegradedBaseline: 1},
		},
		{
			name: "degraded baseline mrr caught",
			mutate: func(base, _ *RunReport) {
				base.Cases[0].Chunk.MRR = 0.75
			},
			wantKinds: map[GateKind]int{GateDegradedBaseline: 1},
		},
		{
			name: "set drift both directions",
			mutate: func(_, cand *RunReport) {
				cand.Cases = []CaseResult{perfectCase("a"), perfectCase("c")}
			},
			wantKinds: map[GateKind]int{GateDrift: 1},
		},
		{
			name: "chunk recall regression",
			mutate: func(_, cand *RunReport) {
				cand.Cases[1].Chunk.RecallAtK = 0.5
			},
			wantKinds: map[GateKind]int{GateRecallRegression: 1},
		},
		{
			name: "mrr-only regression (pure rerank, recall untouched)",
			mutate: func(_, cand *RunReport) {
				cand.Cases[1].Chunk.MRR = 0.5
			},
			wantKinds: map[GateKind]int{GateMRRRegression: 1},
		},
		{
			// F7：钉死 0.02+1e-9 边界——阈值被误改到 (0.02, 0.021] 即可被本断言抓住。
			name: "recall drop exactly 0.021 flags",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Chunk.RecallAtK = 0.979
			},
			wantKinds: map[GateKind]int{GateRecallRegression: 1},
		},
		{
			name: "recall drop exactly 0.020 does not flag",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Chunk.RecallAtK = 0.980
			},
		},
		{
			name: "mrr drop exactly 0.021 flags",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Chunk.MRR = 0.979
			},
			wantKinds: map[GateKind]int{GateMRRRegression: 1},
		},
		{
			name: "mrr drop exactly 0.020 does not flag",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Chunk.MRR = 0.980
			},
		},
		{
			name: "recall and mrr regressions accumulate per case",
			mutate: func(_, cand *RunReport) {
				cand.Cases[0].Chunk.RecallAtK = 0.5
				cand.Cases[0].Chunk.MRR = 0.5
			},
			wantKinds: map[GateKind]int{GateRecallRegression: 1, GateMRRRegression: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, cand := perfectReport(), perfectReport()
			if tc.mutate != nil {
				tc.mutate(&base, &cand)
			}
			violations := CheckGoldGate(base, cand)
			if len(tc.wantKinds) == 0 {
				if len(violations) > 0 {
					t.Fatalf("expected clean gate, got %v", violations)
				}
				return
			}
			got := kindsOf(violations)
			for kind, count := range tc.wantKinds {
				if got[kind] < count {
					t.Fatalf("expected ≥%d %q violation(s), got %v", count, kind, violations)
				}
			}
			for _, v := range violations {
				if v.Detail == "" {
					t.Fatalf("violation missing detail: %+v", v)
				}
			}
		})
	}
}

func TestCheckGoldGateRanksDriftAboveRegression(t *testing.T) {
	// 漂移与回归可同时报告；只验证顺序确定性：检索错误先于 diff 违规。
	base, cand := perfectReport(), perfectReport()
	cand.Cases[0].Err = "boom"
	cand.Cases[1].Chunk.RecallAtK = 0.5
	violations := CheckGoldGate(base, cand)
	if len(violations) == 0 || violations[0].Kind != GateSearchError {
		t.Fatalf("first violation kind = %v, want search-error first", violations)
	}
}

func TestValidateGoldBaseline(t *testing.T) {
	if err := ValidateGoldBaseline(perfectReport()); err != nil {
		t.Fatalf("perfect report rejected: %v", err)
	}
	if err := ValidateGoldBaseline(RunReport{}); err == nil {
		t.Fatal("expected error for empty report")
	}

	withErr := perfectReport()
	withErr.Cases[0].Err = "no seed anchor"
	if err := ValidateGoldBaseline(withErr); err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("expected error naming case a, got %v", err)
	}

	degraded := perfectReport()
	degraded.Cases[1].Chunk.RecallAtK = 0.5
	if err := ValidateGoldBaseline(degraded); err == nil || !strings.Contains(err.Error(), "degraded recall") {
		t.Fatalf("expected degraded-recall refusal, got %v", err)
	}

	degradedMRR := perfectReport()
	degradedMRR.Cases[1].Doc.MRR = 0.5
	if err := ValidateGoldBaseline(degradedMRR); err == nil || !strings.Contains(err.Error(), "degraded mrr") {
		t.Fatalf("expected degraded-mrr refusal, got %v", err)
	}
}

func TestValidateGoldExpectations(t *testing.T) {
	ok := EvalSet{Name: "x", Cases: []GoldenCase{
		{ID: "a", Query: "q", ExpectedChunkIDs: []string{"c"}},
		{ID: "b", Query: "q", ExpectedDocIDs: []string{"d"}},
	}}
	if err := ValidateGoldExpectations(ok); err != nil {
		t.Fatalf("valid set rejected: %v", err)
	}
	bad := EvalSet{Name: "x", Cases: []GoldenCase{
		{ID: "ok", Query: "q", ExpectedChunkIDs: []string{"c"}},
		{ID: "vacuous", Query: "q"},
	}}
	err := ValidateGoldExpectations(bad)
	if err == nil || !strings.Contains(err.Error(), "vacuous") {
		t.Fatalf("expected error naming the vacuous case, got %v", err)
	}
}
