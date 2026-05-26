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
	if profile != trpcllmagent.SkillToolProfileFull || !hints {
		t.Fatalf("complete: profile=%v hints=%v", profile, hints)
	}
}
