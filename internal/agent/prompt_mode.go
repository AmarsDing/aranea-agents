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
		// Directory hints in the overview mutate system[0] with machine
		// paths; loaded bodies go to tool results instead.
		return trpcllmagent.SkillToolProfileFull, false
	}
	return trpcllmagent.SkillToolProfileKnowledgeOnly, false
}

// skillOptionsForAgent picks the skill-tool suite. Spirit / chat_only are
// orchestrators: they must not register skill_exec / skill_run / stdin+poll
// (those schemas dominated tools_schema ~20k tok). complete prompt mode still
// keeps IDENTITY files; only the execution tools are dropped.
func skillOptionsForAgent(ag biz.Agent) (trpcllmagent.SkillToolProfile, bool) {
	if ag.Settings != nil {
		switch strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile)) {
		case "spirit", "chat_only":
			return trpcllmagent.SkillToolProfileKnowledgeOnly, false
		}
	}
	return skillOptionsForPromptMode(ag.SystemPromptMode)
}

// allowedSkillToolsForAgent returns an explicit skill-tool allowlist, or nil
// to keep the profile default. Spirit / chat_only keep skill_load (knowledge
// entry; docs can ride on skill_load args) and drop skill_select_docs /
// skill_list_docs — those are framework-injected after deferred wrapping, so
// identity-mapping them in the catalog cannot hide their schema.
func allowedSkillToolsForAgent(ag biz.Agent) []trpcllmagent.SkillTool {
	if ag.Settings == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile)) {
	case "spirit", "chat_only":
		return []trpcllmagent.SkillTool{trpcllmagent.SkillToolLoad}
	}
	return nil
}
