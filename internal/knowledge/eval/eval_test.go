package eval

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestRecallAtK(t *testing.T) {
	cases := []struct {
		name     string
		ranked   []string
		expected []string
		k        int
		want     float64
	}{
		{name: "empty expected is vacuous success", ranked: []string{"a"}, expected: nil, k: 5, want: 1.0},
		{name: "k greater than ranked length", ranked: []string{"a", "b"}, expected: []string{"a", "b", "c"}, k: 10, want: 2.0 / 3.0},
		{name: "k zero scores nothing", ranked: []string{"a"}, expected: []string{"a"}, k: 0, want: 0.0},
		{name: "negative k scores nothing", ranked: []string{"a"}, expected: []string{"a"}, k: -1, want: 0.0},
		{name: "hit outside window", ranked: []string{"x", "a"}, expected: []string{"a"}, k: 1, want: 0.0},
		{name: "duplicate expected counts once", ranked: []string{"a", "b"}, expected: []string{"a", "a", "b"}, k: 2, want: 1.0},
		{name: "duplicate ranked counts once", ranked: []string{"a", "a"}, expected: []string{"a", "b"}, k: 2, want: 0.5},
		{name: "no hits", ranked: []string{"x", "y"}, expected: []string{"a"}, k: 5, want: 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertClose(t, RecallAtK(tc.ranked, tc.expected, tc.k), tc.want)
		})
	}
}

func TestMRR(t *testing.T) {
	cases := []struct {
		name     string
		ranked   []string
		expected []string
		want     float64
	}{
		{name: "first position hit is 1.0", ranked: []string{"a", "b"}, expected: []string{"a"}, want: 1.0},
		{name: "third position hit", ranked: []string{"x", "y", "a"}, expected: []string{"a"}, want: 1.0 / 3.0},
		{name: "no hit", ranked: []string{"x"}, expected: []string{"a"}, want: 0.0},
		{name: "empty expected is vacuous success", ranked: []string{"x"}, expected: nil, want: 1.0},
		{name: "empty ranked", ranked: nil, expected: []string{"a"}, want: 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertClose(t, MRR(tc.ranked, tc.expected), tc.want)
		})
	}
}

func TestHitAtK(t *testing.T) {
	assertClose(t, HitAtK([]string{"x", "a"}, []string{"a"}, 2), 1.0)
	assertClose(t, HitAtK([]string{"x", "a"}, []string{"a"}, 1), 0.0)
	assertClose(t, HitAtK([]string{"x"}, nil, 1), 1.0)
	assertClose(t, HitAtK([]string{"a"}, []string{"a"}, 0), 0.0)
}

func TestFirstHitRank(t *testing.T) {
	if got := FirstHitRank([]string{"x", "a"}, []string{"a"}); got != 2 {
		t.Fatalf("FirstHitRank = %d, want 2", got)
	}
	if got := FirstHitRank([]string{"x"}, []string{"a"}); got != 0 {
		t.Fatalf("FirstHitRank = %d, want 0", got)
	}
	if got := FirstHitRank([]string{"x"}, nil); got != 0 {
		t.Fatalf("FirstHitRank empty expected = %d, want 0", got)
	}
}

// fakeSearcher returns canned hits per query, or a canned error.
type fakeSearcher struct {
	hits map[string][]Hit
	errs map[string]error
}

func (f fakeSearcher) Search(_ context.Context, query string, topK int) ([]Hit, error) {
	if err, ok := f.errs[query]; ok {
		return nil, err
	}
	hits := f.hits[query]
	if topK < len(hits) {
		hits = hits[:topK]
	}
	return hits, nil
}

