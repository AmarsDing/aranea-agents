package knowledge

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ===== G-33（ADR-KN-20261020-H/I）：卡片检索器宿主投影与候选构造 =====

type stubCardVectorSearcher struct {
	entityHits []bizknowledge.GraphCardHit
	edgeHits   []bizknowledge.GraphCardHit
	err        error
	kindSeen   []string
}

func (s *stubCardVectorSearcher) SearchGraphCards(_ context.Context, _ string, kind string, _ []float32, limit int) ([]bizknowledge.GraphCardHit, error) {
	s.kindSeen = append(s.kindSeen, kind)
	if s.err != nil {
		return nil, s.err
	}
	if kind == bizknowledge.GraphCardKindEdge {
		if len(s.edgeHits) > limit {
			return s.edgeHits[:limit], nil
		}
		return s.edgeHits, nil
	}
	if len(s.entityHits) > limit {
		return s.entityHits[:limit], nil
	}
	return s.entityHits, nil
}

type stubCardLexicalSearcher struct {
	hits []bizknowledge.GraphCardHit
	err  error
}

func (s *stubCardLexicalSearcher) SearchEntityCardsLexical(_ context.Context, _, _ string, _ int) ([]bizknowledge.GraphCardHit, error) {
	return s.hits, s.err
}

type stubCardHostDocs struct {
	byRef map[int64][]string
	err   map[int64]error
}

func (s *stubCardHostDocs) ListGraphCardHostDocs(_ context.Context, _ string, _ string, refID int64, _ int) ([]string, error) {
	if err := s.err[refID]; err != nil {
		return nil, err
	}
	return s.byRef[refID], nil
}

func newTestCardSearcher(vector *stubCardVectorSearcher, lexical *stubCardLexicalSearcher, hosts *stubCardHostDocs) GraphCardSearcher {
	return NewGraphCardSearcher(vector, lexical, hosts, loggateway.NewNoop())
}

func TestGraphCardSearcher_DensePathProjectsHosts(t *testing.T) {
	vector := &stubCardVectorSearcher{
		entityHits: []bizknowledge.GraphCardHit{
			{Kind: "entity", RefID: 7, Text: "实体卡文", Score: 0.77},
		},
		edgeHits: []bizknowledge.GraphCardHit{
			{Kind: "edge", RefID: 9, Text: "边卡文", Score: 0.66},
		},
	}
	hosts := &stubCardHostDocs{byRef: map[int64][]string{
		7: {"d1", "d2", "d3"},
		9: {"d4", "d5"},
	}}
	s := newTestCardSearcher(vector, &stubCardLexicalSearcher{}, hosts)
	got, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	}, []float32{0.1, 0.2})
	if err != nil {
		t.Fatal(err)
	}
	// entity ≤3 宿主 + edge ≤2 宿主。
	if len(got) != 5 {
		t.Fatalf("want 5 candidates (3 entity hosts + 2 edge hosts), got %d", len(got))
	}
	byID := make(map[string]biz.KnowledgeChunk, len(got))
	for _, ch := range got {
		byID[ch.ID] = ch
	}
	e := byID["g33e/7/d1"]
	if e.Content != "实体卡文" || e.EntityScore != 0.77 || e.Score != 0.77 {
		t.Fatalf("entity candidate malformed: %+v", e)
	}
	if e.DocID != "d1" || e.CollectionID != "col" {
		t.Fatalf("host projection fields wrong: %+v", e)
	}
	if e.MetadataJSON != `{"g33":"entity","g33_ref":7}` {
		t.Fatalf("marker = %q", e.MetadataJSON)
	}
	r := byID["g33r/9/d4"]
	if r.Content != "边卡文" || r.EntityScore != 0.66 || r.MetadataJSON != `{"g33":"edge","g33_ref":9}` {
		t.Fatalf("edge candidate malformed: %+v", r)
	}
	// 两条 ANN 以 kind 过滤各一次。
	if len(vector.kindSeen) != 2 || vector.kindSeen[0] != "entity" || vector.kindSeen[1] != "edge" {
		t.Fatalf("kind-filtered ANN calls = %v", vector.kindSeen)
	}
}

func TestGraphCardSearcher_LexicalFallbackEntityOnly(t *testing.T) {
	// vec 为空 → 词法降级：仅实体卡，edge 腿静默关闭（dense.SearchGraphCards
	// 不得被调用）。
	vector := &stubCardVectorSearcher{}
	lexical := &stubCardLexicalSearcher{hits: []bizknowledge.GraphCardHit{
		{Kind: "entity", RefID: 7, Text: "实体卡文", Score: 1.0},
	}}
	hosts := &stubCardHostDocs{byRef: map[int64][]string{7: {"d1"}}}
	s := newTestCardSearcher(vector, lexical, hosts)
	got, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "claude", TopK: 5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vector.kindSeen) != 0 {
		t.Fatalf("lexical path must not run ANN, seen %v", vector.kindSeen)
	}
	if len(got) != 1 || got[0].ID != "g33e/7/d1" {
		t.Fatalf("got %+v", got)
	}
}

