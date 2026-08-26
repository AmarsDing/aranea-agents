package configgraph

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// healthFakeRepo 在 fakeRepo 基础上覆盖 health 全量扫描行为。
type healthFakeRepo struct {
	fakeRepo
	allNodes []Node
	allEdges []StoredEdge
	nodesErr error
	edgesErr error
	gotGen   int64
}

func (f *healthFakeRepo) ListAllNodes(_ context.Context, gen int64) ([]Node, error) {
	f.gotGen = gen
	return f.allNodes, f.nodesErr
}

func (f *healthFakeRepo) ListAllEdges(_ context.Context, gen int64) ([]StoredEdge, error) {
	f.gotGen = gen
	return f.allEdges, f.edgesErr
}

func hnode(id, typ, key string, attrs map[string]any) Node {
	return Node{ID: id, NodeType: typ, RefID: "r-" + id, NodeKey: key, DisplayName: key + " name",
		Status: NodeStatusActive, Attrs: attrs}
}

// hedge 构造测试边；origin 非空时写入 grant_origin evidence。
func hedge(id, src, dst, typ, origin string) StoredEdge {
	ev := map[string]any{}
	if origin != "" {
		ev[EvidenceKeyGrantOrigin] = origin
	}
	return StoredEdge{ID: id, SrcID: src, DstID: dst, Type: typ, Evidence: ev}
}

func hbroken(id, src, typ string) StoredEdge {
	return StoredEdge{ID: id, SrcID: src, Type: typ,
		Evidence: map[string]any{EvidenceKeyBroken: true, EvidenceKeyDstKey: "ghost"}}
}

func godIDs(gods []GodNode) []string {
	out := make([]string, 0, len(gods))
	for _, g := range gods {
		out = append(out, g.Node.ID)
	}
	return out
}

// TestHealth_GodNodeAbsoluteThreshold 绝对阈值并集 + profile 噪音双向排除 +
// top 边明细（验收 1.4：eval_memory_probe 前列、不被 profile 淹没）。
func TestHealth_GodNodeAbsoluteThreshold(t *testing.T) {
	t.Parallel()
	var nodes []Node
	var edges []StoredEdge
	// 25 个 agent 显式 allow eval_memory_probe → fan-in 25（≥20 god）。
	nodes = append(nodes, hnode("n-probe", NodeTypeTool, "eval_memory_probe", nil))
	for i := 0; i < 25; i++ {
		aid := fmt.Sprintf("n-agent-p%02d", i)
		nodes = append(nodes, hnode(aid, NodeTypeAgent, aid, nil))
		edges = append(edges, hedge(fmt.Sprintf("e-p%02d", i), aid, "n-probe", EdgeTypeGrantedTool, GrantOriginAllow))
	}
	// 30 个 agent 经 profile 隐式持有 read_file → fan-in 全排除，不得成 god。
	nodes = append(nodes, hnode("n-readfile", NodeTypeTool, "read_file", nil))
	for i := 0; i < 30; i++ {
		aid := fmt.Sprintf("n-agent-b%02d", i)
		nodes = append(nodes, hnode(aid, NodeTypeAgent, aid, nil))
		edges = append(edges, hedge(fmt.Sprintf("e-b%02d", i), aid, "n-readfile", EdgeTypeGrantedTool, GrantOriginProfile))
	}
	// 1 个 agent 带 35 条 tool_override 出边 → fan-out 35（≥30 god）。
	nodes = append(nodes, hnode("n-agent-x", NodeTypeAgent, "agent-x", nil))
	for i := 0; i < 35; i++ {
		tid := fmt.Sprintf("n-tool-o%02d", i)
		nodes = append(nodes, hnode(tid, NodeTypeTool, tid, nil))
		edges = append(edges, hedge(fmt.Sprintf("e-o%02d", i), "n-agent-x", tid, EdgeTypeToolOverride, ""))
	}

	gods := detectGodNodes(nodes, edges)
	if len(gods) != 2 {
		t.Fatalf("gods = %v, want exactly [n-agent-x n-probe]", godIDs(gods))
	}
	// 排序：max(fan) 降序 → agent-x(35) 先于 probe(25)。
	if gods[0].Node.ID != "n-agent-x" || gods[0].FanOut != 35 || gods[0].FanIn != 0 {
		t.Errorf("gods[0] = %+v", gods[0])
	}
	if gods[1].Node.ID != "n-probe" || gods[1].FanIn != 25 {
		t.Errorf("gods[1] = %+v", gods[1])
	}
	// profile 噪音：read_file fan-in 被排除为 0，不出现在 god 列表。
	for _, g := range gods {
		if g.Node.ID == "n-readfile" {
			t.Errorf("profile-noise tool must not be god: %+v", g)
		}
	}
	// top 边：主导方向（入向）截断 10 条，按 edge_type→src 排序。
	top := gods[1].TopEdges
	if len(top) != godTopEdgesCap {
		t.Fatalf("probe top edges = %d, want %d", len(top), godTopEdgesCap)
	}
	if top[0].Type != EdgeTypeGrantedTool || top[0].SrcID != "n-agent-p00" || top[9].SrcID != "n-agent-p09" {
		t.Errorf("top order: first=%s/%s ninth=%s", top[0].Type, top[0].SrcID, top[9].SrcID)
	}
	// agent-x 主导方向为出向。
	if d := gods[0].TopEdges; len(d) != godTopEdgesCap || d[0].Type != EdgeTypeToolOverride {
		t.Errorf("agent-x top = %d/%s", len(d), d[0].Type)
	}
}

