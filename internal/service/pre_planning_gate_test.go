package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakePlanner implements biz.TaskPlannerPort for testing PrePlanningGate.
type fakePlanner struct {
	quickLevel biz.ComplexityLevel
	quickScore float64
	quickErr   error
}

var _ biz.TaskPlannerPort = (*fakePlanner)(nil)

func (f *fakePlanner) Plan(_ context.Context, _ biz.PlanInput) (*biz.TaskPlan, error) {
	return nil, nil
}
func (f *fakePlanner) QuickAssess(_ context.Context, _ biz.PlanInput) (biz.ComplexityLevel, float64, error) {
	return f.quickLevel, f.quickScore, f.quickErr
}
func (f *fakePlanner) GetPlan(_ context.Context, _ string) (*biz.TaskPlan, error) {
	return nil, nil
}
func (f *fakePlanner) ListPlans(_ context.Context, _ string) ([]*biz.TaskPlan, error) {
	return nil, nil
}
func (f *fakePlanner) ConfirmPlan(_ context.Context, _ string, _ biz.PlanAdjustments) (*biz.TaskPlan, error) {
	return nil, nil
}
func (f *fakePlanner) PublishV2Board(_ context.Context, _ *biz.TaskPlan, _ *biz.AllocationPlan, _ string) (biz.PlanBoard, error) {
	return biz.PlanBoard{}, nil
}

func TestPrePlanningGate_Evaluate(t *testing.T) {
	tests := []struct {
		name           string
		level          biz.ComplexityLevel
		score          float64
		wantForce      bool
		wantEventCount int
	}{
		{
			name:           "simple does not force planning",
			level:          biz.ComplexitySimple,
			score:          0.15,
			wantForce:      false,
			wantEventCount: 2, // start + done
		},
		{
			name:           "moderate forces planning",
			level:          biz.ComplexityModerate,
			score:          0.45,
			wantForce:      true,
			wantEventCount: 2,
		},
		{
			name:           "complex forces planning",
			level:          biz.ComplexityComplex,
			score:          0.75,
			wantForce:      true,
			wantEventCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &captureEventBus{}
			gate := NewPrePlanningGate(
				&fakePlanner{quickLevel: tt.level, quickScore: tt.score},
				bus,
				nil,
				loggateway.NewNoop(),
			)

			decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
				UserMessage:     "test",
				SpiritSessionID: "sess-1",
			})
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}
			if decision.Level != tt.level {
				t.Errorf("level = %s, want %s", decision.Level, tt.level)
			}
			if decision.ForcePlanning != tt.wantForce {
				t.Errorf("ForcePlanning = %v, want %v", decision.ForcePlanning, tt.wantForce)
			}
			if decision.Score != tt.score {
				t.Errorf("Score = %.4f, want %.4f", decision.Score, tt.score)
			}
			published := bus.snapshot()
			if len(published) != tt.wantEventCount {
				t.Errorf("published events = %d, want %d", len(published), tt.wantEventCount)
			}
			// 2026-07-21 P1-5 F3：直发 notice step 必须自终态（Completed）且
			// 携带 StartedAt/CompletedAt/Version，否则 DB 中留下永久 running 的
			// 僵尸步骤（无后续事件会再更新它）。
			if len(published) >= 2 {
				for i, ev := range published {
					stepEv, ok := ev.(*biz.StepCreatedEvent)
					if !ok {
						t.Fatalf("event[%d] type = %T, want *biz.StepCreatedEvent", i, ev)
					}
					if stepEv.EventKind() != biz.EventKindStepCreated {
						t.Errorf("event[%d] kind = %s, want %s", i, stepEv.EventKind(), biz.EventKindStepCreated)
					}
					if stepEv.Step.Status != biz.StepStatusCompleted {
						t.Errorf("event[%d] step status = %s, want %s", i, stepEv.Step.Status, biz.StepStatusCompleted)
					}
					if stepEv.Step.StartedAt.IsZero() {
						t.Errorf("event[%d] step StartedAt is zero", i)
					}
					if stepEv.Step.CompletedAt == nil || stepEv.Step.CompletedAt.IsZero() {
						t.Errorf("event[%d] step CompletedAt is nil/zero", i)
					}
					if stepEv.Step.Version != 1 {
						t.Errorf("event[%d] step version = %d, want 1", i, stepEv.Step.Version)
					}
				}
				// B.4.3: pre-planning assess phase renders as NoticeBlock (Kind=notice),
				// not PlanBlock (Kind=plan), to avoid "UI 提前占位" before real plan arrives.
				startEv := published[0].(*biz.StepCreatedEvent)
				if startEv.Step.Kind != biz.StepKindNotice {
					t.Errorf("first step kind = %s, want %s", startEv.Step.Kind, biz.StepKindNotice)
				}
			}
		})
	}
}

func TestPrePlanningGate_Evaluate_PropagatesQuickAssessError(t *testing.T) {
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickErr: errors.New("assess failed")},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	_, err := gate.Evaluate(context.Background(), biz.PlanInput{})
	if err == nil {
		t.Fatal("expected error from QuickAssess, got nil")
	}
}

func TestRunPrePlanningGate_SkipsContinuationTurn(t *testing.T) {
	// 续跑 turn（synthesis/澄清续答，ParentTaskID 非空）必须跳过门控：复杂度在
	// 根 turn 已评估，重评会重复发布 session 级孤儿 notice（2026-07-27 排查：
	// 7 对重复 = 2 根 turn + 2 澄清续跑 + 3 总结 turn），且 forcedPlanning
	// 系统提示注入 synthesis turn 会强制其再走规划路径。
	orch := &ChatOrchestrator{} // 入口即返回，无需任何依赖
	decision, err := orch.runPrePlanningGate(context.Background(), biz.TurnInput{
		SessionID:    "sess-1",
		Content:      "所有团队已完成，请输出总结",
		ParentTaskID: "task-1",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ForcePlanning {
		t.Error("continuation turn must not force planning")
	}
	if decision.Level != "" {
		t.Errorf("continuation turn must not assess complexity, got level = %s", decision.Level)
	}
}

func TestPrePlanningGate_Evaluate_AttachesTaskID(t *testing.T) {
	// 门控 notice 必须挂接到本 turn 所属根 Task（PlanInput.TaskID，由调用方从
	// ctx RootTaskActivityID 解析，与澄清门同款模式），否则成为 session 级孤儿
	// 步骤：前端 getTaskOrphanSteps 按 TaskID 匹配，无 TaskID 的 notice 永不
	// 渲染且污染 DB。
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexitySimple, quickScore: 0.1},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	_, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "test",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	published := bus.snapshot()
	if len(published) == 0 {
		t.Fatal("expected notice events to be published")
	}
	for i, ev := range published {
		stepEv, ok := ev.(*biz.StepCreatedEvent)
		if !ok {
			t.Fatalf("event[%d] type = %T, want *biz.StepCreatedEvent", i, ev)
		}
		if stepEv.Step.TaskID != "task-1" {
			t.Errorf("event[%d] step TaskID = %q, want %q", i, stepEv.Step.TaskID, "task-1")
		}
	}
}
