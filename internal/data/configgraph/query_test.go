package configgraph

import (
	"context"
	"database/sql"
	"os"
	"testing"

	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/data"
	"aranea-agents/pkg/apierror"

	_ "github.com/lib/pq"
)

// pgQueryRepo 与 repo_test.go 的 pgRepo 同模式（TEMP 表影子 + 单会话），
// 额外建 sessions 影子表（CountActiveSessions 探针目标），并返回 db 供
// 测试直接插 sessions 行。独立 helper 避免改动 repo_test.go。
func pgQueryRepo(t *testing.T) (bizcg.Repo, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("ARANEA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ARANEA_TEST_PG_DSN not set; skipping Postgres repo integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(1) // single session: TEMP tables must shadow across statements
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("postgres unreachable (%v); skipping", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("exec failed: %v\nSQL: %s", err, q)
		}
	}
	// Composite PK (id, generation) mirrors migration 20261261 — same id must
	// coexist across generations (dual-generation switchover).
	mustExec(`CREATE TEMP TABLE config_graph_nodes (
	  id TEXT NOT NULL,
	  node_type TEXT NOT NULL,
	  ref_id TEXT NOT NULL DEFAULT '',
	  node_key TEXT NOT NULL DEFAULT '',
	  display_name TEXT NOT NULL DEFAULT '',
	  workspace_id TEXT NOT NULL DEFAULT '',
	  status TEXT NOT NULL DEFAULT 'active',
	  attrs_json TEXT NOT NULL DEFAULT '{}',
	  generation INTEGER NOT NULL DEFAULT 0,
	  created_at TEXT NOT NULL DEFAULT '',
	  updated_at TEXT NOT NULL DEFAULT '',
	  PRIMARY KEY (id, generation)
	)`)
	mustExec(`CREATE UNIQUE INDEX uq_config_graph_nodes_ref ON config_graph_nodes(node_type, ref_id, generation)`)
	mustExec(`CREATE TEMP TABLE config_graph_edges (
	  id TEXT NOT NULL,
	  src_id TEXT NOT NULL DEFAULT '',
	  dst_id TEXT NOT NULL DEFAULT '',
	  edge_type TEXT NOT NULL,
	  evidence_json TEXT NOT NULL DEFAULT '{}',
	  workspace_id TEXT NOT NULL DEFAULT '',
	  generation INTEGER NOT NULL DEFAULT 0,
	  created_at TEXT NOT NULL DEFAULT '',
	  PRIMARY KEY (id, generation)
	)`)
	mustExec(`CREATE UNIQUE INDEX uq_config_graph_edges ON config_graph_edges(src_id, dst_id, edge_type, generation)`)
	// sessions 影子表（仅 signals 探针所需列；dialect_integration_test.go 先例）。
	mustExec(`CREATE TEMP TABLE sessions (
	  id TEXT PRIMARY KEY,
	  agent_id TEXT NOT NULL DEFAULT '',
	  team_id TEXT NOT NULL DEFAULT '',
	  status TEXT NOT NULL DEFAULT 'idle',
	  deleted_at TEXT NOT NULL DEFAULT ''
	)`)

	return NewRepoFromRWDB(data.NewReadWriteDB(db, db), data.DialectPostgres, nil), db
}

// qedge 构造测试边。
func qedge(id, src, dst, typ string, ev map[string]any, gen int64) bizcg.StoredEdge {
	return bizcg.StoredEdge{ID: id, SrcID: src, DstID: dst, Type: typ, Evidence: ev, Generation: gen, CreatedAt: ts()}
}

