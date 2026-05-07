package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

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
}

// BuildLLMAgent constructs an ADK LLM agent from a hydrated biz catalog agent.
func BuildLLMAgent(ctx context.Context, ag biz.Agent, deps BuilderDeps) (agent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, fmt.Errorf("agent: agent_key required")
	}
	prov := firstNonEmptyString(deps.Provider, ag.Provider)
	mod := firstNonEmptyString(deps.Model, ag.Model)
	if prov == "" || mod == "" {
		return nil, fmt.Errorf("agent: provider and model required")
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
	promptDeps := Deps{Agents: deps.Agents, AgentUC: deps.AgentUC, ToolsCatalog: deps.ToolsCatalog}
	if cue := RuntimeCapabilityCue(ctx, promptDeps, ag); cue != "" {
		sys = sys + "\n\n" + cue
	}

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

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
