package skillruntime

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// skillOverviewHeader 与框架默认 availableSkillsText 的头部字节一致
// （pkg/trpc-agent-go/internal/flow/processor/skills.go skillsOverviewHeader）。
// 未截断时输出与框架默认渲染逐字节相同，保持 system prompt 前缀缓存稳定。
const skillOverviewHeader = "Available skills:\n"

// RenderSkillOverviewBudgeted 渲染「Available skills」概览块，按符文预算截断：
// 逐行累计，首条超预算行即停止（整行粒度，不半截描述），并追加
// "(N more skills available)" 提示告知模型集合不完整。maxChars <= 0 不限量。
// 同输入必同字节（确定性）——prompt 缓存前缀稳定的前提。
func RenderSkillOverviewBudgeted(sums []trpcskill.Summary, maxChars int) string {
	var b strings.Builder
	b.WriteString(skillOverviewHeader)
	if maxChars <= 0 {
		for _, s := range sums {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
		return b.String()
	}
	used := utf8.RuneCountInString(skillOverviewHeader)
	shown := 0
	for _, s := range sums {
		line := fmt.Sprintf("- %s: %s\n", s.Name, s.Description)
		n := utf8.RuneCountInString(line)
		if used+n > maxChars {
			break
		}
		b.WriteString(line)
		used += n
		shown++
	}
	if omitted := len(sums) - shown; omitted > 0 {
		fmt.Fprintf(&b, "(%d more skills available)\n", omitted)
	}
	return b.String()
}

// OverviewBudgetFromRuntime 从 agent skill runtime policy 解析生效的概览预算
// （符文数）。0 = 不限（框架默认全量渲染）。
func OverviewBudgetFromRuntime(runtime RuntimeSettings) int {
	raw := "{}"
	if runtime != nil && strings.TrimSpace(runtime.GetSkillRuntimeJSON()) != "" {
		raw = runtime.GetSkillRuntimeJSON()
	}
	return biz.ParseSkillRuntimePolicy(raw).OverviewBudgetChars()
}

// RunOptionWithOverviewBudget 按 agent skill runtime policy 安装预算化概览渲染器
// （request-scoped AvailableSkillsRenderer）。显式 overview_max_chars=0 时不安装，
// 保留框架默认全量渲染。渲染器只改概览文本，不改变实际可用 skill 集合。
func RunOptionWithOverviewBudget(runtime RuntimeSettings) trpcagent.RunOption {
	budget := OverviewBudgetFromRuntime(runtime)
	if budget <= 0 {
		return func(*trpcagent.RunOptions) {}
	}
	return trpcagent.WithAvailableSkillsRenderer(func(_ context.Context, req trpcagent.AvailableSkillsRenderRequest) string {
		return RenderSkillOverviewBudgeted(req.Summaries, budget)
	})
}
