package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/tool/v1"
	biztool "aranea-agents/internal/biz/tool"
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
	return NewToolService(uc, nil)
}

func TestToolService_ListToolGrants(t *testing.T) {
	store := newFakeGrantStore()
	if err := store.CreateToolGrant(context.Background(), biztool.ToolGrant{
		ID: "g1", AgentID: "agent-1", ToolKey: "bash", GrantedBy: "user-1", CreatedAt: "2026-07-20T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	svc := newToolGrantTestService(store)
	resp, err := svc.ListToolGrants(context.Background(), &v1.ListToolGrantsRequest{AgentId: "agent-1"})
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
	resp, err := svc.ListToolGrants(context.Background(), &v1.ListToolGrantsRequest{AgentId: "agent-x"})
	if err != nil {
		t.Fatalf("ListToolGrants err = %v", err)
	}
	if len(resp.GetItems()) != 0 {
		t.Fatalf("items = %d, want 0", len(resp.GetItems()))
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
	if _, err := svc.DeleteToolGrant(context.Background(), &v1.DeleteToolGrantRequest{AgentId: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("DeleteToolGrant err = %v", err)
	}
	has, err := store.HasToolGrant(context.Background(), "agent-1", "bash")
	if err != nil || has {
		t.Fatalf("after delete has = (%v,%v), want (false,nil)", has, err)
	}
	// Idempotent: deleting again must not fail.
	if _, err := svc.DeleteToolGrant(context.Background(), &v1.DeleteToolGrantRequest{AgentId: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("second DeleteToolGrant err = %v", err)
	}
}
