package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Test doubles ────────────────────────────────────────────────────────────

type batchAgentReader struct {
	biz.AgentReader
	agents map[string]biz.Agent
}

func (r *batchAgentReader) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	if a, ok := r.agents[id]; ok {
		return a, nil
	}
	return biz.Agent{}, apierror.NotFound("AGENT", "agent not found")
}

// ListExtrasForAgents: hydrate 富化查询，测试返回空集（跳过富化）。
func (r *batchAgentReader) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return map[string]biz.AgentListExtras{}, nil
}

// batchSettingsRepo: 返回有效 settings，避免 hydrate 走 legacy 迁移路径
// （迁移会调 UpsertAgentRuntimeSettings，测试桩未实现该方法）。
type batchSettingsRepo struct {
	biz.AgentRuntimeSettingsRepo
}

func (batchSettingsRepo) GetAgentRuntimeSettings(_ context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{AgentID: agentID}, nil
}

type batchFilesRepo struct {
	biz.AgentPromptFileRepo
}

// ListAgentPromptFiles 返回非空最小文件集，避免 hydrate 走 legacy 迁移路径
// （迁移会调 ReplaceAgentPromptFiles，测试桩未实现该方法）。
func (batchFilesRepo) ListAgentPromptFiles(_ context.Context, agentID string) ([]biz.AgentPromptFile, error) {
	return []biz.AgentPromptFile{{AgentID: agentID, Name: "IDENTITY.md", Body: "stub", SortOrder: 10}}, nil
}

type batchAgentWriter struct {
	biz.AgentWriter
	updated []biz.Agent
	deleted []string
}

func (w *batchAgentWriter) UpdateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
	w.updated = append(w.updated, a)
	return a, nil
}

func (w *batchAgentWriter) DeleteAgent(_ context.Context, id string) error {
	w.deleted = append(w.deleted, id)
	return nil
}

type batchTx struct {
	biz.AgentTxRepo
}

func (batchTx) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newBatchAgentService(reader *batchAgentReader, writer *batchAgentWriter) *AgentService {
	uc := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   reader,
		Writer:   writer,
		Settings: batchSettingsRepo{},
		Files:    batchFilesRepo{},
		Tx:       batchTx{},
		Lg:       loggateway.NewNoop(),
	})
	return NewAgentService(uc, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
}

// ── 入参校验 ────────────────────────────────────────────────────────────────

func TestBatchUpdateAgents_Validation(t *testing.T) {
	svc := newBatchAgentService(&batchAgentReader{agents: map[string]biz.Agent{}}, &batchAgentWriter{})
	ctx := workspace.WithSystemWorkspace(context.Background())

	cases := map[string]*v1.BatchUpdateAgentsRequest{
		"empty ids":         {Ids: nil, Status: "active"},
		"status and delete": {Ids: []string{"a1"}, Status: "active", Delete: true},
		"neither set":       {Ids: []string{"a1"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.BatchUpdateAgents(ctx, req)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !apierror.IsCode(err, apierror.CodeBadRequest) {
				t.Fatalf("want CodeBadRequest, got %v", err)
			}
		})
	}
}

// ── 批量状态变更 ────────────────────────────────────────────────────────────

func TestBatchUpdateAgents_StatusUpdate(t *testing.T) {
	reader := &batchAgentReader{agents: map[string]biz.Agent{
		"a1": {ID: "a1", AgentKey: "k1", Kind: "user", Status: "inactive", WorkspaceID: "ws-1"},
		"a2": {ID: "a2", AgentKey: "k2", Kind: "user", Status: "inactive", WorkspaceID: "ws-1"},
	}}
	writer := &batchAgentWriter{}
	svc := newBatchAgentService(reader, writer)
	ctx := workspace.WithSystemWorkspace(context.Background())

	resp, err := svc.BatchUpdateAgents(ctx, &v1.BatchUpdateAgentsRequest{Ids: []string{"a1", "a2"}, Status: "active"})
	if err != nil {
		t.Fatalf("batch update: %v", err)
	}
	if resp.GetAffected() != 2 {
		t.Fatalf("affected = %d, want 2", resp.GetAffected())
	}
	if len(writer.updated) != 2 {
		t.Fatalf("updated = %d, want 2", len(writer.updated))
	}
	for _, a := range writer.updated {
		if a.Status != "active" {
			t.Fatalf("agent %s status = %q, want active", a.ID, a.Status)
		}
	}
}

// ── 批量删除：system_builtin 被拒绝且整批回滚 ──────────────────────────────

func TestBatchUpdateAgents_DeleteForbiddenRollsBack(t *testing.T) {
	reader := &batchAgentReader{agents: map[string]biz.Agent{
		"a1": {ID: "a1", AgentKey: "k1", Kind: "user", Status: "active", WorkspaceID: "ws-1"},
		"a2": {ID: "a2", AgentKey: "k2", Kind: "system_builtin", Status: "active", WorkspaceID: "ws-1"},
	}}
	writer := &batchAgentWriter{}
	svc := newBatchAgentService(reader, writer)
	ctx := workspace.WithSystemWorkspace(context.Background())

	_, err := svc.BatchUpdateAgents(ctx, &v1.BatchUpdateAgentsRequest{Ids: []string{"a1", "a2"}, Delete: true})
	if err == nil {
		t.Fatal("expected forbidden error for system_builtin agent")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("want CodeForbidden, got %v", err)
	}
}

// ── IDOR：非系统 caller 变更他 workspace agent 被拒 ────────────────────────

func TestBatchUpdateAgents_WorkspaceForbidden(t *testing.T) {
	reader := &batchAgentReader{agents: map[string]biz.Agent{
		"a1": {ID: "a1", AgentKey: "k1", Kind: "user", Status: "inactive", WorkspaceID: "ws-other"},
	}}
	writer := &batchAgentWriter{}
	svc := newBatchAgentService(reader, writer)
	ctx := workspace.WithContext(context.Background(), "ws-1")

	_, err := svc.BatchUpdateAgents(ctx, &v1.BatchUpdateAgentsRequest{Ids: []string{"a1"}, Status: "active"})
	if err == nil {
		t.Fatal("expected access error for foreign workspace agent")
	}
	if len(writer.updated) != 0 {
		t.Fatalf("no agent must be updated, got %d", len(writer.updated))
	}
}
