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
	n, err := CancelRunningActivityMessages(context.Background(), &stubStepReader{}, &stubStepWriter{}, "  ", loggateway.NewNoop())
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestCancelRunningActivityMessages_SkipsTerminalStatuses(t *testing.T) {
	reader := &stubStepReader{
		steps: []biz.Step{
			{ID: "a1", SessionID: "sess1", Status: biz.StepStatusCompleted},
			{ID: "a2", SessionID: "sess1", Status: biz.StepStatusFailed},
			{ID: "a3", SessionID: "sess1", Status: biz.StepStatusCancelled},
		},
	}
	writer := &stubStepWriter{}
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
	writer := &stubStepWriter{}
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
		if u.Status != biz.StepStatusCancelled {
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
	writer := &stubStepWriter{updateErr: errUpdateFailed}
	n, err := CancelRunningActivityMessages(context.Background(), reader, writer, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 successful cancellations, got %d", n)
	}
}

func TestIsInFlightStep(t *testing.T) {
	cases := []struct {
		status biz.StepStatus
		want   bool
	}{
		{biz.StepStatusPending, false},
		{biz.StepStatusRunning, true},
		{biz.StepStatusToolRunning, true},
		{biz.StepStatusToolBlocked, true},
		{biz.StepStatusCompleted, false},
		{biz.StepStatusFailed, false},
		{biz.StepStatusCancelled, false},
	}
	for _, c := range cases {
		if got := isInFlightStep(c.status); got != c.want {
			t.Fatalf("isInFlightStep(%s)=%v, want %v", c.status, got, c.want)
		}
	}
}

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

func (s *stubStepReader) MaxSeqBySpiritSession(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (s *stubStepReader) ListStepsBySession(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

func (s *stubStepReader) ListStepsBySessionPaged(_ context.Context, _ string, _ biz.StepListOptions) ([]biz.Step, bool, error) {
	return s.steps, false, nil
}

func (s *stubStepReader) ListStepsBySpiritSession(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

func (s *stubStepReader) ListStepsBySessionID(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}

type stubStepWriter struct {
	updated   []biz.Step
	updateErr error
}

func (s *stubStepWriter) CreateStep(_ context.Context, st biz.Step) (biz.Step, error) {
	return st, nil
}

func (s *stubStepWriter) UpdateStep(_ context.Context, st biz.Step) (biz.Step, error) {
	if s.updateErr != nil {
		return biz.Step{}, s.updateErr
	}
	s.updated = append(s.updated, st)
	return st, nil
}

func (s *stubStepWriter) UpsertStep(_ context.Context, st biz.Step) (biz.Step, error) {
	return s.UpdateStep(context.Background(), st)
}

var errUpdateFailed = errSimple("update failed")

type errSimple string

func (e errSimple) Error() string { return string(e) }
