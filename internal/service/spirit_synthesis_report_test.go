package service

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── stubs ─────────────────────────────────────────────────────────────────

// stubTaskV2Reader implements biz.TaskV2Reader for testing.
type stubTaskV2Reader struct {
	tasks []biz.Task
	err   error
}

func (s *stubTaskV2Reader) GetTask(_ context.Context, id string) (biz.Task, error) {
	for _, t := range s.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return biz.Task{}, biz.ErrNotFound
}
func (s *stubTaskV2Reader) ListTasksBySession(_ context.Context, _ string) ([]biz.Task, error) {
	return s.tasks, s.err
}

// stubTurnGateway implements biz.TurnGateway for testing, recording ExecuteTurn calls.
type stubTurnGateway struct {
	executeTurnCalls int
	lastInput        biz.TurnInput
}

func (s *stubTurnGateway) ExecuteTurn(_ context.Context, in biz.TurnInput) (biz.TurnResult, error) {
	s.executeTurnCalls++
	s.lastInput = in
	return biz.TurnResult{}, nil
}
func (s *stubTurnGateway) RunNativeTurn(_ context.Context, _ biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	return biz.ChatMessage{}, biz.ChatMessage{}, nil
}
func (s *stubTurnGateway) RunNativeTurnWithOutcome(_ context.Context, _ biz.TurnInput) (biz.TurnResult, error) {
	return biz.TurnResult{}, nil
}
func (s *stubTurnGateway) HasActiveRun(_ string) bool                            { return false }
func (s *stubTurnGateway) CancelRun(_ context.Context, _ string) bool            { return false }
func (s *stubTurnGateway) SetRunStatus(_ context.Context, _, _, _, _ string)     {}
func (s *stubTurnGateway) LastPendingMessageID(_ string) string                  { return "" }

// reportEnvelope mirrors the B.10.17.4 Step.Content JSON contract for assertions.
type reportEnvelope struct {
	Version       int                       `json:"version"`
	Kind          string                    `json:"kind"`
	Content       string                    `json:"content"`
	Strategy      string                    `json:"strategy"`
	Degraded      bool                      `json:"degraded"`
	Overview      *biz.ExecutionOverview    `json:"overview"`
	TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
	Deliverables  []biz.DeliverableItem     `json:"deliverables"`
	SynthesizedAt string                    `json:"synthesized_at"`
}

func sampleSynthesisOutput() *biz.SynthesisOutput {
	return &biz.SynthesisOutput{
		Content:       "## 综合分析\n\n核心发现……",
		Strategy:      biz.SynthesisStrategyHybrid,
		SynthesizedAt: "2026-07-22T12:00:00Z",
		Overview: &biz.ExecutionOverview{
			Query: "调研量子计算", FinalStatus: "partial_failure",
			DurationMs: 12345, TotalUnits: 2, CompletedUnits: 1, FailedUnits: 1,
			TokenIn: 1000, TokenOut: 2000,
		},
		TeamResults: []biz.TeamSynthesisResult{
			{TeamID: "t1", TeamName: "调研团队", TaskName: "任务A", Status: "completed", Summary: "调研结论", KeyFindings: "发现一", DurationMs: 8000},
			{TeamID: "t2", TeamName: "分析团队", TaskName: "任务B", Status: "failed", Summary: "", ErrorMessage: "LLM 超时", DurationMs: 4000},
		},
		Deliverables: []biz.DeliverableItem{
			{NodeID: "st_1", UnitName: "调研团队", Summary: "调研结论", Type: "document", Format: "markdown", SizeChars: 500},
		},
	}
}

// ── publisher contract tests (B.10.17.4) ──────────────────────────────────

