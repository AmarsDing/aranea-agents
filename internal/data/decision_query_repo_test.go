package data

import (
	"context"
	"database/sql"
	"strings"
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
	wr, qr, _ := newDecisionQueryTestReposWithDB(t)
	return wr, qr
}

// newDecisionQueryTestReposWithDB 与 newDecisionQueryTestRepos 相同，额外
// 返回 schema 隔离的 raw *sql.DB——链虚拟父测试需要自建 teams/team_runs
// 最小镜像表（Ent 管理，不在本测试套件的 DDL 迁移清单内）。
func newDecisionQueryTestReposWithDB(t *testing.T) (decision.Repo, decision.QueryRepo, *sql.DB) {
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
	return wr, qr, db
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

// TestDecisionQueryRepo_RunGateStats_PG covers 79-runtime-governance R7：
// 六类 trigger_rule 聚合（loop_guard 计数 / budget·no_progress 布尔 /
// prune 求和 observed_value+prune_bytes / compact 计数 / param_rule_deny
// 计数——2026-08-27 三轮审查补钉），run 归属隔离，非 system_guard 类别与
// 无记录 run 返回零值。
func TestDecisionQueryRepo_RunGateStats_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	recs := []decision.Record{
		guardRec("dk-g-1", "run-s", decision.TriggerLoopGuardBlocked, map[string]any{}),
		guardRec("dk-g-2", "run-s", decision.TriggerLoopGuardBlocked, map[string]any{}),
		guardRec("dk-g-3", "run-s", decision.TriggerTokenBudgetTripped, map[string]any{}),
		guardRec("dk-g-4", "run-s", decision.TriggerNoProgressTripped, map[string]any{}),
		guardRec("dk-g-5", "run-s", decision.TriggerToolResultPruned, map[string]any{"observed_value": 3, "prune_bytes": 12000}),
		guardRec("dk-g-6", "run-s", decision.TriggerToolResultPruned, map[string]any{"observed_value": 2, "prune_bytes": 8000}),
		guardRec("dk-g-7", "run-s", decision.TriggerContextCompacted, map[string]any{}),
		// H7：param_rule_deny 计数（三轮审查补钉——此前 case 改坏测试全绿）。
		guardRec("dk-g-10", "run-s", decision.TriggerParamRuleDeny, map[string]any{}),
		guardRec("dk-g-11", "run-s", decision.TriggerParamRuleDeny, map[string]any{}),
		// 他 run 的记录：必须隔离。
		guardRec("dk-g-8", "run-other", decision.TriggerLoopGuardBlocked, map[string]any{"observed_value": 99}),
		// 非 system_guard 类别同 run：不计。
		{
			DecisionKey: "dk-g-9", Category: decision.CategoryHITLApproval,
			Scenario: "审批", Reasoning: "-", Outcome: "approved",
			ActorType: decision.ActorHuman, ActorKey: "user-1",
			RelatedEntities: []decision.EntityRef{},
			SourceRef:       decision.SourceRef{RunID: "run-s"},
			Metadata:        map[string]any{"trigger_rule": decision.TriggerLoopGuardBlocked},
			CreatedAt:       "2026-08-27T01:00:00Z", UpdatedAt: "2026-08-27T01:00:00Z",
		},
	}
	if err := wr.InsertRecords(ctx, recs); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	statsRepo, ok := qr.(decision.RunGateStatsRepo)
	if !ok {
		t.Fatal("decisionQueryRepo 未实现 RunGateStatsRepo 窄接口")
	}
	got, err := statsRepo.RunGateStats(ctx, "run-s")
	if err != nil {
		t.Fatalf("RunGateStats: %v", err)
	}
	if got.LoopGuardBlocks != 2 {
		t.Errorf("LoopGuardBlocks = %d, want 2", got.LoopGuardBlocks)
	}
	if !got.BudgetTripped || !got.NoProgressTripped {
		t.Errorf("tripped = %v/%v, want true/true", got.BudgetTripped, got.NoProgressTripped)
	}
	if got.PruneCount != 5 || got.PruneBytes != 20000 {
		t.Errorf("prune = %d 条/%d 字节, want 5/20000（observed_value+prune_bytes 求和）", got.PruneCount, got.PruneBytes)
	}
	if got.CompactCount != 1 {
		t.Errorf("CompactCount = %d, want 1", got.CompactCount)
	}
	if got.ParamRuleDenies != 2 {
		t.Errorf("ParamRuleDenies = %d, want 2（param_rule_deny 计数）", got.ParamRuleDenies)
	}

	// 他 run 只见自己的记录；无记录 run / 空 runID 返回零值, nil。
	other, err := statsRepo.RunGateStats(ctx, "run-other")
	if err != nil || other.LoopGuardBlocks != 1 || other.BudgetTripped {
		t.Errorf("run-other 隔离失败: %+v err=%v", other, err)
	}
	zero, err := statsRepo.RunGateStats(ctx, "run-missing")
	if err != nil || zero != (decision.RunGateStats{}) {
		t.Errorf("无记录 run 应零值: %+v err=%v", zero, err)
	}
	if _, err := statsRepo.RunGateStats(ctx, "  "); err != nil {
		t.Errorf("空 runID 应 nil error: %v", err)
	}
}

