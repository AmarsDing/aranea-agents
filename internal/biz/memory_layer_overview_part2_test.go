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

// 不报错即视为通过：limit 超限被收敛而非拒绝（与 unified graph hops 上限同策略）。
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
	l0Agg       L0OverviewAgg
	l1Agg       L1OverviewAgg
	factFeed    [][]byte
	injectFeed  [][]byte
	entityToday int32
	aggCalls    int
	l0AggCalls  int
	l1AggCalls  int
}

func (s *overviewAggStub) AggregateFactOverview(context.Context, string, string) (FactOverviewAgg, error) {
	s.aggCalls++
	return s.agg, nil
}

// AggregateL0Overview（P2-1）与 AggregateL1Overview（P2-2）：全景卡 SQL 聚
// 合口的桩实现——生产由 data 层 shim 单条 SQL 完成。
func (s *overviewAggStub) AggregateL0Overview(context.Context, string, string, string) (L0OverviewAgg, error) {
	s.l0AggCalls++
	return s.l0Agg, nil
}

func (s *overviewAggStub) AggregateL1Overview(context.Context, string, string, string) (L1OverviewAgg, error) {
	s.l1AggCalls++
	return s.l1Agg, nil
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

// P2-1/P2-2（2026-10-18 审计）：stats reader 可用时 L0/L1 全景卡必须走 SQL
// 聚合——L0 不再以 LIMIT 100 行扫描计数（真实总量可超 100），headline 仅回
// 表取最新一行；L1 不再逐任务 ListL1FieldRows 的 N+1，field_count 一次聚合。
func TestLayerOverview_L0L1UseSQLAggWhenAvailable(t *testing.T) {
	now := time.Now().UTC()
	today := now.Format(time.RFC3339Nano)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)

	deps := &overviewAggStub{
		fakeCenterAdminDeps: fakeCenterAdminDeps{
			// 行扫描口径只能数到 2/1；聚合值刻意取 250/7 以区分路径。
			l0Rows: [][]byte{
				centerRowJSON(map[string]any{"id": "latest", "used_ratio": 0.5, "warning_codes_json": "[]", "created_at": today}),
				centerRowJSON(map[string]any{"id": "stale", "created_at": old}),
			},
			// 行扫描口径 1 任务/1 字段；聚合值 4/2/11 以区分路径。
			l1Tasks: [][]byte{
				centerRowJSON(map[string]any{"id": "t1", "created_at": today}),
			},
			l1Fields: map[string][][]byte{
				"t1": {centerRowJSON(map[string]any{"id": "f1"})},
			},
			entityRows: [][]byte{},
		},
		l0Agg: L0OverviewAgg{ItemCount: 250, TodayAdded: 7},
		l1Agg: L1OverviewAgg{ItemCount: 4, TodayAdded: 2, FieldCount: 11},
	}
	uc := NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(&fakeL2AdminReader{}, &fakeL4RelReader{})

	ov, err := uc.GetLayerOverview(context.Background(), "agent-1", "sess-1")
	if err != nil {
		t.Fatalf("GetLayerOverview: %v", err)
	}

	if deps.l0AggCalls != 1 {
		t.Fatalf("AggregateL0Overview calls = %d, want 1", deps.l0AggCalls)
	}
	if deps.l1AggCalls != 1 {
		t.Fatalf("AggregateL1Overview calls = %d, want 1", deps.l1AggCalls)
	}
	if deps.fieldCalls != 0 {
		t.Fatalf("ListL1FieldRows calls = %d, want 0 (N+1 must be eliminated)", deps.fieldCalls)
	}
	if deps.l0ListLimit != 1 {
		t.Fatalf("L0 headline fetch limit = %d, want 1 (latest row only)", deps.l0ListLimit)
	}

	l0 := findLayer(ov.Layers, "L0")
	if l0 == nil {
		t.Fatal("L0 layer missing")
	}
	if l0.ItemCount != 250 || l0.TodayAdded != 7 {
		t.Errorf("L0 from agg: item=%d today=%d, want 250/7", l0.ItemCount, l0.TodayAdded)
	}
	// headline 仍由最新快照行渲染（used_ratio 0.5 → 50%）。
	h0 := parseHeadline(t, l0.HeadlineJSON)
	if h0["context_usage_pct"] != float64(50) {
		t.Errorf("L0 context_usage_pct: got %v, want 50 (from latest row)", h0["context_usage_pct"])
	}

	l1 := findLayer(ov.Layers, "L1")
	if l1 == nil {
		t.Fatal("L1 layer missing")
	}
	if l1.ItemCount != 4 || l1.TodayAdded != 2 {
		t.Errorf("L1 from agg: item=%d today=%d, want 4/2", l1.ItemCount, l1.TodayAdded)
	}
	h1 := parseHeadline(t, l1.HeadlineJSON)
	if h1["active_tasks"] != float64(4) || h1["field_count"] != float64(11) {
		t.Errorf("L1 headline: got %v, want active_tasks=4 field_count=11", h1)
	}
}