func TestGraphCardSearcher_HostFailureDropsOnlyThatCard(t *testing.T) {
	vector := &stubCardVectorSearcher{
		entityHits: []bizknowledge.GraphCardHit{
			{Kind: "entity", RefID: 7, Text: "卡A", Score: 0.9},
			{Kind: "entity", RefID: 8, Text: "卡B", Score: 0.8},
		},
	}
	hosts := &stubCardHostDocs{
		byRef: map[int64][]string{8: {"d2"}},
		err:   map[int64]error{7: errors.New("pg hiccup")},
	}
	s := newTestCardSearcher(vector, &stubCardLexicalSearcher{}, hosts)
	got, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	}, []float32{0.1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "g33e/8/d2" {
		t.Fatalf("host failure must drop only that card, got %+v", got)
	}
}

func TestGraphCardSearcher_DenseLegErrorPropagates(t *testing.T) {
	vector := &stubCardVectorSearcher{err: errors.New("ann down")}
	s := newTestCardSearcher(vector, &stubCardLexicalSearcher{}, &stubCardHostDocs{})
	if _, err := s.SearchGraphCardChunks(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q",
	}, []float32{0.1}); err == nil {
		t.Fatal("ANN 失败应上抛（由 RRF 腿失败回退承接）")
	}
}

func TestGraphCardSearcher_NilPortsMeansOff(t *testing.T) {
	// nil=关闭（装配层断言三窄口缺失时关腿语义冻结）。
	if s := NewGraphCardSearcher(nil, &stubCardLexicalSearcher{}, &stubCardHostDocs{}, loggateway.NewNoop()); s != nil {
		t.Fatal("nil dense port must wire off")
	}
	if s := NewGraphCardSearcher(&stubCardVectorSearcher{}, nil, &stubCardHostDocs{}, loggateway.NewNoop()); s != nil {
		t.Fatal("nil lexical port must wire off")
	}
	if s := NewGraphCardSearcher(&stubCardVectorSearcher{}, &stubCardLexicalSearcher{}, nil, loggateway.NewNoop()); s != nil {
		t.Fatal("nil hosts port must wire off")
	}
}

// ===== 第三腿并发接线与失败回退（searchRRF） =====

type parallelCardSearcher struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *parallelCardSearcher) SearchGraphCardChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return []biz.KnowledgeChunk{{
		ID: "g33e/7/d3", DocID: "d3", CollectionID: "col", Content: "卡片文", Score: 0.77, EntityScore: 0.77,
		MetadataJSON: `{"g33":"entity","g33_ref":7}`,
	}}, nil
}

func TestHybridRetriever_RRFStartsCardLegConcurrently(t *testing.T) {
	repo := &parallelSearchRepo{
		denseStarted:  make(chan struct{}),
		sparseStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}
	cards := &parallelCardSearcher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	h.SetGraphCardSearcher(cards)

	done := make(chan error, 1)
	safego.Go(context.Background(), "test.knowledge.rrf_card_parallel", func() {
		got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
			CollectionID: "col", Query: "query", TopK: 8, // dense+sparse+card 三候选平权 1/61，须全部放行的口径
		}, HybridRRF)
		if err == nil {
			found := false
			for _, ch := range got {
				if ch.ID == "g33e/7/d3" && ch.EntityScore == 0.77 {
					found = true
				}
			}
			if !found {
				t.Errorf("card candidate missing after fusion")
			}
		}
		done <- err
	})

	// 三腿都须在任一对端释放前启动。
	select {
	case <-cards.started:
	case <-time.After(time.Second):
		close(repo.release)
		close(cards.release)
		t.Fatal("card leg did not start concurrently")
	}
	for name, started := range map[string]<-chan struct{}{
		"dense": repo.denseStarted, "sparse": repo.sparseStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(repo.release)
			close(cards.release)
			t.Fatalf("%s leg did not start concurrently", name)
		}
	}
	close(repo.release)
	close(cards.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type failingCardSearcher struct{ err error }

func (s *failingCardSearcher) SearchGraphCardChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	return nil, s.err
}

func TestHybridRetriever_RRFCardLegFailureFallsBackTwoLegs(t *testing.T) {
	repo := &stubSparseRepo{
		stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{{ID: "dense", DocID: "d1", Score: 0.9}}},
		bm25Chunks:        []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2", Score: 11}},
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	h.SetGraphCardSearcher(&failingCardSearcher{err: errors.New("card leg boom")})

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "query", TopK: 5,
	}, HybridRRF)
	if err != nil {
		t.Fatal(err)
	}
	// 腿失败 → 回退 dense+sparse 融合（warn 日志侧效应不钉）。
	if len(got) != 2 {
		t.Fatalf("want dense+sparse fallback (2 chunks), got %+v", got)
	}
	for _, ch := range got {
		if strings.HasPrefix(ch.ID, "g33") || ch.EntityScore != 0 {
			t.Fatalf("failed leg must leave no card residue: %+v", ch)
		}
	}
}

