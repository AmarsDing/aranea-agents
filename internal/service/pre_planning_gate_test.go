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
	plans      []*biz.TaskPlan
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
	return f.plans, nil
}
func (f *fakePlanner) ConfirmPlan(_ context.Context, _ string, _ biz.PlanAdjustments) (*biz.TaskPlan, error) {
	return nil, nil
}
func (f *fakePlanner) PublishV2Board(_ context.Context, _ *biz.TaskPlan, _ *biz.AllocationPlan, _ string) (biz.PlanBoard, error) {
	return biz.PlanBoard{}, nil
}

func TestPrePlanningGate_Evaluate(t *testing.T) {
	tests := []struct {
		name       string
		level      biz.ComplexityLevel
		score      float64
		wantForce  bool
		wantReason string
	}{
		{
			name:      "simple does not force planning",
			level:     biz.ComplexitySimple,
			score:     0.15,
			wantForce: false,
			// 2026-07-28：simple 决策文案改中性表述——门控不承诺"直接回答"
			// （LLM 仍可能自主走 plan_and_execute），只陈述评估结论。
			wantReason: "评估完成：简单任务",
		},
		{
			name:      "moderate without team evidence does not force planning",
			level:     biz.ComplexityModerate,
			score:     0.45,
			wantForce: false,
			wantReason: "评估完成：中等任务（无组队证据，不强制规划）",
		},
		{
			name:       "complex forces planning",
			level:      biz.ComplexityComplex,
			score:      0.75,
			wantForce:  true,
			wantReason: "评估完成：复杂任务，强制走规划路径",
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
			if decision.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", decision.Reason, tt.wantReason)
			}
			// 2026-07-28：不再发布评估阶段通知到前端，这些是内部状态变化，
			// 对用户无意义，且会污染会话时间线。
			published := bus.snapshot()
			if len(published) != 0 {
				t.Errorf("published events = %d, want 0 (no events should be published)", len(published))
			}
		})
	}
}

func TestPrePlanningGate_Evaluate_FactQueryDoesNotForcePlanning(t *testing.T) {
	// 词法 LooksLikeFactQuery（已对任务动作词否决）是轻档路由，不是评分器豁免。
	// Moderate 天气问询不得 ForcePlanning，避免空跑 plan_and_execute。
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexityModerate, quickScore: 0.45},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "明天天气怎么样",
		SpiritSessionID: "sess-weather",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if decision.ForcePlanning {
		t.Fatalf("lexical fact query must not force planning: %+v", decision)
	}
	if decision.Reason != "评估完成：中等任务（事实查询，不强制规划）" {
		t.Fatalf("Reason = %q", decision.Reason)
	}
}

func TestPrePlanningGate_Evaluate_TaskActionWithoutDispatchDoesNotForcePlanning(t *testing.T) {
	// 含任务动作词但无组队证据：intent pass 仍跑（shouldSkipIntentPass），
	// 不得 ForcePlanning（S09-t1 / 单交付物）。
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexityModerate, quickScore: 0.45},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "核对昨天的天气数据并生成巡检报告",
		SpiritSessionID: "sess-weather-report",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if decision.ForcePlanning {
		t.Fatalf("task-action without dispatch must not force planning: %+v", decision)
	}
}

func TestPrePlanningGate_Evaluate_DispatchForcesPlanning(t *testing.T) {
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexityModerate, quickScore: 0.45},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。",
		SpiritSessionID: "sess-s06",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if !decision.ForcePlanning {
		t.Fatalf("dispatch signal must force planning: %+v", decision)
	}
}

func TestPrePlanningGate_Evaluate_DirectAnswerDoesNotForcePlanning(t *testing.T) {
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexityModerate, quickScore: 0.45},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "推荐三本关于分布式系统的书",
		SpiritSessionID: "sess-books",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if decision.ForcePlanning {
		t.Fatalf("direct-answer request must not force planning: %+v", decision)
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

func TestLatestPlanUserMessage(t *testing.T) {
	if got := latestPlanUserMessage(context.Background(), nil, "sess-1"); got != "" {
		t.Fatalf("nil planner = %q", got)
	}
	p := &fakePlanner{plans: []*biz.TaskPlan{{UserMessage: "组建几个团队，分析金鹏科技行情"}}}
	got := latestPlanUserMessage(context.Background(), p, "sess-1")
	if got != "组建几个团队，分析金鹏科技行情" {
		t.Fatalf("got %q", got)
	}
}

func TestPrePlanningGate_Evaluate_AttachesTaskID(t *testing.T) {
	// 2026-07-28：不再发布评估阶段通知到前端，因此不再检查 TaskID 挂接。
	// 门控决策完全通过返回值传递，不依赖事件发布。
	bus := &captureEventBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickLevel: biz.ComplexitySimple, quickScore: 0.1},
		bus,
		nil,
		loggateway.NewNoop(),
	)
	decision, err := gate.Evaluate(context.Background(), biz.PlanInput{
		UserMessage:     "test",
		SpiritSessionID: "sess-1",
		TaskID:          "task-1",
	})
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if decision.Level != biz.ComplexitySimple {
		t.Errorf("level = %s, want %s", decision.Level, biz.ComplexitySimple)
	}
	// 验证不再发布事件
	published := bus.snapshot()
	if len(published) != 0 {
		t.Errorf("published events = %d, want 0 (no events should be published)", len(published))
	}
}
