package tools

import (
	"testing"
)

func TestComplexityRuleEngine_SimplePatterns(t *testing.T) {
	engine := NewComplexityRuleEngine()
	cases := []struct {
		input    string
		expected ComplexityLevel
	}{
		{"什么是 Agent？", ComplexitySimple},
		{"解释一下 Graph 编排", ComplexitySimple},
		{"帮我看看这个错误", ComplexitySimple},
		{"怎么用 assemble_team", ComplexitySimple},
		{"告诉我可用的管家", ComplexitySimple},
		{"列出所有团队", ComplexitySimple},
		{"显示系统状态", ComplexitySimple},
	}
	for _, tc := range cases {
		result := engine.Assess(tc.input)
		if result != tc.expected {
			t.Errorf("Assess(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestComplexityRuleEngine_ComplexIndicators(t *testing.T) {
	engine := NewComplexityRuleEngine()
	cases := []struct {
		input    string
		expected ComplexityLevel
	}{
		{"分析并对比两个方案的优劣", ComplexityComplex},
		{"设计一个跨行业协作流程", ComplexityComplex},
		{"规划多个团队的编排策略", ComplexityComplex},
		{"编写并设计一个完整的微服务系统", ComplexityComplex},
	}
	for _, tc := range cases {
		result := engine.Assess(tc.input)
		if result != tc.expected {
			t.Errorf("Assess(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestComplexityRuleEngine_ModerateDefault(t *testing.T) {
	engine := NewComplexityRuleEngine()
	cases := []struct {
		input    string
		expected ComplexityLevel
	}{
		{"帮我创建一个新的 Agent", ComplexityModerate},
		{"更新系统配置", ComplexityModerate},
		{"修复登录页面的样式问题", ComplexityModerate},
		{"hello world", ComplexityModerate},
	}
	for _, tc := range cases {
		result := engine.Assess(tc.input)
		if result != tc.expected {
			t.Errorf("Assess(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestComplexityRuleEngine_SingleComplexIndicator(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.Assess("分析这个日志文件")
	if result != ComplexityModerate {
		t.Errorf("Assess(%q) = %q, want %q (single complex indicator should downgrade to moderate)", "分析这个日志文件", result, ComplexityModerate)
	}
}

func TestComplexityRuleEngine_Reasoning(t *testing.T) {
	engine := NewComplexityRuleEngine()
	_ = engine.Assess("什么是 Agent？")
	reasoning := engine.LastReasoning()
	if reasoning == "" {
		t.Error("LastReasoning() should not be empty after Assess()")
	}
}

func TestComplexityRuleEngine_SimpleTakesPrecedenceOverComplex(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.Assess("解释一下什么是编排分析")
	if result != ComplexitySimple {
		t.Errorf("Assess(%q) = %q, want %q (simple pattern should take precedence)", "解释一下什么是编排分析", result, ComplexitySimple)
	}
}

func TestAssessComplexityTool(t *testing.T) {
	engine := NewComplexityRuleEngine()
	tool := NewAssessComplexityTool(engine, nil)
	if tool == nil {
		t.Fatal("NewAssessComplexityTool returned nil")
	}
}
