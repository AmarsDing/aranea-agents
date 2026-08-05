package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// stubTaskV2Writer is a stub implementation of biz.TaskV2Writer for testing.
type stubTaskV2Writer struct {
	upserted      []biz.Task
	terminalized  []biz.Task
	err           error
}

func (s *stubTaskV2Writer) CreateTask(_ context.Context, t biz.Task) (biz.Task, error) {
	return t, s.err
}

func (s *stubTaskV2Writer) UpdateTask(_ context.Context, t biz.Task) (biz.Task, error) {
	return t, s.err
}

func (s *stubTaskV2Writer) UpsertTask(_ context.Context, t biz.Task) (biz.Task, error) {
	s.upserted = append(s.upserted, t)
	return t, s.err
}

func (s *stubTaskV2Writer) ResumeInterruptedTask(_ context.Context, id string, resumeAt time.Time) (biz.Task, bool, error) {
	return biz.Task{}, false, nil
}

func (s *stubTaskV2Writer) CompleteTaskTerminal(_ context.Context, t biz.Task) (biz.Task, error) {
	s.terminalized = append(s.terminalized, t)
	return t, nil
}

// stubStepV2Writer is a stub implementation of biz.StepV2Writer for testing.
type stubStepV2Writer struct {
	created []biz.Step
	updated []biz.Step
	err     error
}

func (s *stubStepV2Writer) CreateStep(_ context.Context, step biz.Step) (biz.Step, error) {
	s.created = append(s.created, step)
	return step, s.err
}

func (s *stubStepV2Writer) UpdateStep(_ context.Context, step biz.Step) (biz.Step, error) {
	s.updated = append(s.updated, step)
	return step, s.err
}

func (s *stubStepV2Writer) UpsertStep(_ context.Context, step biz.Step) (biz.Step, error) {
	s.created = append(s.created, step)
	return step, s.err
}

// stubEventPublisher is a stub implementation of rt.EventPublisher for testing.
type stubEventPublisher struct {
	published []biz.Event
}

func (s *stubEventPublisher) Publish(_ context.Context, e biz.Event) {
	s.published = append(s.published, e)
}

// stubSessionStateTransitor is a stub implementation of sessionStateTransitor for testing.
type stubSessionStateTransitor struct {
	statuses []sessstatus.SessionStatus
	reasons  []sessstatus.SessionStatusReason
}

func (s *stubSessionStateTransitor) TransitionStatus(_ context.Context, _ string, status sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) {
	s.statuses = append(s.statuses, status)
	s.reasons = append(s.reasons, reason)
}

// stubTaskV2Repo wraps stubTaskV2Writer to implement biz.TaskV2Repo.
type stubTaskV2Repo struct {
	writer *stubTaskV2Writer
}

func (s *stubTaskV2Repo) GetTask(_ context.Context, id string) (biz.Task, error) {
	return biz.Task{}, nil
}

func (s *stubTaskV2Repo) ListTasksBySession(_ context.Context, sessionID string) ([]biz.Task, error) {
	return nil, nil
}

func (s *stubTaskV2Repo) CreateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	return s.writer.CreateTask(ctx, t)
}

func (s *stubTaskV2Repo) UpdateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	return s.writer.UpdateTask(ctx, t)
}

func (s *stubTaskV2Repo) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	return s.writer.UpsertTask(ctx, t)
}

func (s *stubTaskV2Repo) ResumeInterruptedTask(_ context.Context, id string, resumeAt time.Time) (biz.Task, bool, error) {
	return biz.Task{}, false, nil
}

func (s *stubTaskV2Repo) CompleteTaskTerminal(ctx context.Context, t biz.Task) (biz.Task, error) {
	return s.writer.CompleteTaskTerminal(ctx, t)
}

func newClarificationTestOrch(
	taskWriter *stubTaskV2Writer,
	stepWriter *stubStepV2Writer,
	seq rt.EventPublisher,
	stateMgr sessionStateTransitor,
) *ChatOrchestrator {
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{
			TaskV2:     &stubTaskV2Repo{writer: taskWriter},
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

func TestRunClarificationGate_NotTriggeredWhenDisabled(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: false, // disabled
		},
	}
	art := &intent.Artifact{
		RiskFlags: []string{intent.RiskFlagNeedsClarification},
		Clarifications: []intent.ClarificationQuestion{
			{Question: "Q1", Mode: "single", Options: []string{"a"}, Recommended: []string{"a"}},
		},
	}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag, biz.TurnInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Triggered {
		t.Error("expected not triggered when ClarificationEnabled=false")
	}
	if len(taskWriter.upserted) != 0 {
		t.Errorf("expected no task upserted, got %d", len(taskWriter.upserted))
	}
	if len(stepWriter.created) != 0 {
		t.Errorf("expected no step created, got %d", len(stepWriter.created))
	}
}

func TestRunClarificationGate_NotTriggeredWhenNoClarifications(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}
	art := &intent.Artifact{
		RiskFlags:      []string{intent.RiskFlagNeedsClarification},
		Clarifications: nil, // no questions
	}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag, biz.TurnInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Triggered {
		t.Error("expected not triggered when no clarifications")
	}
}

func TestRunClarificationGate_NotTriggeredWhenNoRiskFlag(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}
	art := &intent.Artifact{
		RiskFlags: []string{"other_flag"}, // no needs_clarification flag
		Clarifications: []intent.ClarificationQuestion{
			{Question: "Q1", Mode: "single", Options: []string{"a"}, Recommended: []string{"a"}},
		},
	}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag, biz.TurnInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Triggered {
		t.Error("expected not triggered when no needs_clarification risk flag")
	}
}

