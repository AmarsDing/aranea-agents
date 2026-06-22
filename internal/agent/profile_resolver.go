package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/openclaw/runtimeprofile"
)

// ProfileResolver resolves the active runtime profile for an agent and
// converts it to framework RunOptions that override agent defaults at
// turn execution time.
//
// This is the bridge between the project's biz.RuntimeProfile (DB-backed,
// CRUD-managed) and the framework's runtimeprofile.Profile (in-memory,
// applied per-request via RunOption).
type ProfileResolver struct {
	uc *biz.RuntimeProfileUsecase
	lg loggateway.Logger
}

func NewProfileResolver(uc *biz.RuntimeProfileUsecase, lg loggateway.Logger) *ProfileResolver {
	return &ProfileResolver{uc: uc, lg: lg}
}

// ResolveRunOptions resolves the active runtime profile for the given agent
// and returns framework RunOptions. Returns nil (no error) when no profile
// is configured — callers treat nil as "use agent defaults".
func (r *ProfileResolver) ResolveRunOptions(ctx context.Context, agentID string) ([]trpcagent.RunOption, error) {
	if r == nil || r.uc == nil {
		return nil, nil
	}
	prof, err := r.uc.ResolveForAgent(ctx, agentID)
	if err != nil {
		r.lg.Warn("runtime profile resolve failed, using agent defaults",
			loggateway.StepID("agent.profile_resolve_fail"),
			loggateway.Str("agent_id", agentID),
			loggateway.Err(err))
		return nil, nil // degrade gracefully
	}
	if prof == nil {
		return nil, nil
	}
	fwProfile := bizToFrameworkProfile(*prof)
	opts := runtimeprofile.RunOptions(fwProfile)
	if len(opts) > 0 {
		r.lg.Info("runtime profile applied",
			loggateway.StepID("agent.profile_applied"),
			loggateway.Str("agent_id", agentID),
			loggateway.Str("profile_id", prof.ID),
			loggateway.Str("profile_name", prof.Name),
			loggateway.Int("run_option_count", len(opts)))
	}
	return opts, nil
}

// bizToFrameworkProfile converts a biz.RuntimeProfile to the framework's
// runtimeprofile.Profile. Only non-empty fields are mapped; empty fields
// default to the framework's zero values (no override).
func bizToFrameworkProfile(p biz.RuntimeProfile) runtimeprofile.Profile {
	fw := runtimeprofile.Profile{
		ID:      p.ID,
		Version: strings.TrimSpace(p.Version),
		Prompt: runtimeprofile.Prompt{
			Instruction:  p.PromptConfig.Instruction,
			SystemPrompt: p.PromptConfig.SystemPrompt,
		},
		Tools: runtimeprofile.ToolPolicy{
			Include:          p.ToolPolicy.Include,
			Exclude:          p.ToolPolicy.Exclude,
			ExecutionInclude: p.ToolPolicy.ExecutionInclude,
			ExecutionExclude: p.ToolPolicy.ExecutionExclude,
			ToolSets:         p.ToolPolicy.ToolSets,
			CredentialRefs:   p.ToolPolicy.CredentialRefs,
		},
		Skills: runtimeprofile.SkillPolicy{
			Include: p.SkillPolicy.Include,
			Exclude: p.SkillPolicy.Exclude,
			Roots:   p.SkillPolicy.Roots,
		},
		Knowledge: runtimeprofile.KnowledgePolicy{
			Indexes: p.KnowledgePolicy.Indexes,
			Filter:  p.KnowledgePolicy.Filter,
		},
		Workspace: runtimeprofile.WorkspacePolicy{
			Workdir:      p.WorkspacePolicy.Workdir,
			AllowedRoots: p.WorkspacePolicy.AllowedRoots,
		},
		Credentials: runtimeprofile.CredentialPolicy{
			AllowedRefs: p.CredentialPolicy.AllowedRefs,
		},
		Isolation: runtimeprofile.IsolationPolicy{
			Mode:         runtimeprofile.IsolationMode(p.IsolationPolicy.Mode),
			AgentCache:   p.IsolationPolicy.AgentCache,
			ToolSetCache: p.IsolationPolicy.ToolSetCache,
			ServiceMode:  p.IsolationPolicy.ServiceMode,
		},
		ExtraModel: p.ExtraModelConfig,
	}
	return fw
}