// seedQueryGraph 构建已知图（generation 7）：
//
//	n-agent-a --granted_tool{override}--> n-tool      (e1)
//	n-team    --has_member-------------> n-agent-a    (e2)
//	n-team    --granted_tool{allow}----> n-tool       (e3)
//	n-cron    --runs-------------------> n-agent-a    (e4)
//	n-tool    --hook_ref---------------> n-team       (e5，环：tool→team→agent/tool)
//	n-agent-a --granted_tool-----------> ''           (e6，broken dst_key=shell)
//	n-agent-b --granted_tool-----------> ''           (e7，broken dst_key=ghost)
func seedQueryGraph(t *testing.T, r bizcg.Repo) {
	t.Helper()
	ctx := context.Background()
	const gen = 7
	nodes := []bizcg.Node{
		node("n-tool", bizcg.NodeTypeTool, "uuid-tool", "shell", gen),
		node("n-agent-a", bizcg.NodeTypeAgent, "uuid-agent-a", "agent-a", gen),
		node("n-agent-b", bizcg.NodeTypeAgent, "uuid-agent-b", "agent-b", gen),
		node("n-team", bizcg.NodeTypeTeam, "uuid-team", "main", gen),
		node("n-cron", bizcg.NodeTypeCronTask, "uuid-cron", "nightly", gen),
		// 双解碰撞用例：ref_id=shared 与 node_key=shared 同类型共存，ref 必须赢。
		node("n-c1", bizcg.NodeTypeTool, "shared", "k-c1", gen),
		node("n-c2", bizcg.NodeTypeTool, "r-c2", "shared", gen),
	}
	if err := r.UpsertNodes(ctx, nodes); err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}
	edges := []bizcg.StoredEdge{
		qedge("e1", "n-agent-a", "n-tool", bizcg.EdgeTypeGrantedTool,
			map[string]any{"grant_origin": bizcg.GrantOriginOverride}, gen),
		qedge("e2", "n-team", "n-agent-a", bizcg.EdgeTypeHasMember, map[string]any{}, gen),
		qedge("e3", "n-team", "n-tool", bizcg.EdgeTypeGrantedTool,
			map[string]any{"grant_origin": bizcg.GrantOriginAllow}, gen),
		qedge("e4", "n-cron", "n-agent-a", bizcg.EdgeTypeRuns, map[string]any{}, gen),
		qedge("e5", "n-tool", "n-team", bizcg.EdgeTypeHookRef, map[string]any{}, gen),
		qedge("e6", "n-agent-a", "", bizcg.EdgeTypeGrantedTool,
			map[string]any{"broken": true, "dst_key": "shell"}, gen),
		qedge("e7", "n-agent-b", "", bizcg.EdgeTypeGrantedTool,
			map[string]any{"broken": true, "dst_key": "ghost"}, gen),
	}
	if err := r.UpsertEdges(ctx, edges); err != nil {
		t.Fatalf("UpsertEdges: %v", err)
	}
}

// walkTuple 压平 WalkRow 便于断言。
type walkTuple struct {
	node  string
	depth int
	via   string
}

func flattenWalk(rows []bizcg.WalkRow) []walkTuple {
	out := make([]walkTuple, 0, len(rows))
	for _, r := range rows {
		via := ""
		for i, v := range r.Via {
			if i > 0 {
				via += ">"
			}
			via += v
		}
		out = append(out, walkTuple{node: r.Node.ID, depth: r.Depth, via: via})
	}
	return out
}

func hasTuple(t *testing.T, rows []bizcg.WalkRow, node string, depth int, via string) {
	t.Helper()
	for _, tt := range flattenWalk(rows) {
		if tt.node == node && tt.depth == depth && tt.via == via {
			return
		}
	}
	t.Errorf("missing walk row (%s depth=%d via=%s); got %v", node, depth, via, flattenWalk(rows))
}

func TestRepo_FindNodeDualResolution(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()
	const gen = 7

	// ref_id 命中。
	n, err := r.FindNode(ctx, gen, bizcg.NodeTypeTool, "uuid-tool")
	if err != nil || n.ID != "n-tool" {
		t.Fatalf("by ref_id: n=%+v err=%v", n, err)
	}
	// node_key 命中。
	n, err = r.FindNode(ctx, gen, bizcg.NodeTypeTool, "shell")
	if err != nil || n.ID != "n-tool" {
		t.Fatalf("by node_key: n=%+v err=%v", n, err)
	}
	// 碰撞：ref_id=shared 必须赢过 node_key=shared。
	n, err = r.FindNode(ctx, gen, bizcg.NodeTypeTool, "shared")
	if err != nil || n.ID != "n-c1" {
		t.Fatalf("ref wins on collision: n=%+v err=%v", n, err)
	}
	// 跨代隔离：gen 8 查不到 gen 7 节点。
	if _, err = r.FindNode(ctx, 8, bizcg.NodeTypeTool, "uuid-tool"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("gen isolation: err=%v, want NotFound", err)
	}
	// 不存在 → CodeNotFound。
	if _, err = r.FindNode(ctx, gen, bizcg.NodeTypeTool, "nonexistent"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("not found: err=%v, want NotFound", err)
	}
	// 类型不匹配（同 key 不同 type）→ NotFound。
	if _, err = r.FindNode(ctx, gen, bizcg.NodeTypeAgent, "shell"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("type mismatch: err=%v, want NotFound", err)
	}
}

