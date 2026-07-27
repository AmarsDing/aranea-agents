package biz

import (
	"context"
	"encoding/json"
	"testing"
)

// ── F10 (P-evo-5): sandbox_result JSON must name its validator ───────────────
//
// The two sandbox paths run different check sets (GateVerifier: 5-dimension
// gate; SandboxRunner / rule-based fallback: 3 rule checks). Without a
// validator marker, "passed": true means different things depending on which
// path produced it — the payload must say which validator ran.

func TestRunSandboxCheck_GateVerifier_AnnotatesValidator(t *testing.T) {
	gate := &stubGateVerifier{
		passed: true,
		checks: []GateCheckResult{{Name: "functional", Passed: true, Reason: "ok"}},
	}
	uc := newValidateTestUsecase(newRecordingUnifiedStore(), gate)

	passed, raw := uc.runSandboxCheck(context.Background(), "sg-1", "sk-1", "# Draft\nbody")
	if !passed {
		t.Fatal("expected gate pass verdict")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("sandbox result not JSON: %v", err)
	}
	if got := payload["validator"]; got != SandboxValidatorGateVerifier {
		t.Fatalf("validator = %v, want %q (gate path must be distinguishable)", got, SandboxValidatorGateVerifier)
	}
}

func TestRunSandboxCheck_RuleBasedFallback_AnnotatesValidator(t *testing.T) {
	uc := newValidateTestUsecase(newRecordingUnifiedStore(), nil) // no gate wired

	passed, raw := uc.runSandboxCheck(context.Background(), "sg-1", "sk-1", "# Draft\nbody")
	if !passed {
		t.Fatal("expected rule-based pass verdict for valid draft")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("sandbox result not JSON: %v", err)
	}
	if got := payload["validator"]; got != SandboxValidatorRuleBased {
		t.Fatalf("validator = %v, want %q (fallback must not masquerade as gate)", got, SandboxValidatorRuleBased)
	}
}