// TestHealth_GodNodeP95 P95 统计阈值：离群但低于绝对阈值的节点被捕获，
// 齐平 P95 的普通节点不误报。
func TestHealth_GodNodeP95(t *testing.T) {
	t.Parallel()
	var nodes []Node
	var edges []StoredEdge
	// 20 个工具各被 1 个 agent allow（fan-in 1）；1 个共享工具被 5 个 agent
	// allow（fan-in 5）。非零 fan-in 值 [1×20, 5] → P95=1 → 仅共享工具
	// (5>1) 命中。
	nodes = append(nodes, hnode("n-shared", NodeTypeTool, "shared_tool", nil))
	for i := 0; i < 5; i++ {
		aid := fmt.Sprintf("n-agent-s%d", i)
		nodes = append(nodes, hnode(aid, NodeTypeAgent, aid, nil))
		edges = append(edges, hedge(fmt.Sprintf("e-s%d", i), aid, "n-shared", EdgeTypeGrantedTool, GrantOriginAllow))
	}
	for i := 0; i < 20; i++ {
		tid := fmt.Sprintf("n-tool-%02d", i)
		aid := fmt.Sprintf("n-agent-u%02d", i)
		nodes = append(nodes, hnode(tid, NodeTypeTool, tid, nil), hnode(aid, NodeTypeAgent, aid, nil))
		edges = append(edges, hedge(fmt.Sprintf("e-u%02d", i), aid, tid, EdgeTypeGrantedTool, GrantOriginAllow))
	}

	gods := detectGodNodes(nodes, edges)
	if len(gods) != 1 || gods[0].Node.ID != "n-shared" || gods[0].FanIn != 5 {
		t.Fatalf("gods = %+v, want only n-shared fan-in 5", gods)
	}
}

