package cronrunner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockRegistrySyncAgent struct {
	err error
}

func (m *mockRegistrySyncAgent) RunSync(_ context.Context) error {
	return m.err
}

func TestDispatchCronTask_ModelRegistrySync_Success(t *testing.T) {
	agent := &mockRegistrySyncAgent{err: nil}
	r := NewRunner(Deps{RegistrySyncAgent: agent}, loggateway.NewNoop())
	task := biz.CronTask{ID: "t1", ConfigJSON: `{"target_type":"model_registry_sync","message":"sync"}`}
	cfg := cronTaskConfig{TargetType: "model_registry_sync", Message: "sync"}
	res, err := r.dispatchCronTask(context.Background(), task, cfg, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != (cronDispatchResult{}) {
		t.Fatalf("expected zero result, got %+v", res)
	}
}

func TestDispatchCronTask_ModelRegistrySync_Error(t *testing.T) {
	syncErr := errors.New("sync failed")
	agent := &mockRegistrySyncAgent{err: syncErr}
	r := NewRunner(Deps{RegistrySyncAgent: agent}, loggateway.NewNoop())
	task := biz.CronTask{ID: "t2", ConfigJSON: `{"target_type":"model_registry_sync","message":"sync"}`}
	cfg := cronTaskConfig{TargetType: "model_registry_sync", Message: "sync"}
	res, err := r.dispatchCronTask(context.Background(), task, cfg, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, syncErr) {
		t.Fatalf("expected syncErr, got %v", err)
	}
	if res != (cronDispatchResult{}) {
		t.Fatalf("expected zero result, got %+v", res)
	}
}

func TestDispatchCronTask_ModelRegistrySync_NilAgent(t *testing.T) {
	r := NewRunner(Deps{}, loggateway.NewNoop())
	task := biz.CronTask{ID: "t3", ConfigJSON: `{"target_type":"model_registry_sync","message":"sync"}`}
	cfg := cronTaskConfig{TargetType: "model_registry_sync", Message: "sync"}
	res, err := r.dispatchCronTask(context.Background(), task, cfg, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	// validationErr now returns apierror.BadRequest; check the message content.
	if !strings.Contains(err.Error(), "model registry sync agent not available") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if res != (cronDispatchResult{}) {
		t.Fatalf("expected zero result, got %+v", res)
	}
}

// TestEnsureCronSession_ReusesStateSessionID verifies that when a dispatchState
// already carries a session ID (set by a previous retry attempt),
// ensureCronSession returns it WITHOUT calling Session.Create. This is the
// regression guard for "Domain 6 Claim 2: Cron Retry 包裹整个 dispatch 可重复建 Session".
//
// We cannot easily inject a mock *biz.SessionUsecase (concrete struct), so we
// rely on the fact that when state.sessID != "", ensureCronSession short-circuits
// before touching r.deps.Session. If the short-circuit ever regresses, the nil
// deps.Session pointer would panic — making the test fail loudly.
func TestEnsureCronSession_ReusesStateSessionID(t *testing.T) {
	// Deps.Session is intentionally nil: if ensureCronSession ignores the
	// pre-populated state and tries to call Session.Create, the test panics.
	r := NewRunner(Deps{}, loggateway.NewNoop())
	state := &cronDispatchState{sessID: "sess-already-created"}

	got, err := r.ensureCronSession(context.Background(), state, biz.Session{
		OwnerType:  "agent",
		AgentID:    "agent-1",
		Title:      "cron",
		DialogMode: "cron",
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "sess-already-created" {
		t.Fatalf("expected reused sessID 'sess-already-created', got %q", got)
	}
	if state.sessID != "sess-already-created" {
		t.Fatalf("state.sessID should be unchanged, got %q", state.sessID)
	}
}
