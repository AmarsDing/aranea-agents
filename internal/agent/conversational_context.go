package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// Conversational assembly / compression defaults (Wave 2, 2026-08-28).
// Product chat window stays 256K; Spirit / chat profiles use a tighter
// 40K/60K assembly cap. Specialists (coding / ops tool loops) use a
// higher 64K/96K cap so S05-scale GNS3 traces still trim (FIT-BUDGET-1)
// without inheriting the Spirit 40K/60K chat budget.
const (
	conversationalAssemblySoftTokens    = 40000
	conversationalAssemblyHardTokens    = 60000
	specialistAssemblySoftTokens        = 65536
	specialistAssemblyHardTokens        = 98304
	conversationalCompressWindowTokens  = 64000
	conversationalCompressSoftTopTokens = 32768
)

// usesConversationalContextBudget reports whether this agent is a
// non-team chat face: Spirit, voice butler, or an explicit chat tools
// profile. Specialists (coding / domain profiles) return false so they
// keep the wider compress window (90%×256K) and the specialist assembly cap.
func usesConversationalContextBudget(ag biz.Agent) bool {
	key := strings.TrimSpace(ag.AgentKey)
	switch key {
	case biz.SpiritAgentKey, biz.VoiceButlerAgentKey:
		return true
	}
	if ag.Settings == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile)) {
	case "chat", "chat_only":
		return true
	}
	return false
}

// resolveAssemblyBudget returns the soft/hard assembly token caps.
// Explicit hard>0 always wins (ops SQL grayscale, admin overlay).
// hard<0 forces the gate off. hard==0 uses profile defaults: Spirit/chat
// 40K/60K, named specialists 64K/96K. Anonymous (empty AgentKey) stays off.
func resolveAssemblyBudget(ag biz.Agent) (soft, hard int, enabled bool) {
	if ag.Settings != nil && ag.Settings.AssemblyBudgetHardTokens < 0 {
		return 0, 0, false
	}
	if ag.Settings != nil && ag.Settings.AssemblyBudgetHardTokens > 0 {
		hard = ag.Settings.AssemblyBudgetHardTokens
		soft = ag.Settings.AssemblyBudgetSoftTokens
		if soft <= 0 {
			soft = hard * 2 / 3
		}
		if soft > hard {
			soft = hard
		}
		return soft, hard, true
	}
	if usesConversationalContextBudget(ag) {
		return conversationalAssemblySoftTokens, conversationalAssemblyHardTokens, true
	}
	if strings.TrimSpace(ag.AgentKey) == "" {
		return 0, 0, false
	}
	return specialistAssemblySoftTokens, specialistAssemblyHardTokens, true
}
