package configgraph

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

var errFakeNotFound = apierror.NotFound("CONFIG_GRAPH", "node not found")

// queryFakeRepo 在 fakeRepo（rebuilder_test.go）基础上覆盖 P1 查询行为。
type queryFakeRepo struct {
	fakeRepo
	target     Node
	targetErr  error
	walkRows   []WalkRow
	walkErr    error
	gotReverse bool
	gotDepth   int
	outEdges   []StoredEdge
	inEdges    []StoredEdge
	brokenOwn  []StoredEdge
	brokenTgt  []StoredEdge
	brokenKeys []string
	sessions   int64
	agentIDs   []string
	teamIDs    []string
}

func (f *queryFakeRepo) FindNode(_ context.Context, _ int64, _, _ string) (Node, error) {
	if f.targetErr != nil {
		return Node{}, f.targetErr
	}
	return f.target, nil
}

func (f *queryFakeRepo) WalkGraph(_ context.Context, _ int64, _ string, reverse bool, maxDepth int) ([]WalkRow, error) {
	f.gotReverse = reverse
	f.gotDepth = maxDepth
	return f.walkRows, f.walkErr
}

func (f *queryFakeRepo) ListNodeEdges(_ context.Context, _ int64, _ string) ([]StoredEdge, []StoredEdge, []StoredEdge, error) {
	return f.outEdges, f.inEdges, f.brokenOwn, nil
}

func (f *queryFakeRepo) ListBrokenEdgesTargeting(_ context.Context, _ int64, keys []string) ([]StoredEdge, error) {
	f.brokenKeys = keys
	return f.brokenTgt, nil
}

func (f *queryFakeRepo) CountActiveSessions(_ context.Context, agentIDs, teamIDs []string) (int64, error) {
	f.agentIDs = agentIDs
	f.teamIDs = teamIDs
	return f.sessions, nil
}

func newQuerierWith(repo Repo, gen int64) *Querier {
	return NewQuerier(repo, func() int64 { return gen })
}

var querierTarget = Node{
	ID: "n-tool", NodeType: NodeTypeTool, RefID: "uuid-tool", NodeKey: "shell",
	Attrs: map[string]any{"risk_level": "high"},
}

// TestQuerier_NotReady 首启无图：gen==0 时三个查询一律 ErrNotReady
// （design §4.3 兜底 + §6 错误码）。
func TestQuerier_NotReady(t *testing.T) {
	t.Parallel()
	q := newQuerierWith(&queryFakeRepo{}, 0)
	if _, err := q.Impact(context.Background(), "tool", "shell", 3); !errors.Is(err, ErrNotReady) {
		t.Errorf("Impact err = %v, want ErrNotReady", err)
	}
	if _, err := q.Dependencies(context.Background(), "tool", "shell", 3); !errors.Is(err, ErrNotReady) {
		t.Errorf("Dependencies err = %v, want ErrNotReady", err)
	}
	if _, err := q.NodeEdges(context.Background(), "tool", "shell"); !errors.Is(err, ErrNotReady) {
		t.Errorf("NodeEdges err = %v, want ErrNotReady", err)
	}
}

// TestQuerier_NodeNotFound 目标不存在 → CodeNotFound 透传（service 映射
// CONFIG_GRAPH.NODE_NOT_FOUND）。
func TestQuerier_NodeNotFound(t *testing.T) {
	t.Parallel()
	q := newQuerierWith(&queryFakeRepo{targetErr: errFakeNotFound}, 7)
	_, err := q.Impact(context.Background(), "tool", "ghost", 3)
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("err = %v, want CodeNotFound", err)
	}
}

