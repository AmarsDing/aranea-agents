package biz

import (
	"context"
	"strings"
	"testing"
)

type stubAgentRepo struct {
	agent Agent
}

func (s *stubAgentRepo) SearchAgents(context.Context, AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}
func (s *stubAgentRepo) GetAgentByID(_ context.Context, id string) (Agent, error) {
	if s.agent.ID != id {
		return Agent{}, ErrNotFound
	}
	return s.agent, nil
}
func (s *stubAgentRepo) GetAgentByAgentKey(context.Context, string) (Agent, error) {
	return Agent{}, ErrNotFound
}
func (s *stubAgentRepo) CreateAgent(context.Context, Agent) (Agent, error) { return Agent{}, nil }
func (s *stubAgentRepo) UpdateAgent(context.Context, Agent) (Agent, error)   { return Agent{}, nil }
func (s *stubAgentRepo) DeleteAgent(context.Context, string) error           { return nil }
func (s *stubAgentRepo) GetAgentRuntimeSettings(context.Context, string) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{}, nil
}
func (s *stubAgentRepo) UpsertAgentRuntimeSettings(context.Context, AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return AgentRuntimeSettings{}, nil
}
func (s *stubAgentRepo) ListAgentPromptFiles(context.Context, string) ([]AgentPromptFile, error) {
	return nil, nil
}
func (s *stubAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []AgentPromptFile) ([]AgentPromptFile, error) {
	return nil, nil
}
func (s *stubAgentRepo) CreateAgentPromptFile(context.Context, AgentPromptFile) (AgentPromptFile, error) {
	return AgentPromptFile{}, nil
}
func (s *stubAgentRepo) UpdateAgentPromptFile(context.Context, AgentPromptFile) (AgentPromptFile, error) {
	return AgentPromptFile{}, nil
}
func (s *stubAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error { return nil }

func TestAgentUsecase_UpdateRejectsKindChange(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{
			ID:         "agent-1",
			AgentKey:   "demo",
			Kind:       AgentKindLLM,
			ConfigJSON: EmbedAgentKindInConfigJSON("{}", AgentKindLLM, nil),
		},
	}
	uc := NewAgentUsecase(repo, nil)
	_, err := uc.Update(context.Background(), "agent-1", Agent{Kind: AgentKindA2AProxy})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent_kind is immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