// TestHealth_Cycles 三类环全命中 + 非环边类型（hook_ref 2-环）不报 +
// 规范化去重（skill 环三成员出发仅报一次）。
func TestHealth_Cycles(t *testing.T) {
	t.Parallel()
	nodes := []Node{
		hnode("n-sa", NodeTypeSkill, "skill-a", nil),
		hnode("n-sb", NodeTypeSkill, "skill-b", nil),
		hnode("n-sc", NodeTypeSkill, "skill-c", nil),
		hnode("n-ox", NodeTypeOrganization, "org-x", nil),
		hnode("n-oy", NodeTypeOrganization, "org-y", nil),
		hnode("n-team", NodeTypeTeam, "main", nil),
		hnode("n-graph", NodeTypeGraph, "g1", nil),
		hnode("n-t1", NodeTypeTool, "t1", nil),
		hnode("n-t2", NodeTypeTool, "t2", nil),
	}
	edges := []StoredEdge{
		hedge("e1", "n-sa", "n-sb", EdgeTypeSkillParent, ""),
		hedge("e2", "n-sb", "n-sc", EdgeTypeSkillParent, ""),
		hedge("e3", "n-sc", "n-sa", EdgeTypeSkillParent, ""),
		hedge("e4", "n-ox", "n-oy", EdgeTypeOrgParent, ""),
		hedge("e5", "n-oy", "n-ox", EdgeTypeOrgParent, ""),
		hedge("e6", "n-team", "n-graph", EdgeTypeLinkedGraph, ""),
		hedge("e7", "n-graph", "n-team", EdgeTypeGraphOwnedBy, ""),
		// hook_ref 2-环：不在环检测边类型内，不得报告。
		hedge("e8", "n-t1", "n-t2", EdgeTypeHookRef, ""),
		hedge("e9", "n-t2", "n-t1", EdgeTypeHookRef, ""),
		// 断边不成环。
		hbroken("e10", "n-sa", EdgeTypeSkillParent),
	}

	cycles := detectCycles(nodes, edges)
	if len(cycles) != 3 {
		t.Fatalf("cycles = %+v, want 3", cycles)
	}
	// 发现顺序按起点 ID 排序：n-graph < n-ox < n-sa。
	c0 := cycles[0]
	if len(c0.Nodes) != 2 || c0.Nodes[0].ID != "n-graph" || c0.Nodes[1].ID != "n-team" {
		t.Errorf("cycle0 nodes = %+v", c0.Nodes)
	}
	if len(c0.Edges) != 2 || c0.Edges[0] != EdgeTypeGraphOwnedBy || c0.Edges[1] != EdgeTypeLinkedGraph {
		t.Errorf("cycle0 edges = %v", c0.Edges)
	}
	c1 := cycles[1]
	if len(c1.Nodes) != 2 || c1.Nodes[0].ID != "n-ox" || c1.Nodes[1].ID != "n-oy" {
		t.Errorf("cycle1 nodes = %+v", c1.Nodes)
	}
	c2 := cycles[2]
	if len(c2.Nodes) != 3 || c2.Nodes[0].ID != "n-sa" || c2.Nodes[1].ID != "n-sb" || c2.Nodes[2].ID != "n-sc" {
		t.Errorf("cycle2 nodes = %+v", c2.Nodes)
	}
	for _, et := range c2.Edges {
		if et != EdgeTypeSkillParent {
			t.Errorf("cycle2 edges = %v", c2.Edges)
		}
	}
	// NodeRef 带可读字段。
	if c0.Nodes[1].NodeKey != "main" || c0.Nodes[1].DisplayName == "" {
		t.Errorf("NodeRef readability: %+v", c0.Nodes[1])
	}
}

// TestHealth_BrokenGrouping 断边按类型分组计数、计数降序；无断边返回空片。
func TestHealth_BrokenGrouping(t *testing.T) {
	t.Parallel()
	edges := []StoredEdge{
		hbroken("e1", "n-a", EdgeTypeGrantedTool),
		hbroken("e2", "n-b", EdgeTypeGrantedTool),
		hbroken("e3", "n-c", EdgeTypeHookRef),
		hedge("e4", "n-a", "n-b", EdgeTypeHasMember, ""),
	}
	groups := groupBrokenEdges(edges)
	if len(groups) != 2 {
		t.Fatalf("groups = %+v, want 2", groups)
	}
	if groups[0].EdgeType != EdgeTypeGrantedTool || groups[0].Count != 2 {
		t.Errorf("groups[0] = %+v", groups[0])
	}
	if groups[1].EdgeType != EdgeTypeHookRef || groups[1].Count != 1 {
		t.Errorf("groups[1] = %+v", groups[1])
	}
	if got := groupBrokenEdges(nil); got == nil || len(got) != 0 {
		t.Errorf("empty = %v, want non-nil empty", got)
	}
}

