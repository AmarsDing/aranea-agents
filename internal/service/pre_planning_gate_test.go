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

// gateCaptureBus captures published ActivityEvents for assertion.
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
			bus := &gateCaptureBus{}
			gate := NewPrePlanningGate(
				&fakePlanner{quickLevel: tt.level, quickScore: tt.score},
				bus,
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
			if len(bus.events()) != tt.wantEventCount {
				t.Errorf("published events = %d, want %d", len(bus.events()), tt.wantEventCount)
			}
			// Verify start (created) + done (completed) events
			published := bus.events()
			if len(published) >= 2 {
				if published[0].Event != biz.ActivityEventCreated {
					t.Errorf("first event = %s, want %s", published[0].Event, biz.ActivityEventCreated)
				}
				if published[1].Event != biz.ActivityEventCompleted {
					t.Errorf("last event = %s, want %s", published[1].Event, biz.ActivityEventCompleted)
				}
				// B.4.3: pre-planning assess phase renders as NoticeBlock (Kind=notice),
				// not PlanBlock (Kind=plan), to avoid "UI 提前占位" before real plan arrives.
				if published[0].Activity.Kind != biz.ActivityKindNotice {
					t.Errorf("first activity kind = %s, want %s", published[0].Activity.Kind, biz.ActivityKindNotice)
				}
			}
		})
	}
}

func TestPrePlanningGate_Evaluate_PropagatesQuickAssessError(t *testing.T) {
	bus := &gateCaptureBus{}
	gate := NewPrePlanningGate(
		&fakePlanner{quickErr: errors.New("assess failed")},
		bus,
		loggateway.NewNoop(),
	)
	_, err := gate.Evaluate(context.Background(), biz.PlanInput{})
	if err == nil {
		t.Fatal("expected error from QuickAssess, got nil")
	}
}
