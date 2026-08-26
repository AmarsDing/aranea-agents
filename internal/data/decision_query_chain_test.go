package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/decision"
)

// seedDecisionChain 构造 A(planner,run-1) → B(hitl,parent=A) → C(guard,
// parent=B) 真实链 + D(guard,run-1,无父) 孤儿记录，返回各记录 ID。
func seedDecisionChain(t *testing.T, wr decision.Repo, qr decision.QueryRepo) (a, b, c, d *decision.Record) {
	t.Helper()
	ctx := context.Background()
	mk := func(key, cat, outcome, runID, createdAt string) decision.Record {
		return decision.Record{
			DecisionKey: key, Category: decision.Category(cat),
			Scenario: key + " scenario", Reasoning: key + " reasoning",
			Outcome: outcome, ActorType: decision.ActorSystem, ActorKey: "system:test",
			SourceRef: decision.SourceRef{RunID: runID},
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
	}
	recA := mk("dk-c-a", "planner_orchestration", "selected_dag", "run-1", "2026-08-26T01:00:00Z")
	recB := mk("dk-c-b", "hitl_approval", "approved", "run-1", "2026-08-26T02:00:00Z")
	recC := mk("dk-c-c", "system_guard", "tripped", "run-1", "2026-08-26T03:00:00Z")
	recD := mk("dk-c-d", "system_guard", "blocked", "run-1", "2026-08-26T04:00:00Z")
	if err := wr.InsertRecords(ctx, []decision.Record{recA}); err != nil {
		t.Fatalf("insert A: %v", err)
	}
	a, err := qr.GetByKey(ctx, "dk-c-a")
	if err != nil || a == nil {
		t.Fatalf("get A: %v", err)
	}
	recB.ParentDecisionID = &a.ID
	if err := wr.InsertRecords(ctx, []decision.Record{recB}); err != nil {
		t.Fatalf("insert B: %v", err)
	}
	b, err = qr.GetByKey(ctx, "dk-c-b")
	if err != nil || b == nil {
		t.Fatalf("get B: %v", err)
	}
	recC.ParentDecisionID = &b.ID
	if err := wr.InsertRecords(ctx, []decision.Record{recC, recD}); err != nil {
		t.Fatalf("insert C/D: %v", err)
	}
	if c, err = qr.GetByKey(ctx, "dk-c-c"); err != nil || c == nil {
		t.Fatalf("get C: %v", err)
	}
	if d, err = qr.GetByKey(ctx, "dk-c-d"); err != nil || d == nil {
		t.Fatalf("get D: %v", err)
	}
	return a, b, c, d
}

// TestDecisionChainRepo_UpstreamDownstream_PG covers 1.8 递归 CTE：
// 上游逐级递远 [直接父→祖先]、下游深度升序、深度闸截断。
func TestDecisionChainRepo_UpstreamDownstream_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	a, _, c, _ := seedDecisionChain(t, wr, qr)

	crepo := qr.(decision.ChainRepo)

	// 上游：C → [B, A]（[0]=直接父）。
	up, err := crepo.ListUpstream(ctx, c.ID, 20)
	if err != nil {
		t.Fatalf("upstream: %v", err)
	}
	if len(up) != 2 || up[0].DecisionKey != "dk-c-b" || up[1].DecisionKey != "dk-c-a" {
		t.Fatalf("upstream = %+v", keysOf(up))
	}

	// 深度闸：maxDepth=1 只回直接父。
	up, err = crepo.ListUpstream(ctx, c.ID, 1)
	if err != nil || len(up) != 1 || up[0].DecisionKey != "dk-c-b" {
		t.Fatalf("depth cap upstream = %+v err=%v", keysOf(up), err)
	}

	// 下游：A → [B(depth1), C(depth2)]。
	down, err := crepo.ListDownstream(ctx, a.ID, 20)
	if err != nil {
		t.Fatalf("downstream: %v", err)
	}
	if len(down) != 2 || down[0].DecisionKey != "dk-c-b" || down[1].DecisionKey != "dk-c-c" {
		t.Fatalf("downstream = %+v", keysOf(down))
	}

	// 无父锚点上游为空。
	up, err = crepo.ListUpstream(ctx, a.ID, 20)
	if err != nil || len(up) != 0 {
		t.Fatalf("root upstream = %+v err=%v", keysOf(up), err)
	}
}

