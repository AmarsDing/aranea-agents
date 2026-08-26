package configgraph

import (
	"context"

	"aranea-agents/internal/biz"
)

// SourceRepo reads the 12 config-asset source tables (plus the two agent-side
// policy tables) for full and incremental graph builds. Implemented with raw
// SQL in internal/data/configgraph; rows are plain DTOs so extractors stay
// pure functions of their inputs (table-driven tests need no database).
//
// Rows include soft-deleted records (DeletedAt != "") so the graph can mark
// nodes status=deleted; extractors skip out-edges for deleted rows.
//
// Stability: Evolving
type SourceRepo interface {
	ListAgents(ctx context.Context) ([]AgentRow, error)
	ListTeams(ctx context.Context) ([]TeamRow, error)
	ListSkills(ctx context.Context) ([]SkillRow, error)
	ListTools(ctx context.Context) ([]ToolRow, error)
	ListPromptFiles(ctx context.Context) ([]PromptFileRow, error)
	ListCronTasks(ctx context.Context) ([]CronTaskRow, error)
	ListChannels(ctx context.Context) ([]ChannelRow, error)
	ListOrganizations(ctx context.Context) ([]OrganizationRow, error)
	ListGraphs(ctx context.Context) ([]GraphRow, error)
	ListKnowledgeCollections(ctx context.Context) ([]KnowledgeCollectionRow, error)
	ListMCPServers(ctx context.Context) ([]MCPServerRow, error)
	ListHooks(ctx context.Context) ([]HookRow, error)
	// ListToolOverrides returns tool_agent_overrides rows with deleted_at=''
	// only (a soft-deleted override is no reference at all).
	ListToolOverrides(ctx context.Context) ([]ToolOverrideRow, error)
	// ListAgentToolPolicies returns agent_runtime_settings tool/skill policy
	// columns keyed per agent (allows_skill / enables_mcp sources).
	ListAgentToolPolicies(ctx context.Context) ([]AgentToolPolicyRow, error)
}

// EffectiveToolsProvider resolves the runtime effective-tool set of one agent
// (satisfied by *biz.AgentUsecase). granted_tool edges are extracted
// exclusively through it so the graph matches the runtime gate exactly
// (design R1: same process, same DB catalog, same runtime gates).
type EffectiveToolsProvider interface {
	GetEffectiveTools(ctx context.Context, agentID string) (biz.AgentEffectiveTools, error)
}

// AgentRow mirrors agents columns consumed by the graph.
type AgentRow struct {
	ID           string
	AgentKey     string
	DisplayName  string
	Status       string
	Kind         string
	AgentVariant string
	PositionID   string
	PositionKey  string
	WorkspaceID  string
	DeletedAt    string
}

// TeamRow mirrors teams columns consumed by the graph.
type TeamRow struct {
	ID                 string
	TeamKey            string
	DisplayName        string
	Status             string
	IsDefault          bool
	Kind               string
	Topology           string
	DefinitionJSON     string
	DepartmentID       string
	DeptLeadAgentID    string
	CrossDeptMemberIDs string
	LinkedGraphID      string
	WorkspaceID        string
	DeletedAt          string
}

// SkillRow mirrors skill columns consumed by the graph.
type SkillRow struct {
	ID          string
	SkillKey    string
	Name        string
	Status      string
	Enabled     bool
	ParentID    string
	AgentID     string
	WorkspaceID string
	DeletedAt   string
}

// ToolRow mirrors tools columns consumed by the graph.
type ToolRow struct {
	ID                   string
	ToolKey              string
	DisplayName          string
	Category             string
	RiskLevel            string
	Enabled              bool
	RequiresConfirmation bool
	WorkspaceID          string
	DeletedAt            string
}

// PromptFileRow mirrors agent_prompt_files (+ agents.agent_key via join for
// the human-readable node_key `{agent_key}/{file_name}`).
type PromptFileRow struct {
	ID        string
	AgentID   string
	AgentKey  string
	FileName  string
	Body      string
	SortOrder int
}

// CronTaskRow mirrors cron_task columns consumed by the graph.
type CronTaskRow struct {
	ID          string
	TaskKey     string
	Name        string
	Status      string
	Enabled     bool
	AgentID     string
	ConfigJSON  string
	WorkspaceID string
	DeletedAt   string
}

// ChannelRow mirrors channel columns consumed by the graph.
type ChannelRow struct {
	ID          string
	ChannelKey  string
	Name        string
	Status      string
	Enabled     bool
	ConfigJSON  string
	WorkspaceID string
	DeletedAt   string
}

// OrganizationRow mirrors organizations columns consumed by the graph.
type OrganizationRow struct {
	ID              string
	OrgKey          string
	Name            string
	Status          string
	ParentID        string
	Level           string
	DeptLeadAgentID string
	WorkspaceID     string
	DeletedAt       string
}

// GraphRow mirrors graph_definitions columns consumed by the graph.
type GraphRow struct {
	ID          string
	Name        string
	NodesJSON   string
	TeamID      string
	IsTemplate  bool
	WorkspaceID string
}

// KnowledgeCollectionRow mirrors knowledge_collections columns consumed by
// the graph (workspace column is named `workspace` in that table).
type KnowledgeCollectionRow struct {
	ID        string
	Name      string
	Status    string
	Workspace string
}

// MCPServerRow mirrors mcp_server columns consumed by the graph.
type MCPServerRow struct {
	ID          string
	ServerKey   string
	Name        string
	Status      string
	Enabled     bool
	WorkspaceID string
	DeletedAt   string
}

// HookRow mirrors hooks columns consumed by the graph.
type HookRow struct {
	ID         string
	HookKey    string
	Name       string
	Status     string
	Enabled    bool
	ConfigJSON string
	DeletedAt  string
}

// ToolOverrideRow mirrors tool_agent_overrides columns consumed by the graph.
type ToolOverrideRow struct {
	ID        string
	ToolID    string
	ToolKey   string
	AgentID   string
	Mode      string
	Enabled   bool
	DeletedAt string
}

// AgentToolPolicyRow mirrors agent_runtime_settings policy columns consumed
// by the graph (allows_skill / enables_mcp edge sources).
type AgentToolPolicyRow struct {
	AgentID          string
	ToolsAllowJSON   string
	ToolsDenyJSON    string
	SkillRuntimeJSON string
}
