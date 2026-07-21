package chatactivity

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestCancelRunningActivityMessages_NilReader(t *testing.T) {
	n, err := CancelRunningActivityMessages(context.Background(), nil, nil, "sess1", loggateway.NewNoop())
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestCancelRunningActivityMessages_EmptySessionID(t *testing.T) {
	n, err := CancelRunningActivityMessages(context.Background(), &stubStepReader{}, &stubActivityRepo{}, "  ", loggateway.NewNoop())
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestCancelRunningActivityMessages_SkipsTerminalStatuses(t *testing.T) {
	reader := &stubStepReader{
		steps: []biz.Step{
			{ID: "a1", SessionID: "sess1", Status: biz.StepStatusCompleted},
			{ID: "a2", SessionID: "sess1", Status: biz.StepStatusFailed},
			{ID: "a3", SessionID: "sess1", Status: "cancelled"},
			{ID: "a4", SessionID: "sess1", Status: "interrupted"},
		},
	}
	writer := &stubActivityRepo{}
	n, err := CancelRunningActivityMessages(context.Background(), reader, writer, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 cancellations, got %d", n)
	}
	if len(writer.updated) != 0 {
		t.Fatalf("expected no updates, got %d", len(writer.updated))
	}
}

func TestCancelRunningActivityMessages_CancelsInFlight(t *testing.T) {
	reader := &stubStepReader{
		steps: []biz.Step{
			{ID: "a1", SessionID: "sess1", Status: biz.StepStatusRunning},
			{ID: "a2", SessionID: "sess1", Status: biz.StepStatusToolRunning},
			{ID: "a3", SessionID: "sess1", Status: biz.StepStatusToolBlocked},
			{ID: "a4", SessionID: "sess1", Status: biz.StepStatusCompleted},
		},
	}
	writer := &stubActivityRepo{}
	n, err := CancelRunningActivityMessages(context.Background(), reader, writer, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 cancellations, got %d", n)
	}
	if len(writer.updated) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(writer.updated))
	}
	for _, u := range writer.updated {
		if u.Status != biz.ActivityStatusCancelled {
			t.Fatalf("expected cancelled status, got %s", u.Status)
		}
	}
}

func TestCancelRunningActivityMessages_UpdateErrorContinues(t *testing.T) {
	reader := &stubStepReader{
		steps: []biz.Step{
			{ID: "a1", SessionID: "sess1", Status: biz.StepStatusRunning},
			{ID: "a2", SessionID: "sess1", Status: biz.StepStatusToolRunning},
		},
	}
	writer := &stubActivityRepo{updateErr: errUpdateFailed}
	n, err := CancelRunningActivityMessages(context.Background(), reader, writer, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 successful cancellations, got %d", n)
	}
}

func TestIsInFlightActivity(t *testing.T) {
	cases := []struct {
		status biz.ActivityStatus
		want   bool
	}{
		{biz.ActivityStatusPending, false},
		{biz.ActivityStatusRunning, true},
		{biz.ActivityStatusToolRunning, true},
		{biz.ActivityStatusToolBlocked, true},
		{biz.ActivityStatusCompleted, false},
		{biz.ActivityStatusFailed, false},
		{biz.ActivityStatusPartialFailure, false},
		{biz.ActivityStatusCancelled, false},
		{biz.ActivityStatusInterrupted, false},
	}
	for _, c := range cases {
		if got := isInFlightActivity(c.status); got != c.want {
			t.Fatalf("isInFlightActivity(%s)=%v, want %v", c.status, got, c.want)
		}
	}
}

// stubStepReader implements biz.StepV2Reader for cancel tests.
type stubStepReader struct {
	steps []biz.Step
}

func (s *stubStepReader) GetStep(_ context.Context, _ string) (biz.Step, error) {
	return biz.Step{}, nil
}

func (s *stubStepReader) ListStepsByTurn(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepReader) ListStepsByTask(_ context.Context, _ string) ([]biz.Step, error) {
	return nil, nil
}

func (s *stubStepReader) ListStepsBySession(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

func (s *stubStepReader) ListStepsBySessionID(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

// stubActivityRepo implements biz.ActivityRepo for cancel tests (writer only).
// The same instance can serve as both reader and writer.
type stubActivityRepo struct {
	activities []biz.Activity
	updated    []biz.Activity
	updateErr  error
}

func (s *stubActivityRepo) ListBySessionTurn(_ context.Context, _, _ string) ([]biz.Activity, error) {
	return nil, nil
}

func (s *stubActivityRepo) ListBySession(_ context.Context, _ string) ([]biz.Activity, error) {
	return s.activities, nil
}

func (s *stubActivityRepo) GetActivity(_ context.Context, _ string) (biz.Activity, error) {
	return biz.Activity{}, nil
}

func (s *stubActivityRepo) ListBySpiritSession(_ context.Context, _ string) ([]biz.Activity, error) {
	return nil, nil
}

func (s *stubActivityRepo) ListByTeam(_ context.Context, _ string) ([]biz.Activity, error) {
	return nil, nil
}

func (s *stubActivityRepo) ListByParentSession(_ context.Context, _ string) ([]biz.Activity, error) {
	return nil, nil
}

func (s *stubActivityRepo) CreateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return a, nil
}

func (s *stubActivityRepo) UpdateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	if s.updateErr != nil {
		return biz.Activity{}, s.updateErr
	}
	s.updated = append(s.updated, a)
	return a, nil
}

func (s *stubActivityRepo) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return a, nil
}

// errUpdateFailed is a sentinel error for stub update failures.
var errUpdateFailed = errSimple("update failed")

type errSimple string

func (e errSimple) Error() string { return string(e) }
