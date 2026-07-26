package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/loggateway"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeSkillVersionWriter struct {
	got    skill.CreateVersionInput
	result skill.SkillVersionDetail
	err    error
	called bool
}

func (f *fakeSkillVersionWriter) CreateSkillVersion(_ context.Context, in skill.CreateVersionInput) (skill.SkillVersionDetail, error) {
	f.called = true
	f.got = in
	if f.err != nil {
		return skill.SkillVersionDetail{}, f.err
	}
	return f.result, nil
}

type fakeSkillQueryReader struct {
	versions    []skill.SkillVersionDetail
	versionsErr error
}

func (f *fakeSkillQueryReader) SearchSkills(context.Context, skill.ListQuery) (skill.ListResult, error) {
	return skill.ListResult{}, errors.New("not implemented")
}
func (f *fakeSkillQueryReader) SearchSkillInvocations(context.Context, skill.RunQuery) (skill.RunResult, error) {
	return skill.RunResult{}, errors.New("not implemented")
}
func (f *fakeSkillQueryReader) ListSkillVersions(_ context.Context, q skill.VersionListQuery) (skill.VersionListResult, error) {
	if f.versionsErr != nil {
		return skill.VersionListResult{}, f.versionsErr
	}
	return skill.VersionListResult{Items: f.versions, Total: len(f.versions)}, nil
}
func (f *fakeSkillQueryReader) ListSkillSimilaritySources(context.Context) ([]skill.SimilaritySource, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeSkillQueryReader) ListRegisteredSlugs(context.Context) ([]string, error) {
	return nil, errors.New("not implemented")
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSkillVersionReloader_ExplicitParent(t *testing.T) {
	writer := &fakeSkillVersionWriter{result: skill.SkillVersionDetail{ID: "sv_new", Version: "1.0.1"}}
	reader := &fakeSkillQueryReader{}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	err := r.Reload(context.Background(), "sk-1", "# New Body\n", "sv_parent", "evolution: improve fix_failure")
	if err != nil {
		t.Fatalf("Reload error: %v", err)
	}
	if !writer.called {
		t.Fatal("CreateSkillVersion not called")
	}
	if writer.got.SkillID != "sk-1" || writer.got.Body != "# New Body" {
		t.Fatalf("unexpected input: %+v", writer.got)
	}
	if writer.got.ParentVersionID != "sv_parent" {
		t.Fatalf("explicit parent not propagated: %q", writer.got.ParentVersionID)
	}
	if writer.got.EvolutionReason == "" {
		t.Fatal("evolution reason not propagated")
	}
}

func TestSkillVersionReloader_ResolveParentFromLatest(t *testing.T) {
	writer := &fakeSkillVersionWriter{result: skill.SkillVersionDetail{ID: "sv_new"}}
	reader := &fakeSkillQueryReader{
		versions: []skill.SkillVersionDetail{{ID: "sv_latest", Version: "1.0.0"}},
	}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	err := r.Reload(context.Background(), "sk-1", "# New Body\n", "", "reason")
	if err != nil {
		t.Fatalf("Reload error: %v", err)
	}
	if writer.got.ParentVersionID != "sv_latest" {
		t.Fatalf("parent should resolve to latest version, got %q", writer.got.ParentVersionID)
	}
}

func TestSkillVersionReloader_NoVersionNoParent(t *testing.T) {
	writer := &fakeSkillVersionWriter{}
	reader := &fakeSkillQueryReader{versions: nil}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	err := r.Reload(context.Background(), "sk-1", "# New Body\n", "", "reason")
	if err == nil {
		t.Fatal("expected error when no parent anchor available")
	}
	if writer.called {
		t.Fatal("must not create orphan version without parent anchor")
	}
}

func TestSkillVersionReloader_LookupFailure(t *testing.T) {
	writer := &fakeSkillVersionWriter{}
	reader := &fakeSkillQueryReader{versionsErr: errors.New("db down")}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	err := r.Reload(context.Background(), "sk-1", "# New Body\n", "", "reason")
	if err == nil {
		t.Fatal("expected error when version lookup fails")
	}
	if writer.called {
		t.Fatal("must not create version when lookup fails")
	}
}

func TestSkillVersionReloader_EmptyDraft(t *testing.T) {
	writer := &fakeSkillVersionWriter{}
	reader := &fakeSkillQueryReader{}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	if err := r.Reload(context.Background(), "sk-1", "  ", "sv_p", "reason"); err == nil {
		t.Fatal("expected error for empty draft body")
	}
	if writer.called {
		t.Fatal("must not create version for empty draft")
	}
}

func TestSkillVersionReloader_CreateFailure(t *testing.T) {
	writer := &fakeSkillVersionWriter{err: errors.New("unique constraint")}
	reader := &fakeSkillQueryReader{}
	r := NewSkillVersionReloader(writer, reader, loggateway.NewNoop())

	if err := r.Reload(context.Background(), "sk-1", "# B\n", "sv_p", "reason"); err == nil {
		t.Fatal("expected error propagation from CreateSkillVersion")
	}
}
