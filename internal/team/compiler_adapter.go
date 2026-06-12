package team

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TeamCompilerAdapter adapts the team package's compile functions to the
// biz.TeamCompiler port interface, keeping the service layer free of
// direct internal/team imports.
type TeamCompilerAdapter struct {
	channels TeamChannelReader
	agentKey func(ctx context.Context) func(agentID string) string
	lg       loggateway.Logger
}

// TeamChannelReader is the minimal interface needed by TeamCompilerAdapter
// to look up team definitions. Satisfied by *biz.ChannelUsecase.
type TeamChannelReader interface {
	GetTeamByID(ctx context.Context, id string) (biz.Team, error)
}

// NewTeamCompilerAdapter creates a TeamCompilerAdapter.
// agentKeyFn resolves a function that maps agentID → agentKey from context.
// The signature uses only basic Go types so callers (Wire) need not import
// internal/team's CompileAgentKey type.
func NewTeamCompilerAdapter(channels TeamChannelReader, agentKeyFn func(ctx context.Context) func(agentID string) string, lg loggateway.Logger) *TeamCompilerAdapter {
	return &TeamCompilerAdapter{channels: channels, agentKey: agentKeyFn, lg: lg}
}

// Compile implements biz.TeamCompiler.Compile.
func (a *TeamCompilerAdapter) Compile(ctx context.Context, teamID string) (biz.GraphBuildConfig, error) {
	teamRow, err := a.channels.GetTeamByID(ctx, teamID)
	if err != nil {
		return biz.GraphBuildConfig{}, err
	}
	def, perr := ParseDefinition(teamRow.DefinitionJSON)
	if perr != nil {
		return biz.GraphBuildConfig{}, perr
	}
	agentKey := CompileAgentKey(a.agentKey(ctx))
	cfg, _, cerr := CompileToGraphBuildConfigFromJSON(def, teamRow.DefinitionJSON, agentKey, a.lg)
	if cerr != nil {
		return biz.GraphBuildConfig{}, cerr
	}
	return cfg, nil
}

// CompileFromDefinition implements biz.TeamCompiler.CompileFromDefinition.
func (a *TeamCompilerAdapter) CompileFromDefinition(def biz.TeamDefinition, agentKey func(agentID string) string) (biz.GraphBuildConfig, error) {
	parsedDef := bizToTeamDefinition(def)
	cfg, _, err := CompileToGraphBuildConfigFromJSON(parsedDef, def.RawDefinitionJSON, CompileAgentKey(agentKey), a.lg)
	return cfg, err
}

func bizToTeamDefinition(def biz.TeamDefinition) Definition {
	parsedDef := Definition{
		Mode: def.Mode,
	}
	for _, m := range def.Members {
		enabled := m.Enabled
		parsedDef.Members = append(parsedDef.Members, MemberDef{
			AgentID:    m.AgentID,
			Role:       m.Role,
			Enabled:    &enabled,
			TaskPrompt: m.TaskPrompt,
		})
	}
	if def.FailurePolicy.Default != "" || def.FailurePolicy.Retry.MaxAttempts > 0 {
		parsedDef.FailurePolicy = &FailurePolicy{
			Default: def.FailurePolicy.Default,
			Retry: RetryPolicy{
				MaxAttempts: def.FailurePolicy.Retry.MaxAttempts,
			},
		}
	}
	return parsedDef
}

// Verify interface compliance.
var _ biz.TeamCompiler = (*TeamCompilerAdapter)(nil)
