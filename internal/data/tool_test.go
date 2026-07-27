package data

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newToolTestRepo opens a schema-isolated Postgres backed Data instance with
// the tools and tool_invocations schemas created (via Ent auto-migration),
// ready for toolRepo tests.
func newToolTestRepo(t *testing.T) (biz.ToolRepo, *Data) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &Data{
		entClient: client,
		rw:        NewReadWriteClient(client, client),
		rawDB:     db,
		readDB:    db,
		rwDB:      NewReadWriteDB(db, db),
		lg:        loggateway.NewNoop(),
		dialect:   DialectPostgres,
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
			VALUES ($1, $2, $3, $4, $5, $6, 'adk', $7)`,
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

// TestToolRepo_SearchTools_LatestInvocationDedup is a regression test for
// duplicate tool rows produced by toolSelectSQL's `last` subquery: when a tool
// has multiple invocations sharing the same MAX(started_at), the inner join
// yields one row per tied invocation, and the outer LEFT JOIN then duplicates
// the tool row (observed in Agent settings → 工具覆盖 table: same tool_key
// listed twice). The `last` subquery must collapse ties to a single row per
// tool_key.
func TestToolRepo_SearchTools_LatestInvocationDedup(t *testing.T) {
	ctx := context.Background()
	repo, d := newToolTestRepo(t)

	tool, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key:         "cli_admin_agent_get",
		DisplayName: "Agent 详情",
		Category:    "cli_admin",
		Source:      "builtin",
		RiskLevel:   "low",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("CreateTool: %v", err)
	}

	// Three invocations sharing the exact same started_at — all tie for MAX.
	cutoff := "2026-07-27T00:00:00Z"
	for i := 0; i < 3; i++ {
		if _, err := d.rawDB.ExecContext(ctx, `INSERT INTO tool_invocations
			(id, tool_key, tool_id, status, started_at, duration_ms, source, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'adk', $7)`,
			fmt.Sprintf("%s-inv-%d", tool.ID, i), tool.Key, tool.ID, "success", cutoff, 100+i*10, cutoff); err != nil {
			t.Fatalf("insert invocation %d: %v", i, err)
		}
	}

	result, err := repo.SearchTools(ctx, biz.ToolListQuery{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("SearchTools failed: %v", err)
	}
	count := 0
	for i := range result.Items {
		if result.Items[i].Key == "cli_admin_agent_get" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("tool row count = %d, want 1 (tied latest invocations must not duplicate tool rows)", count)
	}
}

// TestToolRepo_SearchTools_EmptyInvocations verifies SearchTools works when
// tool_invocations is empty (p95 LEFT JOIN yields NULL → COALESCE 0).
func TestToolRepo_SearchTools_EmptyInvocations(t *testing.T) {
	ctx := context.Background()
	repo, _ := newToolTestRepo(t)

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
