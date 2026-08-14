package skill

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type mockTagRepo struct {
	tags         []TagInfo
	names        []string
	createResult TagInfo
	createErr    error
	renameCount  int
	renameErr    error
	deleteCount  int
	deleteErr    error

	createdName string
	renamedOld  string
	renamedNew  string
	deletedName string
}

func (m *mockTagRepo) ListSkillTags(ctx context.Context) ([]TagInfo, error) {
	return m.tags, nil
}

func (m *mockTagRepo) ListSkillTagNames(ctx context.Context) ([]string, error) {
	return m.names, nil
}

func (m *mockTagRepo) CreateSkillTag(ctx context.Context, name string) (TagInfo, error) {
	m.createdName = name
	return m.createResult, m.createErr
}

func (m *mockTagRepo) RenameSkillTag(ctx context.Context, oldName, newName string) (int, error) {
	m.renamedOld, m.renamedNew = oldName, newName
	return m.renameCount, m.renameErr
}

func (m *mockTagRepo) DeleteSkillTag(ctx context.Context, name string) (int, error) {
	m.deletedName = name
	return m.deleteCount, m.deleteErr
}

func TestNormalizeTagName(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantDim  string
		wantErr  bool
	}{
		{"figma", "figma", "", false},
		{"  Figma  ", "figma", "", false},
		{"file_type:XLSX", "file_type:xlsx", "file_type", false},
		{"domain:sales", "domain:sales", "domain", false},
		{"my-tag_v2", "my-tag_v2", "", false},
		{"", "", "", true},
		{"   ", "", "", true},
		{":bad", "", "", true},
		{"bad:", "", "", true},
		{"UPPER ok", "", "", true}, // 空格不允许
		{"中文标签", "", "", true},
		{"a:b:c", "", "", true},
	}
	for _, c := range cases {
		name, dim, err := normalizeTagName(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("normalizeTagName(%q): expected error, got name=%q", c.in, name)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeTagName(%q): unexpected error %v", c.in, err)
			continue
		}
		if name != c.wantName || dim != c.wantDim {
			t.Errorf("normalizeTagName(%q) = (%q, %q), want (%q, %q)", c.in, name, dim, c.wantName, c.wantDim)
		}
	}
}

func TestListTags_NilRepo(t *testing.T) {
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	_, err := u.ListTags(context.Background())
	if err == nil {
		t.Fatal("expected error when tag repo not configured")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != apierror.CodeInternal {
		t.Errorf("expected Internal apierror, got %v", err)
	}
}

func TestListTags_Delegates(t *testing.T) {
	tr := &mockTagRepo{tags: []TagInfo{
		{Name: "domain:sales", Dimension: "domain", Source: "user", UsedCount: 3},
		{Name: "figma", Dimension: "", Source: "system", UsedCount: 1},
	}}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	got, err := u.ListTags(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Name != "domain:sales" || got[0].UsedCount != 3 {
		t.Errorf("unexpected tags: %+v", got)
	}
}

func TestCreateTag_NormalizesInput(t *testing.T) {
	tr := &mockTagRepo{createResult: TagInfo{Name: "file_type:xlsx", Dimension: "file_type", Source: "user"}}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	got, err := u.CreateTag(context.Background(), "  File_Type:XLSX ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.createdName != "file_type:xlsx" {
		t.Errorf("repo received %q, want normalized %q", tr.createdName, "file_type:xlsx")
	}
	if got.Name != "file_type:xlsx" {
		t.Errorf("got %+v", got)
	}
}

func TestCreateTag_InvalidFormat(t *testing.T) {
	tr := &mockTagRepo{}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	_, err := u.CreateTag(context.Background(), "bad tag!")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != apierror.CodeBadRequest {
		t.Errorf("expected BadRequest, got %v", err)
	}
	if tr.createdName != "" {
		t.Error("repo must not be called on invalid input")
	}
}

func TestCreateTag_Conflict(t *testing.T) {
	tr := &mockTagRepo{createErr: apierror.Conflict("SKILL", "tag already exists")}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	_, err := u.CreateTag(context.Background(), "figma")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != apierror.CodeConflict {
		t.Errorf("expected Conflict, got %v", err)
	}
}

func TestRenameTag_InvalidatesCaches(t *testing.T) {
	tr := &mockTagRepo{renameCount: 5}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	u.embedCache = map[string]embedEntry{"a": {vector: []float32{1}, cachedAt: time.Now()}}

	n, err := u.RenameTag(context.Background(), "Figma", "design-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("rewritten = %d, want 5", n)
	}
	if tr.renamedOld != "figma" || tr.renamedNew != "design-tool" {
		t.Errorf("repo got (%q, %q), want (figma, design-tool)", tr.renamedOld, tr.renamedNew)
	}
	if len(u.embedCache) != 0 {
		t.Error("embed cache must be fully invalidated after rename")
	}
}

func TestRenameTag_SameName(t *testing.T) {
	tr := &mockTagRepo{}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	_, err := u.RenameTag(context.Background(), "figma", " FIGMA ")
	if err == nil {
		t.Fatal("expected error for identical names after normalization")
	}
	if tr.renamedOld != "" {
		t.Error("repo must not be called for identical names")
	}
}

func TestRenameTag_RepoErrorKeepsCache(t *testing.T) {
	tr := &mockTagRepo{renameErr: errors.New("db down")}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	u.embedCache = map[string]embedEntry{"a": {vector: []float32{1}, cachedAt: time.Now()}}
	_, err := u.RenameTag(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("expected repo error")
	}
	if len(u.embedCache) != 1 {
		t.Error("embed cache must survive failed rename")
	}
}

func TestDeleteTag_InvalidatesCaches(t *testing.T) {
	tr := &mockTagRepo{deleteCount: 2}
	u := NewUsecase(newMockRepo(), nil, loggateway.NewNoop())
	u.SetTagRepo(tr)
	u.embedCache = map[string]embedEntry{"a": {vector: []float32{1}, cachedAt: time.Now()}}

	n, err := u.DeleteTag(context.Background(), " Figma ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("rewritten = %d, want 2", n)
	}
	if tr.deletedName != "figma" {
		t.Errorf("repo got %q, want figma", tr.deletedName)
	}
	if len(u.embedCache) != 0 {
		t.Error("embed cache must be fully invalidated after delete")
	}
}
