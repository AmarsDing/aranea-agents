package tools

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// TestBatchExecuteSpiritTools_ParallelFasterThanSerial verifies that 5
// independent calls (each sleeping 80ms) complete in less than 40% of the
// serial time when a ParallelToolExecutor is supplied. This is the B5
// acceptance criterion from the integration plan.
func TestBatchExecuteSpiritTools_ParallelFasterThanSerial(t *testing.T) {
	const (
		numCalls    = 5
		callDelay   = 80 * time.Millisecond
		serialTotal = numCalls * callDelay   // 400ms
		parallelMax = serialTotal * 40 / 100 // 160ms (40% threshold)
	)

	var active int32
	var maxActive int32
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		cur := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
				break
			}
		}
		select {
		case <-time.After(callDelay):
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
		atomic.AddInt32(&active, -1)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	exec := NewParallelToolExecutor(nil, loggateway.NewNoop(),
		WithMaxConcurrency(numCalls))

	calls := make([]ToolCall, numCalls)
	for i := range calls {
		calls[i] = ToolCall{ID: string(rune('a' + i)), Name: "slow"}
	}

	start := time.Now()
	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())
	elapsed := time.Since(start)

	if len(results) != numCalls {
		t.Fatalf("expected %d results, got %d", numCalls, len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful: %s", r.CallID, r.Error)
		}
	}
	if elapsed >= parallelMax {
		t.Errorf("parallel execution took %v, expected < %v (40%% of serial %v)",
			elapsed, parallelMax, serialTotal)
	}
	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Errorf("expected concurrent execution, max active = %d", got)
	}
}

// TestBatchExecuteSpiritTools_NilExecutorFallsBackToSerial verifies that when
// the ParallelToolExecutor is nil, calls run serially (no concurrency).
func TestBatchExecuteSpiritTools_NilExecutorFallsBackToSerial(t *testing.T) {
	const callDelay = 40 * time.Millisecond

	var active int32
	var maxActive int32
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		cur := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
				break
			}
		}
		time.Sleep(callDelay)
		atomic.AddInt32(&active, -1)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
		{ID: "c", Name: "z"},
	}

	results := BatchExecuteSpiritTools(context.Background(), nil, handler, calls, loggateway.NewNoop())

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful: %s", r.CallID, r.Error)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("expected max concurrency 1 for serial fallback, got %d", got)
	}
}

// TestBatchExecuteSpiritTools_CycleFallsBackToSerial verifies that when
// ParallelToolExecutor.Execute returns an error (e.g., dependency cycle), the
// function falls back to serial execution and still returns results for all
// calls.
func TestBatchExecuteSpiritTools_CycleFallsBackToSerial(t *testing.T) {
	var mu sync.Mutex
	var order []string
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		mu.Lock()
		order = append(order, call.ID)
		mu.Unlock()
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "ok"}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())

	calls := []ToolCall{
		{ID: "a", Name: "x", DependsOn: []string{"b"}},
		{ID: "b", Name: "y", DependsOn: []string{"a"}},
	}

	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())

	if len(results) != 2 {
		t.Fatalf("expected 2 results after fallback, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result %s was not successful after fallback: %s", r.CallID, r.Error)
		}
	}
}

// TestBatchExecuteSpiritTools_EmptyInputReturnsNil verifies the no-op path.
func TestBatchExecuteSpiritTools_EmptyInputReturnsNil(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	results := BatchExecuteSpiritTools(context.Background(), exec, nil, nil, loggateway.NewNoop())
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}
}

// TestBatchExecuteSpiritTools_NilHandlerWithExecutor verifies that when the
// handler is nil but the executor is non-nil, the function falls back to serial
// execution which produces failure results (no panic).
func TestBatchExecuteSpiritTools_NilHandlerWithExecutor(t *testing.T) {
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop())
	calls := []ToolCall{{ID: "a", Name: "x"}}

	results := BatchExecuteSpiritTools(context.Background(), exec, nil, calls, loggateway.NewNoop())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failure when handler is nil")
	}
}

// TestBatchExecuteSpiritTools_ContextCancelAbortsSerial verifies that context
// cancellation during serial fallback stops further calls and marks the
// in-flight call as failed.
func TestBatchExecuteSpiritTools_ContextCancelAbortsSerial(t *testing.T) {
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		select {
		case <-time.After(100 * time.Millisecond):
			return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
		case <-ctx.Done():
			return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: ctx.Err().Error()}
		}
	}

	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	results := BatchExecuteSpiritTools(ctx, nil, handler, calls, loggateway.NewNoop())

	// Serial fallback: first call hits the deadline, second is skipped.
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("result %s should not be successful after cancel", r.CallID)
		}
	}
}

