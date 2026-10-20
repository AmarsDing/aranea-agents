package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
)

func TestParseHybridSearchMode(t *testing.T) {
	tests := []struct {
		input string
		want  HybridSearchMode
	}{
		{"", HybridAuto},
		{"auto", HybridAuto},
		{"dense", HybridDense},
		{"sparse", HybridSparse},
		{"rrf", HybridRRF},
		{"unknown", HybridAuto},
	}
	for _, tt := range tests {
		got := ParseHybridSearchMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseHybridSearchMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRRFMerge_ScoreBreakdown(t *testing.T) {
	// G-12：RRF 融合保留两侧原始分 + 融合分，回答「为什么命中」。
	dense := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 0.9},
		{ID: "b", Content: "beta", Score: 0.7},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "b", Content: "beta", Score: 11},
		{ID: "c", Content: "gamma", Score: 10},
	}

	merged := rrfMerge(dense, sparse, 60)
	byID := make(map[string]biz.KnowledgeChunk, len(merged))
	for _, ch := range merged {
		byID[ch.ID] = ch
	}

	a := byID["a"]
	if a.DenseScore != 0.9 {
		t.Errorf("a.DenseScore = %v, want 0.9 (dense 侧原始分)", a.DenseScore)
	}
	if a.SparseScore != 0 {
		t.Errorf("a.SparseScore = %v, want 0（未命中 sparse 侧）", a.SparseScore)
	}
	wantA := float32(1.0 / 61.0)
	if !float32Near(a.RRFScore, wantA) || a.Score != a.RRFScore {
		t.Errorf("a RRF = %v Score = %v, want %v 且 Score==RRFScore", a.RRFScore, a.Score, wantA)
	}

	b := byID["b"]
	if b.DenseScore != 0.7 || b.SparseScore != 11 {
		t.Errorf("b breakdown = dense %v sparse %v, want 0.7/11", b.DenseScore, b.SparseScore)
	}
	wantB := float32(1.0/62.0 + 1.0/61.0)
	if !float32Near(b.RRFScore, wantB) {
		t.Errorf("b.RRFScore = %v, want %v（dense#2 + sparse#1 贡献和）", b.RRFScore, wantB)
	}

	c := byID["c"]
	if c.SparseScore != 10 || c.DenseScore != 0 {
		t.Errorf("c breakdown = dense %v sparse %v, want 0/10", c.DenseScore, c.SparseScore)
	}
	wantC := float32(1.0 / 62.0)
	if !float32Near(c.RRFScore, wantC) {
		t.Errorf("c.RRFScore = %v, want %v", c.RRFScore, wantC)
	}
}

func float32Near(got, want float32) bool {
	const eps = 1e-6
	d := got - want
	return d < eps && d > -eps
}

func TestRerankChunks_CarriesScoreBreakdown(t *testing.T) {
	// G-12：rerank 重写 Score 后分解不丢——原始分路从候选集并回，
	// RerankScore = 重排后分数；MetadataJSON 一并回补（reranker 结果元数据原本就丢它）。
	repo := &stubSparseRepo{}
	rr := &reverseReranker{}
	ret := NewRetriever(stubEmbedder{}, repo, rr, loggateway.NewNoop())

	candidates := []biz.KnowledgeChunk{
		{ID: "x", DocID: "d1", CollectionID: "col", Content: "c1", MetadataJSON: `{"k":1}`, Score: 0.20, DenseScore: 0.8, RRFScore: 0.20},
		{ID: "y", DocID: "d2", CollectionID: "col", Content: "c2", MetadataJSON: `{"k":2}`, Score: 0.10, SparseScore: 9.5, RRFScore: 0.10},
	}
	got, err := ret.RerankChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 2,
	}, candidates, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !rr.called {
		t.Fatalf("rerank 未生效：got %d chunks, called=%v", len(got), rr.called)
	}
	byID := make(map[string]biz.KnowledgeChunk, len(got))
	for _, ch := range got {
		byID[ch.ID] = ch
	}
	x := byID["x"]
	if x.DenseScore != 0.8 || x.SparseScore != 0 || x.RRFScore != 0.20 {
		t.Errorf("x breakdown 丢失：dense=%v sparse=%v rrf=%v", x.DenseScore, x.SparseScore, x.RRFScore)
	}
	if x.MetadataJSON != `{"k":1}` {
		t.Errorf("x.MetadataJSON = %q, want 回补的原始 metadata", x.MetadataJSON)
	}
	if x.RerankScore != x.Score {
		t.Errorf("x.RerankScore = %v, want == 重排后 Score %v", x.RerankScore, x.Score)
	}
	y := byID["y"]
	if y.SparseScore != 9.5 || y.MetadataJSON != `{"k":2}` {
		t.Errorf("y breakdown/metadata 丢失：sparse=%v meta=%q", y.SparseScore, y.MetadataJSON)
	}
}

