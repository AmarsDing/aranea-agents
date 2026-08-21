package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

// newPostTurnTestOrch 装配 P2 后置澄清测试用 orchestrator：step reader/writer、
// 事件发布、session 状态翻转均可断言；LLM catalog 刻意不接线（判定器走
// skipped_preflight 失败路径的用例无需真实 LLM）。
func newPostTurnTestOrch(
	stepReader biz.StepV2Reader,
	stepWriter *stubStepV2Writer,
	seq *stubEventPublisher,
	stateMgr *stubSessionStateTransitor,
) *ChatOrchestrator {
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{
			StepReader: stepReader,
			StepWriter: stepWriter,
		},
		v2Seq:  seq,
		turnLC: &chatTurnLifecycleImpl{sessionStateTransitor: stateMgr},
		runMgr: newNoopChatRunManager(),
		infraDeps: ChatInfraDeps{
			LG: loggateway.NewNoop(),
		},
	}
}

func postTurnClarifyEnabledAgent() biz.Agent {
	return biz.Agent{
		AgentKey: "test-agent",
		Settings: &biz.AgentRuntimeSettings{ClarificationEnabled: true},
	}
}

func ctxWithRootTask(taskID string) context.Context {
	return chatagent.ContextWithRootTaskActivityID(context.Background(), chatagent.RootTaskActivityID(taskID))
}

// 守卫组：任一守卫命中都不得建卡/翻转 session/触达 LLM 判定。
func TestMaybeSuspend_Guards(t *testing.T) {
	replyWithQuestion := "我需要您补充：股票代码或交易所？"
	tests := []struct {
		name  string
		input biz.TurnInput
		agent biz.Agent
		reply string
	}{
		{"resume turn skipped", biz.TurnInput{SessionID: "sess-1", Content: "hi", ParentTaskID: "task-1"}, postTurnClarifyEnabledAgent(), replyWithQuestion},
		{"synthesis turn skipped", biz.TurnInput{SessionID: "sess-1", Content: "hi", Synthesis: true}, postTurnClarifyEnabledAgent(), replyWithQuestion},
		{"evaluation turn skipped", biz.TurnInput{SessionID: "sess-1", Content: "hi", EntryConfig: biz.TurnEntryPointConfig{EntryPoint: biz.EntryPointEvaluation}}, postTurnClarifyEnabledAgent(), replyWithQuestion},
		{"clarification disabled", biz.TurnInput{SessionID: "sess-1", Content: "hi"}, biz.Agent{Settings: &biz.AgentRuntimeSettings{ClarificationEnabled: false}}, replyWithQuestion},
		{"nil settings", biz.TurnInput{SessionID: "sess-1", Content: "hi"}, biz.Agent{}, replyWithQuestion},
		{"reply without trailing question", biz.TurnInput{SessionID: "sess-1", Content: "hi"}, postTurnClarifyEnabledAgent(), "已完成全部分析。"},
		{"empty reply", biz.TurnInput{SessionID: "sess-1", Content: "hi"}, postTurnClarifyEnabledAgent(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stepWriter := &stubStepV2Writer{}
			seq := &stubEventPublisher{}
			stateMgr := &stubSessionStateTransitor{}
			orch := newPostTurnTestOrch(&stubStepV2ReaderByID{}, stepWriter, seq, stateMgr)

			if orch.maybeSuspendTurnForClarification(ctxWithRootTask("task-1"), tt.agent, tt.input, "p", "m", tt.reply, nil) {
				t.Error("expected not suspended")
			}
			if len(stepWriter.created) != 0 {
				t.Errorf("expected no step created, got %d", len(stepWriter.created))
			}
			if len(stateMgr.statuses) != 0 {
				t.Errorf("expected no session transition, got %+v", stateMgr.statuses)
			}
		})
	}
}

// LLM catalog 未接线 → 判定器 skipped_preflight → 不挂起、无副作用（增强失效
// 静默降级，保持纯文本回复原样）。
func TestMaybeSuspend_JudgeUnavailable(t *testing.T) {
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newPostTurnTestOrch(&stubStepV2ReaderByID{}, stepWriter, seq, stateMgr)

	input := biz.TurnInput{SessionID: "sess-1", Content: "查一下金鹏科技"}
	if orch.maybeSuspendTurnForClarification(ctxWithRootTask("task-1"), postTurnClarifyEnabledAgent(), input, "p", "m", "我需要您补充：股票代码或交易所？", nil) {
		t.Error("expected not suspended when judge unavailable")
	}
	if len(stepWriter.created) != 0 {
		t.Errorf("expected no step created, got %d", len(stepWriter.created))
	}
	if len(stateMgr.statuses) != 0 {
		t.Errorf("expected no session transition, got %+v", stateMgr.statuses)
	}
}

// postTurnTaskStepReader 覆盖 ListStepsByTask 按任务过滤（共享 stub 的
// ListStepsByTask 恒返回 nil，无法满足 seq 断言）。
type postTurnTaskStepReader struct {
	*stubStepV2ReaderByID
	taskSteps []biz.Step
}