func TestRunnerEndToEnd(t *testing.T) {
	searcher := fakeSearcher{hits: map[string][]Hit{
		"q1": {
			{ChunkID: "c1", DocID: "d1", Score: 0.9},
			{ChunkID: "c2", DocID: "d1", Score: 0.8}, // same doc, deduped on doc basis
			{ChunkID: "c3", DocID: "d2", Score: 0.7},
		},
		"q2": {
			{ChunkID: "x1", DocID: "dx", Score: 0.6},
		},
	}}
	set := EvalSet{Name: "demo", Cases: []GoldenCase{
		{ID: "case1", Query: "q1", ExpectedChunkIDs: []string{"c1", "c3"}, ExpectedDocIDs: []string{"d1", "d2"}},
		{ID: "case2", Query: "q2", ExpectedChunkIDs: []string{"missing"}, ExpectedDocIDs: []string{"dm"}},
		{ID: "case3", Query: "unknown"}, // empty expected: vacuous success
	}}

	report, err := NewRunner(searcher).Run(context.Background(), set, 2)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if report.SetName != "demo" || report.TopK != 2 || len(report.Cases) != 3 {
		t.Fatalf("report header = %s topK=%d cases=%d", report.SetName, report.TopK, len(report.Cases))
	}

	c1 := report.Cases[0]
	assertClose(t, c1.Chunk.RecallAtK, 0.5) // topK=2 keeps c1, drops c3
	assertClose(t, c1.Chunk.MRR, 1.0)
	assertClose(t, c1.Doc.RecallAtK, 0.5) // topK=2 truncation keeps only d1
	if len(c1.RankedDocIDs) != 1 || c1.RankedDocIDs[0] != "d1" {
		t.Fatalf("doc rank list = %v, want [d1] after dedupe+truncation", c1.RankedDocIDs)
	}
	if c1.Chunk.FirstHitRank != 1 {
		t.Fatalf("chunk first hit rank = %d, want 1", c1.Chunk.FirstHitRank)
	}

	c2 := report.Cases[1]
	assertClose(t, c2.Chunk.RecallAtK, 0.0)
	assertClose(t, c2.Chunk.MRR, 0.0)
	assertClose(t, c2.Doc.Hit, 0.0)

	c3 := report.Cases[2]
	assertClose(t, c3.Chunk.RecallAtK, 1.0)
	assertClose(t, c3.Chunk.MRR, 1.0)

	assertClose(t, report.ChunkAgg.RecallAtK, (0.5+0.0+1.0)/3.0)
	assertClose(t, report.ChunkAgg.MRR, (1.0+0.0+1.0)/3.0)
	if report.ChunkAgg.CaseCount != 3 || report.DocAgg.CaseCount != 3 {
		t.Fatalf("aggregate case counts = %d/%d, want 3/3",
			report.ChunkAgg.CaseCount, report.DocAgg.CaseCount)
	}
}

func TestRunnerCaseErrorDoesNotAbortRun(t *testing.T) {
	searcher := fakeSearcher{
		hits: map[string][]Hit{"q-ok": {{ChunkID: "c1", DocID: "d1"}}},
		errs: map[string]error{"q-bad": errors.New("boom")},
	}
	set := EvalSet{Name: "demo", Cases: []GoldenCase{
		{ID: "bad", Query: "q-bad", ExpectedChunkIDs: []string{"c1"}},
		{ID: "ok", Query: "q-ok", ExpectedChunkIDs: []string{"c1"}},
	}}
	report, err := NewRunner(searcher).Run(context.Background(), set, 5)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if report.Cases[0].Err == "" {
		t.Fatalf("expected recorded error on failing case")
	}
	assertClose(t, report.Cases[1].Chunk.RecallAtK, 1.0)
	assertClose(t, report.ChunkAgg.RecallAtK, 0.5)
}