func TestRepo_WalkGraphImpact(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	rows, err := r.WalkGraph(ctx, 7, "n-tool", true, 3)
	if err != nil {
		t.Fatalf("WalkGraph: %v", err)
	}
	// 闭包：agent-a(d1)、team(d1 直达 + d2 经 agent-a 多路径)、cron(d2)。
	hasTuple(t, rows, "n-agent-a", 1, "granted_tool")
	hasTuple(t, rows, "n-team", 1, "granted_tool")
	hasTuple(t, rows, "n-team", 2, "granted_tool>has_member")
	hasTuple(t, rows, "n-cron", 2, "granted_tool>runs")
	// 环截断：n-tool→n-team 的 e5 永远不会带出新行（n-tool 在 path 中）——
	// 总行数恒为 4，无 n-tool 节点行。
	if len(rows) != 4 {
		t.Errorf("rows = %d, want 4（环截断）: %v", len(rows), flattenWalk(rows))
	}
	for _, tt := range flattenWalk(rows) {
		if tt.node == "n-tool" {
			t.Errorf("cycle leaked: start node reached %v", flattenWalk(rows))
		}
	}
	// broken 边（e6/e7）不进闭包。
	for _, rr := range rows {
		if rr.Edge.ID == "e6" || rr.Edge.ID == "e7" {
			t.Errorf("broken edge entered walk: %+v", rr.Edge)
		}
	}
	// 跨代隔离。
	rows, err = r.WalkGraph(ctx, 8, "n-tool", true, 3)
	if err != nil || len(rows) != 0 {
		t.Errorf("gen 8: rows=%v err=%v, want empty", flattenWalk(rows), err)
	}
}

func TestRepo_WalkGraphImpactDepthLimit(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	rows, err := r.WalkGraph(ctx, 7, "n-tool", true, 1)
	if err != nil {
		t.Fatalf("WalkGraph: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("depth=1 rows = %d, want 2: %v", len(rows), flattenWalk(rows))
	}
	for _, rr := range rows {
		if rr.Depth != 1 {
			t.Errorf("depth leak: %+v", rr)
		}
	}
}

func TestRepo_WalkGraphDependencies(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	rows, err := r.WalkGraph(ctx, 7, "n-agent-a", false, 3)
	if err != nil {
		t.Fatalf("WalkGraph: %v", err)
	}
	// 正向：tool(d1)；team(d2 经 tool→hook_ref)。环回 agent-a/tool 被截断。
	hasTuple(t, rows, "n-tool", 1, "granted_tool")
	hasTuple(t, rows, "n-team", 2, "granted_tool>hook_ref")
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2: %v", len(rows), flattenWalk(rows))
	}
}

func TestRepo_ListNodeEdges(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	out, in, broken, err := r.ListNodeEdges(ctx, 7, "n-agent-a")
	if err != nil {
		t.Fatalf("ListNodeEdges: %v", err)
	}
	// out：e1（broken e6 归入 broken 段）。
	if len(out) != 1 || out[0].ID != "e1" {
		t.Errorf("out = %+v, want [e1]", out)
	}
	// in：e2 has_member、e4 runs（按 edge_type 排序）。
	if len(in) != 2 || in[0].ID != "e2" || in[1].ID != "e4" {
		t.Errorf("in = %+v, want [e2 e4]", in)
	}
	// broken：e6。
	if len(broken) != 1 || broken[0].ID != "e6" || !broken[0].Broken() {
		t.Errorf("broken = %+v, want [e6]", broken)
	}
	// evidence 保留（可解释性）。
	if broken[0].Evidence["dst_key"] != "shell" {
		t.Errorf("broken evidence = %+v", broken[0].Evidence)
	}
}

func TestRepo_ListBrokenEdgesTargeting(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	// dst_key=shell → e6。
	rows, err := r.ListBrokenEdgesTargeting(ctx, 7, []string{"shell"})
	if err != nil || len(rows) != 1 || rows[0].ID != "e6" {
		t.Errorf("shell: rows=%+v err=%v, want [e6]", rows, err)
	}
	// 多 key 命中两条（e6/e7）。
	rows, err = r.ListBrokenEdgesTargeting(ctx, 7, []string{"shell", "ghost"})
	if err != nil || len(rows) != 2 {
		t.Errorf("shell+ghost: rows=%d err=%v, want 2", len(rows), err)
	}
	// 不匹配（dst_key 是 shell 而非 uuid-tool）。
	rows, err = r.ListBrokenEdgesTargeting(ctx, 7, []string{"uuid-tool"})
	if err != nil || len(rows) != 0 {
		t.Errorf("uuid-tool: rows=%d err=%v, want 0", len(rows), err)
	}
	// 空 keys → nil。
	rows, err = r.ListBrokenEdgesTargeting(ctx, 7, nil)
	if err != nil || rows != nil {
		t.Errorf("nil keys: rows=%v err=%v, want nil", rows, err)
	}
}

