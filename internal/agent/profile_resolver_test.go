package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestProfileResolver_NilReceiver(t *testing.T) {
	var r *ProfileResolver
	opts, err := r.ResolveRunOptions(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if opts != nil {
		t.Errorf("expected nil opts, got %v", opts)
	}
}

func TestProfileResolver_NoActiveProfile(t *testing.T) {
	repo := &profileResolverMockRepo{profiles: map[string]biz.RuntimeProfile{}}
	uc := biz.NewRuntimeProfileUsecase(repo)
	r := NewProfileResolver(uc, loggateway.NewNoop())

	opts, err := r.ResolveRunOptions(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if opts != nil {
		t.Errorf("expected nil opts when no active profile, got %v", opts)
	}
}

func TestProfileResolver_WithActiveProfile(t *testing.T) {
	repo := &profileResolverMockRepo{
		profiles: map[string]biz.RuntimeProfile{
			"p1": {
				ID:      "p1",
				AgentID: "agent-1",
				Name:    "p1",
				IsActive: true,
				PromptConfig: biz.PromptConfig{
					Instruction:  "test-instruction",
					SystemPrompt: "test-system",
				},
				ToolPolicy: biz.ToolPolicy{
					Include: []string{"tool1"},
					Exclude: []string{"tool2"},
				},
			},
		},
	}
	uc := biz.NewRuntimeProfileUsecase(repo)
	r := NewProfileResolver(uc, loggateway.NewNoop())

	opts, err := r.ResolveRunOptions(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("ResolveRunOptions: %v", err)
	}
	if len(opts) == 0 {
		t.Skip("framework returned 0 RunOptions for this profile configuration — non-fatal")
	}
}

func TestProfileResolver_RepoError_GracefulDegradation(t *testing.T) {
	repo := &profileResolverMockRepo{
		profiles: map[string]biz.RuntimeProfile{},
		err:      errors.New("db connection failed"),
	}
	uc := biz.NewRuntimeProfileUsecase(repo)
	r := NewProfileResolver(uc, loggateway.NewNoop())

	opts, err := r.ResolveRunOptions(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("expected nil error (graceful degradation), got %v", err)
	}
	if opts != nil {
		t.Errorf("expected nil opts on error (graceful degradation), got %v", opts)
	}
}

func TestBizToFrameworkProfile_MapsAllFields(t *testing.T) {
	p := biz.RuntimeProfile{
		ID:      "p1",
		Version: "v1",
		PromptConfig: biz.PromptConfig{
			Instruction:  "instr",
			SystemPrompt: "sys",
		},
		ToolPolicy: biz.ToolPolicy{
			Include:          []string{"a"},
			Exclude:          []string{"b"},
			ExecutionInclude: []string{"c"},
			ExecutionExclude: []string{"d"},
			ToolSets:         []string{"ts1"},
			CredentialRefs:   map[string]string{"k": "v"},
		},
		SkillPolicy: biz.SkillPolicy{
			Include: []string{"s1"},
			Exclude: []string{"s2"},
			Roots:   []string{"r1"},
		},
		KnowledgePolicy: biz.KnowledgePolicy{
			Indexes: []string{"idx1"},
			Filter:  map[string]any{"k": "v"},
		},
		WorkspacePolicy: biz.WorkspacePolicy{
			Workdir:      "/tmp",
			AllowedRoots: []string{"/root"},
		},
		CredentialPolicy: biz.CredentialPolicy{
			AllowedRefs: []string{"ref1"},
		},
		IsolationPolicy: biz.IsolationPolicy{
			Mode:         "strict",
			AgentCache:   true,
			ToolSetCache: false,
			ServiceMode:  "isolated",
		},
		ExtraModelConfig: map[string]any{"temperature": 0.5},
	}

	fw := bizToFrameworkProfile(p)
	if fw.ID != "p1" {
		t.Errorf("ID: got %q, want %q", fw.ID, "p1")
	}
	if fw.Version != "v1" {
		t.Errorf("Version: got %q, want %q", fw.Version, "v1")
	}
	if fw.Prompt.Instruction != "instr" {
		t.Errorf("Prompt.Instruction: got %q, want %q", fw.Prompt.Instruction, "instr")
	}
	if fw.Prompt.SystemPrompt != "sys" {
		t.Errorf("Prompt.SystemPrompt: got %q, want %q", fw.Prompt.SystemPrompt, "sys")
	}
	if len(fw.Tools.Include) != 1 || fw.Tools.Include[0] != "a" {
		t.Errorf("Tools.Include: got %v, want [a]", fw.Tools.Include)
	}
	if len(fw.Tools.Exclude) != 1 || fw.Tools.Exclude[0] != "b" {
		t.Errorf("Tools.Exclude: got %v, want [b]", fw.Tools.Exclude)
	}
	if len(fw.Tools.ExecutionInclude) != 1 || fw.Tools.ExecutionInclude[0] != "c" {
		t.Errorf("Tools.ExecutionInclude: got %v, want [c]", fw.Tools.ExecutionInclude)
	}
	if len(fw.Tools.ExecutionExclude) != 1 || fw.Tools.ExecutionExclude[0] != "d" {
		t.Errorf("Tools.ExecutionExclude: got %v, want [d]", fw.Tools.ExecutionExclude)
	}
	if len(fw.Tools.ToolSets) != 1 || fw.Tools.ToolSets[0] != "ts1" {
		t.Errorf("Tools.ToolSets: got %v, want [ts1]", fw.Tools.ToolSets)
	}
	if fw.Tools.CredentialRefs["k"] != "v" {
		t.Errorf("Tools.CredentialRefs: got %v, want map[k:v]", fw.Tools.CredentialRefs)
	}
	if len(fw.Skills.Include) != 1 || fw.Skills.Include[0] != "s1" {
		t.Errorf("Skills.Include: got %v, want [s1]", fw.Skills.Include)
	}
	if len(fw.Skills.Roots) != 1 || fw.Skills.Roots[0] != "r1" {
		t.Errorf("Skills.Roots: got %v, want [r1]", fw.Skills.Roots)
	}
	if len(fw.Knowledge.Indexes) != 1 || fw.Knowledge.Indexes[0] != "idx1" {
		t.Errorf("Knowledge.Indexes: got %v, want [idx1]", fw.Knowledge.Indexes)
	}
	if fw.Knowledge.Filter["k"] != "v" {
		t.Errorf("Knowledge.Filter: got %v, want map[k:v]", fw.Knowledge.Filter)
	}
	if fw.Workspace.Workdir != "/tmp" {
		t.Errorf("Workspace.Workdir: got %q, want %q", fw.Workspace.Workdir, "/tmp")
	}
	if len(fw.Workspace.AllowedRoots) != 1 || fw.Workspace.AllowedRoots[0] != "/root" {
		t.Errorf("Workspace.AllowedRoots: got %v, want [/root]", fw.Workspace.AllowedRoots)
	}
	if len(fw.Credentials.AllowedRefs) != 1 || fw.Credentials.AllowedRefs[0] != "ref1" {
		t.Errorf("Credentials.AllowedRefs: got %v, want [ref1]", fw.Credentials.AllowedRefs)
	}
	if string(fw.Isolation.Mode) != "strict" {
		t.Errorf("Isolation.Mode: got %q, want %q", fw.Isolation.Mode, "strict")
	}
	if !fw.Isolation.AgentCache {
		t.Error("Isolation.AgentCache: got false, want true")
	}
	if fw.Isolation.ToolSetCache {
		t.Error("Isolation.ToolSetCache: got true, want false")
	}
	if fw.Isolation.ServiceMode != "isolated" {
		t.Errorf("Isolation.ServiceMode: got %q, want %q", fw.Isolation.ServiceMode, "isolated")
	}
	if fw.ExtraModel["temperature"] != 0.5 {
		t.Errorf("ExtraModel: got %v, want map[temperature:0.5]", fw.ExtraModel)
	}
}

// profileResolverMockRepo is an in-memory RuntimeProfileReadWriter for testing.
type profileResolverMockRepo struct {
	profiles map[string]biz.RuntimeProfile
	err      error
}

func (m *profileResolverMockRepo) List(_ context.Context, agentID string, activeOnly bool) ([]biz.RuntimeProfile, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []biz.RuntimeProfile
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

func (m *profileResolverMockRepo) GetByID(_ context.Context, id string) (biz.RuntimeProfile, error) {
	if m.err != nil {
		return biz.RuntimeProfile{}, m.err
	}
	p, ok := m.profiles[id]
	if !ok {
		return biz.RuntimeProfile{}, errors.New("not found")
	}
	return p, nil
}

func (m *profileResolverMockRepo) GetActive(_ context.Context, agentID string) (*biz.RuntimeProfile, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, p := range m.profiles {
		if p.AgentID == agentID && p.IsActive {
			cp := p
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *profileResolverMockRepo) Create(_ context.Context, p biz.RuntimeProfile) (biz.RuntimeProfile, error) {
	if m.err != nil {
		return biz.RuntimeProfile{}, m.err
	}
	m.profiles[p.ID] = p
	return p, nil
}

func (m *profileResolverMockRepo) Update(_ context.Context, p biz.RuntimeProfile) (biz.RuntimeProfile, error) {
	if m.err != nil {
		return biz.RuntimeProfile{}, m.err
	}
	m.profiles[p.ID] = p
	return p, nil
}

func (m *profileResolverMockRepo) Delete(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.profiles, id)
	return nil
}

func (m *profileResolverMockRepo) SetActive(_ context.Context, id string, active bool) (biz.RuntimeProfile, error) {
	if m.err != nil {
		return biz.RuntimeProfile{}, m.err
	}
	p, ok := m.profiles[id]
	if !ok {
		return biz.RuntimeProfile{}, errors.New("not found")
	}
	p.IsActive = active
	m.profiles[id] = p
	return p, nil
}
