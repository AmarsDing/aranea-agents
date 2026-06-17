package trpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// mockRuntimeReader implements skill.SkillRuntimeReader for testing.
type mockRuntimeReader struct {
	candidates []skill.RuntimeCandidate
	err        error
}

func (m *mockRuntimeReader) BatchGetSkillMarkdownBySlugs(_ context.Context, _ []string) (map[string]string, error) {
	return nil, nil
}

func (m *mockRuntimeReader) ListEnabledPublishedSkillKeys(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockRuntimeReader) ListEnabledPublishedSkillCandidates(_ context.Context) ([]skill.RuntimeCandidate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.candidates, nil
}

func (m *mockRuntimeReader) FilesystemHealthStats(_ context.Context) (skill.FilesystemHealthStats, error) {
	return skill.FilesystemHealthStats{}, nil
}

// mockLookupReader implements skill.SkillLookupReader for testing.
type mockLookupReader struct {
	skills    map[string]skill.Skill
	markdowns map[string]string
	dirs      map[string]string
	getErr    error
	mdErr     error
	dirErr    error
}

func (m *mockLookupReader) GetSkillByID(_ context.Context, id string) (skill.Skill, error) {
	return skill.Skill{}, nil
}

func (m *mockLookupReader) GetSkillBySkillKey(_ context.Context, skillKey string) (skill.Skill, error) {
	if m.getErr != nil {
		return skill.Skill{}, m.getErr
	}
	sk, ok := m.skills[skillKey]
	if !ok {
		return skill.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
	}
	return sk, nil
}

func (m *mockLookupReader) GetSkillStorageDir(_ context.Context, id string) (string, error) {
	if m.dirErr != nil {
		return "", m.dirErr
	}
	dir, ok := m.dirs[id]
	if !ok {
		return "", apierror.Internal("SKILL", "no storage dir")
	}
	return dir, nil
}

func (m *mockLookupReader) GetLatestSkillMarkdown(_ context.Context, skillID string) (string, error) {
	if m.mdErr != nil {
		return "", m.mdErr
	}
	md, ok := m.markdowns[skillID]
	if !ok {
		return "", apierror.NotFound(apierror.DomainSkill, "not found")
	}
	return md, nil
}

func TestDBStoreAdapter_ListSummaries(t *testing.T) {
	runtime := &mockRuntimeReader{
		candidates: []skill.RuntimeCandidate{
			{Slug: "my-skill", Name: "My Skill", Description: "A test skill"},
			{Slug: "another", Name: "Another", Description: "Another skill"},
		},
	}
	lookup := &mockLookupReader{}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	summaries, err := adapter.ListSummaries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	// Summary.Name should be the slug (canonical handle)
	if summaries[0].Name != "my-skill" {
		t.Errorf("expected name my-skill, got %s", summaries[0].Name)
	}
	if summaries[1].Name != "another" {
		t.Errorf("expected name another, got %s", summaries[1].Name)
	}
}

func TestDBStoreAdapter_ListSummaries_DisplayNameInDescription(t *testing.T) {
	runtime := &mockRuntimeReader{
		candidates: []skill.RuntimeCandidate{
			{Slug: "my-skill", Name: "My Display Name", Description: "A test skill"},
		},
	}
	lookup := &mockLookupReader{}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	summaries, err := adapter.ListSummaries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	// When display name differs from slug, it should be prefixed in description
	expected := "My Display Name — A test skill"
	if summaries[0].Description != expected {
		t.Errorf("expected description %q, got %q", expected, summaries[0].Description)
	}
}

func TestDBStoreAdapter_ListSummaries_Error(t *testing.T) {
	runtime := &mockRuntimeReader{err: apierror.Internal("SKILL", "db down")}
	lookup := &mockLookupReader{}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	_, err := adapter.ListSummaries(context.Background())
	if err == nil {
		t.Fatal("expected error from runtime reader")
	}
}

func TestDBStoreAdapter_GetByName(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{
		skills: map[string]skill.Skill{
			"my-skill": {ID: "skill_1", Slug: "my-skill", Description: "A test skill"},
		},
		markdowns: map[string]string{
			"skill_1": "# My Skill\nHello world",
		},
	}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	sk, err := adapter.GetByName(context.Background(), "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sk.Summary.Name != "my-skill" {
		t.Errorf("expected name my-skill, got %s", sk.Summary.Name)
	}
	if sk.Body != "# My Skill\nHello world" {
		t.Errorf("unexpected body: %s", sk.Body)
	}
}

func TestDBStoreAdapter_GetByName_NotFound(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{skills: map[string]skill.Skill{}}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	_, err := adapter.GetByName(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestDBStoreAdapter_GetByName_EmptySlug(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	_, err := adapter.GetByName(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty slug")
	}
	if err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestDBStoreAdapter_GetByName_MarkdownError_GracefulDegradation(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{
		skills: map[string]skill.Skill{
			"my-skill": {ID: "skill_1", Slug: "my-skill", Description: "A test skill"},
		},
		markdowns: map[string]string{},
		mdErr:     apierror.Internal("SKILL", "markdown fetch failed"),
	}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	sk, err := adapter.GetByName(context.Background(), "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Body should be empty (graceful degradation), not an error
	if sk.Body != "" {
		t.Errorf("expected empty body on markdown error, got %q", sk.Body)
	}
}

func TestDBStoreAdapter_GetPathByName(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{
		skills: map[string]skill.Skill{
			"my-skill": {ID: "skill_1", Slug: "my-skill"},
		},
		dirs: map[string]string{
			"skill_1": "/data/skills/my-skill",
		},
	}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	dir, err := adapter.GetPathByName(context.Background(), "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/data/skills/my-skill" {
		t.Errorf("expected /data/skills/my-skill, got %s", dir)
	}
}

func TestDBStoreAdapter_GetPathByName_NotFound(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{skills: map[string]skill.Skill{}}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	_, err := adapter.GetPathByName(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound, got %v", err)
	}
}

func TestDBStoreAdapter_GetPathByName_NoStorageDir(t *testing.T) {
	runtime := &mockRuntimeReader{}
	lookup := &mockLookupReader{
		skills: map[string]skill.Skill{
			"my-skill": {ID: "skill_1", Slug: "my-skill"},
		},
		dirs:   map[string]string{},
		dirErr: apierror.Internal("SKILL", "no storage dir"),
	}
	adapter := NewDBStoreAdapter(runtime, lookup, loggateway.NewNoop())

	dir, err := adapter.GetPathByName(context.Background(), "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DB-only skills return empty string for path
	if dir != "" {
		t.Errorf("expected empty string for no storage dir, got %s", dir)
	}
}