type legErrorRepo struct {
	stubSparseRepo
	denseErr, sparseErr error
}

func (s *legErrorRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, v []float32) ([]biz.KnowledgeChunk, error) {
	if s.denseErr != nil {
		return nil, s.denseErr
	}
	return s.stubSparseRepo.SearchChunks(ctx, q, v)
}

func (s *legErrorRepo) SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if s.sparseErr != nil {
		return nil, s.sparseErr
	}
	return s.stubSparseRepo.SearchChunksBM25(ctx, q)
}

type fixedCardSearcher struct{ chunks []biz.KnowledgeChunk }

func (s *fixedCardSearcher) SearchGraphCardChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	return s.chunks, nil
}

func cardProbeChunk() biz.KnowledgeChunk {
	return biz.KnowledgeChunk{
		ID: "g33e/7/d3", DocID: "d3", CollectionID: "col", Content: "卡片文",
		Score: 0.77, EntityScore: 0.77, MetadataJSON: `{"g33":"entity","g33_ref":7}`,
	}
}

func TestHybridRetriever_RRFCardLegSurvivesClassicLegFailure(t *testing.T) {
	// 对抗审查钉（fusion-retrieval/high）：dense 失败时健康卡腿候选必须进融合，
	// 不得被两腿时代的 tag 单路早退静默丢弃。
	repo := &legErrorRepo{
		stubSparseRepo: stubSparseRepo{
			stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{{ID: "dense", DocID: "d1", Score: 0.9}}},
			bm25Chunks:        []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2", Score: 11}},
		},
		denseErr: errors.New("pgvector 抖动"),
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	h.SetGraphCardSearcher(&fixedCardSearcher{chunks: []biz.KnowledgeChunk{cardProbeChunk()}})

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "query", TopK: 5,
	}, HybridRRF)
	if err != nil {
		t.Fatal(err)
	}
	foundCard, foundSparse := false, false
	for _, ch := range got {
		if ch.ID == "g33e/7/d3" && ch.EntityScore == 0.77 {
			foundCard = true
		}
		if ch.ID == "sparse" {
			foundSparse = true
		}
		if ch.ID == "dense" {
			t.Fatalf("failed dense leg must leave no residue: %+v", ch)
		}
	}
	if !foundCard || !foundSparse {
		t.Fatalf("want sparse+card 剩余腿融合, got %+v", got)
	}
}

func TestHybridRetriever_RRFCardOnlyWhenBothClassicFail(t *testing.T) {
	// 对抗审查钉（fusion-retrieval/high）：dense+sparse 双挂而卡腿健康时回退
	// 卡腿单路 RRF，不得坍缩为 Internal。
	repo := &legErrorRepo{
		stubSparseRepo: stubSparseRepo{
			stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{{ID: "dense", DocID: "d1", Score: 0.9}}},
			bm25Chunks:        []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2", Score: 11}},
		},
		denseErr:  errors.New("pgvector 抖动"),
		sparseErr: errors.New("trgm 索引损坏"),
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	h.SetGraphCardSearcher(&fixedCardSearcher{chunks: []biz.KnowledgeChunk{cardProbeChunk()}})

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "query", TopK: 5,
	}, HybridRRF)
	if err != nil {
		t.Fatalf("card 腿健康时双挂不得硬错: %v", err)
	}
	if len(got) != 1 || got[0].ID != "g33e/7/d3" || got[0].EntityScore != 0.77 {
		t.Fatalf("want 卡腿单路, got %+v", got)
	}
}

func TestRerankChunks_PreservesCardMarkerAndEntityScore(t *testing.T) {
	// G-33 尾段保真：rerank 后卡候选的 metadata g33 标记与 EntityScore 不丢
	// （配合 mergeScoreBreakdown 并回纪律）。
	repo := &stubSparseRepo{}
	rr := &reverseReranker{}
	ret := NewRetriever(stubEmbedder{}, repo, rr, loggateway.NewNoop())

	candidates := []biz.KnowledgeChunk{
		{ID: "g33e/7/d1", DocID: "d1", CollectionID: "col", Content: "卡文", Score: 0.20, RRFScore: 0.20, EntityScore: 0.77, MetadataJSON: `{"g33":"entity","g33_ref":7}`},
		{ID: "x", DocID: "d2", CollectionID: "col", Content: "c2", Score: 0.10, SparseScore: 9.5},
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
	card := byID["g33e/7/d1"]
	if card.EntityScore != 0.77 || card.MetadataJSON != `{"g33":"entity","g33_ref":7}` {
		t.Fatalf("rerank dropped card breakdown: %+v", card)
	}
}
