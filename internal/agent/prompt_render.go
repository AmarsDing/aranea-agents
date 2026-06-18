package agent

import (
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/prompt"
)

// TECH-DEBT(B-5): RenderPromptTemplate/RenderCapabilityCue 已实现但未接入生产路径。
// 阻塞类型：设计前提不匹配（模板渲染 vs 结构化拼接）。
// 项目的 BuildSystemPrompt() 使用 strings.Builder 结构化拼接 capability cues/sections/runtime cues，
// 而非模板字符串 + 变量替换。Render() 适用于模板化 Agent 定义，当前项目 prompt 架构非模板化。
// 解除方案：Phase 1 AgentFactory 实施时重新评估（若引入模板化 Agent 定义则接入）。
// 详见 docs/trpc-agent-go/alignment-plan.md §十一/B-5。

// promptRequiredPlaceholders lists the placeholders that are expected in
// RenderPromptTemplate and RenderCapabilityCue templates. If any of these
// are missing from the template, a warning is logged but rendering
// proceeds (graceful degradation).
var promptRequiredPlaceholders = []string{"agent_key", "current_date"}

// RenderPromptTemplate renders a prompt template with variable substitution using
// the framework's prompt.Text.Render() with SyntaxMixedBrace (supports both {name}
// and {{name}} placeholders).
//
// Before rendering, ValidateRequired is called for common required placeholders
// (agent_key, current_date). If validation fails, a warning is logged but the
// render proceeds — this is graceful degradation, not a hard failure.
//
// This is the framework-aligned rendering approach. Existing manual concatenation
// in prompt.go (BuildSystemPrompt, StaticRuntimeCapabilityCue, etc.) is TECH-DEBT
// to be gradually migrated to this utility.
func RenderPromptTemplate(template string, vars map[string]string, lg loggateway.Logger) (string, error) {
	t := prompt.Text{
		Template: template,
		Syntax:   prompt.SyntaxMixedBrace,
	}
	if err := t.ValidateRequired(promptRequiredPlaceholders...); err != nil {
		lg.Warn("Prompt 模板缺少必要占位符",
			loggateway.StepID("prompt.validate_required"),
			loggateway.Err(err))
	}
	env := prompt.RenderEnv{Vars: vars}
	return t.Render(env)
}

// RenderCapabilityCue renders a runtime capability cue template with variable
// substitution. It uses the same SyntaxMixedBrace mode as RenderPromptTemplate
// but is named separately to distinguish capability-cue rendering from general
// prompt rendering, allowing future divergence (e.g. different unknown-behavior
// policy) without breaking callers.
//
// Before rendering, ValidateRequired is called for common required placeholders.
// If validation fails, a warning is logged but the render proceeds.
func RenderCapabilityCue(template string, vars map[string]string, lg loggateway.Logger) (string, error) {
	t := prompt.Text{
		Template: template,
		Syntax:   prompt.SyntaxMixedBrace,
	}
	if err := t.ValidateRequired(promptRequiredPlaceholders...); err != nil {
		lg.Warn("Capability cue 模板缺少必要占位符",
			loggateway.StepID("prompt.validate_required"),
			loggateway.Err(err))
	}
	env := prompt.RenderEnv{Vars: vars}
	return t.Render(env)
}
