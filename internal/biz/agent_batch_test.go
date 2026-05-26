package biz

import (
	"context"
	"testing"
)

type batchAgentRepo struct {
	agents map[string]Agent
}

func (r *batchAgentRepo) SearchAgents(context.Context, AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}
func (r *batchAgentRepo) GetAgentByID(_ context.Context, id string) (Agent, error) {
	a, ok := r.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	return a, nil
}
func (r *batchAgentRepo) GetAgentByAgentKey(context.Context, string) (Agent, error) {
	return Agent{}, ErrNotFound
}
func (r *batchAgentRepo) CreateAgent(context.Context, Agent) (Agent, error) { return Agent{}, nil }
func (r *batchAgentRepo) UpdateAgent(_ context.Context, a Agent) (Agent, error) {
	r.agents[a.ID] = a
	return a, nil
}
func (r *batchAgentRepo) DeleteAgent(_ context.Context, id string) error {
	delete(r.agents, id)
	return nil
}
func (r *batchAgentRepo) GetAgentRuntimeSettings(context.Context, string) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{}, nil
}
func (r *batchAgentRepo) UpsertAgentRuntimeSettings(context.Context, AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{}, nil
}
func (r *batchAgentRepo) ListAgentPromptFiles(context.Context, string) ([]AgentPromptFile, error) {
	return nil, nil
}
func (r *batchAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []AgentPromptFile) ([]AgentPromptFile, error) {
	return nil, nil
}
func (r *batchAgentRepo) CreateAgentPromptFile(context.Context, AgentPromptFile) (AgentPromptFile, error) {
	return AgentPromptFile{}, nil
}
func (r *batchAgentRepo) UpdateAgentPromptFile(context.Context, AgentPromptFile) (AgentPromptFile, error) {
	return AgentPromptFile{}, nil
}
func (r *batchAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error { return nil }
func (r *batchAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]AgentListExtras, error) {
	return nil, nil
}
func (r *batchAgentRepo) ListAgentCreators(context.Context) ([]AgentCreator, error) { return nil, nil }
func (r *batchAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestBatchUpdateAgents_Status(t *testing.T) {
	repo := &batchAgentRepo{agents: map[string]Agent{"a1": {ID: "a1", Status: "active"}}}
	uc := NewAgentUsecase(repo, nil, nil)
	n, err := uc.BatchUpdateAgents(context.Background(), AgentBatchUpdateInput{IDs: []string{"a1"}, Status: "inactive"})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if repo.agents["a1"].Status != "inactive" {
		t.Fatalf("status=%q", repo.agents["a1"].Status)
	}
}
