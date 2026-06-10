package skill

import (
	"context"
	"errors"
	"testing"

	authpkg "aranea-agents/pkg/auth"
)

type mockRepo struct {
	skills      map[string]Skill
	invocations []SkillInvocation
	versions    []SkillVersionDetail
	markdown    map[string]string
	batchMD     map[string]map[string]string
	storageDir  map[string]string
	simSources  []SimilaritySource
	candidates  []RuntimeCandidate
	skillKeys   []string
	slugs       []string
	fsStats     FilesystemHealthStats

	createErr      error
	updateEnabled  map[string]bool
	patchResult    Skill
	publishResult  Skill
	rollbackResult Skill
	dupResult      Skill
	deleteErr      error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		skills:       make(map[string]Skill),
		markdown:     make(map[string]string),
		batchMD:      make(map[string]map[string]string),
		storageDir:   make(map[string]string),
		updateEnabled: make(map[string]bool),
	}
}

func (m *mockRepo) SearchSkills(ctx context.Context, q ListQuery) (ListResult, error) {
	var items []Skill
	for _, s := range m.skills {
		items = append(items, s)
	}
	return ListResult{Items: items, Total: len(items), Limit: q.Limit, Offset: q.Offset}, nil
}

func (m *mockRepo) GetSkillByID(ctx context.Context, id string) (Skill, error) {
	s, ok := m.skills[id]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	return s, nil
}

func (m *mockRepo) GetSkillBySkillKey(ctx context.Context, skillKey string) (Skill, error) {
	for _, s := range m.skills {
		if s.Slug == skillKey {
			return s, nil
		}
	}
	return Skill{}, errors.New("not found")
}

func (m *mockRepo) GetSkillStorageDir(ctx context.Context, id string) (string, error) {
	d, ok := m.storageDir[id]
	if !ok {
		return "", errors.New("not found")
	}
	return d, nil
}

func (m *mockRepo) GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error) {
	md, ok := m.markdown[skillID]
	if !ok {
		return "", errors.New("not found")
	}
	return md, nil
}

func (m *mockRepo) BatchGetSkillMarkdownBySlugs(ctx context.Context, slugs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, slug := range slugs {
		if m, ok := m.batchMD[slug]; ok {
			result[slug] = m[slug]
		}
	}
	return result, nil
}

func (m *mockRepo) ListRegisteredSlugs(ctx context.Context) ([]string, error) {
	return m.slugs, nil
}

func (m *mockRepo) ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error) {
	return m.skillKeys, nil
}

func (m *mockRepo) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]RuntimeCandidate, error) {
	return m.candidates, nil
}

func (m *mockRepo) ListSkillSimilaritySources(ctx context.Context) ([]SimilaritySource, error) {
	return m.simSources, nil
}

func (m *mockRepo) FilesystemHealthStats(ctx context.Context) (FilesystemHealthStats, error) {
	return m.fsStats, nil
}

func (m *mockRepo) SearchSkillInvocations(ctx context.Context, q RunQuery) (RunResult, error) {
	return RunResult{Items: m.invocations, Total: len(m.invocations), Limit: q.Limit, Offset: q.Offset}, nil
}

func (m *mockRepo) ListSkillVersions(ctx context.Context, q VersionListQuery) (VersionListResult, error) {
	return VersionListResult{Items: m.versions, Total: len(m.versions), Limit: q.Limit, Offset: q.Offset}, nil
}

func (m *mockRepo) CreateSkillWithVersion(ctx context.Context, in CreateInput) (Skill, error) {
	if m.createErr != nil {
		return Skill{}, m.createErr
	}
	s := Skill{ID: "new-1", Name: in.Name, Slug: in.Slug, Description: in.Description}
	m.skills[s.ID] = s
	return s, nil
}

func (m *mockRepo) UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (Skill, error) {
	s, ok := m.skills[id]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	s.Enabled = enabled
	m.skills[id] = s
	m.updateEnabled[id] = enabled
	return s, nil
}