// TestBatchExecuteSpiritTools_DependencyOrderPreserved verifies that when
// calls have dependencies, the parallel executor respects the topological
// order: "a" runs before "b" which depends on it.
func TestBatchExecuteSpiritTools_DependencyOrderPreserved(t *testing.T) {
	const callDelay = 30 * time.Millisecond

	var mu sync.Mutex
	var order []string
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		mu.Lock()
		order = append(order, call.ID)
		mu.Unlock()
		time.Sleep(callDelay)
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	exec := NewParallelToolExecutor(handler, loggateway.NewNoop())

	calls := []ToolCall{
		{ID: "a", Name: "first"},
		{ID: "b", Name: "second", DependsOn: []string{"a"}},
	}

	results := BatchExecuteSpiritTools(context.Background(), exec, handler, calls, loggateway.NewNoop())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("expected order [a, b], got %v", order)
	}
}

// TestExecuteToolCallsSerial_NilHandlerProducesFailure verifies the serial
// helper returns failure results (not panics) when handler is nil.
func TestExecuteToolCallsSerial_NilHandlerProducesFailure(t *testing.T) {
	calls := []ToolCall{
		{ID: "a", Name: "x"},
		{ID: "b", Name: "y"},
	}
	results := executeToolCallsSerial(context.Background(), nil, calls, loggateway.NewNoop())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Error("expected failure for nil handler")
		}
		if r.Error == "" {
			t.Error("expected non-empty error message")
		}
	}
}

// TestBatchExecuteSpiritTools_InheritsWorktreeIsolator verifies the fresh
// executor built inside BatchExecuteSpiritTools inherits the Wire-bound
// executor's worktree isolator: a call tagged IsolationStrategyWorktree must
// be routed to the isolator, not executed directly (Phase C integration).
func TestBatchExecuteSpiritTools_InheritsWorktreeIsolator(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()

	iso, err := NewWorktreeIsolator(repoRoot, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewWorktreeIsolator: %v", err)
	}
	exec := NewParallelToolExecutor(nil, loggateway.NewNoop(),
		WithMaxConcurrency(2), WithWorktreeIsolator(iso))

	directHandler := func(ctx context.Context, call ToolCall) ToolResult {
		if dir, ok := WorktreeDirFromContext(ctx); ok {
			return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "worktree:" + dir}
		}
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "direct"}
	}
	calls := []ToolCall{
		{ID: "w1", Name: "file_write", IsolationStrategy: IsolationStrategyWorktree},
		{ID: "d1", Name: "echo"},
	}
	results := BatchExecuteSpiritTools(context.Background(), exec, directHandler, calls, loggateway.NewNoop())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	byID := map[string]ToolResult{}
	for _, r := range results {
		byID[r.CallID] = r
	}
	// Isolator handler is nil, so executeOne runs the call-time handler inside
	// the worktree (dir on ctx). Untagged calls stay on the live handler.
	if got := byID["w1"]; !got.Success || !strings.HasPrefix(got.Output, "worktree:") {
		t.Fatalf("worktree-tagged call should run handler inside the worktree, got %+v", got)
	}
	if got := byID["d1"]; !got.Success || got.Output != "direct" {
		t.Fatalf("untagged call should run via the direct handler, got %+v", got)
	}
}

func TestShouldRejectFactQueryPlan(t *testing.T) {
	if !shouldRejectFactQueryPlan("明天天气怎么样", "direct", false, nil) {
		t.Fatal("light weather must reject plan_and_execute")
	}
	if !shouldRejectFactQueryPlan("明天天气怎么样", "", false, nil) {
		t.Fatal("empty mode weather must reject")
	}
	if shouldRejectFactQueryPlan("明天天气怎么样", "dag", false, nil) {
		t.Fatal("explicit dag must not reject")
	}
	if shouldRejectFactQueryPlan("明天天气怎么样", "direct", true, nil) {
		t.Fatal("force_new must not reject")
	}
	if shouldRejectFactQueryPlan("明天天气怎么样", "direct", false, []string{"__system_admin__"}) {
		t.Fatal("explicit keys must not reject")
	}
	if shouldRejectFactQueryPlan("用 Go 写 REST 接口", "direct", false, nil) {
		t.Fatal("coding task must not look like a fact query")
	}
}

