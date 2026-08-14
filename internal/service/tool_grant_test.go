package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// fakeGrantStore is an in-memory biztool.ToolGrantStore for service tests.
type fakeGrantStore struct {
	grants map[string]biztool.ToolGrant
}

func newFakeGrantStore() *fakeGrantStore {
	return &fakeGrantStore{grants: make(map[string]biztool.ToolGrant)}
}

func (f *fakeGrantStore) HasToolGrant(_ context.Context, agentID, toolKey string) (bool, error) {
	_, ok := f.grants[agentID+"|"+toolKey]
	return ok, nil
}

func (f *fakeGrantStore) ListToolGrants(_ context.Context, agentID string) ([]biztool.ToolGrant, error) {
	var out []biztool.ToolGrant
	for _, g := range f.grants {
		if g.AgentID == agentID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeGrantStore) CreateToolGrant(_ context.Context, g biztool.ToolGrant) error {
	f.grants[g.AgentID+"|"+g.ToolKey] = g
	return nil
}

func (f *fakeGrantStore) DeleteToolGrant(_ context.Context, agentID, toolKey string) error {
	delete(f.grants, agentID+"|"+toolKey)
	return nil
}

func newToolGrantTestService(store *fakeGrantStore) *ToolService {
	uc := biztool.NewToolUsecase(nil, nil, loggateway.NewNoop(), biztool.WithToolGrantStore(store))
	// A2 access checks call agents.Get — inject a real AgentUsecase over stubs
	// (nil would panic). The reader serves the agents used by these tests.
	agents := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   &batchAgentReader{agents: map[string]biz.Agent{"agent-1": {ID: "agent-1"}}},
		Settings: batchSettingsRepo{},
		Files:    batchFilesRepo{},
		Lg:       loggateway.NewNoop(),
	})
	return NewToolService(uc, agents, nil)
}

// toolGrantCtx scopes calls as the system workspace: production callers reach
// these endpoints post-auth, and the batch-test convention uses the system
// bypass to keep fixtures workspace-agnostic.
func toolGrantCtx() context.Context {
	return workspace.WithSystemWorkspace(context.Background())
}

func TestToolService_ListToolGrants(t *testing.T) {
	store := newFakeGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{
		ID: "g1", AgentID: "agent-1", ToolKey: "bash", GrantedBy: "user-1", CreatedAt: "2026-07-20T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	svc := newToolGrantTestService(store)
	resp, err := svc.ListToolGrants(toolGrantCtx(), &v1.ListToolGrantsRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("ListToolGrants err = %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.GetItems()))
	}
	got := resp.GetItems()[0]
	if got.GetAgentId() != "agent-1" || got.GetToolKey() != "bash" || got.GetGrantedBy() != "user-1" {
		t.Fatalf("grant = %+v", got)
	}
}

func TestToolService_ListToolGrantsEmpty(t *testing.T) {
	svc := newToolGrantTestService(newFakeGrantStore())
	// Known agent with no grants → empty list.
	resp, err := svc.ListToolGrants(toolGrantCtx(), &v1.ListToolGrantsRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("ListToolGrants err = %v", err)
	}
	if len(resp.GetItems()) != 0 {
		t.Fatalf("items = %d, want 0", len(resp.GetItems()))
	}
	// Unknown agent → NotFound (A2 access gate).
	if _, err := svc.ListToolGrants(toolGrantCtx(), &v1.ListToolGrantsRequest{AgentId: "agent-x"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("unknown agent err = %v, want NotFound", err)
	}
}

func TestToolService_DeleteToolGrant(t *testing.T) {
	store := newFakeGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{
		ID: "g1", AgentID: "agent-1", ToolKey: "bash",
	}); err != nil {
		t.Fatal(err)
	}
	svc := newToolGrantTestService(store)
	if _, err := svc.DeleteToolGrant(toolGrantCtx(), &v1.DeleteToolGrantRequest{AgentId: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("DeleteToolGrant err = %v", err)
	}
	has, err := store.HasToolGrant(context.Background(), "agent-1", "bash")
	if err != nil || has {
		t.Fatalf("after delete has = (%v,%v), want (false,nil)", has, err)
	}
	// Idempotent: deleting again must not fail.
	if _, err := svc.DeleteToolGrant(toolGrantCtx(), &v1.DeleteToolGrantRequest{AgentId: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("second DeleteToolGrant err = %v", err)
	}
}
