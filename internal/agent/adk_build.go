package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/tool"
)

// BuilderDeps wires catalog resolution and optional services for [BuildLLMAgent].
type BuilderDeps struct {
	Catalog *biz.LlmProviderModelUsecase
	AgentUC *biz.AgentUsecase
	Agents  biz.AgentRepository
	// ToolsCatalog is the platform tool table (biz.ToolRepo). Optional; required only when AgentUC is nil and callers still need effective tools.
	ToolsCatalog biz.ToolRepo
	RT           *provider.RoundTrip
	Memory       memory.Service
	Artifacts    artifact.Service
	Tools        []tool.Tool
	Toolsets     []tool.Toolset
	// Provider / model override from session or request (non-empty wins over ag.Provider / ag.Model).
	Provider string
	Model    string
	// SQLiteSessionMemory is true when the Runner uses SessionMemoryStore-backed ADK memory (durable entities).
	SQLiteSessionMemory bool
	// TeamOrchestrationMode / TeamMemberRole / TeamMemberDisplayName are optional; native team sets them per member.
	TeamOrchestrationMode string
	TeamMemberRole        string
	TeamMemberDisplayName string
}

// appendTeamOrchestrationCue adds team workflow context to the system instruction when any field is set.
func appendTeamOrchestrationCue(sys, mode, role, rosterName string) string {
	mode = strings.TrimSpace(mode)
	role = strings.TrimSpace(role)
	rosterName = strings.TrimSpace(rosterName)
	if mode == "" && role == "" && rosterName == "" {
		return sys
	}
	var b strings.Builder
	b.WriteString("\n\n## Team orchestration (system)\n")
	if mode != "" {
		b.WriteString("- Team workflow mode: ")
		b.WriteString(mode)
		b.WriteByte('\n')
	}
	if role != "" {
		b.WriteString("- Your role in this team workflow: ")
		b.WriteString(role)
		b.WriteByte('\n')
	}
	if rosterName != "" {
		b.WriteString("- Team roster label (coordination with other members): ")
		b.WriteString(rosterName)
		b.WriteByte('\n')
	}
	if strings.EqualFold(strings.TrimSpace(mode), "parallel") {
		b.WriteString("- Parallel workflow: split read-only investigation across members when tasks are independent; avoid two members editing the same file in the same turn. Record searched paths in shared working_memory when available.\n")
	}
	b.WriteString("- Align with this role and workflow mode together with your primary instructions above.\n")
	return sys + b.String()
}

// BuildLLMAgent constructs an ADK LLM agent from a hydrated biz catalog agent.
func BuildLLMAgent(ctx context.Context, ag biz.Agent, deps BuilderDeps) (agent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, kerrors.BadRequest("AGENT", "agent_key required")
	}
	prov := strutil.FirstNonEmpty(deps.Provider, ag.Provider)
	mod := strutil.FirstNonEmpty(deps.Model, ag.Model)
	if prov == "" || mod == "" {
		return nil, kerrors.BadRequest("AGENT", "provider and model required")
	}
	m, err := provider.ModelForProviderModel(ctx, deps.Catalog, deps.RT, prov, mod)
	if err != nil {
		return nil, err
	}

	files := ag.Files
	if len(files) == 0 && deps.Agents != nil {
		files, err = deps.Agents.ListAgentPromptFiles(ctx, ag.ID)
		if err != nil {
			return nil, err
		}
	}
	sys := BuildSystemPrompt(ag, files)
	promptDeps := Deps{
		Agents: deps.Agents, AgentUC: deps.AgentUC, ToolsCatalog: deps.ToolsCatalog,
		SQLiteSessionMemory: deps.SQLiteSessionMemory,
	}
	if cue := RuntimeCapabilityCue(ctx, promptDeps, ag); cue != "" {
		sys = sys + "\n\n" + cue
	}
	sys = appendTeamOrchestrationCue(sys, deps.TeamOrchestrationMode, deps.TeamMemberRole, deps.TeamMemberDisplayName)

	incl := llmagent.IncludeContentsDefault
	if ag.Settings != nil && strings.EqualFold(strings.TrimSpace(ag.Settings.L0SnapshotMode), "none") {
		incl = llmagent.IncludeContentsNone
	}

	disallowTransfer := true
	if ag.Settings != nil && ag.Settings.SubagentsEnabled {
		disallowTransfer = false
	}

	cfg := llmagent.Config{
		Name:                     strings.TrimSpace(ag.AgentKey),
		Description:              strings.TrimSpace(ag.DisplayName),
		Model:                    m,
		Instruction:              sys,
		IncludeContents:          incl,
		DisallowTransferToParent: disallowTransfer,
		DisallowTransferToPeers:  disallowTransfer,
		Tools:                    deps.Tools,
		Toolsets:                 deps.Toolsets,
	}
	return llmagent.New(cfg)
}
