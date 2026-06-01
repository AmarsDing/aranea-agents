package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestDefaultFallbackPolicy(t *testing.T) {
	policy := biz.DefaultFallbackPolicy()
	if policy.Enabled {
		t.Fatalf("Enabled = true, want false")
	}
	if policy.Reason != "graph-only production default" {
		t.Fatalf("Reason = %q, want %q", policy.Reason, "graph-only production default")
	}
	if policy.CanaryPercentage != 100 {
		t.Fatalf("CanaryPercentage = %d, want 100", policy.CanaryPercentage)
	}
}

func TestNativeFallbackPolicy(t *testing.T) {
	policy := biz.NativeFallbackPolicy()
	if !policy.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if policy.Reason != "native fallback enabled via config" {
		t.Fatalf("Reason = %q, want %q", policy.Reason, "native fallback enabled via config")
	}
	if policy.CanaryPercentage != 0 {
		t.Fatalf("CanaryPercentage = %d, want 0", policy.CanaryPercentage)
	}
}
