package agent

import (
	"testing"

	"aranea-agents/internal/biz"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

// TestSkillRunRequireLoadedInvariant pins the precondition for
// WithSkillRunRequireSkillLoaded(true) in trpc_build.go: the framework panics
// when requireLoaded && (Run||Exec) && !Load. Aranea reaches skill tools via
// two paths — profile preset (allowedSkillToolsForAgent == nil) or explicit
// allowlist (spirit/chat_only). This test guarantees neither path can produce
// a Run/Exec-without-Load combination:
//
//   - Allowlist path: must always contain SkillToolLoad.
//   - Profile path: must resolve to a known built-in profile whose preset
//     keeps Load=true (both Full and KnowledgeOnly do; KnowledgeOnly also has
//     Run=false, making the panic condition unreachable).
func TestSkillRunRequireLoadedInvariant(t *testing.T) {
	agents := map[string]biz.Agent{
		"spirit allowlist": {
			SystemPromptMode: "complete",
			Settings:         &biz.AgentRuntimeSettings{ToolsProfile: "spirit"},
		},
		"chat_only allowlist": {
			SystemPromptMode: "complete",
			Settings:         &biz.AgentRuntimeSettings{ToolsProfile: "chat_only"},
		},
		"coding complete (Full)": {
			SystemPromptMode: "complete",
			Settings:         &biz.AgentRuntimeSettings{ToolsProfile: "coding"},
		},
		"coding task (KnowledgeOnly)": {
			SystemPromptMode: "task",
			Settings:         &biz.AgentRuntimeSettings{ToolsProfile: "coding"},
		},
		"nil settings": {
			SystemPromptMode: "complete",
		},
	}
	for name, ag := range agents {
		t.Run(name, func(t *testing.T) {
			allowed := allowedSkillToolsForAgent(ag)
			if len(allowed) > 0 {
				hasLoad := false
				for _, st := range allowed {
					if st == trpcllmagent.SkillToolLoad {
						hasLoad = true
					}
				}
				if !hasLoad {
					t.Fatalf("allowlist %v lacks skill_load — would panic with require-loaded gate", allowed)
				}
				return
			}
			profile, _ := skillOptionsForAgent(ag)
			switch profile {
			case trpcllmagent.SkillToolProfileFull, trpcllmagent.SkillToolProfileKnowledgeOnly:
				// Both presets resolve with Load=true (Full additionally has
				// Run/Exec=true) — safe under the require-loaded gate.
			default:
				t.Fatalf("unknown skill tool profile %q — cannot prove Load invariant", profile)
			}
		})
	}
}