func (m *mockRepo) DuplicateSkill(ctx context.Context, id string) (Skill, error) {
	s, ok := m.skills[id]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	dup := s
	dup.ID = "dup-1"
	dup.Name = s.Name + " (copy)"
	m.skills[dup.ID] = dup
	return dup, nil
}

func (m *mockRepo) DeleteSkill(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.skills, id)
	return nil
}

func (m *mockRepo) PatchSkill(ctx context.Context, id string, patch UpdateDraft) (Skill, error) {
	s, ok := m.skills[id]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	if patch.HasName {
		s.Name = patch.Name
	}
	if patch.HasDescription {
		s.Description = patch.Description
	}
	m.skills[id] = s
	return s, nil
}

func (m *mockRepo) PublishSkill(ctx context.Context, id string) (Skill, error) {
	s, ok := m.skills[id]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	s.Status = "published"
	m.skills[id] = s
	return s, nil
}

func (m *mockRepo) UpsertSkillFromDisk(ctx context.Context, in DiskSyncInput) (Skill, DiskSyncOutcome, error) {
	return Skill{}, DiskSyncOutcome{}, nil
}

func (m *mockRepo) MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	return nil
}

func (m *mockRepo) RecordSkillInvocation(ctx context.Context, in InvocationWrite) error {
	return nil
}

func (m *mockRepo) RollbackSkillVersion(ctx context.Context, skillID string, versionID string) (Skill, error) {
	s, ok := m.skills[skillID]
	if !ok {
		return Skill{}, errors.New("not found")
	}
	return s, nil
}

type mockEmbedder struct {
	embed      func(ctx context.Context, text string) ([]float32, error)
	embedBatch func(ctx context.Context, texts []string) ([][]float32, error)
}

func (m *mockEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if m.embed == nil {
		return []float32{0.1, 0.2, 0.3}, nil
	}
	return m.embed(ctx, text)
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.embedBatch == nil {
		result := make([][]float32, len(texts))
		for i := range texts {
			result[i] = []float32{0.1, 0.2, 0.3}
		}
		return result, nil
	}
	return m.embedBatch(ctx, texts)
}

func adminCtx() context.Context {
	return authpkg.NewContext(context.Background(), &authpkg.Auth{UserID: 1, Access: "admin"})
}

func nonAdminCtx() context.Context {
	return authpkg.NewContext(context.Background(), &authpkg.Auth{UserID: 2, Access: "viewer"})
}

func sampleSkill(id, name, slug string) Skill {
	return Skill{ID: id, Name: name, Slug: slug, Status: "published", Enabled: true}
}

func TestList_PaginationDefaults(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 20 {
		t.Errorf("expected default limit 20, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", result.Offset)
	}
}

func TestList_PaginationClampMax(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.List(context.Background(), ListQuery{Limit: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("expected clamped limit 100, got %d", result.Limit)
	}
}

func TestList_PaginationNegativeOffset(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.List(context.Background(), ListQuery{Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Offset != 0 {
		t.Errorf("expected clamped offset 0, got %d", result.Offset)
	}
}

func TestList_EnabledValidation(t *testing.T) {
	tests := []struct {
		name    string
		enabled string
		wantErr bool
	}{
		{"empty ok", "", false},
		{"true ok", "true", false},
		{"false ok", "false", false},
		{"invalid rejected", "yes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMockRepo()
			u := NewUsecase(r, nil)
			_, err := u.List(context.Background(), ListQuery{Enabled: tt.enabled})
			if (err != nil) != tt.wantErr {
				t.Errorf("enabled=%q err=%v wantErr=%v", tt.enabled, err, tt.wantErr)
			}
		})
	}
}

func TestList_StatusValidation(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"empty ok", "", false},
		{"draft ok", "draft", false},
		{"published ok", "published", false},
		{"archived ok", "archived", false},
		{"invalid rejected", "pending", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMockRepo()
			u := NewUsecase(r, nil)
			_, err := u.List(context.Background(), ListQuery{Status: tt.status})
			if (err != nil) != tt.wantErr {
				t.Errorf("status=%q err=%v wantErr=%v", tt.status, err, tt.wantErr)
			}
		})
	}
}

