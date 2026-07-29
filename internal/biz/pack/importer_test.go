package pack

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
)

// stubImporterRepo implements ImporterRepo for testing.
type stubImporterRepo struct {
	agents          map[string]biz.Agent
	teams           map[string]biz.Team
	graphs          map[string]*biz.GraphDefinition // name → definition
	createdSettings map[string]biz.AgentRuntimeSettings
	updatedSettings map[string]*biz.AgentRuntimeSettings
}

func newStubImporterRepo() *stubImporterRepo {
	return &stubImporterRepo{
		agents:          make(map[string]biz.Agent),
		teams:           make(map[string]biz.Team),
		graphs:          make(map[string]*biz.GraphDefinition),
		createdSettings: make(map[string]biz.AgentRuntimeSettings),
		updatedSettings: make(map[string]*biz.AgentRuntimeSettings),
	}
}

func (r *stubImporterRepo) CreateOrganizationNode(_ context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error) {
	return node, nil
}
func (r *stubImporterRepo) UpdateOrganizationNode(_ context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error) {
	return node, nil
}
func (r *stubImporterRepo) GetOrganizationNodeByKey(_ context.Context, key string) (biz.OrganizationNode, error) {
	return biz.OrganizationNode{}, shared.ErrNotFound
}
func (r *stubImporterRepo) GetOrganizationNodeByKeyAnyState(_ context.Context, key string) (biz.OrganizationNode, error) {
	return biz.OrganizationNode{}, shared.ErrNotFound
}
func (r *stubImporterRepo) ListOrganizationNodesByParentID(_ context.Context, parentID string) ([]biz.OrganizationNode, error) {
	return nil, nil
}
func (r *stubImporterRepo) ListOrganizationNodesByLevel(_ context.Context, level string) ([]biz.OrganizationNode, error) {
	return nil, nil
}
func (r *stubImporterRepo) ExecInTx(_ context.Context, fn func(context.Context) error) error {
	return fn(context.Background())
}

func (r *stubImporterRepo) GetAgentByAgentKey(_ context.Context, agentKey string) (biz.Agent, error) {
	a, ok := r.agents[agentKey]
	if !ok {
		return biz.Agent{}, shared.ErrNotFound
	}
	return a, nil
}
func (r *stubImporterRepo) CreateAgentAtomic(_ context.Context, a biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error) {
	if a.ID == "" {
		a.ID = "agent-" + a.AgentKey
	}
	r.agents[a.AgentKey] = a
	if settings.AgentID == "" {
		settings.AgentID = a.ID
	}
	r.createdSettings[a.AgentKey] = settings
	return a, nil
}
func (r *stubImporterRepo) UpdateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error) {
	r.agents[a.AgentKey] = a
	r.updatedSettings[a.AgentKey] = settings
	return a, nil
}
func (r *stubImporterRepo) ReplaceAgentPromptFiles(_ context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return files, nil
}

func (r *stubImporterRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	return biz.Team{}, shared.ErrNotFound
}
func (r *stubImporterRepo) GetTeamByKey(_ context.Context, teamKey string) (biz.Team, error) {
	t, ok := r.teams[teamKey]
	if !ok {
		return biz.Team{}, shared.ErrNotFound
	}
	return t, nil
}
func (r *stubImporterRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" {
		t.ID = "team-" + t.TeamKey
	}
	r.teams[t.TeamKey] = t
	return t, nil
}
func (r *stubImporterRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	r.teams[t.TeamKey] = t
	return t, nil
}

func (r *stubImporterRepo) SaveGraphDefinition(_ context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	if def.ID == "" {
		def.ID = "graph-" + def.Name
	}
	r.graphs[def.Name] = def
	return def, nil
}
func (r *stubImporterRepo) GetGraphDefinitionByName(_ context.Context, name string) (*biz.GraphDefinition, error) {
	g, ok := r.graphs[name]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return g, nil
}
func (r *stubImporterRepo) UpdateGraphDefinition(_ context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	r.graphs[def.Name] = def
	return def, nil
}

