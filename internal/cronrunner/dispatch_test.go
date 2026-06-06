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
	res, err := r.dispatchCronTask(context.Background(), task, cfg)
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
	res, err := r.dispatchCronTask(context.Background(), task, cfg)
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
	res, err := r.dispatchCronTask(context.Background(), task, cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	// validationErr now returns kerrors.BadRequest; check the message content.
	if !strings.Contains(err.Error(), "model registry sync agent not available") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if res != (cronDispatchResult{}) {
		t.Fatalf("expected zero result, got %+v", res)
	}
}
