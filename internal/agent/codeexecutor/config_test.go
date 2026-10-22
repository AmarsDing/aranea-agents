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

// SurfacePolicy 解析矩阵（P0-1 ③/§12.1-⑧）：3 固定面 × 3 值；空值为现状
// degrade；非三值 typo 全部 fail-closed 归一 refuse（宁可拍死不可放宽）。
// 归一风格沿用上方 FallbackPolicy 先例（ToLower+TrimSpace）。
func TestLoadEnvConfigSurfacePolicy(t *testing.T) {
	// 默认零值：三面全 degrade（零行为变更契约）。
	t.Setenv("CODE_EXECUTOR_POLICY_CHAT_CODEEXEC", "")
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "")
	t.Setenv("CODE_EXECUTOR_POLICY_MONITOR", "")
	cfg := codeexecutor.LoadEnvConfig()
	for _, s := range []codeexecutor.Surface{codeexecutor.SurfaceChatCodeexec, codeexecutor.SurfaceSkillGate, codeexecutor.SurfaceMonitor} {
		if got := cfg.Policy.For(s); got != codeexecutor.PolicyDegrade {
			t.Fatalf("default surface %s=%q, want degrade", s, got)
		}
	}

	// 三值各自生效（大小写/空白不敏感）。
	t.Setenv("CODE_EXECUTOR_POLICY_CHAT_CODEEXEC", " Enforce ")
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "REFUSE")
	t.Setenv("CODE_EXECUTOR_POLICY_MONITOR", "degrade")
	cfg = codeexecutor.LoadEnvConfig()
	if got := cfg.Policy.For(codeexecutor.SurfaceChatCodeexec); got != codeexecutor.PolicyEnforce {
		t.Fatalf("chat=%q, want enforce", got)
	}
	if got := cfg.Policy.For(codeexecutor.SurfaceSkillGate); got != codeexecutor.PolicyRefuse {
		t.Fatalf("skill_gate=%q, want refuse", got)
	}
	if got := cfg.Policy.For(codeexecutor.SurfaceMonitor); got != codeexecutor.PolicyDegrade {
		t.Fatalf("monitor=%q, want degrade", got)
	}

	// typo → fail-closed refuse（与 FallbackPolicy typo→degrade 相反，刻意：
	// 面级策略 typo 放宽执行面风险更高）。
	t.Setenv("CODE_EXECUTOR_POLICY_SKILL_GATE", "enforcce")
	if cfg = codeexecutor.LoadEnvConfig(); cfg.Policy.For(codeexecutor.SurfaceSkillGate) != codeexecutor.PolicyRefuse {
		t.Fatalf("typo must normalize to refuse: got %q", cfg.Policy.For(codeexecutor.SurfaceSkillGate))
	}

	// 未知面 → degrade（防 typo 面名把面误拍死）。
	if got := cfg.Policy.For(codeexecutor.Surface("nope")); got != codeexecutor.PolicyDegrade {
		t.Fatalf("unknown surface=%q, want degrade", got)
	}
	if got := cfg.Policy.For(""); got != codeexecutor.PolicyDegrade {
		t.Fatalf("empty surface=%q, want degrade", got)
	}
}
