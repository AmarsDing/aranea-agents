package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubAdminUsecase returns a non-nil usecase whose admin store is not wired,
// letting tests reach service-level validation and the biz wiring guard.
func stubAdminUsecase() *biz.MemoryAdminUsecase {
	return biz.NewMemoryAdminUsecase(nil, &biz.MemoryUsecase{}, nil, nil, loggateway.NewNoop())
}

func TestListMemoryEpisodes_Validation(t *testing.T) {
	t.Run("service_admin_not_wired", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{})
		_, err := s.ListMemoryEpisodes(context.Background(), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("expected INTERNAL when admin usecase missing, got %v", err)
		}
	})

	t.Run("agent_id_required", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{Admin: stubAdminUsecase()})
		_, err := s.ListMemoryEpisodes(context.Background(), &v1.ListMemoryEpisodesRequest{AgentId: "  "})
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeBadRequest {
			t.Fatalf("expected BAD_REQUEST for blank agent_id, got %v", err)
		}
	})

	t.Run("delegates_to_biz", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{Admin: stubAdminUsecase()})
		_, err := s.ListMemoryEpisodes(context.Background(), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		// biz guard: admin store not wired → INTERNAL (proves handler reached biz layer)
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("expected INTERNAL from biz wiring guard, got %v", err)
		}
	})
}