// guardRec 造一条 system_guard 记录（trigger_rule 等观测字段落 metadata，
// run 归属落 source_ref.run_id——与 EmitGate 写入侧契约一致）。
func guardRec(key, runID, trigger string, extraMeta map[string]any) decision.Record {
	meta := map[string]any{"trigger_rule": trigger}
	for k, v := range extraMeta {
		meta[k] = v
	}
	return decision.Record{
		DecisionKey: key, Category: decision.CategorySystemGuard,
		Scenario: "系统闸", Reasoning: "-", Outcome: "truncated",
		ActorType: decision.ActorSystem, ActorKey: "system:guard",
		RelatedEntities: []decision.EntityRef{},
		SourceRef:       decision.SourceRef{RunID: runID},
		Metadata:        meta,
		CreatedAt:       "2026-08-27T00:00:00Z", UpdatedAt: "2026-08-27T00:00:00Z",
	}
}

// TestDecisionQueryRepo_SessionGateStats_PG covers T5（2026-08-27 chat 侧闸
// 事件聚合面）：会话维度六类 trigger 聚合、新旧两口径会话归属兼容（新记录
// SourceRef.SessionID 一等公民 / 旧记录仅 metadata.session_id）、会话隔离、
// 非 system_guard 类别与无记录会话返回零值。
func TestDecisionQueryRepo_SessionGateStats_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	sessRec := func(key, trigger string, src decision.SourceRef, extraMeta map[string]any) decision.Record {
		r := guardRec(key, "run-x", trigger, extraMeta)
		r.SourceRef = src
		return r
	}
	recs := []decision.Record{
		// 新口径：source_ref.session_id。
		sessRec("dk-s-1", decision.TriggerLoopGuardBlocked, decision.SourceRef{RunID: "run-1", SessionID: "sess-a"}, map[string]any{}),
		sessRec("dk-s-2", decision.TriggerParamRuleDeny, decision.SourceRef{RunID: "run-1", SessionID: "sess-a"}, map[string]any{}),
		sessRec("dk-s-3", decision.TriggerToolResultPruned, decision.SourceRef{RunID: "run-2", SessionID: "sess-a"}, map[string]any{"observed_value": 4, "prune_bytes": 9000}),
		sessRec("dk-s-4", decision.TriggerContextCompacted, decision.SourceRef{RunID: "run-2", SessionID: "sess-a"}, map[string]any{}),
		sessRec("dk-s-5", decision.TriggerTokenBudgetTripped, decision.SourceRef{RunID: "run-3", SessionID: "sess-a"}, map[string]any{}),
		// 旧口径：仅 metadata.session_id（Extra 注入时期的存量形态）——必须命中。
		sessRec("dk-s-6", decision.TriggerLoopGuardBlocked, decision.SourceRef{RunID: "run-4"}, map[string]any{"session_id": "sess-a"}),
		// 他会话记录：必须隔离（含 metadata 旧口径他会话）。
		sessRec("dk-s-7", decision.TriggerLoopGuardBlocked, decision.SourceRef{RunID: "run-5", SessionID: "sess-b"}, map[string]any{}),
		sessRec("dk-s-8", decision.TriggerLoopGuardBlocked, decision.SourceRef{RunID: "run-6"}, map[string]any{"session_id": "sess-b"}),
		// 同会话但非 system_guard 类别：不计。
		{
			DecisionKey: "dk-s-9", Category: decision.CategoryHITLApproval,
			Scenario: "审批", Reasoning: "-", Outcome: "approved",
			ActorType: decision.ActorHuman, ActorKey: "user-1",
			RelatedEntities: []decision.EntityRef{},
			SourceRef:       decision.SourceRef{RunID: "run-1", SessionID: "sess-a"},
			Metadata:        map[string]any{"trigger_rule": decision.TriggerLoopGuardBlocked},
			CreatedAt:       "2026-08-27T01:00:00Z", UpdatedAt: "2026-08-27T01:00:00Z",
		},
	}
	if err := wr.InsertRecords(ctx, recs); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	statsRepo, ok := qr.(decision.SessionGateStatsRepo)
	if !ok {
		t.Fatal("decisionQueryRepo 未实现 SessionGateStatsRepo 窄接口")
	}
	got, err := statsRepo.SessionGateStats(ctx, "sess-a")
	if err != nil {
		t.Fatalf("SessionGateStats: %v", err)
	}
	if got.LoopGuardBlocks != 2 {
		t.Errorf("LoopGuardBlocks = %d, want 2（新口径 1 + 旧口径 metadata 1）", got.LoopGuardBlocks)
	}
	if got.ParamRuleDenies != 1 {
		t.Errorf("ParamRuleDenies = %d, want 1", got.ParamRuleDenies)
	}
	if got.PruneCount != 4 || got.PruneBytes != 9000 {
		t.Errorf("prune = %d 条/%d 字节, want 4/9000", got.PruneCount, got.PruneBytes)
	}
	if got.CompactCount != 1 {
		t.Errorf("CompactCount = %d, want 1", got.CompactCount)
	}
	if !got.BudgetTripped || got.NoProgressTripped {
		t.Errorf("tripped = %v/%v, want true/false", got.BudgetTripped, got.NoProgressTripped)
	}

	// 他会话只见自己的记录；无记录会话 / 空会话 id 返回零值, nil。
	other, err := statsRepo.SessionGateStats(ctx, "sess-b")
	if err != nil || other.LoopGuardBlocks != 2 || other.BudgetTripped {
		t.Errorf("sess-b 隔离失败（新旧口径各 1）: %+v err=%v", other, err)
	}
	zero, err := statsRepo.SessionGateStats(ctx, "sess-missing")
	if err != nil || zero != (decision.RunGateStats{}) {
		t.Errorf("无记录会话应零值: %+v err=%v", zero, err)
	}
	if _, err := statsRepo.SessionGateStats(ctx, "  "); err != nil {
		t.Errorf("空会话 id 应 nil error: %v", err)
	}

	// source_session_id 列表过滤同表达式：sess-a 命中 6 条 system_guard +
	// 1 条 hitl（会话归属不过滤类别），sess-b 2 条。
	_, total, err := qr.ListRecords(ctx, decision.ListFilter{SourceSessionID: "sess-a", Page: 1, PageSize: 50})
	if err != nil || total != 7 {
		t.Errorf("source_session_id filter sess-a: total=%d err=%v, want 7", total, err)
	}
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{SourceSessionID: "sess-b", Page: 1, PageSize: 50})
	if err != nil || total != 2 {
		t.Errorf("source_session_id filter sess-b: total=%d err=%v, want 2", total, err)
	}
}

