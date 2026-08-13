package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// TestToolRepo_ToolAgentOverride_BoolScanRegression is a regression test for
// scanToolAgentOverrides scanning Postgres boolean columns into int variables.
// With any override row present, every override list query failed with:
//
//	sql: Scan error on column index 4: converting driver.Value type bool ("true") to a int
//
// surfacing as a 500 on both the agent override list API and the build-time
// confirm-gate snapshot (fail-closed: all tools forced to require confirmation).
// The test upserts an override and exercises both list paths.
func TestToolRepo_ToolAgentOverride_BoolScanRegression(t *testing.T) {
	ctx := context.Background()
	repo, _ := newToolTestRepo(t)

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

	saved, err := repo.UpsertToolAgentOverride(ctx, biz.ToolAgentOverrideInput{
		ToolKey:              tool.Key,
		AgentID:              "agent_1",
		Enabled:              true,
		Mode:                 "allow",
		ConfigOverrideJSON:   `{"timeout": 5}`,
		RequiresConfirmation: true,
	}, tool.ID)
	if err != nil {
		t.Fatalf("UpsertToolAgentOverride: %v", err)
	}
	if !saved.Enabled || !saved.RequiresConfirmation || saved.Mode != "allow" {
		t.Fatalf("upsert round-trip mismatch: %+v", saved)
	}

	byAgent, err := repo.ListToolAgentOverridesByAgent(ctx, "agent_1")
	if err != nil {
		t.Fatalf("ListToolAgentOverridesByAgent: %v", err)
	}
	if len(byAgent) != 1 || !byAgent[0].Enabled || !byAgent[0].RequiresConfirmation {
		t.Fatalf("by-agent list mismatch: %+v", byAgent)
	}

	byTool, err := repo.ListToolAgentOverrides(ctx, tool.Key)
	if err != nil {
		t.Fatalf("ListToolAgentOverrides: %v", err)
	}
	if len(byTool) != 1 || byTool[0].AgentID != "agent_1" {
		t.Fatalf("by-tool list mismatch: %+v", byTool)
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

// TestToolRepo_ListToolCatalogEntries_Batch verifies the lightweight batch
// catalog query used at agent-build time: a single IN query over the tools
// table returning only (key, config_json, default_config_json,
// requires_confirmation), excluding soft-deleted rows and unknown keys.
// This replaces the previous per-key GetTool monster-SQL loop (N+1).
func TestToolRepo_ListToolCatalogEntries_Batch(t *testing.T) {
	ctx := context.Background()
	repo, _ := newToolTestRepo(t)

	if _, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key: "cat_a", DisplayName: "Cat A", Category: "c", Source: "builtin", RiskLevel: "low", Enabled: true,
		ConfigJSON: `{"ua":"x"}`, DefaultConfigJSON: `{"ua":"d"}`, RequiresConfirmation: true,
	}); err != nil {
		t.Fatalf("CreateTool cat_a: %v", err)
	}
	if _, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key: "cat_b", DisplayName: "Cat B", Category: "c", Source: "builtin", RiskLevel: "low", Enabled: true,
		DefaultConfigJSON: `{"max":10}`,
	}); err != nil {
		t.Fatalf("CreateTool cat_b: %v", err)
	}
	deleted, err := repo.CreateTool(ctx, biz.ToolUpsertInput{
		Key: "cat_c", DisplayName: "Cat C", Category: "c", Source: "builtin", RiskLevel: "low", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateTool cat_c: %v", err)
	}
	if err := repo.DeleteTool(ctx, deleted.ID); err != nil {
		t.Fatalf("DeleteTool cat_c: %v", err)
	}

	entries, err := repo.ListToolCatalogEntries(ctx, []string{"cat_a", "cat_b", "cat_c", "cat_missing", "cat_a"})
	if err != nil {
		t.Fatalf("ListToolCatalogEntries: %v", err)
	}
	byKey := make(map[string]biz.ToolCatalogEntry, len(entries))
	for _, e := range entries {
		if _, dup := byKey[e.Key]; dup {
			t.Fatalf("duplicate row for key %q", e.Key)
		}
		byKey[e.Key] = e
	}
	if len(byKey) != 2 {
		t.Fatalf("entries = %d, want 2 (deleted+missing excluded): %+v", len(byKey), entries)
	}
	if a := byKey["cat_a"]; a.ConfigJSON != `{"ua":"x"}` || a.DefaultConfigJSON != `{"ua":"d"}` || !a.RequiresConfirmation {
		t.Errorf("cat_a = %+v", a)
	}
	// CreateTool normalizes empty config_json to "{}", matching what GetTool
	// would have returned to the old per-key loop.
	if b := byKey["cat_b"]; b.ConfigJSON != "{}" || b.DefaultConfigJSON != `{"max":10}` || b.RequiresConfirmation {
		t.Errorf("cat_b = %+v", b)
	}
}