// TestShouldRejectDirectAnswerPlan 钉住包C Q2-C1 直答拦截边界
// （session-eval-20260827 S11-t5）：「推荐三本书」型明显直答请求进入
// plan_and_execute 必须在工具边界拒绝并引导直答；显式组队/键路由/
// force_new/复合任务一律放行。
func TestShouldRejectDirectAnswerPlan(t *testing.T) {
	if !shouldRejectDirectAnswerPlan("推荐三本关于分布式系统的书", "", false, nil) {
		t.Fatal("S11-t5 荐书必须拒绝 plan_and_execute（空 mode）")
	}
	if !shouldRejectDirectAnswerPlan("推荐三本关于分布式系统的书", "direct", false, nil) {
		t.Fatal("S11-t5 荐书必须拒绝（direct mode）")
	}
	if !shouldRejectDirectAnswerPlan("什么是 Kubernetes", "", false, nil) {
		t.Fatal("概念解释必须拒绝")
	}
	if shouldRejectDirectAnswerPlan("推荐三本关于分布式系统的书", "dag", false, nil) {
		t.Fatal("显式 dag 不得拒绝")
	}
	if shouldRejectDirectAnswerPlan("推荐三本关于分布式系统的书", "", true, nil) {
		t.Fatal("force_new 不得拒绝")
	}
	if shouldRejectDirectAnswerPlan("推荐三本关于分布式系统的书", "", false, []string{"__system_admin__"}) {
		t.Fatal("显式 agent_keys 不得拒绝")
	}
	if shouldRejectDirectAnswerPlan("推荐三本微服务架构的书并整理成对比表格", "", false, nil) {
		t.Fatal("复合任务（整理/对比）必须被任务信号 veto，不得拒绝")
	}
	if shouldRejectDirectAnswerPlan("帮我写一份推荐信", "", false, nil) {
		t.Fatal("交付物生产（写一份）不得拒绝")
	}
	if shouldRejectDirectAnswerPlan("明天天气怎么样", "", false, nil) {
		t.Fatal("天气走事实查询拦截，不在直答边界重复")
	}
}

// ---------------------------------------------------------------------------
// Q7 分解层澄清出口（session-eval-20260827 P4 根修）
// ---------------------------------------------------------------------------

// clarifyPlanStub 是返回澄清计划的最小 TaskPlannerPort 实现——allocator /
// orchestrator 传 nil，若澄清计划误入分配/编排阶段会因 nil 解引用 panic，
// 测试自然失败（反向证明早退生效）。
type clarifyPlanStub struct {
	plan *biz.TaskPlan
}

func (s *clarifyPlanStub) Plan(_ context.Context, _ biz.PlanInput) (*biz.TaskPlan, error) {
	return s.plan, nil
}
func (s *clarifyPlanStub) QuickAssess(_ context.Context, _ biz.PlanInput) (biz.ComplexityLevel, float64, error) {
	return biz.ComplexitySimple, 0, nil
}
func (s *clarifyPlanStub) GetPlan(_ context.Context, _ string) (*biz.TaskPlan, error) {
	return nil, nil
}
func (s *clarifyPlanStub) ListPlans(_ context.Context, _ string) ([]*biz.TaskPlan, error) {
	return nil, nil
}
func (s *clarifyPlanStub) ConfirmPlan(_ context.Context, _ string, _ biz.PlanAdjustments) (*biz.TaskPlan, error) {
	return nil, nil
}
func (s *clarifyPlanStub) PublishV2Board(_ context.Context, _ *biz.TaskPlan, _ *biz.AllocationPlan, _ string) (biz.PlanBoard, error) {
	return biz.PlanBoard{}, nil
}

// plan_and_execute 收到带 ClarificationQuestions 的 plan 时必须早退：
// NextAction=await_user_clarification + 问题透传，不分配/不建板/不编排。
func TestPlanAndExecute_NeedsClarification_ShortCircuits(t *testing.T) {
	planner := &clarifyPlanStub{plan: &biz.TaskPlan{
		ID:                     "tp_clarify",
		SpiritSessionID:        "spirit-1",
		Strategy:               biz.StrategyDirect,
		ComplexityLevel:        biz.ComplexityModerate,
		DecomposeReason:        "needs_clarification",
		ClarificationQuestions: []string{"目标产品是什么？", "营销预算区间？"},
	}}
	tool := NewPlanAndExecuteTool(planner, nil, nil, nil, nil, nil, loggateway.NewNoop())
	ctx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "spirit-1"},
	})
	res, err := tool.Call(ctx, []byte(`{"task_prompt":"组建两个团队为新品做营销方案","mode":"dag"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	out, ok := res.(PlanAndExecuteOutput)
	if !ok {
		t.Fatalf("result type = %T, want PlanAndExecuteOutput", res)
	}
	if out.NextAction != "await_user_clarification" {
		t.Fatalf("NextAction = %q, want await_user_clarification", out.NextAction)
	}
	if len(out.ClarificationQuestions) != 2 || out.ClarificationQuestions[0] != "目标产品是什么？" {
		t.Fatalf("ClarificationQuestions = %v, want 2 questions passthrough", out.ClarificationQuestions)
	}
	if out.OrchestrationID != "" {
		t.Fatalf("OrchestrationID = %q, want empty（澄清计划不得进入编排）", out.OrchestrationID)
	}
}