// TestDecisionRecords_SessionIDIndex_ExplainHit_PG 钉死 T5 索引命中
// （2026-08-27 四轮审查，20261252/20261254 两度索引失配踩坑后的硬性验证）：
// 查询侧 gateSessionExpr 生成串必须与 20261268 索引表达式树精确一致——
// SET enable_seqscan=off 下 EXPLAIN 必须走 idx_decision_records_source_session_id，
// 失配即回归（全表扫在 1M 行生产表上 P95≈125ms，索引后 <5ms）。
func TestDecisionRecords_SessionIDIndex_ExplainHit_PG(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	for _, f := range []string{
		"sql/migrations/20261250_decision_records.sql",
		"sql/migrations/20261251_decision_record_outbox.sql",
		"sql/migrations/20261252_decision_records_idx.sql",
		"sql/migrations/20261268_decision_records_sessionid_idx.sql",
	} {
		if err := executeSQLFileWithDialect(ctx, db, f, DialectPostgres, lg); err != nil {
			t.Fatalf("migrate %s: %v", f, err)
		}
	}
	// 小表优化器倾向 seq scan；关 seqscan 验证索引「可命中性」（表达式树
	// 匹配失败时即便 enable_seqscan=off 也只能输出带高 cost 的 Seq Scan）。
	if _, err := db.ExecContext(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatalf("set enable_seqscan: %v", err)
	}
	rows, err := db.QueryContext(ctx,
		"EXPLAIN SELECT count(*) FROM decision_records WHERE category = 'system_guard' AND "+
			"COALESCE(COALESCE(NULLIF(source_ref::text, '')::jsonb, '{}'::jsonb) ->> 'session_id', "+
			"COALESCE(NULLIF(metadata::text, '')::jsonb, '{}'::jsonb) ->> 'session_id') = 'sess-a'")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan += line + "\n"
	}
	if !strings.Contains(plan, "idx_decision_records_source_session_id") {
		t.Fatalf("查询表达式未命中 20261268 索引（gateSessionExpr 与索引表达式树失配回归）:\n%s", plan)
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

// TestDecisionQueryRepo_WorkspaceFilter_PG covers t-dr-3 租户隔离：非系统
// caller 的可见集 = [callerWS, ""]（本租户 + 共享记录），他租户记录 fail-closed
// 不命中；nil = 系统 caller 不过滤；空非 nil 切片 = 恒空。
func TestDecisionQueryRepo_WorkspaceFilter_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	mk := func(key, ws string) decision.Record {
		return decision.Record{
			DecisionKey: key, Category: decision.CategorySystemGuard,
			Scenario: "系统闸", Reasoning: "-", Outcome: "tripped",
			ActorType: decision.ActorSystem, ActorKey: "system:guard",
			RelatedEntities: []decision.EntityRef{},
			SourceRef:       decision.SourceRef{RunID: "run-ws"},
			WorkspaceID:     ws,
			CreatedAt:       "2026-08-27T00:00:00Z", UpdatedAt: "2026-08-27T00:00:00Z",
		}
	}
	if err := wr.InsertRecords(ctx, []decision.Record{
		mk("dk-ws-a", "ws-a"),  // 租户 A 私有
		mk("dk-ws-b", "ws-b"),  // 租户 B 私有
		mk("dk-ws-shared", ""), // 共享（legacy/系统产物）
	}); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// 租户 A 视角：见 dk-ws-a + dk-ws-shared，不见 dk-ws-b。
	_, total, err := qr.ListRecords(ctx, decision.ListFilter{
		VisibleWorkspaces: []string{"ws-a", ""}, Page: 1, PageSize: 10})
	if err != nil || total != 2 {
		t.Fatalf("ws-a view: total=%d err=%v, want 2", total, err)
	}
	// 租户 B 视角：见 dk-ws-b + dk-ws-shared。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{
		VisibleWorkspaces: []string{"ws-b", ""}, Page: 1, PageSize: 10})
	if err != nil || total != 2 {
		t.Fatalf("ws-b view: total=%d err=%v, want 2", total, err)
	}
	// 系统 caller（nil）：全量 3 条不过滤。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{Page: 1, PageSize: 10})
	if err != nil || total != 3 {
		t.Fatalf("system view: total=%d err=%v, want 3", total, err)
	}
	// 空非 nil 切片：fail-closed 恒空。
	_, total, err = qr.ListRecords(ctx, decision.ListFilter{
		VisibleWorkspaces: []string{}, Page: 1, PageSize: 10})
	if err != nil || total != 0 {
		t.Fatalf("empty visible set: total=%d err=%v, want 0", total, err)
	}
}