// TestToolRepo_ListToolCatalogEntries_EmptyKeys verifies the empty-input
// short-circuit: no query is executed and no error is returned.
func TestToolRepo_ListToolCatalogEntries_EmptyKeys(t *testing.T) {
	repo, _ := newToolTestRepo(t)
	for _, keys := range [][]string{nil, {}, {"  ", ""}} {
		entries, err := repo.ListToolCatalogEntries(context.Background(), keys)
		if err != nil {
			t.Fatalf("keys=%q err=%v", keys, err)
		}
		if len(entries) != 0 {
			t.Fatalf("keys=%q entries=%+v, want empty", keys, entries)
		}
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

// TestToolWhereClause_Abnormal covers the "仅看异常" filter: when Abnormal is
// set, the WHERE clause must restrict to tools whose latest invocation ended
// error/blocked; the condition must reference only `t` + tool_invocations so
// it works in both the COUNT query (no joins) and the list query.
func TestToolWhereClause_Abnormal(t *testing.T) {
	where, args := toolWhereClause(biz.ToolListQuery{Abnormal: true})
	if !strings.Contains(where, "tool_invocations") || !strings.Contains(where, "'error'") || !strings.Contains(where, "'blocked'") {
		t.Fatalf("abnormal where missing latest-status condition: %s", where)
	}
	if len(args) != 0 {
		t.Fatalf("abnormal filter should be arg-free, got %v", args)
	}

	whereOff, _ := toolWhereClause(biz.ToolListQuery{})
	if strings.Contains(whereOff, "tool_invocations") {
		t.Fatalf("abnormal off must not reference tool_invocations: %s", whereOff)
	}
}

// 参数质量信号必须落进 metadata_json（args_repaired / args_invalid），
// 供 GetToolQualityStats 聚合"参数一次合法率"；空信号保持 "{}" 不污染。
func TestInvocationMetaJSON_ArgsQualityFlags(t *testing.T) {
	raw := invocationMetaJSON(biz.ToolInvocationWrite{ToolCallID: "tc1", ArgsRepaired: true, ArgsInvalid: true})
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, raw)
	}
	if m["args_repaired"] != true || m["args_invalid"] != true || m["tool_call_id"] != "tc1" {
		t.Fatalf("metadata mismatch: %v", m)
	}
	if got := invocationMetaJSON(biz.ToolInvocationWrite{}); got != "{}" {
		t.Fatalf("empty write must yield {}, got %s", got)
	}
	// 仅 repaired 单信号
	raw = invocationMetaJSON(biz.ToolInvocationWrite{ArgsRepaired: true})
	var m2 map[string]any
	if err := json.Unmarshal([]byte(raw), &m2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m2["args_repaired"] != true {
		t.Fatalf("args_repaired missing: %v", m2)
	}
	if _, ok := m2["args_invalid"]; ok {
		t.Fatalf("args_invalid must be omitted when false: %v", m2)
	}
}
