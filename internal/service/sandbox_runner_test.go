package service

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// TestSandboxRunner_RunSandbox_ValidDraft verifies that a well-formed draft
// body passes rule-based sandbox validation via the biz.SandboxRunner port
// used by GateVerifier.
func TestSandboxRunner_RunSandbox_ValidDraft(t *testing.T) {
	t.Parallel()
	s := NewSandboxRunner(nil, nil, loggateway.NewNoop())
	passed, raw, err := s.RunSandbox(context.Background(), "skill-1", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if !passed {
		t.Fatalf("expected pass, got fail: %s", string(raw))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if result["passed"] != true {
		t.Fatalf("result.passed = %v, want true", result["passed"])
	}
}

// TestSandboxRunner_RunSandbox_EmptyDraft verifies an empty draft body is
// rejected by rule-based validation.
func TestSandboxRunner_RunSandbox_EmptyDraft(t *testing.T) {
	t.Parallel()
	s := NewSandboxRunner(nil, nil, loggateway.NewNoop())
	passed, _, err := s.RunSandbox(context.Background(), "skill-1", "")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if passed {
		t.Fatal("expected fail for empty draft body")
	}
}

// TestSandboxRunner_RunSandbox_EmptySkillID verifies a missing skill ID is
// rejected by rule-based validation.
func TestSandboxRunner_RunSandbox_EmptySkillID(t *testing.T) {
	t.Parallel()
	s := NewSandboxRunner(nil, nil, loggateway.NewNoop())
	passed, _, err := s.RunSandbox(context.Background(), "", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if passed {
		t.Fatal("expected fail for empty skill ID")
	}
}

// TestSandboxRunner_RunSandbox_NoDBWrites verifies RunSandbox is a pure check:
// it must not touch the usecase (nil uc would panic on any DB call).
func TestSandboxRunner_RunSandbox_NoDBWrites(t *testing.T) {
	t.Parallel()
	s := NewSandboxRunner(nil, nil, loggateway.NewNoop())
	// nil uc: any DB access panics → test fails
	if _, _, err := s.RunSandbox(context.Background(), "skill-1", "# A\n\n## B\n\ncontent"); err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
}
