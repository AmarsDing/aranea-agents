package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ===== G-05-asof（ADR-KN-20261020-J）：时态检索对运营热度零读写副作用 =====
//
// as-of 是审计/回放语义：本次「回到 T/R」的检索不得成为当下兴趣证据——
// 不写 access_log、不触发 Hebbian 共激活（历史快照强化当下边=让时光机污染
// 现在时）；base-level 读取加成同属运营信号，对纯时态排序一并关闭。

type countingCoActivationRepo struct {
	mu    sync.Mutex
	calls int
}

func (s *countingCoActivationRepo) StrengthenCoActivations(_ context.Context, _ string, _ []string, _ float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil
}

func (s *countingCoActivationRepo) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestAdaptiveRouter_AsOfSuppressesHeatSideEffects(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.9},
		{ID: "c2", DocID: "d2", CollectionID: "col", Score: 0.8},
		{ID: "c3", DocID: "d3", CollectionID: "col", Score: 0.7},
	}
	access := &stubAccessLogRepo{scores: map[string]float64{"d2": 5.0}}
	coact := &countingCoActivationRepo{}
	router := newBoostTestRouter(t, chunks, access, 0.1)
	router.SetCoActivation(coact, 0.1)

	out, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5,
			Temporal: bizknowledge.TemporalSpan{ValidAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}},
		nil, HybridDense)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("chunks = %d, want 3 (as-of 不丢检索结果，仅关热度副作用)", len(out))
	}
	// 零写：access_log 不得记录本次回放。
	if got := len(access.loggedEntries()); got != 0 {
		t.Fatalf("as-of must not log access: logged %d entries", got)
	}
	// 零读：base-level 运营加成不得参与纯时态排序（c1 原分保持榜首=反证）。
	if access.scoreQ != nil {
		t.Fatalf("base-level boost must be skipped under as-of: queried %v", access.scoreQ)
	}
	if out[0].ID != "c1" {
		t.Fatalf("order = [%s ...], want c1 first（boost 全关则原分为序）", out[0].ID)
	}
	// 零强化：Hebbian 异步写不得发起（150ms 竞态窗口足够落定反证）。
	time.Sleep(150 * time.Millisecond)
	if got := coact.callCount(); got != 0 {
		t.Fatalf("as-of must not trigger Hebbian: %d calls", got)
	}
}

func TestAdaptiveRouter_NoSpanKeepsHeatSideEffects(t *testing.T) {
	// 反向钉：span 零值 → 既有热度链路一字不改（行为回归护栏）。
	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "col", Score: 0.9},
		{ID: "c2", DocID: "d2", CollectionID: "col", Score: 0.8},
	}
	access := &stubAccessLogRepo{scores: map[string]float64{"d2": 5.0}}
	coact := &countingCoActivationRepo{}
	router := newBoostTestRouter(t, chunks, access, 0.1)
	router.SetCoActivation(coact, 0.1)

	out, err := router.Search(context.Background(),
		biz.KnowledgeSearchQuery{CollectionID: "col", Query: "q", TopK: 5}, nil, HybridDense)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if out[0].ID != "c2" {
		t.Fatalf("boost must stay active without span: first = %s", out[0].ID)
	}
	if len(access.loggedEntries()) != 2 {
		t.Fatalf("access_log must still record without span: %d entries", len(access.loggedEntries()))
	}
	time.Sleep(150 * time.Millisecond)
	if got := coact.callCount(); got != 1 {
		t.Fatalf("Hebbian must still fire without span: %d calls", got)
	}
}
