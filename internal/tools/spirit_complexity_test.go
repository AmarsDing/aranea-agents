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
		{"hello world", ComplexityModerate},
		{"随便聊聊", ComplexityModerate},
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
	result := engine.AssessDetailed("什么是 Agent？")
	if result.Reasoning == "" {
		t.Error("AssessDetailed().Reasoning should not be empty")
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

// --- AssessDetailed tests ---

func TestAssessDetailed_Simple(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.AssessDetailed("什么是 Agent？")

	if result.Level != ComplexitySimple {
		t.Errorf("Level = %q, want %q", result.Level, ComplexitySimple)
	}
	if result.SuggestedPath != PathDirectAnswer {
		t.Errorf("SuggestedPath = %q, want %q", result.SuggestedPath, PathDirectAnswer)
	}
	if len(result.RequiredSkills) != 0 {
		t.Errorf("RequiredSkills = %v, want empty", result.RequiredSkills)
	}
	if result.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}
}

func TestAssessDetailed_ModerateBySingleComplex(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.AssessDetailed("分析这个日志文件")

	if result.Level != ComplexityModerate {
		t.Errorf("Level = %q, want %q", result.Level, ComplexityModerate)
	}
	if result.SuggestedPath != PathSingleButler {
		t.Errorf("SuggestedPath = %q, want %q", result.SuggestedPath, PathSingleButler)
	}
	if len(result.RequiredSkills) == 0 {
		t.Error("RequiredSkills should not be empty for moderate")
	}
	// moderate should have plan_and_execute
	found := false
	for _, s := range result.RequiredSkills {
		if s == "plan_and_execute" {
			found = true
		}
	}
	if !found {
		t.Error("RequiredSkills should contain plan_and_execute")
	}
}

func TestAssessDetailed_ModerateByModerateIndicator(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.AssessDetailed("创建一个新的 Agent")

	if result.Level != ComplexityModerate {
		t.Errorf("Level = %q, want %q", result.Level, ComplexityModerate)
	}
	if result.SuggestedPath != PathSingleButler {
		t.Errorf("SuggestedPath = %q, want %q", result.SuggestedPath, PathSingleButler)
	}
	if result.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}
}

func TestAssessDetailed_Complex(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.AssessDetailed("分析并对比两个方案的优劣")

	if result.Level != ComplexityComplex {
		t.Errorf("Level = %q, want %q", result.Level, ComplexityComplex)
	}
	if result.SuggestedPath != PathOrchestrator {
		t.Errorf("SuggestedPath = %q, want %q", result.SuggestedPath, PathOrchestrator)
	}
	if len(result.RequiredSkills) == 0 {
		t.Error("RequiredSkills should not be empty for complex")
	}
	// complex should have all four tools
	expectedTools := []string{"plan_and_execute", "check_progress", "cancel_orchestration", "synthesize_results"}
	for _, et := range expectedTools {
		found := false
		for _, s := range result.RequiredSkills {
			if s == et {
				found = true
			}
		}
		if !found {
			t.Errorf("RequiredSkills should contain %q", et)
		}
	}
}

func TestAssessDetailed_DefaultModerate(t *testing.T) {
	engine := NewComplexityRuleEngine()
	result := engine.AssessDetailed("hello world")

	if result.Level != ComplexityModerate {
		t.Errorf("Level = %q, want %q", result.Level, ComplexityModerate)
	}
	if result.SuggestedPath != PathSingleButler {
		t.Errorf("SuggestedPath = %q, want %q", result.SuggestedPath, PathSingleButler)
	}
	if result.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}
}

func TestAssessDetailed_NewSimplePatterns(t *testing.T) {
	engine := NewComplexityRuleEngine()
	cases := []struct {
		input string
	}{
		{"查询用户列表"},
		{"查找相关文档"},
		{"搜索最近的变更"},
		{"获取系统配置"},
		{"lookup the agent config"},
		{"search for relevant docs"},
		{"fetch the latest data"},
		{"retrieve user info"},
	}
	for _, tc := range cases {
		result := engine.AssessDetailed(tc.input)
		if result.Level != ComplexitySimple {
			t.Errorf("AssessDetailed(%q).Level = %q, want %q", tc.input, result.Level, ComplexitySimple)
		}
		if result.SuggestedPath != PathDirectAnswer {
			t.Errorf("AssessDetailed(%q).SuggestedPath = %q, want %q", tc.input, result.SuggestedPath, PathDirectAnswer)
		}
	}
}

func TestAssessDetailed_NewComplexPatterns(t *testing.T) {
	engine := NewComplexityRuleEngine()
	cases := []struct {
		input string
	}{
		{"重构并优化整个系统架构"},
		{"迁移并集成多个服务"},
		{"refactor and optimize the codebase"},
		{"migrate and integrate multiple services"},
	}
	for _, tc := range cases {
		result := engine.AssessDetailed(tc.input)
		if result.Level != ComplexityComplex {
			t.Errorf("AssessDetailed(%q).Level = %q, want %q", tc.input, result.Level, ComplexityComplex)
		}
		if result.SuggestedPath != PathOrchestrator {
			t.Errorf("AssessDetailed(%q).SuggestedPath = %q, want %q", tc.input, result.SuggestedPath, PathOrchestrator)
		}
	}
}

func TestAssessDetailed_BackwardCompatWithAssess(t *testing.T) {
	engine := NewComplexityRuleEngine()
	inputs := []string{
		"什么是 Agent？",
		"分析并对比两个方案",
		"创建一个新的 Agent",
		"hello world",
	}
	for _, input := range inputs {
		assessResult := engine.Assess(input)
		detailedResult := engine.AssessDetailed(input)
		if assessResult != detailedResult.Level {
			t.Errorf("Assess(%q) = %q but AssessDetailed(%q).Level = %q, should match",
				input, assessResult, input, detailedResult.Level)
		}
	}
}
