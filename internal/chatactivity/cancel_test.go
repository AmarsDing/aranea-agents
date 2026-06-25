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
	n, err := CancelRunningActivityMessages(context.Background(), &stubActivityRepo{}, &stubActivityRepo{}, "  ", loggateway.NewNoop())
	if err != nil || n != 0 {
		t.Fatalf("expected 0,nil got %d,%v", n, err)
	}
}

func TestCancelRunningActivityMessages_SkipsTerminalStatuses(t *testing.T) {
	repo := &stubActivityRepo{
		activities: []biz.Activity{
			{ID: "a1", SessionID: "sess1", Status: biz.ActivityStatusCompleted},
			{ID: "a2", SessionID: "sess1", Status: biz.ActivityStatusFailed},
			{ID: "a3", SessionID: "sess1", Status: biz.ActivityStatusCancelled},
			{ID: "a4", SessionID: "sess1", Status: biz.ActivityStatusInterrupted},
		},
	}
	n, err := CancelRunningActivityMessages(context.Background(), repo, repo, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 cancellations, got %d", n)
	}
	if len(repo.updated) != 0 {
		t.Fatalf("expected no updates, got %d", len(repo.updated))
	}
}

func TestCancelRunningActivityMessages_CancelsInFlight(t *testing.T) {
	repo := &stubActivityRepo{
		activities: []biz.Activity{
			{ID: "a1", SessionID: "sess1", Status: biz.ActivityStatusRunning},
			{ID: "a2", SessionID: "sess1", Status: biz.ActivityStatusToolRunning},
			{ID: "a3", SessionID: "sess1", Status: biz.ActivityStatusToolBlocked},
			{ID: "a4", SessionID: "sess1", Status: biz.ActivityStatusCompleted},
		},
	}
	n, err := CancelRunningActivityMessages(context.Background(), repo, repo, "sess1", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 cancellations, got %d", n)
	}
	if len(repo.updated) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(repo.updated))
	}
	for _, u := range repo.updated {
		if u.Status != biz.ActivityStatusCancelled {
			t.Fatalf("expected cancelled status, got %s", u.Status)
		}
	}
}

func TestCancelRunningActivityMessages_UpdateErrorContinues(t *testing.T) {
	reader := &stubActivityRepo{
		activities: []biz.Activity{
			{ID: "a1", SessionID: "sess1", Status: biz.ActivityStatusRunning},
			{ID: "a2", SessionID: "sess1", Status: biz.ActivityStatusToolRunning},
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

// stubActivityRepo implements biz.ActivityRepo for cancel tests.
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
