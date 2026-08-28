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
// read_only（2026-08-25 A0.5）：profile 契约本就只有读类工具（datetime/
// read_file/search_*，agent_tool_policy.go toolProfiles），携带技能执行工具
// 既越权又白烧 ~7.3K schema（session-eval-20260825 实测管理层 tools_schema
// 10.2-11.2K 中 top4 全是 skill_exec/run/stdin/poll），同样降为 KnowledgeOnly。
func skillOptionsForAgent(ag biz.Agent) (trpcllmagent.SkillToolProfile, bool) {
	if ag.Settings != nil {
		switch strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile)) {
		case "coding":
			return skillOptionsForPromptMode(ag.SystemPromptMode)
		case "spirit", "chat_only", "read_only", "minimal", "research":
			return trpcllmagent.SkillToolProfileKnowledgeOnly, false
		}
	}
	// 岗位默认 complete 模式不再挂载 skill_exec/run/stdin/poll（C6 之后最大
	// 单次杠杆，~7.3K）。未标 coding 的 profile（含 nil settings）一律只读。
	return trpcllmagent.SkillToolProfileKnowledgeOnly, false
}

// allowedSkillToolsForAgent returns an explicit skill-tool allowlist, or nil
// to keep the profile default. Spirit / chat_only keep skill_load (knowledge
// entry; docs can ride on skill_load args) and drop skill_select_docs /
// skill_list_docs — those are framework-injected after deferred wrapping, so
// identity-mapping them in the catalog cannot hide their schema.
// read_only / minimal / research also keep skill_load only (P1 schema ~7.3K).
func allowedSkillToolsForAgent(ag biz.Agent) []trpcllmagent.SkillTool {
	if ag.Settings == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile)) {
	case "spirit", "chat_only", "read_only", "minimal", "research":
		return []trpcllmagent.SkillTool{trpcllmagent.SkillToolLoad}
	}
	return nil
}
