package codeexecutor_test

import (
	"testing"

	"aranea-agents/internal/agent/codeexecutor"
)

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"docker", codeexecutor.TypeDocker},
		{"DOCKER", codeexecutor.TypeDocker},
		{"e2b", codeexecutor.TypeE2B},
		{"container", codeexecutor.TypeContainer},
		{"", codeexecutor.TypeLocal},
		{"auto", codeexecutor.TypeAuto},
		{"unknown", codeexecutor.TypeLocal},
	}
	for _, tc := range tests {
		if got := codeexecutor.NormalizeType(tc.in); got != tc.want {
			t.Fatalf("NormalizeType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveType(t *testing.T) {
	if got := codeexecutor.ResolveType("docker", ""); got != codeexecutor.TypeDocker {
		t.Fatalf("agent override: got %q", got)
	}
	if got := codeexecutor.ResolveType("", "e2b"); got != codeexecutor.TypeE2B {
		t.Fatalf("env override: got %q", got)
	}
	if got := codeexecutor.ResolveType("docker", "e2b"); got != codeexecutor.TypeDocker {
		t.Fatalf("agent wins: got %q", got)
	}
	if got := codeexecutor.ResolveType("", ""); got != codeexecutor.TypeLocal {
		t.Fatalf("default: got %q", got)
	}
	if got := codeexecutor.ResolveType("auto", ""); got != codeexecutor.TypeAuto {
		t.Fatalf("auto: got %q", got)
	}
}

func TestPreferDockerWhenUnset(t *testing.T) {
	if !codeexecutor.PreferDockerWhenUnset("", "", true) {
		t.Fatal("empty agent+env should prefer docker when available")
	}
	if !codeexecutor.PreferDockerWhenUnset("auto", "", true) {
		t.Fatal("auto should prefer docker when available")
	}
	if codeexecutor.PreferDockerWhenUnset("local", "", true) {
		t.Fatal("explicit local must stay local")
	}
	if codeexecutor.PreferDockerWhenUnset("", "e2b", true) {
		t.Fatal("explicit env backend must win")
	}
	if codeexecutor.PreferDockerWhenUnset("", "", false) {
		t.Fatal("must not prefer docker when daemon is down")
	}
}

func TestCollectOutputDirFilesEmpty(t *testing.T) {
	if files := codeexecutor.CollectOutputDirFiles("", 0); files != nil {
		t.Fatalf("expected nil for empty dir")
	}
}

func TestLoadEnvConfigFallbackPolicy(t *testing.T) {
	// 默认（未设置）：degrade，非 strict。
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "")
	cfg := codeexecutor.LoadEnvConfig()
	if cfg.FallbackPolicy != codeexecutor.FallbackPolicyDegrade || cfg.StrictFallback() {
		t.Fatalf("default: got policy %q strict=%v", cfg.FallbackPolicy, cfg.StrictFallback())
	}
	// strict（大小写不敏感）。
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "STRICT")
	if cfg = codeexecutor.LoadEnvConfig(); !cfg.StrictFallback() {
		t.Fatalf("strict: got policy %q", cfg.FallbackPolicy)
	}
	// typo 归一 degrade，避免被当作显式选择漏过审查。
	t.Setenv("CODE_EXECUTOR_FALLBACK_POLICY", "stict")
	if cfg = codeexecutor.LoadEnvConfig(); cfg.StrictFallback() {
		t.Fatalf("typo must normalize to degrade: got policy %q", cfg.FallbackPolicy)
	}
}
