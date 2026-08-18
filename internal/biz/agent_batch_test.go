package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
func (r *batchAgentRepo) ListAgentsByIDs(_ context.Context, ids []string) ([]Agent, error) {
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		if a, ok := r.agents[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
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
func (r *batchAgentRepo) ListAgentRuntimeSettings(context.Context) (map[string]AgentRuntimeSettings, error) {
	return map[string]AgentRuntimeSettings{}, nil
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
func (r *batchAgentRepo) ReorderAgents(context.Context, []string) error { return nil }
func (r *batchAgentRepo) CountAgentsByProviderAndModel(context.Context, string, string) (int, error) {
	return 0, nil
}
func (r *batchAgentRepo) CreateAgentAtomic(_ context.Context, a Agent, _ []AgentPromptFile, _ AgentRuntimeSettings) (Agent, error) {
	r.agents[a.AgentKey] = a
	return a, nil
}
func (r *batchAgentRepo) UpdateAgentAtomic(_ context.Context, a Agent, _ []AgentPromptFile, _ *AgentRuntimeSettings) (Agent, error) {
	r.agents[a.ID] = a
	return a, nil
}
func (r *batchAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) {
	return 0, nil
}
func (r *batchAgentRepo) ToggleFavorite(_ context.Context, id string) (Agent, error) {
	a, ok := r.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	if a.IsFavorite != nil {
		v := !*a.IsFavorite
		a.IsFavorite = &v
	} else {
		t := true
		a.IsFavorite = &t
	}
	r.agents[id] = a
	return a, nil
}

func TestBatchUpdateAgents_Status(t *testing.T) {
	repo := &batchAgentRepo{agents: map[string]Agent{"a1": {ID: "a1", Status: "active"}}}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	n, err := uc.BatchUpdateAgents(context.Background(), AgentBatchUpdateInput{IDs: []string{"a1"}, Status: "inactive"})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if repo.agents["a1"].Status != "inactive" {
		t.Fatalf("status=%q", repo.agents["a1"].Status)
	}
}

func TestReorderAgents_NotPersisted(t *testing.T) {
	repo := &batchAgentRepo{agents: map[string]Agent{"a1": {ID: "a1"}}}
	uc := NewAgentUsecase(AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
	if err := uc.ReorderAgents(context.Background(), nil); err != nil {
		t.Fatalf("empty ids should no-op: %v", err)
	}
	err := uc.ReorderAgents(context.Background(), []string{"a1"})
	if err == nil || !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("want FAILED_PRECONDITION, got %v", err)
	}
}

func TestMergeAgentCatalog_PreservesTaxonomyAndMission(t *testing.T) {
	current := Agent{
		ID:               "a1",
		AgentKey:         "k1",
		DisplayName:      "Old",
		PositionKey:      "ops/sre",
		AgentVariant:     "general",
		MissionStatement: "keep the lights on",
		DomainPath:       "运维/SRE",
		MetadataJSON:     `{"skip_category_responsibility":true}`,
		Roles:            []string{"worker"},
	}
	got := mergeAgentCatalog(current, Agent{DisplayName: "New Name"})
	if got.DisplayName != "New Name" {
		t.Fatalf("display=%q", got.DisplayName)
	}
	if got.AgentKey != "k1" {
		t.Fatalf("immutable AgentKey mutated: %q", got.AgentKey)
	}
	if got.PositionKey != "ops/sre" || got.AgentVariant != "general" {
		t.Fatalf("taxonomy dropped: key=%q variant=%q", got.PositionKey, got.AgentVariant)
	}
	if got.MissionStatement != "keep the lights on" || got.DomainPath != "运维/SRE" {
		t.Fatalf("mission dropped: statement=%q path=%q", got.MissionStatement, got.DomainPath)
	}
	if got.MetadataJSON != current.MetadataJSON {
		t.Fatalf("metadata dropped: %q", got.MetadataJSON)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "worker" {
		t.Fatalf("roles dropped: %v", got.Roles)
	}

	patched := mergeAgentCatalog(current, Agent{
		PositionKey:      "eng/backend",
		MissionStatement: "ship code",
		Roles:            []string{"coordinator"},
	})
	if patched.PositionKey != "eng/backend" || patched.MissionStatement != "ship code" {
		t.Fatalf("explicit patch not applied: %+v", patched)
	}
	if len(patched.Roles) != 1 || patched.Roles[0] != "coordinator" {
		t.Fatalf("roles=%v", patched.Roles)
	}
}
