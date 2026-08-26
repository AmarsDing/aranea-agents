package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newDecisionQueryTestRepos builds write+query repos over an isolated PG
// schema with the real M80 migration DDL applied (Phase 1.7 integration).
func newDecisionQueryTestRepos(t *testing.T) (decision.Repo, decision.QueryRepo) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	for _, f := range []string{
		"sql/migrations/20261250_decision_records.sql",
		"sql/migrations/20261251_decision_record_outbox.sql",
		"sql/migrations/20261252_decision_records_idx.sql",
	} {
		if err := executeSQLFileWithDialect(ctx, db, f, DialectPostgres, lg); err != nil {
			t.Fatalf("migrate %s: %v", f, err)
		}
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: lg, dialect: DialectPostgres}
	wr := NewDecisionRepo(d, lg)
	qr := NewDecisionQueryRepoFromData(d)
	if wr == nil || qr == nil {
		t.Fatal("repo constructors returned nil over live DB")
	}
	return wr, qr
}

func seedDecisionRecords(t *testing.T, wr decision.Repo) {
	t.Helper()
	conf := 0.9
	recs := []decision.Record{
		{
			DecisionKey: "dk-q-1", Category: decision.CategoryHITLApproval,
			Scenario: "高危工具 gns3_fault_inject 待审批", Reasoning: "审批人备注：同意",
			Outcome: "approved", Confidence: &conf,
			ActorType: decision.ActorHuman, ActorKey: "user-admin",
			RelatedEntities: []decision.EntityRef{{Type: "tool", Key: "gns3_fault_inject"}},
			SourceRef:       decision.SourceRef{RunID: "run-1", ToolInvocationID: "tc-1"},
			Metadata:        map[string]any{"decision_reason": "policy_danger"},
			CreatedAt:       "2026-08-26T01:00:00Z", UpdatedAt: "2026-08-26T01:00:00Z",
		},
		{
			DecisionKey: "dk-q-2", Category: decision.CategoryPlannerOrchestration,
			Scenario: "策略路由: dag/dag", Reasoning: "LLM 指定 dag 模式",
			Outcome:   "selected_dag",
			ActorType: decision.ActorSystem, ActorKey: "system:task_planner",
			RelatedEntities: []decision.EntityRef{},
			SourceRef:       decision.SourceRef{RunID: "run-1", FlowTraceID: "tr-1"},
			CreatedAt:       "2026-08-26T02:00:00Z", UpdatedAt: "2026-08-26T02:00:00Z",
		},
		{
			DecisionKey: "dk-q-3", Category: decision.CategorySystemGuard,
			Scenario: "run 累计 input token 超预算", Reasoning: "run 累计 input 超 150 万",
			Outcome:   "tripped",
			ActorType: decision.ActorSystem, ActorKey: "system:token_budget",
			RelatedEntities: []decision.EntityRef{{Type: "team", Key: "team-9"}},
			SourceRef:       decision.SourceRef{RunID: "run-2"},
			Metadata:        map[string]any{"trigger_rule": "token_budget_tripped"},
			CreatedAt:       "2026-08-26T03:00:00Z", UpdatedAt: "2026-08-26T03:00:00Z",
		},
	}
	if err := wr.InsertRecords(context.Background(), recs); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
}