func TestRunnerRejectsBadInput(t *testing.T) {
	if _, err := NewRunner(nil).Run(context.Background(), EvalSet{Name: "x"}, 1); err == nil {
		t.Fatal("expected error for nil searcher")
	}
	if _, err := NewRunner(fakeSearcher{}).Run(context.Background(), EvalSet{Name: "x"}, 0); err == nil {
		t.Fatal("expected error for non-positive topK")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewRunner(fakeSearcher{}).Run(ctx, EvalSet{Name: "x", Cases: []GoldenCase{{ID: "a", Query: "q"}}}, 1)
	if err == nil {
		t.Fatal("expected context cancellation to abort run")
	}
}

func TestRenderMarkdown(t *testing.T) {
	report := RunReport{
		SetName: "demo",
		TopK:    5,
		Cases: []CaseResult{
			{CaseID: "c1", Chunk: CaseMetrics{RecallAtK: 1, MRR: 1, Hit: 1, FirstHitRank: 1}},
			{CaseID: "c2", Chunk: CaseMetrics{}, Err: "boom"},
		},
		ChunkAgg: Aggregates{CaseCount: 2, RecallAtK: 0.5, MRR: 0.5, HitRate: 0.5},
		DocAgg:   Aggregates{CaseCount: 2, RecallAtK: 1, MRR: 1, HitRate: 1},
	}
	md := RenderMarkdown(report)
	for _, want := range []string{
		"# Eval Report: demo",
		"| Case | Recall | MRR | First Hit | Note |",
		"| c1 | 1.000 | 1.000 | 1 |  |",
		"| c2 | 0.000 | 0.000 | - | boom |",
		"| chunk | 2 | 0.500 | 0.500 | 0.500 |",
		"| doc | 2 | 1.000 | 1.000 | 1.000 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

func TestDiffReports(t *testing.T) {
	base := RunReport{
		SetName: "demo",
		TopK:    3,
		Cases: []CaseResult{
			{CaseID: "keep-ok", Chunk: CaseMetrics{RecallAtK: 1.0, MRR: 1.0}, Doc: CaseMetrics{RecallAtK: 1.0}},
			{CaseID: "regress", Chunk: CaseMetrics{RecallAtK: 1.0, MRR: 1.0}, Doc: CaseMetrics{RecallAtK: 1.0, MRR: 1.0}},
			{CaseID: "boundary-flag", Chunk: CaseMetrics{RecallAtK: 1.0}},
			{CaseID: "boundary-keep", Chunk: CaseMetrics{RecallAtK: 1.0}},
			{CaseID: "gone", Chunk: CaseMetrics{RecallAtK: 1.0}},
		},
		ChunkAgg: Aggregates{CaseCount: 5, RecallAtK: 1.0, MRR: 1.0, HitRate: 1.0},
		DocAgg:   Aggregates{CaseCount: 5, RecallAtK: 1.0, MRR: 1.0, HitRate: 1.0},
	}
	cand := RunReport{
		SetName: "demo",
		TopK:    3,
		Cases: []CaseResult{
			{CaseID: "keep-ok", Chunk: CaseMetrics{RecallAtK: 1.0, MRR: 0.5}, Doc: CaseMetrics{RecallAtK: 1.0}},
			// recall drop 0.05 > 0.02 threshold → regression
			{CaseID: "regress", Chunk: CaseMetrics{RecallAtK: 0.95, MRR: 1.0}, Doc: CaseMetrics{RecallAtK: 1.0, MRR: 1.0}},
			// 边界钉死：跌幅恰好 0.021 > 0.02 → flag（阈值被误改到 (0.02, 0.021]
			// 时这些断言会红）。
			{CaseID: "boundary-flag", Chunk: CaseMetrics{RecallAtK: 0.979}},
			{CaseID: "boundary-keep", Chunk: CaseMetrics{RecallAtK: 0.980}},
			{CaseID: "new", Chunk: CaseMetrics{RecallAtK: 1.0}},
		},
		ChunkAgg: Aggregates{CaseCount: 5, RecallAtK: 0.98, MRR: 0.8, HitRate: 1.0},
		DocAgg:   Aggregates{CaseCount: 5, RecallAtK: 1.0, MRR: 1.0, HitRate: 1.0},
	}

	diff := DiffReports(base, cand)
	if diff.RegressionCount != 2 {
		t.Fatalf("RegressionCount = %d, want 2", diff.RegressionCount)
	}
	if len(diff.Cases) != 4 {
		t.Fatalf("shared case diffs = %d, want 4", len(diff.Cases))
	}
	reg := diff.Cases[1]
	if reg.CaseID != "regress" || !reg.Regression {
		t.Fatalf("regression diff = %+v, want regress flagged", reg)
	}
	assertClose(t, reg.ChunkRecall, -0.05)
	keep := diff.Cases[0]
	if keep.Regression {
		t.Fatalf("small MRR drop must not flag regression: %+v", keep)
	}
	if flag := diff.Cases[2]; flag.CaseID != "boundary-flag" || !flag.Regression {
		t.Fatalf("drop exactly 0.021 must flag regression: %+v", flag)
	}
	if bkeep := diff.Cases[3]; bkeep.CaseID != "boundary-keep" || bkeep.Regression {
		t.Fatalf("drop exactly 0.020 must NOT flag regression (loss up to threshold tolerated): %+v", bkeep)
	}
	if len(diff.RemovedCaseIDs) != 1 || diff.RemovedCaseIDs[0] != "gone" {
		t.Fatalf("removed = %v, want [gone]", diff.RemovedCaseIDs)
	}
	if len(diff.AddedCaseIDs) != 1 || diff.AddedCaseIDs[0] != "new" {
		t.Fatalf("added = %v, want [new]", diff.AddedCaseIDs)
	}
	assertClose(t, diff.Chunk.RecallAtK.Delta, 0.98-1.0)
	assertClose(t, diff.Chunk.MRR.Base, 1.0)
}

func TestRenderDiffMarkdown(t *testing.T) {
	base := RunReport{
		SetName: "demo", TopK: 8,
		Cases: []CaseResult{
			{CaseID: "ok", Chunk: CaseMetrics{RecallAtK: 1, MRR: 1}},
			{CaseID: "reg", Chunk: CaseMetrics{RecallAtK: 1, MRR: 1}},
		},
		ChunkAgg: Aggregates{CaseCount: 2, RecallAtK: 1, MRR: 1, HitRate: 1},
	}
	cand := RunReport{
		SetName: "demo", TopK: 8,
		Cases: []CaseResult{
			{CaseID: "ok", Chunk: CaseMetrics{RecallAtK: 1, MRR: 0.9}},
			{CaseID: "reg", Chunk: CaseMetrics{RecallAtK: 0.5, MRR: 1}},
			{CaseID: "new", Chunk: CaseMetrics{RecallAtK: 1}},
		},
		ChunkAgg: Aggregates{CaseCount: 3, RecallAtK: 0.83, MRR: 0.97, HitRate: 1},
	}
	md := RenderDiffMarkdown(DiffReports(base, cand))
	for _, want := range []string{
		"# Eval Diff: demo",
		"- regressions: 1 (chunk recall drop > 0.020)",
		"- added cases: new",
		"| chunk | recall | 1.000 | 0.830 | -0.170 |",
		"| ok | +0.000 | -0.100 | +0.000 | +0.000 |  |",
		"⚠️ regression",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("diff markdown missing %q:\n%s", want, md)
		}
	}
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("metric = %v, want %v", got, want)
	}
}
