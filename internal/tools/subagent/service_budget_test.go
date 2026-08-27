package subagent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

// C4-②：env 覆盖语义与 team.TokenBudgetInputTokens 对齐——>0 覆盖默认、
// <0 禁用、0/未设/非法回退默认。
func TestResolveSubagentTokenBudget(t *testing.T) {
	const env = "ARANEA_SUBAGENT_TEST_BUDGET"
	t.Setenv(env, "")
	if got := resolveSubagentTokenBudget(env, 500); got != 500 {
		t.Errorf("unset = %d, want 500", got)
	}
	t.Setenv(env, "0")
	if got := resolveSubagentTokenBudget(env, 500); got != 500 {
		t.Errorf("zero = %d, want 500（0 回退默认）", got)
	}
	t.Setenv(env, "abc")
	if got := resolveSubagentTokenBudget(env, 500); got != 500 {
		t.Errorf("invalid = %d, want 500", got)
	}
	t.Setenv(env, "1234")
	if got := resolveSubagentTokenBudget(env, 500); got != 1234 {
		t.Errorf("override = %d, want 1234", got)
	}
	t.Setenv(env, "-1")
	if got := resolveSubagentTokenBudget(env, 500); got != -1 {
		t.Errorf("disable = %d, want -1（负值禁用闸）", got)
	}
}

// C4-② run 级闸：流式累计 input 超阈 → 取消 run ctx + 跳闸事件只发一次 +
// 增量记账到父会话合计（供 Spawn aggregate 闸）。
func TestRunBudgetGuard_TripsOnceAndCancels(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.runBudgetInputTokens = 100
	cancelled := false
	svc.running["run-1"] = &runningRun{cancel: func() { cancelled = true }}
	var trips []BudgetTripInfo
	svc.SetBudgetTripHook(func(_ context.Context, info BudgetTripInfo) {
		trips = append(trips, info)
	})

	g := &runBudgetGuard{svc: svc, runID: "run-1", parentSID: "sess-p"}
	a := &usageAccum{}

	// 阈下：不跳闸，但父会话合计已记账。
	a.consume(usageEvent(60, 5, 0, "m"))
	g.observe(a)
	if cancelled {
		t.Fatal("阈下不得取消 run")
	}
	if len(trips) != 0 {
		t.Fatalf("阈下不得发跳闸事件，got %d", len(trips))
	}
	if got := svc.parentInputTokens["sess-p"]; got != 60 {
		t.Fatalf("parent 合计 = %d, want 60", got)
	}

	// 第二轮 prompt 增长 → 新计费轮，合计 60+150=210 > 100 → 跳闸。
	a.consume(usageEvent(150, 5, 0, "m"))
	g.observe(a)
	if !cancelled {
		t.Fatal("超阈必须取消 run ctx")
	}
	if len(trips) != 1 {
		t.Fatalf("跳闸事件 = %d, want 1", len(trips))
	}
	trip := trips[0]
	if trip.Scope != BudgetScopeRun || trip.RunID != "run-1" || trip.ParentSessionID != "sess-p" {
		t.Errorf("trip 归属错: %+v", trip)
	}
	if trip.UsedInputTokens != 210 || trip.BudgetInputTokens != 100 {
		t.Errorf("trip 额度 = (%d,%d), want (210,100)", trip.UsedInputTokens, trip.BudgetInputTokens)
	}
	if g.tripReason == "" {
		t.Error("tripReason 空，finishRun/usage 将无法区分预算取消与用户取消")
	}
	if got := svc.parentInputTokens["sess-p"]; got != 210 {
		t.Fatalf("parent 合计 = %d, want 210（增量记账）", got)
	}

	// 再次超阈：不重复跳闸、不重复取消（只触发一次）。
	a.consume(usageEvent(300, 5, 0, "m"))
	g.observe(a)
	if len(trips) != 1 {
		t.Fatalf("重复跳闸，events = %d, want 1", len(trips))
	}
}

