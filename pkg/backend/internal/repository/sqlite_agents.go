package repository

import (
	"arenea/backend/internal/catalog/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) catalogAgents() *sqlite.AgentRepository {
	return sqlite.NewAgentRepository(r.db)
}

func (r *SQLiteRepository) ListAgents() ([]domain.Agent, error) {
	return r.catalogAgents().ListAgents()
}

func (r *SQLiteRepository) SearchAgents(query domain.AgentListQuery) (domain.AgentListResult, error) {
	return r.catalogAgents().SearchAgents(query)
}

func (r *SQLiteRepository) GetAgentByID(id string) (domain.Agent, error) {
	return r.catalogAgents().GetAgentByID(id)
}

func (r *SQLiteRepository) GetAgentByKey(key string) (domain.Agent, error) {
	return r.catalogAgents().GetAgentByKey(key)
}

func (r *SQLiteRepository) CreateAgent(a domain.Agent) (domain.Agent, error) {
	return r.catalogAgents().CreateAgent(a)
}

func (r *SQLiteRepository) UpdateAgent(a domain.Agent) (domain.Agent, error) {
	return r.catalogAgents().UpdateAgent(a)
}

func (r *SQLiteRepository) GetAgentRuntimeSettings(agentID string) (domain.AgentRuntimeSettings, error) {
	return r.catalogAgents().GetAgentRuntimeSettings(agentID)
}

func (r *SQLiteRepository) UpsertAgentRuntimeSettings(v domain.AgentRuntimeSettings) (domain.AgentRuntimeSettings, error) {
	return r.catalogAgents().UpsertAgentRuntimeSettings(v)
}

func (r *SQLiteRepository) ListAgentPromptFiles(agentID string) ([]domain.AgentPromptFile, error) {
	return r.catalogAgents().ListAgentPromptFiles(agentID)
}

func (r *SQLiteRepository) ReplaceAgentPromptFiles(agentID string, files []domain.AgentPromptFile) ([]domain.AgentPromptFile, error) {
	return r.catalogAgents().ReplaceAgentPromptFiles(agentID, files)
}

func (r *SQLiteRepository) DeleteAgent(id string) error {
	return r.catalogAgents().DeleteAgent(id)
}
