package agent

import "aranea-agents/internal/llmcontext"

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
// (100% injection by design); MemoryPromptTotalBudgetChars stays the outer
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
