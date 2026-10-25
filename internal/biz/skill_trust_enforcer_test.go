package biz

// D4-c 信任门禁 ready flip 钉板（就绪不翻）：on 档真生效归一（不再降级
// shadow）、执行闭包非 on 恒 nil（=未安装=TrustGateBlocked 恒 false=零行
// 为变化）、on 档闭包只拒低于门槛候选。真翻转仍以 shadow 期零误拒证据为
// 前置（feature-flags-registry §8 反作用#1）。

import "testing"

// TestNewTrustGateEnforcer_NilUnlessOn 钉执行闭包安装面：
// off/shadow/空值/未知值一律 nil（零行为变化承诺）；仅 on 非 nil。
func TestNewTrustGateEnforcer_NilUnlessOn(t *testing.T) {
	required := RequiredTrustForLayer(SkillLoadLayerL2Instruction)
	for _, mode := range []TrustGateMode{"", TrustGateModeOff, TrustGateModeShadow, "garbage"} {
		if enf := NewTrustGateEnforcer(nil, mode, SkillLoadLayerL2Instruction, required); enf != nil {
			t.Fatalf("mode %q: enforcer must be nil (gate not installed)", mode)
		}
	}
	if enf := NewTrustGateEnforcer(nil, TrustGateModeOn, SkillLoadLayerL2Instruction, required); enf == nil {
		t.Fatal("mode on: enforcer must be installed")
	}
}

// TestTrustGateEnforcer_OnBlocksOnlyBelowRequired 钉 on 档判定闭包：
// 只拒 resolved 低于 required 的候选；存量默认（空来源空声明 T3）在 L2
// 门槛下永不误拒。nil Logger 走 Noop 不 panic。
func TestTrustGateEnforcer_OnBlocksOnlyBelowRequired(t *testing.T) {
	required := RequiredTrustForLayer(SkillLoadLayerL2Instruction)
	enf := NewTrustGateEnforcer(nil, TrustGateModeOn, SkillLoadLayerL2Instruction, required)
	if enf == nil {
		t.Fatal("on-mode enforcer must be non-nil")
	}

	cases := []struct {
		name      string
		candidate SkillRuntimeCandidate
		wantBlock bool
	}{
		// zip 导入默认 T4：L2 拒。
		{"zip default T4 blocked", SkillRuntimeCandidate{SkillID: "s1", Slug: "zip-a", Source: "zip_import", TrustLevel: ""}, true},
		// 声明 T4 + builtin：取严仍 T4，拒。
		{"declared T4 blocked even for builtin", SkillRuntimeCandidate{SkillID: "s2", Slug: "b-t4", Source: "builtin", TrustLevel: string(SkillTrustT4Untrusted)}, true},
		// 存量默认（空来源空声明）解析 T3：L2 放行（防误挂全拒载）。
		{"legacy default passes", SkillRuntimeCandidate{SkillID: "s3", Slug: "legacy", Source: "", TrustLevel: ""}, false},
		// LLM 产物默认 T3：L2 放行。
		{"llm_evolution default passes at L2", SkillRuntimeCandidate{SkillID: "s4", Slug: "llm-a", Source: "llm_evolution", TrustLevel: ""}, false},
	}
	for _, c := range cases {
		if got := enf(c.candidate); got != c.wantBlock {
			t.Fatalf("%s: blocked=%v, want %v", c.name, got, c.wantBlock)
		}
	}
}