func TestImport_DefaultKind(t *testing.T) {
	repo := newStubImporterRepo()
	im := NewImporter(repo)

	p := &Pack{
		Agents: []AgentPackSpec{
			{Key: "test-agent", DisplayName: "Test Agent", Provider: "openrouter", Model: "gpt-4"},
		},
		Teams: []TeamPackSpec{
			{Key: "test-team", DisplayName: "Test Team", Mode: "coordinator"},
		},
	}

	result, err := im.Import(context.Background(), p, ConflictOverwrite)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.AgentsCreated != 1 {
		t.Errorf("AgentsCreated = %d, want 1", result.AgentsCreated)
	}
	if result.TeamsCreated != 1 {
		t.Errorf("TeamsCreated = %d, want 1", result.TeamsCreated)
	}

	// Verify agent has default kind "user" (ownership), agent_kind "llm" (technical), and source "imported"
	agent := repo.agents["test-agent"]
	if agent.Kind != "user" {
		t.Errorf("Agent Kind = %q, want %q", agent.Kind, "user")
	}
	if agent.AgentKind != "llm" {
		t.Errorf("Agent AgentKind = %q, want %q", agent.AgentKind, "llm")
	}
	if agent.Source != "imported" {
		t.Errorf("Agent Source = %q, want %q", agent.Source, "imported")
	}

	// Verify team has default source "imported"
	team := repo.teams["test-team"]
	if team.Source != "imported" {
		t.Errorf("Team Source = %q, want %q", team.Source, "imported")
	}
}

func TestImport_WithKindOverride(t *testing.T) {
	repo := newStubImporterRepo()
	im := NewImporter(repo)

	p := &Pack{
		Agents: []AgentPackSpec{
			{Key: "eco-agent", DisplayName: "Eco Agent", Provider: "openrouter", Model: "gpt-4", Kind: "a2a_proxy"},
		},
		Teams: []TeamPackSpec{
			{Key: "eco-team", DisplayName: "Eco Team", Mode: "coordinator"},
		},
	}

	result, err := im.Import(context.Background(), p, ConflictOverwrite, WithKindOverride("ecosystem_preset"))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.AgentsCreated != 1 {
		t.Errorf("AgentsCreated = %d, want 1", result.AgentsCreated)
	}
	if result.TeamsCreated != 1 {
		t.Errorf("TeamsCreated = %d, want 1", result.TeamsCreated)
	}

	// Verify agent kind overridden to "ecosystem_preset" (ownership), source "imported"
	agent := repo.agents["eco-agent"]
	if agent.Kind != "ecosystem_preset" {
		t.Errorf("Agent Kind = %q, want %q", agent.Kind, "ecosystem_preset")
	}
	if agent.Source != "imported" {
		t.Errorf("Agent Source = %q, want %q", agent.Source, "imported")
	}

	// Verify team kind overridden to "ecosystem_preset" (ownership), source stays "imported"
	team := repo.teams["eco-team"]
	if team.Kind != "ecosystem_preset" {
		t.Errorf("Team Kind = %q, want %q", team.Kind, "ecosystem_preset")
	}
	if team.Source != "imported" {
		t.Errorf("Team Source = %q, want %q", team.Source, "imported")
	}
}

// TS9-BUG-3 (root cause): specs without a runtime block must still persist
// platform-default runtime settings on the create path. The zero-value struct
// previously written here disabled ToolsEnabled/MemoryEnabled for every
// pack-imported agent (256 rows in production DB had tools_enabled=false).
func TestImportAgent_RuntimeNil_WritesPlatformDefaults(t *testing.T) {
	repo := newStubImporterRepo()
	im := NewImporter(repo)

	p := &Pack{
		Agents: []AgentPackSpec{
			{Key: "plain-agent", DisplayName: "Plain", Provider: "deepseek", Model: "deepseek-chat"},
		},
	}
	if _, err := im.Import(context.Background(), p, ConflictOverwrite); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	s, ok := repo.createdSettings["plain-agent"]
	if !ok {
		t.Fatal("create path must write runtime settings")
	}
	if !s.ToolsEnabled {
		t.Error("ToolsEnabled must default to true for pack-imported agents")
	}
	if !s.MemoryEnabled {
		t.Error("MemoryEnabled must default to true for pack-imported agents")
	}
	if s.ToolsProfile != "coding" {
		t.Errorf("ToolsProfile = %q, want default %q", s.ToolsProfile, "coding")
	}
}