// 闸禁用（<0）时任何用量都不跳闸，但父会话合计仍记账（记账与跳闸独立）。
func TestRunBudgetGuard_DisabledStillAccountsParent(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.runBudgetInputTokens = -1 // 禁用
	var trips []BudgetTripInfo
	svc.SetBudgetTripHook(func(_ context.Context, info BudgetTripInfo) {
		trips = append(trips, info)
	})
	g := &runBudgetGuard{svc: svc, runID: "run-1", parentSID: "sess-p"}
	a := &usageAccum{}
	a.consume(usageEvent(999999, 5, 0, "m"))
	g.observe(a)
	if len(trips) != 0 {
		t.Fatalf("禁用闸不得跳闸，got %d", len(trips))
	}
	if got := svc.parentInputTokens["sess-p"]; got != 999999 {
		t.Fatalf("parent 合计 = %d, want 999999", got)
	}
}

// C4-② 父会话 aggregate 闸：历史 spawn 合计已达上限 → Spawn 拒绝
// （RateLimit，retry_reflect 判确定性不烧重试配额）+ 跳闸事件落账。
func TestSpawn_ParentAggregateBudgetReject(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()
	svc.parentBudgetInputTokens = 1000
	svc.parentInputTokens["sess-p"] = 1000
	var trips []BudgetTripInfo
	svc.SetBudgetTripHook(func(_ context.Context, info BudgetTripInfo) {
		trips = append(trips, info)
	})

	_, err = svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "sess-p",
		Task:            "do something",
	})
	if err == nil {
		t.Fatal("合计超预算必须拒绝 Spawn")
	}
	if !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("错误须引导模型停止重试: %v", err)
	}
	if len(trips) != 1 {
		t.Fatalf("跳闸事件 = %d, want 1", len(trips))
	}
	if trips[0].Scope != BudgetScopeParentAggregate || trips[0].ParentSessionID != "sess-p" {
		t.Errorf("trip 归属错: %+v", trips[0])
	}
	// 拒绝不产生 run 记录。
	if len(svc.runs) != 0 {
		t.Fatalf("拒绝后不得残留 run 记录，got %d", len(svc.runs))
	}
}

// 对照组：合计未达上限时 Spawn 正常放行。
func TestSpawn_ParentAggregateBudgetUnderLimit(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()
	svc.parentBudgetInputTokens = 1000
	svc.parentInputTokens["sess-p"] = 999
	var trips []BudgetTripInfo
	svc.SetBudgetTripHook(func(_ context.Context, info BudgetTripInfo) {
		trips = append(trips, info)
	})
	run, err := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "sess-p",
		Task:            "do something",
	})
	if err != nil {
		t.Fatalf("阈下 Spawn 不得被拒: %v", err)
	}
	if run.ID == "" {
		t.Fatal("run id 空")
	}
	if len(trips) != 0 {
		t.Fatalf("阈下不得发跳闸事件，got %d", len(trips))
	}
}

// 跳闸留痕：tripReason 写入 usage 行 ErrMsg——跳闸路径 runErr=
// context.Canceled，仅靠 status=cancelled 无法与用户主动取消区分。
func TestRecordRunUsage_TripReasonRecorded(t *testing.T) {
	rec := &stubUsageRecorder{}
	svc := newUsageTestService(t, rec)
	usage := &usageAccum{}
	usage.consume(usageEvent(10, 5, 0, "m"))
	svc.recordRunUsage(
		&runRecord{Run: trpcsubagent.Run{ID: "r"}},
		runningRun{startedAt: svc.clock()}, usage, context.Canceled,
		"subagent input-token budget tripped (used 210, budget 100)")
	if len(rec.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].Status != "cancelled" {
		t.Errorf("Status = %q, want cancelled", rec.calls[0].Status)
	}
	if !strings.Contains(rec.calls[0].ErrMsg, "budget tripped") {
		t.Errorf("ErrMsg = %q, want 含 budget tripped", rec.calls[0].ErrMsg)
	}
}
