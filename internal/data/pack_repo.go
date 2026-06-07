package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/tools"
)

// PackRepoAdapter adapts existing biz repos to satisfy pack engine interfaces.
type PackRepoAdapter struct {
	agents      biz.AgentRepository
	teamReader  biz.TeamReader
	teamWriter  biz.TeamWriter
	organization biz.OrganizationRepo
	graphs      biz.GraphRepo
	skillLookup biz.SkillLookupReader
	execInTx    func(ctx context.Context, fn func(ctx context.Context) error) error
}

var _ pack.ExporterRepo = (*PackRepoAdapter)(nil)
var _ pack.ImporterRepo = (*PackRepoAdapter)(nil)
var _ pack.ValidatorRepo = (*PackRepoAdapter)(nil)

// NewPackRepoAdapter creates a composite adapter from existing repos.
func NewPackRepoAdapter(
	agents biz.AgentRepository,
	teamReader biz.TeamReader,
	teamWriter biz.TeamWriter,
	organization biz.OrganizationRepo,
	graphs biz.GraphRepo,
	skillLookup biz.SkillLookupReader,
) *PackRepoAdapter {
	// Use agents' ExecInTx as the transaction provider (AgentRepository embeds it)
	var txFn func(ctx context.Context, fn func(ctx context.Context) error) error
	if agents != nil {
		txFn = agents.ExecInTx
	}
	return &PackRepoAdapter{
		agents:       agents,
		teamReader:   teamReader,
		teamWriter:   teamWriter,
		organization: organization,
		graphs:       graphs,
		skillLookup:  skillLookup,
		execInTx:     txFn,
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
	return a.teamReader.GetTeamByID(ctx, id)
}

func (a *PackRepoAdapter) ListTeams(ctx context.Context) ([]biz.Team, error) {
	return a.teamReader.ListTeams(ctx)
}

func (a *PackRepoAdapter) GetOrganizationNode(ctx context.Context, id string) (biz.OrganizationNode, error) {
	return a.organization.GetOrgNode(ctx, id)
}

func (a *PackRepoAdapter) GetOrgAncestors(ctx context.Context, positionID string) (biz.OrgAncestors, error) {
	pos, err := a.organization.GetOrgNode(ctx, positionID)
	if err != nil {
		return biz.OrgAncestors{}, err
	}
	var dept biz.OrganizationNode
	if pos.ParentID != "" {
		dept, err = a.organization.GetOrgNode(ctx, pos.ParentID)
		if err != nil {
			return biz.OrgAncestors{}, err
		}
	}
	var company biz.OrganizationNode
	if dept.ParentID != "" {
		company, err = a.organization.GetOrgNode(ctx, dept.ParentID)
		if err != nil {
			return biz.OrgAncestors{}, err
		}
	}
	return biz.OrgAncestors{
		Company:    company,
		Department: dept,
		Position:   pos,
	}, nil
}

func (a *PackRepoAdapter) ListOrganizationNodesByParentID(ctx context.Context, parentID string) ([]biz.OrganizationNode, error) {
	return a.organization.ListOrgNodesByParentID(ctx, parentID)
}

func (a *PackRepoAdapter) ListOrganizationNodesByLevel(ctx context.Context, level string) ([]biz.OrganizationNode, error) {
	return a.organization.ListOrgNodesByLevel(ctx, level)
}

func (a *PackRepoAdapter) GetGraph(ctx context.Context, id string) (*biz.GraphDefinition, error) {
	return a.graphs.GetDefinition(ctx, id)
}

// --- ImporterRepo ---

func (a *PackRepoAdapter) CreateOrganizationNode(ctx context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error) {
	return a.organization.CreateOrgNode(ctx, node)
}

func (a *PackRepoAdapter) UpdateOrganizationNode(ctx context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error) {
	return a.organization.UpdateOrgNode(ctx, node)
}

func (a *PackRepoAdapter) GetOrganizationNodeByKey(ctx context.Context, key string) (biz.OrganizationNode, error) {
	return a.organization.GetOrgNodeByKey(ctx, key)
}

func (a *PackRepoAdapter) GetOrganizationNodeByKeyAnyState(ctx context.Context, key string) (biz.OrganizationNode, error) {
	return a.organization.GetOrgNodeByKeyAnyState(ctx, key)
}

func (a *PackRepoAdapter) CreateAgent(ctx context.Context, agent biz.Agent) (biz.Agent, error) {
	return a.agents.CreateAgent(ctx, agent)
}

func (a *PackRepoAdapter) CreateAgentAtomic(ctx context.Context, agent biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a.agents.CreateAgentAtomic(ctx, agent, files, settings)
}

func (a *PackRepoAdapter) UpdateAgent(ctx context.Context, agent biz.Agent) (biz.Agent, error) {
	return a.agents.UpdateAgent(ctx, agent)
}

func (a *PackRepoAdapter) UpdateAgentAtomic(ctx context.Context, agent biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a.agents.UpdateAgentAtomic(ctx, agent, files, settings)
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
	return a.teamReader.GetTeamByID(ctx, id)
}

func (a *PackRepoAdapter) GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error) {
	return a.teamReader.GetTeamByKey(ctx, teamKey)
}

func (a *PackRepoAdapter) CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	return a.teamWriter.CreateTeam(ctx, t)
}

func (a *PackRepoAdapter) UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error) {
	return a.teamWriter.UpdateTeam(ctx, t)
}

func (a *PackRepoAdapter) SaveGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return a.graphs.SaveDefinition(ctx, def)
}

func (a *PackRepoAdapter) GetGraphDefinitionByName(ctx context.Context, name string) (*biz.GraphDefinition, error) {
	return a.graphs.GetDefinitionByName(ctx, name)
}

func (a *PackRepoAdapter) UpdateGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return a.graphs.UpdateDefinition(ctx, def)
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
	_, err := a.teamReader.GetTeamByKey(ctx, teamKey)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a *PackRepoAdapter) OrgKeyExists(ctx context.Context, key string) (bool, error) {
	_, err := a.organization.GetOrgNodeByKey(ctx, key)
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

// ExecInTx delegates to the underlying transaction provider.
func (a *PackRepoAdapter) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if a.execInTx != nil {
		return a.execInTx(ctx, fn)
	}
	// Fallback: no transaction support, execute directly
	return fn(ctx)
}
