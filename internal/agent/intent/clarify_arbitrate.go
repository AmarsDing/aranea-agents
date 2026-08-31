package intent

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