func TestRRFMerge(t *testing.T) {
	dense := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 0.9},
		{ID: "b", Content: "beta", Score: 0.7},
		{ID: "c", Content: "gamma", Score: 0.5},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "b", Content: "beta", Score: 0.01},
		{ID: "d", Content: "delta", Score: 0.008},
		{ID: "a", Content: "alpha", Score: 0.005},
	}

	merged := rrfMerge(dense, sparse, 60)
	if len(merged) != 4 {
		t.Fatalf("expected 4 unique chunks, got %d", len(merged))
	}

	ids := make(map[string]bool)
	for _, ch := range merged {
		ids[ch.ID] = true
	}
	for _, want := range []string{"a", "b", "c", "d"} {
		if !ids[want] {
			t.Errorf("missing chunk %q in merged results", want)
		}
	}

	if merged[0].ID != "a" && merged[0].ID != "b" {
		t.Errorf("expected 'a' or 'b' to rank first (both appear in both lists), got %q", merged[0].ID)
	}
}

func TestMergeSearchResults(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
	}
	supplement := []biz.KnowledgeChunk{
		{ID: "2", Score: 0.8},
		{ID: "3", Score: 0.6},
	}

	merged := MergeSearchResults(primary, supplement, 5)
	if len(merged) != 3 {
		t.Fatalf("expected 3 unique chunks, got %d", len(merged))
	}
	if merged[0].Score < merged[1].Score {
		t.Errorf("results not sorted by score descending")
	}
}

func TestMergeSearchResultsTopK(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
		{ID: "3", Score: 0.5},
	}
	merged := MergeSearchResults(primary, nil, 2)
	if len(merged) != 2 {
		t.Fatalf("expected 2 chunks with topK=2, got %d", len(merged))
	}
}

type reverseReranker struct {
	called    bool
	lastInput []string
}

func (r *reverseReranker) Rerank(_ context.Context, _ *reranker.Query, in []*reranker.Result) ([]*reranker.Result, error) {
	r.called = true
	for _, item := range in {
		r.lastInput = append(r.lastInput, item.Document.ID)
	}
	out := append([]*reranker.Result(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func TestHybridRetriever_RRFAppliesConfiguredReranker(t *testing.T) {
	repo := &stubSparseRepo{
		stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{
			{ID: "dense", DocID: "d1", Content: "dense"},
		}},
		bm25Chunks: []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2", Content: "sparse"}},
	}
	rr := &reverseReranker{}
	ret := NewRetriever(stubEmbedder{}, repo, rr, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "query", TopK: 2,
	}, HybridRRF)
	if err != nil {
		t.Fatal(err)
	}
	if !rr.called {
		t.Fatal("configured reranker was not called after RRF fusion")
	}
	if len(got) != 2 || len(rr.lastInput) != 2 || got[0].ID != rr.lastInput[1] {
		t.Fatalf("RRF rerank result = %+v, want reversed order", got)
	}
}

type parallelSearchRepo struct {
	stubKnowledgeRepo
	denseStarted  chan struct{}
	sparseStarted chan struct{}
	release       chan struct{}
	onceDense     sync.Once
	onceSparse    sync.Once
}

func (r *parallelSearchRepo) SearchChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	r.onceDense.Do(func() { close(r.denseStarted) })
	<-r.release
	return []biz.KnowledgeChunk{{ID: "dense", DocID: "d1"}}, nil
}

