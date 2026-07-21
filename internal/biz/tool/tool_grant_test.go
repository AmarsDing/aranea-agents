package tool

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeGrantStore struct {
	grants  map[string]ToolGrant
	hasErr  error
	createN int
}

func newFakeGrantStore() *fakeGrantStore {
	return &fakeGrantStore{grants: make(map[string]ToolGrant)}
}

func (f *fakeGrantStore) key(agentID, toolKey string) string { return agentID + "|" + toolKey }

func (f *fakeGrantStore) HasToolGrant(_ context.Context, agentID, toolKey string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	_, ok := f.grants[f.key(agentID, toolKey)]
	return ok, nil
}

func (f *fakeGrantStore) ListToolGrants(_ context.Context, agentID string) ([]ToolGrant, error) {
	var out []ToolGrant
	for _, g := range f.grants {
		if g.AgentID == agentID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeGrantStore) CreateToolGrant(_ context.Context, grant ToolGrant) error {
	f.createN++
	f.grants[f.key(grant.AgentID, grant.ToolKey)] = grant
	return nil
}

func (f *fakeGrantStore) DeleteToolGrant(_ context.Context, agentID, toolKey string) error {
	delete(f.grants, f.key(agentID, toolKey))
	return nil
}

func TestToolUsecase_GrantRoundTrip(t *testing.T) {
	t.Parallel()
	store := newFakeGrantStore()
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolGrantStore(store))
	ctx := context.Background()

	if u.HasToolGrant(ctx, "agent-1", "bash") {
		t.Fatal("unexpected grant before create")
	}
	if err := u.GrantTool(ctx, "agent-1", "bash", "user-1"); err != nil {
		t.Fatalf("GrantTool: %v", err)
	}
	if !u.HasToolGrant(ctx, "agent-1", "bash") {
		t.Fatal("expected grant after GrantTool")
	}
	if store.createN != 1 {
		t.Fatalf("expected 1 store create call, got %d", store.createN)
	}
}

func TestToolUsecase_HasToolGrant_FailClosedOnStoreError(t *testing.T) {
	t.Parallel()
	store := newFakeGrantStore()
	store.hasErr = errors.New("db down")
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolGrantStore(store))
	// DB failure must degrade to "no grant" (confirmation prompt still
	// appears) — the safe direction — never to silently allowed.
	if u.HasToolGrant(context.Background(), "agent-1", "bash") {
		t.Fatal("HasToolGrant must return false on store error (fail-closed)")
	}
}

func TestToolUsecase_GrantWithoutStore(t *testing.T) {
	t.Parallel()
	u := NewToolUsecase(nil, nil, loggateway.NewNoop())
	ctx := context.Background()
	if u.HasToolGrant(ctx, "agent-1", "bash") {
		t.Fatal("HasToolGrant without store must be false")
	}
	if err := u.GrantTool(ctx, "agent-1", "bash", "user-1"); err == nil {
		t.Fatal("GrantTool without store must fail")
	}
}

func TestToolUsecase_RevokeToolGrant(t *testing.T) {
	t.Parallel()
	store := newFakeGrantStore()
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolGrantStore(store))
	ctx := context.Background()

	if err := u.GrantTool(ctx, "agent-1", "bash", "user-1"); err != nil {
		t.Fatalf("GrantTool: %v", err)
	}
	if err := u.RevokeToolGrant(ctx, "agent-1", "bash"); err != nil {
		t.Fatalf("RevokeToolGrant: %v", err)
	}
	if u.HasToolGrant(ctx, "agent-1", "bash") {
		t.Fatal("expected grant removed after RevokeToolGrant")
	}
}

func TestToolUsecase_ListToolGrants(t *testing.T) {
	t.Parallel()
	store := newFakeGrantStore()
	u := NewToolUsecase(nil, nil, loggateway.NewNoop(), WithToolGrantStore(store))
	ctx := context.Background()

	if err := u.GrantTool(ctx, "agent-1", "bash", "user-1"); err != nil {
		t.Fatalf("GrantTool: %v", err)
	}
	if err := u.GrantTool(ctx, "agent-1", "file_save", "user-1"); err != nil {
		t.Fatalf("GrantTool: %v", err)
	}
	if err := u.GrantTool(ctx, "agent-2", "bash", "user-1"); err != nil {
		t.Fatalf("GrantTool: %v", err)
	}
	grants, err := u.ListToolGrants(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListToolGrants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants for agent-1, got %d", len(grants))
	}
}