// TestDecisionRepo_OutboxDeadLetter_PG covers t-dr-4 死信生命周期：
// enqueue → attempts 递增 → 触达 MaxOutboxAttempts 自动翻 dead 退出扫描；
// MarkOutboxDead 直接翻终态（poison 路径）；dead 行不再被 ListPendingOutbox
// 命中（head-of-line blocking 防线）。
func TestDecisionRepo_OutboxDeadLetter_PG(t *testing.T) {
	ctx := context.Background()
	wr, _ := newDecisionQueryTestRepos(t)
	rec := decision.Record{
		DecisionKey: "dk-dl-1", Category: decision.CategorySystemGuard,
		Scenario: "系统闸", Reasoning: "-", Outcome: "tripped",
		ActorType: decision.ActorSystem, ActorKey: "system:guard",
		RelatedEntities: []decision.EntityRef{},
		SourceRef:       decision.SourceRef{RunID: "run-dl"},
		CreatedAt:       "2026-08-27T00:00:00Z", UpdatedAt: "2026-08-27T00:00:00Z",
	}
	if err := wr.EnqueueOutbox(ctx, []decision.Record{rec}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	rows, err := wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending after enqueue: len=%d err=%v, want 1", len(rows), err)
	}
	id := rows[0].ID

	// attempts 递增到上限-1：仍 pending 可扫描。
	for i := 1; i < decision.MaxOutboxAttempts; i++ {
		if err := wr.MarkOutboxAttempt(ctx, id, "boom"); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 || rows[0].Attempts != decision.MaxOutboxAttempts-1 {
		t.Fatalf("before limit: rows=%+v err=%v", rows, err)
	}

	// 触达上限的一次：同事务翻 dead → 退出扫描。
	if err := wr.MarkOutboxAttempt(ctx, id, "boom final"); err != nil {
		t.Fatalf("final attempt: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dead row must leave pending scan: len=%d err=%v", len(rows), err)
	}

	// MarkOutboxDead 直接翻终态（poison 路径）：第二行入队 → dead → 不命中。
	rec2 := rec
	rec2.DecisionKey = "dk-dl-2"
	if err := wr.EnqueueOutbox(ctx, []decision.Record{rec2}); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("pending 2: len=%d err=%v", len(rows), err)
	}
	if err := wr.MarkOutboxDead(ctx, []int64{rows[0].ID}, "poison: payload undecodable"); err != nil {
		t.Fatalf("mark dead: %v", err)
	}
	rows, err = wr.ListPendingOutbox(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("poison dead row must leave pending scan: len=%d err=%v", len(rows), err)
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
