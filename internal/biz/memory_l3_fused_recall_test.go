package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type scoredStoreMock struct {
	hits []RecallHit
}

func (m *scoredStoreMock) RecallL3Hits(_ context.Context, scopeType, scopeID, userID, query string, _ []float32, limit int32) ([]RecallHit, error) {
	_ = scopeType
	_ = scopeID
	_ = userID
	_ = query
	_ = limit
	return append([]RecallHit(nil), m.hits...), nil
}

type recallStoreMock struct{}

func (recallStoreMock) RecallL3Facts(_ context.Context, _, _, _, _ string, _ []float32, _ int32, _ float64) ([][]byte, error) {
	return nil, nil
}

func TestRecallFactsFused_SortsAndDedupes(t *testing.T) {
	uc := NewMemoryL3RecallUsecase(recallStoreMock{}, &scoredStoreMock{hits: []RecallHit{
		{ID: "a", Statement: "low", Raw: []byte(`{"id":"a","statement":"low"}`), Scores: RecallScoreBreakdown{Total: 0.4}},
		{ID: "b", Statement: "high", Raw: []byte(`{"id":"b","statement":"high"}`), Scores: RecallScoreBreakdown{Total: 0.9}},
		{ID: "b", Statement: "high dup", Raw: []byte(`{"id":"b","statement":"high dup"}`), Scores: RecallScoreBreakdown{Total: 0.8}},
	}}, nil, loggateway.NewNoop())
	rows, err := uc.RecallFactsFused(context.Background(), L3FusedRecallQuery{
		Runtime: MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"},
		Scopes:  []string{"agent"},
		Limit:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 deduped rows, got %d", len(rows))
	}
}

func TestRecallFacts_SkipsMinScoreWhenQueryEmpty(t *testing.T) {
	store := &recallStoreMockWithQuery{}
	uc := NewMemoryL3RecallUsecase(store, nil, nil, loggateway.NewNoop())
	_, err := uc.RecallFacts(context.Background(), L3RecallQuery{
		ScopeType: "agent",
		ScopeID:   "ag1",
		Query:     "",
		MinScore:  0.55,
		Limit:     3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.lastMinScore != 0 {
		t.Fatalf("expected min score 0 for empty query, got %v", store.lastMinScore)
	}
}

// P1-2a（2026-08-21）：自适应阈值 = max(静态 minScore, top1×0.6)，仅查询路径。
func TestRecallFactsFused_AdaptiveMinScore(t *testing.T) {
	mk := func(id, stmt string, total float64) RecallHit {
		return RecallHit{ID: id, Statement: stmt,
			Raw:    []byte(`{"id":"` + id + `","statement":"` + stmt + `"}`),
			Scores: RecallScoreBreakdown{Total: total}}
	}
	q := L3FusedRecallQuery{
		Runtime:       MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"},
		Scopes:        []string{"agent"},
		Query:         "空调温度设定",
		Limit:         10,
		MinScoreQuery: 0.35,
	}

	t.Run("top1 强时抬高阈值截掉弱长尾", func(t *testing.T) {
		uc := NewMemoryL3RecallUsecase(recallStoreMock{}, &scoredStoreMock{hits: []RecallHit{
			mk("top", "空调设定为 24℃", 0.9),
			mk("mid", "空调维护记录", 0.6),       // > 0.54 = 0.9×0.6，保留
			mk("tail", "空调 28℃ 告警规则", 0.5), // > 0.35 但 < 0.54，被自适应阈值截掉
		}}, nil, loggateway.NewNoop())
		rows, err := uc.RecallFactsFused(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows after adaptive filter, got %d", len(rows))
		}
	})

	t.Run("top1 偏弱时退回静态下限", func(t *testing.T) {
		uc := NewMemoryL3RecallUsecase(recallStoreMock{}, &scoredStoreMock{hits: []RecallHit{
			mk("top", "弱相关事实", 0.5),
			mk("tail", "刚过静态线", 0.36), // top1×0.6=0.3 < 0.35 → 静态 0.35 生效，保留
		}}, nil, loggateway.NewNoop())
		rows, err := uc.RecallFactsFused(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows under static floor, got %d", len(rows))
		}
	})

	t.Run("空查询（被动召回）不启用自适应", func(t *testing.T) {
		uc := NewMemoryL3RecallUsecase(recallStoreMock{}, &scoredStoreMock{hits: []RecallHit{
			mk("top", "高分事实", 0.9),
			mk("tail", "低分事实", 0.2), // 被动路径 minScore=0 → 全保留
		}}, nil, loggateway.NewNoop())
		passive := q
		passive.Query = ""
		passive.MinScoreQuery = 0
		passive.MinScorePassive = 0
		rows, err := uc.RecallFactsFused(context.Background(), passive)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows on passive path, got %d", len(rows))
		}
	})
}

func TestAdaptiveRecallMinScore(t *testing.T) {
	if got := AdaptiveRecallMinScore(0, 0.9); got != 0 {
		t.Fatalf("disabled configured: got %v, want 0", got)
	}
	if got := AdaptiveRecallMinScore(-1, 0.9); got != 0 {
		t.Fatalf("negative configured: got %v, want 0", got)
	}
	if got := AdaptiveRecallMinScore(0.55, 0.9); got != 0.55 {
		t.Fatalf("top1*0.6 below floor: got %v, want 0.55", got)
	}
	if got := AdaptiveRecallMinScore(0.35, 0.9); got < 0.539 || got > 0.541 {
		t.Fatalf("top1 strong: got %v, want 0.54", got)
	}
	if got := AdaptiveRecallMinScore(0.35, 0); got != 0.35 {
		t.Fatalf("zero top1: got %v, want configured floor", got)
	}
}

type recallStoreMockWithQuery struct {
	lastMinScore float64
}

func (s *recallStoreMockWithQuery) RecallL3Facts(_ context.Context, _, _, _, _ string, _ []float32, _ int32, minScore float64) ([][]byte, error) {
	s.lastMinScore = minScore
	return nil, nil
}
