package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"strings"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// plannerKind extracts the PlannerKind from agent settings, defaulting to "".
func plannerKind(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	return ag.Settings.PlannerKind
}

func plannerConfigJSON(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	return ag.Settings.PlannerConfigJSON
}

// ResolveModelForRunOption resolves a provider/model pair into a trpcmodel.Model
// suitable for use as an agent.WithModel() RunOption. This enables per-request
// model overrides without baking Provider/Model into the agent build cache key.
func ResolveModelForRunOption(ctx context.Context, deps TRPCBuilderDeps, prov, mod string, lg loggateway.Logger) (trpcmodel.Model, error) {
	return provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
}

// ResolveAgentBuildModel resolves the same model that the cached agent build
// uses (agent's own provider/model, falling back to the system default when
// unset or unavailable). Business-layer wrappers around the built agent (e.g.
// the graph agent-node summary fallback) use it so auxiliary LLM calls run on
// the same model as the agent itself.
func ResolveAgentBuildModel(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, lg loggateway.Logger) (trpcmodel.Model, error) {
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	if prov == "" || mod == "" {
		prov, mod = resolveBuildDefaultModel(ctx, deps, lg)
	}
	if prov == "" || mod == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent provider and model required (no system default available)")
	}
	m, err := provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, prov, mod, lg)
	if err != nil && errors.Is(err, biz.ErrProviderModelNotFound) {
		if fbProv, fbMod := resolveBuildDefaultModel(ctx, deps, lg); fbProv != "" && fbMod != "" {
			m, err = provider.TRPCModelForProviderModel(ctx, deps.ModelCatalog, deps.RT, fbProv, fbMod, lg)
		}
	}
	return m, err
}

// isToolPipeEligible determines whether a tool should receive the result_filter
// capability from ToolPipe Extension. Only tools that produce large or structured
// output benefit from filtering; framework-managed tools (todo, memory, skill_*)
// and control-flow tools (transfer_to_agent, await_user_reply) are excluded.
// The framework's own isFrameworkTool check provides additional protection.
func isToolPipeEligible(t trpctool.Tool) bool {
	name := t.Declaration().Name

	if strings.HasPrefix(name, "mcp_") {
		return true
	}

	switch name {
	case "read_file", "execute_command", "web_fetch", "list_directory",
		"search_files", "search_code":
		return true
	}
	return false
}

// hasToolByName reports whether the tool slice contains a tool whose
// Declaration().Name matches the given name. Returns false for empty slices
// or nil declarations.
func hasToolByName(tools []trpctool.Tool, name string) bool {
	for _, t := range tools {
		if decl := t.Declaration(); decl != nil && decl.Name == name {
			return true
		}
	}
	return false
}

// oversizedToolResultMaxTokens caps a single tool result inside an in-flight
// request. Spirit/chat turns should not keep 8k page dumps across the tool
// loop; coding keeps the larger window for file reads.
func oversizedToolResultMaxTokens(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "coding", "full":
		return 8192
	default:
		return 2048
	}
}
