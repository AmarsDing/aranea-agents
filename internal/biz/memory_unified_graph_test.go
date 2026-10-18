package biz

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func entityRow(id, name string, useCount int) []byte {
	return centerRowJSON(map[string]any{
		"id": id, "name": name, "entity_type": "person",
		"use_count": useCount, "confidence": 0.9, "created_at": "2026-07-20T00:00:00Z",
	})
}

func factRow(id, statement string, hitCount int, links []string, sourceEpisodeID string) []byte {
	return centerRowJSON(map[string]any{
		"id": id, "statement": statement, "hit_count": hitCount,
		"confidence": 0.8, "links": links, "source_episode_id": sourceEpisodeID,
		"created_at": "2026-07-20T00:00:00Z",
	})
}

func relationRow(id, src, tgt, relType string, weight float64) []byte {
	return centerRowJSON(map[string]any{
		"id": id, "source_id": src, "target_id": tgt,
		"relation_type": relType, "weight": weight, "status": "active",
	})
}

func episodeRow(id, title string) []byte {
	return centerRowJSON(map[string]any{
		"id": id, "title": title, "outcome_summary": "摘要",
		"created_at": "2026-07-20T00:00:00Z",
	})
}

func edgeTypes(g *UnifiedMemoryGraph) map[string]int {
	m := map[string]int{}
	for _, e := range g.Edges {
		m[e.Type]++
	}
	return m
}

func findEdge(g *UnifiedMemoryGraph, source, target, typ string) *UnifiedGraphEdge {
	for i := range g.Edges {
		e := &g.Edges[i]
		if e.Source == source && e.Target == target && e.Type == typ {
			return e
		}
	}
	return nil
}

func nodeIDs(g *UnifiedMemoryGraph) map[string]bool {
	m := map[string]bool{}
	for _, n := range g.Nodes {
		m[n.ID] = true
	}
	return m
}

func newGraphUsecase(deps *fakeCenterAdminDeps, l2 *fakeL2AdminReader, l4 *fakeL4RelReader) *MemoryAdminUsecase {
	uc := NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	uc.SetMemoryCenterReaders(l2, l4)
	return uc
}

func TestUnifiedMemoryGraph_EdgeClassification(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "实体一", 10), entityRow("e2", "实体二", 5)},
		factRows: [][]byte{
			factRow("f1", "事实一", 11, []string{"f2"}, "ep1"),
			factRow("f2", "事实二", 3, nil, ""),
			factRow("f3", "事实三", 1, nil, ""),
		},
	}
	l2 := &fakeL2AdminReader{byIDs: map[string][]byte{"ep1": episodeRow("ep1", "季度复盘讨论")}}
	l4 := &fakeL4RelReader{
		topID: "e1",
		rows: [][]byte{
			relationRow("r1", "e1", "e2", "WORKS_AT", 0.8),
			relationRow("r2", "e1", "f1", "RELATED_TO", 0.9),
			relationRow("r3", "f1", "f2", "EVOLVED_FROM", 0.7),
			relationRow("r4", "f2", "f3", "INHIBIT", 0.9),
			relationRow("r5", "e1", "ghost", "RELATED_TO", 0.5), // unknown endpoint → dropped
		},
	}
	uc := newGraphUsecase(deps, l2, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 3, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if g.EmptyReason != "" {
		t.Fatalf("empty_reason: got %q", g.EmptyReason)
	}

	types := edgeTypes(g)
	if types["entity_relation"] != 1 {
		t.Errorf("entity_relation count: got %d, want 1", types["entity_relation"])
	}
	if types["entity_fact"] != 1 {
		t.Errorf("entity_fact count: got %d, want 1", types["entity_fact"])
	}
	// fact_link: relation EVOLVED_FROM f1→f2 + links_json f1→f2 deduped → 1
	if types["fact_link"] != 1 {
		t.Errorf("fact_link count: got %d, want 1 (deduped)", types["fact_link"])
	}
	if types["fact_conflict"] != 1 {
		t.Errorf("fact_conflict count: got %d, want 1 (INHIBIT between facts)", types["fact_conflict"])
	}
	if types["fact_source"] != 1 {
		t.Errorf("fact_source count: got %d, want 1", types["fact_source"])
	}
	if findEdge(g, "e1", "ghost", "entity_relation") != nil {
		t.Error("unknown-endpoint edge must be dropped")
	}

	// INHIBIT polarity
	ce := findEdge(g, "f2", "f3", "fact_conflict")
	if ce == nil {
		t.Fatal("fact_conflict edge f2→f3 missing")
	}
	if ce.Polarity != "INHIBIT" {
		t.Errorf("fact_conflict polarity: got %q, want INHIBIT", ce.Polarity)
	}

	// nodes: 2 entities + 3 facts + 1 episode
	ids := nodeIDs(g)
	for _, id := range []string{"e1", "e2", "f1", "f2", "f3", "ep1"} {
		if !ids[id] {
			t.Errorf("node %s missing", id)
		}
	}

	// node shapes
	var epNode *UnifiedGraphNode
	for i := range g.Nodes {
		if g.Nodes[i].ID == "ep1" {
			epNode = &g.Nodes[i]
		}
	}
	if epNode == nil {
		t.Fatal("episode node missing")
	}
	if epNode.Layer != "L2" || epNode.Kind != "episode" || epNode.Label != "季度复盘讨论" {
		t.Errorf("episode node: got %+v", epNode)
	}
}

