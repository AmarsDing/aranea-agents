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
}

func newStubImporterRepo() *stubImporterRepo {
	return &stubImporterRepo{
		agents: make(map[string]biz.Agent),
		teams:  make(map[string]biz.Team),
	}
}

func (r *stubImporterRepo) CreateTaxonomyNode(_ context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	return node, nil
}
func (r *stubImporterRepo) UpdateTaxonomyNode(_ context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	return node, nil
}
func (r *stubImporterRepo) GetTaxonomyNodeByKey(_ context.Context, key string) (biz.TaxonomyNode, error) {
	return biz.TaxonomyNode{}, shared.ErrNotFound
}
func (r *stubImporterRepo) ListTaxonomyNodesByParentID(_ context.Context, parentID string) ([]biz.TaxonomyNode, error) {
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
func (r *stubImporterRepo) UpdateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
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

	// Verify agent has default kind "llm" and source "imported"
	agent := repo.agents["test-agent"]
	if agent.Kind != "llm" {
		t.Errorf("Agent Kind = %q, want %q", agent.Kind, "llm")
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

	// Verify agent kind overridden to "ecosystem_preset"
	agent := repo.agents["eco-agent"]
	if agent.Kind != "ecosystem_preset" {
		t.Errorf("Agent Kind = %q, want %q", agent.Kind, "ecosystem_preset")
	}
	if agent.Source != "ecosystem_preset" {
		t.Errorf("Agent Source = %q, want %q", agent.Source, "ecosystem_preset")
	}

	// Verify team source overridden to "ecosystem_preset"
	team := repo.teams["eco-team"]
	if team.Source != "ecosystem_preset" {
		t.Errorf("Team Source = %q, want %q", team.Source, "ecosystem_preset")
	}
}
