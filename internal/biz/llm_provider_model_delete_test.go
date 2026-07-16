package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubProviderModelReaderForDelete is a minimal LlmProviderModelReader stub for Delete tests.
type stubProviderModelReaderForDelete struct {
	model ProviderModel
	err   error
}

func (s *stubProviderModelReaderForDelete) ListProviderModels(context.Context) ([]ProviderModel, error) {
	return nil, nil
}
func (s *stubProviderModelReaderForDelete) SearchProviderModels(_ context.Context, q ProviderModelListQuery) (ProviderModelListResult, error) {
	return ProviderModelListResult{Limit: q.Limit, Offset: q.Offset}, nil
}
func (s *stubProviderModelReaderForDelete) GetProviderModel(context.Context, string) (ProviderModel, error) {
	return s.model, s.err
}
func (s *stubProviderModelReaderForDelete) GetProviderModelByProviderAndModel(context.Context, string, string) (ProviderModel, error) {
	return s.model, s.err
}

// stubProviderModelWriterForDelete records whether DeleteProviderModel was called.
type stubProviderModelWriterForDelete struct {
	deleted bool
}

func (s *stubProviderModelWriterForDelete) CreateProviderModel(context.Context, ProviderModel) (ProviderModel, error) {
	return ProviderModel{}, nil
}
func (s *stubProviderModelWriterForDelete) UpdateProviderModel(context.Context, ProviderModel) (ProviderModel, error) {
	return ProviderModel{}, nil
}
func (s *stubProviderModelWriterForDelete) DeleteProviderModel(context.Context, string) error {
	s.deleted = true
	return nil
}
func (s *stubProviderModelWriterForDelete) UpdateProviderModelStatus(context.Context, string, string) error {
	return nil
}

// stubAgentRefChecker is a configurable AgentReferenceChecker for tests.
type stubAgentRefChecker struct {
	count int
	err   error
}

func (s *stubAgentRefChecker) CountAgentsByProviderAndModel(context.Context, string, string) (int, error) {
	return s.count, s.err
}

// TestDelete_FailClosedOnNilAgentRefs verifies that when AgentReferenceChecker is nil
// (misconfiguration), Delete must NOT proceed — it must return an Internal error.
// This is the fail-closed fix for BL-2.
func TestDelete_FailClosedOnNilAgentRefs(t *testing.T) {
	u := &LlmProviderModelUsecase{
		reader:    &stubProviderModelReaderForDelete{model: ProviderModel{ID: "m1", Provider: "openai", Model: "gpt-4"}},
		writer:    &stubProviderModelWriterForDelete{},
		agentRefs: nil, // misconfiguration
		lg:        loggateway.NewNoop(),
	}
	err := u.Delete(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected Internal error when agentRefs is nil, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
	if u.writer.(*stubProviderModelWriterForDelete).deleted {
		t.Fatal("DeleteProviderModel must NOT be called when agentRefs is nil")
	}
}

// TestDelete_FailClosedOnRefCheckError verifies that when the reference check itself
// returns an error, Delete must NOT proceed — it must return an Internal error
// wrapping the original error. This is the fail-closed fix for BL-2.
func TestDelete_FailClosedOnRefCheckError(t *testing.T) {
	refErr := errors.New("db connection lost")
	u := &LlmProviderModelUsecase{
		reader:    &stubProviderModelReaderForDelete{model: ProviderModel{ID: "m1", Provider: "openai", Model: "gpt-4"}},
		writer:    &stubProviderModelWriterForDelete{},
		agentRefs: &stubAgentRefChecker{err: refErr},
		lg:        loggateway.NewNoop(),
	}
	err := u.Delete(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected Internal error when ref check fails, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
	if u.writer.(*stubProviderModelWriterForDelete).deleted {
		t.Fatal("DeleteProviderModel must NOT be called when ref check fails")
	}
}

// TestDelete_ConflictWhenReferenced verifies existing behavior: when count > 0,
// Delete returns Conflict and does not proceed.
func TestDelete_ConflictWhenReferenced(t *testing.T) {
	u := &LlmProviderModelUsecase{
		reader:    &stubProviderModelReaderForDelete{model: ProviderModel{ID: "m1", Provider: "openai", Model: "gpt-4"}},
		writer:    &stubProviderModelWriterForDelete{},
		agentRefs: &stubAgentRefChecker{count: 3},
		lg:        loggateway.NewNoop(),
	}
	err := u.Delete(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected Conflict error when count > 0")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("expected CodeConflict, got %v", err)
	}
	if u.writer.(*stubProviderModelWriterForDelete).deleted {
		t.Fatal("DeleteProviderModel must NOT be called when referenced")
	}
}

// TestDelete_ProceedsWhenNoReferences verifies existing behavior: when count == 0,
// Delete proceeds normally.
func TestDelete_ProceedsWhenNoReferences(t *testing.T) {
	u := &LlmProviderModelUsecase{
		reader:    &stubProviderModelReaderForDelete{model: ProviderModel{ID: "m1", Provider: "openai", Model: "gpt-4"}},
		writer:    &stubProviderModelWriterForDelete{},
		agentRefs: &stubAgentRefChecker{count: 0},
		lg:        loggateway.NewNoop(),
	}
	if err := u.Delete(context.Background(), "m1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !u.writer.(*stubProviderModelWriterForDelete).deleted {
		t.Fatal("DeleteProviderModel must be called when no references")
	}
}
