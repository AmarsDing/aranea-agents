package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- extended stubs for AgentUsecase tests ---

// Compile-time check: stubAgentRepoExt must satisfy AgentReader + AgentWriter.
var (
	_ AgentReader = (*stubAgentRepoExt)(nil)
	_ AgentWriter = (*stubAgentRepoExt)(nil)
)

// stubAgentRepoExt extends stubAgentRepo with controllable behavior for
// Create/Update/Delete operations.
type stubAgentRepoExt struct {
	stubAgentRepo
	createErr      error
	updateErr      error
	deleteErr      error
	toggleErr      error
	toggleResult   Agent
	createdAgent   Agent
	updatedAgent   Agent
	createCalls    int
	updateCalls    int
	deleteCalls    int
	toggleCalls    int
}

func (s *stubAgentRepoExt) CreateAgent(_ context.Context, a Agent) (Agent, error) {
	s.createCalls++
	s.createdAgent = a
	if s.createErr != nil {
		return Agent{}, s.createErr
	}
	return a, nil
}

func (s *stubAgentRepoExt) UpdateAgent(_ context.Context, a Agent) (Agent, error) {
	s.updateCalls++
	s.updatedAgent = a
	if s.updateErr != nil {
		return Agent{}, s.updateErr
	}
	return a, nil
}

func (s *stubAgentRepoExt) DeleteAgent(_ context.Context, _ string) error {
	s.deleteCalls++
	return s.deleteErr
}

func (s *stubAgentRepoExt) ToggleFavorite(_ context.Context, id string) (Agent, error) {
	s.toggleCalls++
	if s.toggleErr != nil {
		return Agent{}, s.toggleErr
	}
	if s.toggleResult.ID != "" {
		return s.toggleResult, nil
	}
	return Agent{ID: id}, nil
}

func newAgentUsecaseWithExt(repo *stubAgentRepoExt) *AgentUsecase {
	return NewAgentUsecase(AgentUsecaseDeps{
		Reader:   repo,
		Writer:   repo,
		Settings: repo,
		Files:    repo,
		Position: repo,
		Tx:       repo,
		Lg:       loggateway.NewNoop(),
	})
}

// --- tests ---

// TestAgentUsecase_Create_Validation_MissingFields verifies that Create
// rejects agents with missing agent_key or display_name.
func TestAgentUsecase_Create_Validation_MissingFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		agent Agent
	}{
		{"missing_agent_key", Agent{DisplayName: "Test"}},
		{"missing_display_name", Agent{AgentKey: "test"}},
		{"both_missing", Agent{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{}
			uc := newAgentUsecaseWithExt(repo)
			_, err := uc.Create(context.Background(), tc.agent)
			if err == nil {
				t.Fatal("expected error for missing required fields")
			}
			ae, ok := apierror.From(err)
			if !ok {
				t.Fatalf("expected apierror, got %T: %v", err, err)
			}
			if ae.Code != apierror.CodeBadRequest {
				t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
			}
			if repo.createCalls != 0 {
				t.Fatalf("CreateAgent should not be called, got %d calls", repo.createCalls)
			}
		})
	}
}

