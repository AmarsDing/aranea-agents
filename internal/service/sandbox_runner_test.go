package service

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
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

// TestSandboxRunner_RunSandbox_AnnotatesValidator verifies the sandbox result
// JSON names its producer (F10): the approve-path SandboxRunner runs a
// different check set than the trigger-path GateVerifier, so "passed" is only
// meaningful when the payload says which validator ran.
func TestSandboxRunner_RunSandbox_AnnotatesValidator(t *testing.T) {
	t.Parallel()
	s := NewSandboxRunner(nil, nil, loggateway.NewNoop())
	_, raw, err := s.RunSandbox(context.Background(), "skill-1", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if got := result["validator"]; got != biz.SandboxValidatorSandboxRunner {
		t.Fatalf("validator = %v, want %q", got, biz.SandboxValidatorSandboxRunner)
	}
}

// --- P0-1 ③ executor 不可用时输出结构化 skipped（非静默通过）---

// 退化面（默认 degrade）下 executor 不可用：可见跳过（passed=true + 显式
// code_execution_skipped check），不再静默放行。
func TestSandboxRunner_SkipsVisiblyWhenExecutorUnavailableDegrade(t *testing.T) {
	// 用 ARANEA_ENV=production（未允许 local）逼出 factory 返回 nil executor。
	t.Setenv("ARANEA_ENV", "production")
	t.Setenv("CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD", "")
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "")
	factory := codeexecutor.NewFactoryWithLogger(loggateway.NewNoop())
	s := NewSandboxRunner(nil, factory, loggateway.NewNoop())
	passed, raw, err := s.RunSandbox(context.Background(), "skill-1", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if !passed {
		t.Fatalf("degrade surface skip must stay pass (visible degrade), got fail: %s", raw)
	}
	var result struct {
		Passed bool `json:"passed"`
		Checks []struct {
			Name    string `json:"name"`
			Passed  bool   `json:"passed"`
			Message string `json:"message"`
		} `json:"checks"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == "code_execution_skipped" {
			found = true
			if !c.Passed {
				t.Fatalf("degrade skip check must be Passed=true (visible), got msg %q", c.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected structured code_execution_skipped check, got %s", raw)
	}
}

// 面级 enforce 但无隔离后端可用：fail-closed——skipped check 置失败并拉低
// 总判定（严格语义下宁可门禁变严可见）。
func TestSandboxRunner_FailsClosedWhenEnforceSurfaceUnavailable(t *testing.T) {
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "enforce")
	// 无 docker daemon 的机器上 enforce 返回 nil；有 daemon 时会解析出 docker
	// executor → 跳过此用例（该场景由 factory 层 enforce-docker 用例覆盖）。
	factory := codeexecutor.NewFactoryWithLogger(loggateway.NewNoop())
	s := NewSandboxRunner(nil, factory, loggateway.NewNoop())
	if s.executor != nil {
		t.Skip("isolated backend available — enforce resolves an executor here")
	}
	passed, raw, err := s.RunSandbox(context.Background(), "skill-1", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if passed {
		t.Fatalf("enforce surface with no executor must fail closed, got pass: %s", raw)
	}
	var result struct {
		Passed  bool   `json:"passed"`
		Message string `json:"message"`
		Checks  []struct {
			Name   string `json:"name"`
			Passed bool   `json:"passed"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if result.Message != "Code execution validation unavailable" {
		t.Fatalf("message=%q, want unavailable note", result.Message)
	}
	for _, c := range result.Checks {
		if c.Name == "code_execution_skipped" && !c.Passed {
			return
		}
	}
	t.Fatalf("expected failing code_execution_skipped check, got %s", raw)
}

// 全局 strict（83 FR-3）：executor block 在 strict 语义下不可静默跳过。
func TestSandboxRunner_FailsClosedUnderStrictFallback(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	t.Setenv("CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD", "")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "strict")
	factory := codeexecutor.NewFactoryWithLogger(loggateway.NewNoop())
	s := NewSandboxRunner(nil, factory, loggateway.NewNoop())
	passed, raw, err := s.RunSandbox(context.Background(), "skill-1", "# Guide\n\n## Steps\n\nDo the thing.")
	if err != nil {
		t.Fatalf("RunSandbox err: %v", err)
	}
	if passed {
		t.Fatalf("strict with nil executor must fail closed, got pass: %s", raw)
	}
}
