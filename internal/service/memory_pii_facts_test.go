package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// fakePIIFactsDeps records the params of the last ListPIIFlaggedFacts call so
// the agentId pass-through can be asserted.
type fakePIIFactsDeps struct {
	biz.MemoryAdminDeps // nil-embedded; only ListPIIFlaggedFacts is callable

	calls      int
	gotAgentID string
}

func (f *fakePIIFactsDeps) ListPIIFlaggedFacts(_ context.Context, _, _, agentID string, _, _ int32) ([][]byte, int32, error) {
	f.calls++
	f.gotAgentID = agentID
	return nil, 0, nil
}

// TestListPIIFlaggedFacts_PassesAgentIDFromProtoField pins the 2026-10-19 fix:
// agent_id is a declared proto field, so the generated TS/HTTP client actually
// serializes it — before the field existed the client silently dropped the
// param and the trust-tab PII agent filter was a no-op. Bare HTTP query
// fallback is covered by the memoryHTTPQuery path (unchanged).
func TestListPIIFlaggedFacts_PassesAgentIDFromProtoField(t *testing.T) {
	deps := &fakePIIFactsDeps{}
	admin := biz.NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	svc := NewMemoryService(MemoryServiceConfig{Admin: admin, Logger: loggateway.NewNoop()})

	ctx := workspace.WithSystemWorkspace(context.Background())
	if _, err := svc.ListPIIFlaggedFacts(ctx, &v1.ListPIIFlaggedFactsRequest{AgentId: "agent-7", Limit: 10}); err != nil {
		t.Fatalf("ListPIIFlaggedFacts: %v", err)
	}
	if deps.calls != 1 {
		t.Fatalf("ListPIIFlaggedFacts calls = %d, want 1", deps.calls)
	}
	if deps.gotAgentID != "agent-7" {
		t.Errorf("agentID = %q, want agent-7", deps.gotAgentID)
	}
}
