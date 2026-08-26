package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// stubFactPendingStore implements biz.MemoryFactPendingStore for service tests.
type stubFactPendingStore struct {
	recs []biz.MemoryFactPendingRecord
}

func (s *stubFactPendingStore) InsertPending(context.Context, biz.MemoryFactPendingRecord) error {
	return nil
}

func (s *stubFactPendingStore) GetPending(context.Context, string) (biz.MemoryFactPendingRecord, bool, error) {
	return biz.MemoryFactPendingRecord{}, false, nil
}

func (s *stubFactPendingStore) ListPending(_ context.Context, agentID, status string, _ int) ([]biz.MemoryFactPendingRecord, error) {
	out := make([]biz.MemoryFactPendingRecord, 0, len(s.recs))
	for _, r := range s.recs {
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *stubFactPendingStore) MarkDecided(context.Context, string, string, string, int64) (bool, error) {
	return false, nil
}

func TestListMemoryFactPendings(t *testing.T) {
	pendings := &stubFactPendingStore{recs: []biz.MemoryFactPendingRecord{
		{
			ID: "mfp-1", AgentID: "a1", FactKey: "fact-old",
			Verdict: biz.MemoryFactPendingVerdictUpdate,
			ProposedBody: "新表述", PriorBody: "旧表述",
			AdjudicatorReason: "adjudicated_update",
			Status:            biz.MemoryFactPendingStatusPending,
			CreatedAt:         1787702400,
		},
		{
			ID: "mfp-2", AgentID: "a1", Verdict: biz.MemoryFactPendingVerdictDelete,
			ProposedBody: "", PriorBody: "待删表述",
			Status: biz.MemoryFactPendingStatusApproved, Approver: "twinmonitor:7",
			CreatedAt: 1787702401, DecidedAt: 1787702500,
		},
	}}
	newSvc := func(store biz.MemoryFactPendingStore) *MemoryService {
		return NewMemoryService(MemoryServiceConfig{
			Admin:            stubAdminUsecase(),
			AgentUC:          stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-1"}},
			FactPendingStore: store,
		})
	}

	t.Run("store_not_wired", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{
			Admin:   stubAdminUsecase(),
			AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-1"}},
		})
		_, err := s.ListMemoryFactPendings(userCtx("ws-1"), &v1.ListMemoryFactPendingsRequest{AgentId: "a1"})
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("expected INTERNAL when pending store missing, got %v", err)
		}
	})

	t.Run("cross_workspace_forbidden", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{
			Admin:            stubAdminUsecase(),
			AgentUC:          stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-other"}},
			FactPendingStore: pendings,
		})
		_, err := s.ListMemoryFactPendings(userCtx("ws-1"), &v1.ListMemoryFactPendingsRequest{AgentId: "a1"})
		// IDOR 反枚举设计：跨工作区统一 NOT_FOUND（memory_scope.go assertAgentMemoryAccess）。
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeNotFound {
			t.Fatalf("expected NOT_FOUND cross-workspace, got %v", err)
		}
	})

	t.Run("lists_all_statuses", func(t *testing.T) {
		resp, err := newSvc(pendings).ListMemoryFactPendings(userCtx("ws-1"), &v1.ListMemoryFactPendingsRequest{AgentId: "a1"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(resp.Items) != 2 {
			t.Fatalf("want 2 items, got %d", len(resp.Items))
		}
		it := resp.Items[0]
		if it.Verdict != "UPDATE" || it.ProposedBody != "新表述" || it.PriorBody != "旧表述" || it.AdjudicatorReason == "" {
			t.Fatalf("mapping mismatch: %+v", it)
		}
	})

	t.Run("filters_pending_status", func(t *testing.T) {
		resp, err := newSvc(pendings).ListMemoryFactPendings(userCtx("ws-1"), &v1.ListMemoryFactPendingsRequest{AgentId: "a1", Status: "pending"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(resp.Items) != 1 || resp.Items[0].Id != "mfp-1" {
			t.Fatalf("status filter mismatch: %+v", resp.Items)
		}
	})
}
