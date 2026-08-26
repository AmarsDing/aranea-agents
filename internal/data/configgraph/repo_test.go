package configgraph

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/data"

	_ "github.com/lib/pq"
)

// pgRepo opens a Postgres connection for repo integration tests, or skips when
// ARANEA_TEST_PG_DSN is unset/unreachable (dialect_integration_test.go pattern).
//
// TEMP tables shadow the real config_graph_* names inside the single session
// (MaxOpenConns=1), so tests never touch dev data and auto-clean on disconnect.
//
//	ARANEA_TEST_PG_DSN="postgres://user:pass@127.0.0.1:5432/db?sslmode=disable" \
//	  go test ./internal/data/configgraph/ -count=1
func pgRepo(t *testing.T) bizcg.Repo {
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
	// Column layout mirrors sql/migrations/20261260_config_graph.sql as amended by
	// migration 20261261 (composite PK (id, generation) — same id must coexist
	// across generations for dual-generation switchover).
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

	return NewRepoFromRWDB(data.NewReadWriteDB(db, db), data.DialectPostgres, nil)
}

func ts() time.Time { return time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC) }

func node(id, typ, ref, key string, gen int64) bizcg.Node {
	return bizcg.Node{
		ID: id, NodeType: typ, RefID: ref, NodeKey: key,
		DisplayName: key + " display", Status: bizcg.NodeStatusActive,
		Attrs: map[string]any{"kind": "test"}, Generation: gen,
		CreatedAt: ts(), UpdatedAt: ts(),
	}
}

func TestRepo_NodeUpsertAndMaxGeneration(t *testing.T) {
	r := pgRepo(t)
	ctx := context.Background()

	err := r.UpsertNodes(ctx, []bizcg.Node{
		node("n1", bizcg.NodeTypeAgent, "uuid-a1", "ops_master", 1),
		node("n2", bizcg.NodeTypeTool, "uuid-t1", "shell_exec", 1),
		node("n3", bizcg.NodeTypeAgent, "uuid-a1", "ops_master", 2),
		{}, // invalid — skipped
	})
	if err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}

	gen, err := r.MaxGeneration(ctx)
	if err != nil || gen != 2 {
		t.Fatalf("MaxGeneration=%d err=%v, want 2", gen, err)
	}

	// Upsert same (type,ref,gen) with changed fields → DO UPDATE refresh,
	// created_at preserved.
	updated := node("n1-replace", bizcg.NodeTypeAgent, "uuid-a1", "ops_master_v2", 1)
	updated.DisplayName = "renamed"
	updated.Attrs = map[string]any{"kind": "test", "status": "paused"}
	if err := r.UpsertNodes(ctx, []bizcg.Node{updated}); err != nil {
		t.Fatalf("UpsertNodes update: %v", err)
	}
	got, err := r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, NodeType: bizcg.NodeTypeAgent})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("gen1 agent rows=%d, want 1 (upsert must not duplicate)", len(got))
	}
	if got[0].NodeKey != "ops_master_v2" || got[0].DisplayName != "renamed" {
		t.Fatalf("DO UPDATE did not refresh: %+v", got[0])
	}
	if got[0].Attrs["status"] != "paused" {
		t.Fatalf("attrs not refreshed: %+v", got[0].Attrs)
	}
	if !got[0].CreatedAt.Equal(ts()) {
		t.Fatalf("created_at must be preserved on conflict, got %v", got[0].CreatedAt)
	}
}

func TestRepo_EdgeUpsertBrokenAndCounts(t *testing.T) {
	r := pgRepo(t)
	ctx := context.Background()

	if err := r.UpsertNodes(ctx, []bizcg.Node{
		node("n1", bizcg.NodeTypeAgent, "uuid-a1", "ops_master", 1),
		node("n2", bizcg.NodeTypeTool, "uuid-t1", "shell_exec", 1),
	}); err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}

	edges := []bizcg.StoredEdge{
		{ID: "e1", SrcID: "n1", DstID: "n2", Type: bizcg.EdgeTypeGrantedTool,
			Evidence: map[string]any{"grant_origin": bizcg.GrantOriginAllow}, Generation: 1, CreatedAt: ts()},
		{ID: "e2", SrcID: "n1", DstID: "", Type: bizcg.EdgeTypeBoundPositionKey,
			Evidence: map[string]any{"broken": true, "dst_key": "ghost_position"}, Generation: 1, CreatedAt: ts()},
	}
	if err := r.UpsertEdges(ctx, edges); err != nil {
		t.Fatalf("UpsertEdges: %v", err)
	}

	c, err := r.Counts(ctx, 1)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if c.Nodes != 2 || c.Edges != 2 || c.Broken != 1 {
		t.Fatalf("Counts=%+v, want {2 2 1}", c)
	}

	// Same edge identity with new evidence → DO UPDATE, not duplicate.
	edges[0].Evidence = map[string]any{"grant_origin": bizcg.GrantOriginOverride}
	if err := r.UpsertEdges(ctx, edges[:1]); err != nil {
		t.Fatalf("UpsertEdges update: %v", err)
	}
	c, err = r.Counts(ctx, 1)
	if err != nil || c.Edges != 2 {
		t.Fatalf("after upsert-update Counts=%+v err=%v, want edges=2", c, err)
	}

	// DeleteOutEdges removes only this src's rows in this generation.
	if err := r.DeleteOutEdges(ctx, "n1", 1); err != nil {
		t.Fatalf("DeleteOutEdges: %v", err)
	}
	c, _ = r.Counts(ctx, 1)
	if c.Edges != 0 {
		t.Fatalf("after DeleteOutEdges edges=%d, want 0", c.Edges)
	}
}

