package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
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

// gateCaptureBus captures published envelopes for assertion.
type gateCaptureBus struct {
	published []contract.Envelope
}

func (b *gateCaptureBus) Publish(_ context.Context, env contract.Envelope) {
	b.published = append(b.published, env)
}
func (b *gateCaptureBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	return nil, func() {}
}
func (b *gateCaptureBus) DropCount() uint64 { return 0 }

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
			if len(bus.published) != tt.wantEventCount {
				t.Errorf("published events = %d, want %d", len(bus.published), tt.wantEventCount)
			}
			// Verify start + done events
			if len(bus.published) >= 2 {
				if bus.published[0].Type != contract.EnvelopeTypePlanningPhaseStart {
					t.Errorf("first event = %s, want %s", bus.published[0].Type, contract.EnvelopeTypePlanningPhaseStart)
				}
				if bus.published[1].Type != contract.EnvelopeTypePlanningPhaseDone {
					t.Errorf("last event = %s, want %s", bus.published[1].Type, contract.EnvelopeTypePlanningPhaseDone)
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
