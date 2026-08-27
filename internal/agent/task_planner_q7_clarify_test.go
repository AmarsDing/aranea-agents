package agent

// task_planner_q7_clarify_test.go — Q7 分解层澄清出口（session-eval-20260827
// P4 根修）的回归测试。
//
// 根因：plan_and_execute 分解 prompt 只要求「具体参数必须 verbatim 内联」，
// 未给「参数缺失怎么办」的出口——S07 分解 LLM 虚构产品名/日期/渠道塞进
// subtask 描述。修复：prompt 增加 CLARIFICATION EXIT 契约，LLM 对阻塞性
// 信息缺失输出 {"needs_clarification": true, "questions": [...]}；解析层
// 先于 subtask 数组解析识别该形态，经 decomposeClarificationError（永久性，
// 不重试）上抛，Plan() 终止规划并把问题透传给 plan_and_execute 调用方。
//
// 覆盖：
//   parseClarificationRequest 命中/未命中矩阵（含 subtask 数组不误判）
//   isRetriableDecomposeError 澄清信号永不重试
//   decomposeTaskStream 澄清信号首次即熔断（不耗尽重试配额）
//   Plan() 澄清短路：direct 策略 + needs_clarification 留痕 + board 终态
//     + progress 事件 + ClarificationQuestions 透传，不走 decompose_failed 降级

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestParseClarificationRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		want bool
		qs   int
	}{
		{
			name: "pure clarification object",
			text: `{"needs_clarification": true, "questions": ["目标产品是什么？", "上线日期是什么时候？"]}`,
			want: true,
			qs:   2,
		},
		{
			name: "clarification object with surrounding prose",
			text: "I cannot decompose this faithfully.\n{\"needs_clarification\": true, \"questions\": [\"哪个渠道？\"]}\nPlease answer.",
			want: true,
			qs:   1,
		},
		{
			name: "blank questions are dropped",
			text: `{"needs_clarification": true, "questions": ["  ", "预算区间？"]}`,
			want: true,
			qs:   1,
		},
		{
			name: "flag false is not a hit",
			text: `{"needs_clarification": false, "questions": ["q"]}`,
			want: false,
		},
		{
			name: "flag true but no questions is not a hit",
			text: `{"needs_clarification": true, "questions": []}`,
			want: false,
		},
		{
			name: "subtask array is not a hit",
			text: `[{"id":"st_1","name":"A","description":"d"},{"id":"st_2","name":"B","description":"d"}]`,
			want: false,
		},
		{
			name: "subtask array mentioning the word is not a hit",
			text: `[{"id":"st_1","name":"澄清需求","description":"输出 needs_clarification 字段的说明文档"}]`,
			want: false,
		},
		{
			name: "empty text",
			text: "",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qs, ok := parseClarificationRequest(tc.text)
			if ok != tc.want {
				t.Fatalf("hit = %v, want %v (qs=%v)", ok, tc.want, qs)
			}
			if ok && len(qs) != tc.qs {
				t.Fatalf("questions = %v, want len %d", qs, tc.qs)
			}
		})
	}
}

// 澄清信号判永久性：同一 prompt 重试只会得到同一结论，重试纯烧 LLM 调用。
func TestIsRetriableDecomposeError_ClarificationNeverRetried(t *testing.T) {
	t.Parallel()
	err := &decomposeClarificationError{questions: []string{"q1"}}
	if isRetriableDecomposeError(err) {
		t.Fatal("clarification signal must be permanent (no retry)")
	}
}

// 流式重试循环：澄清信号首次尝试即熔断，不耗尽重试配额。
func TestDecomposeTaskStream_ClarificationNotRetried(t *testing.T) {
	t.Parallel()
	attempts := 0
	impl := &taskPlannerImpl{
		lg:                   loggateway.NewNoop(),
		retryBackoffFn:       func(int) time.Duration { return time.Millisecond },
		maxDecomposeAttempts: 5,
	}
	impl.llmAttemptFn = func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _, _ string, _ biz.ComplexityLevel, _ func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		attempts++
		return nil, nil, &decomposeClarificationError{questions: []string{"目标产品是什么？"}}
	}
	_, _, err := impl.decomposeTaskStream(context.Background(), "msg", nil, 0, "spirit-q7", "tp_q7", "", nil)
	var clarifyErr *decomposeClarificationError
	if !errors.As(err, &clarifyErr) {
		t.Fatalf("err = %v, want decomposeClarificationError passthrough", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1（澄清是有效响应，重试无意义）", attempts)
	}
}