// TestUnifiedMemoryGraph_FactQueryCrossScopeAgentFilter pins the H1 fix: the
// unified graph must load facts by the ORIGINATING agent_id column across ALL
// scopes (session/user/agent) — not scope_type='agent' — because immediate
// facts live in session scope and consolidated facts in user scope (F1). The
// old scope='agent' query silently dropped those fact nodes and their edges.
func TestUnifiedMemoryGraph_FactQueryCrossScopeAgentFilter(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "实体一", 1)},
		factRows:   [][]byte{factRow("f1", "事实一", 1, nil, "")},
	}
	l4 := &fakeL4RelReader{rows: [][]byte{relationRow("r1", "e1", "f1", "RELATED_TO", 0.9)}}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	if _, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 3, 0, nil); err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if deps.factCallScopeType != "" {
		t.Errorf("fact query scope_type = %q, want empty (cross-scope agent filter)", deps.factCallScopeType)
	}
	if deps.factCallAgentID != "agent-1" {
		t.Errorf("fact query agent_id = %q, want agent-1", deps.factCallAgentID)
	}
}

func TestUnifiedMemoryGraph_BFSTruncatesAtHops(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{
			entityRow("e1", "一", 1), entityRow("e2", "二", 1),
			entityRow("e3", "三", 1), entityRow("e4", "四", 1),
		},
	}
	l4 := &fakeL4RelReader{rows: [][]byte{
		relationRow("r1", "e1", "e2", "RELATED_TO", 0.9),
		relationRow("r2", "e2", "e3", "RELATED_TO", 0.9),
		relationRow("r3", "e3", "e4", "RELATED_TO", 0.9),
	}}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "e1", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	ids := nodeIDs(g)
	if !ids["e1"] || !ids["e2"] || !ids["e3"] {
		t.Errorf("BFS hops=2 must include e1,e2,e3; got %v", ids)
	}
	if ids["e4"] {
		t.Errorf("BFS hops=2 must exclude e4 (3 hops away)")
	}
	if len(g.Edges) != 2 {
		t.Errorf("edges: got %d, want 2 (e1-e2, e2-e3)", len(g.Edges))
	}

	// default hops (=2) behaves the same when hops<=0
	g2, _ := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "e1", 0, 0, nil)
	if len(g2.Nodes) != len(g.Nodes) {
		t.Errorf("default hops: got %d nodes, want %d", len(g2.Nodes), len(g.Nodes))
	}
}

func TestUnifiedMemoryGraph_MinWeightFiltersRelationEdges(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "一", 1), entityRow("e2", "二", 1), entityRow("e3", "三", 1)},
	}
	l4 := &fakeL4RelReader{rows: [][]byte{
		relationRow("r1", "e1", "e2", "RELATED_TO", 0.8),
		relationRow("r2", "e2", "e3", "RELATED_TO", 0.2), // below threshold
	}}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "e1", 2, 0.35, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Errorf("edges after filter: got %d, want 1", len(g.Edges))
	}
	if g.FilteredEdgeCount != 1 {
		t.Errorf("filtered_edge_count: got %d, want 1", g.FilteredEdgeCount)
	}
	// BFS happens before weight filtering, so e3 is still a (disconnected-looking) node.
	if !nodeIDs(g)["e3"] {
		t.Error("e3 must remain in nodes (BFS precedes weight filter)")
	}
}

