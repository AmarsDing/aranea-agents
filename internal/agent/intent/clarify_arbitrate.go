package intent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// ArbitrateClarification is the dual-source clarify decision (Q2).
// Intent may propose needs_clarification; memory/recommended-defaults must
// not silently rewrite the lane:
//   - explicit instruction (named device/URL/IP) → strip clarify (armB)
//   - underspecified "帮我做个…" → keep clarify; caller must hang, not auto-resolve (armA)
//
// High-risk flags always keep clarify.
func ArbitrateClarification(userText string, art *Artifact) *Artifact {
	if art == nil || !art.NeedsClarification() {
		return art
	}
	if art.HasHighRiskFlag() {
		return art
	}
	if LooksLikeExplicitInstruction(userText) {
		return art.CloneWithoutClarification()
	}
	return art
}

// ShouldSkipAutoResolveClarification reports armA: recommended defaults
// must not "draft as usual" over a vague request. The gate should hang.
func ShouldSkipAutoResolveClarification(userText string, art *Artifact) bool {
	if art == nil || !art.NeedsClarification() {
		return false
	}
	return LooksLikeUnderspecifiedTask(userText)
}

// UnderspecifiedClarifyArtifact is the single-writer clarify commit when
// the user asked for a nameless deliverable. Intent LLM proposing
// needs_clarification is optional — Wave 4 arbitration never ran on
// 0831-r2 S02 because intent never proposed clarify.
func UnderspecifiedClarifyArtifact(userText string) *Artifact {
	return &Artifact{
		RefinedGoal: strings.TrimSpace(userText),
		IntentKind:  "task",
		Ambiguities: []string{"交付物缺少对象、受众或验收口径"},
		RiskFlags:   []string{RiskFlagNeedsClarification},
		Clarifications: []ClarificationQuestion{{
			Question: "这份交付物给谁用、要达到什么验收口径？",
			Mode:     biz.ClarificationModeSingle,
			Options:  []string{"还缺对象/受众", "还缺验收口径/篇幅", "两者都缺"},
		}},
	}
}
