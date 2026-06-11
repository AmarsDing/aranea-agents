package tools

import (
	"strings"
)

type ComplexityLevel string

const (
	ComplexitySimple   ComplexityLevel = "simple"
	ComplexityModerate ComplexityLevel = "moderate"
	ComplexityComplex  ComplexityLevel = "complex"
)

type SuggestedPath string

const (
	PathDirectAnswer  SuggestedPath = "direct_answer"
	PathSingleButler  SuggestedPath = "single_butler"
	PathOrchestrator  SuggestedPath = "orchestrator"
)

// ComplexityAssessment holds the full result of a complexity assessment.
type ComplexityAssessment struct {
	Level          ComplexityLevel
	SuggestedPath  SuggestedPath
	RequiredSkills []string
	Reasoning      string
}

var (
	lowerSimplePatterns = []string{
		"什么是", "解释一下", "帮我看看", "怎么用",
		"是什么意思", "告诉我", "列出", "显示",
		"what is", "explain", "show me", "how to use",
		"查询", "查找", "搜索", "获取",
		"lookup", "search", "fetch", "retrieve",
	}
	lowerComplexIndicators = []string{
		"分析", "对比", "编写", "设计", "规划", "编排",
		"多个", "跨行业", "团队", "协作", "流程",
		"analyze", "compare", "design", "plan", "orchestrate",
		"重构", "迁移", "集成", "部署", "优化",
		"refactor", "migrate", "integrate", "deploy", "optimize",
	}
	moderateIndicators = []string{
		"创建", "修改", "更新", "删除", "配置",
		"修复", "调试", "测试", "转换",
		"create", "modify", "update", "delete", "configure",
		"fix", "debug", "test", "convert",
	}
	simpleAvailableTools    = []string{}
	moderateAvailableTools  = []string{"plan_and_execute"}
	complexAvailableTools   = []string{"plan_and_execute", "check_progress", "cancel_orchestration", "synthesize_results"}
)

type ComplexityRuleEngine struct{}

func NewComplexityRuleEngine() *ComplexityRuleEngine {
	return &ComplexityRuleEngine{}
}

func (r *ComplexityRuleEngine) Assess(message string) ComplexityLevel {
	return r.AssessDetailed(message).Level
}

// AssessDetailed returns a full complexity assessment including suggested path,
// required skills, and reasoning. The result is self-contained and does not
// rely on shared mutable state, making it safe for concurrent use.
func (r *ComplexityRuleEngine) AssessDetailed(message string) ComplexityAssessment {

	lower := strings.ToLower(message)

	// 1. Check simple patterns first (highest priority)
	for _, p := range lowerSimplePatterns {
		if strings.Contains(lower, p) {
			reasoning := "匹配简单问答模式: " + p
			return ComplexityAssessment{
				Level:          ComplexitySimple,
				SuggestedPath:  PathDirectAnswer,
				RequiredSkills: nil,
				Reasoning:      reasoning,
			}
		}
	}

	// 2. Count complex indicators
	complexHits := 0
	var matchedComplex []string
	for _, p := range lowerComplexIndicators {
		if strings.Contains(lower, p) {
			complexHits++
			matchedComplex = append(matchedComplex, p)
		}
	}

	// 3. Count moderate indicators
	moderateHits := 0
	var matchedModerate []string
	for _, p := range moderateIndicators {
		if strings.Contains(lower, p) {
			moderateHits++
			matchedModerate = append(matchedModerate, p)
		}
	}

	// 4. Determine level based on indicator counts
	if complexHits >= 2 {
		reasoning := "匹配多个复杂任务指标: " + strings.Join(matchedComplex, ", ")
		return ComplexityAssessment{
			Level:          ComplexityComplex,
			SuggestedPath:  PathOrchestrator,
			RequiredSkills: complexAvailableTools,
			Reasoning:      reasoning,
		}
	}

	if complexHits == 1 {
		reasoning := "匹配 1 个复杂任务指标 (" + matchedComplex[0] + ")，但不足以确定，降级为 moderate"
		return ComplexityAssessment{
			Level:          ComplexityModerate,
			SuggestedPath:  PathSingleButler,
			RequiredSkills: moderateAvailableTools,
			Reasoning:      reasoning,
		}
	}

	if moderateHits >= 1 {
		reasoning := "匹配中等任务指标: " + strings.Join(matchedModerate, ", ")
		return ComplexityAssessment{
			Level:          ComplexityModerate,
			SuggestedPath:  PathSingleButler,
			RequiredSkills: moderateAvailableTools,
			Reasoning:      reasoning,
		}
	}

	reasoning := "无法通过规则确定复杂度，使用安全默认值 moderate"
	return ComplexityAssessment{
		Level:          ComplexityModerate,
		SuggestedPath:  PathSingleButler,
		RequiredSkills: moderateAvailableTools,
		Reasoning:      reasoning,
	}
}

// LastReasoning returns the reasoning from the most recent Assess/AssessDetailed call.
// Deprecated: Use AssessDetailed().Reasoning instead for thread-safe access.
func (r *ComplexityRuleEngine) LastReasoning() string {
	return ""
}