// TestDecisionQueryRepo_ListFilters_PG covers the Phase 1.7 query contract:
// pagination + total, category/actor/entity/run/time filters, and the
// ORDER BY created_at DESC stability — all evaluated DB-side (no full scans).
func TestDecisionQueryRepo_ListFilters_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	seedDecisionRecords(t, wr)

	// 全量分页：total=3，按 created_at DESC 排序。
	items, total, err := qr.ListRecords(ctx, decision.ListFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("page1: total=%d len=%d, want 3/2", total, len(items))
	}
	if items[0].DecisionKey != "dk-q-3" || items[1].DecisionKey != "dk-q-2" {
		t.Fatalf("order not created_at DESC: %s, %s", items[0].DecisionKey, items[1].DecisionKey)
	}
	items, total, err = qr.ListRecords(ctx, decision.ListFilter{Page: 2, PageSize: 2})
	if err != nil || total != 3 || len(items) != 1 || items[0].DecisionKey != "dk-q-1" {
		t.Fatalf("page2: total=%d len=%d err=%v", total, len(items), err)
	}

	// category 过滤。
	items, total, err = qr.ListRecords(ctx, decision.ListFilter{Category: "system_guard", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].DecisionKey != "dk-q-3" {
		t.Fatalf("category filter: total=%d len=%d err=%v", total, len(items), err)
	}

	// actor_key 过滤。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{ActorKey: "system:task_planner", Page: 1, PageSize: 10})
	if err != nil || total != 1 {
		t.Fatalf("actor filter: total=%d err=%v", total, err)
	}

	// 实体过滤（jsonb 数组内对象成对匹配）。
	items, total, err = qr.ListRecords(ctx, decision.ListFilter{EntityType: "tool", EntityKey: "gns3_fault_inject", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].DecisionKey != "dk-q-1" {
		t.Fatalf("entity filter hit: total=%d len=%d err=%v", total, len(items), err)
	}
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{EntityType: "tool", EntityKey: "nonexistent", Page: 1, PageSize: 10})
	if err != nil || total != 0 {
		t.Fatalf("entity filter miss: total=%d err=%v", total, err)
	}

	// source_run_id 过滤（source_ref.run_id 表达式索引路径）。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{SourceRunID: "run-1", Page: 1, PageSize: 10})
	if err != nil || total != 2 {
		t.Fatalf("run filter: total=%d err=%v", total, err)
	}

	// 时间窗过滤（含端点）。
	from, _ := time.Parse(time.RFC3339, "2026-08-26T02:00:00Z")
	to, _ := time.Parse(time.RFC3339, "2026-08-26T03:00:00Z")
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{TimeFrom: from, TimeTo: to, Page: 1, PageSize: 10})
	if err != nil || total != 2 {
		t.Fatalf("time window: total=%d err=%v", total, err)
	}

	// 组合过滤：category + run。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{Category: "planner_orchestration", SourceRunID: "run-1", Page: 1, PageSize: 10})
	if err != nil || total != 1 {
		t.Fatalf("combined filter: total=%d err=%v", total, err)
	}
}

// TestDecisionQueryRepo_GetByKey_PG covers the single-record path including
// JSON column decode and the not-found nil contract.
func TestDecisionQueryRepo_GetByKey_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	seedDecisionRecords(t, wr)

	rec, err := qr.GetByKey(ctx, "dk-q-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec == nil {
		t.Fatal("get dk-q-1 returned nil")
	}
	if rec.Category != decision.CategoryHITLApproval || rec.Outcome != "approved" {
		t.Fatalf("roundtrip mismatch: %+v", rec)
	}
	if rec.Confidence == nil || *rec.Confidence != 0.9 {
		t.Fatalf("confidence = %v", rec.Confidence)
	}
	if len(rec.RelatedEntities) != 1 || rec.RelatedEntities[0].Key != "gns3_fault_inject" {
		t.Fatalf("entities = %+v", rec.RelatedEntities)
	}
	if rec.SourceRef.ToolInvocationID != "tc-1" || rec.SourceRef.RunID != "run-1" {
		t.Fatalf("source_ref = %+v", rec.SourceRef)
	}
	if rec.Metadata["decision_reason"] != "policy_danger" {
		t.Fatalf("metadata = %+v", rec.Metadata)
	}

	// 未命中 → nil, nil。
	rec, err = qr.GetByKey(ctx, "dk-nonexistent")
	if err != nil || rec != nil {
		t.Fatalf("miss: rec=%v err=%v, want nil/nil", rec, err)
	}
}
