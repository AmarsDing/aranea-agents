package service

import (
	"context"
	"errors"
	"sync"
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

// gateCaptureBus captures published v1 ActivityEvents for assertion.
// Used by tests that still call v1-only publishers (PublishRunStatus / PublishSessionStatusChanged).
// v2-migrated tests use captureEventBus instead.
type gateCaptureBus struct {
	mu        sync.Mutex
	published []biz.ActivityEvent
}

func (b *gateCaptureBus) Publish(_ context.Context, ev biz.ActivityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}
func (b *gateCaptureBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return nil, func() {}
}
func (b *gateCaptureBus) DropCount() uint64 { return 0 }

func (b *gateCaptureBus) events() []biz.ActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.ActivityEvent, len(b.published))
	copy(out, b.published)
	return out
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
			// Verify start (running) + done (completed) events as v2 StepCreatedEvents.
			if len(published) >= 2 {
				startEv, ok := published[0].(*biz.StepCreatedEvent)
				if !ok {
					t.Fatalf("first event type = %T, want *biz.StepCreatedEvent", published[0])
				}
				if startEv.EventKind() != biz.EventKindStepCreated {
					t.Errorf("first event kind = %s, want %s", startEv.EventKind(), biz.EventKindStepCreated)
				}
				if startEv.Step.Status != biz.StepStatusRunning {
					t.Errorf("first step status = %s, want %s", startEv.Step.Status, biz.StepStatusRunning)
				}
				doneEv, ok := published[1].(*biz.StepCreatedEvent)
				if !ok {
					t.Fatalf("last event type = %T, want *biz.StepCreatedEvent", published[1])
				}
				if doneEv.Step.Status != biz.StepStatusCompleted {
					t.Errorf("last step status = %s, want %s", doneEv.Step.Status, biz.StepStatusCompleted)
				}
				// B.4.3: pre-planning assess phase renders as NoticeBlock (Kind=notice),
				// not PlanBlock (Kind=plan), to avoid "UI 提前占位" before real plan arrives.
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
