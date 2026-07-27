package agent

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
)

func (m *memAgentRepo) ToggleFavorite(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, nil
}

type memAgentRepo struct {
	byKey map[string]biz.Agent
}

func (m *memAgentRepo) SearchAgents(context.Context, biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}

func (m *memAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return map[string]biz.AgentListExtras{}, nil
}
func (m *memAgentRepo) ListAgentCreators(context.Context) ([]biz.AgentCreator, error) {
	return nil, nil
}
func (m *memAgentRepo) GetAgentByID(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (m *memAgentRepo) GetAgentByAgentKey(_ context.Context, key string) (biz.Agent, error) {
	if ag, ok := m.byKey[key]; ok {
		return ag, nil
	}
	return biz.Agent{}, sql.ErrNoRows
}
func (m *memAgentRepo) ListAgentsByIDs(_ context.Context, _ []string) ([]biz.Agent, error) {
	return nil, nil
}
func (m *memAgentRepo) CreateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (m *memAgentRepo) UpdateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (m *memAgentRepo) DeleteAgent(context.Context, string) error { return nil }
func (m *memAgentRepo) GetAgentRuntimeSettings(context.Context, string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (m *memAgentRepo) ListAgentRuntimeSettings(context.Context) (map[string]biz.AgentRuntimeSettings, error) {
	return map[string]biz.AgentRuntimeSettings{}, nil
}
func (m *memAgentRepo) UpsertAgentRuntimeSettings(context.Context, biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (m *memAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (m *memAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (m *memAgentRepo) CreateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (m *memAgentRepo) UpdateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (m *memAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error { return nil }
func (m *memAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (m *memAgentRepo) ReorderAgents(context.Context, []string) error                  { return nil }
func (m *memAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) { return 0, nil }
func (m *memAgentRepo) CountAgentsByProviderAndModel(context.Context, string, string) (int, error) {
	return 0, nil
}
func (m *memAgentRepo) CreateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a, nil
}
func (m *memAgentRepo) UpdateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ *biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a, nil
}

func TestResolveBizAgentByKey(t *testing.T) {
	repo := &memAgentRepo{byKey: map[string]biz.Agent{
		"demo": {ID: "id-1", AgentKey: "demo", Provider: "openai", Model: "gpt-4"},
	}}
	deps := TRPCBuilderDeps{TRPCModelCatalogDeps: TRPCModelCatalogDeps{Agents: repo}}
	ag, err := resolveBizAgentByKey(context.Background(), deps, "demo")
	if err != nil {
		t.Fatalf("resolveBizAgentByKey() error = %v", err)
	}
	if ag.AgentKey != "demo" {
		t.Fatalf("AgentKey = %q, want demo", ag.AgentKey)
	}
}

func TestBizAgentFactoryOptionsDedupesKeys(t *testing.T) {
	opts := BizAgentFactoryOptions(TRPCBuilderDeps{}, "a", "a", "b")
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}
}
