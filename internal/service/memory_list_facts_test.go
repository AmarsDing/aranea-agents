package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// fakeListFactRowsDeps records the params of the last ListFactRows call so
// the F1 agent_id pass-through can be asserted.
type fakeListFactRowsDeps struct {
	biz.MemoryAdminDeps // nil-embedded; only ListFactRows is callable

	calls        int
	gotScopeType string
	gotScopeID   string
	gotAgentID   string
}

func (f *fakeListFactRowsDeps) ListFactRows(_ context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	f.calls++
	f.gotScopeType = scopeType
	f.gotScopeID = scopeID
	f.gotAgentID = agentID
	return nil, 0, 0, 0, nil
}

// TestListMemoryFacts_PassesAgentIDThrough pins the F1 contract: the agent_id
// query param reaches the admin store so the memory-center L3 browse tab shows
// "this agent's facts" across all scopes, matching the panorama card count.
func TestListMemoryFacts_PassesAgentIDThrough(t *testing.T) {
	deps := &fakeListFactRowsDeps{}
	admin := biz.NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	svc := NewMemoryService(MemoryServiceConfig{Admin: admin, Logger: loggateway.NewNoop()})

	ctx := workspace.WithSystemWorkspace(context.Background())
	if _, err := svc.ListMemoryFacts(ctx, &v1.ListMemoryFactsRequest{AgentId: "agent-9", Limit: 10}); err != nil {
		t.Fatalf("ListMemoryFacts: %v", err)
	}
	if deps.calls != 1 {
		t.Fatalf("ListFactRows calls = %d, want 1", deps.calls)
	}
	if deps.gotAgentID != "agent-9" {
		t.Errorf("agentID = %q, want agent-9", deps.gotAgentID)
	}
	if deps.gotScopeType != "" || deps.gotScopeID != "" {
		t.Errorf("scope = %q/%q, want empty/empty (cross-scope agent filter)", deps.gotScopeType, deps.gotScopeID)
	}
}