// TestAgentUsecase_Create_Success verifies that Create persists the agent
// within a transaction and returns the hydrated agent.
func TestAgentUsecase_Create_Success(t *testing.T) {
	t.Parallel()
	const agentID = "agent-new"
	repo := &stubAgentRepoExt{
		stubAgentRepo: stubAgentRepo{
			agent: Agent{
				ID:          agentID,
				AgentKey:    "test-agent",
				DisplayName: "Test Agent",
				Provider:    "",
				Model:       "",
				Status:      "active",
			},
		},
	}
	uc := newAgentUsecaseWithExt(repo)
	out, err := uc.Create(context.Background(), Agent{
		ID:          agentID,
		AgentKey:    "test-agent",
		DisplayName: "Test Agent",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.ID != agentID {
		t.Fatalf("expected agent ID %s, got %s", agentID, out.ID)
	}
	if repo.createCalls != 1 {
		t.Fatalf("expected 1 CreateAgent call, got %d", repo.createCalls)
	}
}

// TestAgentUsecase_Update_StatusTransition_Valid verifies that valid agent
// status transitions are accepted by the state machine (AS-FSM-01).
func TestAgentUsecase_Update_StatusTransition_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		fromStatus string
		toStatus   string
	}{
		{"active_to_inactive", "active", "inactive"},
		{"active_to_archived", "active", "archived"},
		{"inactive_to_active", "inactive", "active"},
		{"inactive_to_archived", "inactive", "archived"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{
				stubAgentRepo: stubAgentRepo{
					agent: Agent{
						ID:        "agent-1",
						AgentKey:  "test",
						Status:    tc.fromStatus,
						AgentKind: AgentKindLLM,
						ConfigJSON: EmbedAgentKindInConfigJSON("{}", AgentKindLLM, nil, loggateway.NewNoop()),
					},
				},
			}
			uc := newAgentUsecaseWithExt(repo)
			_, err := uc.Update(context.Background(), "agent-1", Agent{Status: tc.toStatus})
			if err != nil {
				t.Fatalf("expected transition %s → %s to be valid, got error: %v",
					tc.fromStatus, tc.toStatus, err)
			}
			if repo.updateCalls != 1 {
				t.Fatalf("expected 1 UpdateAgent call, got %d", repo.updateCalls)
			}
		})
	}
}

// TestAgentUsecase_Update_StatusTransition_Illegal verifies that illegal
// agent status transitions are rejected (AS-FSM-01).
func TestAgentUsecase_Update_StatusTransition_Illegal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		fromStatus string
		toStatus   string
	}{
		{"archived_to_active", "archived", "active"},
		{"archived_to_inactive", "archived", "inactive"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{
				stubAgentRepo: stubAgentRepo{
					agent: Agent{
						ID:        "agent-1",
						AgentKey:  "test",
						Status:    tc.fromStatus,
						AgentKind: AgentKindLLM,
						ConfigJSON: EmbedAgentKindInConfigJSON("{}", AgentKindLLM, nil, loggateway.NewNoop()),
					},
				},
			}
			uc := newAgentUsecaseWithExt(repo)
			_, err := uc.Update(context.Background(), "agent-1", Agent{Status: tc.toStatus})
			if err == nil {
				t.Fatalf("expected transition %s → %s to be rejected", tc.fromStatus, tc.toStatus)
			}
			ae, ok := apierror.From(err)
			if !ok {
				t.Fatalf("expected apierror, got %T: %v", err, err)
			}
			if ae.Code != apierror.CodeBadRequest {
				t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
			}
			if repo.updateCalls != 0 {
				t.Fatalf("UpdateAgent should not be called for rejected transition, got %d calls", repo.updateCalls)
			}
		})
	}
}

// TestAgentUsecase_Update_AgentKeyImmutable verifies that changing the
// agent_key is rejected.
func TestAgentUsecase_Update_AgentKeyImmutable(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{
		stubAgentRepo: stubAgentRepo{
			agent: Agent{
				ID:        "agent-1",
				AgentKey:  "original",
				Status:    "active",
				AgentKind: AgentKindLLM,
				ConfigJSON: EmbedAgentKindInConfigJSON("{}", AgentKindLLM, nil, loggateway.NewNoop()),
			},
		},
	}
	uc := newAgentUsecaseWithExt(repo)
	_, err := uc.Update(context.Background(), "agent-1", Agent{AgentKey: "changed"})
	if err == nil {
		t.Fatal("expected error for agent_key change")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("UpdateAgent should not be called, got %d calls", repo.updateCalls)
	}
}

