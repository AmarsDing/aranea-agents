package codeexecutor

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagentcodeexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// errExecutor is a fake CodeExecutor whose ExecuteCode always fails with
// sentinelDockerErr, simulating a runtime docker backend failure.
type errExecutor struct{}

func (errExecutor) CodeBlockDelimiter() trpcagentcodeexec.CodeBlockDelimiter {
	return trpcagentcodeexec.CodeBlockDelimiter{}
}

func (errExecutor) ExecuteCode(_ context.Context, _ trpcagentcodeexec.CodeExecutionInput) (trpcagentcodeexec.CodeExecutionResult, error) {
	return trpcagentcodeexec.CodeExecutionResult{}, sentinelDockerErr
}

var sentinelDockerErr = errors.New("docker boom")

// TestDockerRuntimeFallbackFailClosedInProd asserts that in production env
// without CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD, a runtime docker failure must
// NOT silently fall back to the local executor (fail-open). It must return
// the docker error instead (fail-closed), matching the config-time guard in
// Factory.applyAvailabilityFallback.
func TestDockerRuntimeFallbackFailClosedInProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD intentionally unset → AllowLocalInProd=false

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	_, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err == nil {
		t.Fatal("expected docker error to be returned in production (fail-closed), got nil (silent local fallback)")
	}
	if !errors.Is(err, sentinelDockerErr) {
		t.Fatalf("expected sentinel docker error to propagate, got %v", err)
	}
}

// TestDockerRuntimeFallbackStrictRefusesLocal asserts that under
// CODE_EXECUTOR_FALLBACK_POLICY=strict (83-长时运行韧性 FR-3), a runtime
// docker failure is refused even outside production — no silent degrade to
// the less-isolated local executor.
func TestDockerRuntimeFallbackStrictRefusesLocal(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "strict")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	_, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if !errors.Is(err, sentinelDockerErr) {
		t.Fatalf("strict: expected sentinel docker error, got %v", err)
	}
}

// TestDockerRuntimeFallbackStillLocalInNonProd asserts the existing dev
// behavior is preserved: outside production, a runtime docker failure still
// falls back to the local executor (no error surfaced to the caller).
func TestDockerRuntimeFallbackStillLocalInNonProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	fb := newDockerRuntimeFallback(errExecutor{}, f, t.TempDir(), loggateway.NewNoop())

	res, err := fb.ExecuteCode(context.Background(), trpcagentcodeexec.CodeExecutionInput{})
	if err != nil {
		t.Fatalf("expected local fallback to succeed with empty input in non-prod, got err: %v", err)
	}
	// Empty input → local executor returns an empty result (no code blocks to run).
	_ = res
}
