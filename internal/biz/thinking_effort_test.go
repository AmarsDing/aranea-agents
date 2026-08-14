package biz

import "testing"

// ─── P2-5 思考强度路由（DeepSeek V4 effort 分档对齐）─────────────────────────
//
// 按任务复杂度选 thinking 档：简单=off/low、日常=high、复杂=max。
// 复杂度显式给出时覆盖 agent 静态 ReasoningLevel（与 P2-1 级联覆盖成员
// 模型路由插件同一约定：显式路由策略优先）；未给复杂度时回落静态档。

func TestResolveThinkingEffort_ComplexityWins(t *testing.T) {
	cases := []struct {
		name       string
		static     string
		complexity ThinkingComplexity
		want       string
	}{
		// 简单任务：压掉长思考；高档静态配置保留 low（对齐 off/low 区间）。
		{"simple_no_static", "", ComplexitySimple, ThinkingEffortOff},
		{"simple_low_static", "low", ComplexitySimple, ThinkingEffortOff},
		{"simple_high_static", "high", ComplexitySimple, ThinkingEffortLow},
		{"simple_max_static", "max", ComplexitySimple, ThinkingEffortLow},
		// 日常任务：恒 high（显式路由覆盖静态档）。
		{"routine_no_static", "", ComplexityRoutine, ThinkingEffortHigh},
		{"routine_low_static", "low", ComplexityRoutine, ThinkingEffortHigh},
		{"routine_max_static", "max", ComplexityRoutine, ThinkingEffortHigh},
		// 复杂任务：恒 max。
		{"complex_no_static", "", ComplexityComplex, ThinkingEffortMax},
		{"complex_low_static", "low", ComplexityComplex, ThinkingEffortMax},
		// 未给复杂度：回落静态档（归一化）。
		{"unspecified_static_high", "HIGH", ComplexityUnspecified, ThinkingEffortHigh},
		{"unspecified_static_off", "off", ComplexityUnspecified, ThinkingEffortOff},
		{"unspecified_no_static", "", ComplexityUnspecified, ""},
		{"unspecified_garbage_static", "turbo", ComplexityUnspecified, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveThinkingEffort(tc.static, tc.complexity); got != tc.want {
				t.Fatalf("ResolveThinkingEffort(%q, %q) = %q, want %q", tc.static, tc.complexity, got, tc.want)
			}
		})
	}
}

func TestNormalizeThinkingEffort(t *testing.T) {
	if got := NormalizeThinkingEffort("  Max "); got != ThinkingEffortMax {
		t.Fatalf("normalize = %q, want max", got)
	}
	for _, s := range []string{"", "turbo", " ultra ", "medium2"} {
		if got := NormalizeThinkingEffort(s); got != "" {
			t.Fatalf("NormalizeThinkingEffort(%q) = %q, want empty", s, got)
		}
	}
	// medium 是合法档（框架 validReasoningEfforts 对齐）。
	if got := NormalizeThinkingEffort("medium"); got != ThinkingEffortMedium {
		t.Fatalf("normalize medium = %q", got)
	}
}