// Plan() 澄清短路：终止规划、direct 留痕、board 补终态、progress 事件、
// 问题透传——且不得落入 decompose_failed 降级（降级会让 Spirit 自己圆场
// 再虚构一次）。
func TestPlan_NeedsClarification_ShortCircuits(t *testing.T) {
	t.Parallel()
	seq := &fakeSeqPublisher{}
	bus := &captureNoticeBus{}
	repo := &stubTaskPlanRepo{}
	impl := &taskPlannerImpl{
		repo:                 repo,
		eventBus:             bus,
		lg:                   loggateway.NewNoop(),
		seq:                  seq,
		retryBackoffFn:       func(int) time.Duration { return time.Millisecond },
		maxDecomposeAttempts: 5,
	}
	impl.llmAttemptFn = func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _, _ string, _ biz.ComplexityLevel, _ func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return nil, nil, &decomposeClarificationError{questions: []string{"目标产品是什么？", "营销预算区间？"}}
	}

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "spirit-1",
		ChatSessionID:   "chat-1",
		UserMessage:     "组建两个团队为新品做营销方案",
		Mode:            "dag",
	})
	if err != nil {
		t.Fatalf("Plan must not fail on clarification exit: %v", err)
	}
	if !plan.NeedsClarification() {
		t.Fatal("plan.NeedsClarification() = false, want true（问题必须透传给调用方）")
	}
	if len(plan.ClarificationQuestions) != 2 || plan.ClarificationQuestions[0] != "目标产品是什么？" {
		t.Fatalf("ClarificationQuestions = %v, want 2 questions passthrough", plan.ClarificationQuestions)
	}
	if plan.Strategy != biz.StrategyDirect {
		t.Fatalf("strategy = %q, want direct（澄清计划不进入执行管线）", plan.Strategy)
	}
	if plan.DecomposeReason != "needs_clarification" {
		t.Fatalf("DecomposeReason = %q, want needs_clarification", plan.DecomposeReason)
	}

	// progress 事件：needs_clarification 必须可见，且不得出现 decompose_failed。
	phases := bus.noticePhases()
	sawClarify := false
	for _, ph := range phases {
		if ph == "needs_clarification" {
			sawClarify = true
		}
		if ph == "decompose_failed" {
			t.Fatalf("clarification exit must not publish decompose_failed (phases=%v)", phases)
		}
	}
	if !sawClarify {
		t.Fatalf("needs_clarification progress event missing (phases=%v)", phases)
	}

	// v2 壳终态（F9/Y4 同契约）：流式壳已发，必须补 failed 终态。
	boardTerminal := false
	for _, e := range seq.events {
		if ev, ok := e.(*biz.PlanBoardUpdatedEvent); ok && ev.PlanBoard.Status == biz.PlanStatusFailed {
			boardTerminal = true
		}
	}
	if !boardTerminal {
		t.Fatal("PlanBoard never received terminal update — frontend stuck at 规划中")
	}
}

// buildDecompositionPrompt 必须携带澄清出口契约（防 prompt 回退）。
func TestBuildDecompositionPrompt_CarriesClarificationExit(t *testing.T) {
	t.Parallel()
	prompt := buildDecompositionPrompt("为新品做营销方案", nil, 0, nil)
	if !strings.Contains(prompt, "needs_clarification") {
		t.Fatal("decomposition prompt missing clarification exit contract")
	}
	if !strings.Contains(prompt, "NEVER invent product names") {
		t.Fatal("decomposition prompt missing anti-fabrication rule")
	}
}