// impactFixtureRows 构造已知图（target=tool n-tool）：
//
//	n-agent-a --granted_tool{override}--> n-tool        （depth 1）
//	n-team    --has_member-------------> n-agent-a      （depth 2）
//	n-team    --granted_tool{allow}----> n-tool         （depth 1，多路径）
//	n-cron    --runs-------------------> n-agent-a      （depth 2，与 team 同深度）
//
// n-agent-a 同时以 depth1/depth2 两行到达（多路径）→ 聚合必须保留最短。
var impactFixtureRows = []WalkRow{
	{Edge: StoredEdge{SrcID: "n-agent-a", DstID: "n-tool", Type: EdgeTypeGrantedTool,
		Evidence: map[string]any{EvidenceKeyGrantOrigin: GrantOriginOverride}},
		Node:  Node{ID: "n-agent-a", NodeType: NodeTypeAgent, RefID: "uuid-agent-a", NodeKey: "agent-a"},
		Depth: 1, Via: []string{EdgeTypeGrantedTool}},
	{Edge: StoredEdge{SrcID: "n-team", DstID: "n-agent-a", Type: EdgeTypeHasMember},
		Node: Node{ID: "n-team", NodeType: NodeTypeTeam, RefID: "uuid-team", NodeKey: "main",
			Attrs: map[string]any{"is_default": true}},
		Depth: 2, Via: []string{EdgeTypeGrantedTool, EdgeTypeHasMember}},
	{Edge: StoredEdge{SrcID: "n-team", DstID: "n-tool", Type: EdgeTypeGrantedTool,
		Evidence: map[string]any{EvidenceKeyGrantOrigin: GrantOriginAllow}},
		Node: Node{ID: "n-team", NodeType: NodeTypeTeam, RefID: "uuid-team", NodeKey: "main",
			Attrs: map[string]any{"is_default": true}},
		Depth: 1, Via: []string{EdgeTypeGrantedTool}},
	{Edge: StoredEdge{SrcID: "n-cron", DstID: "n-agent-a", Type: EdgeTypeRuns},
		Node:  Node{ID: "n-cron", NodeType: NodeTypeCronTask, RefID: "uuid-cron", NodeKey: "nightly"},
		Depth: 2, Via: []string{EdgeTypeGrantedTool, EdgeTypeRuns}},
}

// TestQuerier_Impact 全链路：聚合（最短 depth/排序）、signals、risk 加权、
// broken 段、反向标志与 depth 透传。
func TestQuerier_Impact(t *testing.T) {
	t.Parallel()
	repo := &queryFakeRepo{
		target:   querierTarget,
		walkRows: impactFixtureRows,
		brokenTgt: []StoredEdge{
			{SrcID: "n-agent-b", DstID: "", Type: EdgeTypeGrantedTool,
				Evidence: map[string]any{EvidenceKeyBroken: true, EvidenceKeyDstKey: "shell"}},
		},
		sessions: 3,
	}
	q := newQuerierWith(repo, 7)
	res, err := q.Impact(context.Background(), "tool", "shell", 3)
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}

	// 反向 + depth 透传。
	if !repo.gotReverse || repo.gotDepth != 3 {
		t.Errorf("walk args reverse=%v depth=%d, want true/3", repo.gotReverse, repo.gotDepth)
	}

	// 聚合：3 个节点（agent-a/team/cron）；agent-a 与 team 均 depth1 直达，
	// team 多路径保留最短 depth1；排序 depth asc → node_type → node_key：
	// agent(1) < team(1) < cron(2)。
	if len(res.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (%+v)", len(res.Nodes), res.Nodes)
	}
	if res.Nodes[0].Node.ID != "n-agent-a" || res.Nodes[0].Depth != 1 {
		t.Errorf("nodes[0] = %+v, want n-agent-a depth1", res.Nodes[0])
	}
	if res.Nodes[1].Node.ID != "n-team" || res.Nodes[1].Depth != 1 {
		t.Errorf("nodes[1] = %+v, want n-team depth1（多路径取最短）", res.Nodes[1])
	}
	if res.Nodes[2].Node.ID != "n-cron" || res.Nodes[2].Depth != 2 {
		t.Errorf("nodes[2] = %+v, want n-cron depth2", res.Nodes[2])
	}
	if got := res.Nodes[1].Via; len(got) != 1 || got[0] != EdgeTypeGrantedTool {
		t.Errorf("team via = %v, want [granted_tool]（最短路径）", got)
	}

	// signals：agent/team ref 收集、cron 计数、默认团队、会话数。
	if res.Signals.CronTasks != 1 || !res.Signals.DefaultTeam || res.Signals.ActiveSessions != 3 {
		t.Errorf("signals = %+v", res.Signals)
	}
	if len(repo.agentIDs) != 1 || repo.agentIDs[0] != "uuid-agent-a" {
		t.Errorf("agentIDs = %v", repo.agentIDs)
	}
	if len(repo.teamIDs) != 1 || repo.teamIDs[0] != "uuid-team" {
		t.Errorf("teamIDs = %v", repo.teamIDs)
	}

	// risk：边去重后 override30 + has_member5 + allow15 + runs5 = 55；
	// 高危工具目标 +20、默认团队 +15、cron +10、会话 +3 → 103 → high。
	wantScore := 30 + 5 + 15 + 5 + 20 + 15 + 10 + 3
	if res.Risk.Score != wantScore || res.Risk.Level != RiskLevelHigh {
		t.Errorf("risk = %+v, want score %d high", res.Risk, wantScore)
	}
	bd := res.Risk.Breakdown
	if bd.OverrideEdges != 1 || bd.AllowEdges != 1 || bd.OtherEdges != 2 {
		t.Errorf("breakdown = %+v", bd)
	}

	// broken 段：keys 传 ref_id + node_key；结果透传。
	if len(repo.brokenKeys) != 2 || repo.brokenKeys[0] != "uuid-tool" || repo.brokenKeys[1] != "shell" {
		t.Errorf("brokenKeys = %v", repo.brokenKeys)
	}
	if len(res.Broken) != 1 || !res.Broken[0].Broken() {
		t.Errorf("broken = %+v", res.Broken)
	}
	if res.Generation != 7 || res.Target.ID != "n-tool" {
		t.Errorf("meta = gen %d target %s", res.Generation, res.Target.ID)
	}
}