// The synthesis report must be published as a persistent StepCreatedEvent
// (Kind=notice) carrying the JSON envelope, seq preferred.
func TestSynthesisEventPublisher_StepCreatedEventContract(t *testing.T) {
	seq := &capturingSeq{}
	pub := &synthesisEventPublisher{
		seq:          seq,
		taskV2Reader: &stubTaskV2Reader{tasks: []biz.Task{{ID: "task-old"}, {ID: "task-latest"}}},
		lg:           loggateway.NewNoop(),
	}

	pub.PublishSynthesisCompleted(context.Background(), "spirit-1", sampleSynthesisOutput())

	if got := seq.count(); got != 1 {
		t.Fatalf("seq published %d events, want 1", got)
	}
	seq.mu.Lock()
	ev := seq.events[0]
	seq.mu.Unlock()
	stepEv, ok := ev.(*biz.StepCreatedEvent)
	if !ok {
		t.Fatalf("event type = %T, want *biz.StepCreatedEvent", ev)
	}
	step := stepEv.Step
	if step.Kind != biz.StepKindNotice {
		t.Errorf("Kind = %q, want notice", step.Kind)
	}
	if step.NoticeType != "synthesis_completed" {
		t.Errorf("NoticeType = %q, want synthesis_completed", step.NoticeType)
	}
	if step.Status != biz.StepStatusCompleted {
		t.Errorf("Status = %q, want completed (直发 notice 创建即完成)", step.Status)
	}
	if step.Version != 1 {
		t.Errorf("Version = %d, want 1", step.Version)
	}
	if step.StartedAt.IsZero() || step.CompletedAt == nil {
		t.Errorf("StartedAt/CompletedAt must be set, got %+v / %+v", step.StartedAt, step.CompletedAt)
	}
	if step.AuthorAgentKey != "spirit-synthesis" {
		t.Errorf("AuthorAgentKey = %q, want spirit-synthesis", step.AuthorAgentKey)
	}
	if step.SessionID != "spirit-1" || step.SpiritSessionID != "spirit-1" {
		t.Errorf("SessionID/SpiritSessionID = %q/%q, want spirit-1", step.SessionID, step.SpiritSessionID)
	}
	// TaskID 附着最近用户 Task（ListTasksBySession 升序，取最后一个）。
	if step.TaskID != "task-latest" {
		t.Errorf("TaskID = %q, want task-latest", step.TaskID)
	}
	if stepEv.TaskID() != "task-latest" {
		t.Errorf("event.TaskID() = %q, want task-latest", stepEv.TaskID())
	}

	var env reportEnvelope
	if err := json.Unmarshal([]byte(step.Content), &env); err != nil {
		t.Fatalf("Content is not a valid JSON envelope: %v\ncontent: %s", err, step.Content)
	}
	if env.Version != 1 || env.Kind != "execution_report" {
		t.Errorf("envelope version/kind = %d/%q, want 1/execution_report", env.Version, env.Kind)
	}
	if env.Content != "## 综合分析\n\n核心发现……" {
		t.Errorf("envelope content mismatch: %q", env.Content)
	}
	if env.Strategy != "hybrid" || env.SynthesizedAt != "2026-07-22T12:00:00Z" {
		t.Errorf("envelope strategy/synthesized_at = %q/%q", env.Strategy, env.SynthesizedAt)
	}
	if env.Overview == nil || env.Overview.FinalStatus != "partial_failure" || env.Overview.TokenOut != 2000 {
		t.Errorf("envelope overview mismatch: %+v", env.Overview)
	}
	if len(env.TeamResults) != 2 || env.TeamResults[1].ErrorMessage != "LLM 超时" || env.TeamResults[0].DurationMs != 8000 {
		t.Errorf("envelope team_results mismatch: %+v", env.TeamResults)
	}
	if len(env.Deliverables) != 1 || env.Deliverables[0].NodeID != "st_1" || env.Deliverables[0].SizeChars != 500 {
		t.Errorf("envelope deliverables mismatch: %+v", env.Deliverables)
	}
}

// v1-only deployment (seq=nil): fall back to eventBus (WS only).
func TestSynthesisEventPublisher_EventBusFallback(t *testing.T) {
	bus := &capturingEventBus{}
	pub := &synthesisEventPublisher{
		bus: bus,
		lg:  loggateway.NewNoop(),
	}

	pub.PublishSynthesisCompleted(context.Background(), "spirit-1", sampleSynthesisOutput())

	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("eventBus published %d events, want 1", len(events))
	}
	if _, ok := events[0].(*biz.StepCreatedEvent); !ok {
		t.Fatalf("event type = %T, want *biz.StepCreatedEvent", events[0])
	}
}

// TaskID resolution failure (reader nil / no tasks) degrades to a session-level
// notice with empty TaskID — the report must still be published.
func TestSynthesisEventPublisher_NoTask_EmptyTaskID(t *testing.T) {
	seq := &capturingSeq{}
	pub := &synthesisEventPublisher{
		seq: seq,
		lg:  loggateway.NewNoop(),
	}

	pub.PublishSynthesisCompleted(context.Background(), "spirit-1", sampleSynthesisOutput())

	if got := seq.count(); got != 1 {
		t.Fatalf("seq published %d events, want 1", got)
	}
	seq.mu.Lock()
	ev := seq.events[0]
	seq.mu.Unlock()
	step := ev.(*biz.StepCreatedEvent).Step
	if step.TaskID != "" {
		t.Errorf("TaskID = %q, want empty when no task resolvable", step.TaskID)
	}
}

// ── cancelled guard (B.10.17 断点 4) ────────────────────────────────────────

// 用户主动中断（存在 cancelled 团队）时，不得触发综合与总结 turn（中断不出报告）。
func TestCheckAllTeamsCompleted_SkipsSynthesisWhenCancelled(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("checkAllTeamsCompleted panicked (cancelled 守卫缺失？): %v", r)
		}
	}()

	gw := &stubTurnGateway{}
	s := &TeamStarter{
		team: TeamOrchestrationDeps{
			SpiritUC: &stubSpiritTeamController{
				completedResult: biz.AllTeamsCompletedResult{
					AllDone:        true,
					TotalTeams:     2,
					CompletedTeams: 1,
					CancelledTeams: 1,
				},
			},
		},
		eventBus:     &capturingEventBus{},
		lg:           loggateway.NewNoop(),
		synthesisSvc: &SpiritSynthesisService{}, // 非 nil：守卫缺失时会在 buildSynthesisMessage 中 panic
		turnGateway:  gw,
	}

	s.checkAllTeamsCompleted(context.Background(), "spirit-cancelled")

	if gw.executeTurnCalls != 0 {
		t.Errorf("ExecuteTurn called %d times, want 0 when cancelled teams exist", gw.executeTurnCalls)
	}
}
