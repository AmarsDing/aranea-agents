package agent

import "aranea-agents/internal/llmcontext"

// recallL2GistMaxRunes caps one L2 episode line in recall cue blocks.
// L2 outcome_summary 是完整 markdown 答复（结论+表格，常 200~400+ 字符）；
// 注入块只需要「发生了什么」的 gist，全文注入会吃掉共享的 L2+L3 recall 预算、
// 把 L3 事实挤出注入块（2026-08-18 域 B 评测 up-03 缺陷根因：3 条 L2 长摘要
// 占掉 ~700/800 token，答案承载的 L3 变体仅余 2 条容量而被截断）。
// 160 runes 约 105 tokens，3 条 L2 合计 ~320 tokens，为 L3 留足份额。
const recallL2GistMaxRunes = 160

// capL2GistLine truncates an L2 episode cue line to the gist budget.
// Non-L2 lines (short L3 statements) are returned unchanged by callers.
func capL2GistLine(line string) string {
	r := []rune(line)
	if len(r) <= recallL2GistMaxRunes {
		return line
	}
	return string(r[:recallL2GistMaxRunes]) + "…"
}

// recallLinePacker packs recall lines into the L2/L3 recall-block token budget
// (FR-12/P2: biz.MemoryRuntimePolicy.L3RecallBudgetTokens, 评审 §6.4「预算：
// 默认 ≤800 tokens（可配档位），超出按分截断」).
//
// Lines arrive score-descended. A line is kept only while the estimated
// remaining budget covers it; packing continues after a skip so smaller
// lower-scored lines can still fill the tail — higher scores get priority
// ("按分截断"), not a hard cut at the first overflow.
//
// The resident profile card and pinned preference block bypass this packer
// (100% injection by design); MemoryPromptTotalBudgetTokens stays the outer
// backstop for the whole memory cue.
type recallLinePacker struct {
	remaining int
}

// newRecallLinePacker returns a packer for budgetTokens, or nil when the
// budget is unlimited (<=0). Callers resolve policy defaults beforehand, so
// nil here means "budget feature explicitly off", never "use default".
func newRecallLinePacker(budgetTokens int) *recallLinePacker {
	if budgetTokens <= 0 {
		return nil
	}
	return &recallLinePacker{remaining: budgetTokens}
}

// allow reports whether line fits the remaining budget, consuming it when
// kept. Token estimates come from the shared calibrated estimator
// (llmcontext.EstimateTokensFromChars) so all budget call sites share one
// provider-calibrated chars-per-token ratio.
func (p *recallLinePacker) allow(line string) bool {
	if p == nil {
		return true
	}
	est := llmcontext.EstimateTokensFromChars(len([]rune(line)))
	if est <= 0 {
		est = 1
	}
	if est > p.remaining {
		return false
	}
	p.remaining -= est
	return true
}