// TestRepo_ListAll 全量扫描（health 分析输入）：代际过滤、断边/evidence
// 保留、无行截断。
func TestRepo_ListAll(t *testing.T) {
	r, _ := pgQueryRepo(t)
	seedQueryGraph(t, r)
	ctx := context.Background()

	edges, err := r.ListAllEdges(ctx, 7)
	if err != nil {
		t.Fatalf("ListAllEdges: %v", err)
	}
	// 7 条边全量返回（含 broken e6/e7；无 walkRowLimit 截断）。
	if len(edges) != 7 {
		t.Fatalf("edges = %d, want 7", len(edges))
	}
	// 排序：src_id → edge_type → dst_id（首行 src=n-agent-a）。
	if edges[0].SrcID != "n-agent-a" {
		t.Errorf("edges[0].SrcID = %s, want n-agent-a", edges[0].SrcID)
	}
	// broken 边 evidence 保留（health 分组输入）。
	var sawBroken bool
	for _, e := range edges {
		if e.ID == "e6" {
			sawBroken = true
			if !e.Broken() || e.Evidence["dst_key"] != "shell" {
				t.Errorf("e6 evidence = %+v", e.Evidence)
			}
		}
	}
	if !sawBroken {
		t.Error("broken edge e6 missing from ListAllEdges")
	}
	// 跨代隔离。
	edges, err = r.ListAllEdges(ctx, 8)
	if err != nil || len(edges) != 0 {
		t.Errorf("gen 8 edges = %d err=%v, want 0", len(edges), err)
	}

	nodes, err := r.ListAllNodes(ctx, 7)
	if err != nil {
		t.Fatalf("ListAllNodes: %v", err)
	}
	// 7 个节点（5 主图 + 2 双解碰撞），attrs 解析保留。
	if len(nodes) != 7 {
		t.Fatalf("nodes = %d, want 7", len(nodes))
	}
	for _, n := range nodes {
		if n.Attrs["kind"] != "test" {
			t.Errorf("node %s attrs = %+v", n.ID, n.Attrs)
		}
	}
	// 排序：node_type → node_key（首行 agent/agent-a）。
	if nodes[0].NodeType != bizcg.NodeTypeAgent || nodes[0].NodeKey != "agent-a" {
		t.Errorf("nodes[0] = %s/%s", nodes[0].NodeType, nodes[0].NodeKey)
	}
	nodes, err = r.ListAllNodes(ctx, 8)
	if err != nil || len(nodes) != 0 {
		t.Errorf("gen 8 nodes = %d err=%v, want 0", len(nodes), err)
	}
}

func TestRepo_CountActiveSessions(t *testing.T) {
	r, db := pgQueryRepo(t)
	ctx := context.Background()
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("exec failed: %v\nSQL: %s", err, q)
		}
	}
	mustExec(`INSERT INTO sessions VALUES
	  ('s1','uuid-agent-a','','running',''),
	  ('s2','','uuid-team','idle',''),
	  ('s3','uuid-agent-a','','archived',''),
	  ('s4','uuid-agent-a','','running','gone'),
	  ('s5','other','','running','')`)

	// agent+team 命中 s1/s2；archived(s3)/已删(s4)/无关(s5) 不计。
	n, err := r.CountActiveSessions(ctx, []string{"uuid-agent-a"}, []string{"uuid-team"})
	if err != nil || n != 2 {
		t.Errorf("count = %d err=%v, want 2", n, err)
	}
	// 仅 agent。
	n, err = r.CountActiveSessions(ctx, []string{"uuid-agent-a"}, nil)
	if err != nil || n != 1 {
		t.Errorf("agent only = %d err=%v, want 1", n, err)
	}
	// 双空短路与零命中。
	n, err = r.CountActiveSessions(ctx, nil, nil)
	if err != nil || n != 0 {
		t.Errorf("empty = %d err=%v, want 0", n, err)
	}
	n, err = r.CountActiveSessions(ctx, []string{"nobody"}, nil)
	if err != nil || n != 0 {
		t.Errorf("no match = %d err=%v, want 0", n, err)
	}
}
