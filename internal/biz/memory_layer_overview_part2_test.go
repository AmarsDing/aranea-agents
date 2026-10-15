package biz

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"testing"
	"time"
)

func TestListEpisodesAdmin_RequiresAgentID(t *testing.T) {
	uc := NewMemoryAdminUsecase(&fakeCenterAdminDeps{}, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(&fakeL2AdminReader{}, &fakeL4RelReader{})
	if _, _, err := uc.ListEpisodesAdmin(context.Background(), "", "", 20, 0); err == nil {
		t.Fatal("expected error for empty agent id")
	}
}

func TestListEpisodesAdmin_ReaderNotWired(t *testing.T) {
	uc := NewMemoryAdminUsecase(&fakeCenterAdminDeps{}, nil, nil, nil, loggateway.NewNoop())
	if _, _, err := uc.ListEpisodesAdmin(context.Background(), "agent-1", "", 20, 0); err == nil {
		t.Fatal("expected error when l2 reader not wired")
	}
}

func TestListEpisodesAdmin_CapsLimit(t *testing.T) {
	l2 := &fakeL2AdminReader{rows: [][]byte{}, total: 0}
	uc := NewMemoryAdminUsecase(&fakeCenterAdminDeps{}, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(l2, &fakeL4RelReader{})

	if _, _, err := uc.ListEpisodesAdmin(context.Background(), "agent-1", "", 500, -3); err != nil {
		t.Fatalf("ListEpisodesAdmin with out-of-range limit/offset: %v", err)
	}
}

type overviewAggStub struct {
	fakeCenterAdminDeps
	agg         FactOverviewAgg
	factFeed    [][]byte
	injectFeed  [][]byte
	entityToday int32
	aggCalls    int
}

func (s *overviewAggStub) AggregateFactOverview(context.Context, string, string) (FactOverviewAgg, error) {
	s.aggCalls++
	return s.agg, nil
}

func (s *overviewAggStub) ListRecentFactActivityRows(context.Context, string, int32) ([][]byte, error) {
	return s.factFeed, nil
}

func (s *overviewAggStub) ListRecentlyInjectedFactRows(context.Context, string, int32) ([][]byte, error) {
	return s.injectFeed, nil
}

func (s *overviewAggStub) CountEntitiesCreatedSince(context.Context, string, string, string) (int32, error) {
	return s.entityToday, nil
}

func TestLayerOverview_UsesSQLAggWhenAvailable(t *testing.T) {
	deps := &overviewAggStub{
		fakeCenterAdminDeps: fakeCenterAdminDeps{
			factRows:    [][]byte{centerRowJSON(map[string]any{"id": "stale", "use_count": 1, "created_at": time.Now().UTC().Format(time.RFC3339Nano)})},
			factActive:  1,
			entityRows:  [][]byte{},
			entityTotal: 80,
		},
		agg:         FactOverviewAgg{ActiveCount: 999, TodayAdded: 12, RecallHits: 777},
		entityToday: 9,
		factFeed: [][]byte{
			centerRowJSON(map[string]any{"id": "new", "statement": "今日抽取", "created_at": time.Now().UTC().Format(time.RFC3339Nano)}),
		},
		injectFeed: [][]byte{
			centerRowJSON(map[string]any{
				"id": "inj", "statement": "注入旧事实", "injected_count": 3,
				"last_used_at": time.Now().UTC().Format(time.RFC3339Nano),
			}),
		},
	}
	uc := NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(&fakeL2AdminReader{}, &fakeL4RelReader{})

	ov, err := uc.GetLayerOverview(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetLayerOverview: %v", err)
	}
	if deps.factCallCount != 0 {
		t.Fatalf("ListFactRows called %d times; SQL agg path must not scan facts", deps.factCallCount)
	}
	if deps.aggCalls != 1 {
		t.Fatalf("AggregateFactOverview calls = %d, want 1", deps.aggCalls)
	}
	l3 := findLayer(ov.Layers, "L3")
	if l3 == nil {
		t.Fatal("L3 layer missing")
	}
	if l3.ItemCount != 999 || l3.TodayAdded != 12 || l3.RecallHits != 777 {
		t.Errorf("L3 from agg: item=%d today=%d hits=%d, want 999/12/777", l3.ItemCount, l3.TodayAdded, l3.RecallHits)
	}
	l4s := findLayer(ov.Layers, "L4")
	if l4s == nil {
		t.Fatal("L4 layer missing")
	}
	if l4s.ItemCount != 80 || l4s.TodayAdded != 9 {
		t.Errorf("L4 from agg: item=%d today=%d, want 80/9", l4s.ItemCount, l4s.TodayAdded)
	}
	var sawExtracted, sawInjected bool
	for _, it := range ov.ActivityFeed {
		if it.Kind == "fact_extracted" && it.Summary == "今日抽取" {
			sawExtracted = true
		}
		if it.Kind == "fact_injected" && it.Summary == "注入旧事实" {
			sawInjected = true
		}
	}
	if !sawExtracted || !sawInjected {
		t.Errorf("activity feed missing dedicated rows: extracted=%v injected=%v feed=%+v", sawExtracted, sawInjected, ov.ActivityFeed)
	}
}
