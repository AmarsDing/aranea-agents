package biz_test

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

func TestDefaultFunctionResolver_Resolve(t *testing.T) {
	r := biz.NewDefaultFunctionResolver()
	if err := r.Resolve(context.Background(), "any-ref"); err != nil {
		t.Fatalf("DefaultFunctionResolver should accept all refs, got error: %v", err)
	}
	if err := r.Resolve(context.Background(), ""); err != nil {
		t.Fatalf("DefaultFunctionResolver should accept empty ref, got error: %v", err)
	}
}

type stubFunctionResolver struct {
	validRefs map[string]bool
}

func (s *stubFunctionResolver) Resolve(_ context.Context, funcRef string) error {
	if s.validRefs == nil {
		return nil
	}
	if ok, exists := s.validRefs[funcRef]; exists && ok {
		return nil
	}
	return errors.New("function not found: " + funcRef)
}

func TestFunctionResolver_StubValid(t *testing.T) {
	r := &stubFunctionResolver{validRefs: map[string]bool{"tool-a": true}}
	if err := r.Resolve(context.Background(), "tool-a"); err != nil {
		t.Fatalf("expected valid ref to resolve, got: %v", err)
	}
}

func TestFunctionResolver_StubInvalid(t *testing.T) {
	r := &stubFunctionResolver{validRefs: map[string]bool{"tool-a": true}}
	if err := r.Resolve(context.Background(), "missing"); err == nil {
		t.Fatal("expected invalid ref to fail")
	}
}

func TestFunctionResolver_Interface(t *testing.T) {
	// Verify DefaultFunctionResolver implements the interface.
	var _ biz.FunctionResolver = biz.NewDefaultFunctionResolver()
	// Verify stubFunctionResolver implements the interface.
	var _ biz.FunctionResolver = &stubFunctionResolver{}
}
