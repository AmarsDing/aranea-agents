package data

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

// newToolTestRepo opens an in-memory SQLite backed Data instance with the
// tools and tool_invocations schemas created, ready for toolRepo tests.
func newToolTestRepo(t *testing.T) (biz.ToolRepo, *Data) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("pragma fk: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background(), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("schema create: %v", err)
	}
	d := &Data{
		entClient: client,
		rw:        NewReadWriteClient(client, client),
		rawDB:     db,
		readDB:    db,
		rwDB:      NewReadWriteDB(db, db),
		lg:        loggateway.NewNoop(),
		dialect:   DialectSQLite,
	}
	return NewToolRepo(d), d
}

// TestToolRepo_SearchTools_P95Regression is a regression test for the
// Postgres-incompatible `MAX(1, ...)` scalar usage in toolSelectSQL's p95
// subquery. On Postgres, `MAX(a, b)` is not a valid scalar function (only an
// aggregate), causing:
//
//	pq: 函数 max(integer, integer) 不存在 (42883)
//
// This test exercises the SearchTools path (which includes the p95 LEFT JOIN)
// with tool_invocations rows present, ensuring the SQL is valid on both SQLite
// and Postgres (via GREATEST).
func TestToolRepo_SearchTools_P95Regression(t *testing.T) {
	ctx := context.Background()
	repo, d := newToolTestRepo(t)
	defer d.entClient.Close()

	// Seed one tool via the repo (exercises CreateTool path too).
	tool, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key:         "web_fetch",
		DisplayName: "Web Fetch",
		Category:    "network",
		Source:      "builtin",
		RiskLevel:   "low",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("CreateTool: %v", err)
	}

	// Seed tool_invocations rows via raw SQL so the p95 subquery has data to
	// aggregate. The p95 subquery uses ROW_NUMBER() OVER (... ORDER BY
	// duration_ms DESC) and filters `rn <= GREATEST(1, CAST(total_cnt*0.05 AS INTEGER))`.
	// With 20 rows, total_cnt*0.05 = 1, so GREATEST(1, 1) = 1 — top 1 row.
	cutoff := "2026-06-19T00:00:00Z"
	for i := 0; i < 20; i++ {
		if _, err := d.rawDB.ExecContext(ctx, `INSERT INTO tool_invocations
			(id, tool_key, tool_id, status, started_at, duration_ms, source, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'adk', ?)`,
			fmt.Sprintf("%s-inv-%d", tool.ID, i), tool.Key, tool.ID, "success", cutoff, 100+i*10, cutoff); err != nil {
			t.Fatalf("insert invocation %d: %v", i, err)
		}
	}

	// SearchTools must not return an error. Before the fix, this failed on
	// Postgres with "function max(integer, integer) does not exist".
	result, err := repo.SearchTools(ctx, biz.ToolListQuery{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("SearchTools failed: %v", err)
	}
	// Find our seeded tool among the results (Schema.Create may seed builtins).
	var got *biz.Tool
	for i := range result.Items {
		if result.Items[i].Key == "web_fetch" {
			got = &result.Items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("seeded tool web_fetch not found in %d results", len(result.Items))
	}
	if got.InvokeCount != 20 {
		t.Errorf("InvokeCount = %d, want 20", got.InvokeCount)
	}
	// p95 = avg of top-5% rows by duration. With 20 rows, top 5% = 1 row
	// (GREATEST(1, 1) = 1), threshold = min duration among top-1 = max duration
	// = 100 + 19*10 = 290. p95 = avg of rows with duration >= 290 = 290.
	if got.P95DurationMS != 290 {
		t.Errorf("P95DurationMS = %v, want 290", got.P95DurationMS)
	}
}

// TestToolRepo_SearchTools_EmptyInvocations verifies SearchTools works when
// tool_invocations is empty (p95 LEFT JOIN yields NULL → COALESCE 0).
func TestToolRepo_SearchTools_EmptyInvocations(t *testing.T) {
	ctx := context.Background()
	repo, d := newToolTestRepo(t)
	defer d.entClient.Close()

	if _, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key:         "read_file",
		DisplayName: "Read File",
		Category:    "filesystem",
		Source:      "builtin",
		RiskLevel:   "low",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("CreateTool: %v", err)
	}

	result, err := repo.SearchTools(ctx, biz.ToolListQuery{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("SearchTools failed: %v", err)
	}
	// Find our seeded tool among the results (Schema.Create may seed builtins).
	var found bool
	for i := range result.Items {
		if result.Items[i].Key == "read_file" {
			found = true
			if result.Items[i].P95DurationMS != 0 {
				t.Errorf("P95DurationMS = %v, want 0 (no invocations)", result.Items[i].P95DurationMS)
			}
			break
		}
	}
	if !found {
		t.Fatalf("seeded tool read_file not found in %d results", len(result.Items))
	}
}
