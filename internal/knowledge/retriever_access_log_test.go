package knowledge

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ── P2-c：access_log 记账下沉 Retriever ─────────────────────────────────────
// 契约：SetAccessLog 接线后 Search 成功且命中非空即记录（doc 去重、同查询同指纹）；
// 未接线/空命中不记；记录失败仅 Warn 不阻塞检索返回。只记不加成（排序分不变）。

var accessLogTestChunks = []biz.KnowledgeChunk{
	{ID: "c1", DocID: "d1", CollectionID: "col", Content: "alpha", Score: 0.9},
	{ID: "c2", DocID: "d1", CollectionID: "col", Content: "alpha-2", Score: 0.8},
	{ID: "c3", DocID: "d2", CollectionID: "col", Content: "beta", Score: 0.7},
}

func TestRetriever_AccessLog_RecordsHitsDedupByDoc(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: accessLogTestChunks}
	access := &stubAccessLogRepo{}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	ret.SetAccessLog(access)

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("chunks = %d, want 3", len(out))
	}
	// d1 两 chunk 只记一次 → 2 条。
	if !waitForAccessLog(access, 2) {
		t.Fatalf("logged = %+v, want 2 entries (dedup by doc)", access.loggedEntries())
	}
	logged := access.loggedEntries()
	wantHash := accessQueryHash("q")
	seen := map[string]bool{}
	for _, e := range logged {
		seen[e.DocID] = true
		if e.CollectionID != "col" {
			t.Fatalf("collection = %q, want col", e.CollectionID)
		}
		if e.QueryHash != wantHash {
			t.Fatalf("query_hash = %q, want %q（与 Router 同指纹）", e.QueryHash, wantHash)
		}
	}
	if !seen["d1"] || !seen["d2"] {
		t.Fatalf("logged docs = %v, want d1+d2", seen)
	}
}

func TestRetriever_AccessLog_NotWired_NoLog(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: accessLogTestChunks}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	if _, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRetriever_AccessLog_EmptyHits_NoLog(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: nil}
	access := &stubAccessLogRepo{}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	ret.SetAccessLog(access)
	if _, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if logged := access.loggedEntries(); len(logged) != 0 {
		t.Fatalf("empty hits must not log, got %+v", logged)
	}
}

type errAccessLogRepo struct{ bizknowledge.AccessLogRepo }

func (errAccessLogRepo) LogAccess(context.Context, []bizknowledge.AccessLogEntry) error {
	return errors.New("db down")
}

func TestRetriever_AccessLog_LogFailureDoesNotBlock(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: accessLogTestChunks}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	ret.SetAccessLog(errAccessLogRepo{})
	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "q", TopK: 5,
	})
	if err != nil {
		t.Fatalf("log failure must not block search: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("chunks = %d, want 3", len(out))
	}
}

type blockingAccessLogRepo struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingAccessLogRepo) BaseLevelScores(context.Context, string, []string) (map[string]float64, error) {
	return nil, nil
}

func (r *blockingAccessLogRepo) LogAccess(context.Context, []bizknowledge.AccessLogEntry) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return nil
}

func TestRetriever_AccessLog_DoesNotAddDatabaseLatency(t *testing.T) {
	repo := &stubKnowledgeRepo{chunks: accessLogTestChunks}
	access := &blockingAccessLogRepo{started: make(chan struct{}), release: make(chan struct{})}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	ret.SetAccessLog(access)

	done := make(chan error, 1)
	safego.Go(context.Background(), "test.knowledge.access_log_async", func() {
		_, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
			CollectionID: "col", Query: "q", TopK: 5,
		})
		done <- err
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(access.release)
		t.Fatal("search waited for access-log persistence")
	}
	select {
	case <-access.started:
	case <-time.After(time.Second):
		close(access.release)
		t.Fatal("access log was not scheduled")
	}
	close(access.release)
}

func waitForAccessLog(repo *stubAccessLogRepo, want int) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(repo.loggedEntries()) == want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
