package pack

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
)

// stubImporterRepo implements ImporterRepo for testing.
type stubImporterRepo struct {
	agents map[string]biz.Agent
	teams  map[string]biz.Team
	graphs map[string]*biz.GraphDefinition // name → definition
}

func newStubImporterRepo() *stubImporterRepo {
	return &stubImporterRepo{
		agents: make(map[string]biz.Agent),
		teams:  make(map[string]biz.Team),
		graphs: make(map[string]*biz.GraphDefinition),
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

func (r *stubImporterRepo) GetAgentByAgentKey(_ context.Context, agentKey string) (biz.Agent, error) {
	a, ok := r.agents[agentKey]
	if !ok {
		return biz.Agent{}, shared.ErrNotFound
	}
	return a, nil
}
func (r *stubImporterRepo) CreateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
	if a.ID == "" {
		a.ID = "agent-" + a.AgentKey
	}
	r.agents[a.AgentKey] = a
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
	return a, nil
}
func (r *stubImporterRepo) UpdateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
	r.agents[a.AgentKey] = a
	return a, nil
}
func (r *stubImporterRepo) UpdateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ *biz.AgentRuntimeSettings) (biz.Agent, error) {
	r.agents[a.AgentKey] = a
	return a, nil
}
func (r *stubImporterRepo) DeleteAgent(_ context.Context, id string) error {
	return nil
}
func (r *stubImporterRepo) GetAgentRuntimeSettings(_ context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, shared.ErrNotFound
}
func (r *stubImporterRepo) UpsertAgentRuntimeSettings(_ context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return v, nil
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

	// Verify team kind overridden to "ecosystem_preset", source stays "imported"
	team := repo.teams["eco-team"]
	if team.Kind != "ecosystem_preset" {
		t.Errorf("Team Kind = %q, want %q", team.Kind, "ecosystem_preset")
	}
	if team.Source != "imported" {
		t.Errorf("Team Source = %q, want %q", team.Source, "imported")
	}
}
