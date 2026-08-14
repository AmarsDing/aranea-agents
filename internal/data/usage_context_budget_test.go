package data

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// PG-backed integration test for the P2-1 context_budget cross-turn
// aggregation (29-token.development.md §18). Requires the test Postgres
// (ARANEA_TEST_PG_DSN); skips when unreachable.
func setupContextBudgetTestRepo(t *testing.T) *usageRepo {
	t.Helper()
	skipIfPGUnreachable(t)
	rawDB := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if _, err := rawDB.ExecContext(ctx, `CREATE TABLE model_token_usage_events (
	  id TEXT PRIMARY KEY,
	  occurred_at TEXT NOT NULL,
	  date_key TEXT NOT NULL,
	  workspace_id TEXT NOT NULL DEFAULT '',
	  team_id TEXT NOT NULL DEFAULT '',
	  agent_id TEXT NOT NULL DEFAULT '',
	  agent_key TEXT NOT NULL DEFAULT '',
	  provider_code TEXT NOT NULL DEFAULT '',
	  model_api_id TEXT NOT NULL DEFAULT '',
	  usage_kind TEXT NOT NULL DEFAULT '',
	  status TEXT NOT NULL DEFAULT 'success',
	  metadata_json TEXT NOT NULL DEFAULT '{}',
	  created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create model_token_usage_events: %v", err)
	}
	d := &Data{
		rawDB:   rawDB,
		readDB:  rawDB,
		rwDB:    NewReadWriteDB(rawDB, rawDB),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return &usageRepo{data: d}
}

func insertContextBudgetRow(t *testing.T, r *usageRepo, id, dateKey, agentID, agentKey, kind, metadataJSON string) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO model_token_usage_events (id, occurred_at, date_key, agent_id, agent_key, usage_kind, metadata_json, created_at)
		 VALUES ($1, $2 || 'T10:00:00Z', $2, $3, $4, $5, $6, $2 || 'T10:00:00Z')`,
		id, dateKey, agentID, agentKey, kind, metadataJSON); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func cbMetadata(estTokens string, estTotal, toolsCount int, topTools string) string {
	return fmt.Sprintf(`{"trace_id":"t","context_budget":{"est_tokens":%s,"est_total_input":%d,"tools_count":%d%s}}`,
		estTokens, estTotal, toolsCount, topTools)
}

func TestContextBudgetGrains_Aggregates(t *testing.T) {
	r := setupContextBudgetTestRepo(t)
	ctx := context.Background()

	// agent-a 同一天两轮：tools_schema 分别 2000/1000，history 只出现在第一轮。
	insertContextBudgetRow(t, r, "r1", "2026-08-12", "a1", "agent-a", "chat_turn",
		cbMetadata(`{"tools_schema":2000,"history":800}`, 3000, 12,
			`,"top_tools":[{"name":"big_tool","est_tokens":900},{"name":"small_tool","est_tokens":100}]`))
	insertContextBudgetRow(t, r, "r2", "2026-08-12", "a1", "agent-a", "chat_turn",
		cbMetadata(`{"tools_schema":1000}`, 2000, 10,
			`,"top_tools":[{"name":"big_tool","est_tokens":700}]`))
	// agent-b 次日一轮：只有 knowledge_cue。
	insertContextBudgetRow(t, r, "r3", "2026-08-13", "a2", "agent-b", "chat_turn",
		cbMetadata(`{"knowledge_cue":300}`, 1500, 8, ""))
	// 无 context_budget 的行：天然排除（不携带台账）。
	insertContextBudgetRow(t, r, "r4", "2026-08-12", "a1", "agent-a", "chat_turn", `{"trace_id":"t"}`)
	// team_turn 行：billable 子句排除。
	insertContextBudgetRow(t, r, "r5", "2026-08-12", "a1", "agent-a", "team_turn",
		cbMetadata(`{"tools_schema":99999}`, 99999, 99, ""))
	// 范围外行：date_key 过滤排除。
	insertContextBudgetRow(t, r, "r6", "2026-07-15", "a1", "agent-a", "chat_turn",
		cbMetadata(`{"tools_schema":777}`, 777, 7, ""))

	q := bizUsageQueryRange("2026-08-01", "2026-08-31")
	grains, err := r.ContextBudgetGrains(ctx, q)
	if err != nil {
		t.Fatalf("ContextBudgetGrains: %v", err)
	}
	if len(grains) != 2 {
		t.Fatalf("len(grains) = %d, want 2: %+v", len(grains), grains)
	}

	// 确定性排序：agent_id, date_key。
	g := grains[0]
	if g.AgentID != "a1" || g.AgentKey != "agent-a" || g.DateKey != "2026-08-12" {
		t.Fatalf("grain[0] key = %s/%s/%s, want a1/agent-a/2026-08-12", g.AgentID, g.AgentKey, g.DateKey)
	}
	if g.Samples != 2 {
		t.Errorf("grain[0].Samples = %d, want 2 (r4 无台账 / r5 team_turn 均排除)", g.Samples)
	}
	if g.EstTotalInputSum != 5000 || g.ToolsCountSum != 22 {
		t.Errorf("grain[0] sums = %v/%v, want 5000/22", g.EstTotalInputSum, g.ToolsCountSum)
	}
	if g.CategorySums["tools_schema"] != 3000 || g.CategorySums["history"] != 800 {
		t.Errorf("grain[0].CategorySums = %+v, want tools_schema=3000 history=800", g.CategorySums)
	}

	g2 := grains[1]
	if g2.AgentID != "a2" || g2.DateKey != "2026-08-13" {
		t.Fatalf("grain[1] key = %s/%s, want a2/2026-08-13", g2.AgentID, g2.DateKey)
	}
	if g2.Samples != 1 || g2.CategorySums["knowledge_cue"] != 300 {
		t.Errorf("grain[1] = samples %d cats %+v, want 1 / knowledge_cue=300", g2.Samples, g2.CategorySums)
	}
}

func TestContextBudgetTopTools_Aggregates(t *testing.T) {
	r := setupContextBudgetTestRepo(t)
	ctx := context.Background()

	insertContextBudgetRow(t, r, "r1", "2026-08-12", "a1", "agent-a", "chat_turn",
		cbMetadata(`{"tools_schema":2000}`, 3000, 12,
			`,"top_tools":[{"name":"big_tool","est_tokens":900},{"name":"small_tool","est_tokens":100}]`))
	insertContextBudgetRow(t, r, "r2", "2026-08-12", "a1", "agent-a", "chat_turn",
		cbMetadata(`{"tools_schema":1000}`, 2000, 10,
			`,"top_tools":[{"name":"big_tool","est_tokens":700}]`))
	// 无 top_tools 键的行：array 展开为空，不参与。
	insertContextBudgetRow(t, r, "r3", "2026-08-13", "a2", "agent-b", "chat_turn",
		cbMetadata(`{"knowledge_cue":300}`, 1500, 8, ""))

	tools, err := r.ContextBudgetTopTools(ctx, bizUsageQueryRange("2026-08-01", "2026-08-31"), 20)
	if err != nil {
		t.Fatalf("ContextBudgetTopTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2: %+v", len(tools), tools)
	}
	// avg desc：big_tool (800) 在前，small_tool (100) 在后。
	big := tools[0]
	if big.ToolName != "big_tool" || big.Appearances != 2 || big.AvgEstTokens != 800 || big.MaxEstTokens != 900 {
		t.Errorf("tools[0] = %+v, want big_tool 2/800/900", big)
	}
	small := tools[1]
	if small.ToolName != "small_tool" || small.Appearances != 1 || small.AvgEstTokens != 100 || small.MaxEstTokens != 100 {
		t.Errorf("tools[1] = %+v, want small_tool 1/100/100", small)
	}
}

func TestContextBudgetGrains_Empty(t *testing.T) {
	r := setupContextBudgetTestRepo(t)
	ctx := context.Background()
	// 只有无台账行。
	insertContextBudgetRow(t, r, "x1", "2026-08-12", "a1", "agent-a", "chat_turn", `{"trace_id":"t"}`)

	grains, err := r.ContextBudgetGrains(ctx, bizUsageQueryRange("2026-08-01", "2026-08-31"))
	if err != nil {
		t.Fatalf("ContextBudgetGrains: %v", err)
	}
	if len(grains) != 0 {
		t.Errorf("len(grains) = %d, want 0", len(grains))
	}
	tools, err := r.ContextBudgetTopTools(ctx, bizUsageQueryRange("2026-08-01", "2026-08-31"), 20)
	if err != nil {
		t.Fatalf("ContextBudgetTopTools: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("len(tools) = %d, want 0", len(tools))
	}
}
