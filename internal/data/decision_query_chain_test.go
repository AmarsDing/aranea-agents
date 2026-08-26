package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/decision"
)

// seedDecisionChain 构造 A(planner,run-1) → B(hitl,parent=A) → C(guard,
// parent=B) 真实链 + D(guard,run-1,无父) 虚拟父兜底样本，返回各记录 ID。
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

// TestDecisionChainRepo_VirtualParent_PG covers 设计 §5 兜底补链：
// 无父但带 run_id 的记录，取同 run 内最近前置 planner 决策。
func TestDecisionChainRepo_VirtualParent_PG(t *testing.T) {
	ctx := context.Background()
	wr, qr := newDecisionQueryTestRepos(t)
	a, _, _, d := seedDecisionChain(t, wr, qr)

	crepo := qr.(decision.ChainRepo)
	vp, err := crepo.FindLatestPlannerByRun(ctx, d.SourceRef.RunID, d.CreatedAt, d.ID)
	if err != nil {
		t.Fatalf("virtual parent: %v", err)
	}
	if vp == nil || vp.ID != a.ID || vp.DecisionKey != "dk-c-a" {
		t.Fatalf("virtual parent = %+v, want A", vp)
	}

	// 不同 run → 未命中 nil。
	vp, err = crepo.FindLatestPlannerByRun(ctx, "run-nonexistent", d.CreatedAt, d.ID)
	if err != nil || vp != nil {
		t.Fatalf("cross-run = (%+v,%v), want nil/nil", vp, err)
	}
}

func keysOf(recs []decision.Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.DecisionKey
	}
	return out
}