// TestRepo_SameIDCoexistsAcrossGenerations 是 20261261 复合主键的回归测试：
// 双代切换期间同一确定性 id 必须能在相邻两代共存（原 PRIMARY KEY(id) 下第二
// 次全量重建必现 23505）。
func TestRepo_SameIDCoexistsAcrossGenerations(t *testing.T) {
	r := pgRepo(t)
	ctx := context.Background()

	for gen := int64(1); gen <= 2; gen++ {
		if err := r.UpsertNodes(ctx, []bizcg.Node{
			node("n1", bizcg.NodeTypeAgent, "uuid-a1", "ops_master", gen),
		}); err != nil {
			t.Fatalf("UpsertNodes gen=%d (same id): %v", gen, err)
		}
		if err := r.UpsertEdges(ctx, []bizcg.StoredEdge{
			{ID: "e1", SrcID: "n1", DstID: "", Type: bizcg.EdgeTypeBoundPositionKey,
				Evidence: map[string]any{"broken": true}, Generation: gen, CreatedAt: ts()},
		}); err != nil {
			t.Fatalf("UpsertEdges gen=%d (same id): %v", gen, err)
		}
	}
	gen, err := r.MaxGeneration(ctx)
	if err != nil || gen != 2 {
		t.Fatalf("MaxGeneration=%d err=%v, want 2", gen, err)
	}
	c1, err := r.Counts(ctx, 1)
	if err != nil || c1.Nodes != 1 || c1.Edges != 1 {
		t.Fatalf("gen1 Counts=%+v err=%v, want {1 1 _}", c1, err)
	}
	c2, err := r.Counts(ctx, 2)
	if err != nil || c2.Nodes != 1 || c2.Edges != 1 {
		t.Fatalf("gen2 Counts=%+v err=%v, want {1 1 _}", c2, err)
	}
}

func TestRepo_DeleteGenerationBelow(t *testing.T) {
	r := pgRepo(t)
	ctx := context.Background()

	for gen := int64(1); gen <= 3; gen++ {
		if err := r.UpsertNodes(ctx, []bizcg.Node{node(
			"n"+string(rune('0'+gen)), bizcg.NodeTypeTool, "uuid-t1", "shell_exec", gen)}); err != nil {
			t.Fatalf("UpsertNodes gen=%d: %v", gen, err)
		}
		if err := r.UpsertEdges(ctx, []bizcg.StoredEdge{
			{ID: "e" + string(rune('0'+gen)), SrcID: "n" + string(rune('0'+gen)), DstID: "", Type: bizcg.EdgeTypeGrantedTool,
				Evidence: map[string]any{"broken": true}, Generation: gen, CreatedAt: ts()},
		}); err != nil {
			t.Fatalf("UpsertEdges gen=%d: %v", gen, err)
		}
	}

	deleted, err := r.DeleteGenerationBelow(ctx, 3)
	if err != nil {
		t.Fatalf("DeleteGenerationBelow: %v", err)
	}
	if deleted != 4 { // gens 1+2: 2 nodes + 2 edges
		t.Fatalf("deleted=%d, want 4", deleted)
	}
	c, err := r.Counts(ctx, 3)
	if err != nil || c.Nodes != 1 || c.Edges != 1 {
		t.Fatalf("gen3 Counts=%+v err=%v, want {1 1}", c, err)
	}
	gen, _ := r.MaxGeneration(ctx)
	if gen != 3 {
		t.Fatalf("MaxGeneration=%d, want 3", gen)
	}
}

func TestRepo_ListNodesFilters(t *testing.T) {
	r := pgRepo(t)
	ctx := context.Background()

	if err := r.UpsertNodes(ctx, []bizcg.Node{
		func() bizcg.Node {
			n := node("n1", bizcg.NodeTypeAgent, "u1", "ops_master", 1)
			n.WorkspaceID = "ws1"
			return n
		}(),
		func() bizcg.Node {
			n := node("n2", bizcg.NodeTypeAgent, "u2", "ops_helper_100%", 1)
			n.WorkspaceID = "ws2"
			return n
		}(),
		node("n3", bizcg.NodeTypeTool, "u3", "shell_exec", 1),
		node("n4", bizcg.NodeTypeAgent, "u4", "ops_master", 2), // other generation
	}); err != nil {
		t.Fatalf("UpsertNodes: %v", err)
	}

	// generation scoping
	got, err := r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1})
	if err != nil || len(got) != 3 {
		t.Fatalf("gen1 rows=%d err=%v, want 3", len(got), err)
	}
	// type filter
	got, _ = r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, NodeType: bizcg.NodeTypeTool})
	if len(got) != 1 || got[0].NodeKey != "shell_exec" {
		t.Fatalf("type filter wrong: %+v", got)
	}
	// key substring
	got, _ = r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, KeyContains: "ops_"})
	if len(got) != 2 {
		t.Fatalf("key substring rows=%d, want 2", len(got))
	}
	// LIKE wildcard escaping: '%' must match literally, not as wildcard
	got, _ = r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, KeyContains: "100%"})
	if len(got) != 1 || got[0].NodeKey != "ops_helper_100%" {
		t.Fatalf("LIKE escape wrong: %+v", got)
	}
	// workspace filter
	got, _ = r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, WorkspaceID: "ws1"})
	if len(got) != 1 || got[0].ID != "n1" {
		t.Fatalf("workspace filter wrong: %+v", got)
	}
	// limit
	got, _ = r.ListNodes(ctx, bizcg.NodeFilter{Generation: 1, Limit: 2})
	if len(got) != 2 {
		t.Fatalf("limit rows=%d, want 2", len(got))
	}
}