// TestDecisionChainRepo_VirtualParent_PG covers 设计 §5 兜底补链的
// 两段解析（2026-08-26 Gap 修复）：
//  ① source_ref.flow_trace_id 非空 → 同 trace planner 决策精确匹配（且
//    优先于 run_id 桥接）；② source_ref.run_id 非空 → 桥接
//    team_runs→teams.spirit_session_id→planner metadata.spirit_session_id，
//    取 created_at <= before 的最近一条。落空场景（run 不存在、team 无
//    spirit 会话、ref 全空、excludeID 自排除）返回 nil。
func TestDecisionChainRepo_VirtualParent_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr, db := newDecisionQueryTestReposWithDB(t)

	// teams/team_runs 最小镜像表（Ent 管理，仅镜像查询触及的列）。
	for _, ddl := range []string{
		`CREATE TABLE teams (id TEXT PRIMARY KEY, spirit_session_id TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE team_runs (id TEXT PRIMARY KEY, team_id TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO teams (id, spirit_session_id) VALUES
			('team-x', 'ss-x'), ('team-1', 'ss-1'), ('team-manual', '')`,
		`INSERT INTO team_runs (id, team_id) VALUES
			('run-x', 'team-x'), ('run-2', 'team-1'), ('run-manual', 'team-manual')`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	mkPlanner := func(key, ft, sid, createdAt string) decision.Record {
		r := decision.Record{
			DecisionKey: key, Category: decision.CategoryPlannerOrchestration,
			Scenario: key + " scenario", Reasoning: key + " reasoning",
			Outcome: "selected_dag", ActorType: decision.ActorSystem, ActorKey: "system:task_planner",
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
		if ft != "" {
			r.SourceRef.FlowTraceID = ft
		}
		if sid != "" {
			r.Metadata = map[string]any{"spirit_session_id": sid}
		}
		return r
	}
	mkGuard := func(key, ft, runID, createdAt string) decision.Record {
		return decision.Record{
			DecisionKey: key, Category: decision.CategorySystemGuard,
			Scenario: key + " scenario", Reasoning: key + " reasoning",
			Outcome: "tripped", ActorType: decision.ActorSystem, ActorKey: "system:token_budget",
			SourceRef: decision.SourceRef{FlowTraceID: ft, RunID: runID},
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
	}
	recs := []decision.Record{
		// 路径①：ft-1 精确匹配目标 + run-x 桥接诱饵（同刻 01:00，验证优先级）。
		mkPlanner("dk-vp-ft", "ft-1", "", "2026-08-26T01:00:00Z"),
		mkPlanner("dk-vp-run-x", "", "ss-x", "2026-08-26T01:00:00Z"),
		mkGuard("dk-vp-ft-gate", "ft-1", "run-x", "2026-08-26T02:00:00Z"),
		// 路径②：ss-1 三条 planner（01:00/01:30 命中窗口、03:00 未来排除）。
		mkPlanner("dk-vp-old", "", "ss-1", "2026-08-26T01:00:00Z"),
		mkPlanner("dk-vp-new", "", "ss-1", "2026-08-26T01:30:00Z"),
		mkPlanner("dk-vp-future", "", "ss-1", "2026-08-26T03:00:00Z"),
		mkGuard("dk-vp-run-gate", "", "run-2", "2026-08-26T02:00:00Z"),
		// 落空：run 不存在 / team 无 spirit 会话 / ref 全空。
		mkGuard("dk-vp-no-run", "", "run-nonexistent", "2026-08-26T02:00:00Z"),
		mkGuard("dk-vp-manual", "", "run-manual", "2026-08-26T02:00:00Z"),
		mkGuard("dk-vp-empty", "", "", "2026-08-26T02:00:00Z"),
	}
	if err := wr.InsertRecords(ctx, recs); err != nil {
		t.Fatalf("insert: %v", err)
	}
	byKey := map[string]*decision.Record{}
	for _, k := range []string{"dk-vp-ft", "dk-vp-new", "dk-vp-ft-gate", "dk-vp-run-gate",
		"dk-vp-no-run", "dk-vp-manual", "dk-vp-empty"} {
		r, err := qr.GetByKey(ctx, k)
		if err != nil || r == nil {
			t.Fatalf("get %s: %v", k, err)
		}
		byKey[k] = r
	}

	crepo := qr.(decision.ChainRepo)
	find := func(rec *decision.Record) (*decision.Record, error) {
		return crepo.FindVirtualParentPlanner(ctx, rec.SourceRef, rec.CreatedAt, rec.ID)
	}

	// ① flow_trace 精确匹配，且优先于 run_id 桥接（命中 dk-vp-ft 而非诱饵）。
	g := byKey["dk-vp-ft-gate"]
	vp, err := find(g)
	if err != nil || vp == nil || vp.DecisionKey != "dk-vp-ft" {
		t.Fatalf("flow_trace path = (%+v,%v), want dk-vp-ft", vp, err)
	}

	// ① excludeID 自排除：ft-1 唯一 planner 被排除 → nil。
	vp, err = crepo.FindVirtualParentPlanner(ctx, g.SourceRef, g.CreatedAt, byKey["dk-vp-ft"].ID)
	if err != nil || vp != nil {
		t.Fatalf("self-exclusion = (%+v,%v), want nil/nil", vp, err)
	}

	// ② run_id 桥接：最近前置 = 01:30 的 dk-vp-new（03:00 未来记录被
	// created_at <= before 排除）。
	vp, err = find(byKey["dk-vp-run-gate"])
	if err != nil || vp == nil || vp.DecisionKey != "dk-vp-new" {
		t.Fatalf("run bridge path = (%+v,%v), want dk-vp-new", vp, err)
	}

	// 落空三例：run 不存在 / team spirit_session_id 为空（手动 run）/ ref 全空。
	for _, k := range []string{"dk-vp-no-run", "dk-vp-manual", "dk-vp-empty"} {
		if vp, err = find(byKey[k]); err != nil || vp != nil {
			t.Fatalf("%s miss = (%+v,%v), want nil/nil", k, vp, err)
		}
	}
}

func keysOf(recs []decision.Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.DecisionKey
	}
	return out
}
