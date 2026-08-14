package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Test doubles ────────────────────────────────────────────────────────────

type evoSvcStore struct {
	biz.UnifiedEvolutionStore
	row          *biz.UnifiedEvolutionSuggestion
	statusUpdate string
	statusReason string
}

func (s *evoSvcStore) GetByID(context.Context, string) (*biz.UnifiedEvolutionSuggestion, error) {
	return s.row, nil
}

func (s *evoSvcStore) UpdateStatus(_ context.Context, _ string, status string, _ string, reason string) error {
	s.statusUpdate = status
	s.statusReason = reason
	s.row.Status = status
	return nil
}

// UpdateStatusCAS mirrors UnifiedEvolutionRepo.UpdateStatusCAS: update only when
// the current status is in from; report whether the transition happened.
func (s *evoSvcStore) UpdateStatusCAS(ctx context.Context, id string, from []string, to string, actor string, reason string) (bool, error) {
	allowed := len(from) == 0
	for _, f := range from {
		if s.row.Status == f {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, nil
	}
	if err := s.UpdateStatus(ctx, id, to, actor, reason); err != nil {
		return false, err
	}
	return true, nil
}

type evoSvcAgentRepo struct {
	biz.AgentRepository
	files []biz.AgentPromptFile
}

func (r *evoSvcAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return r.files, nil
}

func (r *evoSvcAgentRepo) ReplaceAgentPromptFiles(_ context.Context, _ string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	r.files = files
	return files, nil
}

func evoSvcRow(t *testing.T, status string, preApplySnapshot string) *biz.UnifiedEvolutionSuggestion {
	t.Helper()
	meta := map[string]string{
		biz.EvoMetaLegacyType: "prompt",
		biz.EvoMetaTitle:      "重写系统提示",
	}
	if preApplySnapshot != "" {
		meta[biz.EvoMetaPreApplySnapshot] = preApplySnapshot
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return &biz.UnifiedEvolutionSuggestion{
		ID:            "sug-1",
		TargetType:    biz.EvolutionTargetAgent,
		TargetID:      "agent-1",
		ActionType:    biz.EvolutionActionEvolve,
		TriggerSource: "agent_config",
		Status:        status,
		DraftBody:     "新系统提示。",
		Metadata:      metadata,
		CreatedAt:     time.Now().UTC(),
	}
}

// evoAgentReader 仅实现 assertAgentAccess 所需的读取路径（P0-1c）。
type evoAgentReader struct {
	biz.AgentReader
	agent biz.Agent
}

func (r *evoAgentReader) GetAgentByID(context.Context, string) (biz.Agent, error) {
	return r.agent, nil
}

func (r *evoAgentReader) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return nil, nil
}

// evoAgentSettingsRepo 返回零值 settings，避免 hydrate 触发 legacy 迁移写路径。
type evoAgentSettingsRepo struct {
	biz.AgentRuntimeSettingsRepo
}

func (evoAgentSettingsRepo) GetAgentRuntimeSettings(_ context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{AgentID: agentID}, nil
}

func newEvoSvcAgentService(store *evoSvcStore, agents *evoSvcAgentRepo) *AgentService {
	return newEvoSvcAgentServiceForWorkspace(store, agents, workspace.DefaultWorkspaceID)
}

func newEvoSvcAgentServiceForWorkspace(store *evoSvcStore, agents *evoSvcAgentRepo, agentWS string) *AgentService {
	evoUC := biz.NewEvolutionUsecase(nil, store, agents, loggateway.NewNoop())
	agentUC := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   &evoAgentReader{agent: biz.Agent{ID: "agent-1", WorkspaceID: agentWS}},
		Settings: evoAgentSettingsRepo{},
		Files:    agents,
		Lg:       loggateway.NewNoop(),
	})
	return NewAgentService(agentUC, evoUC, nil, nil, nil, nil, loggateway.NewNoop(), nil)
}

// ── Reject: reason 必须透传到 store ─────────────────────────────────────────