// TestQuerier_Dependencies 正向闭包：reverse=false，行聚合后返回。
func TestQuerier_Dependencies(t *testing.T) {
	t.Parallel()
	repo := &queryFakeRepo{
		target: Node{ID: "n-agent-a", NodeType: NodeTypeAgent, RefID: "uuid-agent-a", NodeKey: "agent-a"},
		walkRows: []WalkRow{
			{Edge: StoredEdge{SrcID: "n-agent-a", DstID: "n-tool", Type: EdgeTypeGrantedTool},
				Node:  Node{ID: "n-tool", NodeType: NodeTypeTool, RefID: "uuid-tool", NodeKey: "shell"},
				Depth: 1, Via: []string{EdgeTypeGrantedTool}},
		},
	}
	q := newQuerierWith(repo, 7)
	res, err := q.Dependencies(context.Background(), "agent", "agent-a", 5)
	if err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
	if repo.gotReverse {
		t.Errorf("dependencies must walk forward (reverse=false)")
	}
	if repo.gotDepth != 5 {
		t.Errorf("depth = %d, want 5", repo.gotDepth)
	}
	if len(res.Nodes) != 1 || res.Nodes[0].Node.ID != "n-tool" {
		t.Errorf("nodes = %+v", res.Nodes)
	}
}

// TestQuerier_NodeEdges 邻接边三段透传。
func TestQuerier_NodeEdges(t *testing.T) {
	t.Parallel()
	repo := &queryFakeRepo{
		target:    querierTarget,
		outEdges:  []StoredEdge{{SrcID: "n-tool", DstID: "n-x", Type: EdgeTypeHookRef}},
		inEdges:   []StoredEdge{{SrcID: "n-agent-a", DstID: "n-tool", Type: EdgeTypeGrantedTool}},
		brokenOwn: []StoredEdge{{SrcID: "n-tool", DstID: "", Type: EdgeTypeHookRef, Evidence: map[string]any{EvidenceKeyBroken: true}}},
	}
	q := newQuerierWith(repo, 7)
	res, err := q.NodeEdges(context.Background(), "tool", "shell")
	if err != nil {
		t.Fatalf("NodeEdges: %v", err)
	}
	if len(res.Out) != 1 || len(res.In) != 1 || len(res.Broken) != 1 {
		t.Errorf("out/in/broken = %d/%d/%d", len(res.Out), len(res.In), len(res.Broken))
	}
}

// TestQuerier_DepthClamp depth 参数边界：<=0 默认 3，>10 截断 10。
func TestQuerier_DepthClamp(t *testing.T) {
	t.Parallel()
	repo := &queryFakeRepo{target: querierTarget}
	q := newQuerierWith(repo, 7)
	if _, err := q.Impact(context.Background(), "tool", "shell", 0); err != nil {
		t.Fatal(err)
	}
	if repo.gotDepth != defaultQueryDepth {
		t.Errorf("depth 0 → %d, want %d", repo.gotDepth, defaultQueryDepth)
	}
	if _, err := q.Impact(context.Background(), "tool", "shell", 99); err != nil {
		t.Fatal(err)
	}
	if repo.gotDepth != maxQueryDepth {
		t.Errorf("depth 99 → %d, want %d", repo.gotDepth, maxQueryDepth)
	}
}

// TestQuerier_NewQuerierNil 装配侧判空语义。
func TestQuerier_NewQuerierNil(t *testing.T) {
	t.Parallel()
	if NewQuerier(nil, func() int64 { return 1 }) != nil {
		t.Error("nil repo must yield nil querier")
	}
	if NewQuerier(&queryFakeRepo{}, nil) != nil {
		t.Error("nil gen func must yield nil querier")
	}
}
