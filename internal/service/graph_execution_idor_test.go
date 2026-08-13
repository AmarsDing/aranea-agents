package service

import (
	"context"
	"testing"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- stubs for graph execution IDOR tests ---

type idorGraphRepo struct {
	defs map[string]*biz.GraphDefinition
}

func (r *idorGraphRepo) GetDefinition(_ context.Context, id string) (*biz.GraphDefinition, error) {
	if d, ok := r.defs[id]; ok {
		return d, nil
	}
	return nil, apierror.NotFound("GRAPH", "graph definition not found")
}
func (r *idorGraphRepo) GetDefinitionByName(context.Context, string) (*biz.GraphDefinition, error) {
	return nil, apierror.NotFound("GRAPH", "graph definition not found")
}
func (r *idorGraphRepo) ListDefinitions(context.Context, int, string) ([]*biz.GraphDefinition, string, error) {
	return nil, "", nil
}
func (r *idorGraphRepo) ListUserTemplateDefinitions(context.Context, int) ([]*biz.GraphDefinition, error) {
	return nil, nil
}
func (r *idorGraphRepo) ListDefinitionsByWorkspace(context.Context, int, string, string) ([]*biz.GraphDefinition, string, error) {
	return nil, "", nil
}
func (r *idorGraphRepo) ListUserTemplateDefinitionsByWorkspace(context.Context, int, string) ([]*biz.GraphDefinition, error) {
	return nil, nil
}
func (r *idorGraphRepo) SaveDefinition(_ context.Context, d *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return d, nil
}
func (r *idorGraphRepo) UpdateDefinition(_ context.Context, d *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	return d, nil
}
func (r *idorGraphRepo) DeleteDefinition(context.Context, string) error { return nil }
func (r *idorGraphRepo) ReorderGraphs(context.Context, []string) error  { return nil }

type idorGraphRunRepo struct {
	execs map[string]*biz.GraphExecution
}

func (r *idorGraphRunRepo) SaveRun(context.Context, *biz.GraphExecution) error { return nil }
func (r *idorGraphRunRepo) GetRun(_ context.Context, id string) (*biz.GraphExecution, error) {
	if e, ok := r.execs[id]; ok {
		return e, nil
	}
	return nil, apierror.NotFound("GRAPH", "execution not found")
}
func (r *idorGraphRunRepo) ListRunsByGraph(context.Context, string, int, string, ...biz.GraphRunListOption) ([]*biz.GraphExecution, string, error) {
	return nil, "", nil
}
func (r *idorGraphRunRepo) UpdateRun(context.Context, *biz.GraphExecution) error { return nil }

func newIDORGraphService() *GraphService {
	repo := &idorGraphRepo{defs: map[string]*biz.GraphDefinition{
		"graph-private-a": {ID: "graph-private-a", Name: "tenant A graph", WorkspaceID: "ws-a"},
		"graph-shared":    {ID: "graph-shared", Name: "shared graph", WorkspaceID: ""},
	}}
	runRepo := &idorGraphRunRepo{execs: map[string]*biz.GraphExecution{
		"exec-a1": biz.NewGraphExecution(context.Background(), "exec-a1", "graph-private-a", "sess-1", string(biz.GraphExecRunning)),
		"exec-s1": biz.NewGraphExecution(context.Background(), "exec-s1", "graph-shared", "sess-2", string(biz.GraphExecRunning)),
	}}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{
		Repo:    repo,
		RunRepo: runRepo,
		Lg:      loggateway.NewNoop(),
	})
	return &GraphService{uc: uc, lg: loggateway.NewNoop()}
}

func wsCtx(wsID string) context.Context {
	return workspace.WithContext(context.Background(), wsID)
}

func assertNotFound(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected NotFound error for cross-tenant access, got nil", what)
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("%s: expected NotFound, got %v", what, err)
	}
}

// TestGraphExecutionService_IDOR covers B3: execution-plane RPCs must enforce
// the same workspace access check as definition-plane RPCs.
func TestGraphExecutionService_IDOR(t *testing.T) {
	svc := newIDORGraphService()
	tenantB := wsCtx("ws-b")

	t.Run("ExecuteGraph_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ExecuteGraph(tenantB, &graphv1.ExecuteGraphRequest{GraphId: "graph-private-a"})
		assertNotFound(t, err, "ExecuteGraph")
	})

	t.Run("GetGraphExecution_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.GetGraphExecution(tenantB, &graphv1.GetGraphExecutionRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "GetGraphExecution")
	})

	t.Run("ListGraphExecutions_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListGraphExecutions(tenantB, &graphv1.ListGraphExecutionsRequest{GraphId: "graph-private-a"})
		assertNotFound(t, err, "ListGraphExecutions")
	})

	t.Run("CancelGraphExecution_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.CancelGraphExecution(tenantB, &graphv1.CancelGraphExecutionRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "CancelGraphExecution")
	})

	t.Run("ResumeGraph_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ResumeGraph(tenantB, &graphv1.ResumeGraphRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "ResumeGraph")
	})

	t.Run("TimeTravelGraph_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.TimeTravelGraph(tenantB, &graphv1.TimeTravelGraphRequest{ExecutionId: "exec-a1", StepIndex: 0})
		assertNotFound(t, err, "TimeTravelGraph")
	})

	t.Run("ListCheckpoints_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListCheckpoints(tenantB, &graphv1.ListCheckpointsRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "ListCheckpoints")
	})

	t.Run("GetStateSnapshot_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.GetStateSnapshot(tenantB, &graphv1.GetStateSnapshotRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "GetStateSnapshot")
	})

	t.Run("EditState_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.EditState(tenantB, &graphv1.EditStateRequest{ExecutionId: "exec-a1"})
		assertNotFound(t, err, "EditState")
	})

	t.Run("GetGraphExecution_OwnerWorkspace_Allowed", func(t *testing.T) {
		resp, err := svc.GetGraphExecution(wsCtx("ws-a"), &graphv1.GetGraphExecutionRequest{ExecutionId: "exec-a1"})
		if err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
		if resp.ExecutionId != "exec-a1" {
			t.Fatalf("unexpected execution id: %s", resp.ExecutionId)
		}
	})

	t.Run("GetGraphExecution_SharedGraph_Allowed", func(t *testing.T) {
		_, err := svc.GetGraphExecution(tenantB, &graphv1.GetGraphExecutionRequest{ExecutionId: "exec-s1"})
		if err != nil {
			t.Fatalf("shared graph execution should be visible to any tenant: %v", err)
		}
	})
}
