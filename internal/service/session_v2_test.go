package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
)

// TestSessionV2Service_ListSteps verifies ListSteps delegates to the v2
// StepV2Reader and converts biz.Step → proto StepV2 correctly.
func TestSessionV2Service_ListSteps(t *testing.T) {
	svc := &SessionV2Service{
		stepReader: &stubStepV2Reader{
			steps: []biz.Step{
				{ID: "s1", SessionID: "sess1", Kind: biz.StepKindReply, Content: "hello"},
			},
		},
	}
	resp, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "sess1"})
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Id != "s1" {
		t.Fatalf("unexpected: %+v", resp.Steps)
	}
	if resp.Steps[0].Kind != string(biz.StepKindReply) {
		t.Fatalf("kind mismatch: got %q want %q", resp.Steps[0].Kind, biz.StepKindReply)
	}
}

// TestSessionV2Service_GetStep verifies GetStep returns the step by ID.
func TestSessionV2Service_GetStep(t *testing.T) {
	svc := &SessionV2Service{
		stepReader: &stubStepV2Reader{
			steps: []biz.Step{
				{ID: "s1", SessionID: "sess1", Kind: biz.StepKindReply, Content: "hello"},
			},
		},
	}
	resp, err := svc.GetStep(context.Background(), &v1.GetStepV2Request{StepId: "s1"})
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if resp.Step == nil || resp.Step.Id != "s1" {
		t.Fatalf("unexpected: %+v", resp.Step)
	}
}

// TestSessionV2Service_GetStep_NotFound verifies GetStep returns biz.ErrNotFound
// when the step does not exist.
func TestSessionV2Service_GetStep_NotFound(t *testing.T) {
	svc := &SessionV2Service{
		stepReader: &stubStepV2Reader{steps: nil},
	}
	_, err := svc.GetStep(context.Background(), &v1.GetStepV2Request{StepId: "missing"})
	if err == nil {
		t.Fatal("expected error for missing step, got nil")
	}
}

// stubStepV2Reader embeds biz.StepV2Reader so only the methods exercised by
// the tests need to be stubbed. Calling an unstubbed method panics — acceptable
// for these focused unit tests.
type stubStepV2Reader struct {
	biz.StepV2Reader
	steps []biz.Step
}

func (s *stubStepV2Reader) ListStepsBySession(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

func (s *stubStepV2Reader) GetStep(_ context.Context, id string) (biz.Step, error) {
	for _, st := range s.steps {
		if st.ID == id {
			return st, nil
		}
	}
	return biz.Step{}, biz.ErrNotFound
}