func TestRunClarificationGate_Triggered(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		AgentKey: "test-agent",
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}
	art := &intent.Artifact{
		RiskFlags: []string{intent.RiskFlagNeedsClarification},
		Clarifications: []intent.ClarificationQuestion{
			{Question: "目标平台？", Mode: "single", Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
			{Question: "受众？", Mode: "multi", Options: []string{"开发者", "设计师"}, Recommended: []string{"开发者"}},
		},
	}

	// 注入 ctx 预生成的 RootTaskActivityID（生产路径由 chat_orchestrator_turn.go
	// 在 BUILD/IntentPass 并行后注入），澄清门必须复用它而非另造 UUID，
	// 保证 ctx 链与落库 Task ID 一致。
	ctxTaskID := "task-pregen-1"
	ctx := chatagent.ContextWithRootTaskActivityID(context.Background(), chatagent.RootTaskActivityID(ctxTaskID))
	decision, err := orch.runClarificationGate(ctx, "sess-1", art, ag, biz.TurnInput{SessionID: "sess-1", Content: "帮我做个应用"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Triggered {
		t.Fatal("expected triggered=true")
	}
	if decision.StepID == "" {
		t.Error("expected non-empty StepID")
	}

	// Verify task was upserted
	if len(taskWriter.upserted) != 1 {
		t.Fatalf("expected 1 task upserted, got %d", len(taskWriter.upserted))
	}
	task := taskWriter.upserted[0]
	if task.SessionID != "sess-1" {
		t.Errorf("task.SessionID = %q, want %q", task.SessionID, "sess-1")
	}
	if task.Status != biz.TaskStatusRunning {
		t.Errorf("task.Status = %q, want %q", task.Status, biz.TaskStatusRunning)
	}
	if task.UserMessage != "帮我做个应用" {
		t.Errorf("task.UserMessage = %q, want %q", task.UserMessage, "帮我做个应用")
	}
	if task.ID != ctxTaskID {
		t.Errorf("task.ID = %q, want ctx-carried preGeneratedTaskID %q", task.ID, ctxTaskID)
	}

	// Verify step was created
	if len(stepWriter.created) != 1 {
		t.Fatalf("expected 1 step created, got %d", len(stepWriter.created))
	}
	step := stepWriter.created[0]
	if step.Kind != biz.StepKindClarify {
		t.Errorf("step.Kind = %q, want %q", step.Kind, biz.StepKindClarify)
	}
	if step.Status != biz.StepStatusAwaitingInput {
		t.Errorf("step.Status = %q, want %q", step.Status, biz.StepStatusAwaitingInput)
	}
	if step.TaskID != task.ID {
		t.Errorf("step.TaskID = %q, want %q", step.TaskID, task.ID)
	}

	// Verify content JSON
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		t.Fatalf("failed to unmarshal step content: %v", err)
	}
	if envelope.Kind != "clarification" {
		t.Errorf("envelope.Kind = %q, want %q", envelope.Kind, "clarification")
	}
	if len(envelope.Questions) != 2 {
		t.Errorf("envelope.Questions len = %d, want 2", len(envelope.Questions))
	}
	if envelope.OriginalInput != "帮我做个应用" {
		t.Errorf("envelope.OriginalInput = %q, want %q", envelope.OriginalInput, "帮我做个应用")
	}

	// Verify events were published: task.created 必须先于 step.created——
	// 前端 TaskCard 需先有 Task 才能挂载 orphan clarify step，否则澄清卡片不渲染。
	if len(seq.published) != 2 {
		t.Fatalf("expected 2 events published, got %d", len(seq.published))
	}
	if _, ok := seq.published[0].(*biz.TaskCreatedEvent); !ok {
		t.Errorf("expected first event TaskCreatedEvent, got %T", seq.published[0])
	}
	if _, ok := seq.published[1].(*biz.StepCreatedEvent); !ok {
		t.Errorf("expected second event StepCreatedEvent, got %T", seq.published[1])
	}

	// Verify session status was transitioned
	if len(stateMgr.statuses) != 1 {
		t.Fatalf("expected 1 status transition, got %d", len(stateMgr.statuses))
	}
	if stateMgr.statuses[0] != sessstatus.SessionStatusAwaitingConfirmation {
		t.Errorf("status = %q, want %q", stateMgr.statuses[0], sessstatus.SessionStatusAwaitingConfirmation)
	}
	if stateMgr.reasons[0] != sessstatus.StatusReasonClarification {
		t.Errorf("reason = %q, want %q", stateMgr.reasons[0], sessstatus.StatusReasonClarification)
	}
}

func TestRunClarificationGate_SkippedForContinuationTurn(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}
	art := &intent.Artifact{
		RiskFlags: []string{intent.RiskFlagNeedsClarification},
		Clarifications: []intent.ClarificationQuestion{
			{Question: "Q1", Mode: "single", Options: []string{"a"}, Recommended: []string{"a"}},
		},
	}
	// 续跑 turn（ParentTaskID 非空）不得再次触发澄清门，否则澄清循环。
	input := biz.TurnInput{SessionID: "sess-1", Content: "澄清上下文 + 原始需求", ParentTaskID: "task-existing"}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Triggered {
		t.Error("expected not triggered for continuation turn (ParentTaskID set)")
	}
	if len(stepWriter.created) != 0 {
		t.Errorf("expected no step created, got %d", len(stepWriter.created))
	}
}

func TestRunClarificationGate_NilArtifact(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", nil, ag, biz.TurnInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Triggered {
		t.Error("expected not triggered when artifact is nil")
	}
}