// TestHealth_DuplicatePrompts body_hash 分组：COUNT>1 命中、软删/空 hash/
// 非 prompt 类型排除。
func TestHealth_DuplicatePrompts(t *testing.T) {
	t.Parallel()
	withHash := func(id, key, hash string) Node {
		return hnode(id, NodeTypePromptFile, key, map[string]any{"body_hash": hash})
	}
	deleted := withHash("n-p4", "p4", "aaa")
	deleted.Status = NodeStatusDeleted
	nodes := []Node{
		withHash("n-p1", "p1", "aaa"),
		withHash("n-p2", "p2", "aaa"),
		withHash("n-p3", "p3", "bbb"),
		deleted,                             // 软删不计
		withHash("n-p5", "p5", ""),          // 空 hash 不计
		hnode("n-a1", NodeTypeAgent, "a1", map[string]any{"body_hash": "aaa"}), // 类型不符
	}
	dups := groupDuplicatePrompts(nodes)
	if len(dups) != 1 {
		t.Fatalf("dups = %+v, want 1 group", dups)
	}
	if dups[0].BodyHash != "aaa" || dups[0].Count != 2 {
		t.Errorf("dup = %+v", dups[0])
	}
	if len(dups[0].Nodes) != 2 || dups[0].Nodes[0].ID != "n-p1" || dups[0].Nodes[1].ID != "n-p2" {
		t.Errorf("dup nodes = %+v", dups[0].Nodes)
	}
}

// TestQuerier_Health 全链路：gen 透传、四段装配、空图非 nil 段。
func TestQuerier_Health(t *testing.T) {
	t.Parallel()
	repo := &healthFakeRepo{
		allNodes: []Node{
			hnode("n-sa", NodeTypeSkill, "skill-a", nil),
			hnode("n-sb", NodeTypeSkill, "skill-b", nil),
			hnode("n-p1", NodeTypePromptFile, "p1", map[string]any{"body_hash": "hhh"}),
			hnode("n-p2", NodeTypePromptFile, "p2", map[string]any{"body_hash": "hhh"}),
		},
		allEdges: []StoredEdge{
			hedge("e1", "n-sa", "n-sb", EdgeTypeSkillParent, ""),
			hedge("e2", "n-sb", "n-sa", EdgeTypeSkillParent, ""),
			hbroken("e3", "n-sa", EdgeTypeOwnsSkill),
		},
	}
	q := newQuerierWith(repo, 7)
	rep, err := q.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if rep.Generation != 7 || repo.gotGen != 7 {
		t.Errorf("gen = %d/%d, want 7", rep.Generation, repo.gotGen)
	}
	if len(rep.Cycles) != 1 || len(rep.Cycles[0].Nodes) != 2 {
		t.Errorf("cycles = %+v", rep.Cycles)
	}
	if len(rep.BrokenByType) != 1 || rep.BrokenByType[0].EdgeType != EdgeTypeOwnsSkill {
		t.Errorf("broken = %+v", rep.BrokenByType)
	}
	if len(rep.DuplicatePrompts) != 1 || rep.DuplicatePrompts[0].BodyHash != "hhh" {
		t.Errorf("dups = %+v", rep.DuplicatePrompts)
	}
	if rep.GodNodes == nil { // 空段必须是非 nil 空片（JSON [] 而非 null）
		t.Error("god_nodes must be non-nil empty slice")
	}
}

// TestQuerier_HealthNotReady 与 repo 错误透传。
func TestQuerier_HealthNotReady(t *testing.T) {
	t.Parallel()
	if _, err := newQuerierWith(&healthFakeRepo{}, 0).Health(context.Background()); !errors.Is(err, ErrNotReady) {
		t.Errorf("err = %v, want ErrNotReady", err)
	}
	errBoom := errors.New("boom")
	if _, err := newQuerierWith(&healthFakeRepo{nodesErr: errBoom}, 7).Health(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("nodes err = %v, want boom", err)
	}
	if _, err := newQuerierWith(&healthFakeRepo{edgesErr: errBoom}, 7).Health(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("edges err = %v, want boom", err)
	}
}

// TestPercentile95 最近秩法边界。
func TestPercentile95(t *testing.T) {
	t.Parallel()
	if got := percentile95(nil); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := percentile95([]int{7}); got != 7 {
		t.Errorf("single = %d, want 7", got)
	}
	// n=21 → rank=ceil(19.95)=20 → 第 20 小值。
	vals := append([]int{5}, make([]int, 20)...)
	copy(vals[1:], repeatInt(1, 20))
	if got := percentile95(vals); got != 1 {
		t.Errorf("p95 = %d, want 1", got)
	}
}

func repeatInt(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}
