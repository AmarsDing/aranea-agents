package biz

import (
	"encoding/json"
	"fmt"
)

// hasFailedResults reports whether any team result is failed or blocked —
// the default template closing must not claim full success in that case
// (2026-07-25 Fix 3).
func hasFailedResults(results []TeamSynthesisResult) bool {
	for _, r := range results {
		if r.Status == "failed" || r.Status == "blocked" {
			return true
		}
	}
	return false
}

func (e *SynthesisEngine) synthesizePrompt(input SynthesisInput) string {
	resultsJSON, _ := json.Marshal(input.TeamResults)
	return fmt.Sprintf(
		"请综合以下 %d 个团队的执行结果，回答用户的原始问题。\n\n"+
			"用户问题: %s\n\n"+
			"团队结果:\n```json\n%s\n```\n\n"+
			"请提供结构化的综合分析，包括：1) 核心发现汇总 2) 各团队结论对比 3) 最终建议。\n"+
			"诚实性要求：如实反映每个团队的完成状态；存在失败或受阻团队时必须明确说明，"+
			"禁止虚构其结论或把提问澄清当作产出；如有未解决问题请汇总列出，供用户澄清。",
		len(input.TeamResults),
		input.SpiritQuery,
		string(resultsJSON),
	)
}

// defaultTemplate returns the built-in report template. The closing line is
// derived from team statuses: with failures it states the partial outcome
// honestly instead of claiming "所有团队已完成任务" (2026-07-25 Fix 3).
func (e *SynthesisEngine) defaultTemplate(hasFailed bool) string {
	closing := "基于以上 {{query}} 的多团队并行分析，所有团队已完成任务。"
	if hasFailed {
		closing = "基于以上 {{query}} 的多团队并行分析，部分团队执行失败（详见各团队状态）；结论仅基于成功团队的产出，失败团队的遗留问题需用户关注。"
	}
	return "# 团队执行结果综合报告\n\n{{results}}\n\n---\n\n" + closing
}