// TestAgentUsecase_ForceDelete_BypassesPermissionChecks verifies that
// ForceDelete deletes agents regardless of kind/readonly status.
func TestAgentUsecase_ForceDelete_BypassesPermissionChecks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		agent Agent
	}{
		{"system_builtin", Agent{ID: "agent-1", Kind: "system_builtin"}},
		{"ecosystem_preset", Agent{ID: "agent-2", Kind: "ecosystem_preset"}},
		{"readonly", Agent{ID: "agent-3", Kind: "user", Readonly: true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &stubAgentRepoExt{
				stubAgentRepo: stubAgentRepo{agent: tc.agent},
			}
			uc := newAgentUsecaseWithExt(repo)
			err := uc.ForceDelete(context.Background(), tc.agent.ID)
			if err != nil {
				t.Fatalf("ForceDelete should bypass permission checks, got error: %v", err)
			}
			if repo.deleteCalls != 1 {
				t.Fatalf("expected 1 DeleteAgent call, got %d", repo.deleteCalls)
			}
		})
	}
}

// TestAgentUsecase_ForceDelete_EmptyID verifies that ForceDelete rejects
// empty IDs.
func TestAgentUsecase_ForceDelete_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{}
	uc := newAgentUsecaseWithExt(repo)
	err := uc.ForceDelete(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("DeleteAgent should not be called, got %d calls", repo.deleteCalls)
	}
}

// TestAgentUsecase_ToggleFavorite_Success verifies that ToggleFavorite
// delegates to the writer and returns the hydrated agent.
func TestAgentUsecase_ToggleFavorite_Success(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{
		stubAgentRepo: stubAgentRepo{
			agent: Agent{ID: "agent-1", AgentKey: "test", Status: "active"},
		},
		toggleResult: Agent{ID: "agent-1", AgentKey: "test", Status: "active"},
	}
	uc := newAgentUsecaseWithExt(repo)
	out, err := uc.ToggleFavorite(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.ID != "agent-1" {
		t.Fatalf("expected agent ID agent-1, got %s", out.ID)
	}
	if repo.toggleCalls != 1 {
		t.Fatalf("expected 1 ToggleFavorite call, got %d", repo.toggleCalls)
	}
}

// TestAgentUsecase_ToggleFavorite_NotFound verifies that ToggleFavorite
// translates ErrNotFound to an apierror.NotFound.
func TestAgentUsecase_ToggleFavorite_NotFound(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{
		toggleErr: shared.ErrNotFound,
	}
	uc := newAgentUsecaseWithExt(repo)
	_, err := uc.ToggleFavorite(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeNotFound {
		t.Fatalf("expected code %s, got %s", apierror.CodeNotFound, ae.Code)
	}
}

// TestAgentUsecase_ToggleFavorite_EmptyID verifies that ToggleFavorite
// rejects empty IDs.
func TestAgentUsecase_ToggleFavorite_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{}
	uc := newAgentUsecaseWithExt(repo)
	_, err := uc.ToggleFavorite(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
	}
	if repo.toggleCalls != 0 {
		t.Fatalf("ToggleFavorite should not be called, got %d calls", repo.toggleCalls)
	}
}

// TestAgentUsecase_Delete_NotFound verifies that Delete returns NotFound
// when the agent doesn't exist.
func TestAgentUsecase_Delete_NotFound(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{
		stubAgentRepo: stubAgentRepo{agent: Agent{ID: "different-id"}},
	}
	uc := newAgentUsecaseWithExt(repo)
	err := uc.Delete(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	if !errors.Is(err, ErrNotFound) && !errors.Is(err, shared.ErrNotFound) {
		// GetAgentByID returns ErrNotFound (defined in agent_usecase_kind_test.go stub)
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("DeleteAgent should not be called, got %d calls", repo.deleteCalls)
	}
}

// TestAgentUsecase_Delete_EmptyID verifies that Delete rejects empty IDs.
func TestAgentUsecase_Delete_EmptyID(t *testing.T) {
	t.Parallel()
	repo := &stubAgentRepoExt{}
	uc := newAgentUsecaseWithExt(repo)
	err := uc.Delete(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("DeleteAgent should not be called, got %d calls", repo.deleteCalls)
	}
}
