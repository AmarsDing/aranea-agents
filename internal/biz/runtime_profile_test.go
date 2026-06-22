package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

// mockRuntimeProfileRepo is an in-memory RuntimeProfileReadWriter for testing.
type mockRuntimeProfileRepo struct {
	profiles map[string]RuntimeProfile
	err      error
}

func newMockRuntimeProfileRepo() *mockRuntimeProfileRepo {
	return &mockRuntimeProfileRepo{profiles: make(map[string]RuntimeProfile)}
}

func (m *mockRuntimeProfileRepo) List(_ context.Context, agentID string, activeOnly bool) ([]RuntimeProfile, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []RuntimeProfile
	for _, p := range m.profiles {
		if p.AgentID != agentID {
			continue
		}
		if activeOnly && !p.IsActive {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (m *mockRuntimeProfileRepo) GetByID(_ context.Context, id string) (RuntimeProfile, error) {
	if m.err != nil {
		return RuntimeProfile{}, m.err
	}
	p, ok := m.profiles[id]
	if !ok {
		return RuntimeProfile{}, apierror.NotFound(apierror.DomainRuntimeProfile, "runtime profile not found")
	}
	return p, nil
}

func (m *mockRuntimeProfileRepo) GetActive(_ context.Context, agentID string) (*RuntimeProfile, error) {
	if m.err != nil {
		return nil, m.err
	}
	var found *RuntimeProfile
	for _, p := range m.profiles {
		if p.AgentID != agentID || !p.IsActive {
			continue
		}
		if found == nil || p.Priority > found.Priority {
			cp := p
			found = &cp
		}
	}
	return found, nil
}

func (m *mockRuntimeProfileRepo) Create(_ context.Context, p RuntimeProfile) (RuntimeProfile, error) {
	if m.err != nil {
		return RuntimeProfile{}, m.err
	}
	m.profiles[p.ID] = p
	return p, nil
}

func (m *mockRuntimeProfileRepo) Update(_ context.Context, p RuntimeProfile) (RuntimeProfile, error) {
	if m.err != nil {
		return RuntimeProfile{}, m.err
	}
	if _, ok := m.profiles[p.ID]; !ok {
		return RuntimeProfile{}, apierror.NotFound(apierror.DomainRuntimeProfile, "runtime profile not found")
	}
	m.profiles[p.ID] = p
	return p, nil
}

func (m *mockRuntimeProfileRepo) Delete(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.profiles, id)
	return nil
}

func (m *mockRuntimeProfileRepo) SetActive(_ context.Context, id string, active bool) (RuntimeProfile, error) {
	if m.err != nil {
		return RuntimeProfile{}, m.err
	}
	p, ok := m.profiles[id]
	if !ok {
		return RuntimeProfile{}, apierror.NotFound(apierror.DomainRuntimeProfile, "runtime profile not found")
	}
	p.IsActive = active
	m.profiles[id] = p
	return p, nil
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestRuntimeProfileUsecase_Create_Validation(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	tests := []struct {
		name    string
		profile RuntimeProfile
		wantErr bool
	}{
		{
			name:    "missing agent_id",
			profile: RuntimeProfile{Name: "p1"},
			wantErr: true,
		},
		{
			name:    "missing name",
			profile: RuntimeProfile{AgentID: "agent-1"},
			wantErr: true,
		},
		{
			name:    "valid",
			profile: RuntimeProfile{AgentID: "agent-1", Name: "p1"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.Create(context.Background(), tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRuntimeProfileUsecase_Create_SingleActiveInvariant(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	// Create first active profile
	p1, err := uc.Create(context.Background(), RuntimeProfile{
		AgentID:  "agent-1",
		Name:     "p1",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create p1: %v", err)
	}

	// Create second active profile — should deactivate p1
	p2, err := uc.Create(context.Background(), RuntimeProfile{
		AgentID:  "agent-1",
		Name:     "p2",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	// Verify p1 is deactivated
	got1, err := repo.GetByID(context.Background(), p1.ID)
	if err != nil {
		t.Fatalf("GetByID p1: %v", err)
	}
	if got1.IsActive {
		t.Error("p1 should be deactivated after p2 becomes active")
	}

	// Verify p2 is active
	got2, err := repo.GetByID(context.Background(), p2.ID)
	if err != nil {
		t.Fatalf("GetByID p2: %v", err)
	}
	if !got2.IsActive {
		t.Error("p2 should be active")
	}
}

func TestRuntimeProfileUsecase_SetActive_DeactivatesOthers(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	// Create two inactive profiles
	p1, _ := uc.Create(context.Background(), RuntimeProfile{AgentID: "a1", Name: "p1"})
	p2, _ := uc.Create(context.Background(), RuntimeProfile{AgentID: "a1", Name: "p2"})

	// Activate p1
	if _, err := uc.SetActive(context.Background(), p1.ID, true); err != nil {
		t.Fatalf("SetActive p1: %v", err)
	}

	// Activate p2 — should deactivate p1
	if _, err := uc.SetActive(context.Background(), p2.ID, true); err != nil {
		t.Fatalf("SetActive p2: %v", err)
	}

	got1, _ := repo.GetByID(context.Background(), p1.ID)
	if got1.IsActive {
		t.Error("p1 should be deactivated after p2 activation")
	}

	got2, _ := repo.GetByID(context.Background(), p2.ID)
	if !got2.IsActive {
		t.Error("p2 should be active")
	}
}

func TestRuntimeProfileUsecase_SetActive_Deactivate(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	p1, _ := uc.Create(context.Background(), RuntimeProfile{AgentID: "a1", Name: "p1", IsActive: true})

	// Deactivate
	if _, err := uc.SetActive(context.Background(), p1.ID, false); err != nil {
		t.Fatalf("SetActive p1 false: %v", err)
	}

	got, _ := repo.GetByID(context.Background(), p1.ID)
	if got.IsActive {
		t.Error("p1 should be deactivated")
	}
}

func TestRuntimeProfileUsecase_ResolveForAgent_NoProfile(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	prof, err := uc.ResolveForAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("ResolveForAgent: %v", err)
	}
	if prof != nil {
		t.Errorf("expected nil profile, got %v", prof)
	}
}

func TestRuntimeProfileUsecase_ResolveForAgent_WithActiveProfile(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	_, _ = uc.Create(context.Background(), RuntimeProfile{
		AgentID:  "agent-1",
		Name:     "p1",
		IsActive: true,
		PromptConfig: PromptConfig{
			Instruction: "test-instruction",
		},
	})

	prof, err := uc.ResolveForAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("ResolveForAgent: %v", err)
	}
	if prof == nil {
		t.Fatal("expected profile, got nil")
	}
	if prof.PromptConfig.Instruction != "test-instruction" {
		t.Errorf("expected instruction 'test-instruction', got %q", prof.PromptConfig.Instruction)
	}
}

func TestRuntimeProfileUsecase_ResolveForAgent_EmptyAgentID(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	prof, err := uc.ResolveForAgent(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveForAgent: %v", err)
	}
	if prof != nil {
		t.Errorf("expected nil for empty agentID, got %v", prof)
	}
}

func TestRuntimeProfileUsecase_Get_Validation(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	_, err := uc.Get(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestRuntimeProfileUsecase_Delete_Validation(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	err := uc.Delete(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestRuntimeProfileUsecase_Update_CannotChangeAgentID(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	uc := NewRuntimeProfileUsecase(repo)

	p, _ := uc.Create(context.Background(), RuntimeProfile{AgentID: "a1", Name: "p1"})

	// Try to change agent_id while activating
	_, err := uc.Update(context.Background(), RuntimeProfile{
		ID:       p.ID,
		AgentID:  "a2",
		Name:     "p1",
		IsActive: true,
	})
	if err == nil {
		t.Error("expected error when changing agent_id")
	}
}

func TestRuntimeProfileUsecase_Update_RepoError(t *testing.T) {
	repo := newMockRuntimeProfileRepo()
	repo.err = errors.New("db error")
	uc := NewRuntimeProfileUsecase(repo)

	_, err := uc.Update(context.Background(), RuntimeProfile{ID: "x", AgentID: "a1", Name: "p1"})
	if err == nil {
		t.Error("expected error from repo")
	}
}

func TestRuntimeProfile_PolicyJSON(t *testing.T) {
	p := RuntimeProfile{
		PromptConfig:     PromptConfig{Instruction: "i", SystemPrompt: "s"},
		ToolPolicy:       ToolPolicy{Include: []string{"t1"}},
		SkillPolicy:      SkillPolicy{Include: []string{"s1"}},
		KnowledgePolicy:  KnowledgePolicy{Indexes: []string{"k1"}},
		WorkspacePolicy:  WorkspacePolicy{Workdir: "/tmp"},
		CredentialPolicy: CredentialPolicy{AllowedRefs: []string{"c1"}},
		IsolationPolicy:  IsolationPolicy{Mode: "strict"},
		ExtraModelConfig: map[string]any{"temperature": 0.7},
	}

	jsonMap, err := p.PolicyJSON()
	if err != nil {
		t.Fatalf("PolicyJSON: %v", err)
	}

	required := []string{
		"prompt_config", "tool_policy", "skill_policy", "knowledge_policy",
		"workspace_policy", "credential_policy", "isolation_policy", "extra_model_config",
	}
	for _, key := range required {
		if _, ok := jsonMap[key]; !ok {
			t.Errorf("missing key %q in PolicyJSON output", key)
		}
	}
}

func TestRuntimeProfile_PolicyJSON_NilExtraModel(t *testing.T) {
	p := RuntimeProfile{}
	jsonMap, err := p.PolicyJSON()
	if err != nil {
		t.Fatalf("PolicyJSON: %v", err)
	}
	if jsonMap["extra_model_config"] != "{}" {
		t.Errorf("expected empty JSON object for nil ExtraModelConfig, got %q", jsonMap["extra_model_config"])
	}
}
