package codeexecutor

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// TestE2BFailClosedInProd asserts that in production env without
// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD, an unavailable E2B backend must NOT
// silently fall back to the local executor (fail-open). It must return nil
// (fail-closed), matching the Docker guard in TestDockerRuntimeFallbackFailClosedInProd.
//
// B-03 fix: E2B/Container previously fell back to Local unconditionally;
// now they respect the same production fail-closed policy as Docker.
func TestE2BFailClosedInProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	// E2B_API_KEY intentionally unset → E2B unavailable
	// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD intentionally unset → AllowLocalInProd=false

	f := NewFactoryWithLogger(loggateway.NewNoop())
	exec := f.Resolve(context.Background(), TypeE2B, t.TempDir())
	if exec != nil {
		t.Fatal("expected nil executor in production when E2B unavailable (fail-closed), got non-nil (silent local fallback)")
	}
}

// TestContainerFailClosedInProd asserts the same fail-closed behavior for
// the Container backend when it is unavailable in production.
func TestContainerFailClosedInProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	// Container backend unavailable (no container runtime in test env)
	// CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD intentionally unset → AllowLocalInProd=false

	f := NewFactoryWithLogger(loggateway.NewNoop())
	exec := f.Resolve(context.Background(), TypeContainer, t.TempDir())
	if exec != nil {
		t.Fatal("expected nil executor in production when Container unavailable (fail-closed), got non-nil (silent local fallback)")
	}
}

// TestE2BFallbackToLocalInNonProd asserts the existing dev behavior is
// preserved: outside production, an unavailable E2B backend still falls
// back to the local executor.
func TestE2BFallbackToLocalInNonProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	// E2B_API_KEY intentionally unset → E2B unavailable

	f := NewFactoryWithLogger(loggateway.NewNoop())
	exec := f.Resolve(context.Background(), TypeE2B, t.TempDir())
	if exec == nil {
		t.Fatal("expected local executor fallback in non-prod when E2B unavailable, got nil")
	}
}

// TestContainerFallbackToLocalInNonProd asserts the same non-prod fallback
// for the Container backend.
func TestContainerFallbackToLocalInNonProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")

	f := NewFactoryWithLogger(loggateway.NewNoop())
	exec := f.Resolve(context.Background(), TypeContainer, t.TempDir())
	if exec == nil {
		t.Fatal("expected local executor fallback in non-prod when Container unavailable, got nil")
	}
}

// TestE2BAllowLocalInProd asserts that when CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD=1
// is set, production env falls back to local even when E2B is unavailable
// (explicit break-glass consent).
func TestE2BAllowLocalInProd(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	t.Setenv("CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD", "1")
	// E2B_API_KEY intentionally unset → E2B unavailable

	f := NewFactoryWithLogger(loggateway.NewNoop())
	exec := f.Resolve(context.Background(), TypeE2B, t.TempDir())
	if exec == nil {
		t.Fatal("expected local executor fallback in production with AllowLocalInProd=1 when E2B unavailable, got nil")
	}
}
