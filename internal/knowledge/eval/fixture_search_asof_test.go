package eval

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── G-05-asof：时态金色探针（决策 a–f，设计文档「时态金色探针」段） ───────────
// 钉点：a) GoldenCase 可选 valid_at/recorded_at（RFC3339）；b) FixtureCorpus
// 声明式 edges 时间窗；c) buildFixtureSearcher 声明即接线 active+temporal 双腿；
// d) Runner 经 TemporalSearcher 窄接口机会断言（旧 14 探针零签名漂移——零 span
// 用例行为字节级不变，缺端口检索器遇时态用例记 Err）；e) 独立第二评测集。

var (
	asOfT0 = "2026-01-01T00:00:00Z"
	asOfT2 = "2026-02-01T00:00:00Z"
)

// asOfProbeCorpus 复刻数据层四象限矩阵的最小世界：hub 为唯一种子文档，
// A 存续 / B T2 闭合 / C T2 生效；distract 孤立（隔离钉）。
func asOfProbeCorpus() FixtureCorpus {
	return FixtureCorpus{
		Docs: []FixtureDoc{
			{ID: "hub", Title: "时态中枢", Body: "中枢正文，不含 wiki 提及。"},
			{ID: "persist", Title: "存续目标", Body: "存续目标正文。"},
			{ID: "closed", Title: "闭合目标", Body: "闭合目标正文。"},
			{ID: "future", Title: "未来目标", Body: "未来目标正文。"},
			{ID: "distract", Title: "孤立文档", Body: "孤立文档正文。"},
		},
		Seeds: map[string]string{"从时态中枢顺出": "hub"},
		Edges: []FixtureEdgeDecl{
			{ID: "A", Doc: "hub", Target: "persist", LinkType: "explicit", ValidFrom: asOfT0, RecordedAt: asOfT0},
			{ID: "B", Doc: "hub", Target: "closed", LinkType: "semantic", ValidFrom: asOfT0, ValidTo: asOfT2, RecordedAt: asOfT0},
			{ID: "C", Doc: "hub", Target: "future", LinkType: "semantic", ValidFrom: asOfT2, RecordedAt: asOfT2},
		},
	}
}

func asOfHits(t *testing.T, s Searcher, query string, span bizknowledge.TemporalSpan) map[string]bool {
	t.Helper()
	ts, ok := s.(TemporalSearcher)
	if !ok {
		t.Fatalf("fixture searcher must implement TemporalSearcher")
	}
	hits, err := ts.SearchAsOf(context.Background(), query, 8, span)
	if err != nil {
		t.Fatalf("SearchAsOf: %v", err)
	}
	docs := map[string]bool{}
	for _, h := range hits {
		docs[h.DocID] = true
	}
	return docs
}

func wantDocSet(t *testing.T, name string, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: doc set = %v, want %v", name, got, want)
		return
	}
	for _, d := range want {
		if !got[d] {
			t.Errorf("%s: missing doc %s in %v", name, d, got)
		}
	}
}