func (r *parallelSearchRepo) SearchChunksBM25(_ context.Context, _ biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	r.onceSparse.Do(func() { close(r.sparseStarted) })
	<-r.release
	return []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2"}}, nil
}

func TestHybridRetriever_RRFStartsDenseAndSparseConcurrently(t *testing.T) {
	repo := &parallelSearchRepo{
		denseStarted:  make(chan struct{}),
		sparseStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	done := make(chan error, 1)
	safego.Go(context.Background(), "test.knowledge.rrf_parallel", func() {
		_, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
			CollectionID: "col", Query: "query", TopK: 2,
		}, HybridRRF)
		done <- err
	})

	for name, started := range map[string]<-chan struct{}{
		"dense": repo.denseStarted, "sparse": repo.sparseStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(repo.release)
			t.Fatalf("%s search did not start before the peer was released", name)
		}
	}
	close(repo.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// G-06 对抗评审 F1 回归：RRF 融合字面量重建块时不得丢 ParentChunkID——
// 它是 Router 尾步父块语境回填的唯一触发键，双侧命中路径都要保。
func TestRRFMerge_PreservesParentChunkID(t *testing.T) {
	dense := []biz.KnowledgeChunk{
		{ID: "d-ch", Content: "dense child", Score: 0.9, ParentChunkID: "d-p-0"},
		{ID: "both", Content: "dual hit", Score: 0.8, ParentChunkID: "b-p-0"},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "s-ch", Content: "sparse child", Score: 11, ParentChunkID: "s-p-0"},
		{ID: "both", Content: "dual hit", Score: 10, ParentChunkID: "b-p-s"},
	}

	merged := rrfMerge(dense, sparse, 60)
	byID := make(map[string]biz.KnowledgeChunk, len(merged))
	for _, ch := range merged {
		byID[ch.ID] = ch
	}
	if got := byID["d-ch"].ParentChunkID; got != "d-p-0" {
		t.Errorf("dense-only chunk lost ParentChunkID: %q", got)
	}
	if got := byID["s-ch"].ParentChunkID; got != "s-p-0" {
		t.Errorf("sparse-only chunk lost ParentChunkID: %q", got)
	}
	// 双侧同 ID：chunkMap 先记 dense 侧，父引用取 dense 值——关键是不丢。
	if got := byID["both"].ParentChunkID; got != "b-p-0" {
		t.Errorf("dual-hit chunk lost ParentChunkID: %q (dense side wins)", got)
	}
}

// G-06 对抗评审 F2 回归：reranker 结果只带 ID/Content/新分，mergeScoreBreakdown
// 必须从融合前候选集并回 ParentChunkID——否则重排后父块回填触发键丢失。
func TestRerankChunks_PreservesParentChunkID(t *testing.T) {
	repo := &stubSparseRepo{}
	rr := &reverseReranker{}
	ret := NewRetriever(stubEmbedder{}, repo, rr, loggateway.NewNoop())

	candidates := []biz.KnowledgeChunk{
		{ID: "x", DocID: "d1", CollectionID: "col", Content: "c1", Score: 0.20, ParentChunkID: "d1-p-0"},
		{ID: "y", DocID: "d2", CollectionID: "col", Content: "c2", Score: 0.10, ParentChunkID: "d2-p-0"},
	}
	got, err := ret.RerankChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 2,
	}, candidates, 2)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]biz.KnowledgeChunk, len(got))
	for _, ch := range got {
		byID[ch.ID] = ch
	}
	if byID["x"].ParentChunkID != "d1-p-0" || byID["y"].ParentChunkID != "d2-p-0" {
		t.Fatalf("rerank path dropped ParentChunkID: %+v", got)
	}
}

