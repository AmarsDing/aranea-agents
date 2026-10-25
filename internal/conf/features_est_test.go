package conf

import "testing"

// TestPromptEstMode_Matrix 钉死 D4-a env 归一：合法三档原样通过、空值回落
// shadow（valid=true）、未知值回落 shadow 且 valid=false（wire 侧据此 Warn，
// conf 包保持零 loggateway 依赖的口子）。
func TestPromptEstMode_Matrix(t *testing.T) {
	cases := []struct {
		env       string
		wantMode  string
		wantValid bool
	}{
		{"", "shadow", true},
		{"legacy", "legacy", true},
		{"shadow", "shadow", true},
		{"segmented", "segmented", true},
		{" LEGACY ", "legacy", true}, // 大小写 + 首尾空白归一
		{"Segmented", "segmented", true},
		{"on", "shadow", false},
		{"segmented-v2", "shadow", false},
		{"shadowy", "shadow", false},
	}
	for _, c := range cases {
		t.Run("env="+c.env, func(t *testing.T) {
			t.Setenv("ARANEA_PROMPT_EST_MODE", c.env)
			mode, valid := PromptEstMode()
			if mode != c.wantMode || valid != c.wantValid {
				t.Fatalf("PromptEstMode() = (%q, %v), want (%q, %v)", mode, valid, c.wantMode, c.wantValid)
			}
		})
	}
}

// TestDurablePrecheckMode_Matrix 钉死 D4-b env 归一：off/observe/on 三档原样
// 通过、空值回落 observe、未知值回落 observe 且 valid=false。
func TestDurablePrecheckMode_Matrix(t *testing.T) {
	cases := []struct {
		env       string
		wantMode  string
		wantValid bool
	}{
		{"", "observe", true},
		{"off", "off", true},
		{"observe", "observe", true},
		{"on", "on", true},
		{" ON ", "on", true},
		{"Observe", "observe", true},
		{"shadow", "observe", false},
		{"1", "observe", false},
	}
	for _, c := range cases {
		t.Run("env="+c.env, func(t *testing.T) {
			t.Setenv("ARANEA_DURABLE_PRECHECK", c.env)
			mode, valid := DurablePrecheckMode()
			if mode != c.wantMode || valid != c.wantValid {
				t.Fatalf("DurablePrecheckMode() = (%q, %v), want (%q, %v)", mode, valid, c.wantMode, c.wantValid)
			}
		})
	}
}
