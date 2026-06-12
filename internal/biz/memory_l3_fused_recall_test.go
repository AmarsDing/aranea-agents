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

type recallStoreMockWithQuery struct {
	lastMinScore float64
}

func (s *recallStoreMockWithQuery) RecallL3Facts(_ context.Context, _, _, _, _ string, _ []float32, _ int32, minScore float64) ([][]byte, error) {
	s.lastMinScore = minScore
	return nil, nil
}
