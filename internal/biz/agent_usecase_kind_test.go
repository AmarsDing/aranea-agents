package biz

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
func (s *stubAgentRepo) ListAgentsByIDs(_ context.Context, ids []string) ([]Agent, error) {
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		if s.agent.ID == id {
			out = append(out, s.agent)
		}
	}
	return out, nil
}
func (s *stubAgentRepo) CreateAgent(context.Context, Agent) (Agent, error) { return Agent{}, nil }
func (s *stubAgentRepo) UpdateAgent(context.Context, Agent) (Agent, error) { return Agent{}, nil }
func (s *stubAgentRepo) DeleteAgent(context.Context, string) error         { return nil }
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
func (s *stubAgentRepo) CountAgentsByProviderAndModel(context.Context, string, string) (int, error) {
	return 0, nil
}
func (s *stubAgentRepo) CreateAgentAtomic(_ context.Context, a Agent, _ []AgentPromptFile, _ AgentRuntimeSettings) (Agent, error) {
	return a, nil
}
func (s *stubAgentRepo) UpdateAgentAtomic(_ context.Context, a Agent, _ []AgentPromptFile, _ *AgentRuntimeSettings) (Agent, error) {
	return a, nil
}
func (s *stubAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) {
	return 0, nil
}
func (s *stubAgentRepo) ToggleFavorite(_ context.Context, id string) (Agent, error) {
	return Agent{ID: id}, nil
}

func TestAgentUsecase_DeleteRejectsSystemBuiltin(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-1", Kind: "system_builtin"},
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	err := uc.Delete(context.Background(), "agent-1")
	if err == nil {
		t.Fatal("expected error when deleting system_builtin agent")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeForbidden {
		t.Fatalf("expected code %s, got %s", apierror.CodeForbidden, e.Code)
	}
	if e.Domain != "AGENT" {
		t.Fatalf("expected domain AGENT, got %s", e.Domain)
	}
}

func TestAgentUsecase_DeleteAllowsUserAgent(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-2", Source: "user"},
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	err := uc.Delete(context.Background(), "agent-2")
	if err != nil {
		t.Fatalf("expected no error when deleting user agent, got %v", err)
	}
}

func TestAgentUsecase_DeleteRejectsEcosystemPresetAgent(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-3", Kind: "ecosystem_preset"},
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	err := uc.Delete(context.Background(), "agent-3")
	if err == nil {
		t.Fatal("expected error when deleting ecosystem_preset agent directly")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeForbidden {
		t.Fatalf("expected code %s, got %s", apierror.CodeForbidden, e.Code)
	}
	if e.Domain != "AGENT" {
		t.Fatalf("expected domain AGENT, got %s", e.Domain)
	}
}

func TestAgentUsecase_DeleteRejectsReadonlyAgent(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepo{
		agent: Agent{ID: "agent-4", Kind: "ecosystem_preset", Readonly: true},
	}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	err := uc.Delete(context.Background(), "agent-4")
	if err == nil {
		t.Fatal("expected error when deleting readonly agent")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeForbidden {
		t.Fatalf("expected code %s, got %s", apierror.CodeForbidden, e.Code)
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
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	_, err := uc.Update(context.Background(), "agent-1", Agent{AgentKind: AgentKindA2AProxy})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "agent_kind is immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