func TestList_PermissionApplied(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	result, err := u.List(adminCtx(), ListQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected at least one item")
	}
	if !result.Items[0].Permissions.CanEdit {
		t.Error("expected CanEdit=true for admin")
	}
}

func TestGet_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Get(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestGet_RepoCall(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.Get(context.Background(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "s1" {
		t.Errorf("expected id s1, got %s", s.ID)
	}
}

func TestGet_PermissionApplied(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.Get(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.Permissions.CanDelete {
		t.Error("expected CanDelete=true for admin")
	}
}

func TestCreate_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Create(nonAdminCtx(), CreateInput{Name: "n", Slug: "s"})
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestCreate_NameRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Create(adminCtx(), CreateInput{Slug: "s"})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestCreate_SlugRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Create(adminCtx(), CreateInput{Name: "n"})
	if err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestCreate_Success(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	s, err := u.Create(adminCtx(), CreateInput{Name: "MySkill", Slug: "my-skill"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "MySkill" {
		t.Errorf("expected name MySkill, got %s", s.Name)
	}
	if !s.Permissions.CanEdit {
		t.Error("expected CanEdit=true for admin")
	}
}

func TestToggleEnabled_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.ToggleEnabled(nonAdminCtx(), "s1", true)
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestToggleEnabled_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.ToggleEnabled(adminCtx(), "", true)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestToggleEnabled_EmbedCacheInvalidation(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	u.embedCache = map[string][]float32{"test": {0.1, 0.2}}
	_, err := u.ToggleEnabled(adminCtx(), "s1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.embedCache != nil {
		t.Error("expected embed cache to be invalidated")
	}
}

func TestDuplicate_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Duplicate(nonAdminCtx(), "s1")
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestDuplicate_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Duplicate(adminCtx(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestDuplicate_EmbedCacheInvalidation(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	u.embedCache = map[string][]float32{"test": {0.1, 0.2}}
	_, err := u.Duplicate(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.embedCache != nil {
		t.Error("expected embed cache to be invalidated")
	}
}

func TestDelete_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	err := u.Delete(nonAdminCtx(), "s1")
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestDelete_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	err := u.Delete(adminCtx(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestDelete_EmbedCacheInvalidation(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	u.embedCache = map[string][]float32{"test": {0.1, 0.2}}
	err := u.Delete(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.embedCache != nil {
		t.Error("expected embed cache to be invalidated")
	}
}

func TestPatch_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Patch(nonAdminCtx(), "s1", UpdateDraft{})
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestPatch_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Patch(adminCtx(), "", UpdateDraft{})
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestPatch_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Old", "old")
	u := NewUsecase(r, nil)
	s, err := u.Patch(adminCtx(), "s1", UpdateDraft{HasName: true, Name: "New"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "New" {
		t.Errorf("expected name New, got %s", s.Name)
	}
}

func TestPublish_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Publish(nonAdminCtx(), "s1")
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestPublish_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.Publish(adminCtx(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestPublish_EmbedCacheInvalidation(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	u.embedCache = map[string][]float32{"test": {0.1, 0.2}}
	s, err := u.Publish(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != "published" {
		t.Errorf("expected status published, got %s", s.Status)
	}
	if u.embedCache != nil {
		t.Error("expected embed cache to be invalidated")
	}
}

func TestSearchRuns_PaginationDefaults(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.SearchRuns(context.Background(), RunQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 20 {
		t.Errorf("expected default limit 20, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", result.Offset)
	}
}

func TestSearchRuns_StatusValidation(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"empty ok", "", false},
		{"success ok", "success", false},
		{"failure ok", "failure", false},
		{"pending ok", "pending", false},
		{"invalid rejected", "running", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMockRepo()
			u := NewUsecase(r, nil)
			_, err := u.SearchRuns(context.Background(), RunQuery{Status: tt.status})
			if (err != nil) != tt.wantErr {
				t.Errorf("status=%q err=%v wantErr=%v", tt.status, err, tt.wantErr)
			}
		})
	}
}

func TestSearchRuns_InvocationPermissions(t *testing.T) {
	r := newMockRepo()
	r.invocations = []SkillInvocation{{ID: "inv1", SkillID: "s1"}}
	u := NewUsecase(r, nil)
	result, err := u.SearchRuns(adminCtx(), RunQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected at least one invocation")
	}
	if !result.Items[0].Permissions.CanViewDetail {
		t.Error("expected CanViewDetail=true for admin")
	}
}

func TestSearchRuns_InvocationPermissionsNonAdmin(t *testing.T) {
	r := newMockRepo()
	r.invocations = []SkillInvocation{{ID: "inv1", SkillID: "s1"}}
	u := NewUsecase(r, nil)
	result, err := u.SearchRuns(nonAdminCtx(), RunQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected at least one invocation")
	}
	if result.Items[0].Permissions.CanViewDetail {
		t.Error("expected CanViewDetail=false for non-admin")
	}
}

func TestListVersions_EmptySkillID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.ListVersions(context.Background(), VersionListQuery{})
	if err == nil {
		t.Error("expected error for empty skill id")
	}
}

func TestListVersions_PaginationDefaults(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.ListVersions(context.Background(), VersionListQuery{SkillID: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 20 {
		t.Errorf("expected default limit 20, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", result.Offset)
	}
}

func TestListVersions_PaginationClamp(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.ListVersions(context.Background(), VersionListQuery{SkillID: "s1", Limit: 500, Offset: -10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("expected clamped limit 100, got %d", result.Limit)
	}
	if result.Offset != 0 {
		t.Errorf("expected clamped offset 0, got %d", result.Offset)
	}
}

func TestRollbackVersion_AdminAccessRequired(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.RollbackVersion(nonAdminCtx(), "s1", "v1")
	if err == nil {
		t.Error("expected forbidden for non-admin")
	}
}

func TestRollbackVersion_EmptySkillID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.RollbackVersion(adminCtx(), "", "v1")
	if err == nil {
		t.Error("expected error for empty skill id")
	}
}

func TestRollbackVersion_EmptyVersionID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.RollbackVersion(adminCtx(), "s1", "")
	if err == nil {
		t.Error("expected error for empty version id")
	}
}

func TestRollbackVersion_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.RollbackVersion(adminCtx(), "s1", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "s1" {
		t.Errorf("expected id s1, got %s", s.ID)
	}
}

func TestGetBySkillKey_EmptyKey(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.GetBySkillKey(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestGetBySkillKey_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.GetBySkillKey(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Slug != "test" {
		t.Errorf("expected slug test, got %s", s.Slug)
	}
}

func TestGetBySlug_EmptySlug(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.GetBySlug(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestGetBySlug_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.GetBySlug(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Slug != "test" {
		t.Errorf("expected slug test, got %s", s.Slug)
	}
}

func TestBatchGetSkillGuidance_EmptySlugs(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.BatchGetSkillGuidance(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty slugs, got %v", result)
	}
}

func TestBatchGetSkillGuidance_RepoCall(t *testing.T) {
	r := newMockRepo()
	r.batchMD["slug-a"] = map[string]string{"slug-a": "# A guidance"}
	r.batchMD["slug-b"] = map[string]string{"slug-b": "# B guidance"}
	u := NewUsecase(r, nil)
	result, err := u.BatchGetSkillGuidance(context.Background(), []string{"slug-a", "slug-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
}

func TestBatchGetSkillGuidance_SlugOrdering(t *testing.T) {
	r := newMockRepo()
	r.batchMD["alpha"] = map[string]string{"alpha": "# Alpha"}
	r.batchMD["beta"] = map[string]string{"beta": "# Beta"}
	u := NewUsecase(r, nil)
	result, err := u.BatchGetSkillGuidance(context.Background(), []string{"beta", "alpha"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result[0].Slug != "beta" {
		t.Errorf("expected first slug beta, got %s", result[0].Slug)
	}
	if result[1].Slug != "alpha" {
		t.Errorf("expected second slug alpha, got %s", result[1].Slug)
	}
}

func TestScoreByEmbedding_NilEmbedder(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.ScoreByEmbedding(context.Background(), "query", []RuntimeCandidate{{Slug: "a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil embedder, got %v", result)
	}
}

func TestScoreByEmbedding_EmptyQuery(t *testing.T) {
	r := newMockRepo()
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	result, err := u.ScoreByEmbedding(context.Background(), "", []RuntimeCandidate{{Slug: "a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty query, got %v", result)
	}
}

func TestScoreByEmbedding_EmptyCandidates(t *testing.T) {
	r := newMockRepo()
	emb := &mockEmbedder{}
	u := NewUsecase(r, emb)
	result, err := u.ScoreByEmbedding(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty candidates, got %v", result)
	}
}

func TestScoreByEmbedding_Success(t *testing.T) {
	r := newMockRepo()
	emb := &mockEmbedder{
		embed: func(ctx context.Context, text string) ([]float32, error) {
			return []float32{1.0, 0.0, 0.0}, nil
		},
		embedBatch: func(ctx context.Context, texts []string) ([][]float32, error) {
			result := make([][]float32, len(texts))
			for i := range texts {
				result[i] = []float32{1.0, 0.0, 0.0}
			}
			return result, nil
		},
	}
	u := NewUsecase(r, emb)
	candidates := []RuntimeCandidate{{Slug: "skill-a", Name: "A", Description: "desc"}}
	scores, err := u.ScoreByEmbedding(context.Background(), "query", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := scores["skill-a"]; !ok {
		t.Error("expected score for skill-a")
	}
}

func TestScoreByEmbedding_CacheRefresh(t *testing.T) {
	r := newMockRepo()
	callCount := 0
	emb := &mockEmbedder{
		embed: func(ctx context.Context, text string) ([]float32, error) {
			return []float32{1.0, 0.0}, nil
		},
		embedBatch: func(ctx context.Context, texts []string) ([][]float32, error) {
			callCount++
			result := make([][]float32, len(texts))
			for i := range texts {
				result[i] = []float32{1.0, 0.0}
			}
			return result, nil
		},
	}
	u := NewUsecase(r, emb)
	candidates := []RuntimeCandidate{{Slug: "a", Name: "A", Description: "d"}}
	if _, err := u.ScoreByEmbedding(context.Background(), "q1", candidates); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 embedBatch call, got %d", callCount)
	}
	if _, err := u.ScoreByEmbedding(context.Background(), "q2", candidates); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected embedBatch not called again (cached), got %d", callCount)
	}
}

func TestScoreByEmbedding_CacheInvalidation(t *testing.T) {
	r := newMockRepo()
	callCount := 0
	emb := &mockEmbedder{
		embed: func(ctx context.Context, text string) ([]float32, error) {
			return []float32{1.0, 0.0}, nil
		},
		embedBatch: func(ctx context.Context, texts []string) ([][]float32, error) {
			callCount++
			result := make([][]float32, len(texts))
			for i := range texts {
				result[i] = []float32{1.0, 0.0}
			}
			return result, nil
		},
	}
	u := NewUsecase(r, emb)
	candidates := []RuntimeCandidate{{Slug: "a", Name: "A", Description: "d"}}
	if _, err := u.ScoreByEmbedding(context.Background(), "q1", candidates); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 embedBatch call, got %d", callCount)
	}
	u.InvalidateEmbedCache()
	if _, err := u.ScoreByEmbedding(context.Background(), "q2", candidates); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 embedBatch calls after invalidation, got %d", callCount)
	}
}

func TestParseRuntimePolicy_EmptyInput(t *testing.T) {
	p := ParseRuntimePolicy("")
	if !p.IntentRoutingEnabled {
		t.Error("expected IntentRoutingEnabled=true by default")
	}
	if p.IntentMaxPaths != 3 {
		t.Errorf("expected IntentMaxPaths=3, got %d", p.IntentMaxPaths)
	}
	if p.MaxSkillsInToolset != 32 {
		t.Errorf("expected MaxSkillsInToolset=32, got %d", p.MaxSkillsInToolset)
	}
	if p.EmbeddingScoreWeight != 0.3 {
		t.Errorf("expected EmbeddingScoreWeight=0.3, got %f", p.EmbeddingScoreWeight)
	}
}

func TestParseRuntimePolicy_Defaults(t *testing.T) {
	p := ParseRuntimePolicy("{}")
	if !p.IntentRoutingEnabled {
		t.Error("expected IntentRoutingEnabled=true by default")
	}
	if p.IntentMaxPaths != 3 {
		t.Errorf("expected IntentMaxPaths=3, got %d", p.IntentMaxPaths)
	}
	if p.MaxSkillsInToolset != 32 {
		t.Errorf("expected MaxSkillsInToolset=32, got %d", p.MaxSkillsInToolset)
	}
}

func TestParseRuntimePolicy_CustomValues(t *testing.T) {
	raw := `{"intent_routing_enabled":false,"intent_max_paths":5,"max_skills_in_toolset":64}`
	p := ParseRuntimePolicy(raw)
	if p.IntentRoutingEnabled {
		t.Error("expected IntentRoutingEnabled=false")
	}
	if p.IntentMaxPaths != 5 {
		t.Errorf("expected IntentMaxPaths=5, got %d", p.IntentMaxPaths)
	}
	if p.MaxSkillsInToolset != 64 {
		t.Errorf("expected MaxSkillsInToolset=64, got %d", p.MaxSkillsInToolset)
	}
}

func TestParseRuntimePolicy_EmbeddingScoringConfig(t *testing.T) {
	raw := `{"embedding_scoring_enabled":true,"embedding_score_weight":0.7}`
	p := ParseRuntimePolicy(raw)
	if !p.EmbeddingScoringEnabled {
		t.Error("expected EmbeddingScoringEnabled=true")
	}
	if p.EmbeddingScoreWeight != 0.7 {
		t.Errorf("expected EmbeddingScoreWeight=0.7, got %f", p.EmbeddingScoreWeight)
	}
}

func TestParseRuntimePolicy_Clamping(t *testing.T) {
	raw := `{"max_skills_in_toolset":500,"embedding_score_weight":2.0}`
	p := ParseRuntimePolicy(raw)
	if p.MaxSkillsInToolset != 256 {
		t.Errorf("expected MaxSkillsInToolset clamped to 256, got %d", p.MaxSkillsInToolset)
	}
	if p.EmbeddingScoreWeight != 1.0 {
		t.Errorf("expected EmbeddingScoreWeight clamped to 1.0, got %f", p.EmbeddingScoreWeight)
	}
}

func TestParseRuntimePolicy_InvalidJSON(t *testing.T) {
	p := ParseRuntimePolicy("{invalid}")
	if !p.IntentRoutingEnabled {
		t.Error("expected IntentRoutingEnabled=true as fallback")
	}
}

func TestParseRuntimePolicy_SliceNormalization(t *testing.T) {
	raw := `{"allowed_slugs":["A","a"," B "],"denied_slugs":["X","x"],"allowed_tags":["Tag","TAG"]}`
	p := ParseRuntimePolicy(raw)
	if len(p.AllowedSlugs) != 2 {
		t.Errorf("expected 2 unique allowed_slugs after normalization, got %d: %v", len(p.AllowedSlugs), p.AllowedSlugs)
	}
	if len(p.DeniedSlugs) != 1 {
		t.Errorf("expected 1 unique denied_slug after normalization, got %d: %v", len(p.DeniedSlugs), p.DeniedSlugs)
	}
	if len(p.AllowedTags) != 1 {
		t.Errorf("expected 1 unique allowed_tag after normalization, got %d: %v", len(p.AllowedTags), p.AllowedTags)
	}
}

func TestApplySkillPermission_NilAuth(t *testing.T) {
	s := &Skill{}
	applySkillPermission(context.Background(), s)
	if s.Permissions.CanEdit || s.Permissions.CanDelete || s.Permissions.CanToggleEnabled || s.Permissions.CanDuplicate {
		t.Error("expected all permissions false for nil auth")
	}
}

func TestApplySkillPermission_AdminAuth(t *testing.T) {
	s := &Skill{}
	applySkillPermission(adminCtx(), s)
	if !s.Permissions.CanEdit || !s.Permissions.CanDelete || !s.Permissions.CanToggleEnabled || !s.Permissions.CanDuplicate {
		t.Error("expected all permissions true for admin")
	}
}

func TestApplySkillPermission_NonAdminAuth(t *testing.T) {
	s := &Skill{}
	applySkillPermission(nonAdminCtx(), s)
	if !s.Permissions.CanEdit {
		t.Error("expected CanEdit=true for non-admin")
	}
	if s.Permissions.CanDelete {
		t.Error("expected CanDelete=false for non-admin")
	}
	if !s.Permissions.CanToggleEnabled {
		t.Error("expected CanToggleEnabled=true for non-admin")
	}
	if !s.Permissions.CanDuplicate {
		t.Error("expected CanDuplicate=true for non-admin")
	}
}

func TestRequireAdminAccess_NilAuth(t *testing.T) {
	err := requireAdminAccess(context.Background())
	if err == nil {
		t.Error("expected error for nil auth")
	}
}

func TestRequireAdminAccess_NonAdmin(t *testing.T) {
	err := requireAdminAccess(nonAdminCtx())
	if err == nil {
		t.Error("expected error for non-admin")
	}
}

func TestRequireAdminAccess_Admin(t *testing.T) {
	err := requireAdminAccess(adminCtx())
	if err != nil {
		t.Errorf("expected no error for admin, got %v", err)
	}
}

func TestCosineSimilarity32_IdenticalVectors(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	score := cosineSimilarity32(v, v)
	if score < 0.9999 || score > 1.0001 {
		t.Errorf("expected ~1.0 for identical vectors, got %f", score)
	}
}

func TestCosineSimilarity32_OrthogonalVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	score := cosineSimilarity32(a, b)
	if score != 0 {
		t.Errorf("expected 0 for orthogonal vectors, got %f", score)
	}
}

func TestCosineSimilarity32_ZeroVectors(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 2.0}
	score := cosineSimilarity32(a, b)
	if score != 0 {
		t.Errorf("expected 0 for zero vector, got %f", score)
	}
}

func TestCosineSimilarity32_DifferentLengthVectors(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0, 2.0, 3.0}
	score := cosineSimilarity32(a, b)
	if score != 0 {
		t.Errorf("expected 0 for different length vectors, got %f", score)
	}
}

func TestCosineSimilarity32_EmptyVectors(t *testing.T) {
	score := cosineSimilarity32([]float32{}, []float32{})
	if score != 0 {
		t.Errorf("expected 0 for empty vectors, got %f", score)
	}
}

func TestCosineSimilarity32_KnownValues(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{4.0, 5.0, 6.0}
	score := cosineSimilarity32(a, b)
	expected := float64(1*4+2*5+3*6) / (sqrt64(1+4+9) * sqrt64(16+25+36))
	if score < expected-0.001 || score > expected+0.001 {
		t.Errorf("expected ~%f, got %f", expected, score)
	}
}

func TestGetLatestMarkdown_EmptyID(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	_, err := u.GetLatestMarkdown(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestGetLatestMarkdown_Success(t *testing.T) {
	r := newMockRepo()
	r.markdown["s1"] = "# Hello"
	u := NewUsecase(r, nil)
	md, err := u.GetLatestMarkdown(context.Background(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md != "# Hello" {
		t.Errorf("expected '# Hello', got %q", md)
	}
}

func TestMarkFilesystemMissing_EmptySlug(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	err := u.MarkFilesystemMissing(context.Background(), "", true)
	if err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestMarkFilesystemMissing_Success(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	err := u.MarkFilesystemMissing(context.Background(), "my-skill", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchRuns_PaginationClampMax(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.SearchRuns(context.Background(), RunQuery{Limit: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("expected clamped limit 100, got %d", result.Limit)
	}
}

func TestSearchRuns_NegativeOffset(t *testing.T) {
	r := newMockRepo()
	u := NewUsecase(r, nil)
	result, err := u.SearchRuns(context.Background(), RunQuery{Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Offset != 0 {
		t.Errorf("expected clamped offset 0, got %d", result.Offset)
	}
}

func TestToggleEnabled_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.ToggleEnabled(adminCtx(), "s1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Enabled {
		t.Error("expected enabled=false")
	}
	if r.updateEnabled["s1"] != false {
		t.Error("expected repo UpdateSkillEnabled called with false")
	}
}

func TestDuplicate_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.Duplicate(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "dup-1" {
		t.Errorf("expected dup-1, got %s", s.ID)
	}
}

func TestDelete_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	err := u.Delete(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.skills["s1"]; ok {
		t.Error("expected skill to be deleted")
	}
}

func TestPublish_Success(t *testing.T) {
	r := newMockRepo()
	r.skills["s1"] = sampleSkill("s1", "Test", "test")
	u := NewUsecase(r, nil)
	s, err := u.Publish(adminCtx(), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != "published" {
		t.Errorf("expected status published, got %s", s.Status)
	}
}

func TestNormalizeLowerSlice_Dedup(t *testing.T) {
	s := &[]string{"a", "A", "a"}
	normalizeLowerSlice(s)
	if len(*s) != 1 {
		t.Errorf("expected 1 unique item, got %d: %v", len(*s), *s)
	}
	if (*s)[0] != "a" {
		t.Errorf("expected 'a', got %q", (*s)[0])
	}
}

func TestNormalizeLowerSlice_TrimAndLower(t *testing.T) {
	s := &[]string{" A ", "B", "  c  "}
	normalizeLowerSlice(s)
	if len(*s) != 3 {
		t.Fatalf("expected 3 items, got %d", len(*s))
	}
	if (*s)[0] != "a" {
		t.Errorf("expected 'a', got %q", (*s)[0])
	}
	if (*s)[2] != "c" {
		t.Errorf("expected 'c', got %q", (*s)[2])
	}
}

func TestNormalizeLowerSlice_EmptyStrings(t *testing.T) {
	s := &[]string{"", "  ", "a"}
	normalizeLowerSlice(s)
	if len(*s) != 1 {
		t.Errorf("expected 1 non-empty item, got %d: %v", len(*s), *s)
	}
}

func TestNormalizeLowerSlice_NilSlice(t *testing.T) {
	s := &[]string{}
	normalizeLowerSlice(s)
	if len(*s) != 0 {
		t.Errorf("expected 0 items, got %d", len(*s))
	}
}

func sqrt64(v float64) float64 {
	if v <= 0 {
		return 0
	}
	z := v
	for i := 0; i < 20; i++ {
		z = (z + v/z) / 2
	}
	return z
}
