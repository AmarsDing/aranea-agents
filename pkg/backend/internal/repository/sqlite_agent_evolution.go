package repository

import (
	"arenea/backend/internal/catalog/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) catalogEvolution() *sqlite.EvolutionRepository {
	return sqlite.NewEvolutionRepository(r.db)
}

func (r *SQLiteRepository) GetAgentIdentity(agentID string) (domain.AgentIdentity, error) {
	return r.catalogEvolution().GetAgentIdentity(agentID)
}

func (r *SQLiteRepository) UpsertAgentIdentity(id domain.AgentIdentity) (domain.AgentIdentity, error) {
	return r.catalogEvolution().UpsertAgentIdentity(id)
}

func (r *SQLiteRepository) GetAgentStrategyProfile(agentID string) (domain.AgentStrategyProfile, error) {
	return r.catalogEvolution().GetAgentStrategyProfile(agentID)
}

func (r *SQLiteRepository) UpsertAgentStrategyProfile(p domain.AgentStrategyProfile) (domain.AgentStrategyProfile, error) {
	return r.catalogEvolution().UpsertAgentStrategyProfile(p)
}

func (r *SQLiteRepository) InsertEvolutionEvent(e domain.EvolutionEvent) (domain.EvolutionEvent, error) {
	return r.catalogEvolution().InsertEvolutionEvent(e)
}

func (r *SQLiteRepository) GetEvolutionEvent(id string) (domain.EvolutionEvent, error) {
	return r.catalogEvolution().GetEvolutionEvent(id)
}

func (r *SQLiteRepository) ListEvolutionEvents(q EvolutionEventQuery) ([]domain.EvolutionEvent, int, error) {
	return r.catalogEvolution().ListEvolutionEvents(q)
}

func (r *SQLiteRepository) MarkEvolutionEventReverted(id, byEventID, atISO string) error {
	return r.catalogEvolution().MarkEvolutionEventReverted(id, byEventID, atISO)
}

func (r *SQLiteRepository) InsertEvolutionProposal(p domain.EvolutionProposal) (domain.EvolutionProposal, error) {
	return r.catalogEvolution().InsertEvolutionProposal(p)
}

func (r *SQLiteRepository) GetEvolutionProposal(id string) (domain.EvolutionProposal, error) {
	return r.catalogEvolution().GetEvolutionProposal(id)
}

func (r *SQLiteRepository) ListEvolutionProposals(q EvolutionProposalQuery) ([]domain.EvolutionProposal, int, error) {
	return r.catalogEvolution().ListEvolutionProposals(q)
}

func (r *SQLiteRepository) UpdateEvolutionProposalStatus(id, status, by, eventID, atISO string) error {
	return r.catalogEvolution().UpdateEvolutionProposalStatus(id, status, by, eventID, atISO)
}

func (r *SQLiteRepository) SupersedeProposalsByTarget(agentID, targetField, sinceISO string) (int, error) {
	return r.catalogEvolution().SupersedeProposalsByTarget(agentID, targetField, sinceISO)
}

func (r *SQLiteRepository) UpsertAgentSkillStat(s domain.AgentSkillStat) (domain.AgentSkillStat, error) {
	return r.catalogEvolution().UpsertAgentSkillStat(s)
}

func (r *SQLiteRepository) GetAgentSkillStat(agentID, scope, scopeValue, toolKey string) (domain.AgentSkillStat, error) {
	return r.catalogEvolution().GetAgentSkillStat(agentID, scope, scopeValue, toolKey)
}

func (r *SQLiteRepository) ListAgentSkillStats(agentID string, limit int) ([]domain.AgentSkillStat, error) {
	return r.catalogEvolution().ListAgentSkillStats(agentID, limit)
}
