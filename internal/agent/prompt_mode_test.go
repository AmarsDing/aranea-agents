package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

func TestCapabilityCueLevelForMode(t *testing.T) {
	if capabilityCueLevelForMode("complete") != cueLevelFull {
		t.Fatal("complete")
	}
	if capabilityCueLevelForMode("task") != cueLevelStandard {
		t.Fatal("task")
	}
	if capabilityCueLevelForMode("minimized") != cueLevelCompact {
		t.Fatal("minimized")
	}
	if capabilityCueLevelForMode("none") != cueLevelMinimal {
		t.Fatal("none")
	}
}

func TestRuntimeCapabilityCue_ModeTrimming(t *testing.T) {
	ag := biz.Agent{
		ID:               "a1",
		SystemPromptMode: "none",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:     true,
			SubagentsEnabled: false,
		},
	}
	cue := RuntimeCapabilityCue(t.Context(), Deps{}, ag)
	if strings.Contains(cue, "exec_command (shell_exec)") {
		t.Fatalf("none mode should omit long filesystem guidance: %q", cue)
	}
}

// 方案E（2026-08-20 token 成本审查）：memory self-marking 指令（~40 行
// <fact> 标签格式说明）只在 L3 事实层启用时注入——<fact> 标记的落点是
// memory_fact（L3），L3 关闭时指令纯属系统提示常驻浪费。
func TestBuildSystemPrompt_SelfMarkingGatedByL3(t *testing.T) {
	mk := func(settings *biz.AgentRuntimeSettings) biz.Agent {
		return biz.Agent{ID: "a1", AgentDescription: "desc", Settings: settings}
	}
	// L3 启用 → 注入 self-marking 指令。
	withL3 := BuildSystemPrompt(mk(&biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true}), nil, "complete")
	if !strings.Contains(withL3, "<memory_self_marking>") {
		t.Fatal("L3 enabled should inject self-marking instructions")
	}
	// L3 关闭（记忆总开关仍开）→ 不注入。
	withoutL3 := BuildSystemPrompt(mk(&biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: false}), nil, "complete")
	if strings.Contains(withoutL3, "<memory_self_marking>") {
		t.Fatal("L3 disabled must not inject self-marking instructions")
	}
	// 记忆总开关关闭 → 不注入（回归）。
	off := BuildSystemPrompt(mk(&biz.AgentRuntimeSettings{MemoryEnabled: false, L3Enabled: true}), nil, "complete")
	if strings.Contains(off, "<memory_self_marking>") {
		t.Fatal("memory disabled must not inject self-marking instructions")
	}
}

func TestIdentityDescriptionForAgent(t *testing.T) {
	files := []biz.AgentPromptFile{{Name: "IDENTITY.md", Body: "persona"}}
	ag := biz.Agent{DisplayName: "Bot", SystemPromptMode: "task"}
	if got := IdentityDescriptionForAgent(ag, files); got != "" {
		t.Fatalf("expected empty when IDENTITY.md present, got %q", got)
	}
	ag.SystemPromptMode = "complete"
	if got := IdentityDescriptionForAgent(ag, files); got != "" {
		t.Fatalf("complete mode skips display name when IDENTITY.md present, got %q", got)
	}
}

func TestRuntimeCapabilityCue_CapabilitiesDedup(t *testing.T) {
	ag := biz.Agent{
		ID:               "a1",
		SystemPromptMode: "task",
		Files: []biz.AgentPromptFile{
			{Name: "CAPABILITIES.md", Body: "use tools wisely"},
		},
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled: true,
			ToolsProfile: "full",
		},
	}
	cue := RuntimeCapabilityCue(t.Context(), Deps{}, ag)
	if strings.Contains(cue, "Effective tool keys this turn:") {
		t.Fatalf("should omit tool key list when CAPABILITIES.md present: %q", cue)
	}
	if !strings.Contains(cue, "CAPABILITIES.md") {
		t.Fatalf("expected CAPABILITIES.md hint: %q", cue)
	}
}

func TestSkillOptionsForPromptMode(t *testing.T) {
	profile, hints := skillOptionsForPromptMode("task")
	if profile != trpcllmagent.SkillToolProfileKnowledgeOnly || hints {
		t.Fatalf("task: profile=%v hints=%v", profile, hints)
	}
	profile, hints = skillOptionsForPromptMode("complete")
	if profile != trpcllmagent.SkillToolProfileFull || hints {
		t.Fatalf("complete: profile=%v hints=%v", profile, hints)
	}
}

func TestSkillOptionsForAgent_SpiritDropsExecSuite(t *testing.T) {
	ag := biz.Agent{
		SystemPromptMode: "complete",
		Settings:         &biz.AgentRuntimeSettings{ToolsProfile: "spirit"},
	}
	profile, hints := skillOptionsForAgent(ag)
	if profile != trpcllmagent.SkillToolProfileKnowledgeOnly || hints {
		t.Fatalf("spirit+complete must be knowledge_only, got profile=%v hints=%v", profile, hints)
	}
	allowed := allowedSkillToolsForAgent(ag)
	if len(allowed) != 1 || allowed[0] != trpcllmagent.SkillToolLoad {
		t.Fatalf("spirit must allow only skill_load, got %v", allowed)
	}
	ag.Settings.ToolsProfile = "chat_only"
	allowed = allowedSkillToolsForAgent(ag)
	if len(allowed) != 1 || allowed[0] != trpcllmagent.SkillToolLoad {
		t.Fatalf("chat_only must allow only skill_load, got %v", allowed)
	}
	// A0.5 (2026-08-25): read_only 契约只有读类工具，技能执行套件必须降为
	// KnowledgeOnly（管理层 agents tools_schema 立省 ~7.3K）；doc helpers
	// （load/list/select 均为读操作）保留 profile 默认，不加白名单。
	ag.Settings.ToolsProfile = "read_only"
	profile, hints = skillOptionsForAgent(ag)
	if profile != trpcllmagent.SkillToolProfileKnowledgeOnly || hints {
		t.Fatalf("read_only must be knowledge_only, got profile=%v hints=%v", profile, hints)
	}
	if allowedSkillToolsForAgent(ag) != nil {
		t.Fatalf("read_only must keep profile default skill tools, got %v", allowedSkillToolsForAgent(ag))
	}
	ag.Settings.ToolsProfile = "coding"
	profile, _ = skillOptionsForAgent(ag)
	if profile != trpcllmagent.SkillToolProfileFull {
		t.Fatalf("coding+complete must keep full skill suite, got %v", profile)
	}
	if allowedSkillToolsForAgent(ag) != nil {
		t.Fatalf("coding must keep profile default skill tools, got %v", allowedSkillToolsForAgent(ag))
	}
}
