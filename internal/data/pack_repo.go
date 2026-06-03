package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/tools"
)

// PackRepoAdapter adapts existing biz repos to satisfy pack engine interfaces.
type PackRepoAdapter struct {
	agents      biz.AgentRepository
	teams       biz.TeamRepository
	taxonomy    biz.TaxonomyRepo
	graphs      biz.GraphRepo
	skillLookup biz.SkillLookupReader
}

var _ pack.ExporterRepo = (*PackRepoAdapter)(nil)
var _ pack.ImporterRepo = (*PackRepoAdapter)(nil)
var _ pack.ValidatorRepo = (*PackRepoAdapter)(nil)

// NewPackRepoAdapter creates a composite adapter from existing repos.
func NewPackRepoAdapter(
	agents biz.AgentRepository,
	teams biz.TeamRepository,
	taxonomy biz.TaxonomyRepo,
	graphs biz.GraphRepo,
	skillLookup biz.SkillLookupReader,
) *PackRepoAdapter {
	return &PackRepoAdapter{
		agents:      agents,
		teams:       teams,
		taxonomy:    taxonomy,
		graphs:      graphs,
		skillLookup: skillLookup,
	}
}

// --- ExporterRepo ---

func (a *PackRepoAdapter) GetAgent(ctx context.Context, id string) (biz.Agent, error) {
	return a.agents.GetAgentByID(ctx, id)
}

func (a *PackRepoAdapter) GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error) {
	return a.agents.GetAgentByAgentKey(ctx, agentKey)
}

func (a *PackRepoAdapter) SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
	return a.agents.SearchAgents(ctx, q)
}

func (a *PackRepoAdapter) GetTeam(ctx context.Context, id string) (biz.Team, error) {
	return a.teams.GetTeamByID(ctx, id)
}

func (a *PackRepoAdapter) ListTeams(ctx context.Context) ([]biz.Team, error) {
	return a.teams.ListTeams(ctx)
}

func (a *PackRepoAdapter) GetTaxonomyNode(ctx context.Context, id string) (biz.TaxonomyNode, error) {
	return a.taxonomy.GetTaxonomyNode(ctx, id)
}

func (a *PackRepoAdapter) GetTaxonomyAncestors(ctx context.Context, positionID string) (biz.TaxonomyAncestors, error) {
	pos, err := a.taxonomy.GetTaxonomyNode(ctx, positionID)
	if err != nil {
		return biz.TaxonomyAncestors{}, err
	}
	var dept biz.TaxonomyNode
	if pos.ParentID != "" {
		dept, err = a.taxonomy.GetTaxonomyNode(ctx, pos.ParentID)
		if err != nil {
			return biz.TaxonomyAncestors{}, err
		}
	}
	var ind biz.TaxonomyNode
	if dept.ParentID != "" {
		ind, err = a.taxonomy.GetTaxonomyNode(ctx, dept.ParentID)
		if err != nil {
			return biz.TaxonomyAncestors{}, err
		}
	}
	return biz.TaxonomyAncestors{
		Industry:   ind,
		Department: dept,
		Position:   pos,
	}, nil
}

func (a *PackRepoAdapter) ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]biz.TaxonomyNode, error) {
	return a.taxonomy.ListTaxonomyNodesByParentID(ctx, parentID)
}

func (a *PackRepoAdapter) ListTaxonomyNodesByLevel(ctx context.Context, level string) ([]biz.TaxonomyNode, error) {
	return a.taxonomy.ListTaxonomyNodesByLevel(ctx, level)
}

func (a *PackRepoAdapter) GetGraph(ctx context.Context, id string) (*biz.GraphDefinition, error) {
	return a.graphs.GetDefinition(ctx, id)
}

// --- ImporterRepo ---

func (a *PackRepoAdapter) CreateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	return a.taxonomy.CreateTaxonomyNode(ctx, node)
}

func (a *PackRepoAdapter) UpdateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	return a.taxonomy.UpdateTaxonomyNode(ctx, node)
}

func (a *PackRepoAdapter) GetTaxonomyNodeByKey(ctx context.Context, key string) (biz.TaxonomyNode, error) {
	return a.taxonomy.GetTaxonomyNodeByKey(ctx, key)
}

func (a *PackRepoAdapter) CreateAgent(ctx context.Context, agent biz.Agent) (biz.Agent, error) {
	return a.agents.CreateAgent(ctx, agent)
}

func (a *PackRepoAdapter) UpdateAgent(ctx context.Context, agent biz.Agent) (biz.Agent, error) {
	return a.agents.UpdateAgent(ctx, agent)
}

func (a *PackRepoAdapter) DeleteAgent(ctx context.Context, id string) error {
	return a.agents.DeleteAgent(ctx, id)
}

func (a *PackRepoAdapter) GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	return a.agents.GetAgentRuntimeSettings(ctx, agentID)
}

func (a *PackRepoAdapter) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return a.agents.UpsertAgentRuntimeSettings(ctx, v)
}

func (a *PackRepoAdapter) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return a.agents.ReplaceAgentPromptFiles(ctx, agentID, files)
}

func (a *PackRepoAdapter) GetTeamByID(ctx context.Context, id string) (biz.Team, error) {
	return a.teams.GetTeamByID(ctx, id)
}

func (a *PackRepoAdapter) GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error) {
	teams, err := a.teams.ListTeams(ctx)
	if err != nil {
		return biz.Team{}, err
	}
	for _, t := range teams {
		if t.TeamKey == teamKey {
			return t, nil
		}
	}
	return biz.Team{}, fmt.Errorf("team with key %q not found", teamKey)
}

func (a *PackRepoAdapter) CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	return a.teams.CreateTeam(ctx, t)
}

func (a *PackRepoAdapter) UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	return a.teams.UpdateTeam(ctx, t)
}

func (a *PackRepoAdapter) SaveGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return a.graphs.SaveDefinition(ctx, def)
}

// --- ValidatorRepo ---

func (a *PackRepoAdapter) AgentKeyExists(ctx context.Context, agentKey string) (bool, error) {
	_, err := a.agents.GetAgentByAgentKey(ctx, agentKey)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *PackRepoAdapter) TeamKeyExists(ctx context.Context, teamKey string) (bool, error) {
	teams, err := a.teams.ListTeams(ctx)
	if err != nil {
		return false, err
	}
	for _, t := range teams {
		if t.TeamKey == teamKey {
			return true, nil
		}
	}
	return false, nil
}

func (a *PackRepoAdapter) TaxonomyKeyExists(ctx context.Context, key string) (bool, error) {
	_, err := a.taxonomy.GetTaxonomyNodeByKey(ctx, key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *PackRepoAdapter) SkillExists(ctx context.Context, slug string) (bool, error) {
	if a.skillLookup == nil {
		return false, nil
	}
	_, err := a.skillLookup.GetSkillBySkillKey(ctx, slug)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *PackRepoAdapter) FuncRefExists(funcRef string) bool {
	for _, reg := range tools.Registry() {
		if reg.Name == funcRef {
			return true
		}
	}
	return false
}