func TestRejectEvolutionSuggestion_PassesReason(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentService(store, &evoSvcAgentRepo{})

	got, err := svc.RejectEvolutionSuggestion(context.Background(), &v1.RejectEvolutionSuggestionRequest{
		AgentId:      "agent-1",
		SuggestionId: "sug-1",
		Reason:       "与当前业务方向不符",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got.GetStatus() != biz.EvolutionStatusRejected {
		t.Errorf("status = %q, want rejected", got.GetStatus())
	}
	if store.statusReason != "与当前业务方向不符" {
		t.Errorf("reason not propagated to store: got %q", store.statusReason)
	}
}

// ── Rollback: applied → rolled_back，prompt 文件恢复到快照 ──────────────────

func TestRollbackEvolutionSuggestion_RestoresSnapshot(t *testing.T) {
	snapshot, err := json.Marshal(map[string]string{"AGENTS_CORE.md": "旧系统提示。"})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusApplied, string(snapshot))}
	agents := &evoSvcAgentRepo{files: []biz.AgentPromptFile{
		{AgentID: "agent-1", Name: "AGENTS_CORE.md", Body: "新系统提示。", SortOrder: 10},
	}}
	svc := newEvoSvcAgentService(store, agents)

	got, err := svc.RollbackEvolutionSuggestion(context.Background(), &v1.RollbackEvolutionSuggestionRequest{
		AgentId:      "agent-1",
		SuggestionId: "sug-1",
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got.GetStatus() != biz.EvolutionStatusRolledBack {
		t.Errorf("status = %q, want rolled_back", got.GetStatus())
	}
	if agents.files[0].Body != "旧系统提示。" {
		t.Errorf("prompt file not restored: %q", agents.files[0].Body)
	}
}

func TestRollbackEvolutionSuggestion_RejectsNonApplied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentService(store, &evoSvcAgentRepo{})

	if _, err := svc.RollbackEvolutionSuggestion(context.Background(), &v1.RollbackEvolutionSuggestionRequest{
		AgentId:      "agent-1",
		SuggestionId: "sug-1",
	}); err == nil {
		t.Fatal("rolling back a pending suggestion must fail")
	}
}

// ── P0-1c: workspace IDOR 断言 ───────────────────────────────────────────────

func TestGetAgentEvolutionSuggestions_CrossWorkspaceDenied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentServiceForWorkspace(store, &evoSvcAgentRepo{}, "ws-b")

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.GetAgentEvolutionSuggestions(ctx, &v1.GetAgentEvolutionSuggestionsRequest{AgentId: "agent-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace list must return NotFound, got %v", err)
	}
}

func TestApplyEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentServiceForWorkspace(store, &evoSvcAgentRepo{}, "ws-b")

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.ApplyEvolutionSuggestion(ctx, &v1.ApplyEvolutionSuggestionRequest{AgentId: "agent-1", SuggestionId: "sug-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace apply must return NotFound, got %v", err)
	}
	if store.statusUpdate != "" {
		t.Fatalf("status must not change after denied apply, got %q", store.statusUpdate)
	}
}

func TestRejectEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentServiceForWorkspace(store, &evoSvcAgentRepo{}, "ws-b")

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.RejectEvolutionSuggestion(ctx, &v1.RejectEvolutionSuggestionRequest{
		AgentId:      "agent-1",
		SuggestionId: "sug-1",
		Reason:       "x",
	})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace reject must return NotFound, got %v", err)
	}
	if store.statusUpdate != "" {
		t.Fatalf("status must not change after denied reject, got %q", store.statusUpdate)
	}
}

func TestRollbackEvolutionSuggestion_CrossWorkspaceDenied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusApplied, `{"AGENTS_CORE.md":"旧"}`)}
	svc := newEvoSvcAgentServiceForWorkspace(store, &evoSvcAgentRepo{}, "ws-b")

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.RollbackEvolutionSuggestion(ctx, &v1.RollbackEvolutionSuggestionRequest{AgentId: "agent-1", SuggestionId: "sug-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace rollback must return NotFound, got %v", err)
	}
	if store.statusUpdate != "" {
		t.Fatalf("status must not change after denied rollback, got %q", store.statusUpdate)
	}
}

func TestGetAgentEvolutionMetrics_CrossWorkspaceDenied(t *testing.T) {
	store := &evoSvcStore{row: evoSvcRow(t, biz.EvolutionStatusPending, "")}
	svc := newEvoSvcAgentServiceForWorkspace(store, &evoSvcAgentRepo{}, "ws-b")

	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.GetAgentEvolutionMetrics(ctx, &v1.GetAgentEvolutionMetricsRequest{AgentId: "agent-1"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-workspace metrics must return NotFound, got %v", err)
	}
}
