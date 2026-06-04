package tools

import (
	"strings"
	"sync"
)

type ComplexityLevel string

const (
	ComplexitySimple   ComplexityLevel = "simple"
	ComplexityModerate ComplexityLevel = "moderate"
	ComplexityComplex  ComplexityLevel = "complex"
)

var (
	lowerSimplePatterns = []string{
		"什么是", "解释一下", "帮我看看", "怎么用",
		"是什么意思", "告诉我", "列出", "显示",
		"what is", "explain", "show me", "how to use",
	}
	lowerComplexIndicators = []string{
		"分析", "对比", "编写", "设计", "规划", "编排",
		"多个", "跨行业", "团队", "协作", "流程",
		"analyze", "compare", "design", "plan", "orchestrate",
	}
	simpleAvailableTools    []string
	moderateAvailableTools  = []string{"assemble_team"}
	complexAvailableTools   = []string{"assemble_team", "check_team_progress", "cancel_team", "synthesize_results"}
)

type ComplexityRuleEngine struct {
	mu            sync.Mutex
	lastReasoning string
}

func NewComplexityRuleEngine() *ComplexityRuleEngine {
	return &ComplexityRuleEngine{}
}

func (r *ComplexityRuleEngine) Assess(message string) ComplexityLevel {
	r.mu.Lock()
	defer r.mu.Unlock()

	lower := strings.ToLower(message)

	for _, p := range lowerSimplePatterns {
		if strings.Contains(lower, p) {
			r.lastReasoning = "匹配简单问答模式: " + p
			return ComplexitySimple
		}
	}

	complexHits := 0
	for _, p := range lowerComplexIndicators {
		if strings.Contains(lower, p) {
			complexHits++
		}
	}
	if complexHits >= 2 {
		r.lastReasoning = "匹配多个复杂任务指标"
		return ComplexityComplex
	}
	if complexHits == 1 {
		r.lastReasoning = "匹配 1 个复杂任务指标，但不足以确定，降级为 moderate"
		return ComplexityModerate
	}

	r.lastReasoning = "无法通过规则确定复杂度，使用安全默认值 moderate"
	return ComplexityModerate
}

func (r *ComplexityRuleEngine) LastReasoning() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastReasoning
}