func TestUnifiedMemoryGraph_DefaultFocusIsTopConnected(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "一", 1), entityRow("e2", "二", 9)},
	}
	l4 := &fakeL4RelReader{
		topID: "e2",
		rows:  [][]byte{relationRow("r1", "e1", "e2", "RELATED_TO", 0.9)},
	}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if g.Focus != "e2" {
		t.Errorf("focus: got %q, want e2 (TopConnectedEntityID)", g.Focus)
	}
}

func TestUnifiedMemoryGraph_EmptyAndMissingFocus(t *testing.T) {
	uc := newGraphUsecase(&fakeCenterAdminDeps{}, &fakeL2AdminReader{}, &fakeL4RelReader{})

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if g.EmptyReason != "no_memory_data" {
		t.Errorf("empty_reason: got %q, want no_memory_data", g.EmptyReason)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("empty graph must have no nodes/edges, got %d/%d", len(g.Nodes), len(g.Edges))
	}

	deps := &fakeCenterAdminDeps{entityRows: [][]byte{entityRow("e1", "一", 1)}}
	uc2 := newGraphUsecase(deps, &fakeL2AdminReader{}, &fakeL4RelReader{})
	g2, err := uc2.GetUnifiedMemoryGraph(context.Background(), "agent-1", "missing", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if g2.EmptyReason != "focus_not_found" {
		t.Errorf("empty_reason: got %q, want focus_not_found", g2.EmptyReason)
	}
}

func TestUnifiedMemoryGraph_LayerFilter(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "一", 1)},
		factRows:   [][]byte{factRow("f1", "事实一", 3, nil, "")},
	}
	l4 := &fakeL4RelReader{rows: [][]byte{
		relationRow("r1", "e1", "f1", "RELATED_TO", 0.9),
	}}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 2, 0, []string{"L3"})
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	ids := nodeIDs(g)
	if ids["e1"] {
		t.Error("L4 entity must be excluded when layers=[L3]")
	}
	if !ids["f1"] {
		t.Error("L3 fact must be included")
	}
	if len(g.Edges) != 0 {
		t.Errorf("cross-layer edge must be excluded when layers=[L3], got %d", len(g.Edges))
	}
}

// fact 节点 Label 被截断为 40 字符，前端「在记忆浏览中打开」依赖 meta_json 中的完整 statement 做搜索。
func TestUnifiedMemoryGraph_FactMetaCarriesFullStatement(t *testing.T) {
	longStatement := "User prefers to organize work using multi-team parallel+sequential workflow patterns (two parallel teams feeding conclusions into a third team)."
	deps := &fakeCenterAdminDeps{
		factRows: [][]byte{factRow("f1", longStatement, 0, nil, "")},
	}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, &fakeL4RelReader{})

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "f1", 1, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if len(g.Nodes) != 1 {
		t.Fatalf("nodes: got %d, want 1", len(g.Nodes))
	}
	n := g.Nodes[0]
	if len([]rune(n.Label)) > 41 { // 40 字 + 省略号
		t.Errorf("label must be truncated to 40 runes, got %d runes", len([]rune(n.Label)))
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(n.MetaJSON), &meta); err != nil {
		t.Fatalf("meta_json unmarshal: %v", err)
	}
	if meta["statement"] != longStatement {
		t.Errorf("meta.statement: got %q, want full statement", meta["statement"])
	}
}