// G-06 F1+F6 全链回归：fake dense/sparse → HybridRRF 融合 → Router.Search 尾步
// 触发一次父块批量回填——锁定「融合丢字段 → 回填静默跳过」的回归链。
func TestRouterSearch_RRFChainBackfillsParentContext(t *testing.T) {
	repo := &stubSparseRepo{
		stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{
			{ID: "doc1-ch-0", DocID: "doc1", CollectionID: "col", Content: "子块0",
				Score: 0.9, ParentChunkID: "doc1-p-0"},
		}},
		bm25Chunks: []biz.KnowledgeChunk{
			{ID: "doc1-ch-1", DocID: "doc1", CollectionID: "col", Content: "子块1",
				Score: 10, ParentChunkID: "doc1-p-0"},
		},
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	hybrid := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	router := NewAdaptiveRouter(hybrid, nil, loggateway.NewNoop())
	resolver := &stubParentResolver{parents: []biz.KnowledgeChunk{
		{ID: "doc1-p-0", Content: "父块零语境"},
	}}
	router.SetParentContextResolver(resolver)

	got, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "物料", TopK: 5},
		nil, HybridRRF)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 merged chunks, got %d: %+v", len(got), got)
	}
	if len(resolver.calls) != 1 {
		t.Fatalf("parent resolver must be called exactly once after RRF fusion, got %d", len(resolver.calls))
	}
	if len(resolver.calls[0]) != 1 || resolver.calls[0][0] != "doc1-p-0" {
		t.Fatalf("resolver got wrong parent ids: %v", resolver.calls[0])
	}
	for _, ch := range got {
		if ch.ParentContent != "父块零语境" {
			t.Fatalf("chunk %s missing backfilled ParentContent: %+v", ch.ID, ch)
		}
	}
}

// ===== G-33（ADR-KN-20261020-I）：rrfMergeN 变参核心 + 卡片第三腿 =====

func TestRRFMergeN_ThreeListScoreBreakdown(t *testing.T) {
	// 卡片腿同权 1/(k+rank+1)：实体卡 #1 → 1/61，边卡 #2 → 1/62；未命中腿
	// 分解为零值；EntityScore/MetadataJSON 携带照常。
	dense := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 0.9},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 11},
	}
	cards := []biz.KnowledgeChunk{
		{ID: "g33e/7/d1", DocID: "d1", CollectionID: "col", Content: "实体卡文", EntityScore: 0.77, MetadataJSON: `{"g33":"entity","g33_ref":7}`},
		{ID: "g33r/9/d2", DocID: "d2", CollectionID: "col", Content: "边卡文", EntityScore: 0.66, MetadataJSON: `{"g33":"edge","g33_ref":9}`},
	}

	merged := rrfMergeN(60, dense, sparse, cards)
	byID := make(map[string]biz.KnowledgeChunk, len(merged))
	for _, ch := range merged {
		byID[ch.ID] = ch
	}
	if len(merged) != 3 {
		t.Fatalf("want 3 unique chunks, got %d", len(merged))
	}

	a := byID["a"]
	if want := float32(1.0/61.0 + 1.0/61.0); !float32Near(a.RRFScore, want) || a.Score != a.RRFScore {
		t.Errorf("a RRF=%v Score=%v want %v（dense+sparse 双 #1）", a.RRFScore, a.Score, want)
	}
	if a.DenseScore != 0.9 || a.SparseScore != 11 || a.EntityScore != 0 {
		t.Errorf("a breakdown = %v/%v/%v, want 0.9/11/0（未命中卡片腿）", a.DenseScore, a.SparseScore, a.EntityScore)
	}

	e := byID["g33e/7/d1"]
	if want := float32(1.0 / 61.0); !float32Near(e.RRFScore, want) || e.Score != e.RRFScore {
		t.Errorf("entity card RRF=%v want %v（卡 #1 贡献）", e.RRFScore, want)
	}
	if e.DenseScore != 0 || e.SparseScore != 0 {
		t.Errorf("entity card dense/sparse = %v/%v, want 双零（未命中两侧）", e.DenseScore, e.SparseScore)
	}
	if e.EntityScore != 0.77 {
		t.Errorf("entity card EntityScore = %v, want 0.77 过融合", e.EntityScore)
	}
	if e.MetadataJSON != `{"g33":"entity","g33_ref":7}` {
		t.Errorf("entity card MetadataJSON = %q, want g33 标记过融合", e.MetadataJSON)
	}

	r := byID["g33r/9/d2"]
	if want := float32(1.0 / 62.0); !float32Near(r.RRFScore, want) {
		t.Errorf("edge card RRF=%v want %v（卡 #2 贡献）", r.RRFScore, want)
	}
	if r.EntityScore != 0.66 {
		t.Errorf("edge card EntityScore = %v, want 0.66", r.EntityScore)
	}
}