// TS9-BUG-3: tools_allow / tools_deny declared at spec top level (without a
// runtime block) must reach the persisted settings. The early return in
// buildRuntimeSettings previously dropped them (production DB showed
// tools_deny_json='[]' despite the pack YAML declaring a deny list).
func TestImportAgent_ToolsPolicyWithoutRuntime_Applied(t *testing.T) {
	repo := newStubImporterRepo()
	im := NewImporter(repo)

	p := &Pack{
		Agents: []AgentPackSpec{
			{
				Key: "ops-agent", DisplayName: "Ops", Provider: "deepseek", Model: "deepseek-chat",
				ToolsAllow: []string{"shell_exec"},
				ToolsDeny:  []string{"workspace_exec"},
			},
		},
	}
	if _, err := im.Import(context.Background(), p, ConflictOverwrite); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	s := repo.createdSettings["ops-agent"]
	if s.ToolsAllowJSON != `["shell_exec"]` {
		t.Errorf("ToolsAllowJSON = %q, want %q", s.ToolsAllowJSON, `["shell_exec"]`)
	}
	if s.ToolsDenyJSON != `["workspace_exec"]` {
		t.Errorf("ToolsDenyJSON = %q, want %q", s.ToolsDenyJSON, `["workspace_exec"]`)
	}
	if !s.ToolsEnabled {
		t.Error("ToolsEnabled must stay true (platform default)")
	}
}

// TS9-BUG-3: an explicit tools_profile in the spec overrides the default, but
// an empty one must not clobber it (unconditional assignment previously wrote
// "" which silently fell back at runtime while persisting misleading data).
func TestBuildRuntimeSettings_ProfileOverlay(t *testing.T) {
	im := NewImporter(newStubImporterRepo())

	withProfile := im.buildRuntimeSettings("", AgentPackSpec{ToolsProfile: "read_only"})
	if withProfile.ToolsProfile != "read_only" {
		t.Errorf("ToolsProfile = %q, want %q", withProfile.ToolsProfile, "read_only")
	}

	withoutProfile := im.buildRuntimeSettings("", AgentPackSpec{})
	if withoutProfile.ToolsProfile != "coding" {
		t.Errorf("ToolsProfile = %q, want default %q", withoutProfile.ToolsProfile, "coding")
	}
}

// TS9-BUG-3: on ConflictOverwrite, a spec carrying any runtime-relevant
// declaration (here: tools_deny only, no runtime block) must pass non-nil
// settings to UpdateAgentAtomic so the pack re-asserts its policy instead of
// preserving a stale zero-value row.
func TestImportAgent_Overwrite_ToolsPolicyReasserted(t *testing.T) {
	repo := newStubImporterRepo()
	repo.agents["existing-agent"] = biz.Agent{ID: "agent-existing", AgentKey: "existing-agent"}
	im := NewImporter(repo)

	p := &Pack{
		Agents: []AgentPackSpec{
			{
				Key: "existing-agent", DisplayName: "Existing", Provider: "deepseek", Model: "deepseek-chat",
				ToolsDeny: []string{"workspace_exec"},
			},
		},
	}
	if _, err := im.Import(context.Background(), p, ConflictOverwrite); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	sp, ok := repo.updatedSettings["existing-agent"]
	if !ok || sp == nil {
		t.Fatal("overwrite path must pass non-nil settings when spec declares tool policy")
	}
	if sp.ToolsDenyJSON != `["workspace_exec"]` {
		t.Errorf("ToolsDenyJSON = %q, want %q", sp.ToolsDenyJSON, `["workspace_exec"]`)
	}
	if !sp.ToolsEnabled {
		t.Error("ToolsEnabled must be rebuilt from platform defaults, not zero values")
	}
}