// P1-1（2026-10-18 审计）：fact 节点权重必须取真实召回计数 recalled_count
// （写管线 merge / 召回链真实递增），缺失或为 0 时回退 importance；
// hit_count 是全库无递增代码的死字段（恒 0），此前的 weight=0 使图谱节点
// 大小/排序全部失真。
func TestUnifiedMemoryGraph_FactNodeWeightUsesRecalledCount(t *testing.T) {
	mkFact := func(id string, recalled int, importance float64) []byte {
		return centerRowJSON(map[string]any{
			"id": id, "statement": "事实-" + id, "recalled_count": recalled,
			"importance": importance, "confidence": 0.8,
			"created_at": "2026-07-20T00:00:00Z",
		})
	}
	deps := &fakeCenterAdminDeps{
		factRows: [][]byte{
			mkFact("f_recall", 7, 0.3),
			mkFact("f_fresh", 0, 0.6),
			mkFact("f_zero", 0, 0),
		},
	}
	// relation 把三个 fact 连成 BFS 可达（图谱输出只含 focus hops 内节点）。
	l4 := &fakeL4RelReader{rows: [][]byte{
		relationRow("r1", "f_recall", "f_fresh", "RELATED_TO", 0.5),
		relationRow("r2", "f_recall", "f_zero", "RELATED_TO", 0.5),
	}}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "f_recall", 1, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	w := map[string]float64{}
	for _, n := range g.Nodes {
		w[n.ID] = n.Weight
	}
	if got := w["f_recall"]; got != 7 {
		t.Errorf("f_recall weight: got %v, want 7 (recalled_count)", got)
	}
	if got := w["f_fresh"]; got != 0.6 {
		t.Errorf("f_fresh weight: got %v, want 0.6 (importance fallback)", got)
	}
	if got := w["f_zero"]; got != 0 {
		t.Errorf("f_zero weight: got %v, want 0", got)
	}
	for _, n := range g.Nodes {
		if n.ID != "f_recall" {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(n.MetaJSON), &meta); err != nil {
			t.Fatalf("meta_json unmarshal: %v", err)
		}
		if _, ok := meta["hit_count"]; ok {
			t.Errorf("meta hit_count must be dropped (dead counter)")
		}
		if got, _ := meta["recalled_count"].(float64); got != 7 {
			t.Errorf("meta recalled_count: got %v, want 7", got)
		}
	}
}

// P2-3（2026-10-18 审计）：scope 规模超过统一扫描上限（unifiedGraphScanLimit）
// 时响应必须带 truncated=true——前端据此提示"仅最近若干条参与构图"，而非静默
// 缺节点。检测口径 = 行数口径匹配的总量（实体 total / 事实 active）> 实际加
// 载行数；SQL 侧 ORDER BY updated_at DESC 保证截断截掉的是最旧数据。焦点因截
// 断缺失时 focus_not_found 同样要带标记，避免"没有这个记忆"的误判。
func TestUnifiedMemoryGraph_TruncatedWhenScanCapsRows(t *testing.T) {
	deps := &fakeCenterAdminDeps{
		entityRows: [][]byte{entityRow("e1", "实体一", 10), entityRow("e2", "实体二", 5)},
		// 真实 SQL 下 total 是全量计数、已加载行受 LIMIT 收敛——fake 忽略
		// limit，直接以 total>len(rows) 模拟 600+ 行 scope 截到 500 的情形。
		entityTotal: 700,
		factRows:    [][]byte{factRow("f1", "事实一", 11, nil, "")},
		factActive:  640,
	}
	l4 := &fakeL4RelReader{
		topID: "e1",
		rows:  [][]byte{relationRow("r1", "e1", "f1", "RELATED_TO", 0.9)},
	}
	uc := newGraphUsecase(deps, &fakeL2AdminReader{}, l4)

	g, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph: %v", err)
	}
	if !g.Truncated {
		t.Error("truncated: got false, want true (entityTotal 700>2, factActive 640>1)")
	}

	// 焦点不在已加载行内 + 截断 → focus_not_found 也必须带 truncated。
	g2, err := uc.GetUnifiedMemoryGraph(context.Background(), "agent-1", "ghost-old-entity", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph(focus): %v", err)
	}
	if g2.EmptyReason != "focus_not_found" {
		t.Fatalf("empty_reason: got %q, want focus_not_found", g2.EmptyReason)
	}
	if !g2.Truncated {
		t.Error("focus_not_found + truncated scan must still report truncated=true")
	}

	// 规模在扫描上限内（总量 == 已加载行数）→ 不标截断。
	depsFits := &fakeCenterAdminDeps{
		entityRows:  [][]byte{entityRow("e1", "实体一", 10)},
		entityTotal: 1,
		factRows:    [][]byte{factRow("f1", "事实一", 11, nil, "")},
		factActive:  1,
	}
	l4Fits := &fakeL4RelReader{topID: "e1", rows: [][]byte{relationRow("r1", "e1", "f1", "RELATED_TO", 0.9)}}
	ucFits := newGraphUsecase(depsFits, &fakeL2AdminReader{}, l4Fits)
	g3, err := ucFits.GetUnifiedMemoryGraph(context.Background(), "agent-1", "", 2, 0, nil)
	if err != nil {
		t.Fatalf("GetUnifiedMemoryGraph(fits): %v", err)
	}
	if g3.Truncated {
		t.Error("truncated: got true, want false when totals fit loaded rows")
	}
}
