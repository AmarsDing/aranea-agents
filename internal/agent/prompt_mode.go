package agent

import (
	"strings"

	"aranea-agents/internal/biz"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

type capabilityCueLevel int

const (
	cueLevelMinimal capabilityCueLevel = iota
	cueLevelCompact
	cueLevelStandard
	cueLevelFull
)

func capabilityCueLevelForMode(mode string) capabilityCueLevel {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "none":
		return cueLevelMinimal
	case "minimized":
		return cueLevelCompact
	case "task":
		return cueLevelStandard
	default:
		return cueLevelFull
	}
}

// SkillsUseFullProfile returns whether to inject full skill tooling guidance (complete mode).
func SkillsUseFullProfile(mode string) bool {
	return capabilityCueLevelForMode(mode) >= cueLevelFull
}

// HasFilteredPromptFile reports whether a named prompt file is included for the mode with non-empty body.
func HasFilteredPromptFile(files []biz.AgentPromptFile, mode, name string) bool {
	name = strings.TrimSpace(name)
	for _, f := range biz.FilesForMode(files, mode) {
		if strings.EqualFold(strings.TrimSpace(f.Name), name) && strings.TrimSpace(f.Body) != "" {
			return true
		}
	}
	return false
}

// IdentityDescriptionForAgent avoids duplicating IDENTITY.md in the Identity processor block.
func IdentityDescriptionForAgent(ag biz.Agent, files []biz.AgentPromptFile) string {
	if HasFilteredPromptFile(files, ag.SystemPromptMode, "IDENTITY.md") {
		return ""
	}
	return strings.TrimSpace(ag.DisplayName)
}

func skillOptionsForPromptMode(mode string) (trpcllmagent.SkillToolProfile, bool) {
	if SkillsUseFullProfile(mode) {
		return trpcllmagent.SkillToolProfileFull, true
	}
	return trpcllmagent.SkillToolProfileKnowledgeOnly, false
}
