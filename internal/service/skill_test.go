package service_test

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

// memSkillRepo is a minimal in-memory SkillRepo.
type memSkillRepo struct {
	items map[string]biz.Skill
}

func newMemSkillRepo() *memSkillRepo {
	return &memSkillRepo{items: make(map[string]biz.Skill)}
}

func (m *memSkillRepo) SearchSkills(_ context.Context, _ biz.SkillListQuery) (biz.SkillListResult, error) {
	out := make([]biz.Skill, 0, len(m.items))
	for _, s := range m.items {
		out = append(out, s)
	}
	return biz.SkillListResult{Items: out, Total: len(out)}, nil
}

func (m *memSkillRepo) GetSkillByID(_ context.Context, id string) (biz.Skill, error) {
	s, ok := m.items[id]
	if !ok {
		return biz.Skill{}, fmt.Errorf("skill not found: %s", id)
	}
	return s, nil
}

func (m *memSkillRepo) UpdateSkillEnabled(_ context.Context, id string, enabled bool) (biz.Skill, error) {
	s, ok := m.items[id]
	if !ok {
		return biz.Skill{}, fmt.Errorf("skill not found: %s", id)
	}
	s.Enabled = enabled
	m.items[id] = s
	return s, nil
}

func (m *memSkillRepo) DeleteSkill(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

func (m *memSkillRepo) DuplicateSkill(_ context.Context, id string) (biz.Skill, error) {
	s, ok := m.items[id]
	if !ok {
		return biz.Skill{}, fmt.Errorf("skill not found: %s", id)
	}
	newID := id + "-copy"
	s.ID = newID
	s.Name = s.Name + " (copy)"
	m.items[newID] = s
	return s, nil
}

func (m *memSkillRepo) GetSkillStorageDir(_ context.Context, id string) (string, error) {
	return "/tmp/skills/" + id, nil
}

func (m *memSkillRepo) SearchSkillInvocations(_ context.Context, _ biz.SkillRunQuery) (biz.SkillRunResult, error) {
	return biz.SkillRunResult{}, nil
}

func (m *memSkillRepo) ListSkillSimilaritySources(_ context.Context) ([]biz.SkillSimilaritySource, error) {
	return nil, nil
}

func (m *memSkillRepo) CreateSkillWithVersion(_ context.Context, in biz.SkillCreateInput) (biz.Skill, error) {
	s := biz.Skill{ID: "new-sk", Name: in.Name, Slug: in.Slug}
	m.items[s.ID] = s
	return s, nil
}

func (m *memSkillRepo) GetSkillBySkillKey(_ context.Context, _ string) (biz.Skill, error) {
	return biz.Skill{}, fmt.Errorf("not found")
}

func (m *memSkillRepo) UpsertSkillFromDisk(_ context.Context, in biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error) {
	return biz.Skill{}, biz.SkillDiskSyncOutcome{}, nil
}

func (m *memSkillRepo) ListRegisteredSlugs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *memSkillRepo) ListEnabledPublishedSkillKeys(_ context.Context) ([]string, error) {
	return nil, nil
}

func (m *memSkillRepo) ListEnabledPublishedSkillCandidates(_ context.Context) ([]biz.SkillRuntimeCandidate, error) {
	return nil, nil
}

func (m *memSkillRepo) RecordSkillInvocation(_ context.Context, _ biz.SkillInvocationWrite) error {
	return nil
}

func (m *memSkillRepo) GetLatestSkillMarkdown(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *memSkillRepo) PatchSkill(_ context.Context, id string, patch biz.SkillUpdateDraft) (biz.Skill, error) {
	s, ok := m.items[id]
	if !ok {
		return biz.Skill{}, fmt.Errorf("skill not found: %s", id)
	}
	if patch.HasName {
		s.Name = patch.Name
	}
	m.items[id] = s
	return s, nil
}

func (m *memSkillRepo) PublishSkill(_ context.Context, id string) (biz.Skill, error) {
	s, ok := m.items[id]
	if !ok {
		return biz.Skill{}, fmt.Errorf("skill not found: %s", id)
	}
	s.Status = "published"
	m.items[id] = s
	return s, nil
}

func (m *memSkillRepo) MarkSkillFilesystemMissing(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *memSkillRepo) FilesystemHealthStats(_ context.Context) (biz.SkillFilesystemHealthStats, error) {
	return biz.SkillFilesystemHealthStats{}, nil
}

func newSkillService() *service.SkillService {
	repo := newMemSkillRepo()
	repo.items["sk1"] = biz.Skill{ID: "sk1", Name: "Test Skill", Enabled: true, Status: "active"}
	return service.NewSkillService(biz.NewSkillUsecase(repo), nil, nil)
}

func TestSkillService_List(t *testing.T) {
	svc := newSkillService()
	ctx := context.Background()

	resp, err := svc.ListSkills(ctx, &v1.ListSkillsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Errorf("expected 1 skill, got %d", len(resp.GetItems()))
	}
}

func TestSkillService_ToggleEnabled(t *testing.T) {
	svc := newSkillService()
	ctx := context.Background()

	out, err := svc.ToggleSkillEnabled(ctx, &v1.ToggleSkillEnabledRequest{Id: "sk1", Enabled: false})
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if out.GetEnabled() {
		t.Error("expected enabled=false")
	}
}

func TestSkillService_Delete(t *testing.T) {
	svc := newSkillService()
	ctx := context.Background()

	_, err := svc.DeleteSkill(ctx, &v1.DeleteSkillRequest{Id: "sk1"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ := svc.ListSkills(ctx, &v1.ListSkillsRequest{})
	if len(list.GetItems()) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list.GetItems()))
	}
}

func TestSkillService_DuplicateSkill(t *testing.T) {
	svc := newSkillService()
	ctx := context.Background()

	copy, err := svc.DuplicateSkill(ctx, &v1.DuplicateSkillRequest{Id: "sk1"})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copy.GetId() == "sk1" {
		t.Error("duplicate should have a new ID")
	}
}