func TestFixtureSearcherAsOf_QuadrantMatrix(t *testing.T) {
	s, err := NewFixtureSearcher(asOfProbeCorpus())
	if err != nil {
		t.Fatalf("NewFixtureSearcher: %v", err)
	}
	t1, _ := time.Parse(time.RFC3339, "2026-01-15T00:00:00Z")
	t2, _ := time.Parse(time.RFC3339, asOfT2)
	r0, _ := time.Parse(time.RFC3339, "2026-01-10T00:00:00Z")
	rmid, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")

	cases := []struct {
		name string
		span bizknowledge.TemporalSpan
		want []string
	}{
		// 零 span 经 Search（非 SearchAsOf）——当前快照：B 闭合剔除、C 生效。
		{"zero-span-current", bizknowledge.TemporalSpan{}, []string{"hub", "persist", "future"}},
		// T 轴：t1 处 B 仍有效、C 未生效。
		{"t1", bizknowledge.TemporalSpan{ValidAt: t1}, []string{"hub", "persist", "closed"}},
		// T 轴：t2 处 B 恰好失效（valid_to 排他）、C 生效。
		{"t2", bizknowledge.TemporalSpan{ValidAt: t2}, []string{"hub", "persist", "future"}},
		// R 轴：R0 处 C 尚未被记录；B 的闭合尚未被记录（valid_to=T2 > R0），视作存续。
		{"r0", bizknowledge.TemporalSpan{RecordedAt: r0}, []string{"hub", "persist", "closed"}},
		// 双轴：T1∧R0——B 经 @>T1 分支存续；C 记录时间未到。
		{"dual-t1-r0", bizknowledge.TemporalSpan{ValidAt: t1, RecordedAt: r0}, []string{"hub", "persist", "closed"}},
		// 双轴：T2∧Rmid——C 经 @>T2 分支生效；B 双分支均已失效。
		{"dual-t2-rmid", bizknowledge.TemporalSpan{ValidAt: t2, RecordedAt: rmid}, []string{"hub", "persist", "future"}},
	}
	for _, tc := range cases {
		got := asOfHits(t, s, "从时态中枢顺出", tc.span)
		wantDocSet(t, tc.name, got, tc.want...)
	}
}

func TestFixtureSearcherAsOf_ZeroSpanByteDelegates(t *testing.T) {
	s, err := NewFixtureSearcher(asOfProbeCorpus())
	if err != nil {
		t.Fatalf("NewFixtureSearcher: %v", err)
	}
	// Search 与 SearchAsOf(零 span) 必须字节级同集（零 span 走旧端口）。
	plain, err := s.Search(context.Background(), "从时态中枢顺出", 8)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	viaAsof := asOfHits(t, s, "从时态中枢顺出", bizknowledge.TemporalSpan{})
	plainDocs := map[string]bool{}
	for _, h := range plain {
		plainDocs[h.DocID] = true
	}
	if len(plainDocs) != len(viaAsof) {
		t.Fatalf("zero-span sets diverge: Search=%v SearchAsOf=%v", plainDocs, viaAsof)
	}
	for d := range plainDocs {
		if !viaAsof[d] {
			t.Errorf("zero-span SearchAsOf missing doc %s (Search had it)", d)
		}
	}
}

func TestFixtureCorpusEdges_Validation(t *testing.T) {
	base := asOfProbeCorpus()
	bads := []struct {
		name string
		edit func(*FixtureCorpus)
	}{
		{"unknown doc", func(c *FixtureCorpus) { c.Edges[0].Doc = "ghost" }},
		{"unknown target", func(c *FixtureCorpus) { c.Edges[0].Target = "ghost" }},
		{"co_activated banned", func(c *FixtureCorpus) { c.Edges[0].LinkType = "co_activated" }},
		{"empty link type", func(c *FixtureCorpus) { c.Edges[0].LinkType = "  " }},
		{"empty edge id", func(c *FixtureCorpus) { c.Edges[0].ID = " " }},
		{"duplicate edge id", func(c *FixtureCorpus) { c.Edges[1].ID = c.Edges[0].ID }},
		{"bad valid_from", func(c *FixtureCorpus) { c.Edges[0].ValidFrom = "once" }},
		{"bad valid_to", func(c *FixtureCorpus) { c.Edges[0].ValidTo = "later" }},
		{"bad recorded_at", func(c *FixtureCorpus) { c.Edges[0].RecordedAt = "2026-01-01" }},
	}
	for _, tc := range bads {
		c := base
		c.Edges = append([]FixtureEdgeDecl{}, base.Edges...)
		tc.edit(&c)
		if _, err := NewFixtureSearcher(c); err == nil {
			t.Errorf("%s: want validation error", tc.name)
		}
	}
}

// 仅存 Search 接口的旧检索器（零 as-of 能力桩）。
type plainOnlySearcher struct{}

func (plainOnlySearcher) Search(_ context.Context, _ string, _ int) ([]Hit, error) {
	return []Hit{{ChunkID: "x-c0", DocID: "x", Score: 1}}, nil
}

