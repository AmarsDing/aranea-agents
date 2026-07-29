package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── fakes ─────────────────────────────────────────────────────────────

type fakeCenterAdminDeps struct {
	MemoryAdminDeps // nil-embedded; only overridden methods are callable

	l0Rows   [][]byte
	l1Tasks  [][]byte
	l1Fields map[string][][]byte

	factRows   [][]byte
	factTotal  int32
	factActive int32

	conflictTotal int32

	entityRows  [][]byte
	entityTotal int32

	evoRows [][]byte
}

func (f *fakeCenterAdminDeps) ListL0SnapshotRows(ctx context.Context, sessionID, agentID string, limit int32) ([][]byte, error) {
	return f.l0Rows, nil
}

func (f *fakeCenterAdminDeps) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	return f.l1Tasks, nil
}

func (f *fakeCenterAdminDeps) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool, requestingAgentID ...string) ([][]byte, error) {
	return f.l1Fields[taskID], nil
}

func (f *fakeCenterAdminDeps) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return f.factRows, f.factTotal, f.factActive, 0, nil
}

func (f *fakeCenterAdminDeps) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	return nil, f.conflictTotal, nil
}

func (f *fakeCenterAdminDeps) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	return f.entityRows, f.entityTotal, nil
}

func (f *fakeCenterAdminDeps) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	return f.evoRows, nil
}

type fakeL2AdminReader struct {
	rows  [][]byte
	total int32
	today int32
	byIDs map[string][]byte
}

func (f *fakeL2AdminReader) ListEpisodeRowsAdmin(ctx context.Context, agentID, sessionID string, limit, offset int32) ([][]byte, int32, int32, error) {
	return f.rows, f.total, f.today, nil
}

