package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

type stubAgentCatalog struct {
	agent biz.Agent
	err   error
}

func (s stubAgentCatalog) Get(_ context.Context, id string) (biz.Agent, error) {
	if s.err != nil {
		return biz.Agent{}, s.err
	}
	if s.agent.ID != "" && s.agent.ID != id {
		return biz.Agent{}, apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	a := s.agent
	if a.ID == "" {
		a.ID = id
	}
	return a, nil
}

func userCtx(ws string) context.Context {
	return auth.NewContext(workspace.WithContext(context.Background(), ws), &auth.Auth{UserID: 7, Access: "user"})
}

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
		_, err := s.ListMemoryEpisodes(workspace.WithSystemWorkspace(context.Background()), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		// biz guard: admin store not wired → INTERNAL (proves handler reached biz layer)
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("expected INTERNAL from biz wiring guard, got %v", err)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{Admin: stubAdminUsecase(), AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-1"}}})
		_, err := s.ListMemoryEpisodes(workspace.WithContext(context.Background(), "ws-1"), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		if err != auth.ErrUnauthorized {
			t.Fatalf("expected unauthorized, got %v", err)
		}
	})

	t.Run("cross_tenant_not_found", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{
			Admin:   stubAdminUsecase(),
			AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-other"}},
		})
		_, err := s.ListMemoryEpisodes(userCtx("ws-1"), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeNotFound {
			t.Fatalf("expected NOT_FOUND for cross-tenant agent, got %v", err)
		}
	})

	t.Run("same_tenant_reaches_biz", func(t *testing.T) {
		s := NewMemoryService(MemoryServiceConfig{
			Admin:   stubAdminUsecase(),
			AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-1"}},
		})
		_, err := s.ListMemoryEpisodes(userCtx("ws-1"), &v1.ListMemoryEpisodesRequest{AgentId: "a1"})
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeInternal {
			t.Fatalf("expected INTERNAL from biz wiring guard, got %v", err)
		}
	})
}

func TestGetMemoryLayerOverview_AgentACL(t *testing.T) {
	s := NewMemoryService(MemoryServiceConfig{
		Admin:   stubAdminUsecase(),
		AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-other"}},
	})
	_, err := s.GetMemoryLayerOverview(userCtx("ws-1"), &v1.GetMemoryLayerOverviewRequest{AgentId: "a1"})
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestMemoryCenterAgentACL_BrowseAndTrust(t *testing.T) {
	cross := NewMemoryService(MemoryServiceConfig{
		Admin:   stubAdminUsecase(),
		AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-other"}},
	})
	same := NewMemoryService(MemoryServiceConfig{
		Admin:   stubAdminUsecase(),
		AgentUC: stubAgentCatalog{agent: biz.Agent{ID: "a1", WorkspaceID: "ws-1"}},
	})
	ctx := userCtx("ws-1")

	type call func(*MemoryService) error
	cases := []struct {
		name string
		fn   call
	}{
		{"ListMemoryFacts", func(s *MemoryService) error {
			_, err := s.ListMemoryFacts(ctx, &v1.ListMemoryFactsRequest{AgentId: "a1"})
			return err
		}},
		{"ListConflictingFacts", func(s *MemoryService) error {
			_, err := s.ListConflictingFacts(ctx, &v1.ListConflictingFactsRequest{AgentId: "a1"})
			return err
		}},
		{"GetAgentIdentity", func(s *MemoryService) error {
			_, err := s.GetAgentIdentity(ctx, &v1.GetAgentIdentityRequest{AgentId: "a1"})
			return err
		}},
		{"GetAgentStrategy", func(s *MemoryService) error {
			_, err := s.GetAgentStrategy(ctx, &v1.GetAgentStrategyRequest{AgentId: "a1"})
			return err
		}},
		{"ListEvolutionProposals", func(s *MemoryService) error {
			_, err := s.ListEvolutionProposals(ctx, &v1.ListEvolutionProposalsRequest{AgentId: "a1"})
			return err
		}},
		{"ListEvolutionEvents", func(s *MemoryService) error {
			_, err := s.ListEvolutionEvents(ctx, &v1.ListEvolutionEventsRequest{AgentId: "a1"})
			return err
		}},
		{"GetEvolutionMetrics", func(s *MemoryService) error {
			_, err := s.GetEvolutionMetrics(ctx, &v1.GetEvolutionMetricsRequest{AgentId: "a1"})
			return err
		}},
		{"AppendEvolutionEvent", func(s *MemoryService) error {
			_, err := s.AppendEvolutionEvent(ctx, &v1.AppendEvolutionEventRequest{AgentId: "a1", EventKind: "proposal_approved"})
			return err
		}},
		{"ListL0Snapshots", func(s *MemoryService) error {
			_, err := s.ListL0Snapshots(ctx, &v1.ListL0SnapshotsRequest{SessionId: "s1", AgentId: "a1"})
			return err
		}},
		{"ListL1Tasks", func(s *MemoryService) error {
			_, err := s.ListL1Tasks(ctx, &v1.ListL1TasksRequest{SessionId: "s1", AgentId: "a1"})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_cross_tenant", func(t *testing.T) {
			err := tc.fn(cross)
			ae, ok := apierror.From(err)
			if !ok || ae.Code != apierror.CodeNotFound {
				t.Fatalf("expected NOT_FOUND, got %v", err)
			}
		})
		t.Run(tc.name+"_same_tenant_reaches_biz", func(t *testing.T) {
			err := tc.fn(same)
			ae, ok := apierror.From(err)
			if !ok || ae.Code != apierror.CodeInternal {
				t.Fatalf("expected INTERNAL from biz wiring guard, got %v", err)
			}
		})
	}
}
