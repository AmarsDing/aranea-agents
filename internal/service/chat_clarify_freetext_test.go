package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

// stubStepV2ReaderByID is a map-based stub implementation of biz.StepV2Reader
// for clarify tests (session_v2_test.go owns the slice-based stubStepV2Reader).
type stubStepV2ReaderByID struct {
	steps map[string]biz.Step
}

func (s *stubStepV2ReaderByID) GetStep(_ context.Context, id string) (biz.Step, error) {
	if step, ok := s.steps[id]; ok {
		return step, nil
	}
	return biz.Step{}, errStepNotFoundForTest
}

func (s *stubStepV2ReaderByID) ListStepsByTurn(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepV2ReaderByID) ListStepsByTask(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepV2ReaderByID) ListStepsBySession(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepV2ReaderByID) ListStepsBySessionPaged(_ context.Context, _ string, _ biz.StepListOptions) ([]biz.Step, bool, error) {
	return nil, false, nil
}

func (s *stubStepV2ReaderByID) ListStepsBySpiritSession(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepV2ReaderByID) ListStepsBySessionID(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepV2ReaderByID) MaxSeqBySpiritSession(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

var errStepNotFoundForTest = &testStepNotFoundError{}

type testStepNotFoundError struct{}

func (e *testStepNotFoundError) Error() string { return "step not found" }

// recordingStepV2Writer records UpdateStep calls for assertion.
type recordingStepV2Writer struct {
	stubStepV2Writer
	updated []biz.Step
}

func (s *recordingStepV2Writer) UpdateStep(_ context.Context, step biz.Step) (biz.Step, error) {
	s.updated = append(s.updated, step)
	return step, s.err
}

func newClarifyFreeTextTestOrch(
	stepReader *stubStepV2ReaderByID,
	stepWriter *recordingStepV2Writer,
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

func seedPendingClarifyStep(t *testing.T, sessionID string) (biz.Step, pendingClarification) {
	t.Helper()
	taskID := "task-clarify-1"
	envelope := biz.ClarificationEnvelope{
		Version: 1,
		Kind:    biz.ClarificationEnvelopeKind,
		Questions: []biz.ClarificationQuestion{
			{Question: "平台？", Mode: biz.ClarificationModeSingle, Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
		},
		OriginalInput: "帮我做个应用",
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	step := biz.Step{
		ID:        taskID + "-clarify",
		TaskID:    taskID,
		SessionID: sessionID,
		Kind:      biz.StepKindClarify,
		Status:    biz.StepStatusAwaitingInput,
		Version:   1,
		Content:   string(raw),
		StartedAt: time.Now().UTC(),
	}
	pc := pendingClarification{
		Input:     biz.TurnInput{SessionID: sessionID, Content: "帮我做个应用", AgentKey: "agent-1"},
		StepID:    step.ID,
		TaskID:    taskID,
		CreatedAt: time.Now().UTC(),
	}
	return step, pc
}

func TestResolveClarificationFreeText_NoPending_PassThrough(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	input := biz.TurnInput{SessionID: "sess-none", Content: "普通消息"}
	got := orch.resolveClarificationFreeText(context.Background(), input)
	if got.Content != "普通消息" {
		t.Errorf("content rewritten without pending: %q", got.Content)
	}
}

func TestResolveClarificationFreeText_EmptyContent_PassThrough(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	step, pc := seedPendingClarifyStep(t, "sess-1")
	orch.pendingClarifications.Store("sess-1", pc)
	_ = step
	got := orch.resolveClarificationFreeText(context.Background(), biz.TurnInput{SessionID: "sess-1", Content: "  "})
	if got.Content != "  " {
		t.Errorf("empty content should pass through unchanged, got %q", got.Content)
	}
}

func TestResolveClarificationFreeText_Hit_CompletesStepAndRewritesInput(t *testing.T) {
	stepReader := &stubStepV2ReaderByID{steps: map[string]biz.Step{}}
	stepWriter := &recordingStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarifyFreeTextTestOrch(stepReader, stepWriter, seq, stateMgr)

	step, pc := seedPendingClarifyStep(t, "sess-1")
	stepReader.steps[step.ID] = step
	orch.pendingClarifications.Store("sess-1", pc)

	got := orch.resolveClarificationFreeText(context.Background(), biz.TurnInput{SessionID: "sess-1", Content: "做成内部工具即可"})

	// Step 完成并回写 free_text
	if len(stepWriter.updated) != 1 {
		t.Fatalf("expected 1 step update, got %d", len(stepWriter.updated))
	}
	updated := stepWriter.updated[0]
	if updated.Status != biz.StepStatusCompleted {
		t.Errorf("step.Status = %q, want %q", updated.Status, biz.StepStatusCompleted)
	}
	if updated.CompletedAt == nil {
		t.Error("step.CompletedAt should be set")
	}
	var env biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(updated.Content), &env); err != nil {
		t.Fatalf("unmarshal updated content: %v", err)
	}
	if env.FreeText != "做成内部工具即可" {
		t.Errorf("env.FreeText = %q, want %q", env.FreeText, "做成内部工具即可")
	}
	if len(env.Answers) != len(env.Questions) {
		t.Errorf("env.Answers len = %d, want %d（空作答按推荐）", len(env.Answers), len(env.Questions))
	}

	// StepUpdated 事件发布
	if len(seq.published) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(seq.published))
	}
	if _, ok := seq.published[0].(*biz.StepUpdatedEvent); !ok {
		t.Errorf("expected StepUpdatedEvent, got %T", seq.published[0])
	}

	// Session 恢复 running
	if len(stateMgr.statuses) != 1 || stateMgr.statuses[0] != sessstatus.SessionStatusRunning {
		t.Errorf("session transitions = %v, want [running]", stateMgr.statuses)
	}

	// pending 清除
	if _, ok := orch.pendingClarifications.Load("sess-1"); ok {
		t.Error("pendingClarification should be deleted after free-text resolution")
	}

	// 输入重写：澄清上下文 + 原始需求，保留 pc.Input 其他字段
	if !strings.Contains(got.Content, "做成内部工具即可") || !strings.Contains(got.Content, "帮我做个应用") {
		t.Errorf("rewritten content missing clarified context or original input: %q", got.Content)
	}
	if got.AgentKey != "agent-1" {
		t.Errorf("rewritten input lost AgentKey: %q", got.AgentKey)
	}
	// 续跑 turn 必须挂接 gate 创建的 Task（同一任务卡片下展示澄清+执行）
	if got.ParentTaskID != pc.TaskID {
		t.Errorf("got.ParentTaskID = %q, want %q", got.ParentTaskID, pc.TaskID)
	}
}

func TestResolveClarificationFreeText_StepNotAwaiting_ClearsPendingAndPassThrough(t *testing.T) {
	stepReader := &stubStepV2ReaderByID{steps: map[string]biz.Step{}}
	stepWriter := &recordingStepV2Writer{}
	orch := newClarifyFreeTextTestOrch(stepReader, stepWriter, &stubEventPublisher{}, &stubSessionStateTransitor{})

	step, pc := seedPendingClarifyStep(t, "sess-1")
	step.Status = biz.StepStatusCompleted // 已通过卡片提交
	stepReader.steps[step.ID] = step
	orch.pendingClarifications.Store("sess-1", pc)

	input := biz.TurnInput{SessionID: "sess-1", Content: "新消息"}
	got := orch.resolveClarificationFreeText(context.Background(), input)
	if got.Content != "新消息" {
		t.Errorf("content should pass through when step not awaiting, got %q", got.Content)
	}
	if len(stepWriter.updated) != 0 {
		t.Errorf("no step update expected, got %d", len(stepWriter.updated))
	}
	if _, ok := orch.pendingClarifications.Load("sess-1"); ok {
		t.Error("stale pendingClarification should be cleared")
	}
}

func TestResolveResumeInput_PendingHit_ReturnsStoredInput(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	_, pc := seedPendingClarifyStep(t, "sess-1")
	orch.pendingClarifications.Store("sess-1", pc)

	got, err := orch.resolveResumeInput("sess-1", pc.TaskID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "帮我做个应用" || got.AgentKey != "agent-1" {
		t.Errorf("resolved input = %+v", got)
	}
	if _, ok := orch.pendingClarifications.Load("sess-1"); ok {
		t.Error("pending should be consumed after resolve")
	}
}

func TestResolveResumeInput_PendingHitTaskMismatch_Error(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	_, pc := seedPendingClarifyStep(t, "sess-1")
	orch.pendingClarifications.Store("sess-1", pc)

	if _, err := orch.resolveResumeInput("sess-1", "other-task", ""); err == nil {
		t.Fatal("expected task mismatch error")
	}
}

func TestResolveResumeInput_PendingMiss_LazyRebuildFromOriginalInput(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	// 模拟服务重启：pending 丢失，信封含 original_input
	got, err := orch.resolveResumeInput("sess-1", "any-task", "帮我做个应用")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "sess-1" || got.Content != "帮我做个应用" {
		t.Errorf("rebuilt input = %+v", got)
	}
}

func TestResolveResumeInput_PendingMissNoOriginalInput_NotFound(t *testing.T) {
	orch := newClarifyFreeTextTestOrch(&stubStepV2ReaderByID{}, &recordingStepV2Writer{}, &stubEventPublisher{}, &stubSessionStateTransitor{})
	if _, err := orch.resolveResumeInput("sess-1", "any-task", ""); err == nil {
		t.Fatal("expected NotFound error when no pending and no original input")
	}
}
