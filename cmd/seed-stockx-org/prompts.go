package main

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

const stockDisclaimer = `
## 合规与风险护栏
- 禁止给出具体买卖价位、目标价、仓位建议
- 禁止表述确定性收益或「必涨/必跌」
- 所有结论必须基于工具返回的数据，禁止凭空捏造
- 输出末尾必须附「本报告仅供学习研究，不构成投资建议」
`

func buildPromptFiles(spec agentSpec) []biz.AgentPromptFile {
	role := spec.roleTitle
	task := spec.taskBrief
	output := spec.outputContract
	identity := fmt.Sprintf(`# IDENTITY

你是 **%s**（%s），隶属于 Stockx AI 投研团队的 Daily Stock Analysis 场景。

%s
`, spec.displayName, role, spec.description)

	soul := fmt.Sprintf(`# SOUL

专业、克制、数据驱动。用中文输出，术语准确，避免煽动性措辞。
作为 %s，你专注 %s，不越界到其他分析维度。
`, role, task)

	capabilities := buildCapabilities(spec)
	rule := buildRule(spec)
	agentsCore := buildAgentsCore(spec)
	agentsTask := buildAgentsTask(spec)

	files := []biz.AgentPromptFile{
		{Name: "IDENTITY.md", Body: identity, SortOrder: 10},
		{Name: "SOUL.md", Body: soul, SortOrder: 20},
		{Name: "CAPABILITIES.md", Body: capabilities, SortOrder: 30},
		{Name: "AGENTS_CORE.md", Body: agentsCore, SortOrder: 40},
		{Name: "AGENTS_TASK.md", Body: agentsTask, SortOrder: 50},
		{Name: "RULE.md", Body: rule, SortOrder: 60},
		{Name: "USER_PREDEFINED.md", Body: "# USER_PREDEFINED\n\n用户问题可能包含股票代码、名称、板块或组合诊断请求。优先用 stock_resolve 类工具消歧。", SortOrder: 70},
		{Name: "USER.md", Body: "# USER\n\n（运行时注入用户上下文）", SortOrder: 80},
		{Name: "HEARTBEAT.md", Body: "# HEARTBEAT\n\n（后台任务心跳占位）", SortOrder: 90},
	}
	if strings.TrimSpace(output) != "" {
		files = append(files, biz.AgentPromptFile{
			Name: "OUTPUT_CONTRACT.md", Body: output, SortOrder: 35,
		})
	}
	return files
}

func buildCapabilities(spec agentSpec) string {
	var b strings.Builder
	b.WriteString("# CAPABILITIES\n\n")
	b.WriteString("## 主要职责\n\n")
	b.WriteString(spec.taskBrief + "\n\n")
	if len(spec.toolsAllow) > 0 {
		b.WriteString("## 授权工具\n\n")
		for _, t := range spec.toolsAllow {
			b.WriteString("- `" + t + "`\n")
		}
		b.WriteString("\n")
	}
	if len(spec.skillsAllow) > 0 {
		b.WriteString("## 必备 Skill\n\n")
		for _, s := range spec.skillsAllow {
			b.WriteString("- `" + s + "`\n")
		}
		b.WriteString("\n")
	}
	if spec.subagentsEnabled {
		b.WriteString("## 协作\n\n")
		b.WriteString("通过 AgentTool 调度团队成员；成员输出需汇总后再回复用户。\n")
	}
	return b.String()
}

func buildRule(spec agentSpec) string {
	var b strings.Builder
	b.WriteString("# RULE\n\n")
	b.WriteString(stockDisclaimer)
	b.WriteString("\n")
	if spec.maxOutputTokens > 0 {
		b.WriteString(fmt.Sprintf("\n- 作为 AgentTool 被调用时，输出长度建议 ≤ %d token\n", spec.maxOutputTokens))
	}
	if spec.outputContract != "" {
		b.WriteString("\n- 严格遵守 OUTPUT_CONTRACT 中的结构与字段\n")
	}
	return b.String()
}

func buildAgentsCore(spec agentSpec) string {
	return fmt.Sprintf(`# AGENTS_CORE

## 角色
%s — %s

## 工作原则
1. 仅基于工具返回结果分析，每条关键结论标注 data_ref（tool_call_id）
2. 使用结构化 Markdown（H2 段落 + 表格）
3. 不确定时明确说明数据缺失或时效限制
4. 场景命名空间：stockx / daily_stock_analysis

## 模型偏好
Provider: %s | Model: %s
`, spec.displayName, spec.roleTitle, spec.provider, spec.model)
}

func buildAgentsTask(spec agentSpec) string {
	return fmt.Sprintf(`# AGENTS_TASK

## 当前任务域
%s

## 输出要求
%s

## 协作上下文
- Team 模式：%s
- 工具配置档：%s
`, spec.taskBrief, strings.TrimSpace(spec.outputContract), spec.teamRole, spec.toolsProfile)
}