// assertChunksSame 逐字段比较（Chunk 含 []float32 Embedding 不能直接 !=；
// 钉全投影面，字面量重建纪律的保真担保）。
func assertChunksSame(t *testing.T, i int, a, b biz.KnowledgeChunk) {
	t.Helper()
	if a.ID != b.ID || a.DocID != b.DocID || a.CollectionID != b.CollectionID ||
		a.Content != b.Content || a.ChunkIndex != b.ChunkIndex || a.ParentChunkID != b.ParentChunkID ||
		a.IsParent != b.IsParent || a.ParentContent != b.ParentContent ||
		a.Score != b.Score || a.DenseScore != b.DenseScore || a.SparseScore != b.SparseScore ||
		a.RRFScore != b.RRFScore || a.RerankScore != b.RerankScore || a.EntityScore != b.EntityScore ||
		a.MetadataJSON != b.MetadataJSON || len(a.Embedding) != len(b.Embedding) {
		t.Fatalf("index %d mismatch: %+v vs %+v", i, a, b)
	}
}

func TestRRFMergeN_TwoListMatchesLegacyWrapper(t *testing.T) {
	// 包装保真钉：rrfMerge(d,s,60) 与 rrfMergeN(60,d,s) 输出逐字段一致——
	// TestRRFMerge_ScoreBreakdown 等存量精确分值测试的零改动由本钉担保。
	dense := []biz.KnowledgeChunk{
		{ID: "a", DocID: "d1", Content: "alpha", Score: 0.9, ParentChunkID: "d1-p-0"},
		{ID: "b", DocID: "d2", Content: "beta", Score: 0.7},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "b", DocID: "d2", Content: "beta", Score: 11},
		{ID: "c", DocID: "d3", Content: "gamma", Score: 10},
	}
	legacy := rrfMerge(dense, sparse, 60)
	variadic := rrfMergeN(60, dense, sparse)
	if len(legacy) != len(variadic) {
		t.Fatalf("length mismatch: %d vs %d", len(legacy), len(variadic))
	}
	for i := range legacy {
		assertChunksSame(t, i, legacy[i], variadic[i])
	}
}

func TestRRFMergeN_NilCardListSameAsLegacy(t *testing.T) {
	// 腿关闭（装配 nil 关腿）时第三列传 nil——融合结果与两腿版完全一致。
	dense := []biz.KnowledgeChunk{{ID: "a", Content: "alpha", Score: 0.9}}
	sparse := []biz.KnowledgeChunk{{ID: "a", Content: "alpha", Score: 11}}
	legacy := rrfMerge(dense, sparse, 60)
	withNilLeg := rrfMergeN(60, dense, sparse, nil)
	if len(legacy) != len(withNilLeg) {
		t.Fatalf("nil card list must not change fusion length: %+v vs %+v", legacy, withNilLeg)
	}
	assertChunksSame(t, 0, legacy[0], withNilLeg[0])
}

func TestRRFMergeN_CarriesCardProjectionFields(t *testing.T) {
	// 宿主投影契约钉：卡候选的 DocID/CollectionID/Content 为真实值并过融合——
	// 父回填/基础加成/access_log/Hebbian/GraphExpander 种子的下游继承全靠它们
	// （同 G-06 字面量重建纪律）。
	cards := []biz.KnowledgeChunk{{
		ID: "g33e/7/d1", DocID: "d1", CollectionID: "col", Content: "卡片文",
		ChunkIndex: 0, MetadataJSON: `{"g33":"entity","g33_ref":7}`, EntityScore: 0.77,
	}}
	merged := rrfMergeN(60, nil, nil, cards)
	if len(merged) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(merged))
	}
	got := merged[0]
	if got.DocID != "d1" || got.CollectionID != "col" || got.Content != "卡片文" {
		t.Fatalf("projection fields lost: %+v", got)
	}
	if want := float32(1.0 / 61.0); !float32Near(got.Score, want) || got.RRFScore != got.Score {
		t.Fatalf("Score=%v RRFScore=%v want %v 且两者相等（mode_used 保持 rrf）", got.Score, got.RRFScore, want)
	}
}
