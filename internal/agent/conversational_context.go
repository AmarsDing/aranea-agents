package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// Conversational assembly / compression defaults (Wave 2, 2026-08-28).
// Product chat window stays 256K; these caps apply only to Spirit / chat
// profiles so team specialists keep long tool-loop headroom.
const (
	conversationalAssemblySoftTokens    = 40000
	conversationalAssemblyHardTokens    = 60000
	conversationalCompressWindowTokens  = 64000
	conversationalCompressSoftTopTokens = 32768
)

// usesConversationalContextBudget reports whether this agent is a
// non-team chat face: Spirit, voice butler, or an explicit chat tools
// profile. Specialists (coding / domain profiles) return false so
// hard=0 still means assembly-off and compress stays 90%×256K.
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
// hard<=0 keeps specialists off and turns Spirit/chat on at 40K/60K.
func resolveAssemblyBudget(ag biz.Agent) (soft, hard int, enabled bool) {
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
	return 0, 0, false
}
