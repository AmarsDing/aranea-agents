package biz

import "context"

// TeamCompiler is the biz-level port for compiling a Team Definition into a
// GraphBuildConfig. This decouples the compilation logic from the team runtime
// and allows consumers (service, graph adapter) to compile without importing
// internal/team.
//
// Implementations live in internal/team (CompileToGraphBuildConfig).
// Wire binding happens in internal/service.
type TeamCompiler interface {
	// Compile compiles a team definition identified by teamID into a GraphBuildConfig.
	Compile(ctx context.Context, teamID string) (GraphBuildConfig, error)

	// CompileFromDefinition compiles from an already-loaded Definition.
	CompileFromDefinition(def TeamDefinition, agentKey func(agentID string) string) (GraphBuildConfig, error)
}

// TeamDefinition is the biz-level representation of a team definition,
// decoupled from the internal/team.Definition concrete type.
type TeamDefinition struct {
	ID            string
	Name          string
	Mode          string
	Members       []TeamMember
	FailurePolicy FailurePolicyConfig
	// RawDefinitionJSON carries the orchestration spec JSON for embedded graph compilation.
	RawDefinitionJSON string
}

// TeamMember is a biz-level team member.
type TeamMember struct {
	AgentID    string
	AgentKey   string
	Role       string
	TaskPrompt string
	Enabled    bool
}
