package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// 自治理知识图谱 M1-2：base-level 激活分注入检索打分。
// finalScore = 检索分 + beta * baseLevel(docID)；返回后记录 access_log。

type stubAccessLogRepo struct {
	mu       sync.Mutex
	scores   map[string]float64
	logged   []bizknowledge.AccessLogEntry
	scoreQ   []string // 收到的 docIDs
	scoreCol string
}

func (s *stubAccessLogRepo) LogAccess(_ context.Context, entries []bizknowledge.AccessLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logged = append(s.logged, entries...)
	return nil
}

func (s *stubAccessLogRepo) loggedEntries() []bizknowledge.AccessLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bizknowledge.AccessLogEntry(nil), s.logged...)
}

func (s *stubAccessLogRepo) BaseLevelScores(_ context.Context, collectionID string, docIDs []string) (map[string]float64, error) {
	s.scoreCol = collectionID
	s.scoreQ = docIDs
	return s.scores, nil
}

func newBoostTestRouter(t *testing.T, chunks []biz.KnowledgeChunk, access *stubAccessLogRepo, beta float64) *AdaptiveRouter {
	t.Helper()
	repo := &stubKnowledgeRepo{chunks: chunks}
	ret := NewRetriever(stubAllEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, nil, loggateway.NewNoop())
	router := NewAdaptiveRouter(h, nil, loggateway.NewNoop())
	if access != nil {
		router.SetAccessLog(access, beta)
	}
	return router
}

func TestAdaptiveRouter_BaseLevelBoost_Reorders(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Content: "top", Score: 0.9},
		{ID: "c2", DocID: "d2", CollectionID: "col", Content: "runner-up", Score: 0.8},
	}
	access := &stubAccessLogRepo{scores: map[string]float64{"d2": 5.0}}
	router := newBoostTestRouter(t, chunks, access, 0.1)

	out, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}, nil, HybridDense)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("chunks = %d, want 2", len(out))
	}
	// d2 base-level 5.0 × beta 0.1 = +0.5 → 0.8+0.5=1.3 反超 c1 的 0.9。
	if out[0].ID != "c2" {
		t.Fatalf("order = [%s %s], want c2 first (base-level boost)", out[0].ID, out[1].ID)
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("scores not desc after boost: %v >= %v expected", out[0].Score, out[1].Score)
	}

	// 激活分查询按返回文档集合发起。
	if access.scoreCol != "col" {
		t.Errorf("BaseLevelScores collection = %q, want col", access.scoreCol)
	}
	if len(access.scoreQ) != 2 {
		t.Errorf("BaseLevelScores docIDs = %v, want 2 docs", access.scoreQ)
	}

	// 返回后记录命中：两文档各一行，QueryHash 非空且同批一致。
	if len(access.logged) != 2 {
		t.Fatalf("logged = %d, want 2", len(access.logged))
	}
	seen := map[string]bool{}
	for _, e := range access.logged {
		seen[e.DocID] = true
		if e.CollectionID != "col" {
			t.Errorf("entry collection = %q, want col", e.CollectionID)
		}
		if e.QueryHash == "" {
			t.Error("entry QueryHash empty")
		}
	}
	if !seen["d1"] || !seen["d2"] {
		t.Errorf("logged docs = %v, want d1+d2", seen)
	}
	if access.logged[0].QueryHash != access.logged[1].QueryHash {
		t.Error("same-batch entries must share QueryHash (Hebbian 分组键)")
	}
}

func TestAdaptiveRouter_NoAccessLog_Passthrough(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.9},
		{ID: "c2", DocID: "d2", CollectionID: "col", Score: 0.8},
	}
	router := newBoostTestRouter(t, chunks, nil, 0)
	out, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}, nil, HybridDense)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out) != 2 || out[0].ID != "c1" {
		t.Fatalf("passthrough broken: %+v", out)
	}
}

// ── M1-3 Hebbian 共激活边：同批召回文档两两强化（异步，不阻塞返回）──────────

type stubCoActivationRepo struct {
	done chan struct{}
	col  string
	docs []string
	eta  float64
}

func (s *stubCoActivationRepo) StrengthenCoActivations(_ context.Context, collectionID string, docIDs []string, eta float64) error {
	s.col = collectionID
	s.docs = append([]string(nil), docIDs...)
	s.eta = eta
	close(s.done)
	return nil
}

func TestAdaptiveRouter_HebbianCoActivation(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.9},
		{ID: "c2", DocID: "d2", CollectionID: "col", Score: 0.8},
		{ID: "c3", DocID: "d3", CollectionID: "col", Score: 0.7},
	}
	coact := &stubCoActivationRepo{done: make(chan struct{})}
	router := newBoostTestRouter(t, chunks, nil, 0)
	router.SetCoActivation(coact, 0.1)

	if _, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}, nil, HybridDense); err != nil {
		t.Fatalf("search: %v", err)
	}
	select {
	case <-coact.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Hebbian 共激活未在超时内触发（异步写入丢失）")
	}
	if coact.col != "col" {
		t.Errorf("collection = %q, want col", coact.col)
	}
	if len(coact.docs) != 3 {
		t.Errorf("docIDs = %v, want 3 docs", coact.docs)
	}
	if coact.eta != 0.1 {
		t.Errorf("eta = %v, want 0.1", coact.eta)
	}
}
