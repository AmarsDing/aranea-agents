package biz

import "testing"

// ─── P2-5 思考强度路由（DeepSeek V4 effort 分档对齐）─────────────────────────

func TestResolveThinkingEffort_ComplexityWins(t *testing.T) {
	cases := []struct {
		name   string
		static string
		level  ComplexityLevel
		want   string
	}{
		// 简单任务：压掉长思考；高档静态配置保留 low（对齐 off/low 区间）。
		{"simple_no_static", "", ComplexitySimple, ThinkingEffortOff},
		{"simple_low_static", "low", ComplexitySimple, ThinkingEffortOff},
		{"simple_high_static", "high", ComplexitySimple, ThinkingEffortLow},
		{"simple_max_static", "max", ComplexitySimple, ThinkingEffortLow},
		// 日常任务：恒 high（显式路由覆盖静态档）。
		{"moderate_no_static", "", ComplexityModerate, ThinkingEffortHigh},
		{"moderate_low_static", "low", ComplexityModerate, ThinkingEffortHigh},
		{"moderate_max_static", "max", ComplexityModerate, ThinkingEffortHigh},
		// 复杂任务：恒 max。
		{"complex_no_static", "", ComplexityComplex, ThinkingEffortMax},
		{"complex_low_static", "low", ComplexityComplex, ThinkingEffortMax},
		// 未给复杂度：回落静态档（归一化）。
		{"unspecified_static_high", "HIGH", ComplexityLevel(""), ThinkingEffortHigh},
		{"unspecified_static_off", "off", ComplexityLevel(""), ThinkingEffortOff},
		{"unspecified_no_static", "", ComplexityLevel(""), ""},
		{"unspecified_garbage_static", "turbo", ComplexityLevel(""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveThinkingEffort(tc.static, tc.level); got != tc.want {
				t.Fatalf("ResolveThinkingEffort(%q, %q) = %q, want %q", tc.static, tc.level, got, tc.want)
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

func TestStaticThinkingEffort(t *testing.T) {
	cases := []struct {
		mode, level, want string
	}{
		{"provider_default", "high", ""},    // 跟随厂商
		{"", "high", ""},                     // 空 mode = 跟随厂商
		{"custom", "off", ThinkingEffortOff},
		{"custom", "high", ThinkingEffortHigh},
		{"custom", "", ""},                   // 未选 level
		{"custom", "turbo", ""},              // 非法档
		{"custom", "  Max ", ThinkingEffortMax}, // 归一化
	}
	for _, tc := range cases {
		t.Run(tc.mode+"_"+tc.level, func(t *testing.T) {
			if got := StaticThinkingEffort(tc.mode, tc.level); got != tc.want {
				t.Fatalf("StaticThinkingEffort(%q, %q) = %q, want %q", tc.mode, tc.level, got, tc.want)
			}
		})
	}
}
