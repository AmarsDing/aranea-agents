package biz

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type stubAgentRepo struct {
	agent Agent
}

func (s *stubAgentRepo) SearchAgents(context.Context, AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}

func (s *stubAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]AgentListExtras, error) {
	return map[string]AgentListExtras{}, nil
}
func (s *stubAgentRepo) ListAgentCreators(context.Context) ([]AgentCreator, error) {
	return nil, nil
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
func (s *stubAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (s *stubAgentRepo) ReorderAgents(context.Context, []string) error { return nil }

func TestAgentUsecase_DeleteRejectsSystemBuiltin(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-1", Kind: "system_builtin"},
	}
	uc := NewAgentUsecase(repo, nil, nil, loggateway.NewNoop())
	err := uc.Delete(context.Background(), "agent-1")
	if err == nil {
		t.Fatal("expected error when deleting system_builtin agent")
	}
	e := kerrors.FromError(err)
	if e.Code != 403 {
		t.Fatalf("expected code 403, got %d", e.Code)
	}
	if e.Reason != "AGENT" {
		t.Fatalf("expected reason AGENT, got %s", e.Reason)
	}
}

func TestAgentUsecase_DeleteAllowsUserAgent(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-2", Source: "user"},
	}
	uc := NewAgentUsecase(repo, nil, nil, loggateway.NewNoop())
	err := uc.Delete(context.Background(), "agent-2")
	if err != nil {
		t.Fatalf("expected no error when deleting user agent, got %v", err)
	}
}

func TestAgentUsecase_UpdateRejectsKindChange(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{
			ID:         "agent-1",
			AgentKey:   "demo",
			AgentKind:  AgentKindLLM,
			ConfigJSON: EmbedAgentKindInConfigJSON("{}", AgentKindLLM, nil, loggateway.NewNoop()),
		},
	}
	uc := NewAgentUsecase(repo, nil, nil, loggateway.NewNoop())
	_, err := uc.Update(context.Background(), "agent-1", Agent{AgentKind: AgentKindA2AProxy})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent_kind is immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