func TestRunner_TemporalCasesNeedTemporalSearcher(t *testing.T) {
	set := EvalSet{Name: "tmp", Cases: []GoldenCase{
		// 零 span 用例对旧检索器行为字节级不变。
		{ID: "plain", Query: "q", ExpectedChunkIDs: []string{"x-c0"}},
		{ID: "needs-span", Query: "q", ExpectedChunkIDs: []string{"x-c0"}, ValidAt: asOfT0},
	}}
	rep, err := NewRunner(plainOnlySearcher{}).Run(context.Background(), set, 8)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range rep.Cases {
		switch c.CaseID {
		case "plain":
			if c.Err != "" {
				t.Errorf("zero-span case must not error on plain searcher: %s", c.Err)
			}
		case "needs-span":
			if c.Err == "" {
				t.Errorf("temporal case on plain searcher must record Err (drift pinned)")
			}
		}
	}
}

func TestRunner_TemporalCaseSpansReachSearcher(t *testing.T) {
	s, err := NewFixtureSearcher(asOfProbeCorpus())
	if err != nil {
		t.Fatalf("NewFixtureSearcher: %v", err)
	}
	set := EvalSet{Name: "golden-temporal", Cases: []GoldenCase{
		{ID: "t1", Query: "从时态中枢顺出", ExpectedDocIDs: []string{"hub", "persist", "closed"},
			ExpectedChunkIDs: []string{"hub-c0", "persist-c0", "closed-c0"}, ValidAt: "2026-01-15T00:00:00Z"},
		{ID: "t2", Query: "从时态中枢顺出", ExpectedDocIDs: []string{"hub", "persist", "future"},
			ExpectedChunkIDs: []string{"hub-c0", "persist-c0", "future-c0"}, ValidAt: asOfT2},
		{ID: "bad-span", Query: "从时态中枢顺出", ExpectedDocIDs: []string{"hub"}, ValidAt: "not-a-time"},
	}}
	rep, err := NewRunner(s).Run(context.Background(), set, 8)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, c := range rep.Cases {
		switch c.CaseID {
		case "t1", "t2":
			if c.Err != "" {
				t.Errorf("%s: unexpected Err %s", c.CaseID, c.Err)
			}
			if c.Chunk.RecallAtK < 1.0 || c.Doc.RecallAtK < 1.0 {
				t.Errorf("%s: recall degraded: chunk %.3f doc %.3f", c.CaseID, c.Chunk.RecallAtK, c.Doc.RecallAtK)
			}
		case "bad-span":
			if c.Err == "" {
				t.Errorf("bad-span case must record Err (RFC3339 pinned at runner)")
			}
		}
	}
}

func TestFixtureSearcherAsOf_WeightAndCurrentValueCarry(t *testing.T) {
	// ADR-J：weight 返回当前值——as-of 命中块的分数映射用当前 weight_f（fixture
	// 声明缺省 1 亦可行为性验证：显式边 1.0 / 语义边 0.9 类型权重分差都在）。
	s, err := NewFixtureSearcher(asOfProbeCorpus())
	if err != nil {
		t.Fatalf("NewFixtureSearcher: %v", err)
	}
	ts := s.(TemporalSearcher)
	t2, _ := time.Parse(time.RFC3339, asOfT2)
	hits, err := ts.SearchAsOf(context.Background(), "从时态中枢顺出", 8, bizknowledge.TemporalSpan{ValidAt: t2})
	if err != nil {
		t.Fatalf("SearchAsOf: %v", err)
	}
	if len(hits) == 0 || hits[0].DocID != "hub" {
		t.Fatalf("rank-1 must be seed hub, got %+v", hits)
	}
	// 种子分数高于扩散块（activation < 1），MRR=1 的探针世界由此成立。
	for _, h := range hits[1:] {
		if h.Score >= hits[0].Score {
			t.Errorf("spread chunk %s score %.3f >= seed %.3f — probe world broken", h.ChunkID, h.Score, hits[0].Score)
		}
	}
}

var _ = biz.KnowledgeSearchQuery{} // 防误删 biz 导入（SearchAsOf 实现拼装 query 用）
