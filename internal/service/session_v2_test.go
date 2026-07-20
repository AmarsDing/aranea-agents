package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
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

// TestSessionService_ListActivities_DelegatesToV2 verifies that after the
// Phase 3b-D Task 4 migration, SessionService.ListActivities delegates to
// SessionV2Service.ListSteps and converts v2 StepV2 → v1 Activity.
func TestSessionService_ListActivities_DelegatesToV2(t *testing.T) {
	v2Svc := &SessionV2Service{
		stepReader: &stubStepV2Reader{
			steps: []biz.Step{
				{
					ID:             "step-1",
					SessionID:      "sess1",
					TurnID:         "turn1",
					Kind:           biz.StepKindReply,
					Content:        "hello world",
					IsFinal:        true,
					NoticeType:     "model_router",
					AuthorAgentKey: "agent-1",
				},
			},
		},
	}
	svc := &SessionService{sessionV2: v2Svc}

	resp, err := svc.ListActivities(context.Background(), &v1.ListActivitiesRequest{SessionId: "sess1"})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(resp.GetItems()))
	}
	act := resp.GetItems()[0]
	if act.Id != "step-1" || act.Kind != string(biz.StepKindReply) || act.Content != "hello world" {
		t.Fatalf("activity field mapping mismatch: %+v", act)
	}
	if act.SessionId != "sess1" || act.TurnId != "turn1" {
		t.Fatalf("session/turn mapping mismatch: %+v", act)
	}
	if act.AgentKey != "agent-1" {
		t.Fatalf("agent_key mapping mismatch: %q", act.AgentKey)
	}
	// MetaJson should contain is_final + notice_type + agent_key.
	if act.MetaJson == "" {
		t.Fatal("expected non-empty meta_json with is_final/notice_type/agent_key")
	}
}

// TestSessionService_ListActivities_RequiresSessionID verifies that empty
// session_id is rejected with BadRequest (Phase 3b-D Task 4 validation).
func TestSessionService_ListActivities_RequiresSessionID(t *testing.T) {
	svc := &SessionService{sessionV2: &SessionV2Service{}}
	_, err := svc.ListActivities(context.Background(), &v1.ListActivitiesRequest{SessionId: ""})
	if err == nil {
		t.Fatal("expected error for empty session_id, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest, got %v", err)
	}
}

// TestSessionService_ListActivities_NotConfigured verifies that a nil
// sessionV2 returns an Internal error (defensive guard for misconfigured Wire).
func TestSessionService_ListActivities_NotConfigured(t *testing.T) {
	svc := &SessionService{}
	_, err := svc.ListActivities(context.Background(), &v1.ListActivitiesRequest{SessionId: "sess1"})
	if err == nil {
		t.Fatal("expected error for unconfigured service, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected Internal, got %v", err)
	}
}