func (s *postTurnTaskStepReader) ListStepsByTask(_ context.Context, _ string) ([]biz.Step, error) {
	return s.taskSteps, nil
}

// 建卡挂起机械路径：step 结构/信封/事件/session 翻转/pending 缓存全断言。
func TestSuspendTurnWithClarificationQuestion_Mechanics(t *testing.T) {
	stepReader := &postTurnTaskStepReader{
		stubStepV2ReaderByID: &stubStepV2ReaderByID{},
		taskSteps:            []biz.Step{{ID: "reply-1", TaskID: "task-1", SessionID: "sess-1", Seq: 3}},
	}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newPostTurnTestOrch(stepReader, stepWriter, seq, stateMgr)

	input := biz.TurnInput{SessionID: "sess-1", Content: "查一下金鹏科技", AgentKey: "test-agent"}
	if !orch.suspendTurnWithClarificationQuestion(context.Background(), postTurnClarifyEnabledAgent(), input, "task-1", "请补充股票代码或交易所？", nil) {
		t.Fatal("expected suspended")
	}

	// step：kind/status/orphan/信封/seq（现有 reply seq=3 → 澄清卡 seq=4）
	if len(stepWriter.created) != 1 {
		t.Fatalf("expected 1 step created, got %d", len(stepWriter.created))
	}
	step := stepWriter.created[0]
	if step.ID != "task-1-clarify-post" {
		t.Errorf("unexpected step ID %q", step.ID)
	}
	if step.Kind != biz.StepKindClarify || step.Status != biz.StepStatusAwaitingInput {
		t.Errorf("unexpected step kind/status: %s/%s", step.Kind, step.Status)
	}
	if step.TurnID != "" {
		t.Errorf("expected orphan step (empty TurnID), got %q", step.TurnID)
	}
	if step.Seq != 4 {
		t.Errorf("expected seq=4 (max existing 3 + 1), got %d", step.Seq)
	}
	var env biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &env); err != nil {
		t.Fatalf("envelope unmarshal failed: %v", err)
	}
	if env.OriginalInput != "查一下金鹏科技" {
		t.Errorf("unexpected original_input %q", env.OriginalInput)
	}
	if len(env.Questions) != 1 || env.Questions[0].Question != "请补充股票代码或交易所？" {
		t.Errorf("unexpected questions %+v", env.Questions)
	}
	if len(env.Questions[0].Options) != 0 || len(env.Questions[0].Recommended) != 0 {
		t.Errorf("expected free-text question (no options/recommended), got %+v", env.Questions[0])
	}

	// 事件：仅 step.created（task 早已存在，不重发 task.created）
	if len(seq.published) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(seq.published))
	}

	// session → awaiting_confirmation(reason=clarification)
	if len(stateMgr.statuses) != 1 || stateMgr.statuses[0] != sessstatus.SessionStatusAwaitingConfirmation {
		t.Fatalf("expected awaiting_confirmation transition, got %+v", stateMgr.statuses)
	}
	if stateMgr.reasons[0] != sessstatus.StatusReasonClarification {
		t.Errorf("expected reason=clarification, got %q", stateMgr.reasons[0])
	}

	// pending 缓存：自由回复/卡片提交的热路径恢复
	pc, ok := orch.pendingClarifications.Load("sess-1")
	if !ok {
		t.Fatal("expected pendingClarification stored")
	}
	if pc.StepID != step.ID || pc.TaskID != "task-1" || pc.Input.Content != "查一下金鹏科技" {
		t.Errorf("unexpected pendingClarification %+v", pc)
	}
}

// step reader 缺失/查询失败的降级：Seq=1，不影响建卡。
func TestSuspendTurn_NilStepReaderFallback(t *testing.T) {
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newPostTurnTestOrch(nil, stepWriter, seq, stateMgr)

	input := biz.TurnInput{SessionID: "sess-1", Content: "hi"}
	if !orch.suspendTurnWithClarificationQuestion(context.Background(), postTurnClarifyEnabledAgent(), input, "task-1", "问题？", nil) {
		t.Fatal("expected suspended even without step reader")
	}
	if stepWriter.created[0].Seq != 1 {
		t.Errorf("expected fallback seq=1, got %d", stepWriter.created[0].Seq)
	}
}

func TestTrailingQuestionFallback(t *testing.T) {
	short := "我需要您补充：股票代码或交易所？"
	if got := trailingQuestionFallback(short); got != short {
		t.Errorf("short reply should return as-is, got %q", got)
	}
	long := strings.Repeat("分析内容。", 50) + "\n\n我需要您补充：\n1. 股票代码\n2. 时间范围？"
	got := trailingQuestionFallback(long)
	if !strings.HasPrefix(got, "我需要您补充") {
		t.Errorf("long reply should return last paragraph, got %q", got)
	}
}