func (f *fakeL2AdminReader) ListEpisodeRowsByIDs(ctx context.Context, ids []string) ([][]byte, error) {
	var out [][]byte
	for _, id := range ids {
		if r, ok := f.byIDs[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeL4RelReader struct {
	count int32
	rows  [][]byte
	topID string
}

func (f *fakeL4RelReader) CountActiveRelations(ctx context.Context, scopeType, scopeID string) (int32, error) {
	return f.count, nil
}

func (f *fakeL4RelReader) ListActiveRelationRows(ctx context.Context, scopeType, scopeID string) ([][]byte, error) {
	return f.rows, nil
}

func (f *fakeL4RelReader) TopConnectedEntityID(ctx context.Context, scopeType, scopeID string) (string, error) {
	return f.topID, nil
}

// ── helpers ───────────────────────────────────────────────────────────

func centerRowJSON(v map[string]any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func parseHeadline(t *testing.T, raw string) map[string]any {
	t.Helper()
	m := map[string]any{}
	if raw == "" {
		return m
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("headline_json not valid JSON: %v (%q)", err, raw)
	}
	return m
}

func findLayer(layers []MemoryLayerStat, id string) *MemoryLayerStat {
	for i := range layers {
		if layers[i].Layer == id {
			return &layers[i]
		}
	}
	return nil
}

func findActionItem(items []MemoryActionItem, kind string) *MemoryActionItem {
	for i := range items {
		if items[i].Kind == kind {
			return &items[i]
		}
	}
	return nil
}

// ── tests ─────────────────────────────────────────────────────────────

func TestLayerOverview_FiveLayerAssembly(t *testing.T) {
	now := time.Now().UTC()
	today := now.Format(time.RFC3339Nano)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)

	deps := &fakeCenterAdminDeps{
		l0Rows: [][]byte{
			centerRowJSON(map[string]any{"id": "s1", "used_ratio": 0.68, "warning_codes_json": `["near_limit"]`, "created_at": today}),
			centerRowJSON(map[string]any{"id": "s2", "used_ratio": 0.4, "warning_codes_json": "[]", "created_at": old}),
		},
		l1Tasks: [][]byte{
			centerRowJSON(map[string]any{"id": "t1", "status": "active", "created_at": today}),
			centerRowJSON(map[string]any{"id": "t2", "status": "active", "created_at": old}),
		},
		l1Fields: map[string][][]byte{
			"t1": {centerRowJSON(map[string]any{"id": "f1"}), centerRowJSON(map[string]any{"id": "f2"}), centerRowJSON(map[string]any{"id": "f3"})},
			"t2": {centerRowJSON(map[string]any{"id": "f4"})},
		},
		factRows: [][]byte{
			centerRowJSON(map[string]any{"id": "fact1", "statement": "偏好简洁回复", "hit_count": 5, "created_at": today}),
			centerRowJSON(map[string]any{"id": "fact2", "statement": "住在杭州", "hit_count": 6, "created_at": old}),
		},
		factTotal:     2,
		factActive:    2,
		conflictTotal: 2,
		entityRows: [][]byte{
			centerRowJSON(map[string]any{"id": "e1", "name": "用户画像", "created_at": today}),
		},
		entityTotal: 1,
		evoRows:     [][]byte{centerRowJSON(map[string]any{"id": "p1"})},
	}
	l2 := &fakeL2AdminReader{total: 45, today: 6}
	l4 := &fakeL4RelReader{count: 143}

	uc := NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(l2, l4)

	ov, err := uc.GetLayerOverview(context.Background(), "agent-1", "sess-1")
	if err != nil {
		t.Fatalf("GetLayerOverview: %v", err)
	}
	if ov == nil {
		t.Fatal("overview is nil")
	}
	if len(ov.Layers) != 5 {
		t.Fatalf("layers: got %d, want 5", len(ov.Layers))
	}

	// L0
	l0 := findLayer(ov.Layers, "L0")
	if l0 == nil {
		t.Fatal("L0 layer missing")
	}
	if l0.ItemCount != 2 || l0.TodayAdded != 1 {
		t.Errorf("L0 counts: got item=%d today=%d, want 2/1", l0.ItemCount, l0.TodayAdded)
	}
	if l0.Health != "warn" {
		t.Errorf("L0 health: got %q, want warn (near_limit warning code)", l0.Health)
	}
	h0 := parseHeadline(t, l0.HeadlineJSON)
	if h0["context_usage_pct"] != float64(68) {
		t.Errorf("L0 context_usage_pct: got %v, want 68", h0["context_usage_pct"])
	}
	if h0["compress_status"] != "warning" {
		t.Errorf("L0 compress_status: got %v, want warning", h0["compress_status"])
	}

	// L1
	l1 := findLayer(ov.Layers, "L1")
	if l1 == nil {
		t.Fatal("L1 layer missing")
	}
	if l1.ItemCount != 2 || l1.TodayAdded != 1 {
		t.Errorf("L1 counts: got item=%d today=%d, want 2/1", l1.ItemCount, l1.TodayAdded)
	}
	h1 := parseHeadline(t, l1.HeadlineJSON)
	if h1["active_tasks"] != float64(2) || h1["field_count"] != float64(4) {
		t.Errorf("L1 headline: got %v, want active_tasks=2 field_count=4", h1)
	}

	// L2
	l2s := findLayer(ov.Layers, "L2")
	if l2s == nil {
		t.Fatal("L2 layer missing")
	}
	if l2s.ItemCount != 45 || l2s.TodayAdded != 6 {
		t.Errorf("L2 counts: got item=%d today=%d, want 45/6", l2s.ItemCount, l2s.TodayAdded)
	}

	// L3
	l3 := findLayer(ov.Layers, "L3")
	if l3 == nil {
		t.Fatal("L3 layer missing")
	}
	if l3.ItemCount != 2 || l3.TodayAdded != 1 {
		t.Errorf("L3 counts: got item=%d today=%d, want 2/1", l3.ItemCount, l3.TodayAdded)
	}
	if l3.RecallHits != 11 {
		t.Errorf("L3 recall_hits: got %d, want 11 (sum of hit_count 5+6)", l3.RecallHits)
	}
	if l3.Health != "warn" {
		t.Errorf("L3 health: got %q, want warn (2 open conflicts)", l3.Health)
	}
	h3 := parseHeadline(t, l3.HeadlineJSON)
	if h3["conflict_open"] != float64(2) {
		t.Errorf("L3 conflict_open: got %v, want 2", h3["conflict_open"])
	}

	// L4
	l4s := findLayer(ov.Layers, "L4")
	if l4s == nil {
		t.Fatal("L4 layer missing")
	}
	if l4s.ItemCount != 1 || l4s.TodayAdded != 1 {
		t.Errorf("L4 counts: got item=%d today=%d, want 1/1", l4s.ItemCount, l4s.TodayAdded)
	}
	h4 := parseHeadline(t, l4s.HeadlineJSON)
	if h4["relation_count"] != float64(143) {
		t.Errorf("L4 relation_count: got %v, want 143", h4["relation_count"])
	}

	// action items
	if ai := findActionItem(ov.ActionItems, "fact_conflict"); ai == nil || ai.Count != 2 || ai.TargetTab != "browse" {
		t.Errorf("fact_conflict action item: got %+v", ai)
	}
	if ai := findActionItem(ov.ActionItems, "evolution_pending"); ai == nil || ai.Count != 1 || ai.TargetTab != "governance" {
		t.Errorf("evolution_pending action item: got %+v", ai)
	}
	if ai := findActionItem(ov.ActionItems, "context_risk"); ai == nil || ai.Count != 1 || ai.TargetTab != "panorama" {
		t.Errorf("context_risk action item: got %+v", ai)
	}

	// activity feed: merged facts + entities, newest first
	if len(ov.ActivityFeed) == 0 {
		t.Fatal("activity feed empty")
	}
	first := ov.ActivityFeed[0]
	if first.Ts != today {
		t.Errorf("activity feed first ts: got %q, want %q (newest first)", first.Ts, today)
	}
	var sawFact, sawEntity bool
	for _, it := range ov.ActivityFeed {
		switch it.Kind {
		case "fact_extracted":
			sawFact = true
			if it.LayerFrom != "L2" || it.LayerTo != "L3" {
				t.Errorf("fact_extracted flow: got %s→%s", it.LayerFrom, it.LayerTo)
			}
		case "entity_created":
			sawEntity = true
			if it.LayerTo != "L4" {
				t.Errorf("entity_created flow: got →%s", it.LayerTo)
			}
		}
	}
	if !sawFact || !sawEntity {
		t.Errorf("activity feed missing kinds: fact=%v entity=%v", sawFact, sawEntity)
	}
}

func TestLayerOverview_EmptySessionSkipsSessionScopedLayers(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		factRows:   [][]byte{},
		entityRows: [][]byte{},
	}
	uc := NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(&fakeL2AdminReader{}, &fakeL4RelReader{})

	ov, err := uc.GetLayerOverview(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetLayerOverview: %v", err)
	}
	l0 := findLayer(ov.Layers, "L0")
	l1 := findLayer(ov.Layers, "L1")
	if l0 == nil || l1 == nil {
		t.Fatal("L0/L1 layers must still be present")
	}
	if l0.ItemCount != 0 || l1.ItemCount != 0 {
		t.Errorf("session-less counts: got L0=%d L1=%d, want 0/0", l0.ItemCount, l1.ItemCount)
	}
	if len(ov.ActionItems) != 0 {
		t.Errorf("action items: got %+v, want empty (no conflicts, no warnings)", ov.ActionItems)
	}
}

func TestLayerOverview_RequiresAgentID(t *testing.T) {
	uc := NewMemoryAdminUsecase(&fakeCenterAdminDeps{}, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(&fakeL2AdminReader{}, &fakeL4RelReader{})
	if _, err := uc.GetLayerOverview(context.Background(), "", "sess-1"); err == nil {
		t.Fatal("expected error for empty agent id")
	}
}

// ── P3: ListEpisodesAdmin（L2 情景浏览，design §10.6.1）──

func TestListEpisodesAdmin_MapsRowsAndTotal(t *testing.T) {
	l2 := &fakeL2AdminReader{
		rows: [][]byte{
			centerRowJSON(map[string]any{
				"id": "ep1", "session_id": "s1", "agent_id": "agent-1",
				"episode_kind": "task", "title": "季度复盘讨论", "outcome_summary": "完成复盘并产出结论",
				"importance": 0.8, "consolidation_status": "consolidated", "consolidated_l3_count": 3,
				"ended_at": "2026-07-20T01:00:00Z", "created_at": "2026-07-20T00:00:00Z",
			}),
			centerRowJSON(map[string]any{
				"id": "ep2", "session_id": "s2", "agent_id": "agent-1",
				"episode_kind": "task", "title": "待提炼任务", "outcome_summary": "",
				"importance": 0.5, "consolidation_status": "pending", "consolidated_l3_count": 0,
				"ended_at": "", "created_at": "2026-07-21T00:00:00Z",
			}),
		},
		total: 21,
	}
	uc := NewMemoryAdminUsecase(&fakeCenterAdminDeps{}, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(l2, &fakeL4RelReader{})

	items, total, err := uc.ListEpisodesAdmin(context.Background(), "agent-1", "", 20, 0)
	if err != nil {
		t.Fatalf("ListEpisodesAdmin: %v", err)
	}
	if total != 21 {
		t.Errorf("total: got %d, want 21", total)
	}
	if len(items) != 2 {
		t.Fatalf("items: got %d, want 2", len(items))
	}
	ep := items[0]
	if ep.ID != "ep1" || ep.Title != "季度复盘讨论" || ep.OutcomeSummary != "完成复盘并产出结论" {
		t.Errorf("item[0] identity: got %+v", ep)
	}
	if ep.Kind != "task" || ep.SessionID != "s1" {
		t.Errorf("item[0] kind/session: got %+v", ep)
	}
	if ep.Importance != 0.8 || ep.ConsolidationStatus != "consolidated" || ep.ConsolidatedL3Count != 3 {
		t.Errorf("item[0] metrics: got %+v", ep)
	}
	if ep.CreatedAt != "2026-07-20T00:00:00Z" || ep.EndedAt != "2026-07-20T01:00:00Z" {
		t.Errorf("item[0] timestamps: got %+v", ep)
	}
	if items[1].ConsolidationStatus != "pending" {
		t.Errorf("item[1] status: got %q, want pending", items[1].ConsolidationStatus)
	}
}

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

	// 不报错即视为通过：limit 超限被收敛而非拒绝（与 unified graph hops 上限同策略）。
	if _, _, err := uc.ListEpisodesAdmin(context.Background(), "agent-1", "", 500, -3); err != nil {
		t.Fatalf("ListEpisodesAdmin with out-of-range limit/offset: %v", err)
	}
}
